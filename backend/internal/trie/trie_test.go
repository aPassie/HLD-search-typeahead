package trie

import "testing"

func TestTopKByCount(t *testing.T) {
	tr := New(10)
	tr.Insert("iphone", 100)
	tr.Insert("iphone 15", 85)
	tr.Insert("ipad", 70)
	tr.Insert("java", 95)
	tr.Finalize()

	got := tr.TopK("ip", 10)
	if len(got) != 3 {
		t.Fatalf("prefix 'ip': want 3 candidates, got %d (%+v)", len(got), got)
	}
	want := []string{"iphone", "iphone 15", "ipad"} // by count, descending
	for i, q := range want {
		if got[i].Query != q {
			t.Fatalf("position %d: want %q, got %q", i, q, got[i].Query)
		}
	}
	if n := len(tr.TopK("xyz", 10)); n != 0 {
		t.Fatalf("no-match prefix: want 0, got %d", n)
	}
	if n := len(tr.TopK("java", 10)); n != 1 {
		t.Fatalf("prefix 'java': want 1, got %d", n)
	}
}

func TestTopKRespectsLimit(t *testing.T) {
	tr := New(2)
	tr.Insert("ab", 1)
	tr.Insert("ac", 2)
	tr.Insert("ad", 3)
	tr.Finalize()
	if n := len(tr.TopK("a", 10)); n != 2 {
		t.Fatalf("stored k=2 should cap at 2, got %d", n)
	}
}

func TestApplyBatchBumpAndInsert(t *testing.T) {
	tr := New(10)
	tr.Insert("iphone", 100)
	tr.Insert("ipad", 70)
	tr.Finalize()

	// bump ipad past iphone (70 + 50 = 120 > 100)
	tr.ApplyBatch(map[string]int64{"ipad": 50})
	got := tr.TopK("ip", 10)
	if got[0].Query != "ipad" || got[0].Count != 120 {
		t.Fatalf("after bump want ipad@120 first, got %+v", got)
	}

	// a new query gets inserted and becomes reachable by its prefix
	tr.ApplyBatch(map[string]int64{"ipod touch": 5})
	if n := len(tr.TopK("ipo", 10)); n != 1 {
		t.Fatalf("new query should be inserted, got %d for 'ipo'", n)
	}
}
