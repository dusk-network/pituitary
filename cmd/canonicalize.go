package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

type canonicalizeRequest struct {
	Path      string `json:"path"`
	BundleDir string `json:"bundle_dir,omitempty"`
	Write     bool   `json:"write,omitempty"`
}

func runCanonicalize(args []string, stdout, stderr io.Writer) int {
	return runCanonicalizeContext(context.Background(), args, stdout, stderr)
}

func runCanonicalizeContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		path      string
		bundleDir string
		write     bool
	)

	return runCommand[canonicalizeRequest, source.CanonicalizeResult](
		ctx, args, stdout, stderr,
		commandRun[canonicalizeRequest, source.CanonicalizeResult]{
			Name:  "canonicalize",
			Usage: "pituitary canonicalize --path PATH [--bundle-dir PATH] [--write] [--format FORMAT]",
			Options: commandRunOptions{
				Standalone: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.StringVar(&path, "path", "", "workspace-relative or absolute path to a markdown contract")
				fs.StringVar(&bundleDir, "bundle-dir", "", "bundle directory to preview or write")
				fs.BoolVar(&write, "write", false, "write the generated bundle")
			},
			BuildRequest: func(_ context.Context, _ *config.Config, _ string, _ []string) (canonicalizeRequest, error) {
				if strings.TrimSpace(path) == "" {
					return canonicalizeRequest{}, fmt.Errorf("--path is required")
				}
				return canonicalizeRequest{
					Path:      path,
					BundleDir: strings.TrimSpace(bundleDir),
					Write:     write,
				}, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req canonicalizeRequest, _ string) (canonicalizeRequest, *source.CanonicalizeResult, *app.Issue) {
				// Run semantic metadata inference if runtime.analysis is configured.
				var metadataInference *source.CanonicalizeInference
				if cfg, err := config.Load(cfgPath); err == nil {
					metadataInference = runCanonicalizeInference(ctx, cfg, req.Path)
				}

				result, err := source.CanonicalizeMarkdownContract(source.CanonicalizeOptions{
					Path:              req.Path,
					BundleDir:         req.BundleDir,
					Write:             req.Write,
					MetadataInference: metadataInference,
				})
				return req, result, plainIssue(err, "canonicalize_error")
			},
		},
	)
}

// runCanonicalizeInference calls the analysis runtime to infer metadata from
// the markdown file. Returns nil when inference is unavailable or fails.
func runCanonicalizeInference(ctx context.Context, cfg *config.Config, path string) *source.CanonicalizeInference {
	// Read the file to get body text and title.
	absPath := path
	if !filepath.IsAbs(absPath) {
		wd, err := os.Getwd()
		if err != nil {
			return nil
		}
		absPath = filepath.Join(wd, absPath)
	}
	// #nosec G304 -- path is user-provided CLI argument for a local file.
	body, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	// Extract a title from the first heading line.
	title := ""
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			break
		}
	}

	result, err := analysis.InferSpecMetadata(ctx, cfg, title, string(body))
	if err != nil || result == nil {
		return nil
	}

	inference := &source.CanonicalizeInference{}
	if result.Domain != nil {
		inference.Domain = &source.InferredValue{
			Value:      result.Domain.Value,
			Confidence: result.Domain.Confidence,
		}
	}
	if result.Tags != nil {
		inference.Tags = &source.InferredValues{
			Values:     result.Tags.Values,
			Confidence: result.Tags.Confidence,
		}
	}
	if result.AppliesTo != nil {
		inference.AppliesTo = &source.InferredValues{
			Values:     result.AppliesTo.Values,
			Confidence: result.AppliesTo.Confidence,
		}
	}
	if result.Status != nil {
		inference.Status = &source.InferredValue{
			Value:      result.Status.Value,
			Confidence: result.Status.Confidence,
		}
	}
	for _, dep := range result.DependsOn {
		inference.DependsOn = append(inference.DependsOn, source.InferredRef{
			Ref:        dep.Ref,
			Confidence: dep.Confidence,
		})
	}
	for _, sup := range result.Supersedes {
		inference.Supersedes = append(inference.Supersedes, source.InferredRef{
			Ref:        sup.Ref,
			Confidence: sup.Confidence,
		})
	}

	return inference
}
