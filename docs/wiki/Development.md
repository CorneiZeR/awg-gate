# Development

## Layout

```text
cmd/awg-gate/        the CLI: daemons and operator commands
internal/
  keys/              X25519 keys: generate, derive, parse (stdlib crypto/ecdh)
  obfs/              AmneziaWG obfuscation profiles: draw and validate
  conf/              WireGuard-format configuration files: parse and render
  gate/              gate.yaml: schema, defaults, validation, the plan, in-place edits
  state/             the hub's persisted key and parameters; hub-info
  issue/             issued configurations and their QR codes
  nft/               the nftables rules of the hub and of a spoke
  link/              amneziawg-go, awg and ip
  daemon/            the hub and up loops, health
test/e2e/            the end-to-end bench
deploy/              compose files for the hub, a spoke, a sidecar
docs/wiki/           this site
```

## The gate

```shell
go vet ./... && go test -race ./...
golangci-lint run
test/e2e/run.sh && SKIP_BUILD=1 PROFILE=v3 test/e2e/run.sh
```

The unit tests need nothing but Go. The end-to-end test needs Docker with `/dev/net/tun`
(Docker Desktop and every Linux host have it): it builds the image, issues a network with
the image's own CLI, starts a hub, two servers, four people and an outsider and checks 27 properties.
`KEEP=1` leaves the bench running.

A behaviour change carries a test that fails without it.

## Dependencies

- `amneziawg-go` is a `tool` dependency in `go.mod`: the image builds it from there, so it
  links the same `golang.org/x` modules awg-gate does, and Dependabot raises both.
  `govulncheck` checks both binaries.
- `amneziawg-tools` is pinned by tag **and** commit in the `Dockerfile`; the weekly
  *upstream* workflow opens a pull request when a new release appears.
- Base images are pinned by digest; Dependabot raises them.
- Dependabot runs daily. Its patch and minor pull requests merge by themselves once every
  required check is green (*Dependabot auto-merge* workflow); a major one waits for a person.
- A merged update reaches users only in a new image: when it changes the image (Go modules,
  the `Dockerfile`), release a patch version. The daily *scan* of the published image fails
  when a fix exists that a rebuild would pick up - that is the signal to release.

## Releasing

1. Move the *Unreleased* section of `CHANGELOG.md` under the new version and date.
2. Merge, then tag the merge commit: `git tag -a v0.2.0 -m v0.2.0 && git push origin v0.2.0`.
3. The release workflow builds both architectures, scans the image (and stops on a finding),
   pushes `0.2.0`, `0.2` and `latest`, signs them, attaches SBOM and provenance and creates
   the GitHub release from the changelog section.

## Documentation

```shell
pip install -r docs/requirements.txt
mkdocs serve
```

A change is finished when every page that describes it says the new thing.
