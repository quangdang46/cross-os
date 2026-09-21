package daemon

import (
	"fmt"
	"sync"
	"time"

	"crossos/core/pkg/pluginapi"
)

// Daemon is the core lifecycle owner. States mirror the daemon half of the
// pluginapi transition table (initializing/running/safemode/paused/stopped).
// All transitions go through pluginapi.Transition — illegal moves fail,
// never silently pass.
type Daemon struct {
	mu     sync.Mutex
	state  pluginapi.LifecycleState
	since  time.Time // last transition time; see Since()
	events []StateEvent
}

// Since returns when the current state was entered.
func (d *Daemon) Since() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.since
}

// StateEvent audits one daemon transition.
type StateEvent struct {
	At   time.Time
	From pluginapi.LifecycleState
	To   pluginapi.LifecycleState
}

// New returns a Daemon in initializing state.
func New() *Daemon {
	return &Daemon{state: pluginapi.LifecycleInitializing, since: time.Now()}
}

// State returns the current lifecycle state.
func (d *Daemon) State() pluginapi.LifecycleState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// transition validates and records a move.
func (d *Daemon) transition(to pluginapi.LifecycleState) error {
	if err := pluginapi.Transition(d.state, to); err != nil {
		return err
	}
	d.events = append(d.events, StateEvent{At: time.Now(), From: d.state, To: to})
	d.state = to
	d.since = time.Now()
	return nil
}

// Run moves initializing → running.
func (d *Daemon) Run() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.transition(pluginapi.LifecycleRunning)
}

// EnterSafeMode moves running → safemode (mechanism; the 30s TRIAL
// semantics are safety.Trial's job).
func (d *Daemon) EnterSafeMode() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.transition(pluginapi.LifecycleSafeMode)
}

// ExitSafeMode moves safemode → running (user confirmed).
func (d *Daemon) ExitSafeMode() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.transition(pluginapi.LifecycleRunning)
}

// Pause moves running → paused.
func (d *Daemon) Pause() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.transition(pluginapi.LifecyclePaused)
}

// Resume moves paused → running. The pluginapi table routes paused through
// enabled (paused → enabled is legal); the daemon then treats enabled as
// live and returns to running, since enabled → running is a daemon-internal
// step, not a plugin enable state. Both moves are recorded.
func (d *Daemon) Resume() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state != pluginapi.LifecyclePaused {
		return fmt.Errorf("daemon: cannot resume from %q", d.state)
	}
	if err := d.transition(pluginapi.LifecycleEnabled); err != nil {
		return err
	}
	// enabled → running is daemon-internal (live again). Recorded directly
	// with full From/At like every other event — a sourceless event would
	// corrupt recorder/UI history. (review fix: cross-os-c0)
	d.events = append(d.events, StateEvent{
		At: time.Now(), From: pluginapi.LifecycleEnabled,
		To: pluginapi.LifecycleRunning})
	d.state = pluginapi.LifecycleRunning
	d.since = time.Now()
	return nil
}

// Stop moves running/safemode/paused/enabled → stopped. Enabled is accepted
// because Resume lands there transiently (see above); stopping from a live
// state must never fail.
func (d *Daemon) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.state == pluginapi.LifecycleEnabled {
		d.events = append(d.events, StateEvent{From: d.state, To: pluginapi.LifecycleStopped})
		d.state = pluginapi.LifecycleStopped
		d.since = time.Now()
		return nil
	}
	return d.transition(pluginapi.LifecycleStopped)
}

// Events returns the audited transition history.
func (d *Daemon) Events() []StateEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]StateEvent(nil), d.events...)
}
