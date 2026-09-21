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
run_case() { # name, setup-fn, want(exit code)
  local name="$1" setup="$2" want="$3"
  rm -rf third_party && mkdir -p third_party
  "$setup"
  if bash scripts/ci/license-gate.sh --all >/tmp/gate-out.txt 2>&1; then got=0; else got=1; fi
  if [ "$got" = "$want" ]; then echo "PASS [$name]"; PASS=$((PASS+1)); else echo "FAIL [$name] (want exit $want, got $got)"; cat /tmp/gate-out.txt; FAIL=$((FAIL+1)); fi
  rm -rf third_party
}

mk_base() { # $1=repo — LICENSE + NOTICE only
  mkdir -p "third_party/$1"
  echo "MIT License" > "third_party/$1/LICENSE"
  echo "Copyright (c) test" > "third_party/$1/NOTICE"
}

positive() {
  mk_base good
  cat > third_party/good/ATTRIBUTION.md <<'EOF'
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
  echo "code" > third_party/good/a.swift
}

unattributed() { mk_base bad; echo "code" > third_party/bad/a.swift; }

gpl() {
  mk_base gplrepo
  cat > third_party/gplrepo/ATTRIBUTION.md <<'EOF'
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
  cat > third_party/nonelic/ATTRIBUTION.md <<'EOF'
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
  cat > third_party/floatrepo/ATTRIBUTION.md <<'EOF'
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
  cat > third_party/smug/ATTRIBUTION.md <<'EOF'
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
  cat > third_party/smug/a.c <<'EOF'
/* GNU GENERAL PUBLIC LICENSE Version 3 */
// SPDX-License-Identifier: GPL-3.0-only
int main(void) { return 0; }
EOF
}

spdxonly() {
  # SPDX-only GPL marker (no full phrase) → must FAIL.
  mk_base spdxrepo
  cat > third_party/spdxrepo/ATTRIBUTION.md <<'EOF'
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
  echo '// SPDX-License-Identifier: GPL-3.0-only' > third_party/spdxrepo/a.c
}

partialattr() {
  # 2 source files, 1 attribution entry → must FAIL (per-file completeness).
  mk_base partial
  cat > third_party/partial/ATTRIBUTION.md <<'EOF'
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
  echo "a" > third_party/partial/a.c
  echo "b" > third_party/partial/b.c
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
