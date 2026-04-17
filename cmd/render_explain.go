package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/source"
)

func renderExplainFileResult(w io.Writer, result *source.ExplainFileResult) {
	fmt.Fprintf(w, "file: %s\n", result.AbsolutePath)
	if result.WorkspacePath != "" {
		fmt.Fprintf(w, "workspace path: %s\n", result.WorkspacePath)
	}
	fmt.Fprintf(w, "summary: %s\n", result.Summary.Status)
	if len(result.Summary.IndexedBy) > 0 {
		fmt.Fprintf(w, "indexed by: %s\n", strings.Join(result.Summary.IndexedBy, ", "))
	}

	for i, explanation := range result.Sources {
		fmt.Fprintf(w, "%d. %s | %s | root: %s | reason: %s\n", i+1, explanation.Name, explanation.Kind, explanation.Path, explanation.Reason)
		if explanation.UnderSourceRoot {
			fmt.Fprintf(w, "   relative path: %s\n", explanation.RelativePath)
			fmt.Fprintf(w, "   selected: %t\n", explanation.Selected)
		} else {
			fmt.Fprintln(w, "   outside source root")
		}
		if explanation.ArtifactKind != "" {
			fmt.Fprintf(w, "   artifact: %s\n", explanation.ArtifactKind)
		}
		if len(explanation.Files) > 0 {
			fmt.Fprintf(w, "   files: %s\n", strings.Join(explanation.Files, ", "))
		}
		if len(explanation.FilesMatched) > 0 {
			fmt.Fprintf(w, "   files matched: %s\n", strings.Join(explanation.FilesMatched, ", "))
		}
		if len(explanation.Include) > 0 {
			fmt.Fprintf(w, "   include: %s\n", strings.Join(explanation.Include, ", "))
		}
		if len(explanation.IncludeMatches) > 0 {
			fmt.Fprintf(w, "   include matches: %s\n", strings.Join(explanation.IncludeMatches, ", "))
		}
		if len(explanation.Exclude) > 0 {
			fmt.Fprintf(w, "   exclude: %s\n", strings.Join(explanation.Exclude, ", "))
		}
		if len(explanation.ExcludeMatches) > 0 {
			fmt.Fprintf(w, "   exclude matches: %s\n", strings.Join(explanation.ExcludeMatches, ", "))
		}
		if explanation.BundlePath != "" {
			fmt.Fprintf(w, "   bundle: %s\n", explanation.BundlePath)
		}
		if explanation.ConflictsWith != "" {
			fmt.Fprintf(w, "   conflicts with: %s\n", explanation.ConflictsWith)
		}
		if explanation.InferredSpec != nil {
			fmt.Fprintf(w, "   inferred ref: %s\n", explanation.InferredSpec.Ref)
			fmt.Fprintf(w, "   inferred title: %s\n", explanation.InferredSpec.Title)
			fmt.Fprintf(w, "   inferred status: %s\n", explanation.InferredSpec.Status)
			if explanation.InferredSpec.Domain != "" {
				fmt.Fprintf(w, "   inferred domain: %s\n", explanation.InferredSpec.Domain)
			}
			if len(explanation.InferredSpec.DependsOn) > 0 {
				fmt.Fprintf(w, "   inferred depends_on: %s\n", strings.Join(explanation.InferredSpec.DependsOn, ", "))
			}
			if len(explanation.InferredSpec.Supersedes) > 0 {
				fmt.Fprintf(w, "   inferred supersedes: %s\n", strings.Join(explanation.InferredSpec.Supersedes, ", "))
			}
			if len(explanation.InferredSpec.AppliesTo) > 0 {
				fmt.Fprintf(w, "   inferred applies_to: %s\n", strings.Join(explanation.InferredSpec.AppliesTo, ", "))
			}
			if explanation.InferredSpec.Inference != nil {
				fmt.Fprintf(w, "   inference: %s (%.2f)\n", explanation.InferredSpec.Inference.Level, explanation.InferredSpec.Inference.Score)
			}
		}
	}
}
