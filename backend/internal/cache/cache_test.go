package cache

import (
	"testing"
	"time"

	"searchtypeahead/internal/hashring"
	"searchtypeahead/internal/model"
)

func cand(q string) []model.Candidate { return []model.Candidate{{Query: q, Count: 1}} }

func TestMemoryGetSetTTL(t *testing.T) {
	m := NewMemory("n", 10)
	m.Set("a", cand("apple"), 40*time.Millisecond)
	if _, ok := m.Get("a"); !ok {
		t.Fatal("want hit before TTL")
	}
	time.Sleep(60 * time.Millisecond)
	if _, ok := m.Get("a"); ok {
		t.Fatal("want miss after TTL")
	}
}

func TestMemoryLRUEvictionAndDelete(t *testing.T) {
	m := NewMemory("n", 2)
	m.Set("a", nil, time.Minute)
	m.Set("b", nil, time.Minute)
	m.Set("c", nil, time.Minute) // evicts least-recently-used "a"
	if m.Peek("a") != StateMiss {
		t.Fatal("a should have been evicted")
	}
	m.Delete("b")
	if m.Peek("b") != StateMiss {
		t.Fatal("b should have been deleted")
	}
}

func TestDistributedRoutingAndDebug(t *testing.T) {
	ring := hashring.New(50, "n0", "n1", "n2")
	nodes := map[string]Node{
		"n0": NewMemory("n0", 100),
		"n1": NewMemory("n1", 100),
		"n2": NewMemory("n2", 100),
	}
	d := NewDistributed(ring, nodes, time.Minute)

	if _, ok := d.Get("iphone"); ok {
		t.Fatal("want miss before set")
	}
	d.Set("iphone", cand("iphone"))
	if _, ok := d.Get("iphone"); !ok {
		t.Fatal("want hit after set")
	}

	owner := ring.Get("iphone")
	if got := nodes[owner].(*Memory).Len(); got != 1 {
		t.Fatalf("entry should live on owner %s (len=%d)", owner, got)
	}
	info := d.Debug("iphone")
	if info.Node != owner || info.State != StateHit {
		t.Fatalf("debug mismatch: %+v (owner=%s)", info, owner)
	}

	d.Invalidate("iphone")
	if _, ok := d.Get("iphone"); ok {
		t.Fatal("want miss after invalidate")
	}
}
