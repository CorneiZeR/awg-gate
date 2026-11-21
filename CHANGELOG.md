# Changelog

All notable changes to this project are documented here. The project follows
[semantic versioning](https://semver.org/): renaming a `gate.yaml` key, a flag or an
environment variable, or changing a default that issued configurations carry, is a major
version.

## Unreleased

- **Dependency fixes land without waiting.** Dependabot checks daily, and its patch and
  minor updates merge by themselves once every check (tests, end-to-end, govulncheck,
  Trivy) is green; the published image is rescanned daily instead of weekly.
- The repository carries a code of conduct, an OpenSSF Scorecard (badge in the README),
  Discussions for questions, and a feature-request template.

## 0.1.0 — 2026-11-21

The first release.

- **The hub** (`awg-gate hub`): AmneziaWG in userspace, peers and per-person access rules
  from `gate.yaml`, re-read every 30 seconds and on `SIGHUP`; a file that fails validation
  is rejected and the last good one kept; people whose `expires` day has ended are dropped
  without a restart.
- **Spokes and Linux clients** (`awg-gate up`): any WireGuard-format configuration;
  `--allow-ports` limits what the tunnel reaches, `--lockdown` closes a sidecar to
  everything but the tunnel; an endpoint name that does not resolve yet is retried rather
  than fatal; healthy only with a recent handshake.
- **Operator commands**: `hub-info`, `issue` (keys generated locally, configuration and QR
  code, `gate.yaml` edited in place with comments kept), `revoke`, `check`, `status`.
- **Obfuscation profiles** `v1` (every Amnezia client) and `v3` (AmneziaWG 3: header
  ranges, transport padding, header protection).
- **The image**: `linux/amd64` and `linux/arm64`, AmneziaWG built from source in CI with
  current `golang.org/x` modules, no known vulnerabilities, signed, with SBOM and provenance.
- **Documentation**: concepts, hub, servers, Traefik / nginx / Caddy, Cloudflare (DNS and
  DNS-01 certificates), people, clients, obfuscation, operations, security model,
  troubleshooting and reference pages.
