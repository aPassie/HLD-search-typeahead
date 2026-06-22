package batch

import (
	"sync"
	"testing"
	"time"
)

func TestAggregationAndWriteReduction(t *testing.T) {
	var mu sync.Mutex
	applied := map[string]int64{}
	// large batch size + long interval means the only flush is the one Stop forces.
	w := New(1000, 1000, time.Hour, func(b map[string]int64) {
		mu.Lock()
		for k, v := range b {
			applied[k] += v
		}
		mu.Unlock()
	})
	w.Start()
	for i := 0; i < 50; i++ {
		w.Submit("iphone")
	}
	for i := 0; i < 30; i++ {
		w.Submit("ipad")
	}
	w.Stop() // drains + final flush

	mu.Lock()
	defer mu.Unlock()
	if applied["iphone"] != 50 || applied["ipad"] != 30 {
		t.Fatalf("aggregation wrong: %+v", applied)
	}
	st := w.Stats()
	if st.Received != 80 {
		t.Fatalf("received=%d want 80", st.Received)
	}
	if st.Upserts != 2 { // 80 searches collapsed into 2 distinct UPSERTs
		t.Fatalf("upserts=%d want 2 (write reduction)", st.Upserts)
	}
}
