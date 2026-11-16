#!/bin/bash
# One throwaway 2.x to 2.x rehearsal: the Swift app updates itself through
# Sparkle to the next 2.x while three open tabs keep saving, and the run is
# judged by named pass checks rather than by a log alone.
#
# The cases: `next` is the plain update, triggered by the canary through
# update-now, so Sparkle installs the new version and relaunches it; `next-quit`
# is the plain quit, where Sparkle installs an update the person has already
# prepared and the supervisor checks the new version serves before it leaves it
# alone; `next-rollback` installs a bundle whose host is a version the bundle
# does not claim, so the recovery supervisor puts the old bundle back and
# relaunches it.
#
# Both bundles carry the throwaway id com.htmlclay.UpgradeCheck and claim no
# file types or URL schemes. Everything runs on a scratch home and config base
# under ~/scratch, and the installed HTML Clay is only ever read: its login
# item, its config, its process and the app LaunchServices opens .htmlclay with
# are compared before and after.
#
# Usage: scripts/next-rehearsal.sh next|next-quit|next-rollback [--served]
#
# --served (the `next` case only): the old app serves ~/site before the update,
# and the run also checks that the new app serves the same address again and
# runs its service.
#
# Exit codes: 0 every check passed, 1 one of them did not, 2 usage. The verdict
# is the last line on stderr and the logs stay in the run folder.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ID="${HTMLCLAY_REHEARSAL_ID:-com.htmlclay.UpgradeCheck}"
case "com.htmlclay.HTMLClay" in
  "$ID"*) echo "[rehearsal] refusing bundle id '$ID': it is the installed HTML Clay's id or a prefix of it" >&2; exit 2 ;;
esac
CASE=""
SERVED=""
SERVED_ADDRESS=""
OLD_VERSION="9.1.0"
NEW_VERSION="9.1.1"
SWIFT_CACHE="$HOME/scratch/20261115-100943-macos-packaging/swift-cache"
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
DEADLINE="${HTMLCLAY_REHEARSAL_DEADLINE:-240}"
OBSERVE="${HTMLCLAY_REHEARSAL_OBSERVE:-60}"

say() { echo "[rehearsal] $*" >&2; }

usage() {
  say "usage: next-rehearsal.sh next|next-quit|next-rollback [--served]"
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    next|next-quit|next-rollback)
      [ -z "$CASE" ] || usage
      CASE="$1"
      shift
      ;;
    --served)
      SERVED=yes
      shift
      ;;
    *)
      usage
      ;;
  esac
done
[ -n "$CASE" ] || usage
[ -z "$SERVED" ] || [ "$CASE" = "next" ] || usage

# The host's single-instance socket lives at <config>/sock, and macOS refuses a
# unix socket path past 102 bytes, so the run folder's name stays short: the
# case and a UTC clock time, under ~/scratch.
make_scratch() {
  local leaf
  leaf="$CASE-$(date -u '+%H%M%S')"
  while [ -e "$HOME/scratch/$leaf" ]; do
    sleep 1
    leaf="$CASE-$(date -u '+%H%M%S')"
  done
  echo "$HOME/scratch/$leaf"
}

SCRATCH="$(make_scratch)"
OLD="$SCRATCH/old.noindex/HTMLClay.app"
NEW_SRC="$SCRATCH/new.noindex/HTMLClay.app"
NEW_ZIP=""
FEED_HTTP="$SCRATCH/feed"
FAKE_HOME="$SCRATCH/home"
CONFIG="$FAKE_HOME/Library/Application Support/htmlclay"
LOGS="$SCRATCH/logs"
BIN="$SCRATCH/bin"
KEY="$SCRATCH/keys/ed25519.seed"
PROBE="$SCRATCH/probe/probe.htmlclay"
OPEN_LOG="$LOGS/open.log"
REAL_HOME="$HOME"
HTTP_PORT=""
FEED_URL=""
APP_PID=""
HTTP_PID=""
WATCH_PID=""
CANARY_PID=""
LEDGER_PID=""
LEDGER_STOP=""
SAMPLER_PID=""
HOST_SAMPLER_PID=""
PORT_BEFORE=""
PUB=""
PRIV=""
PREPARED="no"
BUILD_VERSION="$NEW_VERSION"
SHIPPED_VERSION="$NEW_VERSION"
LEDGER_MODE="first"
[ "$CASE" = "next-quit" ] && LEDGER_MODE="quit"
[ "$CASE" = "next-rollback" ] && { LEDGER_MODE="rollback"; BUILD_VERSION="9.1.2"; }
STARTED=$SECONDS
CHECKS=()
FAILED=()
CHECKS_RAN="no"
VERDICT=""
VERDICT_OK="no"
BEFORE=()
NOW=()
COMPARE=""

free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

fingerprint() { if [ -e "$1" ]; then shasum -a 256 "$1" | cut -d' ' -f1; else echo absent; fi; }

pids_comm() { ps -axo pid=,comm= | awk -v p="$1" '$2 == p { print $1 }' | tr '\n' ' ' | sed 's/ *$//'; }

plist_key() { plutil -extract "$2" raw -o - "$1" 2>/dev/null; }

now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

# iso_ms ISO8601: the epoch milliseconds of a timestamp Go wrote as JSON.
iso_ms() {
  [ -n "$1" ] || return 0
  python3 - "$1" <<'PY' 2>/dev/null
import datetime, sys
s = sys.argv[1].strip()
if s.endswith("Z"):
    s = s[:-1] + "+00:00"
head, dot, rest = s.partition(".")
if dot:
    i = 0
    while i < len(rest) and rest[i].isdigit():
        i += 1
    s = head + "." + rest[:i][:6] + rest[i:]
print(int(datetime.datetime.fromisoformat(s).timestamp() * 1000))
PY
}

alive() {
  local stat
  stat="$(ps -p "$1" -o stat= 2>/dev/null | tr -d ' ')"
  [ -n "$stat" ] || return 1
  case "$stat" in Z*) return 1 ;; esac
  return 0
}

snapshot() {
  BEFORE=()
  BEFORE+=("$(fingerprint "$REAL_HOME/Library/LaunchAgents/com.htmlclay.plist")")
  BEFORE+=("$(fingerprint "$REAL_HOME/Library/Application Support/htmlclay/config.json")")
  BEFORE+=("$(pgrep -f '/Applications/HTMLClay.app/Contents/MacOS/htmlclay' | sort | tr '\n' ' ')")
  BEFORE+=("$(swift "$BIN/handler.swift" "$PROBE" 2>/dev/null)")
}

now_state() {
  NOW=()
  NOW+=("$(fingerprint "$REAL_HOME/Library/LaunchAgents/com.htmlclay.plist")")
  NOW+=("$(fingerprint "$REAL_HOME/Library/Application Support/htmlclay/config.json")")
  NOW+=("$(pgrep -f '/Applications/HTMLClay.app/Contents/MacOS/htmlclay' | sort | tr '\n' ' ')")
  NOW+=("$(swift "$BIN/handler.swift" "$PROBE" 2>/dev/null)")
}

kill_pid() {
  local pid="$1" i
  [ -n "$pid" ] || return 0
  kill -0 "$pid" 2>/dev/null || return 0
  kill -TERM "$pid" 2>/dev/null
  for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    kill -0 "$pid" 2>/dev/null || return 0
    sleep 0.25
  done
  kill -KILL "$pid" 2>/dev/null
  return 0
}

# listen_port: the port pid listens on. A host also listens on a private port for
# its Mac app, in no fixed order, so PORT_BEFORE wins when it is among them.
listen_port() {
  local ports
  ports="$(lsof -nP -a -iTCP -sTCP:LISTEN -p "$1" 2>/dev/null | awk 'NR > 1 { sub(/.*:/, "", $9); print $9 }')"
  if [ -n "$PORT_BEFORE" ] && printf '%s\n' "$ports" | grep -qx "$PORT_BEFORE"; then
    echo "$PORT_BEFORE"
  else
    printf '%s\n' "$ports" | head -n 1
  fi
}

record_check() {
  local name="$1" pass="$2" detail="${3:-}"
  if [ "$pass" = "yes" ]; then
    CHECKS+=("$name: ok${detail:+ $detail}")
    say "CHECK $name: ok${detail:+ $detail}"
  else
    CHECKS+=("$name: FAILED${detail:+ $detail}")
    FAILED+=("$name")
    say "CHECK $name: FAILED${detail:+ $detail}"
  fi
}

# verdict_line: one verdict from the checks, printed as the last stderr line.
verdict_line() {
  local name names=""
  if [ "$CHECKS_RAN" = "yes" ] && [ ${#FAILED[@]} -eq 0 ]; then
    VERDICT_OK=yes
    VERDICT="OK $CASE"
    return 0
  fi
  VERDICT_OK=no
  for name in "${FAILED[@]:-}"; do
    [ -n "$name" ] || continue
    names="${names:+$names, }$name"
  done
  VERDICT="FAILED $CASE: ${names:-no checks ran}"
}

set_bundle_id() {
  local app="$1" id="$2"
  plutil -replace CFBundleIdentifier -string "$id" "$app/Contents/Info.plist"
  local key
  for key in CFBundleDocumentTypes UTExportedTypeDeclarations UTImportedTypeDeclarations CFBundleURLTypes; do
    plutil -remove "$key" "$app/Contents/Info.plist" 2>/dev/null || true
  done
  plutil -lint "$app/Contents/Info.plist" >/dev/null
}

set_local_networking() {
  local plist="$1/Contents/Info.plist"
  if plutil -extract NSAppTransportSecurity raw -o - "$plist" >/dev/null 2>&1; then
    plutil -replace NSAppTransportSecurity.NSAllowsLocalNetworking -bool true "$plist"
  else
    plutil -insert NSAppTransportSecurity -json '{"NSAllowsLocalNetworking":true}' "$plist"
  fi
}

set_environment() {
  local app="$1" plist="$1/Contents/Info.plist" json
  json="$(printf '{"HTMLCLAY_CANDIDATE_HOME":"%s","HTMLCLAY_CONFIG_BASE":"%s","HTMLCLAY_OPEN_LOG":"%s"}' \
    "$FAKE_HOME" "$FAKE_HOME/Library/Application Support" "$OPEN_LOG")"
  if plutil -extract LSEnvironment raw -o - "$plist" >/dev/null 2>&1; then
    plutil -replace LSEnvironment -json "$json" "$plist"
  else
    plutil -insert LSEnvironment -json "$json" "$plist"
  fi
}

resign() {
  local app="$1"
  codesign --force --deep -s - "$app" >>"$LOGS/codesign.log" 2>&1 &&
    codesign --verify --strict "$app" >>"$LOGS/codesign.log" 2>&1
}

make_appcast() {
  local zip="$1" base="$2" out="$3"
  mkdir -p "$out"
  SPARKLE_ED_PRIVATE_KEY="$PRIV" SPARKLE_PUBLIC_ED_KEY="$PUB" SPARKLE_BASE_URL="$base" \
    bash "$ROOT/packaging/macos/sparkle-appcast.sh" "$SHIPPED_VERSION" "$zip" "$out" >>"$LOGS/appcast.log" 2>&1 || return 1
  mkdir -p "$out/$SHIPPED_VERSION"
  cp "$zip" "$out/$SHIPPED_VERSION/"
}

# watch_timing: record when the bundle changed, keep a copy of the handoff while
# it exists, and record when it went. It runs beside the canary and the
# supervisor.
watch_timing() {
  local limit=$((DEADLINE + OBSERVE)) start=$SECONDS present=0
  while [ $((SECONDS - start)) -lt "$limit" ]; do
    if [ ! -f "$LOGS/version-changed.ms" ] &&
       [ "$(plist_key "$OLD/Contents/Info.plist" CFBundleVersion)" = "$SHIPPED_VERSION" ]; then
      now_ms >"$LOGS/version-changed.ms"
    fi
    if [ -f "$CONFIG/update-handoff.json" ]; then
      cp "$CONFIG/update-handoff.json" "$LOGS/handoff.json" 2>/dev/null
      present=1
    elif [ "$present" = 1 ] && [ ! -f "$LOGS/handoff-gone.ms" ]; then
      now_ms >"$LOGS/handoff-gone.ms"
    fi
    sleep 0.25
  done
}

cleanup() {
  local status=$?
  case "com.htmlclay.HTMLClay" in
    "$ID"*) echo "[rehearsal] cleanup refused for bundle id '$ID'" >&2; return 1 ;;
  esac
  trap - EXIT
  say "cleaning up"
  kill_pid "$CANARY_PID"
  kill_pid "$HTTP_PID"
  kill_pid "$APP_PID"
  kill_pid "$WATCH_PID"
  kill_pid "$LEDGER_PID"
  kill_pid "$SAMPLER_PID"
  kill_pid "$HOST_SAMPLER_PID"

  local pid
  for pid in $(ps -axo pid=,comm= | awk -v p="$SCRATCH/" 'index($2, p) == 1 { print $1 }'); do
    [ "$pid" = "$$" ] && continue
    say "stopping leftover process $pid: $(ps -p "$pid" -o comm= 2>/dev/null)"
    kill_pid "$pid"
  done

  local app inner
  while IFS= read -r -d '' app; do
    "$LSREGISTER" -u "$app" >/dev/null 2>&1
    echo "lsregister -u $app" >>"$LOGS/cleanup.log"
  done < <(find "$SCRATCH" -maxdepth 4 -name '*.app' -type d -print0 2>/dev/null)

  defaults delete "$ID" >>"$LOGS/cleanup.log" 2>&1
  echo "defaults delete $ID" >>"$LOGS/cleanup.log"

  local path found=()
  for dir in "$REAL_HOME/Library/Caches" "$REAL_HOME/Library/Application Support" "$REAL_HOME/Library/Preferences" "$REAL_HOME/Library/Saved Application State" "$REAL_HOME/Library/HTTPStorages" "$REAL_HOME/Library/WebKit"; do
    [ -d "$dir" ] || continue
    for path in "$dir"/*"$ID"*; do
      [ -e "$path" ] || continue
      found+=("$path")
    done
  done
  for path in "${found[@]:-}"; do
    [ -n "$path" ] || continue
    while IFS= read -r -d '' inner; do
      "$LSREGISTER" -u "$inner" >/dev/null 2>&1
      echo "lsregister -u $inner" >>"$LOGS/cleanup.log"
    done < <(find "$path" -maxdepth 6 -name '*.app' -type d -print0 2>/dev/null)
    echo "removing $path" >>"$LOGS/cleanup.log"
    rm -rf "$path"
  done

  now_state
  if [ "${NOW[3]}" != "${BEFORE[3]}" ] && [ -n "${BEFORE[3]}" ]; then
    swift "$BIN/restore-handler.swift" "${BEFORE[3]}" "$PROBE" >>"$LOGS/cleanup.log" 2>&1 || true
    say "put the .htmlclay handler back to ${BEFORE[3]}"
    now_state
  fi
  local i
  COMPARE=yes
  for i in 0 1 2 3; do
    if [ "${NOW[$i]}" != "${BEFORE[$i]}" ]; then
      say "the installed app's surroundings changed: '$i' was '${BEFORE[$i]}' and is '${NOW[$i]}'"
      COMPARE=no
    fi
  done
  if [ "$COMPARE" = "yes" ]; then
    record_check installed-unchanged yes
  else
    record_check installed-unchanged no "the installed HTML Clay's surroundings changed"
  fi

  rm -rf "$OLD" "$NEW_SRC" "$SCRATCH/stage" "$SCRATCH/zip" "$SCRATCH/old.noindex" "$SCRATCH/new.noindex"
  echo "removed the scratch bundles, kept $LOGS" >>"$LOGS/cleanup.log"

  # Sparkle's installer leaves Launcher copies of its Updater.app behind, and
  # LaunchServices keeps a record of a path after the files are gone; anything
  # still naming this run's id goes now.
  local round stale
  for round in 1 2 3; do
    while IFS= read -r stale; do
      "$LSREGISTER" -u "$stale" >/dev/null 2>&1
      echo "lsregister -u $stale" >>"$LOGS/cleanup.log"
    done < <("$LSREGISTER" -dump 2>/dev/null | sed -n 's/^path: *\(.*\) (0x[0-9a-f]*)$/\1/p' | grep -F "$ID")
    "$LSREGISTER" -dump 2>/dev/null | grep -q "$ID" || break
  done
  "$LSREGISTER" -dump 2>/dev/null | grep -c "$ID" >"$LOGS/cleanup-lsregister-count.txt"
  { defaults read "$ID"; } >"$LOGS/cleanup-defaults.txt" 2>&1 || true
  ps -A -o pid,command | grep -cF "$SCRATCH/" >"$LOGS/cleanup-ps-count.txt"
  ps -A -o pid,command | grep -F "$SCRATCH/" >"$LOGS/cleanup-ps.txt" 2>&1

  verdict_line
  say "run folder kept: $SCRATCH"
  echo "REHEARSAL $VERDICT" >&2
  if [ "$VERDICT_OK" = "yes" ]; then exit 0; fi
  exit 1
}
trap cleanup EXIT

# --- the app's own state files ---------------------------------------------

candidate_field() {
  [ -f "$FAKE_HOME/candidate-state.json" ] || return 0
  python3 - "$FAKE_HOME/candidate-state.json" "$1" <<'PY' 2>/dev/null
import json, sys
try:
    value = json.load(open(sys.argv[1])).get(sys.argv[2], "")
except Exception:
    value = ""
print(value if isinstance(value, str) else json.dumps(value, sort_keys=True))
PY
}

state_field() { plutil -extract "$1" raw -o - "$CONFIG/updates/state.json" 2>/dev/null; }

state_phase() { state_field phase; }

health_field() { plutil -extract "$1" raw -o - "$CONFIG/updates/health.json" 2>/dev/null; }

state_shape_ok() {
  python3 - "$1" "$APP_PID" <<'PY'
import json, sys
try:
    state = json.load(open(sys.argv[1]))
except Exception:
    raise SystemExit(1)
ok = (state.get("executable") == "Contents/MacOS/htmlclay"
      and "Contents/MacOS/htmlclay-host" in (state.get("helpers") or [])
      and str(state.get("oldPid")) == sys.argv[2])
raise SystemExit(0 if ok else 1)
PY
}

host_info_version() {
  "$OLD/Contents/MacOS/htmlclay-host" update-info 2>/dev/null |
    python3 -c 'import json, sys
try:
    print(json.load(sys.stdin).get("version", ""))
except Exception:
    print("")' 2>/dev/null
}

# --- setup -----------------------------------------------------------------

mkdir -p "$SCRATCH" "$LOGS" "$BIN" "$CONFIG" "$FAKE_HOME" "$FAKE_HOME/docs" "$FAKE_HOME/docs2" "$FAKE_HOME/loose" "$(dirname "$PROBE")" "$(dirname "$KEY")"
: >"$LOGS/cleanup.log"
say "run folder: $SCRATCH"

printf '%s\n' '<!DOCTYPE html><html><body><p>rehearsal</p></body></html>' >"$FAKE_HOME/docs/a.htmlclay"
printf '%s\n' '<!DOCTYPE html><html><body><p>second</p></body></html>' >"$FAKE_HOME/docs2/b.htmlclay"
printf '%s\n' '<!DOCTYPE html><html><body><p>loose</p></body></html>' >"$FAKE_HOME/loose/c.htmlclay"
printf '<!doctype html><title>probe</title><p>probe</p>\n' >"$PROBE"

cat >"$BIN/handler.swift" <<'SWIFT'
import AppKit
import Foundation
let url = URL(fileURLWithPath: CommandLine.arguments[1])
if let app = NSWorkspace.shared.urlForApplication(toOpen: url) {
    print(app.path)
} else {
    print("none")
}
SWIFT

cat >"$BIN/restore-handler.swift" <<'SWIFT'
import AppKit
import Foundation
let app = URL(fileURLWithPath: CommandLine.arguments[1])
let probe = URL(fileURLWithPath: CommandLine.arguments[2])
do {
    try NSWorkspace.shared.setDefaultApplication(at: app, toOpen: probe)
    print("restored \(app.path)")
} catch {
    print("could not restore \(app.path): \(error)")
}
SWIFT

cat >"$BIN/genkey.swift" <<'SWIFT'
import CryptoKit
import Foundation
let key = Curve25519.Signing.PrivateKey()
try! key.rawRepresentation.base64EncodedString().write(toFile: CommandLine.arguments[1], atomically: true, encoding: .utf8)
print(key.publicKey.rawRepresentation.base64EncodedString())
SWIFT

snapshot
say "before: agent=${BEFORE[0]} config=${BEFORE[1]} pid=${BEFORE[2]} handler=${BEFORE[3]}"

if [ -n "${BEFORE[2]// /}" ]; then
  say "the installed HTML Clay is running as ${BEFORE[2]}"
else
  say "warning: the installed HTML Clay does not appear to be running"
fi

HTTP_PORT="$(free_port)"
FEED_URL="http://127.0.0.1:$HTTP_PORT/appcast.xml"
NEW_ZIP="$SCRATCH/zip/HTMLClay-$SHIPPED_VERSION.app.zip"
say "the feed will be $FEED_URL"

PORT="$(free_port)"
mkdir -p "$(dirname "$CONFIG/config.json")"
cat >"$CONFIG/config.json" <<JSON
{"startOnLogin": false, "port": $PORT, "workspaceFolders": [{"path": "$FAKE_HOME/docs"}, {"path": "$FAKE_HOME/docs2"}]}
JSON

PUB="$(swift "$BIN/genkey.swift" "$KEY" 2>>"$LOGS/keys.log")"
if [ -z "$PUB" ]; then
  say "could not make the throwaway key"
  record_check key no "could not make the throwaway Ed25519 key"
  CHECKS_RAN=yes
  exit 1
fi
PRIV="$(cat "$KEY")"
if [ "$(SPARKLE_ED_PRIVATE_KEY="$PRIV" swift "$ROOT/packaging/macos/sparkle-pubkey.swift" 2>>"$LOGS/keys.log")" != "$PUB" ]; then
  say "the throwaway key pair does not match"
  record_check key no "the throwaway key pair does not match"
  CHECKS_RAN=yes
  exit 1
fi
echo "public $PUB" >"$LOGS/keys.log"

# --- both bundles ----------------------------------------------------------

build_bundle() {
  local out="$1" version="$2" tag="$3"
  BUNDLE_ID="$ID" VERSION="$version" UPDATE_CHANNEL=stable SPARKLE_PUBLIC_ED_KEY="$PUB" \
    SPARKLE_FEED_URL="$FEED_URL" OUT_DIR="$out" HTMLCLAY_SWIFT_CACHE="$SWIFT_CACHE" \
    bash "$ROOT/packaging/macos/build.sh" --adhoc-hardened --no-dmg >"$LOGS/build-$tag.log" 2>&1
}

say "building the old bundle $OLD_VERSION"
if ! build_bundle "$SCRATCH/old.noindex" "$OLD_VERSION" old || [ ! -x "$OLD/Contents/MacOS/htmlclay" ]; then
  say "the old bundle was not built; see logs/build-old.log"
  record_check build-old no "packaging/macos/build.sh did not produce $OLD; see logs/build-old.log"
  CHECKS_RAN=yes
  exit 1
fi

say "building the new bundle $SHIPPED_VERSION (host built as $BUILD_VERSION)"
if ! build_bundle "$SCRATCH/new.noindex" "$BUILD_VERSION" new || [ ! -x "$NEW_SRC/Contents/MacOS/htmlclay" ]; then
  say "the new bundle was not built; see logs/build-new.log"
  record_check build-new no "packaging/macos/build.sh did not produce $NEW_SRC; see logs/build-new.log"
  CHECKS_RAN=yes
  exit 1
fi
if [ "$CASE" = "next-rollback" ]; then
  plutil -replace CFBundleVersion -string "$SHIPPED_VERSION" "$NEW_SRC/Contents/Info.plist"
  plutil -replace CFBundleShortVersionString -string "$SHIPPED_VERSION" "$NEW_SRC/Contents/Info.plist"
  say "set the new bundle's plist back to $SHIPPED_VERSION over a $BUILD_VERSION host"
fi

for app in "$OLD" "$NEW_SRC"; do
  set_bundle_id "$app" "$ID"
  set_local_networking "$app"
  set_environment "$app"
done
if ! resign "$OLD"; then
  say "could not sign the old bundle"
  record_check build-old no "the old bundle could not be signed"
  CHECKS_RAN=yes
  exit 1
fi
if ! resign "$NEW_SRC"; then
  say "could not sign the new bundle"
  record_check build-new no "the new bundle could not be signed"
  CHECKS_RAN=yes
  exit 1
fi
[ "$(plist_key "$OLD/Contents/Info.plist" SUPublicEDKey)" = "$PUB" ] || say "warning: the old bundle does not carry the throwaway key"

defaults write "$ID" welcomeDone -bool true
if [ "$CASE" = "next-quit" ]; then
  defaults write "$ID" SULastCheckTime -date "$(date -u -v-1d '+%Y-%m-%d %H:%M:%S +0000')"
fi

mkdir -p "$SCRATCH/zip"
rm -rf "$SCRATCH/stage"
mkdir -p "$SCRATCH/stage"
ditto "$NEW_SRC" "$SCRATCH/stage/HTMLClay.app"
ditto -c -k --sequesterRsrc --keepParent "$SCRATCH/stage/HTMLClay.app" "$NEW_ZIP" >>"$LOGS/zip.log" 2>&1
if [ ! -f "$NEW_ZIP" ]; then
  say "could not zip the new bundle"
  record_check appcast no "could not zip the new bundle"
  CHECKS_RAN=yes
  exit 1
fi

say "serving the feed on 127.0.0.1:$HTTP_PORT"
if ! make_appcast "$NEW_ZIP" "http://127.0.0.1:$HTTP_PORT" "$FEED_HTTP"; then
  say "could not write the appcast"
  record_check appcast no "could not write the appcast; see logs/appcast.log"
  CHECKS_RAN=yes
  exit 1
fi
( cd "$FEED_HTTP" && exec python3 -m http.server --bind 127.0.0.1 "$HTTP_PORT" >"$LOGS/http.server.log" 2>&1 ) &
HTTP_PID=$!
FEED_UP=""
for _ in $(seq 1 40); do
  curl -fsS -o /dev/null "http://127.0.0.1:$HTTP_PORT/appcast.xml" 2>/dev/null && { FEED_UP=ok; break; }
  sleep 0.25
done
say "http feed ready: ${FEED_UP:-no}"
if [ -z "$FEED_UP" ]; then
  record_check appcast no "nothing answered http://127.0.0.1:$HTTP_PORT/appcast.xml"
  CHECKS_RAN=yes
  exit 1
fi

# --- wait helpers ----------------------------------------------------------

wait_gate() {
  local start=$SECONDS state gate sparkle
  while [ $((SECONDS - start)) -lt 90 ]; do
    state="$(candidate_field state)"
    gate="$(candidate_field updateGate)"
    sparkle="$(candidate_field sparkle)"
    if [ "$state" = "running" ] && [ "$gate" = "on" ] && [ "$sparkle" = "started" ]; then
      say "the old app runs with its gate on and Sparkle started after $((SECONDS - start)) s"
      return 0
    fi
    alive "$APP_PID" || return 1
    sleep 0.5
  done
  return 1
}

open_loose_document() {
  local start=$SECONDS
  : >"$OPEN_LOG"
  open -a "$OLD" "$FAKE_HOME/loose/c.htmlclay" >>"$LOGS/open-command.log" 2>&1
  while [ $((SECONDS - start)) -lt 20 ]; do
    if [ -f "$OPEN_LOG" ] && grep -q 'c\.htmlclay' "$OPEN_LOG"; then
      say "the old app opened the loose file: $(grep -m 1 'c\.htmlclay' "$OPEN_LOG")"
      return 0
    fi
    sleep 0.25
  done
  return 1
}

wait_prepared() {
  local start=$SECONDS value
  while [ $((SECONDS - start)) -lt 120 ]; do
    value="$(candidate_field prepared)"
    if [ "$value" = "$NEW_VERSION" ]; then
      say "the app has $value prepared after $((SECONDS - start)) s"
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_phase_done() {
  local start=$SECONDS phase last=""
  while [ $((SECONDS - start)) -lt "$DEADLINE" ]; do
    phase="$(state_phase)"
    if [ -n "$phase" ] && [ "$phase" != "$last" ]; then
      say "state.json phase: $phase"
      last="$phase"
    fi
    case "$phase" in
      done)
        now_ms >"$LOGS/done.ms"
        say "state.json reached done after $((SECONDS - start)) s"
        return 0
        ;;
      rolledBack|rollbackFailed) return 1 ;;
    esac
    sleep 1
  done
  return 1
}

wait_rolled_back() {
  local start=$SECONDS phase
  while [ $((SECONDS - start)) -lt "$DEADLINE" ]; do
    phase="$(state_phase)"
    case "$phase" in
      rolledBack|rollbackFailed)
        [ -f "$LOGS/rolled-back.ms" ] || now_ms >"$LOGS/rolled-back.ms"
        say "state.json reached $phase after $((SECONDS - start)) s"
        return 0
        ;;
    esac
    sleep 1
  done
  return 1
}

wait_host_port() {
  local start=$SECONDS pid listen
  while [ $((SECONDS - start)) -lt 60 ]; do
    for pid in $(pids_comm "$OLD/Contents/MacOS/htmlclay-host"); do
      listen="$(listen_port "$pid")"
      if [ -n "$PORT_BEFORE" ] && [ "$listen" = "$PORT_BEFORE" ]; then
        say "htmlclay-host $pid listens on $listen after $((SECONDS - start)) s"
        return 0
      fi
    done
    sleep 0.5
  done
  return 1
}

start_canary() {
  (
    export HTMLCLAY_CANARY_APP="$OLD" HTMLCLAY_CANARY_CONFIG_DIR="$CONFIG" HTMLCLAY_CANARY_INSTALL=/usr/bin/false
    exec bash "$ROOT/scripts/update-canary.sh" "$NEW_VERSION"
  ) >"$LOGS/canary.out" 2>"$LOGS/canary.err" &
  CANARY_PID=$!
  say "the canary runs as $CANARY_PID"
}

start_ledger() {
  local mode="$1" i
  LEDGER_STOP="$LOGS/ledger.stop"
  rm -f "$LEDGER_STOP" "$LOGS/ledger-ready"
  node "$ROOT/scripts/upgrade-ledger.mjs" --port "$PORT_BEFORE" --config "$CONFIG" --out "$LOGS" \
    --mode "$mode" --stop-file "$LEDGER_STOP" --times "$LOGS" \
    "$FAKE_HOME/docs/a.htmlclay" "$FAKE_HOME/docs2/b.htmlclay" "$FAKE_HOME/loose/c.htmlclay" \
    >"$LOGS/ledger.out" 2>"$LOGS/ledger.err" &
  LEDGER_PID=$!
  for i in $(seq 1 200); do
    [ -f "$LOGS/ledger-ready" ] && break
    sleep 0.1
  done
  [ -f "$LOGS/ledger-ready" ] || say "the ledger's tabs never saved"
  say "the ledger runs as $LEDGER_PID in $mode mode"
}

start_state_sampler() {
  (
    n=0
    last=""
    while :; do
      if [ -f "$CONFIG/updates/state.json" ]; then
        sum="$(fingerprint "$CONFIG/updates/state.json")"
        if [ -n "$sum" ] && [ "$sum" != "$last" ]; then
          last="$sum"
          n=$((n + 1))
          if cp "$CONFIG/updates/state.json" "$LOGS/state-$n.json" 2>/dev/null; then
            case "$(plutil -extract phase raw -o - "$LOGS/state-$n.json" 2>/dev/null)" in
              rolledBack|rollbackFailed)
                [ -f "$LOGS/rolled-back.ms" ] || now_ms >"$LOGS/rolled-back.ms"
                ;;
            esac
          fi
        fi
      fi
      sleep 0.2
    done
  ) &
  SAMPLER_PID=$!
}

start_host_sampler() {
  (
    while :; do
      ms="$(now_ms)"
      for pid in $(pids_comm "$OLD/Contents/MacOS/htmlclay-host"); do
        echo "$ms $pid" >>"$LOGS/hosts.txt"
      done
      sleep 0.2
    done
  ) &
  HOST_SAMPLER_PID=$!
}

# stop_ledger stops the ledger's tabs and waits for its verdict. It runs once:
# the first stop clears LEDGER_PID, so every path that stops a run early can call
# it without knowing which one got there first.
stop_ledger() {
  local start
  [ -n "$LEDGER_PID" ] || return 0
  : >"$LEDGER_STOP"
  start=$SECONDS
  while [ $((SECONDS - start)) -lt 30 ]; do
    if ! kill -0 "$LEDGER_PID" 2>/dev/null; then
      LEDGER_PID=""
      return 0
    fi
    sleep 0.25
  done
  say "the ledger did not stop within 30 s"
  kill_pid "$LEDGER_PID"
  LEDGER_PID=""
}

stop_run_pids() {
  local pid
  for pid in $(pids_comm "$OLD/Contents/MacOS/htmlclay") $(pids_comm "$OLD/Contents/MacOS/htmlclay-host"); do
    kill_pid "$pid"
  done
}

stop_old_run() {
  stop_ledger
  stop_run_pids
  kill_pid "$CANARY_PID"
  CANARY_PID=""
  kill_pid "$APP_PID"
  APP_PID=""
}

# --- the checks ------------------------------------------------------------

# check_ledger: the ledger's own verdict, which is the last line it printed. Its
# per-tab lines go to this run's stderr so the reasons show beside the checks.
check_ledger() {
  local line
  line="$(tail -n 1 "$LOGS/ledger.out" 2>/dev/null)"
  grep '^\[ledger\]' "$LOGS/ledger.err" >&2 2>/dev/null || true
  if [ "${line#LEDGER OK}" != "$line" ]; then
    record_check ledger yes "$line"
  else
    record_check ledger no "${line:-no ledger output}"
  fi
}

# address_answers: the served address answers a page within $1 seconds.
address_answers() {
  local i code
  for ((i = 0; i < $1; i++)); do
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "${SERVED_ADDRESS%/}/probe.html" 2>/dev/null)"
    echo "$(date +%T) ${SERVED_ADDRESS%/}/probe.html $code" >>"$LOGS/served-probe.log"
    [ "$code" = "200" ] && return 0
    sleep 1
  done
  return 1
}

# serve_docs: the old app serves ~/site through its own command line, as a
# person would, and the address answers before the update starts.
serve_docs() {
  local answer
  # Its own page, so the probe never opens a document the ledger's tabs hold.
  # A folder of its own, which no ledger tab uses: the tabs stay on the 1.x
  # addresses of docs, docs2 and loose.
  mkdir -p "$FAKE_HOME/site"
  printf '%s\n' '<!DOCTYPE html><html><body><p>probe</p></body></html>' >"$FAKE_HOME/site/probe.html"
  answer="$(env HTMLCLAY_CANDIDATE_HOME="$FAKE_HOME" HTMLCLAY_CONFIG_BASE="$FAKE_HOME/Library/Application Support" \
    "$OLD/Contents/MacOS/htmlclay-host" serve "$FAKE_HOME/site" --name site 2>>"$LOGS/serve.log")"
  echo "$answer" >>"$LOGS/serve.log"
  # "served <name> <address>" for a new folder, or a sentence naming the address
  # when the folder is already served (the seeded config serves docs).
  SERVED_ADDRESS="$(printf '%s' "$answer" | grep -oE 'https?://[^ ]+|[a-z0-9-]+\.htmlclay\.localhost:[0-9]+/?' | head -1)"
  if [ -z "$SERVED_ADDRESS" ]; then
    record_check served no "the old app did not serve site: ${answer:-no answer}"
    return 1
  fi
  case "$SERVED_ADDRESS" in http://*) ;; *) SERVED_ADDRESS="http://$SERVED_ADDRESS" ;; esac
  if ! address_answers 30; then
    record_check served no "$SERVED_ADDRESS did not answer before the update"
    return 1
  fi
  say "the old app serves docs at $SERVED_ADDRESS"
}

# check_served: after the update the same address answers again, and the new
# host has started its service as a child process.
check_served() {
  local host_pids children
  host_pids="$(pids_comm "$OLD/Contents/MacOS/htmlclay-host")"
  children=""
  [ -n "$host_pids" ] && children="$(pgrep -P "$host_pids" | tr '\n' ' ' | sed 's/ *$//')"
  if address_answers 30 && [ -n "$children" ]; then
    record_check served yes "$SERVED_ADDRESS answers; host $host_pids runs child $children"
  else
    record_check served no "$SERVED_ADDRESS answers: $(address_answers 1 && echo yes || echo no); host ${host_pids:-none}, children ${children:-none}"
  fi
}

run_next_checks() {
  local file found="" phase version info app_pids host_pids

  for file in "$LOGS"/state-*.json; do
    [ -f "$file" ] || continue
    if state_shape_ok "$file"; then
      found="$(basename "$file")"
      break
    fi
  done
  if [ -n "$found" ]; then
    record_check state yes "$found: executable Contents/MacOS/htmlclay, helpers holds Contents/MacOS/htmlclay-host, oldPid $APP_PID"
  else
    record_check state no "no state-*.json has the app's shape; snapshots: $(ls "$LOGS"/state-*.json 2>/dev/null | tr '\n' ' ')"
  fi

  phase="$(state_phase)"
  if [ "$phase" = "done" ]; then
    record_check done yes
  else
    record_check done no "state.json phase is ${phase:-absent}"
  fi

  app_pids="$(pids_comm "$OLD/Contents/MacOS/htmlclay")"
  host_pids="$(pids_comm "$OLD/Contents/MacOS/htmlclay-host")"
  version="$(plist_key "$OLD/Contents/Info.plist" CFBundleShortVersionString)"
  info="$(host_info_version)"
  if [ "$(printf '%s' "$app_pids" | wc -w | tr -d ' ')" = "1" ] &&
     [ "$(printf '%s' "$host_pids" | wc -w | tr -d ' ')" = "1" ] &&
     [ "$app_pids" != "$APP_PID" ] && [ "$host_pids" != "$APP_PID" ] &&
     [ "$version" = "$NEW_VERSION" ] && [ "$info" = "$NEW_VERSION" ]; then
    record_check processes yes "htmlclay=$app_pids htmlclay-host=$host_pids, $version"
  else
    record_check processes no "htmlclay=${app_pids:-none} htmlclay-host=${host_pids:-none}, old app $APP_PID, bundle ${version:-?}, host update-info ${info:-?}"
  fi

  check_login_item
  [ -z "$SERVED" ] || check_served

  CHECKS_RAN=yes
}

# check_login_item: the new app's reconciler leaves Open at Login alone for a
# minute and then unregisters it once, since the config has startOnLogin false.
check_login_item() {
  local t now elapsed start
  t="$(iso_ms "$(health_field time)")"
  if [ -z "$t" ]; then
    record_check login-item-wait no "health.json has no time"
    record_check login-item-runs no "health.json has no time"
    return 0
  fi
  now="$(now_ms)"
  elapsed=$(( (now - t) / 1000 ))
  if [ "$elapsed" -lt 55 ] && [ ! -e "$FAKE_HOME/login-item.log" ]; then
    record_check login-item-wait yes "no login-item.log $elapsed s after the new version served"
  elif [ -e "$FAKE_HOME/login-item.log" ]; then
    record_check login-item-wait no "login-item.log already held $(cat "$FAKE_HOME/login-item.log") $elapsed s after the new version served"
  else
    record_check login-item-wait no "$elapsed s after the new version served, past the 55 s the reconciler waits"
  fi

  start=$SECONDS
  while [ $((SECONDS - start)) -lt 90 ]; do
    now="$(now_ms)"
    [ $(( (now - t) / 1000 )) -ge 75 ] && break
    sleep 1
  done
  elapsed=$(( ($(now_ms) - t) / 1000 ))
  if [ "$(cat "$FAKE_HOME/login-item.log" 2>/dev/null)" = "unregister" ] &&
     [ "$(wc -l <"$FAKE_HOME/login-item.log" 2>/dev/null | tr -d ' ')" = "1" ]; then
    record_check login-item-runs yes "one unregister $elapsed s after the new version served"
  else
    record_check login-item-runs no "login-item.log is '$(cat "$FAKE_HOME/login-item.log" 2>/dev/null || echo absent)' $elapsed s after the new version served"
  fi
}

run_quit_checks() {
  local phase version app_pids host_pids done_ms now

  if [ "$PREPARED" != "yes" ]; then
    record_check prepared no "not exercised: Sparkle never prepared $NEW_VERSION within 120 s"
    CHECKS_RAN=yes
    return 0
  fi
  record_check prepared yes

  phase="$(state_phase)"
  if [ "$phase" = "done" ]; then
    record_check done yes
  else
    record_check done no "state.json phase is ${phase:-absent}"
  fi

  version="$(plist_key "$OLD/Contents/Info.plist" CFBundleShortVersionString)"
  if [ "$version" = "$NEW_VERSION" ]; then
    record_check bundle yes "$version"
  else
    record_check bundle no "$OLD carries ${version:-no version}, wanted $NEW_VERSION"
  fi

  done_ms="$(cat "$LOGS/done.ms" 2>/dev/null)"
  if [ -n "$done_ms" ]; then
    while [ $(( $(now_ms) - done_ms )) -lt 15000 ]; do
      sleep 0.5
    done
  fi
  app_pids="$(pids_comm "$OLD/Contents/MacOS/htmlclay")"
  host_pids="$(pids_comm "$OLD/Contents/MacOS/htmlclay-host")"
  if [ -z "${app_pids// /}" ] && [ -z "${host_pids// /}" ]; then
    record_check no-process yes
  else
    record_check no-process no "htmlclay=${app_pids:-none} htmlclay-host=${host_pids:-none} 15 s after done"
  fi

  CHECKS_RAN=yes
}

run_rollback_checks() {
  local phase failed_version version app_pids host_pids pid listen info bad changed rolled

  phase="$(state_phase)"
  failed_version="$(state_field failed.version)"
  if [ "$phase" = "rolledBack" ] && [ "$failed_version" = "$NEW_VERSION" ]; then
    record_check rolled-back yes "$phase, $failed_version failed"
  else
    record_check rolled-back no "state.json phase is ${phase:-absent}, failed.version is ${failed_version:-absent}"
  fi

  version="$(plist_key "$OLD/Contents/Info.plist" CFBundleShortVersionString)"
  if [ "$version" = "$OLD_VERSION" ]; then
    record_check bundle yes "$version"
  else
    record_check bundle no "$OLD carries ${version:-no version}, wanted $OLD_VERSION"
  fi

  app_pids=""
  for pid in $(pids_comm "$OLD/Contents/MacOS/htmlclay"); do
    [ "$pid" = "$APP_PID" ] && continue
    app_pids="${app_pids:+$app_pids }$pid"
  done
  listen=""
  host_pids="$(pids_comm "$OLD/Contents/MacOS/htmlclay-host")"
  for pid in $host_pids; do
    listen="$(listen_port "$pid")"
    if [ -n "$PORT_BEFORE" ] && [ "$listen" = "$PORT_BEFORE" ]; then
      host_pids="$pid"
      break
    fi
    listen=""
  done
  [ -n "$listen" ] || host_pids=""
  info="$(host_info_version)"
  if [ -n "$app_pids" ] && [ -n "$host_pids" ] && [ "$info" = "$OLD_VERSION" ]; then
    record_check old-app yes "htmlclay=$app_pids htmlclay-host=$host_pids listens on $listen, host update-info $info"
  else
    record_check old-app no "htmlclay=${app_pids:-none} htmlclay-host=${host_pids:-none} listens on ${listen:-nothing}, host update-info ${info:-?}; the old app was on ${PORT_BEFORE:-unknown}"
  fi

  changed="$(cat "$LOGS/version-changed.ms" 2>/dev/null)"
  rolled="$(cat "$LOGS/rolled-back.ms" 2>/dev/null)"
  if [ -z "$changed" ] || [ -z "$rolled" ]; then
    record_check no-failed-host no "version-changed.ms ${changed:-absent}, rolled-back.ms ${rolled:-absent}"
  else
    bad=""
    for pid in $(awk -v a="$changed" -v b="$rolled" '$1 > a && $1 < b { print $2 }' "$LOGS/hosts.txt" 2>/dev/null | sort -u); do
      alive "$pid" && bad="${bad:+$bad }$pid"
    done
    if [ -z "$bad" ]; then
      record_check no-failed-host yes "no host sampled during the failed update is running"
    else
      record_check no-failed-host no "still running: $bad"
    fi
  fi

  CHECKS_RAN=yes
}

run_checks() {
  case "$CASE" in
    next) run_next_checks ;;
    next-quit) run_quit_checks ;;
    next-rollback) run_rollback_checks ;;
  esac
  check_ledger
}

# --- the run ----------------------------------------------------------------

say "launching the old bundle"
env HTMLCLAY_CANDIDATE_HOME="$FAKE_HOME" HTMLCLAY_CONFIG_BASE="$FAKE_HOME/Library/Application Support" \
  HTMLCLAY_OPEN_LOG="$OPEN_LOG" "$OLD/Contents/MacOS/htmlclay" >"$LOGS/old-app.out" 2>&1 &
APP_PID=$!
say "the old bundle runs as $APP_PID"

if ! wait_gate; then
  say "the old app never reached running with its gate on and Sparkle started"
  record_check gate no "$(cat "$FAKE_HOME/candidate-state.json" 2>/dev/null || echo 'no candidate-state.json')"
  CHECKS_RAN=yes
  exit 1
fi

PORT_BEFORE="$(plutil -extract port raw -o - "$CONFIG/config.json" 2>/dev/null)"
say "the old app's port: ${PORT_BEFORE:-unknown}"

if ! open_loose_document; then
  say "the old app never opened the loose file"
  record_check open-loose no "no c.htmlclay in $OPEN_LOG; open log: $(cat "$OPEN_LOG" 2>/dev/null || echo empty)"
  CHECKS_RAN=yes
  exit 1
fi

if [ -n "$SERVED" ] && ! serve_docs; then
  CHECKS_RAN=yes
  exit 1
fi

start_ledger "$LEDGER_MODE"
watch_timing &
WATCH_PID=$!
start_state_sampler
start_host_sampler

case "$CASE" in
  next|next-rollback)
    start_canary
    if [ "$CASE" = "next" ]; then
      wait_phase_done || say "state.json never reached done within $DEADLINE s"
    else
      wait_rolled_back || say "state.json never rolled back within $DEADLINE s"
    fi
    wait_host_port || say "no htmlclay-host on ${PORT_BEFORE:-unknown} within 60 s"
    sleep 5
    stop_ledger
    ;;
  next-quit)
    if wait_prepared; then
      PREPARED=yes
      osascript -e "tell application \"$OLD\" to quit" >>"$LOGS/quit-osascript.log" 2>&1
      say "sent the quit Apple event"
      wait_phase_done || say "state.json never reached done within $DEADLINE s"
    else
      PREPARED=no
      say "Sparkle never prepared $NEW_VERSION within 120 s"
    fi
    stop_ledger
    ;;
esac

run_checks
stop_old_run
say "the run took $((SECONDS - STARTED)) s"
exit 0
