#!/bin/bash
# Trial regression gate: every managed-trial condition (synthetic root, native rehearsal, P8, P9 to P13,
# the deadline measurement) run against a managed-trial binary built from this checkout.
# Usage: trial-regression.sh <htmlclay tree> <hyperclay-local tree, with node_modules> <empty out dir>
# Runs on a test box through scripts/trial-regression-box.sh. Synthetic roots only.
set -uo pipefail
H=$(cd "${1:?htmlclay tree}" && pwd)
L=$(cd "${2:?hyperclay-local tree}" && pwd)
OUT=${3:?out dir}
mkdir -p "$OUT"
[ -z "$(ls -A "$OUT")" ] || { echo "$OUT is not empty"; exit 2; }
OUT=$(cd "$OUT" && pwd)
T=$(mktemp -d /tmp/trial-regression.XXXX)
export HTMLCLAY_SOURCE="$H"
export MANAGED_TRIAL_LOCAL_SOURCE="$L"
( cd "$H" && CGO_ENABLED=0 go build -tags managedtrial -o "$T/managed-trial" ./cmd/managed-trial ) || { echo "BUILD FAIL"; exit 3; }
{
  [ -f "$H/../pins.txt" ] && cat "$H/../pins.txt"
  echo "binary-sha256 $(sha256sum "$T/managed-trial" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$T/managed-trial" | cut -d' ' -f1)"
  echo "started $(date '+%Y-%m-%d %H:%M:%S %Z')"
} > "$OUT/pins.txt"
cd "$L"
fail=0
step() {
  local name=$1; shift
  echo "== $name $(date +%H:%M:%S)" | tee -a "$OUT/summary.log"
  "$@" > "$OUT/$name.log" 2>&1
  local code=$?
  tail -3 "$OUT/$name.log" | tee -a "$OUT/summary.log"
  echo "exit $code" | tee -a "$OUT/summary.log"
  [ $code -eq 0 ] || fail=1
}
step synthetic node trial/synthetic-root.mjs --out "$T/synthetic" --htmlclay "$H"
step native node trial/rehearse.mjs --source "$T/synthetic" --htmlclay "$H" --out "$OUT/native"
step p8 node trial/p8.mjs --source "$T/synthetic" --go-bin "$T/managed-trial" --out "$OUT/p8"
for c in p9 p10 p11 p12 p13; do
  step "$c" node trial/full-host.mjs --condition "$c" --go-bin "$T/managed-trial" --out "$OUT/$c"
done
step deadline node trial/full-host.mjs --measure deadline --go-bin "$T/managed-trial" --out "$OUT/deadline"
echo "finished $(date '+%Y-%m-%d %H:%M:%S %Z')" >> "$OUT/pins.txt"
rm -rf "$T"
if [ $fail -eq 0 ]; then echo DONE >> "$OUT/summary.log"; else echo FAILED >> "$OUT/summary.log"; fi
exit $fail
