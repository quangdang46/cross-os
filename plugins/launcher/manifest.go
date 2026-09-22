package launcher

import "crossos/core/pkg/pluginapi"

// Manifest returns the plugin manifest (builtin type — compiled into Core,
// no loading risk per §3.6).
func Manifest() pluginapi.Manifest {
	return pluginapi.Manifest{
		ID:      "launcher",
		Name:    "Launcher",
		Version: "0.1.0",
		Entry:   "builtin",
		Type:    pluginapi.PluginBuiltin,
		// Least-privilege (§3.7): intercept for the hotkey + app control for
		// app.launch. No filesystem, no shell.
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
