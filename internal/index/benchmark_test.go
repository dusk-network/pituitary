package index

import (
	"testing"

	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func BenchmarkRebuildFixture(b *testing.B) {
	cfg := loadFixtureConfig(b)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		b.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		b.Fatalf("warm Rebuild() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Rebuild(cfg, records); err != nil {
			b.Fatalf("Rebuild() error = %v", err)
		}
	}
}

func BenchmarkSearchSpecs(b *testing.B) {
	cfg := loadFixtureConfig(b)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		b.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		b.Fatalf("Rebuild() error = %v", err)
	}

	cases := []struct {
		name  string
		query SearchSpecQuery
	}{
		{
			name:  "default",
			query: SearchSpecQuery{Query: "rate limiting", Limit: 5},
		},
		{
			name: "historical",
			query: SearchSpecQuery{
				Query:    "fixed window rate limiting",
				Statuses: []string{model.StatusSuperseded},
				Limit:    5,
			},
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, err := SearchSpecs(cfg, tc.query)
				if err != nil {
					b.Fatalf("SearchSpecs() error = %v", err)
				}
				if len(result.Matches) == 0 {
					b.Fatal("SearchSpecs() returned no matches")
				}
			}
		})
	}
}
