#!/usr/bin/env bash
# Build the Go daemon, with the cgo cache hole closed.
#
# Go's build cache keys a cgo package on the CONTENT of its .go files and its
# compiler flags. The AX bridge and the tap bridge are plain .c files that
# the cgo preambles reach with `#include "ax_bridge.c"` — and a file reached
# that way is invisible to the cache. So editing
# platform/darwin/adapter/ax_bridge.c and rebuilding produces the SAME
# binary: the change compiles nowhere, and the daemon keeps the old bridge.
#
# Measured: a SIGBUS fix in cxax_app_titles was written, rebuilt, and tested
# against a binary that did not contain it. The crash then reproduced three
# times with a byte-identical faulting address, which is what "the change was
# never compiled" looks like from the outside.
#
# The fix is to make the C sources' CONTENT part of the compiler flags. The
# action id then changes whenever a C file changes, and the bridge is
# recompiled. Nothing else about the build changes.
#
#   ./scripts/build-core.sh              # build the daemon
#   ./scripts/build-core.sh --test       # build and run the Go tests too
#   ./scripts/build-core.sh --out PATH   # build the daemon to PATH
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

RUN_TESTS=0
OUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --test) RUN_TESTS=1; shift ;;
    --out) OUT="$2"; shift 2 ;;
    -h|--help) sed -n '2,22p' "$0"; exit 0 ;;
    *) echo "unknown option $1" >&2; exit 1 ;;
  esac
done

# The stamp is the content of every C source the cgo preambles include, plus
# the headers they include. `find -newer` would be wrong here — a file whose
# content changed back and forth keeps an old mtime, and a checkout can set
# every mtime to the same second — so this hashes what is there now.
C_SOURCES=$(find platform -type f \( -name '*.c' -o -name '*.h' -o -name '*.m' \) | sort)
if [[ -z "$C_SOURCES" ]]; then
  STAMP=0
else
  # **THE HASH IS TAKEN WITH WHATEVER THE MACHINE HAS.**
  #
  # `shasum` is macOS and Perl; `sha256sum` is Linux and Git for Windows.
  # The Windows lane of CI runs this script under Git Bash, where shasum is
  # not there — and a script that dies on a missing hash tool is a script
  # that does not run on half the machines it is meant to.
  #
  # The stamp's job is to CHANGE when a C source changes, not to agree with
  # itself across platforms, so either tool answers it. Both print
  # "<hash>  <name>", so the second pass hashes the same text either way.
  if command -v shasum >/dev/null 2>&1; then
    HASH="shasum -a 256"
  else
    HASH="sha256sum"
  fi
  STAMP=$($HASH $C_SOURCES | $HASH | cut -c1-16)
fi

# The flag is a preprocessor define, so it reaches the compiler without being
# a source file, and it is harmless if the C code never reads it: its job is
# to change the action id, not to change the program's behaviour.
export CGO_CFLAGS="${CGO_CFLAGS:-} -DCXOS_C_STAMP=${STAMP}"

mkdir -p "$ROOT/.crossos"
# The default path is the one `scripts/run.sh` starts; `--out` is for the
# places that want the binary somewhere else (the CI artifact upload wants
# `core/crossos`, and it wants it built the same way, not by a bare
# `go build` that links a stale bridge).
if [[ -z "$OUT" ]]; then
  OUT="$ROOT/.crossos/crossos"
else
  mkdir -p "$(dirname "$OUT")"
fi
go -C core build -o "$OUT" ./cmd/crossos/

if [[ "$RUN_TESTS" == "1" ]]; then
  go -C core test ./...
fi

echo "daemon built (cgo stamp ${STAMP})"
