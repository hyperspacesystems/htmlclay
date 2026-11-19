#!/bin/bash
set -euo pipefail

UNIVERSAL=false
HARDENED=false
while [ $# -gt 0 ]; do
  case "$1" in
    --universal) UNIVERSAL=true; shift ;;
    --hardened) HARDENED=true; shift ;;
    *) break ;;
  esac
done
[ $# -eq 3 ] || { echo "usage: verify-bundle.sh [--universal] [--hardened] <app> <version> <bundle id>" >&2; exit 1; }
APP="$1"
VERSION="$2"
ID="$3"
PLIST="$APP/Contents/Info.plist"
PRODUCT_ID=com.htmlclay.HTMLClay
fail() { echo "BUNDLE FAILED: $*" >&2; exit 1; }

[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "version $VERSION is not X.Y.Z"
if [ "$ID" = "$PRODUCT_ID" ] && [ "${HTMLCLAY_RELEASE_BUILD:-}" != 1 ]; then
  fail "the product id needs HTMLCLAY_RELEASE_BUILD=1"
fi

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
for program in htmlclay htmlclay-host; do
  for arch in $(lipo -archs "$APP/Contents/MacOS/$program"); do
    minos="$(vtool -arch "$arch" -show-build "$APP/Contents/MacOS/$program" | awk '$1 == "minos" { print $2; exit }')"
    [ "$minos" = "14.0" ] || fail "$program ($arch) needs macOS ${minos:-unknown}, but Info.plist promises 14.0"
  done
done
for key in LSUIElement SUFeedURL SUPublicEDKey SUEnableAutomaticChecks SUAutomaticallyUpdate SUAllowsAutomaticUpdates SUScheduledCheckInterval SUVerifyUpdateBeforeExtraction SURequireSignedFeed HCUpdateNowCommand; do
  plutil -extract "$key" raw "$PLIST" >/dev/null 2>&1 || fail "Info.plist has no $key"
done
[ "$(plutil -extract SUScheduledCheckInterval raw "$PLIST")" = "21600" ] || fail "SUScheduledCheckInterval is not 21600"
[ -f "$APP/Contents/Resources/$(plutil -extract CFBundleIconFile raw "$PLIST").icns" ] || fail "the icon named by CFBundleIconFile is not in Resources"
if [ "$ID" != "$PRODUCT_ID" ] && plutil -extract CFBundleDocumentTypes raw "$PLIST" >/dev/null 2>&1; then
  fail "a test id ($ID) claims document types"
fi

for path in "$APP/Contents/MacOS/htmlclay" "$APP/Contents/MacOS/htmlclay-host" "$APP"; do
  ents="$(codesign -d --entitlements - --xml "$path" 2>/dev/null || true)"
  case "$ents" in
    *get-task-allow*) fail "$path carries get-task-allow" ;;
  esac
done

for helper in "$APP/Contents/Frameworks/Sparkle.framework/Versions/B/Autoupdate" "$APP/Contents/Frameworks/Sparkle.framework/Versions/B/Updater.app"; do
  codesign --verify --strict "$helper" 2>/dev/null || fail "$helper is not signed"
done

if [ "$HARDENED" = true ]; then
  while IFS= read -r file; do
    case "$(file -b "$file")" in
      Mach-O*) ;;
      *) continue ;;
    esac
    dv="$(codesign -dv "$file" 2>&1 || true)"
    case "$dv" in
      *flags=*runtime*) ;;
      *) fail "$file has no hardened runtime" ;;
    esac
  done < <(find "$APP/Contents" -type f)
  appdv="$(codesign -dv "$APP" 2>&1 || true)"
  case "$appdv" in
    *Signature=adhoc*) ;;
    *)
      for path in "$APP/Contents/MacOS/htmlclay" "$APP/Contents/MacOS/htmlclay-host"; do
        ents="$(codesign -d --entitlements - --xml "$path" 2>/dev/null || true)"
        case "$ents" in
          *"<key>"*) fail "$path has entitlements" ;;
        esac
      done
      ;;
  esac
fi

if [ "$ID" = "$PRODUCT_ID" ]; then
  echo "skipping update-info for the product id"
else
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
fi

codesign --verify --deep --strict "$APP" || fail "codesign --verify --deep --strict"
for part in XPCServices Headers PrivateHeaders Modules; do
  [ ! -e "$APP/Contents/Frameworks/Sparkle.framework/Versions/B/$part" ] || fail "Sparkle.framework still has $part"
done
echo "BUNDLE OK $APP"
