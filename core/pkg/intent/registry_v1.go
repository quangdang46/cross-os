package intent

// Canonical registry v1 (§3.12). Every capability CrossOS v1 provides, with
// permission, platforms, side effects, and reversibility. Plugin-owned
// capabilities live outside this table, namespaced under their plugin ID.
func canonicalV1() []CapabilityDescriptor {
	noParams := Schema{}
	return []CapabilityDescriptor{
		{
			ID: "input.observe", Version: "1",
			Description: "Observe input events without modifying them",
			Permission:  PermInputMonitor,
			InputSchema: noParams,
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			Reversible:  true,
		},
		{
			ID: "input.intercept", Version: "1",
			Description: "Suppress/replace input events",
			Permission:  PermInputIntercept,
			InputSchema: noParams,
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectInjectsInput},
			Reversible:  true,
		},
		{
			ID: "window.read", Version: "1",
			Description: "Query focused app/window identity and frame",
			Permission:  PermAccessControl,
			InputSchema: noParams,
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectReadsScreen},
			Reversible:  true,
		},
		{
			ID: "window.move", Version: "1",
			Description: "Move/resize the focused window to a frame or snap zone",
			Permission:  PermAccessControl,
			InputSchema: Schema{
				Required:   []string{"zone"},
				Properties: map[string]string{"zone": "string"},
			},
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectMovesWindow},
			Reversible:  true,
		},
		{
			ID: "window.close", Version: "1",
			Description: "Close the focused window (not quit the app)",
			Permission:  PermAccessControl,
			InputSchema: noParams,
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectClosesWindow},
			Reversible:  false,
		},
		{
			ID: "window.minimize", Version: "1",
			Description: "Minimize the focused window",
			Permission:  PermAccessControl,
			InputSchema: noParams,
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectMovesWindow},
			Reversible:  true,
		},
		{
			ID: "window.maximize", Version: "1",
			Description: "Maximize/zoom the focused window",
			Permission:  PermAccessControl,
			InputSchema: noParams,
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectMovesWindow},
			Reversible:  true,
		},
		{
			ID: "clipboard.read", Version: "1",
			Description: "Read clipboard content",
			Permission:  PermAccessControl,
			InputSchema: noParams,
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			Reversible:  true,
		},
		{
			ID: "clipboard.write", Version: "1",
			Description: "Write clipboard content",
			Permission:  PermAccessControl,
			InputSchema: Schema{
				Required:   []string{"text"},
				Properties: map[string]string{"text": "string"},
			},
			Platforms:  []Platform{PlatformMacOS, PlatformWindows},
			Reversible: true,
		},
		{
			ID: "clipboard.copy", Version: "1",
			Description:  "Copy the current selection via the native copy path",
			Permission:   PermAccessControl,
			InputSchema:  noParams,
			Platforms:    []Platform{PlatformMacOS, PlatformWindows},
			Reversible:   true,
		},
		{
			ID: "clipboard.copyPath", Version: "1",
			Description: "Copy selected file paths to the clipboard",
			Permission:  PermAccessControl,
			InputSchema: noParams,
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			Reversible:  true,
		},
		{
			ID: "filesystem.read", Version: "1",
			Description: "Read file content",
			Permission:  PermFilesystem,
			InputSchema: Schema{
				Required:   []string{"path"},
				Properties: map[string]string{"path": "string"},
			},
			Platforms:  []Platform{PlatformMacOS, PlatformWindows},
			Reversible: true,
		},
		{
			ID: "filesystem.write", Version: "1",
			Description: "Write file content",
			Permission:  PermFilesystem,
			InputSchema: Schema{
				Required:   []string{"path", "content"},
				Properties: map[string]string{"path": "string", "content": "string"},
			},
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectModifiesFile},
			Reversible:  false,
		},
		{
			ID: "filesystem.createFile", Version: "1",
			Description: "Create a new file, optionally from a template",
			Permission:  PermFilesystem,
			InputSchema: Schema{
				Required:   []string{"path"},
				Properties: map[string]string{"path": "string", "template": "string"},
			},
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectCreatesFile},
			Reversible:  false,
		},
		{
			ID: "filesystem.createFolder", Version: "1",
			Description: "Create a new folder",
			Permission:  PermFilesystem,
			InputSchema: Schema{
				Required:   []string{"path"},
				Properties: map[string]string{"path": "string"},
			},
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectCreatesFile},
			Reversible:  false,
		},
		{
			ID: "file.moveToTrash", Version: "1",
			Description: "Move files to trash (reversible delete)",
			Permission:  PermFilesystem,
			InputSchema: Schema{
				Required:   []string{"paths"},
				Properties: map[string]string{"paths": "array"},
			},
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectModifiesFile},
			Reversible:  true,
		},
		{
			ID: "app.launch", Version: "1",
			Description: "Launch an application by ID",
			Permission:  PermAccessControl,
			InputSchema: Schema{
				Required:   []string{"app"},
				Properties: map[string]string{"app": "string"},
			},
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectLaunchesApp},
			Reversible:  true,
		},
		{
			ID: "app.open", Version: "1",
			Description: "Open a file/URL with its default handler",
			Permission:  PermAccessControl,
			InputSchema: Schema{
				Required:   []string{"target"},
				Properties: map[string]string{"target": "string"},
			},
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectLaunchesApp},
			Reversible:  true,
		},
		{
			ID: "terminal.openAt", Version: "1",
			Description: "Open a terminal at a directory",
			Permission:  PermShellExecution,
			InputSchema: Schema{
				Required:   []string{"path"},
				Properties: map[string]string{"path": "string"},
			},
			Platforms:   []Platform{PlatformMacOS, PlatformWindows},
			SideEffects: []SideEffect{SideEffectLaunchesApp},
			Reversible:  true,
		},
		{
			ID: "finder.menu", Version: "1",
			Description: "Contribute Finder context-menu items",
			Permission:  PermFinderModify,
			InputSchema: Schema{
				Required:   []string{"items"},
				Properties: map[string]string{"items": "array"},
			},
			Platforms:   []Platform{PlatformMacOS},
			SideEffects: []SideEffect{SideEffectModifiesMenus},
			Reversible:  true,
		},
	}
}

// DefaultRegistry returns the canonical v1 registry. It panics on internal
// inconsistency — the table above is compiled in, so any error is a
// programmer bug, not a runtime condition.
func DefaultRegistry() *Registry {
	r, err := NewRegistry(canonicalV1())
	if err != nil {
		panic("intent: canonical v1 registry invalid: " + err.Error())
	}
	return r
}
