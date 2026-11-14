# Wire conformance cases

One JSON file per wire behavior, replayed by `TestM3WireConformance` in
`cmd/htmlclay/wire_conformance_test.go` on both hosts: a loose file's site and a
served folder. Cases whose `hosts` name both are also compared: every refusal
body (status 400 and above) must be byte-identical.

A case has `name`, `hosts` (`loose`, `served`), `served` (`released` or
`unreleased`), `setup` (`program` none/echo/wait, `decision` none/allow, optional
`aiEdit` on/off), and `steps`. A step is `subscribe`, `send`, `see`, `closed`,
`drop` or `resubscribe`; `as` is `page`, `page-other-origin`, `handler` or
`local-with-cookie`. `subscribe` and `send` carry `expect.status`. A
`knownDifference` string reports, rather than fails, the body comparison.

Run it with:

    go test ./cmd/htmlclay/ -run '^TestM3WireConformance' -count=1 -v

Cases with `"served": "unreleased"` describe behavior that is not released yet.
