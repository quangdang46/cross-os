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
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

RUN_TESTS=0
for arg in "$@"; do
  case "$arg" in
    --test) RUN_TESTS=1 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown option $arg" >&2; exit 1 ;;
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
  STAMP=$(shasum -a 256 $C_SOURCES | shasum -a 256 | cut -c1-16)
fi

# The flag is a preprocessor define, so it reaches the compiler without being
# a source file, and it is harmless if the C code never reads it: its job is
# to change the action id, not to change the program's behaviour.
export CGO_CFLAGS="${CGO_CFLAGS:-} -DCXOS_C_STAMP=${STAMP}"

mkdir -p "$ROOT/.crossos"
go -C core build -o "$ROOT/.crossos/crossos" ./cmd/crossos/

if [[ "$RUN_TESTS" == "1" ]]; then
  go -C core test ./...
fi

echo "daemon built (cgo stamp ${STAMP})"
