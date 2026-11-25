#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$ROOT_DIR"
source "$SCRIPT_DIR/sparkle-common.sh"

PRODUCT_ID="com.htmlclay.HTMLClay"
IDENTITY="Hyperspace Systems LLC (JC7YGGXYKH)"
UNSIGNED=false
UNIVERSAL=false
ADHOC_HARDENED=false
NO_DMG=false

for arg in "$@"; do
  case "$arg" in
    --unsigned) UNSIGNED=true ;;
    --universal) UNIVERSAL=true ;;
    --adhoc-hardened) ADHOC_HARDENED=true ;;
    --no-dmg) NO_DMG=true ;;
    *) echo "BUILD FAILED: unknown option $arg" >&2; exit 1 ;;
  esac
done

if [ -f .env ]; then
  set -a
  source .env
  set +a
fi

BUNDLE_ID="${BUNDLE_ID:-$PRODUCT_ID}"
RELEASE="${HTMLCLAY_RELEASE_BUILD:-}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR}"
SWIFT_CACHE="${HTMLCLAY_SWIFT_CACHE:-$HOME/Library/Caches/htmlclay-build/swift}"
VERSION="${VERSION:-$(grep 'var version' cmd/htmlclay/main.go | sed 's/.*"\(.*\)"/\1/')}"

[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "BUILD FAILED: VERSION must be X.Y.Z ($VERSION)" >&2; exit 1; }
if [ "$UNSIGNED" = true ] && [ "$ADHOC_HARDENED" = true ]; then
  echo "BUILD FAILED: --unsigned and --adhoc-hardened are two different signings; pick one" >&2
  exit 1
fi
SIGNED=true
if [ "$UNSIGNED" = true ] || [ "$ADHOC_HARDENED" = true ]; then
  SIGNED=false
fi
if [ "$RELEASE" != 1 ]; then
  if [ "$BUNDLE_ID" = "$PRODUCT_ID" ]; then
    echo "BUILD FAILED: $PRODUCT_ID is the installed app's id; set BUNDLE_ID to a test id, or HTMLCLAY_RELEASE_BUILD=1 for a release" >&2
    exit 1
  fi
  if [ "$SIGNED" = true ]; then
    echo "BUILD FAILED: signing with $IDENTITY needs HTMLCLAY_RELEASE_BUILD=1; pass --unsigned or --adhoc-hardened" >&2
    exit 1
  fi
  case "$OUT_DIR" in
    *.noindex|*.noindex/) ;;
    *) echo "BUILD FAILED: a test build goes to a folder ending .noindex ($OUT_DIR)" >&2; exit 1 ;;
  esac
fi
case "$OUT_DIR" in
  /Applications|/Applications/*) echo "BUILD FAILED: never build into /Applications ($OUT_DIR)" >&2; exit 1 ;;
esac
mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd -P)"

if [ "$UNIVERSAL" = true ]; then
  ARCH="universal"
  GOARCHES=(arm64 amd64)
else
  ARCH="$(uname -m)"
  [ "$ARCH" = "x86_64" ] && ARCH="amd64"
  GOARCHES=("$ARCH")
fi

APP="$OUT_DIR/HTMLClay.app"
CONTENTS="$APP/Contents"
MACOS="$CONTENTS/MacOS"
RESOURCES="$CONTENTS/Resources"
FRAMEWORKS="$CONTENTS/Frameworks"
SPARKLE_FW="$FRAMEWORKS/Sparkle.framework"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "Building HTMLClay v${VERSION} (${ARCH}) as ${BUNDLE_ID} into ${OUT_DIR}..."

rm -rf "$APP"
mkdir -p "$MACOS" "$RESOURCES" "$FRAMEWORKS"

LDFLAGS="-s -w -X main.version=${VERSION} -X main.updateChannel=${UPDATE_CHANNEL:-}"
HOSTS=()
APPS=()
for goarch in "${GOARCHES[@]}"; do
  swiftarch="$goarch"
  [ "$goarch" = amd64 ] && swiftarch=x86_64
  node service/build.mjs --goos darwin --goarch "$goarch"
  GOOS= GOARCH= go run ./cmd/fetchtailwind -goos darwin -goarch "$goarch"
  # The flags carry the target into Go's build cache key (MACOSX_DEPLOYMENT_TARGET alone is not
  # part of it), so cgo objects cached from a newer-SDK build are rebuilt for 14.0 too.
  MACOSX_DEPLOYMENT_TARGET=14.0 CGO_CFLAGS="-O2 -g -mmacosx-version-min=14.0" CGO_LDFLAGS="-mmacosx-version-min=14.0" CGO_ENABLED=1 GOOS=darwin GOARCH="$goarch" go build -tags "tailwind_bundle service_bundle" -trimpath -ldflags="$LDFLAGS" -o "$WORK/htmlclay-host-$goarch" ./cmd/htmlclay
  go version -m "$WORK/htmlclay-host-$goarch" | grep -q -- '-tags=tailwind_bundle,service_bundle' || { echo "BUILD FAILED: the $goarch host was built without the Tailwind and service bundles" >&2; exit 1; }
  xcrun swift build -c release --package-path macos --scratch-path "$SWIFT_CACHE/$swiftarch" --triple "$swiftarch-apple-macosx14.0" --product HTMLClay
  bin="$(xcrun swift build -c release --package-path macos --scratch-path "$SWIFT_CACHE/$swiftarch" --triple "$swiftarch-apple-macosx14.0" --show-bin-path)/HTMLClay"
  cp "$bin" "$WORK/htmlclay-app-$goarch"
  HOSTS+=("$WORK/htmlclay-host-$goarch")
  APPS+=("$WORK/htmlclay-app-$goarch")
done
lipo -create -output "$MACOS/htmlclay-host" "${HOSTS[@]}"
lipo -create -output "$MACOS/htmlclay" "${APPS[@]}"

PLIST="$CONTENTS/Info.plist"
cp macos/Resources/Product/Info.plist "$PLIST"
plutil -replace CFBundleIdentifier -string "$BUNDLE_ID" "$PLIST"
plutil -replace CFBundleVersion -string "$VERSION" "$PLIST"
plutil -replace CFBundleShortVersionString -string "$VERSION" "$PLIST"
if [ -n "${SPARKLE_PUBLIC_ED_KEY:-}" ]; then
  plutil -insert SUPublicEDKey -string "$SPARKLE_PUBLIC_ED_KEY" "$PLIST"
fi
if [ -n "${SPARKLE_FEED_URL:-}" ]; then
  plutil -replace SUFeedURL -string "$SPARKLE_FEED_URL" "$PLIST"
fi
if [ -n "${HC_LS_ENV:-}" ]; then
  plutil -insert LSEnvironment -json "$HC_LS_ENV" "$PLIST"
fi
if [ "$BUNDLE_ID" != "$PRODUCT_ID" ]; then
  for key in CFBundleDocumentTypes UTExportedTypeDeclarations UTImportedTypeDeclarations CFBundleURLTypes; do
    plutil -remove "$key" "$PLIST" 2>/dev/null || true
  done
  plutil -insert NSAppTransportSecurity -json '{"NSAllowsLocalNetworking":true}' "$PLIST"
fi
plutil -lint "$PLIST" >/dev/null

fetch_sparkle
ditto "$SPARKLE_CACHE/Sparkle.framework" "$SPARKLE_FW"
for part in XPCServices Headers PrivateHeaders Modules; do
  rm -rf "$SPARKLE_FW/$part" "$SPARKLE_FW/Versions/B/$part"
done

echo -n "APPL????" > "$CONTENTS/PkgInfo"
cp packaging/macos/doc.icns "$RESOURCES/"
find macos/Resources -type f ! -name Info.plist ! -name 'dashboard-preview*' ! -path '*/Candidate/*' ! -path '*/Product/*' -exec cp {} "$RESOURCES/" \;

if [ "$UNSIGNED" = true ]; then
  SIGN=(codesign -f -s -)
  SIGN_PROGRAM=("${SIGN[@]}")
elif [ "$ADHOC_HARDENED" = true ]; then
  SIGN=(codesign -f -s - --options runtime)
  SIGN_PROGRAM=("${SIGN[@]}" --entitlements packaging/macos/entitlements-adhoc.plist)
else
  SIGN=(codesign -f -s "$IDENTITY" --options runtime --timestamp)
  SIGN_PROGRAM=("${SIGN[@]}" --entitlements packaging/macos/entitlements.plist)
fi
"${SIGN[@]}" "$SPARKLE_FW/Versions/B/Autoupdate"
"${SIGN[@]}" "$SPARKLE_FW/Versions/B/Updater.app"
"${SIGN[@]}" "$SPARKLE_FW"
"${SIGN_PROGRAM[@]}" "$MACOS/htmlclay-host"
"${SIGN_PROGRAM[@]}" "$APP"
codesign -v --deep --strict "$APP"
echo "Signed $APP"

VERIFY_ARGS=()
[ "$UNIVERSAL" = true ] && VERIFY_ARGS+=(--universal)
if [ "$ADHOC_HARDENED" = true ] || [ "$SIGNED" = true ]; then
  VERIFY_ARGS+=(--hardened)
fi
bash "$SCRIPT_DIR/verify-bundle.sh" ${VERIFY_ARGS[@]+"${VERIFY_ARGS[@]}"} "$APP" "$VERSION" "$BUNDLE_ID"

if [ "$NO_DMG" = true ]; then
  exit 0
fi

DMG_NAME="$OUT_DIR/HTMLClay-${VERSION}-${ARCH}.dmg"
rm -f "$DMG_NAME"
python3 - "$SCRIPT_DIR/dmg/appdmg.json" "$WORK/appdmg.json" "$APP" "$SCRIPT_DIR/dmg" <<'PY'
import json, sys
spec = json.load(open(sys.argv[1]))
spec["background"] = sys.argv[4] + "/" + spec["background"]
for item in spec["contents"]:
    if item["type"] == "file":
        item["path"] = sys.argv[3]
json.dump(spec, open(sys.argv[2], "w"))
PY
npx --yes appdmg "$WORK/appdmg.json" "$DMG_NAME"
if [ "$SIGNED" = true ]; then
  codesign -f -s "$IDENTITY" "$DMG_NAME"
fi
echo "Created $DMG_NAME"

# Notarize
if [ "$SIGNED" = false ]; then
  echo "Skipping notarization (unsigned build)"
  exit 0
fi

if [ -z "${APPLE_ID:-}" ] || [ -z "${APPLE_TEAM_ID:-}" ] || [ -z "${APPLE_APP_SPECIFIC_PASSWORD:-}" ]; then
  echo "Skipping notarization (missing Apple credentials)"
  echo "Set APPLE_ID, APPLE_TEAM_ID, APPLE_APP_SPECIFIC_PASSWORD in .env"
  exit 0
fi

bash "$SCRIPT_DIR/notarize.sh" "$DMG_NAME" "$APP"
