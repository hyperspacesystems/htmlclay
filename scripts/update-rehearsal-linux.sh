#!/bin/bash
# One throwaway update rehearsal on Linux: the last public HTML Clay (1.x)
# updates itself to this checkout's build through its own updater, and the
# supervisor that judges the new version is the one 1.x shipped, unchanged.
#
# The old side is the public tag's source with one file replaced at build time
# (internal/update/keys.go, so it trusts a throwaway feed key). It runs with its
# tray on a private session bus, finds the update on a loopback HTTPS feed on
# its own 60 s check, and restarts when this script clicks its tray row
# "Restart to Update" over D-Bus, which is the only trigger 1.x has on Linux.
# Three documents are open and saving once a second the whole time
# (scripts/upgrade-ledger.mjs).
#
# Cases:
#   first             the plain update: the new version serves and the supervisor keeps it
#   rollback-version  the new build reports another version than the feed promised, so it
#                     runs and never counts as serving; after 20 s the supervisor kills it
#                     and puts 1.x back
#   rollback-early    the new "build" exits at once, and the supervisor puts 1.x back
#   next              2.0 to the version after it: both sides are this checkout, the old
#                     side serves a folder with its service running, and the update is
#                     started with `htmlclay update-now`, with no click
#   next-rollback     the same, and the new build reports another version than the feed
#                     promised: the supervisor puts the old 2.0 back, and its service
#                     runs again from the copy it unpacked before the update
#   next-start        2.0 to the version after it, installed at the next start: a browser
#                     tab presses Check for updates, the app downloads the update and
#                     keeps it waiting, the person quits, and starting HTML Clay again
#                     with a file installs the update and opens that file
#   next-button       the same as next, started by a browser tab pressing Check for
#                     updates and then the Update button, the way the dashboard does
#
# Usage: scripts/update-rehearsal-linux.sh first|rollback-version|rollback-early|next|next-rollback|next-start|next-button [--old-tag <tag>]
#
#   HTMLCLAY_REHEARSAL_OLD_SRC  a checkout or export of the old tag (default: clones the public tag)
#   HTMLCLAY_REHEARSAL_DIR      the run folder (default: a new folder under /tmp)
#
# Everything runs under a scratch HOME inside the run folder, with xdg-open and
# notify-send replaced by stubs. Nothing connects anywhere but the loopback feed
# (and, for the builds, the Go module proxy and GitHub for the public tag).
#
# Exit codes: 0 every check passed, 1 one of them did not, 2 usage or setup.
# The last line on stdout is "REHEARSAL OK <case> ..." or "REHEARSAL FAILED <case>: ...".
set -uo pipefail

say() { echo "[rehearsal] $*" >&2; }
usage() { say "usage: update-rehearsal-linux.sh first|rollback-version|rollback-early|next|next-rollback|next-start|next-button [--old-tag <tag>]"; exit 2; }

[ "$(uname -s)" = "Linux" ] || { say "this rehearsal runs on Linux only"; exit 2; }
if [ -z "${HTMLCLAY_REHEARSAL_BUS:-}" ]; then
  command -v dbus-run-session >/dev/null || { say "dbus-run-session is missing (apt-get install dbus)"; exit 2; }
  exec dbus-run-session -- env HTMLCLAY_REHEARSAL_BUS=1 bash "$0" "$@"
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CASE=""
OLD_TAG="v1.16.4"
OLD_VERSION="9.0.0"
NEW_VERSION="9.0.1"
PUBLIC_URL="https://github.com/panphora/htmlclay.git"

while [ $# -gt 0 ]; do
  case "$1" in
    first|rollback-version|rollback-early|next|next-rollback|next-start|next-button) [ -z "$CASE" ] || usage; CASE="$1"; shift ;;
    --old-tag) [ $# -ge 2 ] || usage; OLD_TAG="$2"; shift 2 ;;
    *) usage ;;
  esac
done
[ -n "$CASE" ] || usage
NEXT=""
case "$CASE" in next|next-rollback|next-start|next-button) NEXT=1; OLD_VERSION="9.0.1"; NEW_VERSION="9.0.2" ;; esac

for tool in go node git curl gzip sha256sum; do
  command -v "$tool" >/dev/null || { say "$tool is missing"; exit 2; }
done

LAB="${HTMLCLAY_REHEARSAL_DIR:-$(mktemp -d /tmp/htmlclay-rehearsal.XXXXXX)}"
LOGS="$LAB/logs"
FAKE_HOME="$LAB/home"
CONFIG="$FAKE_HOME/.config/htmlclay"
UPDATES="$CONFIG/updates"
INSTALL="$LAB/install/htmlclay"
PREVIOUS="$LAB/install/.htmlclay.prev"
ARCH="$(go env GOARCH)"
mkdir -p "$LAB/bin" "$LAB/keys" "$LAB/old" "$LAB/new" "$LAB/install" "$LAB/feed/update" "$LAB/stubs" "$LOGS" "$CONFIG" \
  "$FAKE_HOME/docs" "$FAKE_HOME/docs2" "$FAKE_HOME/loose"
say "case $CASE, run folder $LAB"

# The builds use the real HOME's Go caches; only the apps get the scratch HOME.
export GOCACHE="$(go env GOCACHE)" GOMODCACHE="$(go env GOMODCACHE)" GOPATH="$(go env GOPATH)"
export CGO_ENABLED=0

FEED_PID=""
LEDGER_PID=""
WATCH_PID=""
FAILED=()
FACTS=()
OLD_PID=""

now_ms() { date +%s%3N; }
fail() { FAILED+=("$*"); say "FAILED: $*"; }
pass() { say "ok: $*"; }
note() { FACTS+=("$*"); }

# jget FILE a.b.c prints one value of a JSON file, and fails when it is not there.
jget() {
  node -e '
    const fs = require("fs");
    try {
      let v = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
      for (const k of process.argv[2].split(".")) v = v == null ? undefined : v[k];
      if (v === undefined || v === null) process.exit(1);
      process.stdout.write(typeof v === "string" ? v : JSON.stringify(v));
    } catch { process.exit(1); }' "$1" "$2"
}

free_port() {
  node -e 'const s = require("net").createServer().listen(0, "127.0.0.1", () => { console.log(s.address().port); s.close(); });'
}

lock_pid() { head -n 1 "$CONFIG/lock" 2>/dev/null | tr -d '[:space:]'; }
alive() { [ -n "$1" ] && kill -0 "$1" 2>/dev/null; }
port_answers() { [ "$(curl -s -o /dev/null -m 2 -w '%{http_code}' "http://127.0.0.1:$PORT/" 2>/dev/null)" != "000" ]; }

keep_logs() {
  [ -n "${TESTBOX_OUT:-}" ] || return 0
  local out="$TESTBOX_OUT/update-rehearsal-$CASE"
  mkdir -p "$out"
  cp -r "$LOGS" "$out/" 2>/dev/null
  cp "$CONFIG/htmlclay.log" "$CONFIG/config.json" "$out/" 2>/dev/null
  cp -r "$UPDATES" "$out/updates" 2>/dev/null
  cp "$LAB/opened-urls" "$LAB/notifications" "$out/" 2>/dev/null
  return 0
}

cleanup() {
  local pid
  [ -n "$LEDGER_PID" ] && kill "$LEDGER_PID" 2>/dev/null
  [ -n "$WATCH_PID" ] && kill "$WATCH_PID" 2>/dev/null
  pid="$(lock_pid)"
  alive "$pid" && kill -TERM "$pid" 2>/dev/null
  sleep 1
  pkill -KILL -f "$LAB/install/" 2>/dev/null
  pkill -KILL -f "$CONFIG/service-runtime" 2>/dev/null
  [ -n "$FEED_PID" ] && kill "$FEED_PID" 2>/dev/null
  keep_logs
}
trap cleanup EXIT

die() { say "$*"; echo "REHEARSAL FAILED $CASE: setup: $*"; exit 2; }

finish() {
  if [ "${#FAILED[@]}" -eq 0 ]; then
    local facts
    facts="$(IFS=';'; echo "${FACTS[*]}")"
    echo "REHEARSAL OK $CASE:${facts//;/, }"
    exit 0
  fi
  local reasons
  reasons="$(IFS=';'; echo "${FAILED[*]}")"
  echo "REHEARSAL FAILED $CASE: ${reasons//;/; }"
  exit 1
}

# --- builds -------------------------------------------------------------------

say "building the helpers"
(cd "$ROOT" && go build -o "$LAB/bin/updaterehearsal" ./cmd/updaterehearsal && go build -o "$LAB/bin/mkfeed" ./cmd/mkfeed && go build -o "$LAB/bin/signfeed" ./cmd/signfeed) \
  >"$LOGS/build-helpers.log" 2>&1 || die "the helpers did not build, see $LOGS/build-helpers.log"
"$LAB/bin/updaterehearsal" keys -seed "$LAB/keys/seed" -go "$LAB/keys/keys.go" || die "could not make the throwaway feed key"

# The key goes in through -overlay, so no source tree is ever edited.
overlay() { printf '{"Replace": {"%s/internal/update/keys.go": "%s"}}\n' "$1" "$LAB/keys/keys.go"; }

BLOB="$ROOT/internal/service/blobs/linux-$ARCH"
if [ ! -s "$BLOB.tar.gz" ]; then
  say "building the service bundle"
  (cd "$ROOT" && node service/build.mjs --goos linux --goarch "$ARCH") >"$LOGS/build-service.log" 2>&1 || die "the service did not build, see $LOGS/build-service.log"
fi

if [ -n "$NEXT" ]; then
  # The old side is this checkout too. Its service bundle is the same files packed
  # again at another compression level, so the two versions unpack into two
  # folders, as two real releases do.
  say "building the old app (this checkout as $OLD_VERSION) with the service inside"
  gzip -dc "$BLOB.tar.gz" | gzip -1 -n >"$LAB/keys/old-service.tar.gz" || die "could not repack the service bundle"
  if cmp -s "$BLOB.tar.gz" "$LAB/keys/old-service.tar.gz"; then
    gzip -dc "$BLOB.tar.gz" | gzip -9 -n >"$LAB/keys/old-service.tar.gz" || die "could not repack the service bundle"
  fi
  cmp -s "$BLOB.tar.gz" "$LAB/keys/old-service.tar.gz" && die "the repacked service bundle is the same bytes as the original"
  printf '%s %s\n' "$(sha256sum "$LAB/keys/old-service.tar.gz" | cut -d' ' -f1)" "$(stat -c %s "$LAB/keys/old-service.tar.gz")" >"$LAB/keys/old-service.sha256"
  printf '{"Replace": {"%s/internal/update/keys.go": "%s", "%s.tar.gz": "%s", "%s.sha256": "%s"}}\n' \
    "$ROOT" "$LAB/keys/keys.go" "$BLOB" "$LAB/keys/old-service.tar.gz" "$BLOB" "$LAB/keys/old-service.sha256" >"$LAB/keys/overlay-old.json"
  (cd "$ROOT" && go build -overlay "$LAB/keys/overlay-old.json" -tags service_bundle -ldflags "-X main.version=$OLD_VERSION -X main.updateChannel=stable" -o "$LAB/old/htmlclay" ./cmd/htmlclay) \
    >"$LOGS/build-old.log" 2>&1 || die "the old app did not build, see $LOGS/build-old.log"
else
  OLD_SRC="${HTMLCLAY_REHEARSAL_OLD_SRC:-}"
  if [ -z "$OLD_SRC" ]; then
    OLD_SRC="$LAB/old-src"
    say "cloning the public $OLD_TAG"
    git clone --quiet --depth 1 --branch "$OLD_TAG" "$PUBLIC_URL" "$OLD_SRC" >"$LOGS/clone.log" 2>&1 || die "could not clone $OLD_TAG from $PUBLIC_URL"
  fi
  [ -f "$OLD_SRC/internal/update/keys.go" ] || die "$OLD_SRC is not an HTML Clay source tree"
  if [ -d "$OLD_SRC/.git" ] && [ -n "$(git -C "$OLD_SRC" status --porcelain)" ]; then
    die "$OLD_SRC differs from its tag: $(git -C "$OLD_SRC" status --porcelain | head -3)"
  fi
  say "building the old app ($OLD_TAG as $OLD_VERSION)"
  overlay "$OLD_SRC" >"$LAB/keys/overlay-old.json"
  (cd "$OLD_SRC" && go build -overlay "$LAB/keys/overlay-old.json" -ldflags "-X main.version=$OLD_VERSION -X main.updateChannel=stable" -o "$LAB/old/htmlclay" ./cmd/htmlclay) \
    >"$LOGS/build-old.log" 2>&1 || die "the old app did not build, see $LOGS/build-old.log"
fi

BUILD_VERSION="$NEW_VERSION"
[ "$CASE" = "rollback-version" ] && BUILD_VERSION="9.0.2"
[ "$CASE" = "next-rollback" ] && BUILD_VERSION="9.0.3"
say "building the new app (this checkout as $BUILD_VERSION) with the service inside"
overlay "$ROOT" >"$LAB/keys/overlay-new.json"
(cd "$ROOT" && go build -overlay "$LAB/keys/overlay-new.json" -tags service_bundle -ldflags "-X main.version=$BUILD_VERSION -X main.updateChannel=stable" -o "$LAB/new/htmlclay" ./cmd/htmlclay) \
  >"$LOGS/build-new.log" 2>&1 || die "the new app did not build, see $LOGS/build-new.log"
INFO="$("$LAB/new/htmlclay" update-info 2>/dev/null)"
case "$INFO" in
  *"\"version\":\"$BUILD_VERSION\""*"\"service\":true"*) say "the new app: $INFO" ;;
  *) die "the new app does not carry the service: $INFO" ;;
esac
if [ "$CASE" = "rollback-early" ]; then
  printf '#!/bin/sh\nexit 3\n' >"$LAB/new/artifact"
else
  cp "$LAB/new/htmlclay" "$LAB/new/artifact"
fi

# --- the feed -----------------------------------------------------------------

"$LAB/bin/updaterehearsal" feed -dir "$LAB/feed" -cert "$LAB/keys/cert.pem" -addr "$LAB/keys/addr" >"$LOGS/feed.log" 2>&1 &
FEED_PID=$!
for _ in $(seq 1 50); do [ -s "$LAB/keys/addr" ] && break; sleep 0.1; done
FEED_ADDR="$(tr -d '[:space:]' <"$LAB/keys/addr" 2>/dev/null)"
[ -n "$FEED_ADDR" ] || die "the feed server did not start"

ART="$LAB/feed/update/$NEW_VERSION"
mkdir -p "$ART"
for target in linux-amd64 linux-arm64 windows-amd64 windows-arm64; do
  ext=".gz"; case "$target" in windows-*) ext=".exe" ;; esac
  printf 'not built for the rehearsal\n' >"$ART/htmlclay-$NEW_VERSION-$target$ext"
done
gzip -c "$LAB/new/artifact" >"$ART/htmlclay-$NEW_VERSION-linux-$ARCH.gz"
SOURCE_SHA="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || true)"
[[ "$SOURCE_SHA" =~ ^[0-9a-f]{40}$ ]] || SOURCE_SHA="0000000000000000000000000000000000000000"
"$LAB/bin/mkfeed" payload -version "$NEW_VERSION" -source-sha "$SOURCE_SHA" -base-url "https://$FEED_ADDR/update" "$ART" >"$LAB/keys/payload.json" \
  || die "could not build the feed payload"
HTMLCLAY_UPDATE_SEED="$(cat "$LAB/keys/seed")" "$LAB/bin/signfeed" sign -key-id rehearsal "$LAB/keys/payload.json" >"$LAB/feed/update/stable.json" \
  || die "could not sign the feed"

# --- the old app, with three documents open -----------------------------------

printf '#!/bin/sh\nprintf "%%s\\n" "$1" >> "$HTMLCLAY_LAB/opened-urls"\n' >"$LAB/stubs/xdg-open"
printf '#!/bin/sh\nprintf "%%s\\n" "$*" >> "$HTMLCLAY_LAB/notifications"\n' >"$LAB/stubs/notify-send"
chmod +x "$LAB/stubs/xdg-open" "$LAB/stubs/notify-send"

APP_ENV=(env HOME="$FAKE_HOME" XDG_CONFIG_HOME="$FAKE_HOME/.config" XDG_DATA_HOME="$FAKE_HOME/.local/share"
  XDG_CACHE_HOME="$FAKE_HOME/.cache" PATH="$LAB/stubs:$PATH" HTMLCLAY_LAB="$LAB"
  HTMLCLAY_LOCAL_USERDATA="$FAKE_HOME/no-hyperclay-local"
  SSL_CERT_FILE="$LAB/keys/cert.pem" HTMLCLAY_UPDATE_FEED="https://$FEED_ADDR/update/stable.json")

PORT="$(free_port)"
identity() { stat -c '%d:%i' "$1"; }
printf '%s\n' '<!DOCTYPE html><html><body><p>rehearsal</p></body></html>' >"$FAKE_HOME/docs/a.htmlclay"
printf '%s\n' '<!DOCTYPE html><html><body><p>second</p></body></html>' >"$FAKE_HOME/docs2/b.htmlclay"
printf '%s\n' '<!DOCTYPE html><html><body><p>loose</p></body></html>' >"$FAKE_HOME/loose/c.htmlclay"
cat >"$CONFIG/config.json" <<JSON
{"port": $PORT, "workspaceFolders": [{"path": "$FAKE_HOME/docs", "identity": "$(identity "$FAKE_HOME/docs")"}, {"path": "$FAKE_HOME/docs2", "identity": "$(identity "$FAKE_HOME/docs2")"}]}
JSON
cp "$LAB/old/htmlclay" "$INSTALL"

say "starting the old app on port $PORT"
"${APP_ENV[@]}" "$INSTALL" >"$LOGS/app.out" 2>&1 &
for _ in $(seq 1 100); do port_answers && break; sleep 0.2; done
port_answers || die "the old app never served port $PORT: $(tail -5 "$LOGS/app.out")"
OLD_PID="$(lock_pid)"
alive "$OLD_PID" || die "the old app holds no lock"
STARTED_AT=$SECONDS

open_document() {
  local file="$1" marker="$2"
  "${APP_ENV[@]}" "$INSTALL" "$file" >>"$LOGS/open.out" 2>&1
  for _ in $(seq 1 40); do
    grep -q "$marker" "$LAB/opened-urls" 2>/dev/null && return 0
    sleep 0.25
  done
  return 1
}
# 1.x gives its first site the name the dashboard takes in 2.0; 2.0 never does.
FIRST_MARK="htmlclay.localhost"
[ -n "$NEXT" ] && FIRST_MARK="a.htmlclay"
open_document "$FAKE_HOME/docs/a.htmlclay" "$FIRST_MARK" || die "the old app never opened a.htmlclay"
open_document "$FAKE_HOME/docs2/b.htmlclay" "b.htmlclay" || die "the old app never opened b.htmlclay"
open_document "$FAKE_HOME/loose/c.htmlclay" "c.htmlclay" || die "the old app never opened c.htmlclay"
for _ in $(seq 1 40); do jget "$CONFIG/config.json" siteSlugs >"$LOGS/slugs-before.json" && break; sleep 0.25; done
[ -s "$LOGS/slugs-before.json" ] || die "the old app wrote no siteSlugs"
if [ -z "$NEXT" ]; then
  grep -q '"htmlclay"' "$LOGS/slugs-before.json" || die "no first site named htmlclay: $(cat "$LOGS/slugs-before.json")"
fi
say "sites before the update: $(cat "$LOGS/slugs-before.json")"

# serve_site serves a folder through the installed binary's command line, the
# way a person would, retrying while the app is not up yet. Serving is what
# makes the app start its service.
serve_site() {
  local out="" i
  mkdir -p "$FAKE_HOME/site"
  printf '%s\n' '<!DOCTYPE html><html><body><p>site</p></body></html>' >"$FAKE_HOME/site/index.html"
  for i in $(seq 1 40); do
    if out="$("${APP_ENV[@]}" "$INSTALL" serve "$FAKE_HOME/site" --name site 2>&1)"; then
      printf '%s' "$out"
      return 0
    fi
    sleep 0.25
  done
  printf '%s' "$out"
  return 1
}

service_starts() { grep -c 'The live-room service started in' "$CONFIG/htmlclay.log" 2>/dev/null || true; }
runtime_versions() { ls "$CONFIG/service-runtime" 2>/dev/null | grep -v '^\.' | sort | tr '\n' ' '; }
site_answers() {
  case "$(curl -s -m 5 -H "Host: $SITE_HOST" "http://127.0.0.1:$PORT/index.html" 2>/dev/null)" in
    *"<p>site</p>"*) return 0 ;;
  esac
  return 1
}

# In the next cases the old version is 2.0: it serves a folder before the update,
# so its service runs and its unpacked copy is on disk when the new version starts.
SITE_HOST=""
OLD_RUNTIME=""
if [ -n "$NEXT" ]; then
  SERVE_OUT="$(serve_site)" || die "the old app could not serve a folder: $SERVE_OUT"
  SITE_HOST="$(printf '%s' "$SERVE_OUT" | sed -n 's|.*http://\([^/ ]*\)/.*|\1|p' | head -n 1)"
  for _ in $(seq 1 240); do [ "$(service_starts)" -ge 1 ] && break; sleep 0.5; done
  [ "$(service_starts)" -ge 1 ] || die "the old app's service never started: $(grep -i 'live-room' "$CONFIG/htmlclay.log" | tail -n 3)"
  site_answers || die "the old app does not answer its served folder at '$SITE_HOST'"
  OLD_RUNTIME="$(runtime_versions | tr -d ' ')"
  say "the old app serves $SITE_HOST, its service runs from service-runtime/$OLD_RUNTIME"
fi

LEDGER_MODE="rollback"
[ "$CASE" = "first" ] && LEDGER_MODE="first"
case "$CASE" in next|next-button) LEDGER_MODE="next" ;; esac
# After a quit the tabs' save tokens are gone (they live in the running app's
# memory), so next-start asks only that every save acked before the quit survives.
[ "$CASE" = "next-start" ] && LEDGER_MODE="quit"
node "$ROOT/scripts/upgrade-ledger.mjs" --port "$PORT" --config "$CONFIG" --out "$LOGS" --mode "$LEDGER_MODE" \
  --stop-file "$LOGS/ledger.stop" --times "$LOGS" --home "$FAKE_HOME" \
  "$FAKE_HOME/docs/a.htmlclay" "$FAKE_HOME/docs2/b.htmlclay" "$FAKE_HOME/loose/c.htmlclay" >"$LOGS/ledger.out" 2>"$LOGS/ledger.err" &
LEDGER_PID=$!
for _ in $(seq 1 120); do [ -f "$LOGS/ledger-ready" ] && break; sleep 0.25; done
[ -f "$LOGS/ledger-ready" ] || die "the ledger's tabs never saved: $(tail -n 3 "$LOGS/ledger.out" "$LOGS/ledger.err" 2>/dev/null | tr '\n' ' ')"

# The moment the binary changed, which the ledger reads to tell the old version's saves from the new one's.
( while [ ! -e "$PREVIOUS" ]; do sleep 0.05; done; now_ms >"$LOGS/version-changed.ms" ) &
WATCH_PID=$!

# --- the update ---------------------------------------------------------------

# A browser tab's requests: the host answers a first load of / with a ticket
# redirect, /enter spends the ticket and sets the session cookie, and the page
# carries the token every action needs (internal/dashboard/server.go).
PAGE_JAR="$LOGS/page.cookies"
PAGE_TOKEN=""
page_curl() { curl -sS --max-time 10 --resolve "htmlclay.localhost:$PORT:127.0.0.1" -b "$PAGE_JAR" "$@" 2>>"$LOGS/page-curl.log"; }
page_sign_in() {
  page_curl -L -c "$PAGE_JAR" -o "$LOGS/page.html" "http://htmlclay.localhost:$PORT/" || return 1
  PAGE_TOKEN="$(sed -n 's/.*<meta name="workspace-token" content="\([^"]*\)".*/\1/p' "$LOGS/page.html" | head -n 1)"
  [ -n "$PAGE_TOKEN" ]
}
# page_action TYPE sends what a page button sends, and succeeds when the host ran
# it (it answers with the new snapshot) rather than refusing it (a code).
page_action() {
  local id answer
  id="$(node -e 'console.log(require("crypto").randomUUID())')"
  answer="$(page_curl -H "X-Workspace-Token: $PAGE_TOKEN" -H 'Content-Type: application/json' \
    -X POST --data "{\"id\":\"$id\",\"type\":\"$1\"}" "http://htmlclay.localhost:$PORT/api/app/actions")"
  printf '%s\n' "$answer" >"$LOGS/action-$1.json"
  printf '%s' "$answer" | node -e '
    let t = ""; process.stdin.on("data", c => t += c).on("end", () => {
      try { const d = JSON.parse(t); process.exit("snapshot" in d && !("code" in d) && !d.pending ? 0 : 1); } catch { process.exit(1); }
    });'
}
# wait_update_offered: the page's Update button shows when the snapshot has an
# update ready, none finishing, and update.start allowed.
wait_update_offered() {
  local start=$SECONDS
  while [ $((SECONDS - start)) -lt 120 ]; do
    page_curl -H "X-Workspace-Token: $PAGE_TOKEN" -o "$LOGS/page-snapshot.json" "http://htmlclay.localhost:$PORT/api/app/snapshot"
    if node -e '
      const s = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"));
      const u = s.update || {};
      process.exit(u.ready === process.argv[2] && !u.finishing && ((s.host || {}).can || {})["update.start"] === true ? 0 : 1);' \
      "$LOGS/page-snapshot.json" "$NEW_VERSION" 2>/dev/null; then
      say "the page offers Update to $NEW_VERSION after $((SECONDS - start)) s"
      return 0
    fi
    sleep 1
  done
  return 1
}

if [ "$CASE" = "next-start" ] || [ "$CASE" = "next-button" ]; then
  page_sign_in || { fail "could not sign in as a browser tab: $(tail -n 2 "$LOGS/page-curl.log" 2>/dev/null)"; finish; }
  # update.check is refused as not yet until the updater has started.
  CHECKED=""
  for _ in $(seq 1 30); do page_action update.check && { CHECKED=1; break; }; sleep 1; done
  [ -n "$CHECKED" ] || { fail "Check for updates was refused: $(cat "$LOGS/action-update.check.json")"; finish; }
  say "pressed Check for updates after $((SECONDS - STARTED_AT)) s"
  wait_update_offered || { fail "the page never offered $NEW_VERSION: $(head -c 600 "$LOGS/page-snapshot.json")"; finish; }
  if [ "$CASE" = "next-button" ]; then
    page_action update.start || { fail "Update was refused: $(cat "$LOGS/action-update.start.json")"; finish; }
    CLICK_MS="$(now_ms)"
    pass "pressed Update after $((SECONDS - STARTED_AT)) s"
  else
    [ -s "$UPDATES/waiting.json" ] && pass "the downloaded update waits in waiting.json" || fail "no waiting.json after the download"
    OPENED_BEFORE="$(grep -c 'a.htmlclay' "$LAB/opened-urls")"
    # SIGTERM takes the tray's own Quit path (internal/tray/tray.go).
    kill -TERM "$OLD_PID"
    for _ in $(seq 1 100); do alive "$OLD_PID" || break; sleep 0.2; done
    if alive "$OLD_PID"; then fail "the old app did not quit"; finish; fi
    pass "the old app quit with the update still waiting"
    CLICK_MS="$(now_ms)"
    "${APP_ENV[@]}" "$INSTALL" "$FAKE_HOME/docs/a.htmlclay" >"$LOGS/start-again.out" 2>&1 &
    AGAIN_PID=$!
    for _ in $(seq 1 100); do alive "$AGAIN_PID" || break; sleep 0.2; done
    if alive "$AGAIN_PID"; then
      fail "starting again ran the old version instead of installing the waiting update"
    else
      wait "$AGAIN_PID"
      AGAIN_EXIT=$?
      [ "$AGAIN_EXIT" = 0 ] && pass "starting again handed over to the update and exited after $(( $(now_ms) - CLICK_MS )) ms" \
        || fail "starting again exited $AGAIN_EXIT: $(tail -n 3 "$LOGS/start-again.out")"
    fi
  fi
elif [ -n "$NEXT" ]; then
  # 2.0 takes update-now on Linux. "checking" means it is fetching the version now.
  say "asking the old app to update to $NEW_VERSION with update-now"
  ANSWER=""
  for _ in $(seq 1 240); do
    ANSWER="$("${APP_ENV[@]}" "$INSTALL" update-now --version "$NEW_VERSION" 2>&1 | tail -n 1)"
    [ "$ANSWER" = "accepted" ] && break
    sleep 0.5
  done
  if [ "$ANSWER" != "accepted" ]; then
    fail "update-now was never accepted: $ANSWER"
    finish
  fi
  CLICK_MS="$(now_ms)"
  say "update-now accepted after $((SECONDS - STARTED_AT)) s"
else
  say "waiting for the old app to download $NEW_VERSION (its first check runs 60 s after it starts)"
  if ! "$LAB/bin/updaterehearsal" click -pid "$OLD_PID" -prefix "Restart to Update" -wait 150 >"$LOGS/click.out" 2>&1; then
    fail "the old app never offered the update: $(cat "$LOGS/click.out")"
    finish
  fi
  CLICK_MS="$(now_ms)"
  say "$(cat "$LOGS/click.out") after $((SECONDS - STARTED_AT)) s"
fi

# In rollback-version the new version runs for the supervisor's whole 20 s.
# Serve a folder in that window so its service runs, and the checks below can
# see that the rollback takes the service down with the app.
JUDGED_SERVICE=""
if [ "$CASE" = "rollback-version" ]; then
  # 1.x has no serve command, so wait until the installed binary is the new one.
  for _ in $(seq 1 150); do [ -s "$LOGS/version-changed.ms" ] && break; sleep 0.1; done
  if SERVE_OUT="$(serve_site)"; then
    for _ in $(seq 1 60); do
      JUDGED_SERVICE="$(pgrep -f "$CONFIG/service-runtime" | head -n 1)"
      [ -n "$JUDGED_SERVICE" ] && break
      sleep 0.25
    done
  fi
  if [ -n "$JUDGED_SERVICE" ]; then
    pass "the judged version's service was running as pid $JUDGED_SERVICE ($SERVE_OUT)"
  else
    fail "the judged version's service never ran, so the rollback did not have to stop it: ${SERVE_OUT:-no answer}"
  fi
fi

PHASE=""
for _ in $(seq 1 300); do
  PHASE="$(jget "$UPDATES/state.json" phase 2>/dev/null || true)"
  case "$PHASE" in done|rolledBack|rollbackFailed) break ;; esac
  sleep 0.3
done
VERDICT_MS="$(now_ms)"
say "the supervisor's verdict: ${PHASE:-none} after $(( (VERDICT_MS - CLICK_MS) / 1000 )) s"
[ -f "$UPDATES/supervisor.log" ] && sed 's/^/[supervisor] /' "$UPDATES/supervisor.log" >&2

stop_ledger() {
  : >"$LOGS/ledger.stop"
  for _ in $(seq 1 120); do alive "$LEDGER_PID" || break; sleep 0.25; done
  LEDGER_PID=""
  local line
  line="$(tail -n 1 "$LOGS/ledger.out" 2>/dev/null)"
  grep '^\[ledger\]' "$LOGS/ledger.err" >&2 2>/dev/null
  case "$line" in
    "LEDGER OK"*) pass "$line"; note " $line" ;;
    *) fail "${line:-the ledger printed nothing}" ;;
  esac
}

if [ "$CASE" = "first" ] || [ "$CASE" = "next" ] || [ "$CASE" = "next-start" ] || [ "$CASE" = "next-button" ]; then
  [ "$PHASE" = "done" ] && pass "state.json says done" || fail "state.json says '${PHASE:-nothing}', not done"
  H_VERSION="$(jget "$UPDATES/health.json" version 2>/dev/null || true)"
  H_PHASE="$(jget "$UPDATES/health.json" phase 2>/dev/null || true)"
  H_PID="$(jget "$UPDATES/health.json" pid 2>/dev/null || true)"
  H_TXID="$(jget "$UPDATES/health.json" txid 2>/dev/null || true)"
  S_TXID="$(jget "$UPDATES/state.json" txid 2>/dev/null || true)"
  if [ "$H_VERSION" = "$NEW_VERSION" ] && [ "$H_PHASE" = "serving" ] && [ -n "$S_TXID" ] && [ "$H_TXID" = "$S_TXID" ]; then
    pass "health.json: $NEW_VERSION serving for this update"
  else
    fail "health.json says version '$H_VERSION' phase '$H_PHASE' txid '$H_TXID' (state txid '$S_TXID')"
  fi
  if alive "$H_PID" && [ "$H_PID" != "$OLD_PID" ] && [ "$(lock_pid)" = "$H_PID" ]; then
    pass "the new process $H_PID holds the lock and the old one ($OLD_PID) is gone"
  else
    fail "the serving pid '$H_PID' is not a new live process holding the lock (old $OLD_PID, lock $(lock_pid))"
  fi
  cmp -s "$INSTALL" "$LAB/new/htmlclay" && pass "the installed binary is the new build" || fail "the installed binary is not the new build"
  SERVING_MS="$(node -e 'const t = Date.parse(process.argv[1].replace(/\.(\d{3})\d+/, ".$1")); if (!Number.isFinite(t)) process.exit(1); console.log(t);' "$(jget "$UPDATES/health.json" time 2>/dev/null || true)" 2>/dev/null || true)"
  if [ -n "$SERVING_MS" ]; then
    note " $OLD_VERSION -> $NEW_VERSION serving $(( SERVING_MS - CLICK_MS )) ms after the click"
  else
    fail "health.json holds no time"
  fi

  if [ "$CASE" = "first" ]; then
    # Nothing 1.x leaves behind needs the service, so serve a folder: that is what starts it.
    if SERVE_OUT="$(serve_site)"; then
      pass "htmlclay serve: $SERVE_OUT"
    else
      fail "htmlclay serve failed: $SERVE_OUT"
    fi
    SITE_HOST="$(printf '%s' "$SERVE_OUT" | sed -n 's|.*http://\([^/ ]*\)/.*|\1|p' | head -n 1)"
    WANT_STARTS=1
  else
    # The folder was served before the update, so the new version starts its service by itself.
    WANT_STARTS=2
  fi
  for _ in $(seq 1 240); do [ "$(service_starts)" -ge "$WANT_STARTS" ] && break; sleep 0.5; done
  if [ "$(service_starts)" -ge "$WANT_STARTS" ]; then
    STARTED_LINE="$(grep 'The live-room service started in' "$CONFIG/htmlclay.log" | tail -n 1)"
    pass "the new version's service runs: ${STARTED_LINE#*The live-room service }"
    note " the service ${STARTED_LINE#*The live-room service }"
  else
    fail "the new version's service never started: $(grep -i 'live-room' "$CONFIG/htmlclay.log" | tail -n 3)"
  fi
  for _ in $(seq 1 20); do site_answers && break; sleep 0.5; done
  site_answers && pass "the served folder answers at $SITE_HOST" || fail "the served folder does not answer at '$SITE_HOST'"
  port_answers && pass "port $PORT answers" || fail "port $PORT does not answer"
  alive "$(lock_pid)" || fail "the new app died after it reported serving"

  if [ "$CASE" = "next-start" ]; then
    [ -e "$UPDATES/waiting.json" ] && fail "waiting.json is still there after the update" || pass "waiting.json is gone"
    for _ in $(seq 1 40); do [ "$(grep -c 'a.htmlclay' "$LAB/opened-urls")" -gt "$OPENED_BEFORE" ] && break; sleep 0.25; done
    [ "$(grep -c 'a.htmlclay' "$LAB/opened-urls")" -gt "$OPENED_BEFORE" ] && pass "the file HTML Clay was started with opened after the update" \
      || fail "a.htmlclay did not open after the update: $(tail -n 3 "$LAB/opened-urls")"
  fi
  if [ "$CASE" != "first" ]; then
    sleep 8
    stop_ledger
    NOW_RUNTIME="$(runtime_versions | tr -d ' ')"
    if [ -n "$NOW_RUNTIME" ] && [ "$NOW_RUNTIME" != "$OLD_RUNTIME" ] && [ "$(runtime_versions | wc -w)" -eq 1 ]; then
      pass "the old version's service copy was removed after the verdict (service-runtime holds $NOW_RUNTIME)"
    else
      fail "service-runtime holds '$(runtime_versions)' after the verdict; the old copy was $OLD_RUNTIME"
    fi
    cmp -s "$LOGS/slugs-before.json" <(jget "$CONFIG/config.json" siteSlugs 2>/dev/null) && pass "the sites are the ones from before the update" || fail "the sites changed across the update: $(jget "$CONFIG/config.json" siteSlugs)"
    finish
  fi

  # The first site moves only after the verdict, and the ledger needs saves on both sides of it.
  sleep 30
  stop_ledger
  if grep -q '"htmlclay"' <(jget "$CONFIG/config.json" siteSlugs 2>/dev/null || echo '"htmlclay"'); then
    fail "the first site still has the dashboard's name after the update: $(jget "$CONFIG/config.json" siteSlugs)"
  else
    pass "the first site was renamed after the verdict: $(jget "$CONFIG/config.json" siteSlugs)"
  fi
  finish
fi

# The two rollback cases.
[ "$PHASE" = "rolledBack" ] && pass "state.json says rolledBack" || fail "state.json says '${PHASE:-nothing}', not rolledBack"
F_VERSION="$(jget "$UPDATES/state.json" failed.version 2>/dev/null || true)"
F_REASON="$(jget "$UPDATES/state.json" failed.reason 2>/dev/null || true)"
[ "$F_VERSION" = "$NEW_VERSION" ] && pass "the failure is recorded: $F_REASON" || fail "state.json records no failure of $NEW_VERSION"
note " rolled back $(( (VERDICT_MS - CLICK_MS) / 1000 )) s after the click ($F_REASON)"

BACK_PID=""
for _ in $(seq 1 75); do
  BACK_PID="$(lock_pid)"
  if alive "$BACK_PID" && [ "$BACK_PID" != "$OLD_PID" ] && port_answers; then break; fi
  BACK_PID=""
  sleep 0.2
done
if [ -n "$BACK_PID" ] && [ "$(readlink -f "/proc/$BACK_PID/exe")" = "$(readlink -f "$INSTALL")" ]; then
  pass "the old app is back as pid $BACK_PID on port $PORT"
else
  fail "the old app did not come back on port $PORT within 15 s"
fi
cmp -s "$INSTALL" "$LAB/old/htmlclay" && pass "the installed binary is the old build again" || fail "the installed binary is not the old build"
[ ! -e "$PREVIOUS" ] && pass "no previous binary is left beside the install" || fail "$PREVIOUS is still there"

H_TXID="$(jget "$UPDATES/health.json" txid 2>/dev/null || true)"
S_TXID="$(jget "$UPDATES/state.json" txid 2>/dev/null || true)"
H_PHASE="$(jget "$UPDATES/health.json" phase 2>/dev/null || true)"
if [ -n "$S_TXID" ] && [ "$H_TXID" = "$S_TXID" ] && [ "$H_PHASE" = "serving" ]; then
  fail "the new version reported serving and was rolled back anyway"
else
  pass "the new version never reported serving for this update"
fi

sleep 10
if [ "$CASE" = "next-rollback" ]; then
  # The old 2.0 is back and serves its folder again. Its service must run from the
  # copy it unpacked before the update, which the judged version had to leave alone.
  for _ in $(seq 1 60); do pgrep -f "$CONFIG/service-runtime/$OLD_RUNTIME/" >/dev/null && site_answers && break; sleep 0.5; done
  BACK_SERVICE="$(pgrep -f "$CONFIG/service-runtime/$OLD_RUNTIME/" | head -n 1)"
  [ -n "$BACK_SERVICE" ] && pass "the old version's service runs again from service-runtime/$OLD_RUNTIME (pid $BACK_SERVICE)" || fail "the old version's service is not running from service-runtime/$OLD_RUNTIME; service-runtime holds '$(runtime_versions)'"
  site_answers && pass "the served folder answers again at $SITE_HOST" || fail "the served folder does not answer at '$SITE_HOST' after the rollback"
  OTHER="$(pgrep -af "$CONFIG/service-runtime" | grep -v "service-runtime/$OLD_RUNTIME/" | tr '\n' ' ')"
  [ -z "$OTHER" ] && pass "no service process of the judged version is left" || fail "a service process of the judged version is still running: $OTHER"
  if grep -q 'extract' <(grep -i 'live-room' "$CONFIG/htmlclay.log" | tail -n 5); then
    say "service log tail: $(grep -i 'live-room' "$CONFIG/htmlclay.log" | tail -n 5 | tr '\n' ' ')"
  fi
else
  LEFT="$(pgrep -f "$CONFIG/service-runtime" | tr '\n' ' ')"
  [ -z "$LEFT" ] && pass "no service process of the new version is left" || fail "a service process of the new version is still running: $LEFT"
  if grep -q '"servedFolders"' "$CONFIG/config.json" 2>/dev/null && grep -q "$FAKE_HOME/site" "$CONFIG/config.json"; then
    fail "the rolled-back version wrote its served folder into the config the old app reads"
  else
    pass "nothing the judged version did reached the config"
  fi
fi
jget "$CONFIG/config.json" siteSlugs >"$LOGS/slugs-after.json" 2>/dev/null
if cmp -s "$LOGS/slugs-before.json" "$LOGS/slugs-after.json"; then
  pass "the sites are the ones from before the update"
else
  fail "the sites changed across a rolled-back update: $(cat "$LOGS/slugs-before.json") became $(cat "$LOGS/slugs-after.json" 2>/dev/null)"
fi
stop_ledger
finish
