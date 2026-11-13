#!/bin/bash
# Served-product gate: the trial binary's conformance (P1, managed), the real app's own smoke test,
# the shared conformance package against a folder the real app serves, and the trial's differential
# between the trial binary and the real app. Usage: served-product-gate.sh <htmlclay tree>
# <hyperclay-local tree, with node_modules> <empty out dir>. Runs on a test box through
# scripts/served-product-box.sh. Synthetic roots only.
set -uo pipefail
H=$(cd "${1:?htmlclay tree}" && pwd)
L=$(cd "${2:?hyperclay-local tree}" && pwd)
OUT=${3:?out dir}
mkdir -p "$OUT"
[ -z "$(ls -A "$OUT")" ] || { echo "$OUT is not empty"; exit 2; }
OUT=$(cd "$OUT" && pwd)
T=$(mktemp -d /tmp/served-product.XXXX)
# Local v1.37.0's trial scripts read fixtures under the package's old name.
[ -e "$H/internal/managedtrial" ] || ln -s served "$H/internal/managedtrial"
export HTMLCLAY_SOURCE="$H"
export MANAGED_TRIAL_LOCAL_SOURCE="$L"
( cd "$H" && CGO_ENABLED=0 go build -tags managedtrial -o "$T/managed-trial" ./cmd/managed-trial ) || { echo "BUILD FAIL"; exit 3; }
( cd "$H" && CGO_ENABLED=0 go build -o "$T/htmlclay" ./cmd/htmlclay ) || { echo "BUILD FAIL"; exit 3; }
# The smoke test builds its own binary unless this names one; the gate has just built it.
export HTMLCLAY_BIN="$T/htmlclay"
{
  [ -f "$H/../pins.txt" ] && cat "$H/../pins.txt"
  echo "managed-trial-sha256 $(sha256sum "$T/managed-trial" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$T/managed-trial" | cut -d' ' -f1)"
  echo "htmlclay-sha256 $(sha256sum "$T/htmlclay" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$T/htmlclay" | cut -d' ' -f1)"
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
step p1-trial node trial/p1.mjs --target managed --go-bin "$T/managed-trial" --report "$OUT/p1-trial.json" --skip-local-gate
step served-product-smoke node --test "$H/scripts/served-product.test.mjs"
step conformance-served node --import "$H/scripts/localhost-dns.mjs" "$H/scripts/conformance-served.mjs" \
  --bin "$T/htmlclay" --local "$L" --out "$OUT/conformance" --trial-report "$OUT/p1-trial.json"
step differential-served node --import "$H/scripts/localhost-dns.mjs" "$H/scripts/differential-served.mjs" \
  --bin "$T/htmlclay" --trial-bin "$T/managed-trial" --local "$L" --source "$T/synthetic" --out "$OUT/differential"
step watcher-cpu node --import "$H/scripts/localhost-dns.mjs" "$H/scripts/watcher-cpu.mjs" --bin "$T/htmlclay" --seconds 120
echo "finished $(date '+%Y-%m-%d %H:%M:%S %Z')" >> "$OUT/pins.txt"
rm -rf "$T"
if [ $fail -eq 0 ]; then echo DONE >> "$OUT/summary.log"; else echo FAILED >> "$OUT/summary.log"; fi
exit $fail
