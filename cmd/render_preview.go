package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/source"
)

func renderPreviewSourcesResult(w io.Writer, result *source.PreviewResult) {
	if len(result.Sources) == 0 {
		fmt.Fprintln(w, "no sources")
		return
	}

	for i, preview := range result.Sources {
		fmt.Fprintf(w, "source: %s | %s | path: %s | items: %d\n", preview.Name, preview.Kind, preview.Path, preview.ItemCount)
		if preview.ResolvedPath != "" {
			fmt.Fprintf(w, "resolved path: %s\n", preview.ResolvedPath)
		}
		if len(preview.Files) > 0 {
			fmt.Fprintf(w, "files: %s\n", strings.Join(preview.Files, ", "))
		}
		if len(preview.Include) > 0 {
			fmt.Fprintf(w, "include: %s\n", strings.Join(preview.Include, ", "))
		}
		if len(preview.Exclude) > 0 {
			fmt.Fprintf(w, "exclude: %s\n", strings.Join(preview.Exclude, ", "))
		}
		if preview.CandidateCount > 0 {
			fmt.Fprintf(w, "candidate files: %d\n", preview.CandidateCount)
		}
		if len(preview.Items) == 0 {
			fmt.Fprint(w, "no matching items")
			if preview.CandidateCount > 0 {
				fmt.Fprint(w, " (candidate files were found under the source root, but the selectors excluded them)")
			}
			fmt.Fprintln(w)
		} else {
			for _, item := range preview.Items {
				fmt.Fprintf(w, "- %s | %s\n", item.ArtifactKind, item.Path)
				if len(item.FilesMatched) > 0 {
					fmt.Fprintf(w, "  files matched: %s\n", strings.Join(item.FilesMatched, ", "))
				}
				if len(item.IncludeMatches) > 0 {
					fmt.Fprintf(w, "  include matches: %s\n", strings.Join(item.IncludeMatches, ", "))
				}
			}
		}
		if len(preview.RejectedItems) > 0 {
			fmt.Fprintln(w, "rejected candidates:")
			for _, item := range preview.RejectedItems {
				fmt.Fprintf(w, "- %s | %s\n", item.Path, humanizePreviewRejectionReason(item.Reason))
				if len(item.FilesMatched) > 0 {
					fmt.Fprintf(w, "  files matched: %s\n", strings.Join(item.FilesMatched, ", "))
				}
				if len(item.IncludeMatches) > 0 {
					fmt.Fprintf(w, "  include matches: %s\n", strings.Join(item.IncludeMatches, ", "))
				}
				if len(item.ExcludeMatches) > 0 {
					fmt.Fprintf(w, "  exclude matches: %s\n", strings.Join(item.ExcludeMatches, ", "))
				}
			}
		}
		if i < len(result.Sources)-1 {
			fmt.Fprintln(w)
		}
	}
}

func humanizePreviewRejectionReason(reason string) string {
	switch reason {
	case "not_listed_in_files":
		return "not listed in files selectors"
	case "not_matched_by_include":
		return "not matched by include selectors"
	case "excluded_by_selector":
		return "excluded by exclude selectors"
	default:
		return reason
	}
}
