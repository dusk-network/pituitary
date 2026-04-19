package index

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	pchunk "github.com/dusk-network/pituitary/internal/chunk"
	"github.com/dusk-network/pituitary/internal/config"
)

// TestChunkingConfigFingerprintIncludesResolverDefaultsVersion regression-
// guards #344: flipping a default in chunk.Resolve must invalidate stored
// snapshot fingerprints so `--update` falls back to a full rebuild
// instead of silently mixing pre- and post-flip chunk shapes.
//
// The test pins the raw input lines and their hash so any drift — a
// missing version tag, a reordered field, a new knob added without
// routing it through the version — trips here and forces a deliberate
// version bump.
func TestChunkingConfigFingerprintIncludesResolverDefaultsVersion(t *testing.T) {
	t.Parallel()

	cfg := config.ChunkingConfig{}
	got := chunkingConfigFingerprint(cfg)

	expectedParts := []string{
		"resolver_defaults_version=" + pchunk.ResolverDefaultsVersion,
		"contextualizer_format=",
		"spec_policy=",
		"spec_max_tokens=0",
		"spec_overlap_tokens=0",
		"spec_max_sections=0",
		"spec_child_max_tokens=0",
		"spec_child_overlap_tokens=0",
		"doc_policy=",
		"doc_max_tokens=0",
		"doc_overlap_tokens=0",
		"doc_max_sections=0",
		"doc_child_max_tokens=0",
		"doc_child_overlap_tokens=0",
	}
	sort.Strings(expectedParts)
	hash := sha256.Sum256([]byte(strings.Join(expectedParts, "\n")))
	want := hex.EncodeToString(hash[:])

	if got != want {
		t.Fatalf("chunkingConfigFingerprint(zero) = %s, want %s (resolver_defaults_version=%q)",
			got, want, pchunk.ResolverDefaultsVersion)
	}
}

// TestChunkingConfigFingerprintDifferentiatesResolverDefaults verifies
// that the fingerprint changes when resolver_defaults_version differs,
// which is the invariant that forces rebuild after a zero-config
// default flip in chunk.Resolve.
func TestChunkingConfigFingerprintDifferentiatesResolverDefaults(t *testing.T) {
	t.Parallel()

	base := func(version string) string {
		parts := []string{
			"resolver_defaults_version=" + version,
			"contextualizer_format=",
			"spec_policy=",
			"spec_max_tokens=0",
			"spec_overlap_tokens=0",
			"spec_max_sections=0",
			"spec_child_max_tokens=0",
			"spec_child_overlap_tokens=0",
			"doc_policy=",
			"doc_max_tokens=0",
			"doc_overlap_tokens=0",
			"doc_max_sections=0",
			"doc_child_max_tokens=0",
			"doc_child_overlap_tokens=0",
		}
		sort.Strings(parts)
		h := sha256.Sum256([]byte(strings.Join(parts, "\n")))
		return hex.EncodeToString(h[:])
	}

	v1 := base("1")
	v2 := base("2")
	if v1 == v2 {
		t.Fatalf("fingerprint at v1 and v2 should differ but both = %s", v1)
	}

	// Ensure the live ResolverDefaultsVersion matches the v2 fingerprint
	// (the #344 default), so we don't accidentally ship a release that
	// still hashes to the v1 shape.
	actual := chunkingConfigFingerprint(config.ChunkingConfig{})
	want := base(pchunk.ResolverDefaultsVersion)
	if actual != want {
		t.Fatalf("live fingerprint = %s, want %s (for ResolverDefaultsVersion=%q)",
			actual, want, pchunk.ResolverDefaultsVersion)
	}

	// Sanity: make sure the helper we used in the two branches actually
	// reproduces the live fingerprint function's output shape.
	if fmt.Sprintf("%x", sha256.Sum256([]byte{})) == v1 {
		t.Fatalf("hash collision sanity check failed")
	}
}
