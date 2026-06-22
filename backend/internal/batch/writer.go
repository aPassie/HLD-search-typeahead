// Package batch decouples search recording from durable writes. Search events
// go to a buffered channel; a single goroutine drains them into an aggregating
// map and flushes in bulk, either by size or on a timer.
//
// This reduces DB writes (repeated queries collapse into one UPSERT) and keeps
// POST /search latency down to a channel send.
package batch

import (
	"sync"
	"sync/atomic"
	"time"
)

// Stats is a snapshot of writer counters (for /metrics and write-reduction evidence).
type Stats struct {
	Received  int64   `json:"received"`         // searches accepted into the buffer
	Dropped   int64   `json:"dropped"`          // dropped because the buffer was full
	Upserts   int64   `json:"upserts"`          // distinct (query) rows written across flushes
	Flushes   int64   `json:"flushes"`          // number of flushes
	Reduction float64 `json:"reduction_factor"` // received / upserts (searches per DB write)
}

// Writer buffers and batches search-count updates.
type Writer struct {
	events    chan string
	batchSize int
	interval  time.Duration
	apply     func(map[string]int64) // flush callback: aggregated {query: Δcount}

	received atomic.Int64
	dropped  atomic.Int64
	upserts  atomic.Int64
	flushes  atomic.Int64

	quit chan struct{}
	wg   sync.WaitGroup
}

// New creates a writer. apply is invoked with the aggregated batch on each flush.
func New(bufCap, batchSize int, interval time.Duration, apply func(map[string]int64)) *Writer {
	if bufCap < 1 {
		bufCap = 1000
	}
	if batchSize < 1 {
		batchSize = 100
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Writer{
		events:    make(chan string, bufCap),
		batchSize: batchSize,
		interval:  interval,
		apply:     apply,
		quit:      make(chan struct{}),
	}
}

// submitBlockTimeout caps how long Submit waits on a full buffer before
// dropping. Long enough to ride out short bursts, short enough to keep
// POST /search responsive.
const submitBlockTimeout = 50 * time.Millisecond

// Submit records a search. It tries a non-blocking send first; if the buffer is
// full it waits up to submitBlockTimeout, then drops and counts the drop. So we
// only drop under sustained overload, not on a brief burst.
func (w *Writer) Submit(query string) {
	select {
	case w.events <- query:
		w.received.Add(1)
		return
	default:
	}
	select {
	case w.events <- query:
		w.received.Add(1)
	case <-time.After(submitBlockTimeout):
		w.dropped.Add(1)
	}
}

// Start launches the drain/flush goroutine.
func (w *Writer) Start() {
	w.wg.Add(1)
	go w.loop()
}

func (w *Writer) loop() {
	defer w.wg.Done()
	pending := make(map[string]int64)
	total := 0
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = make(map[string]int64)
		total = 0
		w.apply(batch)
		w.upserts.Add(int64(len(batch)))
		w.flushes.Add(1)
	}

	for {
		select {
		case q := <-w.events:
			pending[q]++
			total++
			if total >= w.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.quit:
			// drain whatever is buffered, then do a final flush
			for {
				select {
				case q := <-w.events:
					pending[q]++
				default:
					flush()
					return
				}
			}
		}
	}
}

// Stop signals shutdown and blocks until the final flush completes. Call it after
// the HTTP server has stopped accepting requests so no searches are lost.
func (w *Writer) Stop() {
	close(w.quit)
	w.wg.Wait()
}

// Stats snapshots the current counters.
func (w *Writer) Stats() Stats {
	r, u := w.received.Load(), w.upserts.Load()
	red := 0.0
	if u > 0 {
		red = float64(r) / float64(u)
	}
	return Stats{Received: r, Dropped: w.dropped.Load(), Upserts: u, Flushes: w.flushes.Load(), Reduction: red}
}
