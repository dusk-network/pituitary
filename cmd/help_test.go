package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommandHelpAcrossSurface(t *testing.T) {
	t.Parallel()

	testCases := map[string][]string{
		"index":           {"usage: pituitary [--config PATH] index (--rebuild | --dry-run) [--format FORMAT]", "shared config resolution:", "PITUITARY_CONFIG", "--rebuild", "--dry-run", "--verbose"},
		"status":          {"usage: pituitary [--config PATH] status", "shared config resolution:", "--format VALUE"},
		"preview-sources": {"usage: pituitary [--config PATH] preview-sources", "shared config resolution:", "--format VALUE"},
		"search-specs":    {"usage: pituitary [--config PATH] search-specs --query TEXT", "shared config resolution:", "--query VALUE", "--limit N"},
		"check-overlap":   {"usage: pituitary [--config PATH] check-overlap", "shared config resolution:", "--spec-ref VALUE", "--spec-record-file VALUE"},
		"compare-specs":   {"usage: pituitary [--config PATH] compare-specs", "shared config resolution:", "--spec-ref VALUE"},
		"analyze-impact":  {"usage: pituitary [--config PATH] analyze-impact --spec-ref REF", "shared config resolution:", "--change-type VALUE"},
		"check-doc-drift": {"usage: pituitary [--config PATH] check-doc-drift", "shared config resolution:", "--doc-ref VALUE", "--scope VALUE"},
		"review-spec":     {"usage: pituitary [--config PATH] review-spec", "shared config resolution:", "--spec-record-file VALUE"},
		"serve":           {"usage: pituitary [--config PATH] serve", "shared config resolution:", "--transport VALUE"},
	}

	for name, wantSubstrings := range testCases {
		t.Run(name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := Run([]string{name, "--help"}, &stdout, &stderr)
			if exitCode != 0 {
				t.Fatalf("Run(%q, --help) exit code = %d, want 0", name, exitCode)
			}
			if stderr.Len() != 0 {
				t.Fatalf("Run(%q, --help) wrote unexpected stderr: %q", name, stderr.String())
			}

			out := stdout.String()
			for _, want := range wantSubstrings {
				if !strings.Contains(out, want) {
					t.Fatalf("Run(%q, --help) output %q does not contain %q", name, out, want)
				}
			}
		})
	}
}
