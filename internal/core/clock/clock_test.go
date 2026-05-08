package clock

import (
	"testing"
	"time"
)

// Real() must move forward across calls. This is the only test the real
// clock needs — it pins the contract that Now() reflects wall time, so a
// future "optimisation" that caches the result would break it.
func TestRealClockAdvancesAcrossCalls(t *testing.T) {
	c := Real()
	first := c.Now()
	time.Sleep(2 * time.Millisecond)
	second := c.Now()
	if !second.After(first) {
		t.Fatalf("Real().Now() did not advance: first=%v second=%v", first, second)
	}
}

// Fake.Now() must be deterministic until Advance is called. This is the
// property the scanner / firmware tests will lean on.
func TestFakeClockIsDeterministicUntilAdvance(t *testing.T) {
	anchor := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := NewFake(anchor)

	if got := f.Now(); !got.Equal(anchor) {
		t.Errorf("first Now() = %v, want %v", got, anchor)
	}
	if got := f.Now(); !got.Equal(anchor) {
		t.Errorf("repeat Now() = %v, want %v (must not advance on its own)", got, anchor)
	}

	f.Advance(90 * time.Second)
	want := anchor.Add(90 * time.Second)
	if got := f.Now(); !got.Equal(want) {
		t.Errorf("after Advance(90s), Now() = %v, want %v", got, want)
	}

	f.Advance(-30 * time.Second)
	want = anchor.Add(60 * time.Second)
	if got := f.Now(); !got.Equal(want) {
		t.Errorf("after Advance(-30s), Now() = %v, want %v (negative advance must rewind)", got, want)
	}
}
