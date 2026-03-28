package github

import (
	"context"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/sdk"
)

func TestAdapterLoadsSpecsAndDocs(t *testing.T) {
	t.Parallel()

	adapter := &adapter{
		clientFactory: func(options sourceOptions) (issueClient, error) {
			return fakeIssueClient{
				issues: []githubIssue{
					{
						Number:  151,
						Title:   "RFC 0002 Phase 1: Extract adapter interface",
						Body:    "Phase 1 body",
						HTMLURL: "https://github.com/dusk-network/pituitary/issues/151",
						State:   "open",
						Labels:  []string{"rfc", "area:config"},
						Author:  "autholykos",
					},
					{
						Number:  154,
						Title:   "pituitary new scaffold",
						Body:    "Feature body",
						HTMLURL: "https://github.com/dusk-network/pituitary/issues/154",
						State:   "closed",
						Labels:  []string{"feature"},
						Author:  "autholykos",
					},
					{
						Number:        999,
						Title:         "Ignore PR",
						HTMLURL:       "https://github.com/dusk-network/pituitary/pull/999",
						State:         "open",
						IsPullRequest: true,
					},
				},
			}, nil
		},
	}

	result, err := adapter.Load(context.Background(), sdk.SourceConfig{
		Name:    "github-issues",
		Adapter: adapterName,
		Kind:    kindIssue,
		Options: map[string]any{
			"repo":     "dusk-network/pituitary",
			"state":    "all",
			"per_page": int64(50),
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := len(result.Specs), 1; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}
	if got, want := len(result.Docs), 1; got != want {
		t.Fatalf("doc count = %d, want %d", got, want)
	}

	spec := result.Specs[0]
	if got, want := spec.Ref, "github-issue://dusk-network/pituitary/151"; got != want {
		t.Fatalf("spec ref = %q, want %q", got, want)
	}
	if got, want := spec.Status, sdk.StatusReview; got != want {
		t.Fatalf("spec status = %q, want %q", got, want)
	}
	if got, want := spec.Domain, "config"; got != want {
		t.Fatalf("spec domain = %q, want %q", got, want)
	}
	if !strings.Contains(spec.BodyText, "Phase 1 body") {
		t.Fatalf("spec body = %q, want markdown issue body", spec.BodyText)
	}

	doc := result.Docs[0]
	if got, want := doc.Ref, "github-issue://dusk-network/pituitary/154"; got != want {
		t.Fatalf("doc ref = %q, want %q", got, want)
	}
	if got, want := doc.Metadata["issue_state"], "closed"; got != want {
		t.Fatalf("doc state metadata = %q, want %q", got, want)
	}
}

func TestAdapterRejectsUnsupportedOption(t *testing.T) {
	t.Parallel()

	adapter := &adapter{}
	_, err := adapter.Load(context.Background(), sdk.SourceConfig{
		Kind: kindIssue,
		Options: map[string]any{
			"repo":    "dusk-network/pituitary",
			"unknown": true,
		},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want unsupported option failure")
	}
	if !strings.Contains(err.Error(), `unsupported option "unknown"`) {
		t.Fatalf("Load() error = %q, want unsupported option detail", err)
	}
}

func TestAdapterRejectsMissingTokenEnvVar(t *testing.T) {
	t.Parallel()

	adapter := &adapter{}
	_, err := adapter.Load(context.Background(), sdk.SourceConfig{
		Kind: kindIssue,
		Options: map[string]any{
			"repo":        "dusk-network/pituitary",
			"api_key_env": "PITUITARY_MISSING_GITHUB_TOKEN",
		},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want missing token env failure")
	}
	if !strings.Contains(err.Error(), `PITUITARY_MISSING_GITHUB_TOKEN`) {
		t.Fatalf("Load() error = %q, want missing env var detail", err)
	}
}

type fakeIssueClient struct {
	issues []githubIssue
}

func (f fakeIssueClient) ListIssues(ctx context.Context, owner, repo string, options issueListOptions) ([]githubIssue, error) {
	return append([]githubIssue(nil), f.issues...), nil
}
