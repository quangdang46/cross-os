// Command crossos-shell is the CrossOS UI shell entrypoint (bead cross-os-4gb).
//
// Plan: COMPREHENSIVE_PLAN.md §7 UI Layer. Wails v3 (docs/wails-spike.md
// decision): services are plain structs bound via application.NewService,
// windows are created explicitly, assets embed from frontend/dist.
//
// Layout mirrors the wails3 react template: this main sits at the module
// root so `//go:embed all:frontend/dist` resolves. The shell stays thin:
// Core logic (contribution host, bridge) lives in backend/ with NO Wails
// import; this main only wires Host + Service + transport and opens the
// window. Daemon transport: Unix socket at the conventional path
// (extensions/finder-sync DefaultSocketPath convention); dial failure
// degrades to bridge-logged UI errors, never a blank crash.
package main

import (
	"embed"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	shell "crossos/app/backend"
	"crossos/core/pkg/pluginapi"
)

//go:embed all:frontend/dist
var assets embed.FS

// SocketTransport dials the Core daemon's Unix socket.
type SocketTransport struct{ Path string }

// Dial implements shell.IPCTransport.
func (t SocketTransport) Dial() (net.Conn, error) {
	return net.Dial("unix", t.Path)
}

func socketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/crossos.sock"
	}
	return filepath.Join(home, "Library", "Application Support", "CrossOS", "crossos.sock")
}

func buildService() *shell.Service {
	// Register the Core pages through the SAME Registry path plugins use —
	// the Host discovers them, the shell never hardcodes a page list (§7.2).
	reg := pluginapi.NewRegistry(nil)
	for _, p := range shell.CorePages() {
		if err := reg.RegisterUI(p); err != nil {
			log.Fatalf("shell: register core page %s: %v", p.ID, err)
		}
	}
	host := shell.NewHost()
	if err := host.Register(reg); err != nil {
		log.Fatalf("shell: register core pages: %v", err)
	}

	// Live transport; dial failure → the bridge surfaces it in UI logs
	// (never a silent blank page). Interim: stub until daemon serves (§3.9).
	core := shell.NewIPCCore(SocketTransport{Path: socketPath()})
	return shell.NewService(shell.NewApp(core), host)
}

func main() {
	app := application.New(application.Options{
		Name:        "CrossOS",
		Description: "Cross-platform OS integration layer",
		Services: []application.Service{
			application.NewService(buildService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "CrossOS",
		Width:  1100,
		Height: 720,
		URL:    "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
