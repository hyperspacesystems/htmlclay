#!/bin/bash
# 08-sync.md step 8.26: the live-room service wrote only its own files while the
# real pair synced. Builds the real htmlclay with the tag synctest and the packaged
# Linux service, proves both write detectors can see a write into a root (the
# canary), then runs scripts/sync-pair.mjs twice: the service from source under
# service/test/guard-product.cjs, and the packaged service under strace -f. Run on
# a box from the repo root: testbox run --command 'scripts/sync-pair-box.sh'.
# Passing output ends with: PAIR OK forbidden=0 strace=0 canary=seen
set -uo pipefail
export PATH=$HOME/.local/bin:$PATH CGO_ENABLED=0 TZ=America/New_York
umask 022
H=$(pwd)
LOCAL=${HTMLCLAY_SYNC_LOCAL:-$H/service/test/fake}
O=${TESTBOX_OUT:-$HOME/pair-out}/sync-pair
rm -rf "$O" && mkdir -p "$O/guard" "$O/strace" "$O/canary/guard" "$O/canary/strace"
W=$HOME/pair-work
rm -rf "$W" && mkdir -p "$W/tmp"
ROOTS=$HOME/pair-roots
GUARD=$H/service/test/guard-product.cjs

( go build -tags synctest -o "$W/htmlclay" ./cmd/htmlclay && node service/build.mjs --goos linux --goarch amd64 ) > "$O/build.log" 2>&1 \
  || { echo "BUILD FAIL"; tail -20 "$O/build.log"; exit 3; }
mkdir -p "$W/pkg" && tar -xzf internal/service/blobs/linux-amd64.tar.gz -C "$W/pkg"
[ -x "$W/pkg/node" ] && [ -f "$W/pkg/app/main.cjs" ] || { echo "the packaged service has no node or app/main.cjs"; ls "$W/pkg"; exit 3; }

# Go runs "<wrapper> --data-dir <config>/htmlclay/service". The data dir and TMPDIR
# are the service's own; everything under $ROOTS is a root.
cat > "$W/svc-guard.sh" <<W
#!/bin/bash
data=\${2:-}
exec env TMPDIR=$W/tmp PRODUCT_GUARD_OUT=\${GUARD_DIR:-$O/guard}/g-\$\$.json PRODUCT_GUARD_OWN="[\"\$data\",\"$W/tmp\"]" PRODUCT_GUARD_ROOTS='["$ROOTS"]' \\
  node --require $GUARD \${SERVICE_MAIN:-$H/service/main.cjs} "\$@"
W
cat > "$W/svc-strace.sh" <<W
#!/bin/bash
exec env TMPDIR=$W/tmp strace -f -qq -e trace=%file -e signal=none -s 512 -o \${STRACE_DIR:-$O/strace}/s-\$\$.txt $W/pkg/node \${SERVICE_MAIN:-$W/pkg/app/main.cjs} "\$@"
W
chmod +x "$W/svc-guard.sh" "$W/svc-strace.sh"

# The canary: a program that writes one file into a root, under each wrapper,
# must be seen by both methods, or a clean result proves nothing.
mkdir -p "$ROOTS/canary"
echo "require('fs').writeFileSync(process.env.CANARY_TARGET, '<p>canary</p>');" > "$W/canary.cjs"
for mode in guard strace; do
  GUARD_DIR=$O/canary/guard STRACE_DIR=$O/canary/strace SERVICE_MAIN=$W/canary.cjs CANARY_TARGET=$ROOTS/canary/$mode.html \
    "$W/svc-$mode.sh" --data-dir "$W/canary-data" < /dev/null
done
canary=$(node "$GUARD" analyze "$O/canary" --own "$W/canary-data,$W/tmp" --roots "$ROOTS" --expect "$ROOTS/canary/" | tee -a "$O/summary.log" | tail -1)
[ "$canary" = "canary=seen" ] || { echo "CANARY FAILED: $canary"; exit 4; }

fail=0
for mode in guard strace; do
  rm -rf "$ROOTS" "$W/config-$mode" && mkdir -p "$ROOTS" "$W/config-$mode"
  echo "== $mode $(date +%T)" | tee -a "$O/summary.log"
  node scripts/sync-pair.mjs --bin "$W/htmlclay" --service-cmd "$W/svc-$mode.sh" --roots "$ROOTS" --local "$LOCAL" \
    --config-base "$W/config-$mode" > "$O/pair-$mode.log" 2>&1
  code=$?
  cp "$W/config-$mode/app.log" "$O/app-$mode.log" 2>/dev/null
  mkdir -p "$O/state-$mode"
  cp -r "$W/config-$mode/htmlclay/service/logs" "$W/config-$mode/htmlclay/service/sync-meta" "$O/state-$mode/" 2>/dev/null
  tail -12 "$O/pair-$mode.log" | tee -a "$O/summary.log"
  [ $code -eq 0 ] || fail=1
done
# Both runs share one config path shape; the data dirs differ, so both are own.
verdict=$(node "$GUARD" analyze "$O" --own "$W/config-guard/htmlclay/service,$W/config-strace/htmlclay/service,$W/tmp" --roots "$ROOTS" | tee -a "$O/summary.log" | tail -1)
[ "$verdict" = "forbidden=0 strace=0" ] || fail=1
if [ $fail -eq 0 ]; then echo "PAIR OK $verdict $canary" | tee -a "$O/summary.log"; else echo "PAIR FAILED $verdict $canary" | tee -a "$O/summary.log"; fi
exit $fail
