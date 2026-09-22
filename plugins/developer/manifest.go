package developer

import "crossos/core/pkg/pluginapi"

// Manifest returns the plugin manifest (builtin type — compiled into Core,
// no loading risk per §3.6).
func Manifest() pluginapi.Manifest {
	return pluginapi.Manifest{
		ID:      "developer",
		Name:    "Developer UX",
		Version: "0.1.0",
		Entry:   "builtin",
		Type:    pluginapi.PluginBuiltin,
		// Least-privilege (§3.7): terminal/editor launching + path copy need
		// app control + filesystem; shell.execute is declared for the Level B
		// terminal-in-dir path (gated, never default).
		Permissions: []string{"accessibility.control", "filesystem", "shell.execute"},
		APIVersion: pluginapi.APIVersion{
			ManifestVersion:      "1",
			PluginAPIVersion:     "1",
			CapabilityAPIVersion: "1",
			MinCoreVersion:       "0.1.0",
		},
		Safety: pluginapi.PluginSafety{
			SafeModeDurationSec: 30,
			AutoRollback:        true,
		},
	}
}
