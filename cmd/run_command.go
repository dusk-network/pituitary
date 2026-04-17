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

	// LoadRequestFile parses the request JSON and may run follow-up reads. It
	// returns a pointer to the parsed request: nil means no request should
	// surface in the error envelope (e.g., JSON parse failure); non-nil means
	// write the pointee into the envelope. Required when Options.RequestFile
	// is true.
	LoadRequestFile func(ctx context.Context, cfg *config.Config, trimmedPath string) (*Req, error)

	// BuildRequest builds the request from the inline flags. cfg is non-nil
	// iff Options.ConfigForFlags is true. Required.
	BuildRequest func(ctx context.Context, cfg *config.Config, resolvedConfigPath string) (Req, error)

	// Normalize (optional) runs after the request is composed, before Execute,
	// and fires for both the --request-file path and the inline-flags path.
	// On error, the runner writes the returned Req into the envelope, so
	// Normalize can return a pre- or post-normalize request as appropriate.
	Normalize func(ctx context.Context, req Req) (Req, error)

	// Execute calls the app layer. Required.
	Execute func(ctx context.Context, resolvedConfigPath string, req Req) (Req, *Res, *app.Issue)

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
	// AcceptsPositional, when false (default), rejects fs.NArg() != 0.
	AcceptsPositional bool
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
	help := newCommandHelp(plan.Name, plan.Usage)

	var (
		configPath  string
		format      string
		timings     bool
		requestFile string
	)
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")
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

	if !plan.Options.AcceptsPositional && fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	if err := validateCLIFormat(plan.Name, format); err != nil {
		return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, plan.Name, nil, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
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
		if loaded != nil {
			request = *loaded
		}
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
		request, err = plan.BuildRequest(ctx, cfg, resolvedConfigPath)
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
		normalized, normErr := plan.Normalize(ctx, request)
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

	enrichedReq, result, issue := plan.Execute(execCtx, resolvedConfigPath, request)
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
