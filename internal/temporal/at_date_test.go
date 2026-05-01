package temporal

import "testing"

func TestNormalizeAtDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "plain date", input: "2026-04-29", want: "2026-04-29"},
		{name: "trimmed plain date", input: " 2026-04-29 ", want: "2026-04-29"},
		{name: "rfc3339 timestamp", input: "2026-04-29T16:33:49Z", want: "2026-04-29"},
		{name: "lowercase t timestamp", input: "2026-04-29t16:33:49Z", want: "2026-04-29"},
		{name: "space timestamp with zone", input: "2026-04-29 16:33:49+02:00", want: "2026-04-29"},
		{name: "timestamp normalized to previous UTC date", input: "2026-04-29T00:30:00+02:00", want: "2026-04-28"},
		{name: "timestamp normalized to next UTC date", input: "2026-04-29T23:30:00-02:00", want: "2026-04-30"},
		{name: "fractional timestamp", input: "2026-04-29T16:33:49.123456789Z", want: "2026-04-29"},
		{name: "invalid text", input: "not-a-date", wantErr: true},
		{name: "invalid calendar date", input: "2026-02-30", wantErr: true},
		{name: "compact date", input: "20260429", wantErr: true},
		{name: "bad timestamp separator", input: "2026-04-29x16:33:49Z", wantErr: true},
		{name: "timezone missing", input: "2026-04-29 16:33:49", wantErr: true},
		{name: "malformed timestamp suffix", input: "2026-04-29Tgarbage", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeAtDate(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeAtDate(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeAtDate(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeAtDate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
