#!/bin/bash
set -euo pipefail

UNIVERSAL=false
if [ "${1:-}" = "--universal" ]; then
  UNIVERSAL=true
  shift
fi
[ $# -eq 3 ] || { echo "usage: verify-bundle.sh [--universal] <app> <version> <bundle id>" >&2; exit 1; }
APP="$1"
VERSION="$2"
ID="$3"
PLIST="$APP/Contents/Info.plist"
fail() { echo "BUNDLE FAILED: $*" >&2; exit 1; }

[ -x "$APP/Contents/MacOS/htmlclay" ] || fail "no Contents/MacOS/htmlclay"
[ -x "$APP/Contents/MacOS/htmlclay-host" ] || fail "no Contents/MacOS/htmlclay-host"
if [ "$UNIVERSAL" = true ]; then
  for program in htmlclay htmlclay-host; do
    archs="$(lipo -archs "$APP/Contents/MacOS/$program")"
    case " $archs " in *" x86_64 "*) ;; *) fail "$program has no x86_64 slice ($archs)" ;; esac
    case " $archs " in *" arm64 "*) ;; *) fail "$program has no arm64 slice ($archs)" ;; esac
  done
fi
[ "$(plutil -extract CFBundleIdentifier raw "$PLIST")" = "$ID" ] || fail "CFBundleIdentifier is not $ID"
[ "$(plutil -extract CFBundleExecutable raw "$PLIST")" = "htmlclay" ] || fail "CFBundleExecutable is not htmlclay"
[ "$(plutil -extract CFBundleVersion raw "$PLIST")" = "$VERSION" ] || fail "CFBundleVersion is not $VERSION"
[ "$(plutil -extract CFBundleShortVersionString raw "$PLIST")" = "$VERSION" ] || fail "CFBundleShortVersionString is not $VERSION"
[ "$(plutil -extract LSMinimumSystemVersion raw "$PLIST")" = "14.0" ] || fail "LSMinimumSystemVersion is not 14.0"
[ -f "$APP/Contents/Resources/$(plutil -extract CFBundleIconFile raw "$PLIST").icns" ] || fail "the icon named by CFBundleIconFile is not in Resources"
if [ "$ID" != "com.htmlclay.HTMLClay" ] && plutil -extract CFBundleDocumentTypes raw "$PLIST" >/dev/null 2>&1; then
  fail "a test id ($ID) claims document types"
fi

info="$(mktemp)"
"$APP/Contents/MacOS/htmlclay" update-info > "$info" 2>&1 &
pid=$!
for _ in $(seq 1 50); do
  kill -0 "$pid" 2>/dev/null || break
  sleep 0.1
done
if kill -0 "$pid" 2>/dev/null; then
  kill -9 "$pid" 2>/dev/null || true
  fail "update-info did not answer within 5 s"
fi
python3 - "$info" "$VERSION" <<'PY' || fail "update-info: $(cat "$info")"
import json, sys
got = json.load(open(sys.argv[1]))
want = {"version": sys.argv[2], "service": True, "tailwind": True}
bad = {k: got.get(k) for k in want if got.get(k) != want[k]}
sys.exit(1 if bad else 0)
PY
rm -f "$info"

codesign --verify --deep --strict "$APP" || fail "codesign --verify --deep --strict"
for part in XPCServices Headers PrivateHeaders Modules; do
  [ ! -e "$APP/Contents/Frameworks/Sparkle.framework/Versions/B/$part" ] || fail "Sparkle.framework still has $part"
done
echo "BUNDLE OK $APP"
