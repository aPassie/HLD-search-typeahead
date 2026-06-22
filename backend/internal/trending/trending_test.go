package trending

import (
	"testing"
	"time"

	"searchtypeahead/internal/ranking"
)

func TestTopAndPrefix(t *testing.T) {
	x := New(ranking.New(time.Hour, 2, 1), 100)
	x.Update(map[string]int64{"iphone": 10, "ipad": 5, "java": 3}, 0)

	top := x.Top(2, 0)
	if len(top) != 2 || top[0].Query != "iphone" {
		t.Fatalf("top-2: %+v", top)
	}
	m := x.MatchingPrefix("ip")
	if len(m) != 2 { // iphone, ipad
		t.Fatalf("prefix 'ip': want 2, got %v", m)
	}
}

func TestEviction(t *testing.T) {
	x := New(ranking.New(time.Hour, 2, 1), 2)
	x.Update(map[string]int64{"a": 1, "b": 2, "c": 3}, 0) // cap 2 → lowest (a) evicted
	if len(x.items) != 2 {
		t.Fatalf("cap not enforced: %d", len(x.items))
	}
	if _, ok := x.items["a"]; ok {
		t.Fatalf("lowest-recency 'a' should have been evicted")
	}
}
