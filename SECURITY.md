# Security policy

## Reporting a vulnerability

Please report security issues privately through GitHub's
[private vulnerability reporting](https://github.com/CorneiZeR/awg-gate/security/advisories/new),
not in a public issue. Expect an acknowledgement within a few days.

Vulnerabilities in AmneziaWG itself belong upstream
([amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go),
[amneziawg-tools](https://github.com/amnezia-vpn/amneziawg-tools)); a report here is still
welcome when the image needs a rebuild.

## Supported versions

The latest minor release receives security fixes, and its image is rebuilt when a fix
lands in a dependency or the base image.

## What the project promises

- A server on the access network answers only the devices `gate.yaml` lets through, on the
  ports it allows; people never reach people, servers never reach servers or open
  connections to people.
- The hub never holds anyone's private key: key pairs are generated where `awg-gate issue`
  runs, and only public keys reach `gate.yaml` and the hub.
- A `gate.yaml` that fails validation is never applied.
- No log line contains a private key, a configuration file or traffic.
- A released image has no known fixable HIGH or CRITICAL vulnerability at release, is
  signed with cosign (keyless, GitHub OIDC) and carries an SBOM and provenance.

The [Security model](https://corneizer.github.io/awg-gate/Security-model/) page describes
the threat model and its limits.
