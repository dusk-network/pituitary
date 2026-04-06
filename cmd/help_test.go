package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommandHelpAcrossSurface(t *testing.T) {
	t.Parallel()

	testCases := map[string][]string{
		"init":              {"usage: pituitary init [--path PATH] [--config-path PATH] [--dry-run] [--format FORMAT]", "--path VALUE", "--config-path VALUE", "--dry-run"},
		"index":             {"usage: pituitary [--config PATH] index (--rebuild | --update | --dry-run) [--full] [--format FORMAT]", "shared config resolution:", "PITUITARY_CONFIG", "--rebuild", "--update", "--dry-run", "--full", "--verbose"},
		"new":               {"usage: pituitary [--config PATH] new --title TITLE [--domain DOMAIN] [--id ID] [--bundle-dir PATH] [--format FORMAT]", "shared config resolution:", "--title VALUE", "--domain VALUE", "--id VALUE", "--bundle-dir VALUE"},
		"migrate-config":    {"usage: pituitary migrate-config [--path PATH] [--write] [--format FORMAT]", "--path VALUE", "--write"},
		"status":            {"usage: pituitary [--config PATH] status [--format FORMAT] [--check-runtime SCOPE]", "shared config resolution:", "--format VALUE", "--check-runtime VALUE"},
		"preview-sources":   {"usage: pituitary [--config PATH] preview-sources", "shared config resolution:", "--format VALUE"},
		"explain-file":      {"usage: pituitary [--config PATH] explain-file PATH [--format FORMAT]", "shared config resolution:", "--format VALUE"},
		"search-specs":      {"usage: pituitary [--config PATH] search-specs (--query TEXT | --request-file PATH|-)", "shared config resolution:", "--query VALUE", "--request-file VALUE", "--limit N"},
		"check-overlap":     {"usage: pituitary [--config PATH] check-overlap (--path PATH | --spec-ref REF | --spec-record-file PATH|- | --request-file PATH|-) [--format FORMAT]", "shared config resolution:", "--path VALUE", "--spec-ref VALUE", "--spec-record-file VALUE", "--request-file VALUE"},
		"compare-specs":     {"usage: pituitary [--config PATH] compare-specs (--spec-ref REF --spec-ref REF | --path PATH --path PATH | --request-file PATH|-) [--format FORMAT]", "shared config resolution:", "--spec-ref VALUE", "--path VALUE", "--request-file VALUE"},
		"analyze-impact":    {"usage: pituitary [--config PATH] analyze-impact (--path PATH | --spec-ref REF | --request-file PATH|-) [--change-type TYPE] [--summary] [--format FORMAT]", "shared config resolution:", "--path VALUE", "--change-type VALUE", "--request-file VALUE", "--summary"},
		"check-terminology": {"usage: pituitary [--config PATH] check-terminology ([--term TERM]... [--canonical-term TERM]... [--spec-ref REF | --path PATH] [--scope SCOPE] | --request-file PATH|-) [--format FORMAT]", "shared config resolution:", "--term VALUE", "--canonical-term VALUE", "--scope VALUE", "--request-file VALUE"},
		"check-compliance":  {"usage: pituitary [--config PATH] check-compliance (--path PATH... | --diff-file PATH|- | --request-file PATH|-) [--format FORMAT]", "shared config resolution:", "--path VALUE", "--diff-file VALUE", "--request-file VALUE"},
		"check-doc-drift":   {"usage: pituitary [--config PATH] check-doc-drift ([--doc-ref REF]... | [--scope all] | [--diff-file PATH|-]) [--request-file PATH|-] [--format FORMAT]", "shared config resolution:", "--doc-ref VALUE", "--scope VALUE", "--diff-file VALUE", "--request-file VALUE"},
		"fix":               {"usage: pituitary [--config PATH] fix (--path PATH | --scope VALUE) [--dry-run] [--yes] [--format FORMAT]", "shared config resolution:", "--path VALUE", "--scope VALUE", "--dry-run", "--yes"},
		"review-spec":       {"usage: pituitary [--config PATH] review-spec (--path PATH | --spec-ref REF | --spec-record-file PATH|- | --request-file PATH|-) [--format FORMAT]", "shared config resolution:", "--path VALUE", "--spec-record-file VALUE", "--request-file VALUE"},
		"schema":            {"usage: pituitary schema [COMMAND] [--format FORMAT]", "--format VALUE"},
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
