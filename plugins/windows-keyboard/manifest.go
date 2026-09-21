package windowskeyboard

import "crossos/core/pkg/pluginapi"

// Manifest returns the plugin manifest (builtin type — compiled into Core,
// no loading risk per §3.6).
func Manifest() pluginapi.Manifest {
	return pluginapi.Manifest{
		ID:      "windows-keyboard",
		Name:    "Windows Keyboard",
		Version: "0.1.0",
		Entry:   "builtin",
		Type:    pluginapi.PluginBuiltin,
		// Least-privilege (§3.7): this plugin intercepts input and moves/
		// closes windows for CLOSE_WINDOW — nothing else.
		Permissions: []string{"input.intercept", "accessibility.control"},
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
