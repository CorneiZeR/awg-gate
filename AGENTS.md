# AGENTS.md

Instructions for coding agents working on **this repository**. For running awg-gate, see
the [documentation](https://corneizer.github.io/awg-gate/).

## What this is

An AmneziaWG access network for reaching private servers: one hub, servers as spokes,
people as peers, access per person decided by one file, `gate.yaml`. One Go binary is
both the daemons (`hub`, `up`) and the operator's CLI (`hub-info`, `issue`, `revoke`,
`check`, `status`, `health`); one container image carries it with `amneziawg-go`, `awg`,
`ip` and `nft`.

```text
cmd/awg-gate/main.go   every command and flag
internal/keys          X25519 keys (crypto/ecdh)
internal/obfs          obfuscation profiles v1/v3: draw, validate, render
internal/conf          WireGuard-format files: parse, render, strip wg-quick keys
internal/gate          gate.yaml: schema, defaults, validation, Plan (peers + rules), YAML-tree edits
internal/state         the hub's key and parameters in /data; hub-info
internal/issue         issued configurations, QR codes
internal/nft           nftables rules of the hub and of a spoke (table inet awg_gate)
internal/link          amneziawg-go child process, awg, ip
internal/daemon        the hub and up loops, health reports
test/e2e               the Docker bench and its checks
deploy/                compose files users copy
docs/wiki              the mkdocs site
```

## Invariants - do not break these

- **The hub never sees a private key** other than its own. `issue` generates keys where it
  runs.
- **Rules only drop, and only tunnel traffic.** Every nftables rule matches the tunnel
  interface; chains keep `policy accept`. A hub or spoke in a host's namespace must never
  affect the host's other traffic. The one exception, `--lockdown`, exists for a sidecar's
  own namespace and is refused where `docker0` exists.
- **A gate.yaml that fails validation is never applied**, and unknown keys are errors.
- **Issued configurations carry no DNS line** and route only one subnet.
- **No wg-quick/awg-quick** at runtime: `internal/link` does what they would.
- **Nothing logs a private key or a configuration's contents.**

## The gate

```shell
go vet ./... && go test -race ./...
golangci-lint run
shellcheck test/e2e/run.sh
test/e2e/run.sh && SKIP_BUILD=1 PROFILE=v3 test/e2e/run.sh
```

A behaviour change carries a test that fails without it, and updates every page and
example that describes the behaviour (see CONTRIBUTING.md).
