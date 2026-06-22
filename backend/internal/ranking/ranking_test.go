package ranking

import (
	"math"
	"testing"
	"time"
)

func TestEffectiveHalfLife(t *testing.T) {
	p := New(time.Hour, 2, 1)
	// 8 decays to ~4 after one half-life (1h)
	if got := p.Effective(8, 0, 3600); math.Abs(got-4) > 0.01 {
		t.Fatalf("half-life decay: want ~4, got %v", got)
	}
}

func TestBump(t *testing.T) {
	p := New(time.Hour, 2, 1)
	v, ts := p.Bump(0, 0, 5, 100) // 5 searches, weight 1
	if v != 5 || ts != 100 {
		t.Fatalf("bump from 0: got %v,%v want 5,100", v, ts)
	}
	// one half-life later, with no new searches, it halves
	v2, _ := p.Bump(v, ts, 0, 100+3600)
	if math.Abs(v2-2.5) > 0.01 {
		t.Fatalf("decay then add 0: want ~2.5, got %v", v2)
	}
}

func TestKeyMonotonicOnSearch(t *testing.T) {
	p := New(time.Hour, 2, 1)
	v, ts := 3.0, int64(0)
	k0 := p.Key(v, ts)
	v, ts = p.Bump(v, ts, 1, 5000) // a later search
	if p.Key(v, ts) <= k0 {
		t.Fatalf("recency key must increase on a search")
	}
}
