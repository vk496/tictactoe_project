package store

import "testing"

func TestMapPutGet(t *testing.T) {
	m := NewMap[int]()

	if _, ok := m.Get("missing"); ok {
		t.Fatal("empty map should not find a key")
	}

	m.Put("a", 1)
	v, ok := m.Get("a")
	if !ok || v != 1 {
		t.Fatalf("Get(a) = %d, %v; want 1, true", v, ok)
	}
}

func TestMapDelete(t *testing.T) {
	m := NewMap[string]()
	m.Put("k", "v")

	if !m.Delete("k") {
		t.Fatal("Delete of an existing key should report true")
	}
	if m.Delete("k") {
		t.Fatal("Delete of a missing key should report false")
	}
	if _, ok := m.Get("k"); ok {
		t.Fatal("key should be gone after Delete")
	}
}

func TestStatsStore(t *testing.T) {
	s := NewStatsStore()

	if got := s.Get("nobody"); got != (Stats{}) {
		t.Fatalf("unknown user = %+v; want zero Stats", got)
	}

	s.AddWin("amy")
	s.AddWin("amy")
	s.AddLoss("amy")

	got := s.Get("amy")
	if got.Wins != 2 || got.Losses != 1 || got.Draws != 0 {
		t.Fatalf("amy = %+v; want 2 wins, 1 loss, 0 draws", got)
	}
}
