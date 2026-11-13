#!/bin/bash
# Ships this checkout's committed HEAD plus hyperclay-local at a pinned tag to a test box and runs
# scripts/served-product-gate.sh there. Usage: served-product-box.sh [local-tag, default v1.37.0]
# Run it in a tmux session; the result lands in <scratch>/test-box-out/latest/trial/summary.log.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOCAL_REPO="$(cd "$ROOT/../hyperclay-local" && pwd)"
TAG=${1:-v1.37.0}
S="$HOME/scratch/$(date +%Y%m%d-%H%M%S)-served-product"
mkdir -p "$S/htmlclay" "$S/local"
git -C "$ROOT" archive HEAD | tar -x -C "$S/htmlclay"
git -C "$LOCAL_REPO" archive "$TAG" | tar -x -C "$S/local"
{
  echo "htmlclay-mac $(git -C "$ROOT" rev-parse HEAD) dirty=$(git -C "$ROOT" status --short | wc -l | tr -d ' ') (archive of HEAD)"
  echo "hyperclay-local $TAG $(git -C "$LOCAL_REPO" rev-parse "$TAG^{commit}")"
} > "$S/pins.txt"
cat > "$S/.testbox" <<'BOX'
TESTBOX_REPO=htmlclay-served-product
TESTBOX_SETUP='set -e
v=1.26.5 sum=5c2c3b16caefa1d968a94c1daca04a7ca301a496d9b086e17ad77bb81393f053
if [ "$(~/.local/go/bin/go env GOVERSION 2>/dev/null)" != "go$v" ]; then
  curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go$v.linux-amd64.tar.gz"
  echo "$sum  /tmp/go.tgz" | sha256sum -c -
  rm -rf ~/.local/go && mkdir -p ~/.local && tar -xzf /tmp/go.tgz -C ~/.local
fi
mkdir -p ~/.local/bin
ln -sf ~/.local/go/bin/go ~/.local/go/bin/gofmt ~/.local/bin/
if ! command -v rg >/dev/null; then
  curl -fsSL -o /tmp/rg.tgz https://github.com/BurntSushi/ripgrep/releases/download/14.1.1/ripgrep-14.1.1-x86_64-unknown-linux-musl.tar.gz
  tar -xzf /tmp/rg.tgz -C /tmp && install -m 755 /tmp/ripgrep-14.1.1-x86_64-unknown-linux-musl/rg ~/.local/bin/rg
fi
(cd htmlclay && npm ci --no-audit --no-fund)
(cd local && npm ci --no-audit --no-fund && ./node_modules/.bin/install-electron --no && node scripts/chrome-for-testing/browser-runtime.cjs install)'
TESTBOX_COMMAND='export PATH=$HOME/.local/bin:$PATH CGO_ENABLED=0 TZ=America/New_York HYPER_CHROME_SANDBOX=0 && umask 022 && bash htmlclay/scripts/served-product-gate.sh htmlclay local "$TESTBOX_OUT/trial"'
TESTBOX_TIMEOUT=50m
BOX
printf 'test-box-out/\n' > "$S/.gitignore"
git -C "$S" init -q && git -C "$S" add -A && git -C "$S" -c user.name=box -c user.email=box@localhost commit -qm "served product input"
echo "SCRATCH $S"
cd "$S"
testbox run
