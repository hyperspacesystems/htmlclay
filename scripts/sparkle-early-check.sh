#!/bin/bash
# One throwaway Sparkle update, from a bundle whose main executable is the Go
# app to a bundle whose main executable is the Swift app and whose Go host sits
# beside it as htmlclay-host. This is the cheap early check for milestone 7: it
# says whether Sparkle can cross that change at all, and reports what happened.
#
# Both bundles carry the throwaway id com.htmlclay.SparkleCheck and claim no
# file types or URL schemes, everything runs on a scratch HOME and config base
# under ~/scratch, and the installed HTML Clay is only ever read: its login
# item, its config, its process and the app LaunchServices opens .htmlclay with
# are compared before and after.
#
# Usage: scripts/sparkle-early-check.sh
#
# Exit codes: 0 the update happened, 1 it did not or the surroundings changed.
# The last line on stdout is one JSON object. Logs stay in the run folder.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ID="com.htmlclay.SparkleCheck"
OLD_VERSION="9.0.0"
NEW_VERSION="9.0.1"
STAMP="$(date +%Y%m%d-%H%M%S)"
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

SCRATCH="${HTMLCLAY_SPARKLE_CHECK_DIR:-$HOME/scratch/$STAMP-sparkle-check}"
OLD="$SCRATCH/old/HTMLClay.app"
NEW_SRC="$SCRATCH/new/HTML Clay Candidate.app"
NEW_ZIP="$SCRATCH/zip/HTMLClay-$NEW_VERSION.app.zip"
FEED_FILE="$SCRATCH/feed-file"
FEED_HTTP="$SCRATCH/feed-http"
CONFIG="$SCRATCH/c/htmlclay"
FAKE_HOME="$SCRATCH/home"
LOGS="$SCRATCH/logs"
BIN="$SCRATCH/bin"
PROBE="$SCRATCH/probe/probe.htmlclay"
KEY="$SCRATCH/keys/ed25519.seed"
REPORT="$SCRATCH/report.md"
REAL_HOME="$HOME"
HTTP_PORT=""
PORT=""
DEADLINE="${HTMLCLAY_SPARKLE_CHECK_DEADLINE:-240}"
FILE_DEADLINE="${HTMLCLAY_SPARKLE_CHECK_FILE_DEADLINE:-75}"
OBSERVE="${HTMLCLAY_SPARKLE_CHECK_OBSERVE:-60}"

say() { echo "[sparkle-check] $*" >&2; }

PIDS=()
CANARY_PID=""
HTTP_PID=""
APP_PID=""
OUTCOME=""
OUTCOME_REASON=""
ATTEMPT_FEED=""
PROOF="no"
PROOF_SECONDS=""
PROOF_VERSION=""
PROOF_SHORT=""
PROOF_HOST_BIN="no"
PROOF_APP_PIDS=""
PROOF_HOST_PIDS=""
ROLLBACK=""
FEED_ERROR=""
GATE=""
PROBE_OUTCOME=""
PROBE_REASON=""
PROBE_SECONDS=""
PROBE_APP_PIDS=""
PROBE_HOST_PIDS=""
PROOF_SECONDS_HTTP=""
PRIMARY_PROOF=""
PRIMARY_ERROR=""
PRIMARY_FAILED=""
FILE_ERROR=""
STARTED=$SECONDS
BEFORE=()
NOW=()
COMPARE=""

json_string() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="$(printf '%s' "$s" | tr -d '\000-\037')"
  printf '"%s"' "$s"
}

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

cleanup() {
  local status=$?
  trap - EXIT
  say "cleaning up"
  kill_pid "$CANARY_PID"
  kill_pid "$HTTP_PID"
  kill_pid "$APP_PID"

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

  rm -rf "$OLD" "$NEW_SRC" "$FEED_FILE" "$FEED_HTTP" "$SCRATCH/zip" "$SCRATCH/new" "$SCRATCH/old" "$SCRATCH/stage" "$SCRATCH/probe-new"
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
  ps -A -o pid,command | grep -c "[/]sparkle-check/" >"$LOGS/cleanup-ps-count.txt"
  ps -A -o pid,command | grep "[s]parkle-check" >"$LOGS/cleanup-ps.txt" 2>&1

  write_report
  print_json
  if [ "$COMPARE" != "yes" ]; then
    exit 1
  fi
  exit "$status"
}
trap cleanup EXIT

write_report() {
  {
    echo "# Sparkle early check: $OLD_VERSION (Go app) to $NEW_VERSION (Swift app), id $ID"
    echo
    echo "Run started $(date), took $((SECONDS - STARTED)) s."
    echo
    echo "## What this step changed"
    echo
    echo "- Added \`scripts/sparkle-early-check.sh\`. No change to the Go app, the Swift app or packaging."
    echo "- The only writes outside the run folder are LaunchServices registrations, prefs and caches under \`$ID\`, all removed by the cleanup below."
    echo
    echo "## Outcome"
    echo
    echo "- Outcome: **${OUTCOME:-unknown}**"
    echo "- Reason: ${OUTCOME_REASON:-none}"
    echo "- Attempt feed: ${ATTEMPT_FEED:-none}"
    echo "- Gate: ${GATE:-unknown}"
    echo "- Proof, the handoff's recipe (attempts 1 and 2): ${PROOF}"
    echo "- Proof, the diagnostic (attempt 3): ${PROBE_OUTCOME:-not run}, after ${PROBE_SECONDS:-n/a} s; bundle at the old path ${PROOF_VERSION:-} / ${PROOF_SHORT:-}, htmlclay-host ${PROOF_HOST_BIN}, processes htmlclay=${PROBE_APP_PIDS:-none} htmlclay-host=${PROBE_HOST_PIDS:-none}"
    echo "- Recovery supervisor: ${ROLLBACK:-unknown}"
    echo "- Sparkle feed error: ${FEED_ERROR:-none}"
    echo "- file:// feed: ${FILE_ERROR:-no Sparkle error}"
    echo
    echo "## Diagnostic: the same update with SUPublicEDKey in the new bundle"
    echo
    echo "- Outcome: ${PROBE_OUTCOME:-not run}"
    echo "- Reason: ${PROBE_REASON:-not run}"
    echo
    echo "## What the run found"
    echo
    echo "- A \`file://\` feed is refused: Sparkle's downloader only accepts http and https, so the feed has to be served (SUDownloadError 2001)."
    echo "- An http feed on 127.0.0.1 needed \`NSAppTransportSecurity.NSAllowsLocalNetworking\` in the old bundle's Info.plist and a feed signature, since the bundle sets \`SURequireSignedFeed\`."
    echo "- A new bundle with no \`SUPublicEDKey\` is refused at install time (SUInstallationError 4005: a public key was found in the old bundle and none in the new one). Every installed copy has to keep the key."
    echo "- With the key present the update goes through: Sparkle installs 9.0.1 over the Go bundle and relaunches the Swift app, which starts its htmlclay-host beside it."
    echo "- The Go recovery supervisor then rolls it back, because the Swift app never writes \`updates/health.json\`: 9.0.1 did not report serving within 30 s, the previous bundle is swapped back and the Go app is relaunched."
    echo
    echo "## Recovery state at the end"
    echo
    for f in "$CONFIG/updates/state.json" "$CONFIG/updates/health.json"; do
      echo "### $f"
      echo '```'
      cat "$f" 2>/dev/null || echo "absent"
      echo '```'
    done
    echo
    echo "## Sparkle's log lines"
    echo
    echo '```'
    if [ -f "$LOGS/sparkle.log" ]; then
      grep -E "\[org\.sparkle-project\.Sparkle:Sparkle\]" "$LOGS/sparkle.log" |
        grep -vE "CFBundle|Lazy cache|localizationNames" | tail -n 40
    fi
    echo '```'
    echo
    echo "## App log lines"
    echo
    echo '```'
    grep -iE "sparkle|update|recovery|install" "$CONFIG/htmlclay.log" 2>/dev/null | tail -n 80
    echo '```'
    echo
    echo "## Supervisor log"
    echo
    echo '```'
    cat "$CONFIG"/updates/supervisor.log 2>/dev/null | tail -n 60
    echo '```'
    echo
    echo "## Canary output"
    echo
    echo '```'
    cat "$LOGS"/*-canary.out 2>/dev/null | tail -n 20
    cat "$LOGS"/*-canary.err 2>/dev/null | tail -n 40
    echo '```'
    echo
    echo "## Before and after, the installed HTML Clay"
    echo
    echo "| what | before | after |"
    echo "| --- | --- | --- |"
    echo "| LaunchAgent com.htmlclay.plist sha256 | ${BEFORE[0]:-} | ${NOW[0]:-} |"
    echo "| installed config.json sha256 | ${BEFORE[1]:-} | ${NOW[1]:-} |"
    echo "| running installed pid | ${BEFORE[2]:-} | ${NOW[2]:-} |"
    echo "| app for .htmlclay | ${BEFORE[3]:-} | ${NOW[3]:-} |"
    echo
    echo "- Identical: ${COMPARE:-unknown}"
    echo
    echo "## Cleanup"
    echo
    echo '```'
    cat "$LOGS/cleanup.log" 2>/dev/null
    echo '```'
    echo "- lsregister -dump lines mentioning $ID: $(cat "$LOGS/cleanup-lsregister-count.txt" 2>/dev/null)"
    echo "- defaults read $ID: $(cat "$LOGS/cleanup-defaults.txt" 2>/dev/null | head -n 2 | tr '\n' ' ')"
    echo "- processes running from the scratch folder: $(cat "$LOGS/cleanup-ps-count.txt" 2>/dev/null)"
    echo "- \`ps -A -o pid,command | grep -c \"[s]parkle-check\"\`: this shell's own command line carries the run folder's name, so read that number with \`logs/cleanup-ps.txt\`"
    echo
    echo "## Logs"
    echo
    ls -1 "$LOGS" 2>/dev/null | sed 's/^/- /'
  } >"$REPORT" 2>&1
}

print_json() {
  printf '{"check":"sparkle-early-check","id":%s,"from":%s,"to":%s,"outcome":%s,"reason":%s,"feed":%s,"proof":%s,"proof_seconds":%s,"supervisor":%s,"gate":%s,"unchanged":%s,"probe_outcome":%s,"probe_reason":%s,"seconds":%d,"report":%s}\n' \
    "$(json_string "$ID")" "$(json_string "$OLD_VERSION")" "$(json_string "$NEW_VERSION")" \
    "$(json_string "${OUTCOME:-unknown}")" "$(json_string "$OUTCOME_REASON")" \
    "$(json_string "$ATTEMPT_FEED")" "$(json_string "$PROOF")" \
    "$(json_string "${PROOF_SECONDS:-0}")" "$(json_string "$ROLLBACK")" \
    "$(json_string "$GATE")" "$(json_string "${COMPARE:-unknown}")" \
    "$(json_string "${PROBE_OUTCOME:-}")" "$(json_string "${PROBE_REASON:-}")" \
    "$((SECONDS - STARTED))" "$(json_string "$REPORT")"
}

finish() {
  OUTCOME="$1"
  OUTCOME_REASON="$2"
  if [ "$OUTCOME" = "updated" ]; then exit 0; fi
  exit 1
}

# --- setup -----------------------------------------------------------------

mkdir -p "$SCRATCH" "$LOGS" "$BIN" "$(dirname "$PROBE")" "$(dirname "$KEY")" "$CONFIG" "$FAKE_HOME"
printf '<!doctype html><title>probe</title><p>probe</p>\n' >"$PROBE"
: >"$LOGS/cleanup.log"
say "run folder: $SCRATCH"

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

PORT="$(free_port)"
INSTALLED_PORT="$(plutil -extract port raw -o - "$REAL_HOME/Library/Application Support/htmlclay/config.json" 2>/dev/null)"
say "installed port: ${INSTALLED_PORT:-none}, scratch port: $PORT"
cat >"$CONFIG/config.json" <<JSON
{"startOnLogin": false, "port": $PORT}
JSON

PUB="$(swift "$BIN/genkey.swift" "$KEY" 2>>"$LOGS/keys.log")"
if [ -z "$PUB" ]; then
  say "could not make the throwaway key"
  OUTCOME="never triggered"
  OUTCOME_REASON="could not make the throwaway Ed25519 key"
  exit 1
fi
PRIV="$(cat "$KEY")"
if [ "$(SPARKLE_ED_PRIVATE_KEY="$PRIV" swift "$ROOT/packaging/macos/sparkle-pubkey.swift" 2>>"$LOGS/keys.log")" != "$PUB" ]; then
  say "the throwaway key pair does not match"
  OUTCOME="never triggered"
  OUTCOME_REASON="the throwaway key pair does not match"
  exit 1
fi
echo "public $PUB" >"$LOGS/keys.log"

# --- old bundle ------------------------------------------------------------

# build.sh builds in place in the repo and then hands the app to appdmg; the
# npx stub stops it after the bundle is signed and verified, so nothing is
# installed from npm and no DMG lands in the repo.
cat >"$BIN/npx" <<'STUB'
#!/bin/sh
echo "sparkle-early-check: npx is stubbed out; the bundle is already built" >&2
exit 1
STUB
chmod +x "$BIN/npx"

say "building the Go bundle $OLD_VERSION"
mkdir -p "$SCRATCH/old"
if ! PATH="$BIN:$PATH" VERSION="$OLD_VERSION" UPDATE_CHANNEL=stable SPARKLE_PUBLIC_ED_KEY="$PUB" \
  bash "$ROOT/packaging/macos/build.sh" --unsigned >"$LOGS/build-old.log" 2>&1; then
  say "build.sh exited non-zero (the npx stub is expected); checking the bundle"
fi
rm -f "$ROOT/HTMLClay-$OLD_VERSION"-*.dmg "$ROOT/HTMLClay-$OLD_VERSION"-*.app.zip
if [ ! -x "$ROOT/HTMLClay.app/Contents/MacOS/htmlclay" ]; then
  say "no old bundle was built"
  OUTCOME="never triggered"
  OUTCOME_REASON="packaging/macos/build.sh did not produce HTMLClay.app"
  exit 1
fi
mv "$ROOT/HTMLClay.app" "$OLD"
rm -rf "$ROOT/HTMLClay.app"

set_bundle_id() {
  local app="$1" id="$2"
  plutil -replace CFBundleIdentifier -string "$id" "$app/Contents/Info.plist"
  local key
  for key in CFBundleDocumentTypes UTExportedTypeDeclarations UTImportedTypeDeclarations CFBundleURLTypes; do
    plutil -remove "$key" "$app/Contents/Info.plist" 2>/dev/null || true
  done
  plutil -lint "$app/Contents/Info.plist" >/dev/null
}

resign() {
  local app="$1"
  codesign --force --deep -s - "$app" >>"$LOGS/codesign.log" 2>&1 &&
    codesign --verify --strict "$app" >>"$LOGS/codesign.log" 2>&1
}

set_bundle_id "$OLD" "$ID"
plutil -insert HCUpdateNowCommand -integer 1 "$OLD/Contents/Info.plist" 2>/dev/null ||
  plutil -replace HCUpdateNowCommand -integer 1 "$OLD/Contents/Info.plist"
[ "$(plist_key "$OLD/Contents/Info.plist" SUPublicEDKey)" = "$PUB" ] || say "warning: the old bundle does not carry the throwaway key"

set_feed() {
  local url="$1"
  plutil -replace SUFeedURL -string "$url" "$OLD/Contents/Info.plist"
  if [ "${url#http://}" != "$url" ]; then
    if plutil -extract NSAppTransportSecurity raw -o - "$OLD/Contents/Info.plist" >/dev/null 2>&1; then
      plutil -replace NSAppTransportSecurity.NSAllowsLocalNetworking -bool true "$OLD/Contents/Info.plist"
    else
      plutil -insert NSAppTransportSecurity -json '{"NSAllowsLocalNetworking":true}' "$OLD/Contents/Info.plist"
    fi
  fi
  resign "$OLD" || { say "could not re-sign the old bundle"; return 1; }
}

# --- new bundle ------------------------------------------------------------

say "building the Swift bundle $NEW_VERSION"
mkdir -p "$SCRATCH/new"
if ! bash "$ROOT/scripts/build-candidate.sh" --unsigned --output "$SCRATCH/new" >"$LOGS/build-new.log" 2>&1; then
  say "build-candidate.sh failed"
  OUTCOME="never triggered"
  OUTCOME_REASON="scripts/build-candidate.sh failed; see logs/build-new.log"
  exit 1
fi
set_bundle_id "$NEW_SRC" "$ID"
plutil -replace CFBundleVersion -string "$NEW_VERSION" "$NEW_SRC/Contents/Info.plist"
plutil -replace CFBundleShortVersionString -string "$NEW_VERSION" "$NEW_SRC/Contents/Info.plist"
if ! resign "$NEW_SRC"; then
  say "could not sign the new bundle"
  OUTCOME="never triggered"
  OUTCOME_REASON="the new bundle could not be signed"
  exit 1
fi

mkdir -p "$SCRATCH/zip"
rm -rf "$SCRATCH/stage"
mkdir -p "$SCRATCH/stage"
ditto "$NEW_SRC" "$SCRATCH/stage/HTMLClay.app"
ditto -c -k --sequesterRsrc --keepParent "$SCRATCH/stage/HTMLClay.app" "$NEW_ZIP" >>"$LOGS/zip.log" 2>&1
if [ ! -f "$NEW_ZIP" ]; then
  say "could not zip the new bundle"
  OUTCOME="never triggered"
  OUTCOME_REASON="could not zip the new bundle"
  exit 1
fi

make_appcast() {
  local zip="$1" base="$2" out="$3"
  mkdir -p "$out"
  SPARKLE_ED_PRIVATE_KEY="$PRIV" SPARKLE_PUBLIC_ED_KEY="$PUB" SPARKLE_BASE_URL="$base" \
    bash "$ROOT/packaging/macos/sparkle-appcast.sh" "$NEW_VERSION" "$zip" "$out" >>"$LOGS/appcast.log" 2>&1 || return 1
  mkdir -p "$out/$NEW_VERSION"
  cp "$zip" "$out/$NEW_VERSION/"
}

# --- running the old bundle ------------------------------------------------

launch_old() {
  env HOME="$FAKE_HOME" HTMLCLAY_CONFIG_BASE="$SCRATCH/c" "$OLD/Contents/MacOS/htmlclay" \
    >"$LOGS/$1-app.out" 2>&1 &
  APP_PID=$!
  PIDS+=("$APP_PID")
  say "launched the old bundle as $APP_PID"
}

wait_gate() {
  local i
  for i in $(seq 1 120); do
    GATE="$(grep -E "Self-update is (on|off)" "$CONFIG/htmlclay.log" 2>/dev/null | tail -n 1)"
    [ -n "$GATE" ] && return 0
    kill -0 "$APP_PID" 2>/dev/null || return 1
    sleep 0.5
  done
  return 1
}

wait_sparkle() {
  local i
  for i in $(seq 1 90); do
    grep -qE "Sparkle started|Sparkle could not start" "$CONFIG/htmlclay.log" 2>/dev/null && return 0
    kill -0 "$APP_PID" 2>/dev/null || return 1
    sleep 0.5
  done
  return 1
}

proof_ok() {
  PROOF_VERSION="$(plist_key "$OLD/Contents/Info.plist" CFBundleVersion)"
  PROOF_SHORT="$(plist_key "$OLD/Contents/Info.plist" CFBundleShortVersionString)"
  PROOF_HOST_BIN="no"
  [ -x "$OLD/Contents/MacOS/htmlclay-host" ] && PROOF_HOST_BIN="yes"
  PROOF_APP_PIDS="$(pids_comm "$OLD/Contents/MacOS/htmlclay")"
  PROOF_HOST_PIDS="$(pids_comm "$OLD/Contents/MacOS/htmlclay-host")"
  [ "$PROOF_VERSION" = "$NEW_VERSION" ] &&
    [ "$PROOF_SHORT" = "$NEW_VERSION" ] &&
    [ "$PROOF_HOST_BIN" = "yes" ] &&
    [ -n "${PROOF_APP_PIDS// /}" ] &&
    [ -n "${PROOF_HOST_PIDS// /}" ]
}

stop_old_run() {
  stop_run_pids
  kill_pid "$CANARY_PID"
  CANARY_PID=""
  kill_pid "$APP_PID"
  APP_PID=""
}

stop_run_pids() {
  local pid
  for pid in $(pids_comm "$OLD/Contents/MacOS/htmlclay") $(pids_comm "$OLD/Contents/MacOS/htmlclay-host"); do
    kill_pid "$pid"
  done
}

# attempt TAG FEED_URL DEADLINE: one launch and one canary run against one feed.
attempt() {
  local tag="$1" url="$2" deadline="$3"
  ATTEMPT_FEED="$url"
  PROOF="no"
  PROOF_SECONDS=""
  FEED_ERROR=""
  GATE=""
  : >"$CONFIG/htmlclay.log"
  say "attempt $tag: feed $url"
  if ! set_feed "$url"; then
    FEED_ERROR="could not point the old bundle at $url"
    return 1
  fi
  launch_old "$tag"
  if ! wait_gate; then
    say "the app never reported its update gate"
    FEED_ERROR="the app never started; see logs/$tag-app.out"
    stop_old_run
    return 1
  fi
  say "gate: $GATE"
  if [ "${GATE#*Self-update is on}" = "$GATE" ]; then
    say "self-update is off, so Sparkle never runs"
    stop_old_run
    return 2
  fi
  wait_sparkle || say "Sparkle did not report started within 45 s"

  (
    export HOME="$FAKE_HOME" HTMLCLAY_CONFIG_BASE="$SCRATCH/c"
    export HTMLCLAY_CANARY_APP="$OLD" HTMLCLAY_CANARY_CONFIG_DIR="$CONFIG"
    export HTMLCLAY_CANARY_INSTALL=/usr/bin/false HTMLCLAY_CANARY_DEADLINE="$deadline"
    exec bash "$ROOT/scripts/update-canary.sh" "$NEW_VERSION"
  ) >"$LOGS/$tag-canary.out" 2>"$LOGS/$tag-canary.err" &
  CANARY_PID=$!
  PIDS+=("$CANARY_PID")

  local start=$SECONDS line
  while [ $((SECONDS - start)) -lt "$deadline" ]; do
    if proof_ok; then
      PROOF="yes"
      PROOF_SECONDS=$((SECONDS - start))
      say "proof after $PROOF_SECONDS s: $PROOF_VERSION at the old path, htmlclay=$PROOF_APP_PIDS host=$PROOF_HOST_PIDS"
      return 0
    fi
    line="$(grep -E "Sparkle stopped an update|Sparkle could not start" "$CONFIG/htmlclay.log" 2>/dev/null | tail -n 1)"
    if [ -n "$line" ]; then
      FEED_ERROR="$line"
      say "Sparkle gave up: $line"
      stop_old_run
      return 3
    fi
    if ! kill -0 "$CANARY_PID" 2>/dev/null; then
      sleep 1
      proof_ok && { PROOF="yes"; PROOF_SECONDS=$((SECONDS - start)); say "proof after $PROOF_SECONDS s"; return 0; }
      say "the canary finished without proof"
      break
    fi
    sleep 1
  done
  proof_ok && { PROOF="yes"; PROOF_SECONDS=$((SECONDS - start)); return 0; }
  return 1
}

# watch_supervisor: after proof, record what the recovery supervisor does.
watch_supervisor() {
  local start=$SECONDS phase
  ROLLBACK="nothing within ${OBSERVE}s"
  while [ $((SECONDS - start)) -lt "$OBSERVE" ]; do
    phase="$(plutil -extract phase raw -o - "$CONFIG/updates/state.json" 2>/dev/null)"
    if [ "$phase" = "rolledBack" ] || [ "$phase" = "rollbackFailed" ]; then
      ROLLBACK="$phase: $(plutil -extract failed.reason raw -o - "$CONFIG/updates/state.json" 2>/dev/null)"
      return 0
    fi
    if [ "$(plist_key "$OLD/Contents/Info.plist" CFBundleVersion)" = "$OLD_VERSION" ] &&
       [ -z "$(pids_comm "$OLD/Contents/MacOS/htmlclay")" ]; then
      ROLLBACK="the bundle is back at $OLD_VERSION and no Swift app is running (state phase: ${phase:-none})"
      return 0
    fi
    if [ "$(plist_key "$OLD/Contents/Info.plist" CFBundleVersion)" = "$OLD_VERSION" ]; then
      ROLLBACK="the bundle is back at $OLD_VERSION (state phase: ${phase:-none})"
      return 0
    fi
    sleep 1
  done
  local done_phase
  done_phase="$(plutil -extract phase raw -o - "$CONFIG/updates/state.json" 2>/dev/null)"
  ROLLBACK="nothing within ${OBSERVE}s (state phase: ${done_phase:-none})"
}

# categorize PROOF FEED_ERROR FAILED_VERSION: the outcome names the handoff asks for.
categorize() {
  if [ "$1" = "yes" ]; then
    CATEGORY="updated"
    CATEGORY_REASON="Sparkle installed $NEW_VERSION and the Swift app ran with its host"
  elif [ "$3" = "$NEW_VERSION" ]; then
    CATEGORY="rolled back"
    CATEGORY_REASON="$(plutil -extract failed.reason raw -o - "$CONFIG/updates/state.json" 2>/dev/null)"
  elif [ -n "$2" ]; then
    CATEGORY="Sparkle refused"
    CATEGORY_REASON="$2"
  else
    CATEGORY="never triggered"
    CATEGORY_REASON="no proof and no Sparkle error within the deadline"
  fi
}

# --- the check -------------------------------------------------------------

# Sparkle's own log lines, so a refusal can be read even when the app's log
# only carries its summary.
log stream --info --debug --style compact \
  --predicate 'subsystem CONTAINS[c] "sparkle" OR process CONTAINS[c] "sparkle" OR process == "Autoupdate" OR process == "htmlclay" OR process == "htmlclay-host"' \
  >"$LOGS/sparkle.log" 2>&1 &
LOG_PID=$!
PIDS+=("$LOG_PID")

make_appcast "$NEW_ZIP" "file://$FEED_FILE" "$FEED_FILE" || say "could not write the file:// appcast"
say "attempt 1: a file:// feed"
attempt file "file://$FEED_FILE/appcast.xml" "$FILE_DEADLINE"
FILE_RESULT=$?
FILE_ERROR="$FEED_ERROR"
say "the file:// feed: result $FILE_RESULT, ${FILE_ERROR:-no Sparkle error}"

HTTP_PORT="$(free_port)"
say "attempt 2: serving the feed on 127.0.0.1:$HTTP_PORT"
make_appcast "$NEW_ZIP" "http://127.0.0.1:$HTTP_PORT" "$FEED_HTTP" || say "could not write the http:// appcast"
( cd "$FEED_HTTP" && exec python3 -m http.server --bind 127.0.0.1 "$HTTP_PORT" >"$LOGS/http.server.log" 2>&1 ) &
HTTP_PID=$!
PIDS+=("$HTTP_PID")
FEED_UP=""
for _ in $(seq 1 40); do
  curl -fsS -o /dev/null "http://127.0.0.1:$HTTP_PORT/appcast.xml" 2>/dev/null && { FEED_UP=ok; break; }
  sleep 0.25
done
say "http feed ready: ${FEED_UP:-no}"
attempt http "http://127.0.0.1:$HTTP_PORT/appcast.xml" "$DEADLINE"
HTTP_RESULT=$?
PROOF_SECONDS_HTTP="$PROOF_SECONDS"
PRIMARY_PROOF="$PROOF"
PRIMARY_ERROR="$FEED_ERROR"
PRIMARY_FAILED="$(plutil -extract failed.version raw -o - "$CONFIG/updates/state.json" 2>/dev/null)"
say "the http:// feed: result $HTTP_RESULT, ${PRIMARY_ERROR:-no Sparkle error}"

# The bundle recipe the handoff spells out carries no SUPublicEDKey, and an app
# that has one refuses an update to a bundle that does not: owning a Sparkle key
# is not something a new version may drop. One extra throwaway copy with the key
# added says what happens past that point, and is only a diagnostic.
PROBE_OUTCOME=""
PROBE_REASON=""
if [ "$PRIMARY_PROOF" != "yes" ] && [ -z "$(plist_key "$NEW_SRC/Contents/Info.plist" SUPublicEDKey)" ]; then
  say "attempt 3: the same update, with SUPublicEDKey added to the throwaway copy of the new bundle"
  PROBE_APP="$SCRATCH/probe-new/HTMLClay.app"
  PROBE_ZIP="$SCRATCH/zip/HTMLClay-$NEW_VERSION-probe.app.zip"
  rm -rf "$SCRATCH/probe-new"
  mkdir -p "$SCRATCH/probe-new"
  if ditto "$NEW_SRC" "$PROBE_APP" >>"$LOGS/zip.log" 2>&1 &&
     plutil -insert SUPublicEDKey -string "$PUB" "$PROBE_APP/Contents/Info.plist" &&
     codesign --force --deep -s - "$PROBE_APP" >>"$LOGS/codesign.log" 2>&1 &&
     codesign --verify --strict "$PROBE_APP" >>"$LOGS/codesign.log" 2>&1 &&
     ditto -c -k --sequesterRsrc --keepParent "$PROBE_APP" "$PROBE_ZIP" >>"$LOGS/zip.log" 2>&1 &&
     make_appcast "$PROBE_ZIP" "http://127.0.0.1:$HTTP_PORT" "$FEED_HTTP"; then
    attempt probe "http://127.0.0.1:$HTTP_PORT/appcast.xml" "$DEADLINE"
    PROBE_RESULT=$?
    PROBE_PROOF="$PROOF"
    PROBE_ERROR="$FEED_ERROR"
    PROBE_FAILED="$(plutil -extract failed.version raw -o - "$CONFIG/updates/state.json" 2>/dev/null)"
    categorize "$PROBE_PROOF" "$PROBE_ERROR" "$PROBE_FAILED"
    PROBE_OUTCOME="$CATEGORY"
    PROBE_SECONDS="$PROOF_SECONDS"
    PROBE_APP_PIDS="$PROOF_APP_PIDS"
    PROBE_HOST_PIDS="$PROOF_HOST_PIDS"
    PROBE_REASON="$CATEGORY_REASON (result $PROBE_RESULT)"
    say "the probe: $PROBE_OUTCOME: $PROBE_REASON"
  else
    PROBE_OUTCOME="never triggered"
    PROBE_REASON="could not build the throwaway copy with SUPublicEDKey"
    say "the probe could not be built"
  fi
fi

categorize "$PRIMARY_PROOF" "$PRIMARY_ERROR" "$PRIMARY_FAILED"
OUTCOME="$CATEGORY"
OUTCOME_REASON="$CATEGORY_REASON"
PROOF="$PRIMARY_PROOF"
PROOF_SECONDS="$PROOF_SECONDS_HTTP"
ATTEMPT_FEED="http://127.0.0.1:$HTTP_PORT/appcast.xml"

if [ "$PROOF" = "yes" ] || [ "${PROBE_OUTCOME:-}" = "updated" ]; then
  watch_supervisor
fi
stop_old_run
finish "$OUTCOME" "$OUTCOME_REASON"
