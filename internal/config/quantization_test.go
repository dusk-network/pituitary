package config

import (
	"strings"
	"testing"
)

func TestLoadRuntimeQuantizationDefaultsToEmpty(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"
`)

	if cfg.Runtime.Quantization != "" {
		t.Fatalf("runtime.quantization default = %q, want empty (preserves pre-#340 float32 default)", cfg.Runtime.Quantization)
	}
}

func TestLoadRuntimeQuantizationParsesValidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want string
	}{
		{"float32", QuantizationFloat32},
		{"int8", QuantizationInt8},
		{"binary", QuantizationBinary},
		{"INT8", QuantizationInt8},
		{"  binary  ", QuantizationBinary},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()

			cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime]
quantization = "`+tc.raw+`"
`)
			if cfg.Runtime.Quantization != tc.want {
				t.Fatalf("runtime.quantization = %q, want %q", cfg.Runtime.Quantization, tc.want)
			}
		})
	}
}

func TestLoadRuntimeQuantizationRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime]
quantization = "fp8"
`)
	if err == nil {
		t.Fatal("expected error for unsupported quantization value")
	}
	if !strings.Contains(err.Error(), "runtime.quantization") {
		t.Fatalf("error should mention runtime.quantization; got %v", err)
	}
	for _, want := range []string{QuantizationFloat32, QuantizationInt8, QuantizationBinary} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should list supported value %q; got %v", want, err)
		}
	}
}

func TestLoadRuntimeQuantizationRejectsMatryoshkaPrefilterCombo(t *testing.T) {
	t.Parallel()

	for _, q := range []string{QuantizationInt8, QuantizationBinary} {
		q := q
		t.Run(q, func(t *testing.T) {
			t.Parallel()

			_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime]
quantization = "`+q+`"

[runtime.search]
matryoshka_prefilter_dimension = 256
`)
			if err == nil {
				t.Fatalf("expected error: matryoshka prefilter is incompatible with %q quantization", q)
			}
			if !strings.Contains(err.Error(), "matryoshka_prefilter_dimension") {
				t.Fatalf("error should mention matryoshka_prefilter_dimension; got %v", err)
			}
			if !strings.Contains(err.Error(), QuantizationFloat32) {
				t.Fatalf("error should steer users back to %q; got %v", QuantizationFloat32, err)
			}
		})
	}
}

// TestRenderRoundTripPreservesQuantization is a regression test for the
// migrate-config silent-drop hazard: Render must emit runtime.quantization
// so a Load(Render(cfg)) cycle preserves the configured value. Without
// this, `pituitary migrate-config --write` (or any other path that
// normalizes a config through Render) would silently revert a user's
// runtime.quantization = "int8" back to the implicit float32 default,
// changing storage shape on the next index operation without an explicit
// user decision.
func TestRenderRoundTripPreservesQuantization(t *testing.T) {
	t.Parallel()

	for _, q := range []string{QuantizationInt8, QuantizationBinary} {
		q := q
		t.Run(q, func(t *testing.T) {
			t.Parallel()

			cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime]
quantization = "`+q+`"
`)

			rendered, err := Render(cfg)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !strings.Contains(rendered, "[runtime]") {
				t.Fatalf("rendered output missing standalone [runtime] table:\n%s", rendered)
			}
			if !strings.Contains(rendered, `quantization = "`+q+`"`) {
				t.Fatalf("rendered output missing quantization = %q:\n%s", q, rendered)
			}

			round := loadRenderedConfig(t, rendered)
			if got := round.Runtime.Quantization; got != q {
				t.Fatalf("round-tripped runtime.quantization = %q, want %q", got, q)
			}
		})
	}
}

// TestRenderOmitsQuantizationWhenDefault keeps Render's output minimal
// when the config preserves the float32 default. An empty value is the
// pre-#340 contract; emitting [runtime] for a zero value would leak a
// new section into every existing user's normalized config.
func TestRenderOmitsQuantizationWhenDefault(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"
`)

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(rendered, "quantization") {
		t.Fatalf("rendered output should omit quantization when unset:\n%s", rendered)
	}
}

func TestLoadRuntimeQuantizationFloat32WithMatryoshkaPrefilterIsAllowed(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime]
quantization = "float32"

[runtime.search]
matryoshka_prefilter_dimension = 256
`)

	if cfg.Runtime.Quantization != QuantizationFloat32 {
		t.Fatalf("runtime.quantization = %q, want %q", cfg.Runtime.Quantization, QuantizationFloat32)
	}
	if cfg.Runtime.Search.PrefilterDimension != 256 {
		t.Fatalf("runtime.search.matryoshka_prefilter_dimension = %d, want 256", cfg.Runtime.Search.PrefilterDimension)
	}
}
