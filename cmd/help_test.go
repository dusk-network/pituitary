package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommandHelpAcrossSurface(t *testing.T) {
	t.Parallel()

	testCases := map[string][]string{
		"index":             {"usage: pituitary [--config PATH] index (--rebuild | --dry-run) [--format FORMAT]", "shared config resolution:", "PITUITARY_CONFIG", "--rebuild", "--dry-run", "--verbose"},
		"status":            {"usage: pituitary [--config PATH] status [--format FORMAT] [--check-runtime SCOPE]", "shared config resolution:", "--format VALUE", "--check-runtime VALUE"},
		"preview-sources":   {"usage: pituitary [--config PATH] preview-sources", "shared config resolution:", "--format VALUE"},
		"explain-file":      {"usage: pituitary [--config PATH] explain-file PATH [--format FORMAT]", "shared config resolution:", "--format VALUE"},
		"search-specs":      {"usage: pituitary [--config PATH] search-specs --query TEXT", "shared config resolution:", "--query VALUE", "--limit N"},
		"check-overlap":     {"usage: pituitary [--config PATH] check-overlap (--path PATH | --spec-ref REF | --spec-record-file PATH|-) [--format FORMAT]", "shared config resolution:", "--path VALUE", "--spec-ref VALUE", "--spec-record-file VALUE"},
		"compare-specs":     {"usage: pituitary [--config PATH] compare-specs (--spec-ref REF --spec-ref REF | --path PATH --path PATH) [--format FORMAT]", "shared config resolution:", "--spec-ref VALUE", "--path VALUE"},
		"analyze-impact":    {"usage: pituitary [--config PATH] analyze-impact (--path PATH | --spec-ref REF) [--change-type TYPE] [--format FORMAT]", "shared config resolution:", "--path VALUE", "--change-type VALUE"},
		"check-terminology": {"usage: pituitary [--config PATH] check-terminology --term TERM... [--canonical-term TERM]... [--spec-ref REF | --path PATH] [--scope SCOPE] [--format FORMAT]", "shared config resolution:", "--term VALUE", "--canonical-term VALUE", "--scope VALUE"},
		"check-compliance":  {"usage: pituitary [--config PATH] check-compliance (--path PATH... | --diff-file PATH|-) [--format FORMAT]", "shared config resolution:", "--path VALUE", "--diff-file VALUE"},
		"check-doc-drift":   {"usage: pituitary [--config PATH] check-doc-drift", "shared config resolution:", "--doc-ref VALUE", "--scope VALUE"},
		"review-spec":       {"usage: pituitary [--config PATH] review-spec (--path PATH | --spec-ref REF | --spec-record-file PATH|-) [--format FORMAT]", "shared config resolution:", "--path VALUE", "--spec-record-file VALUE"},
		"serve":             {"usage: pituitary [--config PATH] serve", "shared config resolution:", "--transport VALUE"},
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
