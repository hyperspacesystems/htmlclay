# Contributing

Issues and pull requests are welcome.

## License of contributions

Contributions are accepted under the MIT License (inbound MIT) with a
Developer Certificate of Origin sign-off (`git commit -s`, DCO 1.1). The
project itself ships under the First Million of Revenue from Building a Competing Service Stays Yours License; see LICENSE.

## Testing helper programs

HTML Clay resolves a login-shell `PATH` for helper programs (`internal/server/helper.go` sets `LoginPath: true`, and `internal/helper/runner.go:62` runs `$SHELL -l -c 'printf %s "$PATH"'` with caching and an exit-127 retry), so Finder and Dock launches already hand helpers the user's real `PATH` including Homebrew. Simulating a GUI launch with `env -i HOME=<scratch> PATH=/usr/bin:/bin:/usr/sbin:/sbin <binary> <file>` strips `SHELL`, which silently disables `resolveLoginPath` and manufactures a missing-tool failure no real launch produces. Always include `SHELL=/bin/zsh` in `env -i` reproductions instead of patching `PATH` fallbacks into helpers.
