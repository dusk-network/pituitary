package chunk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	stchunk "github.com/dusk-network/stroma/v2/chunk"
	stcorpus "github.com/dusk-network/stroma/v2/corpus"
	stindex "github.com/dusk-network/stroma/v2/index"

	"github.com/dusk-network/pituitary/sdk"
)

func TestResolve_ZeroConfigReturnsNilPolicy(t *testing.T) {
	t.Parallel()

	policy, err := Resolve(Config{})
	if err != nil {
		t.Fatalf("Resolve(zero) returned error: %v", err)
	}
	if policy != nil {
		t.Fatalf("Resolve(zero) = %T, want nil (preserves default stroma behavior)", policy)
	}
}

func TestResolve_SpecOnlyRoutesSpecAndDefaultsOthers(t *testing.T) {
	t.Parallel()

	policy, err := Resolve(Config{
		Spec: KindConfig{Policy: PolicyMarkdown, MaxTokens: 512},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	router, ok := policy.(stchunk.KindRouterPolicy)
	if !ok {
		t.Fatalf("Resolve returned %T, want KindRouterPolicy", policy)
	}
	if _, ok := router.ByKind[sdk.ArtifactKindSpec]; !ok {
		t.Fatalf("router missing spec entry; ByKind=%+v", router.ByKind)
	}
	if _, ok := router.ByKind[sdk.ArtifactKindDoc]; ok {
		t.Fatalf("router has doc entry but doc was not configured")
	}
	defaultPolicy, ok := router.Default.(stchunk.MarkdownPolicy)
	if !ok {
		t.Fatalf("router.Default = %T, want MarkdownPolicy", router.Default)
	}
	// Router Default must carry stroma's DefaultMaxChunkSections cap
	// so the unconfigured kind keeps the same DoS guard as the
	// pre-#338 nil-policy rebuild path.
	if got, want := defaultPolicy.Options.MaxSections, stindex.DefaultMaxChunkSections; got != want {
		t.Fatalf("router.Default MarkdownPolicy.Options.MaxSections = %d, want %d", got, want)
	}
}

func TestResolve_BothKindsWireThroughDispatch(t *testing.T) {
	t.Parallel()

	policy, err := Resolve(Config{
		Spec: KindConfig{Policy: PolicyMarkdown, MaxTokens: 512, OverlapTokens: 32, MaxSections: 128},
		Doc:  KindConfig{Policy: PolicyLateChunk, MaxTokens: 2048, ChildMaxTokens: 512, ChildOverlapTokens: 64, MaxSections: 256},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	router, ok := policy.(stchunk.KindRouterPolicy)
	if !ok {
		t.Fatalf("Resolve returned %T, want KindRouterPolicy", policy)
	}
	specPolicy, ok := router.ByKind[sdk.ArtifactKindSpec].(stchunk.MarkdownPolicy)
	if !ok {
		t.Fatalf("spec policy is %T, want MarkdownPolicy", router.ByKind[sdk.ArtifactKindSpec])
	}
	if got := specPolicy.Options; got.MaxTokens != 512 || got.OverlapTokens != 32 || got.MaxSections != 128 {
		t.Fatalf("spec MarkdownPolicy.Options = %+v, want max=512 overlap=32 sections=128", got)
	}
	docPolicy, ok := router.ByKind[sdk.ArtifactKindDoc].(stchunk.LateChunkPolicy)
	if !ok {
		t.Fatalf("doc policy is %T, want LateChunkPolicy", router.ByKind[sdk.ArtifactKindDoc])
	}
	if docPolicy.ParentMaxTokens != 2048 || docPolicy.ChildMaxTokens != 512 || docPolicy.ChildOverlapTokens != 64 || docPolicy.MaxSections != 256 {
		t.Fatalf("doc LateChunkPolicy = %+v, want parent=2048 child=512 overlap=64 sections=256", docPolicy)
	}
}

func TestResolve_DefaultPolicyWhenEmptyName(t *testing.T) {
	t.Parallel()

	// An explicit overlap knob with empty Policy should resolve to
	// MarkdownPolicy; this keeps the config surface forgiving when
	// users only set tuning knobs.
	policy, err := Resolve(Config{Doc: KindConfig{MaxTokens: 1024}})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	router := policy.(stchunk.KindRouterPolicy)
	if _, ok := router.ByKind[sdk.ArtifactKindDoc].(stchunk.MarkdownPolicy); !ok {
		t.Fatalf("doc policy = %T, want MarkdownPolicy by default", router.ByKind[sdk.ArtifactKindDoc])
	}
}

func TestResolve_LateChunkRequiresChildMaxTokens(t *testing.T) {
	t.Parallel()

	_, err := Resolve(Config{Doc: KindConfig{Policy: PolicyLateChunk}})
	if err == nil {
		t.Fatal("expected error when LateChunkPolicy has no child_max_tokens")
	}
	if !strings.Contains(err.Error(), "child_max_tokens") {
		t.Fatalf("error should mention child_max_tokens; got: %v", err)
	}
}

func TestResolve_MarkdownDefaultsToStromaSectionCap(t *testing.T) {
	t.Parallel()

	// A spec-only config with tuning knobs set but max_sections left
	// at zero must resolve to stroma's DefaultMaxChunkSections so the
	// per-record DoS guard stays on — matching the pre-#338 nil-policy
	// rebuild contract rather than silently producing "no cap".
	policy, err := Resolve(Config{
		Spec: KindConfig{Policy: PolicyMarkdown, MaxTokens: 512},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	router := policy.(stchunk.KindRouterPolicy)
	spec := router.ByKind[sdk.ArtifactKindSpec].(stchunk.MarkdownPolicy)
	if got, want := spec.Options.MaxSections, stindex.DefaultMaxChunkSections; got != want {
		t.Fatalf("spec MarkdownPolicy.Options.MaxSections = %d, want %d (DefaultMaxChunkSections)", got, want)
	}
}

func TestResolve_LateChunkDefaultsToStromaSectionCap(t *testing.T) {
	t.Parallel()

	policy, err := Resolve(Config{
		Doc: KindConfig{Policy: PolicyLateChunk, ChildMaxTokens: 512},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	router := policy.(stchunk.KindRouterPolicy)
	doc := router.ByKind[sdk.ArtifactKindDoc].(stchunk.LateChunkPolicy)
	if got, want := doc.MaxSections, stindex.DefaultMaxChunkSections; got != want {
		t.Fatalf("doc LateChunkPolicy.MaxSections = %d, want %d (DefaultMaxChunkSections)", got, want)
	}
}

func TestResolve_NegativeMaxSectionsDisablesCap(t *testing.T) {
	t.Parallel()

	policy, err := Resolve(Config{
		Spec: KindConfig{Policy: PolicyMarkdown, MaxSections: -1},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	router := policy.(stchunk.KindRouterPolicy)
	spec := router.ByKind[sdk.ArtifactKindSpec].(stchunk.MarkdownPolicy)
	if got := spec.Options.MaxSections; got != 0 {
		t.Fatalf("spec MarkdownPolicy.Options.MaxSections = %d, want 0 (cap disabled)", got)
	}
}

func TestResolve_SpecOnlyConfigStillGuardsDocsAgainstOversectionedBodies(t *testing.T) {
	t.Parallel()

	// Regression for the silent-drift Codex flagged: enabling
	// spec-only chunking must not remove the doc-side DoS guard that
	// the pre-#338 nil-policy rebuild path gave us. Building a body
	// with more headings than stroma's DefaultMaxChunkSections must
	// still error through the router's Default policy.
	policy, err := Resolve(Config{
		Spec: KindConfig{Policy: PolicyMarkdown, MaxTokens: 512},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	overLimit := stindex.DefaultMaxChunkSections + 10
	var body strings.Builder
	for i := 0; i < overLimit; i++ {
		fmt.Fprintf(&body, "# h%d\n\nbody-%d\n\n", i, i)
	}
	record := stcorpus.Record{
		Ref:        "doc-overflow",
		Kind:       sdk.ArtifactKindDoc,
		Title:      "Overflow",
		BodyFormat: sdk.BodyFormatMarkdown,
		BodyText:   body.String(),
	}
	if _, err := policy.Chunk(context.Background(), record); err == nil {
		t.Fatal("expected over-cap doc rebuild to fail; router Default silently dropped the DoS guard")
	} else if !errors.Is(err, stchunk.ErrTooManySections) {
		t.Fatalf("expected ErrTooManySections; got %v", err)
	}
}

func TestResolve_UnknownPolicyNameRejected(t *testing.T) {
	t.Parallel()

	_, err := Resolve(Config{Spec: KindConfig{Policy: "semantic"}})
	if err == nil {
		t.Fatal("expected error for unknown policy name")
	}
	if !strings.Contains(err.Error(), "unknown policy") {
		t.Fatalf("error should mention unknown policy; got: %v", err)
	}
}
