#!/usr/bin/env bash
# Verify the CrossOS tree on a machine that is NOT the macOS build host.
#
# The daemon's product target is macOS (CGEventTap + AX), but most of the tree
# is portable Go. This script typechecks and tests that portable part on
# whatever GOOS you happen to be on, so a Windows/Linux dev box gets the same
# compiler help a mac gets. See docs/dev-verification.md for the findings and
# the per-GOOS file sets this depends on.
#
# It is NOT scripts/run.sh: that one builds the real product and needs wails3
# plus a macOS host. This one needs only go + node + npm.
#
# Usage:
#   ./scripts/dev-verify.sh              run every check
#   ./scripts/dev-verify.sh core         core module only
#   ./scripts/dev-verify.sh app          app (shell) module only
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

# Gates that could not run on this machine (e.g. the frontend typecheck on a
# box with no node_modules or no generated Wails bindings). Tracked so the
# closing summary can say "these did NOT run" instead of a bare OK. The
# failure mode bead cross-os-64q names is exactly "cannot check" being reported
# as a pass; this array is what stops that. The skip stays non-fatal on
# purpose — this script's whole job is to run on whatever machine you have,
# including one without the Wails toolchain — but it is never silent and never
# counted as a pass.
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
# makes the gate runnable. Scoped to interactive dev convenience: the same
# gate is enforced for real in CI (the "frontend" job in
# .github/workflows/ci.yml), so a local skip never hides a broken typecheck.
skip_gate() {
  local label="$1" reason="$2" fix="$3"
  printf '  \033[33mSKIP\033[0m %s — gate NOT run: %s\n' "$label" "$reason"
  printf '        to run it: %s\n' "$fix"
  SKIPPED+=("$label — $reason")
}

check_core() {
  say "core module (GOOS=$(go env GOOS))"
  local failures=0
  run_gate "go build ./..." go -C core build ./... || failures=$((failures+1))
  run_gate "go vet ./..."   go -C core vet   ./... || failures=$((failures+1))
  run_gate "go test ./..."  go -C core test  ./... || failures=$((failures+1))
  return $failures
}

# The shell embeds //go:embed all:frontend/dist, so `go build ./...` at the
# app/ module root cannot even typecheck main.go until dist/ holds at least
# one file. Two ways to satisfy that, in order of preference:
#
#   1. wails3 on PATH  -> generate the bindings and build the REAL bundle.
#      The @wailsio/runtime vite plugin hard-requires
#      bindings/github.com/wailsapp/wails/v3/internal/eventcreate, which only
#      `wails3 generate bindings ./...` writes. No other input produces it.
#   2. no wails3       -> write a placeholder index.html so the Go toolchain
#      can typecheck main.go and backend/. The placeholder states in its own
#      body that no bundle exists; it is NOT a build product and must never be
#      shipped. dist/ is gitignored, so it cannot reach a commit.
ensure_dist() {
  if [[ -f app/frontend/dist/index.html ]]; then return 0; fi
  if command -v wails3 >/dev/null 2>&1 && [[ -d app/frontend/node_modules ]]; then
    say "wails3 found — building the real frontend bundle"
    # The generator takes the MODULE root, and the module is app/ (its go.mod
    # lives there and main.go is the only Wails entrypoint). Pointing it at the
    # repo root finds no go.mod, writes nothing, and the vite build then fails
    # on the missing event bindings with the reason swallowed by 2>/dev/null.
    (cd app && wails3 generate bindings ./) || return 1
    (cd app/frontend && npm run build) || return 1
    return 0
  fi
  warn "no wails3 on PATH: the real frontend bundle cannot be built here."
  warn "  @wailsio/runtime's vite plugin requires generated event bindings, and"
  warn "  only 'wails3 generate bindings ./...' writes them. Writing a"
  warn "  PLACEHOLDER dist/index.html so the Go build can typecheck main.go."
  mkdir -p app/frontend/dist
  cat > app/frontend/dist/index.html <<'HTML'
<!doctype html>
<meta charset="utf-8">
<title>CrossOS — bundle not built</title>
<body style="font:14px system-ui;margin:3rem;max-width:40rem">
<h1>Frontend bundle not built</h1>
<p>This directory is a <strong>placeholder</strong>, written by
<code>scripts/dev-verify.sh</code> so <code>go build</code> can typecheck the
shell. It contains no application code and the window will not work.</p>
<p>To build the real bundle, install the Wails v3 CLI and run
<code>scripts/run.sh</code> (or <code>wails3 dev</code>) from a checkout.</p>
</body>
HTML
}

check_app() {
  say "app module (GOOS=$(go env GOOS))"
  local failures=0
  # backend/ carries no embed directive, so this is the app-side check that
  # works from a clean checkout with nothing built.
  run_gate "go test ./backend/..." go -C app test ./backend/... || failures=$((failures+1))
  # check_frontend returns 0 when it SKIPS (inputs absent — reported in the
  # summary) and >0 only when the typecheck actually ran and FAILED. Calling
  # it as a bare statement used to discard that non-zero, so a genuinely
  # broken typecheck printed FAIL but still exited 0 with a green OK. Fold its
  # result into the gate count so the script is fail-closed as its header
  # promises (bead cross-os-64q).
  check_frontend || failures=$((failures+1))
  ensure_dist
  run_gate "go build ./..." go -C app build ./... || failures=$((failures+1))
  return $failures
}

# The frontend typecheck is a real gate, not a courtesy. The Wails models are
# generated JavaScript, and tsconfig sets allowJs precisely so the ServiceApi
# annotation in src/lib/service.ts checks the generated Service instead of an
# `any` — a renamed model property is otherwise a silent `undefined` in a
# settings row with a green typecheck.
#
# It is skipped only when an input it needs (node_modules, the generated
# bindings) is absent, because this script also runs on boxes that have no
# Wails toolchain. But the skip is EXPLICIT and is recorded in SKIPPED so the
# closing summary prints "skipped, NOT verified" instead of a bare OK: an
# unchecked typecheck must never read as a checked one (bead cross-os-64q).
# When the inputs are present the typecheck is a real gate and a failure is a
# FAILURE. The same typecheck is enforced in CI regardless.
check_frontend() {
  say "app/frontend"
  local failures=0
  if [[ ! -d app/frontend/node_modules ]]; then
    skip_gate "npx tsc --noEmit" "app/frontend/node_modules is absent" \
      "cd app/frontend && npm ci"
    return 0
  fi
  if [[ ! -f app/frontend/bindings/crossos/app/backend/service.js ]]; then
    skip_gate "npx tsc --noEmit" "Wails bindings are not generated" \
      "cd app && wails3 generate bindings ./"
    return 0
  fi
  if (cd app/frontend && npx tsc --noEmit); then
    printf '  \033[32mPASS\033[0m npx tsc --noEmit\n'
  else
    printf '  \033[31mFAIL\033[0m npx tsc --noEmit\n'
    failures=$((failures+1))
  fi
  return $failures
}

rc=0
case "${1:-all}" in
  -h|--help) sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  core) check_core; rc=$? ;;
  app)  check_app;  rc=$? ;;
  all)  check_core || rc=1; check_app || rc=1 ;;
  *)    die "unknown target '$1' (try --help)" ;;
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
  printf 'CI runs this for real — see the "frontend" job in .github/workflows/ci.yml.\n'
  printf '\n\033[33mOK\033[0m for every gate that ran; the items above were NOT checked.\n'
else
  printf '\n\033[32mOK\033[0m\n'
fi
