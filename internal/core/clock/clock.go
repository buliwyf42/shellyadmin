// Package clock is a deliberately tiny abstraction over time.Now so the
// scanner / firmware / setters packages can be exercised by deterministic
// unit tests without mocking the time package globally. Production paths use
// Real(); tests construct a Fake and Advance it explicitly.
package clock

import "time"

// Clock is the read surface used by callers that need a wall-clock value
// (LastSeen, AuthLockedUntil, CheckedAt). Kept to a single method on purpose:
// every additional method is one more thing a fake has to keep in sync, and
// time.Since(t) is just time.Now().Sub(t) — callers who need duration math
// can compose it locally.
type Clock interface {
	Now() time.Time
}

// Real returns a Clock backed by the real time package. Cheap to call; safe
// to share across goroutines.
func Real() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Fake is a hand-controlled Clock for tests. Construct with NewFake; Advance
// to move the wall forward. Not safe for concurrent mutation — tests should
// drive a single goroutine, or wrap with sync if they don't.
type Fake struct {
	t time.Time
}

// NewFake returns a Fake anchored at t. A test that doesn't care about the
// absolute moment can pass time.Time{} or time.Date(...) — what matters is
// that successive Now() calls return the same value until Advance is called.
func NewFake(t time.Time) *Fake { return &Fake{t: t} }

func (f *Fake) Now() time.Time { return f.t }

// Advance moves the fake clock forward by d. Negative durations are allowed
// (a test exercising clock-skew behaviour can rewind), but most callers will
// only step forward.
func (f *Fake) Advance(d time.Duration) { f.t = f.t.Add(d) }
