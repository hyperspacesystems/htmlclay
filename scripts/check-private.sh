#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if git -C "$ROOT" remote -v | /usr/bin/grep -qi 'hyperspacesystems/htmlclay'; then
  echo "PRIVATE CHECK FAILED: a remote names the public htmlclay repo" >&2
  git -C "$ROOT" remote -v >&2
  exit 1
fi
url="$(git -C "$ROOT" remote get-url origin 2>/dev/null || true)"
case "$url" in
  *panphora/htmlclay-mac*) echo "PRIVATE CHECK OK ($url)" ;;
  *) echo "PRIVATE CHECK FAILED: origin is '$url', expected panphora/htmlclay-mac" >&2; exit 1 ;;
esac
