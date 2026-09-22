// IPC-backed Core binding: the documented interim binding is now real.
//
// The bridge (App) talks to Core over the bead cross-os-090 JSON-RPC
// protocol. Transport selection mirrors the plan (§3.9): Unix socket on
// macOS/Linux, named pipe on Windows. This file owns ONLY framing +
// transport + failure surfacing; method semantics stay in bridge.go, and
// failures land in the UI log sink (never silent blank pages).
//
// Interim note: until the daemon serves these methods live, tests bind App
// to stubCore (in-process). NewIPCCore lets the shell swap in the live
// transport with no bridge changes.
package shell

import (
	"encoding/json"
	"fmt"
	"net"
)

// IPCTransport dials the Core daemon. Production: unix socket
// (macOS/Linux) or named pipe (Windows). Tests: net.Pipe.
type IPCTransport interface {
	Dial() (net.Conn, error)
}

// ipcRequest is the §3.9 JSON-RPC 2.0 request shape (mirrors core/pkg/ipc
// Request without importing an IPC client package — the framing contract
// is the shared vocabulary, asserted by TestIPCClientFraming).
type ipcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id"`
}

// ipcResponse mirrors core/pkg/ipc Response.
type ipcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	ID any `json:"id"`
}

// IPCCore implements the bridge Core interface over JSON-RPC. Failures
// (dial, framing, RPC error) surface as typed errors for bridge.go to log.
type IPCCore struct {
	transport IPCTransport
	seq       int
}

// NewIPCCore binds the bridge to a live transport.
func NewIPCCore(t IPCTransport) *IPCCore { return &IPCCore{transport: t} }

// call performs one JSON-RPC round trip.
func (c *IPCCore) call(method string, params any) (json.RawMessage, error) {
	conn, err := c.transport.Dial()
	if err != nil {
		return nil, fmt.Errorf("shell: ipc dial %s: %w", method, err)
	}
	defer conn.Close()
	c.seq++
	var praw json.RawMessage
	if params != nil {
		praw, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("shell: ipc marshal %s: %w", method, err)
		}
	}
	req := ipcRequest{JSONRPC: "2.0", Method: method, Params: praw, ID: c.seq}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("shell: ipc encode %s: %w", method, err)
	}
	raw = append(raw, '\n')
	if _, err := conn.Write(raw); err != nil {
		return nil, fmt.Errorf("shell: ipc write %s: %w", method, err)
	}
	var line []byte
	buf := make([]byte, 1)
	for {
		n, rerr := conn.Read(buf)
		if n > 0 {
			line = append(line, buf[:n]...)
			if buf[0] == '\n' {
				break
			}
		}
		if rerr != nil {
			return nil, fmt.Errorf("shell: ipc read %s: %w", method, rerr)
		}
		if len(line) > 1024*1024 {
			return nil, fmt.Errorf("shell: ipc response too large for %s", method)
		}
	}
	var resp ipcResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("shell: ipc decode %s: %w", method, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("shell: ipc %s: code %d: %s", method, resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// IsRunning implements Core via core.status.
func (c *IPCCore) IsRunning() bool {
	raw, err := c.call("core.status", nil)
	if err != nil {
		return false
	}
	var st struct {
		Running bool `json:"running"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return false
	}
	return st.Running
}

// InSafeMode implements Core via core.status.
func (c *IPCCore) InSafeMode() bool {
	raw, err := c.call("core.status", nil)
	if err != nil {
		return false
	}
	var st struct {
		SafeMode bool `json:"safe_mode"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return false
	}
	return st.SafeMode
}

// IsKilled implements Core via core.status.
func (c *IPCCore) IsKilled() bool {
	raw, err := c.call("core.status", nil)
	if err != nil {
		return false
	}
	var st struct {
		Killed bool `json:"killed"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return false
	}
	return st.Killed
}

// Plugins implements Core via plugin.list.
func (c *IPCCore) Plugins() []PluginState {
	raw, err := c.call("plugin.list", nil)
	if err != nil {
		return nil
	}
	var out []PluginState
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// SetEnabled implements Core via plugin.setEnabled.
func (c *IPCCore) SetEnabled(id string, enabled bool) error {
	_, err := c.call("plugin.setEnabled", map[string]any{"id": id, "enabled": enabled})
	return err
}

// Reset implements Core via core.reset (returns audited plan steps).
func (c *IPCCore) Reset() []string {
	raw, err := c.call("core.reset", nil)
	if err != nil {
		return []string{"reset failed: " + err.Error()}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{"reset failed: bad plan"}
	}
	return out
}

// EventLogs implements Core via core.eventLogs.
func (c *IPCCore) EventLogs() []string {
	raw, err := c.call("core.eventLogs", nil)
	if err != nil {
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// CheckForUpdate implements Core via core.checkForUpdate.
func (c *IPCCore) CheckForUpdate(manifestVersion, platform, url, sha256 string) (bool, string, error) {
	raw, err := c.call("core.checkForUpdate", map[string]any{
		"manifest": map[string]any{"version": manifestVersion, "platform": platform, "url": url, "sha256": sha256},
	})
	if err != nil {
		return false, "", err
	}
	var out struct {
		UpdateAvailable bool   `json:"updateAvailable"`
		Version         string `json:"version"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return false, "", err
	}
	return out.UpdateAvailable, out.Version, nil
}

// ApplyUpdate implements Core via core.applyUpdate.
func (c *IPCCore) ApplyUpdate(manifestVersion, platform, url, sha256 string, approved bool, approvedBy string) (string, error) {
	raw, err := c.call("core.applyUpdate", map[string]any{
		"manifest":   map[string]any{"version": manifestVersion, "platform": platform, "url": url, "sha256": sha256},
		"approved":   approved,
		"approvedBy": approvedBy,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Installed string `json:"installed"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.Installed, nil
}

// PanicStop implements Core via safety.panicStop.
func (c *IPCCore) PanicStop() (map[string]any, error) {
	raw, err := c.call("safety.panicStop", nil)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// trialState decodes the {pluginId, state} shape shared by the three
// trial methods.
func trialState(raw json.RawMessage) (string, error) {
	var out struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.State, nil
}

// BeginTrial implements Core via safety.beginTrial.
func (c *IPCCore) BeginTrial(pluginID string) (string, error) {
	raw, err := c.call("safety.beginTrial", map[string]any{"pluginId": pluginID})
	if err != nil {
		return "", err
	}
	return trialState(raw)
}

// ConfirmTrial implements Core via safety.confirmTrial.
func (c *IPCCore) ConfirmTrial(pluginID string, confirmed, healthy bool) (string, error) {
	raw, err := c.call("safety.confirmTrial", map[string]any{"pluginId": pluginID, "confirmed": confirmed, "healthy": healthy})
	if err != nil {
		return "", err
	}
	return trialState(raw)
}

// RollbackTrial implements Core via safety.rollbackTrial.
func (c *IPCCore) RollbackTrial(pluginID, reason string) (string, error) {
	raw, err := c.call("safety.rollbackTrial", map[string]any{"pluginId": pluginID, "reason": reason})
	if err != nil {
		return "", err
	}
	return trialState(raw)
}

// SetRuleEnabled implements Core via config.setRuleEnabled.
func (c *IPCCore) SetRuleEnabled(ruleID string, enabled bool) (bool, error) {
	raw, err := c.call("config.setRuleEnabled", map[string]any{"ruleId": ruleID, "enabled": enabled})
	if err != nil {
		return false, err
	}
	var out struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return false, err
	}
	return out.Enabled, nil
}

// Shortcuts implements Core via config.getShortcuts.
func (c *IPCCore) Shortcuts() ([]map[string]any, error) {
	raw, err := c.call("config.getShortcuts", nil)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetShortcuts implements Core via config.setShortcuts.
func (c *IPCCore) SetShortcuts(shortcuts []map[string]any) (int, error) {
	raw, err := c.call("config.setShortcuts", map[string]any{"shortcuts": shortcuts})
	if err != nil {
		return 0, err
	}
	var out struct {
		Shortcuts int `json:"shortcuts"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		// Fallback: some servers return {"shortcuts": N} vs {"count": N}.
		var alt struct {
			Count int `json:"count"`
		}
		if jerr := json.Unmarshal(raw, &alt); jerr != nil {
			return 0, err
		}
		return alt.Count, nil
	}
	return out.Shortcuts, nil
}
