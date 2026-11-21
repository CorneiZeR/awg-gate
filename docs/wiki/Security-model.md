# Security model

## What awg-gate promises

- **A server answers only those `gate.yaml` lets through.** Every packet between a person
  and a server crosses the hub; the hub forwards it only when a rule names that device and
  that server (and, with `ports`, that port). Everything else arriving on the tunnel is
  dropped.
- **Addresses are authenticated.** AmneziaWG, like WireGuard, accepts a packet from a peer
  only with a source address that peer owns (*cryptokey routing*). A rule for
  `10.247.1.10` is a rule for the holder of that device's private key.
- **People never reach people, servers never reach servers, and servers never open
  connections to people.** A compromised dev server cannot be used to attack the laptops
  of the people who use it, nor its neighbours.
- **The hub never holds anyone's private key.** Key pairs are generated where `issue` runs;
  `gate.yaml` and the hub carry public keys only. A stolen hub cannot impersonate a person.
- **Access ends by itself.** An expired person is dropped by the hub within 30 seconds of
  the end of their last day, without anyone remembering to do it.
- **A broken change cannot open anything.** `gate.yaml` is validated whole - unknown keys,
  duplicate addresses or keys, names that do not exist - and a file that fails is not
  applied at all.

## What it does not do

- **It is not authentication for the application.** Anyone on the network who reaches a
  server reaches everything on it that its `ports` (and the spoke's `AWG_GATE_ALLOW_PORTS`)
  allow. The application's own logins still matter; the network removes the internet from
  the list of people who can try them.
- **It does not hide that a hub exists.** The hub's address and UDP port are public;
  obfuscation hides the protocol, not the flow.
- **A device is as safe as the person's device.** A stolen unlocked laptop with the tunnel
  on is on the network until its device is revoked.
- **The hub is trusted.** Whoever controls the hub can change the rules and see traffic
  metadata (who talks to which server, when, how much). The contents stay TLS-encrypted end
  to end as long as the sites use HTTPS, which the [Cloudflare](Cloudflare.md) setup keeps.

## Trust in the image

- Built in GitHub Actions from this repository and the pinned upstream sources:
  `amneziawg-go` as a Go module dependency in `go.mod`, `amneziawg-tools` at a commit the
  build verifies. No compiler is left in the image; it runs nothing but `awg-gate`,
  `amneziawg-go`, `awg`, `ip` and `nft`.
- Release images are multi-arch (`linux/amd64`, `linux/arm64`), signed keylessly with
  cosign through GitHub's OIDC identity, and carry an SBOM and SLSA provenance attestation
  ([verify](Operations.md#upgrades)).
- **No known vulnerabilities at release.** CI fails on any fixable HIGH or CRITICAL finding
  (Trivy) in the image and on any reachable vulnerability (`govulncheck`) in either Go
  binary; the published image is rescanned daily. Dependabot checks Go modules - including
  AmneziaWG and the `golang.org/x` modules it links - base images, compose images and
  actions daily, and its patch and minor updates merge by themselves once every check is
  green; a fix that changes the image ships as a patch release. A weekly job proposes new
  upstream AmneziaWG tools releases.
- The containers run with every capability dropped but `NET_ADMIN`, a read-only root
  filesystem and `no-new-privileges`.

## Reporting a vulnerability

Privately, through
[GitHub's private vulnerability reporting](https://github.com/CorneiZeR/awg-gate/security/advisories/new).
See [SECURITY.md](https://github.com/CorneiZeR/awg-gate/blob/master/SECURITY.md).
