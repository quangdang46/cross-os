#!/usr/bin/env bash
# Build and run the whole CrossOS app — the first thing a new user runs.
#
# Two halves ship as one product and neither is useful alone: the daemon owns
# the keyboard tap and the IPC socket, the shell is the window. This script
# builds both and starts the daemon only when one is not already serving.
#
# Usage:
#   ./scripts/run.sh            build what is stale, start the daemon, open the shell
#   ./scripts/run.sh --no-open  same, but do not open the window (headless/CI)
#
# Bead: cross-os-jn1.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

say() { printf '\033[1m==>\033[0m %s\n' "$*"; }
die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

OPEN_APP=1
REBUILD=0
for arg in "$@"; do
  case "$arg" in
    --no-open) OPEN_APP=0 ;;
    --rebuild-frontend) REBUILD=1 ;;  # accepted; SwiftPM always rebuilds what is stale
    -h|--help) sed -n '2,14p' "$0"; exit 0 ;;
    *) die "unknown option $arg (try --help)" ;;
  esac
done

BIN="$ROOT/.crossos"
SOCKET="$HOME/Library/Application Support/CrossOS/crossos.sock"
DAEMON="$BIN/crossos"
SHELL_BIN="$ROOT/macos/.build/release/CrossOS"

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not on PATH ($2)"; }

need go "install Go from https://go.dev"
need node "install Node.js 18+"
need npm "ships with Node.js"
# The shell is Swift now. The client speaks the daemon's JSON-RPC directly,
# so there is nothing to generate and nothing to install but a toolchain —
# which is the whole reason the 7,735-line Wails bridge could be deleted
# rather than ported: it was a typed mirror of a wire protocol that already
# existed underneath it.
need swift "install the Xcode command line tools: xcode-select --install"

mkdir -p "$BIN"

# --- daemon -----------------------------------------------------------------
say "Building the daemon"
go -C core build -o "$DAEMON" ./cmd/crossos/

# --- shell ------------------------------------------------------------------
# The shell is a Swift package now, and SwiftPM's own staleness rules are the
# ones that apply: it rebuilds what changed and nothing else, so there is no
# "is the frontend newer than the bundle" question to answer here any more. The
# webview version needed one because the bundle was an embed; this one is a
# binary SwiftPM produces, and the incremental decision belongs to the tool
# that knows how to make it.
#
# --rebuild-frontend keeps its name, because it is in the help text and because
# "frontend" still describes what it rebuilds: the part of the app the daemon
# does not own.
# SwiftPM decides what is stale, so there is no branch here: the flag is
# accepted and the build runs either way. It stays in the interface because it
# is in --help and because "rebuild the shell" is the thing somebody reaches
# for when a change does not show up, and a flag that silently does nothing
# is worse than one that always rebuilds.
[[ "$REBUILD" == "1" ]] && say "Building the shell (--rebuild-frontend)"
swift build --package-path "$ROOT/macos" -c release --product CrossOS

[[ -x "$SHELL_BIN" ]] || die "the shell binary is still missing at $SHELL_BIN — the build output above is where to look"

# --- daemon lifecycle -------------------------------------------------------
# A second daemon must NOT be started: it would unlink the running daemon's
# socket on exit and leave the survivor deaf (audit blocker). Probe first.
daemon_serving() {
  [[ -S "$SOCKET" ]] || return 1
  python3 - "$SOCKET" <<'PY' >/dev/null 2>&1
import json, socket, sys
s = socket.socket(socket.AF_UNIX); s.settimeout(1); s.connect(sys.argv[1])
s.send(b'{"jsonrpc":"2.0","method":"core.status","id":1}\n')
sys.exit(0 if s.recv(4096) else 1)
PY
}

if daemon_serving; then
  say "Daemon already serving on $SOCKET — leaving it alone"
else
  say "Starting the daemon"
  "$DAEMON" serve >"$BIN/daemon.log" 2>&1 &
  DAEMON_PID=$!
  echo "$DAEMON_PID" >"$BIN/daemon.pid"
  # Ctrl-C must not orphan a daemon this script started. A daemon that was
  # already running is left alone — this trap only owns its own child.
  trap 'kill "$DAEMON_PID" 2>/dev/null || true; rm -f "$BIN/daemon.pid"; exit 130' INT TERM
  for _ in $(seq 1 40); do
    daemon_serving && break
    sleep 0.25
  done
  if ! daemon_serving; then
    # A leftover daemon that holds the lock but lost its socket is the
    # common cause; its own message names the pid. Surface it rather than
    # leaving the user with "start it" on one side and "already running"
    # on the other.
    echo
    tail -n 3 "$BIN/daemon.log" >&2 || true
    die "daemon did not come up — see $BIN/daemon.log"
  fi
fi

# --- what the user needs to know --------------------------------------------
status=$(python3 - "$SOCKET" <<'PY'
import json, socket, sys
s = socket.socket(socket.AF_UNIX); s.settimeout(2); s.connect(sys.argv[1])
s.send(b'{"jsonrpc":"2.0","method":"core.status","id":1}\n')
print(s.recv(8192).decode())
PY
)

printf '\n'
say "Socket:    $SOCKET"
if [[ -f "$BIN/daemon.pid" ]] && kill -0 "$(cat "$BIN/daemon.pid")" 2>/dev/null; then
  say "Daemon:    running (pid $(cat "$BIN/daemon.pid"))"
else
  # No verified pid: the socket probe above already proved it answers, so
  # it was started by an earlier run or by launchd.
  say "Daemon:    running (started earlier — no pid from this run)"
fi

# Two different "off" states, and they need different advice:
#   interception:false + no tap_error  → never granted consent
#   tap_error mentioning timing out     → tap installed but degraded
if grep -q '"interception": *false' <<<"$status" && ! grep -q '"tap_error":""' <<<"$status"; then
  cat <<'EOF'

Keyboard remapping is OFF until you grant input monitoring:

  System Settings → Privacy & Security → Input Monitoring
    → add CrossOS (or the terminal that started it) → enable it → restart CrossOS

Everything else works meanwhile: the window, the settings pages, plugin
trials and config all talk to the live daemon.
EOF
elif grep -q 'timing out' <<<"$status"; then
  cat <<'EOF'

WARNING: the keyboard tap keeps timing out and macOS has disabled it, so
remapping is degraded. The window and settings still work. If it does not
recover on its own, restart the daemon — and if it keeps happening, the
daemon log below will say why.
EOF
fi

# Autostart is opt-in and never installed implicitly (§8.4): point at the
# command rather than running it.
cat <<'EOF'

Start at login:  ./.crossos/crossos install-autostart
                 (remove with: ./.crossos/crossos uninstall-autostart)
EOF

if [[ "$OPEN_APP" == "1" ]]; then
  say "Opening the shell"
  open "$SHELL_BIN"
else
  say "Not opening the window (--no-open); launch it with: $SHELL_BIN"
fi

printf '\n'
say "Logs: $BIN/daemon.log"
say "Stop: kill \$(cat $BIN/daemon.pid 2>/dev/null)   # or Ctrl-C in that terminal"
