#!/usr/bin/env bash
# License/attribution merge gate (§9.11). Fail-closed: any reused-OSS file
# without pinned commit + verified license + ATTRIBUTION entry fails the job.
#
# Bead: cross-os-qhp.5.
#
# Unit of enforcement is PER-FILE (§9.11: one 9-field block per reused
# file), not per-repo. A repo with 20 files and 1 entry FAILS.
# Scope note (review: cross-os-c0): the gate checks COMPLETENESS-OF-COUNT
# (N entries >= N files); CORRECTNESS-OF-MAPPING (entry X names file Y)
# stays a human-reviewer duty.
#
# What it checks (on the PR diff vs origin/main, or on all files with --all):
#   1. Files under third_party/<repo>/ carry LICENSE + NOTICE + ATTRIBUTION.md
#      with one full 9-field block per reused source file.
#   2. Entry count >= reused source-file count (no partial attribution).
#   3. Every block pins its own 40-hex Source commit (per-file, not per-repo).
#   4. Declared AND vendored licenses are never GPL/copyleft/no-license —
#      the vendored files themselves are scanned, so a GPL header with a
#      clean declaration still fails.
#   5. All 9 mandatory fields present in EVERY BLOCK (cross-os-baz). A
#      per-FILE check is a check one well-formed block satisfies for the whole
#      directory, which is how third_party/rectangle came to carry three blocks
#      with only two complete and the gate reporting PASS.
#
# What this gate still CANNOT see, so nobody mistakes the above for coverage:
#   - Whether an entry names the file it claims to. Nothing here reads a path
#     out of a Source file: line and compares it to anything on disk. A block
#     can name a file that does not exist and pass.
#   - Whether the pinned commit EXISTS in the named repository, or whether that
#     repository is the one the source actually came from. The pin is checked
#     for SHAPE (40 hex) and nothing else. Verifying it is step one of the
#     §9.11 merge gate and it is still a reviewer's job.
#   - Whether the named file's licence matches the declared one.
#   - Anything under a path other than third_party/. A learn-only record lives
#     at docs/references/alt-tab-macos/ precisely because the gate's model is
#     "this directory holds something we copied" and a decision not to copy is
#     not that — see that directory's ATTRIBUTION.md.
# Those are the same three gaps the count check always had. The per-block
# assertion closes a fourth that was found by auditing the real tree, not by a
# test: a block missing a field while a sibling block supplies it.
#
# Pure-Core PRs (no third_party changes) pass trivially with a note.
set -euo pipefail

MODE="${1:---diff}"
BASE="${2:-origin/main}"
# GATE_ROOT lets the self-test redirect at a scratch tree (default: repo
# third_party). The scratch tree mirrors the real layout: $GATE_ROOT/
# third_party/<repo>/ — so ROOT is always $GATE_ROOT/third_party.
# Never point it at anything but a disposable dir.
ROOT="${GATE_ROOT:-.}/third_party"
ROOT="${ROOT#./}"
FAIL=0

if [ "$MODE" = "--all" ]; then
  # NOTE: ls-files pathspec quirks vary by git version — list tracked
  # files plus untracked fixture files (self-test) via --others.
  FILES=$( (git ls-files "$ROOT" ; git ls-files --others --exclude-standard "$ROOT") 2>/dev/null | sort -u || true)
  # Scratch trees aren't git-tracked: fall back to filesystem listing so
  # fixtures are found even with no git index entries.
  if [ -z "$FILES" ] && [ -d "$ROOT" ]; then
    FILES=$(find "$ROOT" -type f | sort || true)
  fi
else
  if ! git fetch origin main --quiet 2>/dev/null; then
    echo "license-gate WARN: cannot fetch $BASE — diffing against possibly-stale base"
  else
    echo "license-gate: base $(git rev-parse --short "$BASE" 2>/dev/null || echo "$BASE")"
  fi
  # Portable prefix form (no globstar dependency): everything under third_party/.
  FILES=$(git diff --name-only --diff-filter=ACMR "$BASE"...HEAD -- third_party/ || true)
fi

if [ -z "$FILES" ]; then
  echo "license-gate: no third_party changes — pass (pure-Core PR)"
  exit 0
fi

# Banned license signals (fail closed). NOTE: match full phrases ("GENERAL
# PUBLIC LICENSE", "AFFERO", "LESSER"), not bare "GPL" — real GPL headers
# read "GNU GENERAL PUBLIC LICENSE" with no standalone "GPL" substring,
# and bare-GPL matching misses them while over-matching words like
# "gpl-anything" in unrelated text. (debugged via smuggledgpl fixture)
# Declared-license signals: SPDX IDs + names (what a declaration line holds).
DECL_BANNED='GPL|LGPL|AGPL|COPYLEFT|PROPRIETARY|ALL RIGHTS|NO[- ]LICENSE|UNLICENSED'
# Vendored-file signals: full phrases + SPDX IDs (headers rarely contain a
# bare "GPL" substring — "GNU GENERAL PUBLIC LICENSE" doesn't).
FILE_BANNED='GENERAL PUBLIC LICENSE|SPDX-License-Identifier:.*GPL|AFFERO|LESSER GENERAL|COPYLEFT|PROPRIETARY|ALL RIGHTS|NO[- ]LICENSE|UNLICENSED'

# 9 mandatory fields (§9.11 lines 1235-1243).
FIELDS="Source repository:|Source commit:|Source file:|Original license:|Original copyright:|CrossOS destination:|Modification:|Reason for modification:|CrossOS license:"

check_repo() {
  local repo="$1"
  local dir="$ROOT/$repo"
  local ok=1
  for f in LICENSE NOTICE ATTRIBUTION.md; do
    if [ ! -f "$dir/$f" ]; then
      echo "license-gate FAIL [$repo]: missing $dir/$f (§9.11: LICENSE + NOTICE + ATTRIBUTION.md required)"
      ok=0
    fi
  done
  [ "$ok" = "0" ] && return 1
  # (a) 9 fields, not 8: assert the count, so the header and the loop can
  # never drift apart again. (review: cross-os-c0)
  local nfields
  nfields=$(echo "$FIELDS" | tr '|' '\n' | grep -c .)
  if [ "$nfields" != "9" ]; then
    echo "license-gate FAIL [$repo]: gate bug — field list has $nfields entries, want 9"
    return 1
  fi
  # (b) Per-file completeness: one full 9-field block PER reused source
  # file. Count 'Source file:' entries vs vendored source files (non-meta
  # files under the repo dir, excluding LICENSE/NOTICE/ATTRIBUTION.md).
  local nentries nfiles
  nentries=$(grep -c '^Source file:' "$dir/ATTRIBUTION.md" || true)
  nfiles=$(git ls-files "$dir" 2>/dev/null | grep -vE '/(LICENSE|NOTICE|ATTRIBUTION\.md)$' | wc -l)
  # Untracked fixture files (self-test --all) aren't in ls-files: count
  # them from the filesystem too.
  local fsfiles
  fsfiles=$(find "$dir" -type f -not -name LICENSE -not -name NOTICE -not -name ATTRIBUTION.md 2>/dev/null | wc -l)
  if [ "$fsfiles" -gt "$nfiles" ]; then nfiles="$fsfiles"; fi
  if [ "$nentries" -lt 1 ]; then
    echo "license-gate FAIL [$repo]: ATTRIBUTION.md has no 'Source file:' entries"
    return 1
  fi
  if [ "$nentries" -lt "$nfiles" ]; then
    echo "license-gate FAIL [$repo]: $nentries attribution entries cover $nfiles source files (per-file attribution required §9.11)"
    return 1
  fi
  # (c) Per-file pinned commits: EVERY 'Source commit:' line must be 40-hex.
  local npins ncommits
  ncommits=$(grep -c '^Source commit:' "$dir/ATTRIBUTION.md" || true)
  npins=$(grep -Ec '^Source commit: [0-9a-f]{40}([^0-9a-f]|$)' "$dir/ATTRIBUTION.md" || true)
  if [ "$ncommits" -lt "$nentries" ]; then
    echo "license-gate FAIL [$repo]: $ncommits commit lines for $nentries file entries (per-file pin required)"
    return 1
  fi
  if [ "$npins" -lt "$ncommits" ]; then
    echo "license-gate FAIL [$repo]: unpinned commit (want 40-hex SHA, not branch/tag)"
    return 1
  fi
  # Banned licenses in declarations (SPDX-ID matching: declarations hold
  # short IDs like "GPL-3.0", never full phrases).
  if grep -Ei "Original license:.*($DECL_BANNED)" "$dir/ATTRIBUTION.md" >/dev/null; then
    echo "license-gate FAIL [$repo]: banned license signal in ATTRIBUTION.md (GPL/copyleft/no-license can never pass §9.11)"
    grep -Ei "Original license:.*($DECL_BANNED)" "$dir/ATTRIBUTION.md" | head -5
    return 1
  fi
  # (d) Banned signals in the VENDORED FILES themselves: a clean
  # declaration over a GPL header still fails. A real MIT file contains no
  # GPL signal; a false positive just forces a human look (fail-closed).
  # NOTE: `grep -r PAT dir` (no trailing slash, no --include) — the
  # trailing-slash and --include='*' forms silently miss files on some
  # grep builds; this form is verified by the smuggledgpl fixture.
  local hit
  hit=$(find "$dir" -type f -not -name LICENSE -not -name NOTICE -not -name ATTRIBUTION.md -exec grep -lEi "($FILE_BANNED)" {} + 2>/dev/null | head -5 || true)
  if [ -n "$hit" ]; then
    echo "license-gate FAIL [$repo]: banned license signal inside vendored files (declaration says clean):"
    echo "$hit"
    return 1
  fi
  # (a) All 9 mandatory fields present — PER BLOCK, not per file.
  #
  # A per-FILE check is a check that one well-formed block satisfies for the
  # whole directory, which is how third_party/rectangle came to carry three
  # blocks and two of them complete: block 1 named its file and its reason only
  # inside another field's prose, and the gate passed. Nine fields per REUSED
  # FILE is what §9.11 says, and a file is a block.
  #
  # The split is on ^Source repository:, which is what starts a block. Prose
  # before the first one (the header) is not a block and is not counted, which
  # is why the awk below reports "no Source repository block" rather than
  # silently passing a file with none.
  local blockreport
  blockreport=$(awk '
    BEGIN {
      n = 0; f = 0; bad = 0; repo = ""; commit = ""
      split("Source repository|Source commit|Source file|Original license|Original copyright|CrossOS destination|Modification|Reason for modification|CrossOS license", a, "|")
    }
    function close_block(   msg) {
      if (n == 0) return
      if (f < 9) {
        printf "block %d (%s @ %s) carries %d of 9 mandatory fields\n", n, repo, (commit == "" ? "no commit" : commit), f
        bad++
      }
    }
    /^Source repository:/ {
      close_block()
      n++; f = 0; repo = $0; sub(/^Source repository:[[:space:]]*/, "", repo); commit = ""
      f = 1
      next
    }
    n > 0 {
      if ($0 ~ /^Source commit:/) { commit = $0; sub(/^Source commit:[[:space:]]*/, "", commit) }
      for (i = 1; i <= 9; i++) if (index($0, a[i] ":") == 1) f++
    }
    END {
      close_block()
      if (n == 0) { print "no Source repository block found in ATTRIBUTION.md"; bad++ }
      exit (bad > 0) ? 1 : 0
    }
  ' "$dir/ATTRIBUTION.md" 2>/dev/null || true)
  if [ -n "$blockreport" ]; then
    echo "license-gate FAIL [$repo]: ATTRIBUTION.md is not nine-fields-per-block:"
    echo "$blockreport"
    return 1
  fi
  echo "license-gate PASS [$repo]"
  return 0
}

# (e) Guard stray paths: entries directly under the root (no repo dir)
# fail with a clear message, not a misleading missing-LICENSE path.
# NOTE: when ROOT itself is nested (self-test scratch .tmp-gate-test/
# third_party), FILES entries carry the full scratch prefix — strip it
# first, then split the REMAINDER into repo/file.
REPOS=""
while IFS= read -r f; do
  rel="${f#$ROOT/}"
  # Defensive: an entry equal to ROOT or outside it yields no slash.
  case "$rel" in
    */*) repo=$(echo "$rel" | cut -d/ -f1); REPOS="$REPOS $repo";;
    *) echo "license-gate FAIL: '$f' is directly under $ROOT/ (must live in $ROOT/<repo>/ — §9.11)"; FAIL=1;;
  esac
done <<< "$FILES"
REPOS=$(echo "$REPOS" | tr ' ' '\n' | sort -u)
for repo in $REPOS; do
  [ -z "$repo" ] && continue
  check_repo "$repo" || FAIL=1
done

if [ "$FAIL" = "1" ]; then
  echo "license-gate: FAIL — see §9.11 gate steps above"
  exit 1
fi
echo "license-gate: all third_party repos pass"
