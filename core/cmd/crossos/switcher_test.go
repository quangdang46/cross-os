// Switcher tests: the window list, the bounded long-poll, the focus, and the
// two halves of the Alt+Tab gesture.
//
// Two layers, deliberately kept apart. The fixture layer binds the daemon to
// a window source the test controls, so the daemon's own contract — MRU order,
// the wait's bound, the focus reordering the list, the gesture's two phases
// — is asserted everywhere, on a machine with no windows and no Accessibility
// consent. The live layer binds the real adapter and asserts the same
// contract against the machine's own windows, skipping with the real reason
// when the platform denies consent. A fixture that passes where the machine
// cannot is a passing test; a fixture is not a substitute for it.

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"crossos/core/internal/adapter"
	"crossos/core/pkg/event"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/winswitch"
	builtin "crossos/core/rules"
)

// fixtureWindows is a window source the test owns. It models the two things
// the platform contract guarantees and nothing else: the list arrives front
// to back, and a focus moves that window to the front. The front-to-back
// order is deliberately NOT alphabetical, so a re-sort by title in the daemon
// would be visible in the assertions.
type fixtureWindows struct {
	mu    sync.Mutex
	rows  []adapter.WindowRow
	focus []uint32 // every id Focus was asked for, in order
}

func newFixtureWindows() *fixtureWindows {
	return &fixtureWindows{rows: []adapter.WindowRow{
		{ID: 40, PID: 1, BundleID: "com.apple.finder", Title: "zebra.txt"},
		{ID: 10, PID: 2, BundleID: "com.apple.Terminal", Title: "alpha — zsh"},
		{ID: 30, PID: 3, BundleID: "com.microsoft.VSCode", Title: "middle.go"},
	}}
}

func (f *fixtureWindows) ListWindows() ([]adapter.WindowRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]adapter.WindowRow(nil), f.rows...), nil
}

// Focus moves the named window to the front, which is the whole of what the
// WindowServer does to the list when a window is raised.
func (f *fixtureWindows) Focus(id uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.rows {
		if r.ID == id {
			f.rows = append([]adapter.WindowRow{r}, append(append([]adapter.WindowRow{}, f.rows[:i]...), f.rows[i+1:]...)...)
			f.focus = append(f.focus, id)
			return nil
		}
	}
	return fmt.Errorf("fixture: no window %d", id)
}

func (f *fixtureWindows) focusedIDs() []uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint32(nil), f.focus...)
}

// switcherCore builds a daemon whose switcher reads src, with the builtin
// plugins enabled so the gesture's rules are in the decision path.
func switcherCore(t *testing.T, src windowSource) *Core {
	t.Helper()
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), "")
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	c.switcher = newSwitcherService(src)
	return c
}

// liveListener serves c on a real Unix socket and returns a call helper that
// speaks the same JSON-RPC framing the shell sends. Nothing here calls a
// handler directly: a source that was never registered in methods() would
// still pass a direct-call test, which is the failure this file exists to
// rule out.
func liveListener(t *testing.T, c *Core) func(method string, params any) any {
	t.Helper()
	path := filepath.Join(os.TempDir(), fmt.Sprintf("cx-switcher-%d.sock", os.Getpid()))
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := c.Serve(ln)
	t.Cleanup(func() {
		srv.Close()
		_ = os.Remove(path)
	})
	return func(method string, params any) any {
		t.Helper()
		conn, err := net.Dial("unix", path)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		var praw json.RawMessage
		if params != nil {
			if praw, err = json.Marshal(params); err != nil {
				t.Fatalf("marshal params: %v", err)
			}
		}
		req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": praw, "id": 1})
		req = append(req, '\n')
		if _, err := conn.Write(req); err != nil {
			t.Fatalf("write: %v", err)
		}
		dec := json.NewDecoder(conn)
		var resp ipc.Response
		if err := dec.Decode(&resp); err != nil {
			t.Fatalf("read: %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("%s: rpc error %d %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result
	}
}

type wireWindowRow struct {
	WindowID  string `json:"window_id"`
	AppID     string `json:"app_id"`
	Title     string `json:"title"`
	Index     int    `json:"index"`
	Selected  bool   `json:"selected"`
	Skippable bool   `json:"skippable"`
}

type wireWait struct {
	Triggered bool   `json:"triggered"`
	Action    string `json:"action"`
	WindowID  string `json:"window_id"`
}

// TestWindowsServesTheMRUOrderWinswitchComputes: core.windows answers in the
// order the ordering kernel produces, not in an order this file could have
// sorted. The expectation is re-derived THROUGH the kernel on the same
// snapshot the daemon read, which is the only way to say "the order winswitch
// computed" without restating it as a hand-written comparison.
func TestWindowsServesTheMRUOrderWinswitchComputes(t *testing.T) {
	src := newFixtureWindows()
	c := switcherCore(t, src)
	call := liveListener(t, c)

	raw := call("core.windows", nil)
	rows := decode[[]wireWindowRow](t, raw, nil)
	if len(rows) == 0 {
		t.Fatal("core.windows returned no rows; the switcher has nothing to choose from")
	}

	snapshot, err := src.ListWindows()
	if err != nil {
		t.Fatalf("fixture list: %v", err)
	}
	var want []winswitch.Row
	for i, w := range snapshot {
		want = append(want, winswitch.Row{
			ID: strconv.FormatUint(uint64(w.ID), 10), AppID: w.BundleID,
			Title: w.Title, LastFocusOrder: i, Minimized: w.Minimized,
		})
	}
	winswitch.Sort(want, winswitch.OrderOptions{Mode: winswitch.SortRecentlyFocused})
	if len(want) != len(rows) {
		t.Fatalf("core.windows served %d rows, the kernel ranked %d", len(rows), len(want))
	}
	for i := range want {
		if rows[i].WindowID != want[i].ID {
			t.Fatalf("row %d: window_id=%q, the kernel ranked %q first there (full order served %v)",
				i, rows[i].WindowID, want[i].ID, idsOf(rows))
		}
		if rows[i].Index != i {
			t.Fatalf("row %d carries index %d — the index must be its position in the served order", i, rows[i].Index)
		}
	}
	// The fixture's titles are deliberately not in MRU order, so an ordering
	// that sorted by them would not read the same list twice.
	if rows[0].Title != "zebra.txt" {
		t.Fatalf("front row is %q; the platform answers front to back and zebra.txt is the front window", rows[0].Title)
	}
}

func idsOf(rows []wireWindowRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.WindowID)
	}
	return out
}

// TestSwitcherWaitIsBoundedWhenIdle: the long-poll spends its budget and
// says nothing happened. Both halves matter — answering instantly would be
// a busy loop the shell cannot survive, and never answering is the hang the
// bound exists to prevent.
func TestSwitcherWaitIsBoundedWhenIdle(t *testing.T) {
	c := switcherCore(t, newFixtureWindows())
	call := liveListener(t, c)

	start := time.Now()
	got := decode[wireWait](t, call("core.switcherWait", map[string]any{"timeout_ms": 2000}), nil)
	elapsed := time.Since(start)

	if got.Triggered {
		t.Fatalf("idle wait reported %q for window %q — nothing summoned it", got.Action, got.WindowID)
	}
	if elapsed > 2200*time.Millisecond {
		t.Fatalf("idle wait(2000) took %v, over the 2.2s bound", elapsed)
	}
	if elapsed < 1900*time.Millisecond {
		t.Fatalf("idle wait(2000) returned after %v — it did not wait, so the shell would spin", elapsed)
	}
}

// TestSwitcherWaitIsCappedNotJustBounded: a caller asking to park forever is
// refused the park. Without the cap a shell that reconnects on a timer leaves
// one parked connection per reconnect, and the daemon grows a goroutine per
// reconnect for as long as it is up. Asserted on the budget the handler would
// spend, so the test does not have to sit through the cap to watch it work.
func TestSwitcherWaitIsCappedNotJustBounded(t *testing.T) {
	for _, tc := range []struct {
		requested int
		want      time.Duration
	}{
		{0, switcherWaitDefaultMs * time.Millisecond},
		{-5, switcherWaitDefaultMs * time.Millisecond},
		{2000, 2 * time.Second},
		{99999999, switcherWaitMaxMs * time.Millisecond},
	} {
		if got := waitBudget(tc.requested); got != tc.want {
			t.Errorf("waitBudget(%d)=%v, want %v", tc.requested, got, tc.want)
		}
	}
}

// TestTheChordCommitsWithin100msOfDispatch: the release of Alt+Tab goes
// through the real decision path and its dispatch, and a shell already
// waiting learns the commit inside 100ms. The summon fires first because a
// commit with no summon behind it is refused by design, so the test performs
// the whole gesture rather than the release alone.
func TestTheChordCommitsWithin100msOfDispatch(t *testing.T) {
	src := newFixtureWindows()
	c := switcherCore(t, src)
	call := liveListener(t, c)
	ctx := event.FastContext{AppID: "com.apple.finder", AppMode: event.AppModeNative}
	press := event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard,
		KeyCode: 0x09, Modifiers: 1 << 2}
	release := event.Event{Type: event.EventKeyUp, Source: event.SourceKeyboard,
		KeyCode: 0x09, Modifiers: 1 << 2}

	// Drain the summon so the waiter below is parked on the commit.
	if out := decideAndDispatch(c, press, ctx); out.WinnerRule != "windows-keyboard.alt-tab-summon-switcher" {
		t.Fatalf("press resolved to %q", out.WinnerRule)
	}
	if got := decode[wireWait](t, call("core.switcherWait", nil), nil); !got.Triggered || got.Action != "summon" {
		t.Fatalf("summon trigger not delivered: %+v", got)
	}

	// Park a waiter, then release the chord. The release has to travel the
	// real path: decideLocked (real router, real grants) then dispatch.
	type answer struct {
		raw   json.RawMessage
		at    time.Time
		fired bool
	}
	answers := make(chan answer, 1)
	go func() {
		raw, rerr := c.handleSwitcherWait(json.RawMessage(`{"timeout_ms":5000}`))
		if rerr != nil {
			answers <- answer{fired: false}
			return
		}
		blob, _ := json.Marshal(raw)
		answers <- answer{raw: blob, at: time.Now(), fired: true}
	}()
	time.Sleep(50 * time.Millisecond) // let the waiter park

	dispatched := time.Now()
	out := decideAndDispatch(c, release, ctx)
	if out.WinnerRule != "windows-keyboard.alt-tab-commit-switcher" {
		t.Fatalf("release resolved to %q, want the commit rule", out.WinnerRule)
	}
	if out.Decision != pluginapi.DecisionReplace {
		t.Fatalf("release decision=%v, want replace", out.Decision)
	}
	if out.Intent.ID != "window.switcher" {
		t.Fatalf("release intent=%q, want window.switcher", out.Intent.ID)
	}

	select {
	case a := <-answers:
		if !a.fired {
			t.Fatal("the parked wait returned an rpc error")
		}
		if latency := a.at.Sub(dispatched); latency > 100*time.Millisecond {
			t.Fatalf("commit reached the waiter in %v, over the 100ms bound", latency)
		}
		got := decode[wireWait](t, a.raw, nil)
		if !got.Triggered || got.Action != "commit" {
			t.Fatalf("waiter got %+v, want a commit trigger", got)
		}
		if got.WindowID == "" {
			t.Fatal("commit trigger named no window")
		}
		// The default pick is the window the user was on before, never the
		// one already on top — committing to the front window would make the
		// gesture a no-op.
		if got.WindowID == "40" {
			t.Fatalf("commit chose window 40, the window already in front")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the parked wait never woke for the commit")
	}
}

// decideAndDispatch is the tap's pair: decide, then run what the decision
// authorized. Handing the request straight to dispatch from a test would skip
// exactly the step under test.
func decideAndDispatch(c *Core, ev event.Event, ctx event.FastContext) event.Outcome {
	out := c.decideLocked(ev, ctx)
	if out.Decision == pluginapi.DecisionReplace && out.Request != nil {
		if err := c.dispatch(*out.Request); err != nil {
			// A refusal is reported, never swallowed — and a commit that
			// finds no window is not a reason to keep the key suppressed
			// silently. The tests that care read this.
			out.Traces = append(out.Traces, event.StageLog{Stage: "dispatch", Detail: err.Error()})
		}
	}
	return out
}

// TestSwitcherFocusMakesTheWindowFront: focusing a window that is not in
// front puts it at index 0 of the next core.windows answer. The assertion
// reads the list back over the listener rather than trusting the focus call,
// so "it returned ok" cannot stand in for "it actually moved".
func TestSwitcherFocusMakesTheWindowFront(t *testing.T) {
	src := newFixtureWindows()
	c := switcherCore(t, src)
	call := liveListener(t, c)

	before := decode[[]wireWindowRow](t, call("core.windows", nil), nil)
	if len(before) < 2 {
		t.Skipf("need two windows to have a non-front one to focus; the machine listed %d", len(before))
	}
	target := before[len(before)-1].WindowID
	if target == before[0].WindowID {
		t.Fatalf("picked window %q is already in front", target)
	}

	res := decode[struct {
		WindowID string `json:"window_id"`
		Focused  bool   `json:"focused"`
	}](t, call("core.switcherFocus", map[string]any{"window_id": target}), nil)
	if !res.Focused || res.WindowID != target {
		t.Fatalf("switcherFocus answered %+v for %q", res, target)
	}

	after := decode[[]wireWindowRow](t, call("core.windows", nil), nil)
	if after[0].WindowID != target {
		t.Fatalf("after focusing %q the front window is %q — focus did not move it (order %v)",
			target, after[0].WindowID, idsOf(after))
	}
	// The reordering is the commitment: the focused window leads the MRU it
	// is now part of, and the windows it passed keep their relative order. A
	// focus that reshuffled the rest would be a list the user cannot predict.
	want := []string{target}
	for _, r := range before {
		if r.WindowID != target {
			want = append(want, r.WindowID)
		}
	}
	for i := range want {
		if after[i].WindowID != want[i] {
			t.Fatalf("row %d is %q, want %q — a focus must move the target to the front and nothing else (served %v, want %v)",
				i, after[i].WindowID, want[i], idsOf(after), want)
		}
	}
	if got := src.focusedIDs(); len(got) != 1 || strconv.FormatUint(uint64(got[0]), 10) != target {
		t.Fatalf("the seam was asked to focus %v, want exactly [%s]", got, target)
	}
}

// TestAReleaseWithNoSummonCommitsNothing: the release half of the gesture on
// its own must not focus a window. Committing anyway would move the user to
// whatever the default pick happened to name, which is the one outcome they
// cannot have meant.
func TestAReleaseWithNoSummonCommitsNothing(t *testing.T) {
	src := newFixtureWindows()
	c := switcherCore(t, src)
	release := event.Event{Type: event.EventKeyUp, Source: event.SourceKeyboard,
		KeyCode: 0x09, Modifiers: 1 << 2}

	out := c.decideLocked(release, event.FastContext{AppID: "com.apple.finder", AppMode: event.AppModeNative})
	if out.WinnerRule != "windows-keyboard.alt-tab-commit-switcher" {
		t.Fatalf("release resolved to %q, want the commit rule", out.WinnerRule)
	}
	if out.Request == nil {
		t.Fatal("the commit rule produced no authorized request")
	}
	if err := c.dispatch(*out.Request); err == nil {
		t.Fatal("a release with no summon committed a window")
	}
	if got := src.focusedIDs(); len(got) != 0 {
		t.Fatalf("focus was called with %v on a release with no summon", got)
	}
	if _, queued := c.switcher.take(); queued {
		t.Fatal("a refused commit still queued a trigger for the shell")
	}
}

// TestTheGestureIsNotAChordConflict: two rules on one chord that fire on
// different phases are one gesture, not a collision. Reporting them as a
// conflict would put a phantom row in the resolver and train the user to
// expect the switcher to break.
func TestTheGestureIsNotAChordConflict(t *testing.T) {
	c := switcherCore(t, newFixtureWindows())
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	conflicts, rerr := c.handleConflicts(nil)
	if rerr != nil {
		t.Fatalf("handleConflicts: %d %s", rerr.Code, rerr.Message)
	}
	rows := decodeRows[struct {
		Keys   string   `json:"keys"`
		Winner string   `json:"winner"`
		Losers []string `json:"losers"`
	}](t, conflicts, nil)
	for _, r := range rows {
		for _, loser := range r.Losers {
			if loser == "windows-keyboard.alt-tab-commit-switcher" ||
				loser == "windows-keyboard.alt-tab-summon-switcher" {
				t.Fatalf("the switcher gesture is reported as a conflict on %q: winner %q, loser %q",
					r.Keys, r.Winner, loser)
			}
		}
	}
}

// TestSwitcherMethodsAreRegistered: a handler left out of methods() is dead
// code the shell can never reach, and only a direct call would see it.
func TestSwitcherMethodsAreRegistered(t *testing.T) {
	c := switcherCore(t, newFixtureWindows())
	reg := c.methods()
	for _, name := range []string{"core.windows", "core.switcherWait", "core.switcherFocus"} {
		if _, ok := reg[name]; !ok {
			t.Errorf("switcher source %q is not in the method table", name)
		}
	}
}

// TestLiveListenerAgainstTheMachineWindows: the same contract, against the
// machine's own windows through the real adapter seam. Skipped — with the
// platform's own reason — when the machine has not granted Accessibility
// consent, because that is a permission state and not a defect to assert
// around.
func TestLiveListenerAgainstTheMachineWindows(t *testing.T) {
	lister := adapter.NewWindowLister(&adapter.Driver{Log: daemonTapLog{}})
	snapshot, err := lister.ListWindows()
	if err != nil {
		t.Skipf("the window server is not answering on this machine: %v", err)
	}
	if len(snapshot) < 2 {
		t.Skipf("the machine lists %d named windows; focusing a non-front one needs 2", len(snapshot))
	}

	c := switcherCore(t, lister)
	call := liveListener(t, c)
	before := decode[[]wireWindowRow](t, call("core.windows", nil), nil)
	if len(before) == 0 {
		t.Fatal("core.windows served no rows over a listener while the machine has windows")
	}
	target := before[len(before)-1].WindowID
	if _, rerr := c.handleSwitcherFocus(json.RawMessage(`{"window_id":"` + target + `"}`)); rerr != nil {
		t.Fatalf("switcherFocus(%s): %v", target, rerr.Message)
	}
	after := decode[[]wireWindowRow](t, call("core.windows", nil), nil)
	if after[0].WindowID != target {
		t.Fatalf("after focusing %s the front window is %s", target, after[0].WindowID)
	}
}
