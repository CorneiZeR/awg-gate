# Docker

## The image

`ghcr.io/corneizer/awg-gate` - one image for every role, `linux/amd64` and `linux/arm64`.

| Tag | |
|---|---|
| `0.2.0` | a release; pin this |
| `0.2` | the latest release of a minor series |
| `latest` | the latest release |

Inside: `awg-gate` (entrypoint), `amneziawg-go` (the userspace AmneziaWG), `awg` (its
configuration tool), `ip` and `nft`, on Alpine. About 40 MB. Signed, with an SBOM and
provenance - see [Security model](Security-model.md#trust-in-the-image).

## What a container needs

| | Hub | Host spoke | Sidecar | Why |
|---|---|---|---|---|
| `cap_add: [NET_ADMIN]` | ✓ | ✓ | ✓ | create the tunnel, set addresses, routes and nftables rules |
| `cap_drop: [ALL]` | ✓ | ✓ | ✓ | nothing else is needed |
| `devices: /dev/net/tun` | ✓ | ✓ | ✓ | the userspace tunnel |
| `sysctls: net.ipv4.ip_forward=1` | ✓ | | | the hub forwards between peers |
| `network_mode: host` | | ✓ | | the host gets the tunnel address |
| `read_only: true`, `tmpfs: /run` | ✓ | ✓ | ✓ | runtime files live in `/run` only |
| `security_opt: no-new-privileges` | ✓ | ✓ | ✓ | |
| published port | `443/udp` | | | |

No `privileged`, no `SYS_MODULE`, no kernel module: AmneziaWG runs in userspace.

## Volumes

| Path | Role | |
|---|---|---|
| `/config` | hub | the directory holding `gate.yaml`, read-only |
| `/data` | hub | `hub.key` and `params.json`, mode `600`; back it up |
| `/config` | up | the directory holding `awg.conf`, read-only |

Mount directories, not single files: a file replaced on the host (every editor and
`awg-gate issue` replace rather than rewrite) stays the old one inside a single-file bind
mount.

## Health check

Built into the image: `awg-gate health` every 30 s, 60 s start period. Healthy means a
working tunnel - for a spoke, a handshake with the hub within 3 minutes - not merely a
running process.

## Ready-made compose files

[`deploy/hub`](https://github.com/CorneiZeR/awg-gate/tree/master/deploy/hub),
[`deploy/spoke`](https://github.com/CorneiZeR/awg-gate/tree/master/deploy/spoke),
[`deploy/sidecar`](https://github.com/CorneiZeR/awg-gate/tree/master/deploy/sidecar).
CI validates all three with `docker compose config`.

## Building it yourself

```shell
docker build -t awg-gate:local --build-arg VERSION=local .
```

Everything is compiled in the build: Go cross-compiles on the build machine's architecture,
the C tool for the target's. `--platform linux/arm64` with QEMU or a native builder builds
the other architecture.
