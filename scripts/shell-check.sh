#!/bin/bash
# A throwaway HTML Clay to drive the Mac shell with: the menu bar icon, the
# panel, Settings and Make default, on this Mac, while the installed HTML Clay
# is only ever read.
#
# The bundle carries the throwaway id com.htmlclay.ShellCheck and claims one
# throwaway file type, .htmlclaycheck (com.htmlclay.check), which it names in
# HTMLClayDocumentType, so its Make default and Stop opening act on that type
# and never on .htmlclay. It runs on a scratch home and config base, with Open
# at Login off and an update feed nobody serves. Its program is renamed
# htmlclay-shellcheck, so System Events, which finds a process by name, can
# never reach the installed HTML Clay's htmlclay process instead.
#
# `start` builds and launches it and prints the run folder. Drive it, then
# `stop <run folder>` quits it, unregisters it, removes its defaults and
# compares the installed HTML Clay's login item, config, process and the app
# that opens .htmlclay with what `start` recorded.
#
# Usage: scripts/shell-check.sh start | opener <run folder> | stop <run folder>
#
# `opener` prints the app that opens .htmlclaycheck files now.
#
# Exit codes: 0 ok, 1 a step or the comparison failed, 2 usage.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ID="com.htmlclay.ShellCheck"
TYPE_ID="com.htmlclay.check"
case "com.htmlclay.HTMLClay" in
  "$ID"*) echo "[shell-check] refusing bundle id '$ID'" >&2; exit 2 ;;
esac
SWIFT_CACHE="${HTMLCLAY_SWIFT_CACHE:-$HOME/scratch/20261115-100943-macos-packaging/swift-cache}"
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
REAL_HOME="$HOME"

say() { echo "[shell-check] $*" >&2; }
usage() { say "usage: shell-check.sh start | opener <run folder> | stop <run folder>"; exit 2; }
fingerprint() { if [ -e "$1" ]; then shasum -a 256 "$1" | cut -d' ' -f1; else echo absent; fi; }

paths() {
  DIR="$1"
  FAKE_HOME="$DIR/home"
  CONFIG="$FAKE_HOME/Library/Application Support/htmlclay"
  LOGS="$DIR/logs"
  BIN="$DIR/bin"
  OUT="$DIR/app.noindex"
  APP="$OUT/HTMLClay.app"
  PROBE="$DIR/probe/probe.htmlclay"
  CHECK_FILE="$FAKE_HOME/loose/check.htmlclaycheck"
}

state() {
  fingerprint "$REAL_HOME/Library/LaunchAgents/com.htmlclay.plist"
  fingerprint "$REAL_HOME/Library/Application Support/htmlclay/config.json"
  pgrep -f '/Applications/HTMLClay.app/Contents/MacOS/htmlclay' | sort | tr '\n' ' '; echo
  swift "$BIN/opener.swift" "$PROBE" 2>/dev/null
}

free_port() {
  python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()'
}

start() {
  paths "$HOME/scratch/shell-check-$(date '+%Y%m%d-%H%M%S')"
  mkdir -p "$LOGS" "$BIN" "$OUT" "$(dirname "$PROBE")" "$(dirname "$CHECK_FILE")" "$CONFIG" "$FAKE_HOME/docs"
  cat >"$BIN/opener.swift" <<'SWIFT'
import AppKit
import Foundation
let url = URL(fileURLWithPath: CommandLine.arguments[1])
print(NSWorkspace.shared.urlForApplication(toOpen: url)?.path ?? "none")
SWIFT
  printf '<!doctype html><title>probe</title>\n' >"$PROBE"
  printf '<!doctype html><title>check</title><p>check</p>\n' >"$CHECK_FILE"
  printf '<!doctype html><title>doc</title><p contenteditable>doc</p>\n' >"$FAKE_HOME/docs/doc.htmlclay"
  state >"$DIR/before.txt"
  say "the installed HTML Clay before: $(tr '\n' '|' <"$DIR/before.txt")"

  local port
  port="$(free_port)"
  cat >"$CONFIG/config.json" <<JSON
{"startOnLogin": false, "port": $port, "workspaceFolders": [{"path": "$FAKE_HOME/docs"}]}
JSON

  cat >"$BIN/genkey.swift" <<'SWIFT'
import CryptoKit
print(Curve25519.Signing.PrivateKey().publicKey.rawRepresentation.base64EncodedString())
SWIFT
  local pub
  pub="$(swift "$BIN/genkey.swift" 2>>"$LOGS/keys.log")"
  [ -n "$pub" ] || { say "could not make a throwaway update key"; exit 1; }

  say "building $ID into $OUT"
  if ! BUNDLE_ID="$ID" VERSION="9.2.0" UPDATE_CHANNEL=stable SPARKLE_PUBLIC_ED_KEY="$pub" SPARKLE_FEED_URL="http://127.0.0.1:9/appcast.xml" \
      OUT_DIR="$OUT" HTMLCLAY_SWIFT_CACHE="$SWIFT_CACHE" \
      bash "$ROOT/packaging/macos/build.sh" --adhoc-hardened --no-dmg >"$LOGS/build.log" 2>&1; then
    say "the build failed; see $LOGS/build.log"
    exit 1
  fi

  local plist="$APP/Contents/Info.plist" key
  plutil -replace CFBundleIdentifier -string "$ID" "$plist"
  mv "$APP/Contents/MacOS/htmlclay" "$APP/Contents/MacOS/htmlclay-shellcheck"
  plutil -replace CFBundleExecutable -string "htmlclay-shellcheck" "$plist"
  for key in CFBundleDocumentTypes UTExportedTypeDeclarations UTImportedTypeDeclarations CFBundleURLTypes; do
    plutil -remove "$key" "$plist" 2>/dev/null || true
  done
  plutil -insert HTMLClayDocumentType -string "$TYPE_ID" "$plist"
  plutil -insert UTExportedTypeDeclarations -json "[{\"UTTypeIdentifier\":\"$TYPE_ID\",\"UTTypeDescription\":\"HTML Clay shell check\",\"UTTypeConformsTo\":[\"public.html\"],\"UTTypeTagSpecification\":{\"public.filename-extension\":[\"htmlclaycheck\"]}}]" "$plist"
  plutil -insert CFBundleDocumentTypes -json "[{\"CFBundleTypeName\":\"HTML Clay shell check\",\"CFBundleTypeRole\":\"Editor\",\"LSHandlerRank\":\"Alternate\",\"LSItemContentTypes\":[\"$TYPE_ID\"]}]" "$plist"
  plutil -insert LSEnvironment -json "{\"HTMLCLAY_CANDIDATE_HOME\":\"$FAKE_HOME\",\"HTMLCLAY_CONFIG_BASE\":\"$FAKE_HOME/Library/Application Support\"}" "$plist" 2>/dev/null ||
    plutil -replace LSEnvironment -json "{\"HTMLCLAY_CANDIDATE_HOME\":\"$FAKE_HOME\",\"HTMLCLAY_CONFIG_BASE\":\"$FAKE_HOME/Library/Application Support\"}" "$plist"
  plutil -lint "$plist" >/dev/null || { say "the Info.plist does not lint"; exit 1; }
  if ! { codesign --force --deep -s - "$APP" && codesign --verify --strict "$APP"; } >>"$LOGS/codesign.log" 2>&1; then
    say "re-signing failed; see $LOGS/codesign.log"
    exit 1
  fi
  defaults write "$ID" SUEnableAutomaticChecks -bool false
  "$LSREGISTER" -f "$APP" >>"$LOGS/lsregister.log" 2>&1

  open -g -a "$APP" >>"$LOGS/open.log" 2>&1
  local i
  for i in $(seq 1 60); do
    pgrep -f "$APP/Contents/MacOS/htmlclay" >/dev/null && break
    sleep 0.5
  done
  if ! pgrep -f "$APP/Contents/MacOS/htmlclay" >/dev/null; then
    say "the app did not start; see $LOGS"
    exit 1
  fi
  say "running: $(pgrep -f "$APP/Contents/MacOS/htmlclay" | tr '\n' ' ')on port $port"
  say ".htmlclaycheck opens with: $(swift "$BIN/opener.swift" "$CHECK_FILE" 2>/dev/null)"
  echo "$DIR"
}

opener() {
  paths "$1"
  [ -d "$DIR" ] || usage
  swift "$BIN/opener.swift" "$CHECK_FILE" 2>/dev/null
}

stop() {
  paths "$1"
  [ -f "$DIR/before.txt" ] || usage
  local pid i
  osascript -e "tell application \"$APP\" to quit" >>"$LOGS/quit.log" 2>&1
  for i in $(seq 1 40); do
    pgrep -f "$APP/Contents/MacOS/" >/dev/null || break
    sleep 0.25
  done
  for pid in $(pgrep -f "$APP/Contents/MacOS/"); do
    say "stopping leftover process $pid"
    kill -TERM "$pid" 2>/dev/null
  done
  "$LSREGISTER" -u "$APP" >>"$LOGS/lsregister.log" 2>&1
  defaults delete "$ID" >>"$LOGS/cleanup.log" 2>&1
  local dir path
  for dir in "$REAL_HOME/Library/Caches" "$REAL_HOME/Library/Application Support" "$REAL_HOME/Library/Preferences" "$REAL_HOME/Library/Saved Application State" "$REAL_HOME/Library/HTTPStorages" "$REAL_HOME/Library/WebKit"; do
    for path in "$dir"/*"$ID"*; do
      [ -e "$path" ] || continue
      echo "removing $path" >>"$LOGS/cleanup.log"
      rm -rf "$path"
    done
  done
  state >"$DIR/after.txt"
  if cmp -s "$DIR/before.txt" "$DIR/after.txt"; then
    say "the installed HTML Clay is unchanged"
    echo "SHELL CHECK CLEAN"
    return 0
  fi
  say "the installed HTML Clay changed:"
  diff "$DIR/before.txt" "$DIR/after.txt" >&2
  echo "SHELL CHECK CHANGED THE INSTALLED APP"
  return 1
}

case "${1:-}" in
  start) [ $# -eq 1 ] || usage; start ;;
  opener) [ $# -eq 2 ] || usage; opener "$2" ;;
  stop) [ $# -eq 2 ] || usage; stop "$2" ;;
  *) usage ;;
esac
