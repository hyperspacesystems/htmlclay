#!/bin/bash
set -euo pipefail

[ $# -ge 1 ] && [ $# -le 2 ] || { echo "usage: hardened-run.sh <app> [--x86]" >&2; exit 1; }
APP="$1"
X86=false
if [ "${2:-}" = "--x86" ]; then
  X86=true
elif [ $# -eq 2 ]; then
  echo "usage: hardened-run.sh <app> [--x86]" >&2
  exit 1
fi
PRODUCT_ID=com.htmlclay.HTMLClay
fail() { echo "HARDENED FAILED: $*" >&2; exit 1; }

ID="$(plutil -extract CFBundleIdentifier raw "$APP/Contents/Info.plist" 2>/dev/null || true)"
[ -n "$ID" ] || fail "$APP has no CFBundleIdentifier"
[ "$ID" != "$PRODUCT_ID" ] || fail "$APP is the product id; this run refuses $PRODUCT_ID"

mkdir -p "$HOME/scratch"
D="$(mktemp -d "$HOME/scratch/hardened.XXXXXX")"
HOME_D="$D/home"
CONFIG_BASE="$HOME_D/Library/Application Support"
CONFIG_DIR="$CONFIG_BASE/htmlclay"
SERVED="$D/served/site"
LOG="$CONFIG_DIR/htmlclay.log"
HOST_PID=""

cleanup() {
  if [ -n "$HOST_PID" ] && kill -0 "$HOST_PID" 2>/dev/null; then
    kill -TERM "$HOST_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

mkdir -p "$SERVED" "$CONFIG_DIR"
cat > "$SERVED/index.html" <<'HTML'
<!DOCTYPE html><html><body><p>hardened</p></body></html>
HTML

PORT="$(python3 -c 'import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()')"
python3 - "$CONFIG_DIR/config.json" "$SERVED" "$PORT" <<'PY'
import json, secrets, sys
config = {
    "startOnLogin": False,
    "port": int(sys.argv[3]),
    "updates": "off",
    "servedFolders": [{
        "id": secrets.token_hex(8),
        "name": "site",
        "path": sys.argv[2],
        "identity": "",
        "serving": True,
    }],
}
with open(sys.argv[1], "w") as f:
    f.write(json.dumps(config, indent=2) + "\n")
PY

PREFIX=""
[ "$X86" = true ] && PREFIX="arch -x86_64"
env HOME="$HOME_D" HTMLCLAY_CONFIG_BASE="$CONFIG_BASE" HTMLCLAY_OPEN_LOG="$D/open.log" \
  $PREFIX "$APP/Contents/MacOS/htmlclay-host" --no-tray > "$D/host.out" 2>&1 &
HOST_PID=$!

ready=false
start=$SECONDS
while [ $((SECONDS - start)) -lt 60 ]; do
  if [ -f "$LOG" ] && grep -q "The live-room service started in" "$LOG" \
    && grep -qF "Serving folder $SERVED at http://" "$LOG"; then
    ready=true
    break
  fi
  kill -0 "$HOST_PID" 2>/dev/null || break
  sleep 0.5
done
if [ "$ready" != true ]; then
  echo "last 20 log lines ($LOG):" >&2
  tail -20 "$LOG" 2>/dev/null >&2 || true
  tail -20 "$D/host.out" >&2 || true
  fail "the service was not ready within 60 s"
fi

serving_line="$(grep -F "Serving folder $SERVED at http://" "$LOG" | head -1)"
HOSTADDR="${serving_line##* at http://}"
HOSTADDR="${HOSTADDR%% *}"
[ -n "$HOSTADDR" ] || fail "the log names no address for $SERVED"

MARKER="hardened-${D##*/}"
BODY="<!DOCTYPE html><html><body><p>hardened</p><!-- $MARKER --></body></html>"
SAVE_JS="$(cat <<'JS'
const http = require('node:http');
const [port, host, doc, body] = process.argv.slice(1);
const request = http.request({
  host: '127.0.0.1',
  port: Number(port),
  method: 'POST',
  path: '/_/save',
  headers: {
    Host: host,
    Origin: `http://${host}`,
    'Sec-Fetch-Site': 'same-origin',
    'Content-Type': 'text/html',
    'Document-URL': `http://${host}/${doc}`,
    'Content-Length': Buffer.byteLength(body),
  },
  timeout: 15000,
}, (response) => {
  response.resume();
  response.on('end', () => process.stdout.write(String(response.statusCode)));
});
request.on('timeout', () => request.destroy(new Error('timeout')));
request.on('error', (error) => {
  process.stdout.write(`error ${error.code || error.message}`);
  process.exit(1);
});
request.end(body);
JS
)"
status="$(node -e "$SAVE_JS" "$PORT" "$HOSTADDR" index.html "$BODY" || true)"
case "$status" in
  2*) ;;
  *) fail "the save answered $status" ;;
esac
grep -qF "$MARKER" "$SERVED/index.html" || fail "the save did not land"

NODE_BIN="$(find "$CONFIG_DIR/service-runtime" -type f -name node 2>/dev/null | head -1)"
[ -n "$NODE_BIN" ] || fail "the host unpacked no Node runtime under $CONFIG_DIR/service-runtime"
node_ents="$(codesign -d --entitlements - --xml "$NODE_BIN" 2>/dev/null || true)"
case "$node_ents" in
  *com.apple.security.cs.allow-jit*) ;;
  *) fail "$NODE_BIN carries no com.apple.security.cs.allow-jit" ;;
esac
node_dv="$(codesign -dvv "$NODE_BIN" 2>&1 || true)"
case "$node_dv" in
  *"Authority=Developer ID Application"*) ;;
  *) fail "$NODE_BIN has no Developer ID Application authority" ;;
esac

kill -TERM "$HOST_PID" 2>/dev/null || true
stopped=false
start=$SECONDS
while [ $((SECONDS - start)) -lt 5 ]; do
  if ! kill -0 "$HOST_PID" 2>/dev/null; then
    stopped=true
    break
  fi
  sleep 0.2
done
if [ "$stopped" != true ]; then
  kill -9 "$HOST_PID" 2>/dev/null || true
  fail "the host did not stop within 5 s"
fi
HOST_PID=""

echo "logs in $D"
if [ "$X86" = true ]; then
  echo "HARDENED OK $APP x86_64"
else
  echo "HARDENED OK $APP"
fi
