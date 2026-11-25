#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PUBLIC="$(cd "$ROOT/../htmlclay" && pwd)"
git -C "$ROOT" fetch -q "$PUBLIC" main
base="$(git -C "$ROOT" rev-parse --short FETCH_HEAD)"
git -C "$ROOT" merge --no-edit -m "Merge public htmlclay main ($base) into the private fork" FETCH_HEAD
echo "PUBLIC BASE $base"
