package ctx

import (
	"sync"
	"sync/atomic"
	"time"
)

// AppMode classifies the focused app for rule filtering. Mirrors the event
// package's AppMode (kept as a distinct type so ctx owns its taxonomy;
// convert at the boundary).
type AppMode string

const (
	AppModeNative   AppMode = "native"
	AppModeTerminal AppMode = "terminal"
	AppModeRemote   AppMode = "remote"
	AppModeVM       AppMode = "vm"
	AppModeExcluded AppMode = "excluded"
)

// AppCategory groups apps for behavior defaults.
type AppCategory string

const (
	AppTerminal AppCategory = "terminal"
	AppBrowser  AppCategory = "browser"
	AppRemote   AppCategory = "remote"
	AppSystem   AppCategory = "system"
	AppUser     AppCategory = "user"
)

// ApplicationInfo describes the focused application.
type ApplicationInfo struct {
	BundleID    string // macOS: com.apple.finder
	Executable  string // process name
	PID         int
	DisplayName string
	AppMode     AppMode
	Category    AppCategory
}

// WindowInfo describes the focused window.
type WindowInfo struct {
	Title        string
	Role         string
	X, Y, W, H   int
	ScreenIndex  int
	IsFullScreen bool
	WindowID     string
}

// SessionInfo carries login/power state (§3.3 bead criterion).
type SessionInfo struct {
	User          string
	Locked        bool
	OnBattery     bool
	DisplayAsleep bool
}

// SelectionInfo describes a Finder/focused selection.
type SelectionInfo struct {
	Items     []string
	Container string
	UTITypes  []string
}

// CursorInfo describes the cursor.
type CursorInfo struct {
	X, Y    int
	Screen  int
	Element string // AX path under cursor
}

// FastContext is the cache-resident hot-path context. Plain data — the
// keydown path copies it out of the Cache, never querying the OS.
//
// Modifiers are CALLER-FILLED, not cache-filled: the native callback overlays
// the live mask from keyboard.State.Words() onto the returned struct before
// matching (modifiers change per keystroke; the cache updates on app/window/
// device change only). Never trust Read().Modifiers without the overlay —
// it is always zero out of the cache. (review: cross-os-c0)
type FastContext struct {
	AppID     string
	AppMode   AppMode
	WindowID  string
	WinClass  string
	DeviceID  string
	Modifiers uint32 // caller-filled from keyboard state; zero from Read()
}

// WithModifiers overlays the live modifier mask onto a cached FastContext.
// The single call site is the native callback between Read and match.
func (f FastContext) WithModifiers(mods uint32) FastContext {
	f.Modifiers = mods
	return f
}

// LazyContext is resolved async on the slow path, only for rules declaring
// requires on it.
type LazyContext struct {
	Selection      SelectionInfo
	Cursor         CursorInfo
	FocusedElement string
	ResolvedAt     time.Time
}

// Context is the full slow-path/debugging context. Lazy is nil unless a
// matching rule required it.
type Context struct {
	Fast        FastContext
	Lazy        *LazyContext
	Application ApplicationInfo
	Window      WindowInfo
	Session     SessionInfo
	At          time.Time
}

// Requires declares which lazy fields a rule needs. Empty = no lazy lookup.
type Requires struct {
	Selection bool
	Cursor    bool
	Element   bool
}

// Needed reports whether any lazy resolution is required.
func (r Requires) Needed() bool {
	return r.Selection || r.Cursor || r.Element
}

// Resolver is a LazyContext provider: given the fast context, resolve the
// lazy fields a rule requires. Implemented by the platform adapter (Spike C
// primitives); tests use a fake. Never called on the callback path.
type Resolver interface {
	Resolve(fast FastContext, req Requires) (LazyContext, error)
	Name() string
}

// Cache holds the hot-path context. Updates arrive on app/window/device
// change events (off-path); reads are lock-free-ish (RWMutex: many
// concurrent readers, rare writers — writes never happen per keydown).
type Cache struct {
	mu       sync.RWMutex
	fast     FastContext
	app      ApplicationInfo
	win      WindowInfo
	session  SessionInfo
	updates  atomic.Uint64 // diagnostic: update count
	reads    atomic.Uint64 // diagnostic: read count
	resolver Resolver      // slow-path lazy provider (nil = lazy unavailable)
}

// NewCache returns a Cache with an optional lazy Resolver (nil allowed).
func NewCache(resolver Resolver) *Cache {
	return &Cache{resolver: resolver}
}

// UpdateApp records an app change (called off-path on focus events).
func (c *Cache) UpdateApp(app ApplicationInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.app = app
	c.fast.AppID = app.BundleID
	if c.fast.AppID == "" {
		c.fast.AppID = app.Executable
	}
	c.fast.AppMode = app.AppMode
	c.updates.Add(1)
}

// UpdateWindow records a window change (called off-path).
func (c *Cache) UpdateWindow(win WindowInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.win = win
	c.fast.WindowID = win.WindowID
	c.fast.WinClass = win.Role
	c.updates.Add(1)
}

// UpdateDevice records a device change.
func (c *Cache) UpdateDevice(deviceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fast.DeviceID = deviceID
	c.updates.Add(1)
}

// UpdateSession records login/power state.
func (c *Cache) UpdateSession(s SessionInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = s
	c.updates.Add(1)
}

// Read returns the hot-path FastContext. This is the ONLY method the
// keydown path calls — sub-microsecond, no syscalls, no AX/UIA.
func (c *Cache) Read() FastContext {
	c.reads.Add(1)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fast
}

// ResolveLazy resolves lazy fields for rules declaring requires. Called on
// the SLOW path only. Returns nil when req needs nothing (no lookup), or
// when no resolver is installed.
func (c *Cache) ResolveLazy(req Requires) *LazyContext {
	if !req.Needed() || c.resolver == nil {
		return nil
	}
	c.mu.RLock()
	fast := c.fast
	c.mu.RUnlock()
	lz, err := c.resolver.Resolve(fast, req)
	if err != nil {
		return nil
	}
	lz.ResolvedAt = time.Now()
	return &lz
}

// Full builds the debugging slow-path Context, resolving lazy only when
// req needs it. Snapshot under ONE RLock, release, THEN resolve lazy
// outside the lock: ResolveLazy takes its own RLock, and Go's RWMutex
// blocks new readers behind a waiting writer — nesting the two deadlocks
// when an Update lands between them. (review P0: cross-os-c0)
func (c *Cache) Full(req Requires) Context {
	c.mu.RLock()
	fast := c.fast
	app := c.app
	win := c.win
	session := c.session
	c.mu.RUnlock()
	return Context{
		Fast:        fast,
		Lazy:        c.ResolveLazy(req),
		Application: app,
		Window:      win,
		Session:     session,
		At:          time.Now(),
	}
}

// Stats returns diagnostic counters.
func (c *Cache) Stats() (updates, reads uint64) {
	return c.updates.Load(), c.reads.Load()
}

// ToEventContext converts to the event package's FastContext shape. The
// boundary converter both taxonomies share — add fields here, not by
// hand-editing two structs. (Kept dependency-free via plain fields: event
// import would cycle event→ctx for future matcher integration.)
// NOTE: returns app/window/device identity only; modifiers overlay via
// WithModifiers at the callback (see FastContext docs).
// TODO(adapter): appMode crosses as a bare string, so a spelling drift
// between ctx.AppMode and event.AppMode would compile silently. The adapter
// bead must add a shared string-constant or compile-time assertion making
// that drift loud. (review follow-up: cross-os-c0)
func (f FastContext) ToEventFields() (appID string, appMode string, windowID string, winClass string, deviceID string) {
	return f.AppID, string(f.AppMode), f.WindowID, f.WinClass, f.DeviceID
}
