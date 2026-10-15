package main

import (
	"context"
	"sync/atomic"
	"time"
)

// Transfer throughput is separate from verified, durable resume checkpoints.
// The counter includes bytes transferred again on retry and resets for each run.
type transferMeter struct{ bytes atomic.Int64 }
type transferMeterKey struct{}

func recordTransfer(ctx context.Context, n int64) {
	if meter, ok := ctx.Value(transferMeterKey{}).(*transferMeter); ok && n > 0 {
		meter.bytes.Add(n)
	}
}

type speedPoint struct {
	at    time.Time
	bytes int64
}
type speedWindow struct{ points []speedPoint }

const speedWindowDuration = 4 * time.Second

func (w *speedWindow) reset(at time.Time, bytes int64) { w.points = []speedPoint{{at, bytes}} }
func (w *speedWindow) sample(at time.Time, bytes int64) int64 {
	if len(w.points) == 0 {
		w.reset(at, bytes)
		return 0
	}
	last := w.points[len(w.points)-1]
	if !at.After(last.at) {
		return 0
	}
	if bytes < last.bytes {
		w.reset(at, bytes)
		return 0
	}
	w.points = append(w.points, speedPoint{at, bytes})
	cutoff := at.Add(-speedWindowDuration)
	for len(w.points) > 2 && !w.points[1].at.After(cutoff) {
		w.points = w.points[1:]
	}
	first := w.points[0]
	return int64(float64(bytes-first.bytes) / at.Sub(first.at).Seconds())
}
