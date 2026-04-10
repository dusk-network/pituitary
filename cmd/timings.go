package cmd

import (
	"context"
	"time"

	"github.com/dusk-network/pituitary/internal/resultmeta"
)

func withCommandTimings(ctx context.Context, enabled bool) (context.Context, *resultmeta.TimingTracker, time.Time) {
	if !enabled {
		return ctx, nil, time.Time{}
	}
	tracker := resultmeta.NewTimingTracker()
	return resultmeta.WithTimingTracker(ctx, tracker), tracker, time.Now()
}

func snapshotCommandTimings(tracker *resultmeta.TimingTracker, started time.Time) *resultmeta.Timings {
	if tracker == nil || started.IsZero() {
		return nil
	}
	return tracker.Snapshot(time.Since(started))
}
