// Command crossos is the CrossOS core daemon entrypoint.
//
// Bead: cross-os-ymh.2 (scaffold half). Lifecycle mechanism lives in
// package daemon; TRIAL/rollback semantics live in package safety.
// TODO(daemon): block on signals, wire IPC server (cross-os-090) + plugin
// loading here — currently starts and exits (scaffold, not a daemon yet).
package main

import (
	"fmt"
	"os"

	"crossos/core/pkg/daemon"
)

func main() {
	d := daemon.New()
	if err := d.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "crossos: run:", err)
		os.Exit(1)
	}
	fmt.Println("crossos: running")
}
