// Package metrics provides a lock-free latency histogram with fixed buckets, so
// p50/p95/p99 can be reported without storing every sample.
package metrics

import (
	"sort"
	"sync/atomic"
	"time"
)

// bucket upper bounds in microseconds; an extra overflow bucket sits above the last.
var bounds = []int64{25, 50, 100, 200, 400, 800, 1600, 3200, 6400, 12800, 25600, 51200, 102400, 256000, 512000, 1024000}

// Histogram counts observations into micro-second buckets.
type Histogram struct {
	buckets []atomic.Int64 // len(bounds)+1 (last = overflow)
	count   atomic.Int64
	sum     atomic.Int64 // total micros, for the average
}

// NewHistogram returns an empty histogram.
func NewHistogram() *Histogram {
	return &Histogram{buckets: make([]atomic.Int64, len(bounds)+1)}
}

// Observe records one latency sample.
func (h *Histogram) Observe(d time.Duration) {
	us := d.Microseconds()
	h.count.Add(1)
	h.sum.Add(us)
	i := sort.Search(len(bounds), func(i int) bool { return us <= bounds[i] })
	h.buckets[i].Add(1) // i == len(bounds) → overflow bucket
}

// Stats is a percentile snapshot (microseconds).
type Stats struct {
	Count int64 `json:"count"`
	AvgUS int64 `json:"avg_us"`
	P50US int64 `json:"p50_us"`
	P95US int64 `json:"p95_us"`
	P99US int64 `json:"p99_us"`
}

// Snapshot computes the current stats.
func (h *Histogram) Snapshot() Stats {
	total := h.count.Load()
	st := Stats{Count: total}
	if total == 0 {
		return st
	}
	st.AvgUS = h.sum.Load() / total
	st.P50US = h.quantile(0.50, total)
	st.P95US = h.quantile(0.95, total)
	st.P99US = h.quantile(0.99, total)
	return st
}

// quantile returns the upper bound of the bucket containing the q-th percentile.
func (h *Histogram) quantile(q float64, total int64) int64 {
	target := int64(q * float64(total))
	var cum int64
	for i := 0; i < len(bounds); i++ {
		cum += h.buckets[i].Load()
		if cum >= target {
			return bounds[i]
		}
	}
	return bounds[len(bounds)-1] // everything in the overflow bucket
}
