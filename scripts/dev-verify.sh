#!/usr/bin/env bash
# Verify the CrossOS tree on a machine that is NOT the macOS build host.
#
# The daemon's product target is macOS (CGEventTap + AX), but most of the tree
# is portable Go. This script typechecks and tests that portable part on
# whatever GOOS you happen to be on, so a Windows/Linux dev box gets the same
# compiler help a mac gets. See docs/dev-verification.md for the findings and
# the per-GOOS file sets this depends on.
#
# **THE `app/` GATES ARE GONE, AND THE SHELL'S GATES ARE SWIFT'S.**
#
# The Wails app was deleted (bead cross-os-app-delete): the window is a
# native AppKit shell under macos/ and the daemon is a Go service, and the
# two speak JSON-RPC over a Unix socket. This script still drove the deleted
# module — `go -C app test ./backend/...` answered "chdir app: no such file
# or directory", and the frontend typecheck looked for app/frontend/node_modules
# — so it FAILED on every current checkout:
#
#     FAIL go test ./backend/... — unrecognised error
#           go: chdir app: no such file or directory
#
# The Swift shell's gates replaced them, and they are the ones that exist.
#
# **THE DAEMON'S GATES GO THROUGH scripts/build-core.sh.**
#
# Not `go build` and not `go test` directly: Go's build cache keys a cgo
# package on the .go files and the compiler flags, and NOT on the C files a
# cgo preamble reaches with `#include` — so a plain `go build` after editing
# the AX or tap bridge links a STALE bridge and the daemon keeps the old
# behaviour. build-core.sh stamps the C sources' content into CGO_CFLAGS,
# which changes the action id and does recompile them.
#
# Usage:
#   ./scripts/dev-verify.sh              run every check
#   ./scripts/dev-verify.sh core         daemon module only
#   ./scripts/dev-verify.sh shell        the Swift shell only
#   ./scripts/dev-verify.sh --help
#
# Exit status: 0 when every check passed except the KNOWN GOOS gaps listed
# below, 1 on any real regression. Fail-closed: an error this script does not
# recognise is a failure, not a warning. The gap lists are currently EMPTY, so
# every check below is a real gate: a command that fails is a FAILURE.
#
# Bead: cross-os-j7p.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

say()  { printf '\033[1m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33mgap:\033[0m %s\n' "$*"; }
die()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# --- KNOWN GOOS GAPS --------------------------------------------------------
# Every compiler-error line a GOOS gap is allowed to produce. Classification
# is per LINE, not per command: a command whose output is entirely covered by
# these patterns is a recorded gap; one extra unrecognised error line makes it
# a FAILURE. A single-needle "did anything match?" test would let a brand new
# error ride along inside a command that already failed for a known reason,
# which is the opposite of fail-closed.
#
# BOTH LISTS ARE EMPTY. core/cmd/crossos used to need two: main.go called
# syscall.Flock with no build constraint, and main_test.go called the
# darwin-only adapter tap seam the same way. Both are fixed — the lock is in
# lock_unix.go / lock_windows.go / lock_other.go, the darwin tests in
# main_darwin_test.go — so those patterns are deleted rather than left to
# absorb errors. An empty list is the strong form of this check: a
# regression in either file is now a FAILURE, not a recorded gap.
#
# When a lane fixes the underlying file, delete its pattern here — the check
# then stops being a gap, and if the fix is wrong the check goes red.
GAP_PATTERNS=()

KNOWN_GAPS=()

# Gates that could not run on this machine (e.g. the Swift shell's suite on a
# box with no Swift toolchain). Tracked so the closing summary can say "these
# did NOT run" instead of a bare OK. The failure mode bead cross-os-64q names
# is exactly "cannot check" being reported as a pass; this array is what stops
# that. The skip stays non-fatal on purpose — this script's whole job is to
# run on whatever machine you have, including one with no Swift toolchain —
# but it is never silent and never counted as a pass.
SKIPPED=()

# An unrecognised compiler diagnostic. Matches go build / go vet / go test
# error lines on both slash styles, so a path-separator difference between
# Windows and Linux cannot disguise a real error as a gap.
ERR_LINE='(\.go:[0-9]+:[0-9]+:|vet\.exe:)'

# run_gate <label> <cmd...>
run_gate() {
  local label="$1"; shift
  local out status
  out="$("$@" 2>&1)"; status=$?
  if [[ $status -eq 0 ]]; then
    printf '  \033[32mPASS\033[0m %s\n' "$label"
    return 0
  fi
  # Strip every line a known gap is allowed to produce, then look for any
  # compiler diagnostic left over. Left over == real regression == fail.
  #
  # ${arr[@]+"${arr[@]}"} is the empty-safe spelling of "${arr[@]}". The gap
  # lists are empty now, and macOS still ships bash 3.2, where expanding an
  # empty array under set -u raises "unbound variable" — which would replace
  # a real error report with a crash. The classifier still has to run on an
  # empty list; it just has nothing to strip.
  local leftover
  leftover="$(while IFS= read -r line; do
                local skip=0 p
                for p in ${GAP_PATTERNS[@]+"${GAP_PATTERNS[@]}"}; do
                  grep -qE "$p" <<<"$line" && { skip=1; break; }
                done
                [[ $skip -eq 0 ]] && printf '%s\n' "$line"
              done <<<"$out" | grep -E "$ERR_LINE" || true)"
  # A failure is a FAILURE unless its output is entirely covered by a
  # recorded gap. With the gap list empty there is nothing a failing command
  # could be excused for, including a failure that printed no compiler
  # diagnostic at all (a module that will not resolve, say) — reporting that
  # as a gap is the fail-open this script is written to refuse.
  if [[ -n "$leftover" || ${#GAP_PATTERNS[@]} -eq 0 ]]; then
    printf '  \033[31mFAIL\033[0m %s — unrecognised error\n' "$label"
    printf '%s\n' "$out" | sed 's/^/        /'
    return 1
  fi
  printf '  \033[33mGAP\033[0m %s (known platform gap only)\n' "$label"
  return 0
}

# skip_gate <label> <reason> <fix>
# A gate that could not run is NOT a pass. This prints a distinct SKIP line
# (never the green PASS word), records the label so the closing summary can
# list it under "skipped, NOT verified", and states the exact command that
# makes the gate runnable. The skip stays non-fatal on purpose: this script
# runs on whatever machine you have, and "cannot check here" is not the same
# as "broken here".
skip_gate() {
  local label="$1" reason="$2" fix="$3"
  printf '  \033[33mSKIP\033[0m %s — gate NOT run: %s\n' "$label" "$reason"
  printf '        to run it: %s\n' "$fix"
  SKIPPED+=("$label — $reason")
}

# The daemon.
#
# **THE BUILD AND THE TESTS GO THROUGH THE REPO'S OWN SCRIPT.**
#
# `go -C core build` and `go -C core test` both link the cgo package, and
# Go's build cache does not know that the C sources behind that package
# changed: the AX bridge and the tap bridge are plain .c files the cgo
# preamble reaches with `#include`, and a file reached that way is invisible
# to the cache. A test run that way can be measuring the bridge from before
# the edit — which is how a "failing" live test turned out to be an old
# binary.
#
# build-core.sh stamps the C sources' content into CGO_CFLAGS, so the action
# id changes whenever a C file changes and the bridge is recompiled.
check_core() {
  say "core module (GOOS=$(go env GOOS))"
  local failures=0
  run_gate "daemon build (cgo-stamped)" scripts/build-core.sh || failures=$((failures+1))
  run_gate "go vet ./..."              go -C core vet ./... || failures=$((failures+1))
  run_gate "daemon test (cgo-stamped)" scripts/build-core.sh --test || failures=$((failures+1))
  return $failures
}

# The plugin modules.
#
# They are separate Go modules — core cannot import them back — and each has
# its own suite, including the parity test that pins the core rule table
# against the plugin matrices. Nothing ran them from this script, and CI ran
# them only after the plugin lane was added there.
check_plugins() {
  say "builtin plugin modules (GOOS=$(go env GOOS))"
  local failures=0
  for m in plugins/developer plugins/launcher plugins/windows-keyboard; do
    run_gate "$m" bash -c "cd '$m' && go vet ./... && go test ./..." || failures=$((failures+1))
  done
  return $failures
}

# The Swift shell.
#
# **THE SUITE IS AN EXECUTABLE, NOT A `.testTarget`.**
#
# Neither XCTest nor swift-testing is importable from the Command Line Tools
# toolchain, so a `.testTarget` is dead on arrival there and `swift test`
# reports a failure that is about the machine rather than the code. The
# assertions live in macos/Tests/Harness/Sources/CoreTests/ and run as a
# program: `swift run CoreTests`.
#
# The suite is also why the harness imports CrossOSCore WITHOUT `@testable`:
# an executable target's dependency is not compiled with -enable-testing, so
# a `@testable` import fails to build on a clean checkout ("module
# 'CrossOSCore' was not compiled for testing"). The geometry the suite
# checks is public API, so the plain import is the same coverage.
check_shell() {
  say "swift shell (macos/)"
  if ! command -v swift >/dev/null 2>&1; then
    skip_gate "swift build + CoreTests" "no swift toolchain on this machine" \
      "install Xcode or the Command Line Tools, then re-run"
    return 0
  fi
  local failures=0
  run_gate "swift build" bash -c "cd macos && swift build" || failures=$((failures+1))
  run_gate "CoreTests"   bash -c "cd macos && swift run CoreTests" || failures=$((failures+1))
  # The geometry probe is the switcher's arithmetic, checked against the
  # numbers in docs/baseline/ — the contract the port is judged by.
  run_gate "switcher geometry probe" bash -c "cd macos && ./.build/debug/CrossOS --geometry" || failures=$((failures+1))
  return $failures
}

rc=0
case "${1:-all}" in
  -h|--help) sed -n '2,35p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  core)   check_core;   rc=$? ;;
  plugins) check_plugins; rc=$? ;;
  shell)  check_shell;  rc=$? ;;
  all)    check_core || rc=1; check_plugins || rc=1; check_shell || rc=1 ;;
  *)      die "unknown target '$1' (try --help)" ;;
esac

if (( rc != 0 )); then
  printf '\n\033[31mFAILED\033[0m — an unrecognised error above is a real regression.\n'
  exit 1
fi

if (( ${#KNOWN_GAPS[@]} )); then
  printf '\n\033[33mRecorded GOOS gaps\033[0m (known, tracked, not regressions):\n'
  for g in "${KNOWN_GAPS[@]}"; do printf '  - %s\n' "$g"; done
  printf 'See docs/dev-verification.md for the exact symbols and the fix shape.\n'
fi
if (( ${#SKIPPED[@]} )); then
  printf '\n\033[33mSkipped, NOT verified\033[0m (these gates did not run here):\n'
  for s in "${SKIPPED[@]}"; do printf '  - %s\n' "$s"; done
  printf 'CI runs these for real — see the "shell" job in .github/workflows/ci.yml.\n'
  printf '\n\033[33mOK\033[0m for every gate that ran; the items above were NOT checked.\n'
else
  printf '\n\033[32mOK\033[0m\n'
fi
