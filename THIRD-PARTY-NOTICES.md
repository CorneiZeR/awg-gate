# Third-party software in the awg-gate image

The image ships these programs besides awg-gate itself (MIT, see `LICENSE`). Each is built
from the exact upstream source named here; the image's `Dockerfile` and `go.mod` pin them.

| Program | Upstream | Version | License |
|---|---|---|---|
| `amneziawg-go` | https://github.com/amnezia-vpn/amneziawg-go | the `github.com/amnezia-vpn/amneziawg-go/v3` version in `go.mod` | MIT |
| `awg` (amneziawg-tools) | https://github.com/amnezia-vpn/amneziawg-tools | `AWG_TOOLS_REF` / `AWG_TOOLS_COMMIT` in the `Dockerfile` | GPL-2.0-only |
| `ip` (iproute2), `nft` (nftables), Alpine base | Alpine Linux packages | the Alpine release in the `Dockerfile` | GPL-2.0 and others, per package |

The `awg` binary is built unmodified from the commit the `Dockerfile` names and verifies;
its complete corresponding source is that commit of the upstream repository. On request
the maintainer provides the same source directly: open an issue at
https://github.com/CorneiZeR/awg-gate/issues.

The Go modules linked into `awg-gate` and `amneziawg-go` are listed, with versions, in the
SBOM attached to every released image.
