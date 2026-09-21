package ctx

import "strings"

// Classifier maps an executable/bundle ID to (AppMode, Category). The
// Phase-0 seed list classifies well-known terminals/remotes/VMs; jd9 plugs
// a config-driven classifier into the same interface.
type Classifier interface {
	Classify(bundleID, executable string) (AppMode, AppCategory)
}

// seedEntry is one built-in classification rule.
type seedEntry struct {
	match string // lowercase substring of bundle ID or executable
	mode  AppMode
	cat   AppCategory
}

// seedList is the Phase-0 built-in table. Small and conservative: unknown
// apps classify native/user (fail-open to normal behavior, never disable).
var seedList = []seedEntry{
	// Terminals (Ctrl+C = INTERRUPT).
	{"terminal", AppModeTerminal, AppTerminal},
	{"iterm", AppModeTerminal, AppTerminal},
	{"wezterm", AppModeTerminal, AppTerminal},
	{"warp", AppModeTerminal, AppTerminal},
	{"alacritty", AppModeTerminal, AppTerminal},
	{"kitty", AppModeTerminal, AppTerminal},
	{"hyper", AppModeTerminal, AppTerminal},
	{"ghostty", AppModeTerminal, AppTerminal},
	{"conhost", AppModeTerminal, AppTerminal},
	{"windowsterminal", AppModeTerminal, AppTerminal},
	// Remote desktops (CrossOS disabled).
	{"rdc", AppModeRemote, AppRemote},
	{"mstsc", AppModeRemote, AppRemote},
	{"citrix", AppModeRemote, AppRemote},
	{"teamviewer", AppModeRemote, AppRemote},
	{"anydesk", AppModeRemote, AppRemote},
	// VMs (CrossOS disabled).
	{"parallels", AppModeVM, AppRemote},
	{"vmware", AppModeVM, AppRemote},
	{"virtualbox", AppModeVM, AppRemote},
	{"vbox", AppModeVM, AppRemote},
	// Browsers.
	{"chrome", AppModeNative, AppBrowser},
	{"firefox", AppModeNative, AppBrowser},
	{"safari", AppModeNative, AppBrowser},
	{"edge", AppModeNative, AppBrowser},
	{"arc", AppModeNative, AppBrowser},
}

// SeedClassifier is the Phase-0 built-in classifier.
type SeedClassifier struct{}

// Classify returns the seed-list match, or native/user for unknown apps.
func (SeedClassifier) Classify(bundleID, executable string) (AppMode, AppCategory) {
	hay := strings.ToLower(bundleID + "\x00" + executable)
	for _, e := range seedList {
		if strings.Contains(hay, e.match) {
			return e.mode, e.cat
		}
	}
	return AppModeNative, AppUser
}
