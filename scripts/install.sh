#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD="${PREVIEW_BUILD_DIR:-$HOME/scratch/htmlclay-preview-cache}"
DEST="${PREVIEW_INSTALL_DIR:-$HOME/Applications}"
APP="$DEST/HTML Clay Preview.app"
STAGE="$(mktemp -d)/HTML Clay Preview.app"

xcrun swift build -c release --package-path "$ROOT" --scratch-path "$BUILD"
BIN="$(swift build -c release --package-path "$ROOT" --scratch-path "$BUILD" --show-bin-path)/HTMLClayPreview"

mkdir -p "$STAGE/Contents/MacOS" "$STAGE/Contents/Resources"
cp "$BIN" "$STAGE/Contents/MacOS/HTMLClayPreview"
cp "$ROOT/Resources/Info.plist" "$STAGE/Contents/Info.plist"
find "$ROOT/Resources" -type f ! -name Info.plist -exec cp {} "$STAGE/Contents/Resources/" \;
plutil -lint "$STAGE/Contents/Info.plist" >/dev/null
for key in CFBundleDocumentTypes CFBundleURLTypes UTExportedTypeDeclarations; do
  if /usr/libexec/PlistBuddy -c "Print :$key" "$STAGE/Contents/Info.plist" >/dev/null 2>&1; then
    echo "INSTALL FAILED: the preview must not claim file types or URL schemes ($key)" >&2
    exit 1
  fi
done
ID="com.htmlclay.HTMLClayPreview"
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$STAGE/Contents/Info.plist")" = "$ID" ] || { echo "INSTALL FAILED: wrong bundle id" >&2; exit 1; }
if [ -d "$APP" ] && [ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$APP/Contents/Info.plist" 2>/dev/null)" != "$ID" ]; then
  echo "INSTALL FAILED: $APP is not the preview" >&2
  exit 1
fi
codesign --force --sign - "$STAGE"
codesign --verify --strict "$STAGE"
"$STAGE/Contents/MacOS/HTMLClayPreview" --self-check

osascript -e "tell application id \"$ID\" to quit" >/dev/null 2>&1 || true
for _ in $(seq 1 20); do pgrep -f "$APP/Contents/MacOS/HTMLClayPreview" >/dev/null || break; sleep 0.25; done
mkdir -p "$DEST"
if [ -d "$APP" ]; then mkdir -p "$HOME/scratch/htmlclay-preview-old"; mv "$APP" "$HOME/scratch/htmlclay-preview-old/$(date +%Y%m%d-%H%M%S).app"; fi
mv "$STAGE" "$APP"
echo "INSTALL OK $APP"
