// The settings-page schema, and where it lives.
//
// The fifteen core pages are DATA, and this file is the argument for that.
//
// Until the AppKit port they were Go constants in `app/backend/pages.go` —
// built with map[string]any, marshalled, and handed to a shell that rendered
// them. That worked, but it made the daemon unable to answer a question the
// whole point of a schema-driven UI is that it can answer: "what pages do you
// serve?" There was no RPC for it, because the page list lived in the shell
// process and was never sent anywhere. `core.pages` is that RPC, and this file
// is its data.
//
// The move is small and the direction is the point. A plugin could already
// contribute a page — `UIContribution` is in the plugin API and custom views
// are rejected upstream (MVP rule, bead cross-os-4lm). What a plugin could not
// do was be seen by anything other than the shell that happened to host it.
// Now the page list is something the daemon serves, so the shell, the CLI, a
// test and a future second window all read the same list. The shell stops
// being the only thing that knows what the product contains.
//
// Nothing here renders. A page is a `type`, a `description` and a list of
// controls, and a control is a `kind` plus the fields that kind reads. The
// shell dispatches on `kind`; a kind nobody has heard of is named rather than
// skipped, so a plugin shipping a control the shell predates produces a visible
// gap instead of a hole in the page.

package pluginapi

import "encoding/json"

// PageBody is a page's declarative content: what it is, and what is on it.
type PageBody struct {
	Type        string           `json:"type"`
	Description string           `json:"description,omitempty"`
	Controls    []map[string]any `json:"controls"`
	// FirstRun marks the page a fresh profile lands on. It is read out of the
	// schema rather than declared beside it because the schema IS the page's own
	// description of what it is, and a page that declares nothing is an ordinary
	// page — a schema that is not JSON at all is a malformed contribution, not
	// a first-run one (app/backend/host.go:114-120).
	FirstRun bool `json:"firstRun,omitempty"`
	// TrialTimeoutNote and friends appear on some pages and are not read by the
	// first-run flow; they are here because a schema is allowed to carry more
	// than the shell currently draws, and a plugin that shipped one had no way
	// to know it would not be drawn.
	TrialTimeoutNote string `json:"trialTimeoutNote,omitempty"`
}

// control is the small builder for one control on a page. It exists so the
// page definitions below read as a list of controls rather than as a wall of
// map literals — the same reason the React registry names every kind.
func control(kind, id string, fields map[string]any) map[string]any {
	out := map[string]any{"kind": kind, "id": id}
	for k, v := range fields {
		out[k] = v
	}
	return out
}

// page is the builder for a page body.
func page(body PageBody) json.RawMessage {
	raw, _ := json.Marshal(body)
	return raw
}

// Pages is every page the core serves, in authored order.
//
// The order here is for readability. A shell sorts on Group/Order/firstRun, and
// `TestServedPageOrder` pins the served order so the two cannot drift apart
// silently — the failure being a nav whose order depends on which file a page
// was added to.
func Pages() []UIContribution {
	return []UIContribution{
		homePage(),
		profilesPage(),
		matrixPage(),
		ruleBuilderPage(),
		windowsPage(),
		switcherPage(),
		commandsPage(),
		finderPage(),
		activityPage(),
		observePage(),
		extensionsPage(),
		schemaFormHelp(),
		safetyPage(),
		aboutPage(),
		onboardingFlow(),
	}
}

// --- the pages -------------------------------------------------------------

func homePage() UIContribution {
	return contrib("core.home", "Home", nav{group: "home", symbol: "house"}, page(PageBody{
		Type:        "page",
		Description: "What CrossOS is doing right now — what is on, what is ready, and which profile is active.",
		Controls: []map[string]any{
			control("homeSummary", "status", map[string]any{
				"profile":    "core:profiles",
				"onboarding": "core:onboardingState",
				"note":       "Running, interception, and the active profile in one card.",
			}),
			control("checklist", "readiness", map[string]any{
				"source": "core:readiness",
				"items":  []string{"keyboard", "windows", "finder"},
				"note":   "Each line says what to do when it is not ready.",
			}),
		},
	}), nil, "true")
}

func onboardingFlow() UIContribution {
	return contrib("core.onboard", "Welcome", nav{group: "home", order: 0, symbol: "sparkles"}, page(PageBody{
		Type:        "page",
		Description: "Set up CrossOS. Nothing is enabled until you enable it.",
		FirstRun:    true,
		Controls: []map[string]any{
			control("wizard", "onboard", map[string]any{
				"label":     "Welcome to CrossOS",
				"steps":     []string{"Welcome", "Enable per plugin", "Open System Settings", "Verify ready"},
				"aboutLink": "core.about",
				"trialLink": "core.safety",
			}),
		},
	}), nil, "true")
}

func profilesPage() UIContribution {
	return contrib("core.profiles", "Profiles", nav{group: "home", order: 10, symbol: "square.grid.2x2"}, page(PageBody{
		Type:        "page",
		Description: "Apply a whole configuration at once — rules, shortcuts and extensions together.",
		Controls: []map[string]any{
			control("profileList", "profiles", map[string]any{
				"source": "core:profiles",
				"note":   "Applying a profile records what it overwrote, so it can be put back.",
			}),
		},
	}), nil, "true")
}

func matrixPage() UIContribution {
	return contrib("core.matrix", "Keyboard", nav{group: "shortcuts", order: 10, symbol: "keyboard"}, page(PageBody{
		Type:        "page",
		Description: "What each chord does, and what overrides it.",
		Controls: []map[string]any{
			control("matrix", "behavior", map[string]any{
				"source": "core:matrix",
				"note":   "The base behaviour matrix. An override below replaces one row.",
			}),
			control("overrides", "overrides", map[string]any{
				"source": "core:overrides",
				"note":   "Overrides win over the matrix. Empty means none.",
			}),
		},
	}), nil, "true")
}

func ruleBuilderPage() UIContribution {
	return contrib("core.rules", "Shortcuts", nav{group: "shortcuts", order: 20, symbol: "command"}, page(PageBody{
		Type:        "page",
		Description: "When this key is pressed, do that.",
		Controls: []map[string]any{
			control("shortcutList", "rules", map[string]any{
				"source": "config:userRules",
				"note":   "Conflicts are listed rather than applied — the first rule wins and the rest are shown greyed.",
			}),
			control("ruleBuilder", "builder", map[string]any{
				"note": "Add a rule: an app, an action, and a chord.",
			}),
		},
	}), nil, "true")
}

func windowsPage() UIContribution {
	return contrib("core.windows", "Windows", nav{group: "shortcuts", order: 30, symbol: "rectangle.3.group"}, page(PageBody{
		Type:        "page",
		Description: "How a window behaves when it opens and when you ask it to move.",
		Controls: []map[string]any{
			control("note", "zones", map[string]any{
				"label": "Zones are drawn on screen. Drag one to place it.",
			}),
			control("zoneEditor", "zone", map[string]any{
				"source": "core:zones",
			}),
		},
	}), nil, "true")
}

func switcherPage() UIContribution {
	return contrib("core.switcher", "Switcher", nav{group: "shortcuts", order: 40, symbol: "arrow.triangle.2.circlepath"}, page(PageBody{
		Type:        "page",
		Description: "Hold the switcher chord to see every window, release to go to one.",
		Controls: []map[string]any{
			control("switcherPanel", "panel", map[string]any{
				"source": "core:windows",
				"note":   "The list is in the order the daemon remembers.",
			}),
		},
	}), nil, "true")
}

func commandsPage() UIContribution {
	return contrib("core.commands", "Explorer", nav{group: "shortcuts", order: 50, symbol: "magnifyingglass"}, page(PageBody{
		Type:        "page",
		Description: "Find things: apps, files, and what CrossOS can do with them.",
		Controls: []map[string]any{
			control("menuList", "finder", map[string]any{
				"source": "core:finderMenu",
			}),
			control("fileTypeList", "fileTypes", map[string]any{
				"source": "core:fileTypes",
				"note":   "What the New menu offers, and what opens with which app.",
			}),
		},
	}), nil, "true")
}

func finderPage() UIContribution {
	return contrib("core.finder", "Finder", nav{group: "shortcuts", order: 60, symbol: "list.bullet.rectangle"}, page(PageBody{
		Type:        "page",
		Description: "What the Finder extension adds.",
		Controls: []map[string]any{
			control("note", "finderNote", map[string]any{
				"label": "The Finder Sync extension is what draws CrossOS's entries in Finder.",
				"note":  "If nothing appears in a Finder menu, this is the first thing to check.",
			}),
		},
	}), nil, "true")
}

func activityPage() UIContribution {
	return contrib("core.activity", "Activity", nav{group: "activity", order: 10, symbol: "list.bullet.indent"}, page(PageBody{
		Type:        "page",
		Description: "What CrossOS has seen and done this session.",
		Controls: []map[string]any{
			control("traceList", "traces", map[string]any{
				"source": "core:traces",
				"note":   "Newest last. Clear empties the list; it does not stop recording.",
			}),
		},
	}), nil, "true")
}

func observePage() UIContribution {
	return contrib("core.observe", "Observe", nav{group: "activity", order: 20, symbol: "eye"}, page(PageBody{
		Type:        "page",
		Description: "Watch what CrossOS sees without acting on it.",
		Controls: []map[string]any{
			control("observeToggle", "observe", map[string]any{
				"source": "core:observeState",
				"note":   "Observe records; it does not intercept. Safe to leave on.",
			}),
		},
	}), nil, "true")
}

func extensionsPage() UIContribution {
	return contrib("core.extensions", "Extensions", nav{group: "advanced", order: 10, symbol: "puzzlepiece.extension"}, page(PageBody{
		Type:        "page",
		Description: "What CrossOS can do, and what you have turned on.",
		Controls: []map[string]any{
			control("pluginList", "plugins", map[string]any{
				"source": "plugin:list",
			}),
		},
	}), nil, "true")
}

func schemaFormHelp() UIContribution {
	return contrib("core.schemaForm", "Form example", nav{group: "advanced", order: 20, symbol: "rectangle"}, page(PageBody{
		Type:        "page",
		Description: "A plugin's declarative configuration form, drawn by the shell.",
		Controls: []map[string]any{
			control("schemaForm", "example", map[string]any{
				"source": "plugin:schemas",
				"note":   "This page exists to show the path a plugin's own config form takes. It is a real rendering, not a screenshot.",
			}),
		},
	}), nil, "true")
}

func safetyPage() UIContribution {
	return contrib("core.safety", "Safety", nav{group: "advanced", order: 100, symbol: "gearshape"}, page(PageBody{
		Type:             "page",
		Description:      "Stop everything instantly, reset CrossOS state, review what CrossOS changed.",
		TrialTimeoutNote: "Countdown uses the Core TRIAL timeout (safety.TRIALTimeout); the page never hardcodes it.",
		Controls: []map[string]any{
			control("button", "panicStop", map[string]any{
				"label":  "PANIC STOP — disable interception + plugin actions",
				"action": "safety.panicStop",
				"note":   "Login item stays. Reversible via Re-enable.",
			}),
			control("button", "resume", map[string]any{
				"label":  "Re-enable interception",
				"action": "safety.resume",
				"note":   "Clears a latched PANIC STOP and installs the tap again.",
			}),
			control("button", "reset", map[string]any{
				"label":   "Reset Everything…",
				"action":  "safety.reset",
				"confirm": "Remove login item, disable extension, clean CrossOS-owned state, verify no process remains?",
			}),
			control("trial", "trialCountdown", map[string]any{
				"label":   "New integration trial",
				"source":  "core:trialCountdown",
				"actions": []string{"safety.confirmTrial", "safety.rollbackTrial"},
			}),
			control("auditList", "ownership", map[string]any{
				"label":     "What CrossOS created",
				"source":    "core:ownershipAudit",
				"rowAction": "safety.rollback",
			}),
		},
	}), nil, "true")
}

func aboutPage() UIContribution {
	return contrib("core.about", "About", nav{group: "advanced", order: 110, symbol: "info.circle"}, page(PageBody{
		Type:        "page",
		Description: "What this is, and what it is licensed under.",
		Controls: []map[string]any{
			control("version", "version", map[string]any{
				"source": "core:status",
			}),
			control("license", "license", map[string]any{}),
			control("credits", "credits", map[string]any{}),
		},
	}), nil, "true")
}

// --- the builder -----------------------------------------------------------

type nav struct {
	group  string
	order  int
	symbol string
}

// contrib builds one UIContribution. It is the same shape `app/backend`'s
// `contrib` had, kept here so the daemon and any future consumer build a page
// the same way.
func contrib(id, title string, at nav, schema json.RawMessage, actions []string, visibility string) UIContribution {
	return UIContribution{
		ID:         id,
		Location:   UILocationSettingsPage,
		Title:      title,
		Group:      at.group,
		Symbol:     at.symbol,
		Order:      at.order,
		Schema:     schema,
		Visibility: visibility,
		Actions:    actions,
	}
}
