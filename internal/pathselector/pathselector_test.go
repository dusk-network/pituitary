package pathselector

import (
	"strings"
	"testing"
)

func TestMatchRecursiveSelectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{name: "recursive includes root file", pattern: "**/*.md", path: "root.md", want: true},
		{name: "recursive includes nested file", pattern: "**/*.md", path: "guides/deep/nested.md", want: true},
		{name: "recursive directory excludes unrelated", pattern: "archive/**", path: "guides/api.md", want: false},
		{name: "single star is segment local", pattern: "guides/*.md", path: "guides/deep/nested.md", want: false},
		{name: "repeated recursive segments", pattern: "**/**/nested.md", path: "guides/deep/nested.md", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Match(tt.pattern, tt.path)
			if err != nil {
				t.Fatalf("Match(%q, %q) error = %v", tt.pattern, tt.path, err)
			}
			if got != tt.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestValidateRejectsInvalidRelativePatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "empty", pattern: "", want: "value must not be empty"},
		{name: "absolute", pattern: "/**/*.md", want: "must be relative to the source root"},
		{name: "empty segment", pattern: "guides//*.md", want: "must not contain empty path segments"},
		{name: "current segment", pattern: "guides/./*.md", want: `must not contain "." path segments`},
		{name: "parent segment", pattern: "guides/../*.md", want: `must not contain ".." path segments`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Validate(tt.pattern)
			if err == nil {
				t.Fatalf("Validate(%q) error = nil, want error", tt.pattern)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate(%q) error = %q, want %q", tt.pattern, err, tt.want)
			}
		})
	}
}
