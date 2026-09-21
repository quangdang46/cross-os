package event

// EventType is the minimal normalized input vocabulary. The native callback
// produces these; anything richer (selection, cursor, UTI) is LazyContext on
// the slow path, never here.
type EventType int

const (
	EventKeyDown EventType = iota
	EventKeyUp
	EventFlagsChanged
)

// EventSource names the physical origin.
type EventSource string

const (
	SourceKeyboard  EventSource = "keyboard"
	SourceHotkey    EventSource = "hotkey"
	SourceSynthetic EventSource = "synthetic" // CrossOS-injected; see injectTag
)

// AppMode classifies the focused app for rule filtering.
type AppMode string

const (
	AppModeNative   AppMode = "native"
	AppModeTerminal AppMode = "terminal"
	AppModeRemote   AppMode = "remote"
	AppModeVM       AppMode = "vm"
	AppModeExcluded AppMode = "excluded"
)

// Event is the normalized minimal input record. Allocation-light by design:
// the callback fills this struct, nothing more. Known callback-path allocs
// (named, not hidden): Outcome.At (one timestamp) and the candidates slice
// header in Decide. n (rule count) is small; the winner re-scan is O(n).
// (review: cross-os-c0)
type Event struct {
	Type      EventType
	Source    EventSource
	KeyCode   uint32 // platform virtual key
	Modifiers uint32 // modifier bitmask (platform-normalized)
	DeviceID  string // physical keyboard identity (for remapping)
}

// FastContext supplies decision inputs from cache (updated on app/window
// change, NEVER queried synchronously per keydown — no AX, no selection
// lookup on this path).
type FastContext struct {
	AppID    string // bundle ID / executable
	AppMode  AppMode
	WindowID string
	WinClass string
	DeviceID string
}
