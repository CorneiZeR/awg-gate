# awg-gate

[![CI](https://github.com/CorneiZeR/awg-gate/actions/workflows/ci.yml/badge.svg)](https://github.com/CorneiZeR/awg-gate/actions/workflows/ci.yml)
[![CodeQL](https://github.com/CorneiZeR/awg-gate/actions/workflows/codeql.yml/badge.svg)](https://github.com/CorneiZeR/awg-gate/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/CorneiZeR/awg-gate/badge)](https://scorecard.dev/viewer/?uri=github.com/CorneiZeR/awg-gate)
[![Release](https://img.shields.io/github/v/release/CorneiZeR/awg-gate)](https://github.com/CorneiZeR/awg-gate/releases/latest)
[![Image](https://img.shields.io/badge/image-ghcr.io%2Fcorneizer%2Fawg--gate-blue?logo=docker)](https://github.com/CorneiZeR/awg-gate/pkgs/container/awg-gate)
[![Docs](https://img.shields.io/badge/docs-corneizer.github.io%2Fawg--gate-blue)](https://corneizer.github.io/awg-gate/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

An [AmneziaWG](https://docs.amnezia.org/documentation/amnezia-wg/) access network for
reaching private servers - dev and staging environments, admin panels, internal tools -
and nothing else.

```
  alice's laptop ─┐                          ┌─ shop-dev   10.247.0.10
  alice's phone  ─┤   AmneziaWG, udp/443     │
  bob's laptop   ─┼──────────►  hub  ◄───────┼─ blog-dev   10.247.0.11
                  │         10.247.0.1       │
       people 10.247.1.0/24      access rules: who reaches which server, on which ports
```

- **One hub, servers as spokes, people as peers.** One configuration per device reaches
  every server its owner may reach. Servers never reach each other, people never reach
  each other, and a server never opens a connection to a person.
- **Access per person,** in one reviewed file: `reach: [shop-dev]`, optional ports per
  server, an optional last day after which the hub drops the person by itself.
- **AmneziaWG, not plain WireGuard.** Traffic is obfuscated so DPI that blocks WireGuard
  does not recognise it; clients exist for iOS, Android, macOS, Windows and Linux, and run
  alongside (or inside) a commercial VPN.
- **Split tunnel, no DNS.** Only the access network goes through the tunnel. Names keep
  resolving through the normal DNS - to private addresses - so certificates stay publicly
  trusted and nobody's browsing goes through your hub.
- **Any server, any service.** It is a network, not a library: Django, Next.js, Grafana,
  Postgres, anything behind Traefik or nginx, or a single container behind a sidecar.
- **Containers only, built to be audited.** One image for every role, multi-arch, built in
  CI with no compiler left in it, signed, with an SBOM - and **zero known CVEs** at release
  (the upstream AmneziaWG image had 70, 32 of them high, when this was written).

## Quick start

On the hub (any small VPS with a public IP):

```shell
curl -fsSLO https://raw.githubusercontent.com/CorneiZeR/awg-gate/master/deploy/hub/compose.yaml
mkdir config && curl -fsSL -o config/gate.yaml \
  https://raw.githubusercontent.com/CorneiZeR/awg-gate/master/deploy/hub/gate.example.yaml
$EDITOR config/gate.yaml                                    # set endpoint: vpn.example.com:443
docker compose run --rm hub hub-info --init > hub.json      # the hub's identity, drawn once
docker compose up -d
```

On your machine, with `gate.yaml` and `hub.json` next to you:

```shell
alias awg-gate='docker run --rm -it -v "$PWD:/work" -w /work --user "$(id -u):$(id -g)" ghcr.io/corneizer/awg-gate:0.2.0'

awg-gate issue server shop-dev --hub-info hub.json --gate gate.yaml --ports 443,80
awg-gate issue person alice laptop --hub-info hub.json --gate gate.yaml --reach shop-dev
```

Copy the new `gate.yaml` to the hub (it picks it up within 30 seconds), put
`shop-dev.conf` on the server and start [`deploy/spoke`](deploy/spoke/compose.yaml) there,
and give Alice `alice-laptop.conf` (or `alice-laptop.png` to scan) for the AmneziaVPN app.
Then make the server's sites answer only the access network - one middleware in Traefik,
two lines in nginx.

The whole walk-through, including the DNS and certificate side in Cloudflare, is the
[Quick start](https://corneizer.github.io/awg-gate/Quick-start/) page.

## Documentation

**[corneizer.github.io/awg-gate](https://corneizer.github.io/awg-gate/)** - concepts, the
hub, servers, Traefik / nginx / Caddy recipes, Cloudflare, people and devices, clients on
every platform, obfuscation profiles, operations, security model, troubleshooting, and the
`gate.yaml` and CLI reference. The pages live in [`docs/wiki`](docs/wiki).

## How it is tested

`go test ./...` covers configuration, planning, rule rendering and key handling.
[`test/e2e/run.sh`](test/e2e/run.sh) issues a whole network with the image's own CLI,
starts a hub, two servers (Traefik with an allowlist, nginx behind a locked-down sidecar)
and four people in Docker, and proves 29 properties for each obfuscation profile: each
person reaches exactly their servers and ports, nobody reaches anyone else, an expired
person and a plain-WireGuard client get nothing, the internet gets a 403 or silence, and
edits to `gate.yaml` apply live - while a broken one is refused and the last good one kept.

## Questions and contributions

Questions and ideas go to [Discussions](https://github.com/CorneiZeR/awg-gate/discussions),
bugs to [issues](https://github.com/CorneiZeR/awg-gate/issues), vulnerabilities privately
through [SECURITY.md](SECURITY.md). Contributions: [CONTRIBUTING.md](CONTRIBUTING.md) and
the [code of conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE). AmneziaWG itself is MIT (amneziawg-go) and GPL-2.0 (amneziawg-tools, of
which the image ships the `awg` binary unmodified, built from the pinned upstream commit).
