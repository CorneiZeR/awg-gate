# Contributing

## Set up

Go (the version in `go.mod`'s `toolchain` line, fetched automatically), Docker with
`/dev/net/tun` for the end-to-end test, and optionally
[golangci-lint](https://golangci-lint.run/) and [shellcheck](https://www.shellcheck.net/).

## The gate

```shell
go vet ./... && go test -race ./...
golangci-lint run
shellcheck test/e2e/run.sh
test/e2e/run.sh && SKIP_BUILD=1 PROFILE=v3 test/e2e/run.sh
```

CI runs the same, plus `govulncheck`, a Trivy scan of the image and `docker compose
config` of every file in `deploy/`.

## What a change carries

- **A test that fails without it.** Revert the fix, watch the test fail, put it back.
- **Every place the behaviour lives.** A flag is in the code, its `--help` text, the
  [CLI](docs/wiki/CLI.md) page, the compose files that set it and the changelog; a
  `gate.yaml` key is in the schema, its validation, the [gate.yaml](docs/wiki/Gate-yaml.md)
  page and `deploy/hub/gate.example.yaml`. Search for the phrase describing the old
  behaviour, not only the identifier.
- **A line in `CHANGELOG.md`** under *Unreleased*, saying what changes for someone running
  it - and in bold what they must do, if anything.

## Commits and pull requests

Small, focused commits with a subject that says what changes. A pull request describes the
problem and how the change was verified. Security issues go through
[SECURITY.md](SECURITY.md), not a pull request.
