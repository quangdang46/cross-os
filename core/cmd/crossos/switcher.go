// The Alt+Tab window switcher on the daemon side: the window list the shell
// draws, the bounded long-poll it waits on, and the focus action a commit
// performs. The ordering and the highlight are NOT decided here — winswitch
// owns both (pkg/winswitch), this file is the seam that feeds it a snapshot
// and acts on the index it hands back.
//
// Two rules and one capability make the whole gesture. window.switcher with
// {"action":"summon"} fires on the key-down, {"action":"commit"} on the
// key-up of the same chord, so the release commits the highlight the way
// AltTab does (ShortcutAction.swift:39-49). Acting on a tile is a
// commitment, so after the focus lands the switcher's own selection follows
// that window by id rather than being re-derived.
package main

import (
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"crossos/core/internal/adapter"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/winswitch"
)

// switcherWaitDefaultMs and switcherWaitMaxMs bound the long-poll. The cap is
// the point: an unbounded wait parks an IPC connection forever, and a shell
// that reconnects on a timer would pile them up one per reconnect.
const (
	switcherWaitDefaultMs = 2000
	switcherWaitMaxMs     = 30000
)

// windowSource is the platform seam the switcher programs against — the
// window list and the focus action, nothing else. adapter.WindowQuery
// satisfies it, so production binds the real AX/WindowServer answer and a
// test can bind a fixture without the window server being involved at all.
type windowSource interface {
	ListWindows() ([]adapter.WindowRow, error)
	Focus(id uint32) error
}

// switcherTrigger is one thing the switcher did that the shell has not seen
// yet. Action is the switcher's own vocabulary (summon/commit), not a rule
// ID and not an intent ID: the shell draws a switcher, it does not read the
// decision path.
type switcherTrigger struct {
	Action   string
	WindowID string
}

// switcherService is the open switcher. The zero value is closed and ready to
// be summoned; the shell never owns this state, the daemon does, so a reload
// cannot strand a highlight on a window the list has since dropped.
type switcherService struct {
	source windowSource

	mu   sync.Mutex
	sess winswitch.Session
	// open records that a summon is waiting for its commit, so a release with
	// no summon behind it commits nothing instead of focusing whatever the
	// default pick happens to be.
	open bool
	// queue is the triggers the shell has not read. A slice rather than a
	// fixed-size channel because a commit that is dropped is a keystroke the
	// user gets no answer to.
	queue  []switcherTrigger
	signal chan struct{}
	// max bounds the queue against a shell that never reads it. The oldest
	// trigger goes first: a stale summon is noise, and the newest commit is
	// the one the user is still holding a chord down for.
	max int
}

func newSwitcherService(src windowSource) *switcherService {
	return &switcherService{source: src, signal: make(chan struct{}, 1), max: 8}
}

// fire queues a trigger and wakes one waiter. Never blocks: it runs on the
// tap's dispatch worker, and a stalled dispatch worker bricks the keyboard.
func (s *switcherService) fire(t switcherTrigger) {
	s.mu.Lock()
	if len(s.queue) >= s.max {
		s.queue = s.queue[1:]
	}
	s.queue = append(s.queue, t)
	s.mu.Unlock()
	select {
	case s.signal <- struct{}{}:
	default: // a waiter is already awake and will drain the queue
	}
}

// take pops the oldest unread trigger.
func (s *switcherService) take() (switcherTrigger, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return switcherTrigger{}, false
	}
	t := s.queue[0]
	s.queue = s.queue[1:]
	return t, true
}

// rows reads the live window list and turns it into switcher rows.
//
// LastFocusOrder is the list's own position: the platform answers front to
// back, and front-most is the window that was focused last, so the index IS
// the MRU rank. There is no separate focus history to consult, and inventing
// one by watching focus events would be a second source of truth that the
// ordering kernel has no way to know about.
func (s *switcherService) rows() ([]winswitch.Row, error) {
	raw, err := s.source.ListWindows()
	if err != nil {
		return nil, err
	}
	out := make([]winswitch.Row, 0, len(raw))
	for i, w := range raw {
		out = append(out, winswitch.Row{
			ID:             strconv.FormatUint(uint64(w.ID), 10),
			AppID:          w.BundleID,
			Title:          w.Title,
			LastFocusOrder: i,
			Minimized:      w.Minimized,
		})
	}
	// The ordering is winswitch's to decide. MRU is the default mode, and the
	// zero OrderOptions is exactly that, so nothing here re-sorts the list.
	winswitch.Sort(out, winswitch.OrderOptions{Mode: winswitch.SortRecentlyFocused})
	return out, nil
}

// handleWindows serves core.windows, the switcher page's data source: the
// list in MRU order, each row carrying the index the selection is on so the
// shell can draw a highlight from one response. An empty list is [] and not
// null — a switcher with no windows is a state the shell has to render, not
// an error.
func (c *Core) handleWindows(_ json.RawMessage) (any, *ipc.RPCError) {
	rows, err := c.switcher.rows()
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	c.switcher.mu.Lock()
	// The current window is the front tile whenever the list has one, so the
	// default pick steps over it to reach "the window you were on before".
	currentDrawn := len(rows) > 0
	out := make([]switcherRow, 0, len(rows))
	for i, r := range rows {
		out = append(out, switcherRow{
			WindowID:  r.ID,
			AppID:     r.AppID,
			Title:     r.Title,
			Index:     i,
			Skippable: r.Skippable(),
		})
	}
	// The highlight is read AFTER the decide, never before it: the decide is
	// what moves the selection, so a response that labelled the rows first
	// would draw the previous turn's highlight and then disagree with the
	// next response about the same list.
	dec := c.switcher.sess.Decide(rows, winswitch.Update{CurrentWindowDrawn: currentDrawn})
	c.switcher.mu.Unlock()
	for i := range out {
		out[i].Selected = i == dec.Index
	}
	return out, nil
}

// switcherRow is one drawn tile. Selected is the highlight the session just
// decided on, not a second opinion kept alongside it.
// switcherActionNames is the window.switcher action vocabulary, in the words
// the user performs it. Kept next to summon/commit so the wire vocabulary and
// the display vocabulary cannot drift apart by editing one of them.
var switcherActionNames = map[string]string{
	"summon": "Open Window Switcher",
	"commit": "Switch To Selected Window",
}

// switcherAction names a window.switcher's action for the matrix, or "" when
// the intent carries no action the switcher has a word for. A malformed or
// unknown action falls through to the intent ID rather than borrowing a
// neighbour's label.
func switcherAction(in intent.Intent) string {
	if len(in.Parameters) == 0 {
		return ""
	}
	var p struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(in.Parameters, &p); err != nil {
		return ""
	}
	return switcherActionNames[p.Action]
}

type switcherRow struct {
	WindowID  string `json:"window_id"`
	AppID     string `json:"app_id"`
	Title     string `json:"title"`
	Index     int    `json:"index"`
	Selected  bool   `json:"selected"`
	Skippable bool   `json:"skippable"`
}

// waitBudget is the budget a wait actually spends: the caller's, the default
// when they name none, and never more than the cap. It is a function so the
// cap is testable without spending 30 seconds watching one expire.
func waitBudget(requestedMs int) time.Duration {
	if requestedMs <= 0 {
		requestedMs = switcherWaitDefaultMs
	}
	if requestedMs > switcherWaitMaxMs {
		requestedMs = switcherWaitMaxMs
	}
	return time.Duration(requestedMs) * time.Millisecond
}

// handleSwitcherWait serves core.switcherWait: the long-poll the shell waits
// on while no chord is down. It returns the oldest unread trigger, or
// {triggered:false} once the caller's own budget is spent. The wait is always
// bounded by the request — there is no path that blocks indefinitely.
func (c *Core) handleSwitcherWait(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		TimeoutMs int `json:"timeout_ms"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {timeout_ms}"}
		}
	}
	budget := waitBudget(p.TimeoutMs)
	// The loop, not a single select: a signal is only a hint that the queue
	// may be non-empty, and the two can disagree. A trigger taken by an
	// earlier caller leaves its hint behind, and a waiter that treated the
	// hint as the answer woke to say "nothing happened" on a queue that was
	// about to receive a commit. Re-checking the queue and parking again is
	// what makes a stale hint cost one turn instead of a lost keystroke.
	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	for {
		if t, ok := c.switcher.take(); ok {
			return waitAnswer(t), nil
		}
		select {
		case <-c.switcher.signal:
		case <-deadline.C:
			return waitAnswer(switcherTrigger{}), nil
		}
	}
}

// waitAnswer is the wait's answer shape. triggered is the whole contract:
// false means the budget ran out with nothing to say, and the shell asks
// again.
func waitAnswer(t switcherTrigger) map[string]any {
	if t.Action == "" {
		return map[string]any{"triggered": false}
	}
	out := map[string]any{"triggered": true, "action": t.Action}
	if t.WindowID != "" {
		out["window_id"] = t.WindowID
	}
	return out
}

// handleSwitcherFocus serves core.switcherFocus: the tile the user pointed at
// becomes the front window. It is the same action the commit half of the
// gesture performs, exposed so the shell can act on a click without
// synthesizing a keystroke.
func (c *Core) handleSwitcherFocus(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		WindowID string `json:"window_id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.WindowID == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {window_id}"}
	}
	id, err := strconv.ParseUint(p.WindowID, 10, 32)
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "window_id is not a window id"}
	}
	if err := c.switcher.source.Focus(uint32(id)); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"window_id": p.WindowID, "focused": true}, nil
}

// errNoSummon is what a release with nothing summoned behind it resolves to.
// Committing anyway would focus whatever the default pick happens to name,
// which is the one outcome the user cannot have meant.
var errNoSummon = errors.New("switcher: release with no summon behind it")

// apply runs an authorized window.switcher request. It is the capability's
// execution, called from the dispatch worker exactly like every other
// capability — the rules decide, this only acts.
func (c *Core) applySwitcher(req intent.Request) error {
	var p struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(req.Intent.Parameters, &p); err != nil {
		return err
	}
	switch p.Action {
	case "summon":
		return c.switcher.summon()
	case "commit":
		return c.switcher.commit()
	default:
		return errors.New("core: window.switcher action " + strconv.Quote(p.Action) + " is not in the vocabulary")
	}
}

// summon opens the session over the live list and tells the shell to draw.
// The first pick is taken on this turn, over the same rows the first refresh
// will read, so the count the newcomer rule is compared against is measured
// before any window event can land behind the press.
func (s *switcherService) summon() error {
	rows, err := s.rows()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.sess.Summon(rows, len(rows) > 0)
	s.open = true
	s.mu.Unlock()
	s.fire(switcherTrigger{Action: "summon"})
	return nil
}

// commit focuses the window the session is on and closes the switcher. The
// target is the session's, not a re-derivation: a default that re-picked
// here would land on a different tile than the one the user was looking at
// when they let go.
func (s *switcherService) commit() error {
	s.mu.Lock()
	open := s.open
	s.open = false
	s.mu.Unlock()
	if !open {
		return errNoSummon
	}
	rows, err := s.rows()
	if err != nil {
		return err
	}
	s.mu.Lock()
	dec := s.sess.Decide(rows, winswitch.Update{CurrentWindowDrawn: len(rows) > 0})
	s.sess.Close()
	s.mu.Unlock()
	if dec.Index == winswitch.NoIndex {
		return nil // nothing drawn: the switcher closes, and no window is touched
	}
	id, err := strconv.ParseUint(dec.TargetID, 10, 32)
	if err != nil {
		return err
	}
	if err := s.source.Focus(uint32(id)); err != nil {
		return err
	}
	s.fire(switcherTrigger{Action: "commit", WindowID: dec.TargetID})
	return nil
}
