#!/bin/bash
# notarize.sh [--dry-run] <dmg> <app>: notarize the signed DMG, staple it and the
# app inside it, and zip the app for Sparkle. Needs APPLE_ID, APPLE_TEAM_ID and
# APPLE_APP_SPECIFIC_PASSWORD. --dry-run prints each command with the credentials
# left as variable names, and runs nothing.
set -euo pipefail

DRY=false
if [ "${1:-}" = "--dry-run" ]; then
  DRY=true
  shift
fi
[ $# -eq 2 ] || { echo "usage: notarize.sh [--dry-run] <dmg> <app>" >&2; exit 1; }
DMG_NAME="$1"
APP="$2"
UPDATE_ZIP="${DMG_NAME%.dmg}.app.zip"

if [ "$DRY" = true ]; then
  echo "xcrun notarytool submit $DMG_NAME --apple-id \$APPLE_ID --team-id \$APPLE_TEAM_ID --password \$APPLE_APP_SPECIFIC_PASSWORD --wait --output-format json"
  echo "xcrun stapler staple $DMG_NAME"
  echo "xcrun stapler staple $APP"
  echo "ditto -c -k --sequesterRsrc --keepParent $APP $UPDATE_ZIP"
  exit 0
fi

echo "Submitting $DMG_NAME for notarization..."
SUBMIT_OUTPUT=$(xcrun notarytool submit "$DMG_NAME" \
  --apple-id "$APPLE_ID" \
  --team-id "$APPLE_TEAM_ID" \
  --password "$APPLE_APP_SPECIFIC_PASSWORD" \
  --wait \
  --output-format json)

STATUS=$(echo "$SUBMIT_OUTPUT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','Unknown'))")

if [ "$STATUS" != "Accepted" ]; then
  echo "Notarization failed with status: $STATUS"
  echo "$SUBMIT_OUTPUT"
  exit 1
fi

# Stapling fetches the ticket from Apple's CDN, which can drop the connection
# (stapler error 68) or lag a moment behind an accepted notarization. Retry
# before failing the build.
for attempt in 1 2 3 4 5; do
  if xcrun stapler staple "$DMG_NAME"; then
    break
  fi
  if [ "$attempt" = 5 ]; then
    echo "Could not staple $DMG_NAME after 5 attempts"
    exit 1
  fi
  sleep 15
done
echo "Notarized and stapled $DMG_NAME"

# The DMG's ticket covers the app inside it, so the bare app staples from it
# with no second submission. Stapled before it is zipped, so a Sparkle update
# opens offline too. The ticket can take a moment to reach the CDN; an app that
# still will not staple ships unstapled, and Gatekeeper checks it online.
for attempt in 1 2 3 4 5; do
  if xcrun stapler staple "$APP"; then
    break
  fi
  if [ "$attempt" = 5 ]; then
    echo "::warning::Could not staple $APP; the update archive ships without a ticket"
    break
  fi
  sleep 15
done

# The archive Sparkle installs. ditto keeps the bundle's symlinks, extended
# attributes and signature intact, which zip does not.
rm -f "$UPDATE_ZIP"
ditto -c -k --sequesterRsrc --keepParent "$APP" "$UPDATE_ZIP"
echo "Created $UPDATE_ZIP"
