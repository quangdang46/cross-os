package event

import "sync"

// Observer receives already-decided Outcomes on the slow path. Observers
// MUST be async and best-effort: a slow observer never blocks a decision,
// and a dropped record is accounted, never retried into the fast path.
type Observer interface {
	OnOutcome(Outcome)
	Name() string
}

// Bus is the EventBus-named observation interface. It is NOT a queue the
// decision path sits behind — Decide runs first and returns to the OS;
// Publish is called AFTER return with the already-decided Outcome.
//
// "No-drop" applies to accounting, not delivery: if a bounded buffer fills,
// the Bus counts the drop (Dropped++) so the Recorder can report the gap,
// instead of blocking or growing unboundedly.
type Bus struct {
	mu        sync.Mutex
	observers []Observer
	pending   []Outcome // bounded buffer, drained by Drain
	cap       int
	Dropped   int
}

// NewBus returns a Bus with the given buffer bound. Non-positive cap means
// a sane default (256).
func NewBus(capacity int) *Bus {
	if capacity <= 0 {
		capacity = 256
	}
	return &Bus{cap: capacity}
}

// Subscribe registers a slow-path observer (recorder, UI log, plugin IPC).
func (b *Bus) Subscribe(o Observer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.observers = append(b.observers, o)
}

// Publish enqueues an already-decided Outcome. Called AFTER the native
// callback returns — never inside it. Drops (counted) when full.
func (b *Bus) Publish(out Outcome) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.pending) >= b.cap {
		b.Dropped++
		return
	}
	b.pending = append(b.pending, out)
}

// Drain delivers all buffered Outcomes to every observer, in order. Called
// by the slow-path worker, never on the callback path.
func (b *Bus) Drain() {
	b.mu.Lock()
	batch := b.pending
	b.pending = nil
	obs := append([]Observer(nil), b.observers...)
	b.mu.Unlock()
	for _, out := range batch {
		for _, o := range obs {
			o.OnOutcome(out)
		}
	}
}

// Pending returns the current buffer depth (for tests/health).
func (b *Bus) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}
