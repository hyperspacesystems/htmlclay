# The private fork of HTML Clay

This repo (`github.com/panphora/htmlclay-mac`, private) is HTML Clay plus the unreleased merged app: served and synced folders in the Go host, the JavaScript service, and the native Mac app. Plan: `hyper/plans/shared/desktop-unification/real-app/00-meta-plan.md`.

- **Layout:** the Go module at the root exactly as in public `htmlclay`; the Swift package in `macos/`; the JavaScript service in `service/` (from sub plan 04).
- **Public base:** public `htmlclay` main at `86bd453` was merged on 2026-11-14. `scripts/merge-public.sh` merges the current public main and prints the new base; run it at the start of every batch.
- **Never public before release.** Nothing from this repo goes to `hyperspacesystems/htmlclay` until David releases. `scripts/check-private.sh` fails if any remote names the public repo. Release is one merge of this repo into public main, rehearsed first in a scratch clone (milestone 8); the public checkout is never the merge target before David says so, because Ferry pushes it.
- **No releases from here.** `scripts/release.sh` refuses to run in this checkout. Every workflow in `.github/workflows/` starts only by hand.
- **Candidate isolation.** Builds from this repo use their own config folder (`HTMLCLAY_CONFIG_BASE`), their own test port and app id, and never attach to the installed HTML Clay.
- **At release:** the release merge deletes this file in the same commit, because `scripts/release.sh` refuses to run while it exists.
- **Fork-only files:** `.github/workflows/fork-platforms.yml` runs the Go suite on GitHub's Windows (amd64) and Linux ARM machines, started by hand outside work hours. The release merge deletes it along with this file.
- **The wire code moves (slice 5, steps 7.26c and 7.26d).** `internal/server/wire.go`, `helper.go`, `aiedit.go`, `pagetool.go` and `helper_setup.go` move into `internal/wire` (stream helpers into `internal/sse`). After that, a fix to any of those five files on public main is ported by hand into `internal/wire`; `scripts/merge-public.sh` conflicts on them, which is the signal.
