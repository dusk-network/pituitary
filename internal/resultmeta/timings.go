package resultmeta

import (
	"context"
	"sync"
	"time"
)

// Timings reports optional command timing metadata for JSON CLI output.
type Timings struct {
	TotalMS        int64 `json:"total_ms"`
	IndexingMS     int64 `json:"indexing_ms,omitempty"`
	EmbeddingMS    int64 `json:"embedding_ms,omitempty"`
	AnalysisMS     int64 `json:"analysis_ms,omitempty"`
	AnalysisCalls  int   `json:"analysis_calls,omitempty"`
	EmbeddingCalls int   `json:"embedding_calls,omitempty"`
}

// TimingTracker accumulates coarse command timing categories.
type TimingTracker struct {
	mu sync.Mutex

	indexing       time.Duration
	embedding      time.Duration
	analysis       time.Duration
	analysisCalls  int
	embeddingCalls int
}

type timingTrackerKey struct{}

func NewTimingTracker() *TimingTracker {
	return &TimingTracker{}
}

func WithTimingTracker(ctx context.Context, tracker *TimingTracker) context.Context {
	if tracker == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, timingTrackerKey{}, tracker)
}

func TimingTrackerFromContext(ctx context.Context) *TimingTracker {
	if ctx == nil {
		return nil
	}
	tracker, _ := ctx.Value(timingTrackerKey{}).(*TimingTracker)
	return tracker
}

func (t *TimingTracker) AddIndexing(duration time.Duration) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.indexing += duration
}

func (t *TimingTracker) AddEmbedding(duration time.Duration, calls int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.embedding += duration
	t.embeddingCalls += calls
}

func (t *TimingTracker) AddAnalysis(duration time.Duration, calls int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.analysis += duration
	t.analysisCalls += calls
}

func (t *TimingTracker) Snapshot(total time.Duration) *Timings {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return &Timings{
		TotalMS:        durationMillis(total),
		IndexingMS:     durationMillis(t.indexing),
		EmbeddingMS:    durationMillis(t.embedding),
		AnalysisMS:     durationMillis(t.analysis),
		AnalysisCalls:  t.analysisCalls,
		EmbeddingCalls: t.embeddingCalls,
	}
}

func durationMillis(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	if duration < time.Millisecond {
		return 1
	}
	return duration.Milliseconds()
}
