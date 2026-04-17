package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
)

// commandRun describes a CLI command's lifecycle for runCommand. It collapses
// the shared parse → validate → resolve-config → branch-on-request-file →
// build-request → normalize → execute → post-process → write pipeline that
// every command in this package reproduces.
//
// Type parameters: Req is the command's request DTO; Res is the result value
// type held by app.Response (the runner receives *Res from the app layer and
// passes it to the renderer as-is).
type commandRun[Req any, Res any] struct {
	Name  string
	Usage string

	Options commandRunOptions

	// BindFlags registers command-specific flags on fs. The runner pre-registers
	// --config, --format, optionally --timings and --request-file per Options.
	BindFlags func(fs *flag.FlagSet)

	// InlineFlagsSet reports whether any fine-grained inline flag is set, used
	// to reject combinations with --request-file. Required when
	// Options.RequestFile is true.
	InlineFlagsSet func(fs *flag.FlagSet) bool

	// LoadRequestFile parses the request JSON and may run follow-up reads.
	// Contract:
	//   - On success (nil error), the returned pointer MUST be non-nil; the
	//     runner dereferences it to build the request passed to Normalize and
	//     Execute. Returning (nil, nil) is a programmer error.
	//   - On error, nil means no request should surface in the envelope (e.g.
	//     JSON parse failure). Non-nil means the pointee is a partial request
	//     (e.g. JSON parsed but a follow-up diff-file resolution failed) and
	//     the runner writes it into the error envelope.
	// Required when Options.RequestFile is true.
	LoadRequestFile func(ctx context.Context, cfg *config.Config, trimmedPath string) (*Req, error)

	// BuildRequest builds the request from the inline flags. cfg is non-nil
	// iff Options.ConfigForFlags is true. positional holds any positional args
	// captured under Options.ExactPositional. Required.
	BuildRequest func(ctx context.Context, cfg *config.Config, resolvedConfigPath string, positional []string) (Req, error)

	// Normalize (optional) runs after the request is composed, before Execute,
	// and fires for both the --request-file path and the inline-flags path.
	// format is the resolved output format so Normalize can reject
	// format-incompatible request shapes. On error, the runner writes the
	// returned Req into the envelope, so Normalize can return a pre- or
	// post-normalize request as appropriate.
	Normalize func(ctx context.Context, req Req, format string) (Req, error)

	// Execute calls the app layer. format is the resolved output format so
	// Execute can dispatch on it (e.g. for interactive text-mode paths).
	// Required.
	Execute func(ctx context.Context, resolvedConfigPath string, req Req, format string) (Req, *Res, *app.Issue)

	// PostProcess (optional) runs after Execute succeeds. When it returns a
	// non-nil cliIssue, the runner writes it with the returned exit code. An
	// exit code of 0 is treated as the default 2 so the common "validation
	// error" case stays terse at the call site.
	PostProcess func(ctx context.Context, resolvedConfigPath string, req Req, res *Res) (*Res, *cliIssue, int)
}

// commandRunOptions toggles the shared flags and config-loading behavior the
// runner itself controls.
type commandRunOptions struct {
	// RequestFile enables the --request-file flag and the mutual-exclusion
	// branch. Requires LoadRequestFile and InlineFlagsSet.
	RequestFile bool
	// Timings enables the --timings flag (JSON-only timing metadata).
	Timings bool
	// AcceptsPositional, when false (default), rejects fs.NArg() != 0 unless
	// ExactPositional is set.
	AcceptsPositional bool
	// ExactPositional, when > 0, requires exactly that many positional
	// arguments. Mutually exclusive with AcceptsPositional; takes precedence
	// when both are set. The captured args are passed to BuildRequest.
	ExactPositional int
	// Standalone, when true, disables the shared --config flag and omits the
	// "shared config resolution" help line. The resolved config path is still
	// computed from the project default, so Execute callbacks can load a
	// config opportunistically when present.
	Standalone bool
	// ConfigForFlags loads config before calling BuildRequest.
	ConfigForFlags bool
	// ConfigForFile loads config before calling LoadRequestFile.
	ConfigForFile bool
}

// cliIssueError lets BuildRequest/LoadRequestFile/Normalize callbacks surface a
// classified cliIssue (Code/Message/Details/ExitCode) through the standard
// error channel. The runner unwraps it via errors.As and writes the embedded
// issue directly, rather than defaulting to validation_error/exit 2.
type cliIssueError struct {
	issue    cliIssue
	exitCode int
}

func (e *cliIssueError) Error() string {
	return e.issue.Message
}

// configLoadError wraps a config.Load failure surfaced from a BuildRequest
// callback so the runner classifies it as a config_error rather than the
// default validation_error.
func configLoadError(err error) error {
	return &cliIssueError{
		issue:    cliIssue{Code: "config_error", Message: err.Error()},
		exitCode: 2,
	}
}

// plainIssue wraps a bare error as an *app.Issue with the given code and a
// default exit code of 2. Returns nil when err is nil, so Execute callbacks
// can pipe `return req, nil, plainIssue(err, "foo_error")` without a branch.
func plainIssue(err error, code string) *app.Issue {
	if err == nil {
		return nil
	}
	return &app.Issue{
		Code:     code,
		Message:  err.Error(),
		ExitCode: 2,
	}
}

// asCliIssue unwraps a cliIssueError chain into its (issue, exitCode) pair.
func asCliIssue(err error) (cliIssue, int, bool) {
	var wrap *cliIssueError
	if errors.As(err, &wrap) && wrap != nil {
		exitCode := wrap.exitCode
		if exitCode == 0 {
			exitCode = 2
		}
		return wrap.issue, exitCode, true
	}
	return cliIssue{}, 0, false
}

// runCommand executes the described CLI command lifecycle and returns the
// process exit code.
func runCommand[Req any, Res any](
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	plan commandRun[Req, Res],
) int {
	if plan.BuildRequest == nil {
		panic("commandRun.BuildRequest is required for " + plan.Name)
	}
	if plan.Execute == nil {
		panic("commandRun.Execute is required for " + plan.Name)
	}
	if plan.Options.RequestFile && (plan.LoadRequestFile == nil || plan.InlineFlagsSet == nil) {
		panic("commandRun.LoadRequestFile and InlineFlagsSet are required when RequestFile is enabled for " + plan.Name)
	}
	fs := flag.NewFlagSet(plan.Name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var help commandHelp
	if plan.Options.Standalone {
		help = newStandaloneCommandHelp(plan.Name, plan.Usage)
	} else {
		help = newCommandHelp(plan.Name, plan.Usage)
	}

	var (
		configPath  string
		format      string
		timings     bool
		requestFile string
	)
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	if !plan.Options.Standalone {
		fs.StringVar(&configPath, "config", "", "path to workspace config")
	}
	if plan.Options.Timings {
		fs.BoolVar(&timings, "timings", false, "include timing metadata in JSON output")
	}
	if plan.Options.RequestFile {
		fs.StringVar(&requestFile, "request-file", "", "path to request JSON, or - for stdin")
	}
	if plan.BindFlags != nil {
		plan.BindFlags(fs)
	}

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}

	switch {
	case plan.Options.ExactPositional > 0:
		if fs.NArg() != plan.Options.ExactPositional {
			return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
				Code:    "validation_error",
				Message: fmt.Sprintf("exactly %d positional argument(s) required", plan.Options.ExactPositional),
			}, 2)
		}
	case !plan.Options.AcceptsPositional:
		if fs.NArg() != 0 {
			return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
				Code:    "validation_error",
				Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
			}, 2)
		}
	}
	positional := fs.Args()

	if err := validateCLIFormat(plan.Name, format); err != nil {
		return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		// Standalone commands tolerate a missing workspace config; callbacks
		// that can opportunistically use a config must handle an empty path.
		if !plan.Options.Standalone {
			return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		resolvedConfigPath = ""
	}

	trimmedRequestFile := strings.TrimSpace(requestFile)
	var request Req
	switch {
	case plan.Options.RequestFile && trimmedRequestFile != "" && plan.InlineFlagsSet != nil && plan.InlineFlagsSet(fs):
		return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
			Code:    "validation_error",
			Message: "use either --request-file or the fine-grained flags",
		}, 2)
	case plan.Options.RequestFile && trimmedRequestFile != "":
		var cfg *config.Config
		if plan.Options.ConfigForFile {
			loaded, cfgErr := config.Load(resolvedConfigPath)
			if cfgErr != nil {
				return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
					Code:    "config_error",
					Message: cfgErr.Error(),
				}, 2)
			}
			cfg = loaded
		}
		loaded, loadErr := plan.LoadRequestFile(ctx, cfg, trimmedRequestFile)
		if loadErr != nil {
			var envelopeReq any
			if loaded != nil {
				envelopeReq = *loaded
			}
			if issue, exitCode, ok := asCliIssue(loadErr); ok {
				return writeCLIError(stdout, stderr, format, plan.Name, envelopeReq, issue, exitCode)
			}
			return writeCLIError(stdout, stderr, format, plan.Name, envelopeReq, cliIssue{
				Code:    "validation_error",
				Message: loadErr.Error(),
			}, 2)
		}
		if loaded == nil {
			// Programmer error: LoadRequestFile returned (nil, nil). Surface as
			// internal_error so the bug is visible in the envelope instead of
			// letting Execute run against a zero-valued request.
			return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
				Code:    "internal_error",
				Message: plan.Name + ": request file loader returned no request",
			}, 2)
		}
		request = *loaded
	default:
		var cfg *config.Config
		if plan.Options.ConfigForFlags {
			loaded, cfgErr := config.Load(resolvedConfigPath)
			if cfgErr != nil {
				return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
					Code:    "config_error",
					Message: cfgErr.Error(),
				}, 2)
			}
			cfg = loaded
		}
		request, err = plan.BuildRequest(ctx, cfg, resolvedConfigPath, positional)
		if err != nil {
			if issue, exitCode, ok := asCliIssue(err); ok {
				return writeCLIError(stdout, stderr, format, plan.Name, nil, issue, exitCode)
			}
			return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	}

	if plan.Normalize != nil {
		normalized, normErr := plan.Normalize(ctx, request, format)
		if normErr != nil {
			if issue, exitCode, ok := asCliIssue(normErr); ok {
				return writeCLIError(stdout, stderr, format, plan.Name, normalized, issue, exitCode)
			}
			return writeCLIError(stdout, stderr, format, plan.Name, normalized, cliIssue{
				Code:    "validation_error",
				Message: normErr.Error(),
			}, 2)
		}
		request = normalized
	}

	execCtx, tracker, started := withCommandTimings(ctx, plan.Options.Timings && timings && format == commandFormatJSON)

	enrichedReq, result, issue := plan.Execute(execCtx, resolvedConfigPath, request, format)
	if issue != nil {
		return writeCLIError(stdout, stderr, format, plan.Name, enrichedReq, cliIssueFromAppIssue(issue), issue.ExitCode)
	}

	if plan.PostProcess != nil {
		postResult, postIssue, postExit := plan.PostProcess(execCtx, resolvedConfigPath, enrichedReq, result)
		if postIssue != nil {
			if postExit == 0 {
				postExit = 2
			}
			return writeCLIError(stdout, stderr, format, plan.Name, enrichedReq, *postIssue, postExit)
		}
		result = postResult
	}

	return writeCLISuccessWithTimings(stdout, stderr, format, plan.Name, enrichedReq, result, nil, snapshotCommandTimings(tracker, started))
}
