#!/bin/bash
# Launches the candidate bundle on a throwaway home and proves it works on the
# real Go host without touching the installed HTML Clay: it starts and reports a
# real snapshot, a file opened with the app reaches the host's file.open (logged,
# never opened in a browser), Quit stops the app and its host, and the installed
# app's login item, config and running process are unchanged.
set -euo pipefail

APP="${1:-$HOME/scratch/htmlclay-mac-candidate/HTML Clay Candidate.app}"
ID="com.htmlclay.HTMLClayCandidate"
LOGIN_ITEM="$HOME/Library/LaunchAgents/com.htmlclay.plist"
CONFIG="$HOME/Library/Application Support/htmlclay/config.json"

fail() { echo "PROOF FAILED: $*" >&2; exit 1; }
fingerprint() { if [ -e "$1" ]; then shasum -a 256 "$1" | cut -d' ' -f1; stat -f '%m' "$1"; else echo missing; fi; }
wait_for() {
  local seconds="$1"; shift
  for _ in $(seq 1 $((seconds * 4))); do "$@" && return 0; sleep 0.25; done
  return 1
}

[ -d "$APP" ] || fail "no candidate at $APP (run scripts/build-candidate.sh --unsigned --output <dir>)"
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$APP/Contents/Info.plist")" = "$ID" ] || fail "$APP is not the candidate"
pgrep -f "$APP/Contents/MacOS/htmlclay" >/dev/null && fail "a candidate is already running; quit it first"

login_before="$(fingerprint "$LOGIN_ITEM")"
config_before="$(fingerprint "$CONFIG")"
installed_before="$(pgrep -f '/Applications/HTMLClay.app/Contents/MacOS/htmlclay' | sort | tr '\n' ' ')"

home="$(mktemp -d /tmp/hc-XXXXXXXX)"
home="$(cd "$home" && pwd -P)"
opened="$home/opened.log"
state="$home/candidate-state.json"
mkdir -p "$home/notes"
printf '<!doctype html><title>a</title><p>a</p>\n' > "$home/notes/a.htmlclay"
cleanup() {
  osascript -e "tell application id \"$ID\" to quit" >/dev/null 2>&1 || true
  pkill -f "$APP/Contents/MacOS/htmlclay" >/dev/null 2>&1 || true
  rm -rf "$home"
}
trap cleanup EXIT

open -n --env "HTMLCLAY_CANDIDATE_HOME=$home" --env "HTMLCLAY_OPEN_LOG=$opened" "$APP" --args --no-welcome

running() { [ -f "$state" ] && grep -q '"state" *: *"running"' "$state" && grep -q '"epoch" *: *"[0-9a-f]' "$state"; }
wait_for 30 running || fail "the candidate never reported a running host: $(cat "$state" 2>/dev/null || echo 'no state file')"
echo "running: $(cat "$state")"

host_pid="$(pgrep -f "$APP/Contents/MacOS/htmlclay-host" | head -1)"
[ -n "$host_pid" ] || fail "no host process inside the candidate bundle"
host_home="$(ps -E -p "$host_pid" -o command= | tr ' ' '\n' | sed -n 's/^HOME=//p' | head -1)"
[ -z "$host_home" ] || [ "$host_home" = "$home" ] || fail "the host runs with HOME=$host_home, not the throwaway home"

open -a "$APP" "$home/notes/a.htmlclay"
logged() { [ -f "$opened" ] && grep -q 'a\.htmlclay' "$opened"; }
wait_for 15 logged || fail "opening a.htmlclay never reached the host's file.open (open log: $(cat "$opened" 2>/dev/null || echo empty))"
echo "opened: $(cat "$opened")"

osascript -e "tell application id \"$ID\" to quit" >/dev/null
gone() { ! pgrep -f "$APP/Contents/MacOS/htmlclay" >/dev/null; }
wait_for 15 gone || fail "the candidate or its host is still running 15 s after Quit"
echo "quit: app and host exited"

[ "$(fingerprint "$LOGIN_ITEM")" = "$login_before" ] || fail "the installed app's login item changed"
[ "$(fingerprint "$CONFIG")" = "$config_before" ] || fail "the installed app's config changed"
[ "$(pgrep -f '/Applications/HTMLClay.app/Contents/MacOS/htmlclay' | sort | tr '\n' ' ')" = "$installed_before" ] || fail "the installed app's process changed"
echo "installed app: login item, config and process unchanged"
echo "PROOF OK"
