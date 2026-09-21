#!/usr/bin/env bash
# Self-test for the license gate (bead cross-os-qhp.5 pass criteria as
# executable fixtures, run locally and in CI before the gate itself).
#
# Fixtures:
#   1. positive: MIT file + pinned SHA + LICENSE + NOTICE + full ATTRIBUTION → PASS
#   2. unattributed: file without ATTRIBUTION.md → FAIL (missing file step)
#   3. GPL: ATTRIBUTION declares GPL-3.0 → FAIL (banned license step)
#   4. no-license: ATTRIBUTION declares none → FAIL (banned license step)
#   5. unpinned: branch name instead of SHA → FAIL (pin step)
set -euo pipefail
cd "$(dirname "$0")/../.."

PASS=0; FAIL=0
# SAFETY (incident fix, bead cross-os-nir.1 review): run_cases execute in a
# SCRATCH dir (./.tmp-gate-test/third_party), NEVER in the real
# ./third_party. The old form did `rm -rf third_party` around each case and
# wiped the real vendored attribution once. Refuse to run if invoked
# anywhere the scratch path escapes the repo.
run_case() { # name, setup-fn, want(exit code)
  local name="$1" setup="$2" want="$3"
  local scratch=".tmp-gate-test/third_party"
  # Marker-file guard (review: cross-os-c0): dirname matching is
  # rename-fragile, so require the repo marker instead.
  if [ ! -f "COMPREHENSIVE_PLAN.md" ]; then
    echo "refusing: run from repo root (COMPREHENSIVE_PLAN.md not found)"
    return 1
  fi
  rm -rf .tmp-gate-test && mkdir -p "$scratch"
  # Redirect the fixture helpers at the scratch dir via env.
  GATE_SCRATCH="$scratch" "$setup"
  if GATE_ROOT=".tmp-gate-test" bash scripts/ci/license-gate.sh --all >/tmp/gate-out.txt 2>&1; then got=0; else got=1; fi
  if [ "$got" = "$want" ]; then echo "PASS [$name]"; PASS=$((PASS+1)); else echo "FAIL [$name] (want exit $want, got $got)"; cat /tmp/gate-out.txt; FAIL=$((FAIL+1)); fi
  rm -rf .tmp-gate-test
}

mk_base() { # $1=repo — LICENSE + NOTICE only
  mkdir -p "$GATE_SCRATCH/$1"
  echo "MIT License" > "$GATE_SCRATCH/$1/LICENSE"
  echo "Copyright (c) test" > "$GATE_SCRATCH/$1/NOTICE"
}

positive() {
  mk_base good
  cat > $GATE_SCRATCH/good/ATTRIBUTION.md <<'EOF'
Source repository: example/good
Source commit: 0123456789abcdef0123456789abcdef01234567
Source file: src/a.swift
Original license: MIT
Original copyright: Copyright (c) test
CrossOS destination: platform/darwin/
Modification: adapted to Capability API
Reason for modification: strip app UI
CrossOS license: MIT
EOF
  echo "code" > $GATE_SCRATCH/good/a.swift
}

unattributed() { mk_base bad; echo "code" > $GATE_SCRATCH/bad/a.swift; }

gpl() {
  mk_base gplrepo
  cat > $GATE_SCRATCH/gplrepo/ATTRIBUTION.md <<'EOF'
Source repository: example/gpl
Source commit: 0123456789abcdef0123456789abcdef01234567
Source file: src/a.c
Original license: GPL-3.0
Original copyright: Copyright (c) test
CrossOS destination: platform/
Modification: none
Reason for modification: n/a
CrossOS license: MIT
EOF
}

nolicense() {
  mk_base nonelic
  cat > $GATE_SCRATCH/nonelic/ATTRIBUTION.md <<'EOF'
Source repository: example/none
Source commit: 0123456789abcdef0123456789abcdef01234567
Source file: src/a.c
Original license: none (no license file in source repo)
Original copyright: unknown
CrossOS destination: platform/
Modification: none
Reason for modification: n/a
CrossOS license: MIT
EOF
}

unpinned() {
  mk_base floatrepo
  cat > $GATE_SCRATCH/floatrepo/ATTRIBUTION.md <<'EOF'
Source repository: example/float
Source commit: main
Source file: src/a.c
Original license: MIT
Original copyright: Copyright (c) test
CrossOS destination: platform/
Modification: none
Reason for modification: n/a
CrossOS license: MIT
EOF
}

smuggledgpl() {
  # Clean declaration over a GPL-header file → must FAIL (vendored scan).
  # Two header shapes: full-phrase ("GNU GENERAL PUBLIC LICENSE", no bare
  # GPL substring) and SPDX ("GPL-3.0-only") — the gate must catch BOTH.
  mk_base smug
  cat > $GATE_SCRATCH/smug/ATTRIBUTION.md <<'EOF'
Source repository: example/smug
Source commit: 0123456789abcdef0123456789abcdef01234567
Source file: src/a.c
Original license: MIT
Original copyright: Copyright (c) test
CrossOS destination: platform/
Modification: none
Reason for modification: n/a
CrossOS license: MIT
EOF
  cat > $GATE_SCRATCH/smug/a.c <<'EOF'
/* GNU GENERAL PUBLIC LICENSE Version 3 */
// SPDX-License-Identifier: GPL-3.0-only
int main(void) { return 0; }
EOF
}

spdxonly() {
  # SPDX-only GPL marker (no full phrase) → must FAIL.
  mk_base spdxrepo
  cat > $GATE_SCRATCH/spdxrepo/ATTRIBUTION.md <<'EOF'
Source repository: example/spdx
Source commit: 0123456789abcdef0123456789abcdef01234567
Source file: src/a.c
Original license: MIT
Original copyright: Copyright (c) test
CrossOS destination: platform/
Modification: none
Reason for modification: n/a
CrossOS license: MIT
EOF
  echo '// SPDX-License-Identifier: GPL-3.0-only' > $GATE_SCRATCH/spdxrepo/a.c
}

partialattr() {
  # 2 source files, 1 attribution entry → must FAIL (per-file completeness).
  mk_base partial
  cat > $GATE_SCRATCH/partial/ATTRIBUTION.md <<'EOF'
Source repository: example/partial
Source commit: 0123456789abcdef0123456789abcdef01234567
Source file: src/a.c
Original license: MIT
Original copyright: Copyright (c) test
CrossOS destination: platform/
Modification: none
Reason for modification: n/a
CrossOS license: MIT
EOF
  echo "a" > $GATE_SCRATCH/partial/a.c
  echo "b" > $GATE_SCRATCH/partial/b.c
}

run_case "positive-mit-passes" positive 0
run_case "unattributed-fails" unattributed 1
run_case "gpl-fails" gpl 1
run_case "no-license-fails" nolicense 1
run_case "unpinned-fails" unpinned 1
run_case "smuggled-gpl-header-fails" smuggledgpl 1
run_case "spdx-only-fails" spdxonly 1
run_case "partial-attribution-fails" partialattr 1

echo "---"
echo "gate self-test: $PASS passed, $FAIL failed"
[ "$FAIL" = "0" ]
