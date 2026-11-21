## What changes, and why

## How it was verified

- [ ] a test that fails without the change
- [ ] `go test -race ./...`, `golangci-lint run`, `test/e2e/run.sh` (v1 and v3) where the change touches the tunnel
- [ ] every page, example and help text describing the behaviour updated
- [ ] a line under *Unreleased* in CHANGELOG.md
