# Operations

## Status

```shell
docker compose exec hub awg-gate status
```

```text
hub on awg0, healthy (reported 12s ago)
PEER          ADDRESS      ENDPOINT             HANDSHAKE  RX        TX        EXPIRES
alice/laptop  10.247.1.10  198.51.100.7:51234   41s ago    1.2 MiB   18.4 MiB  2027-03-31
blog-dev      10.247.0.11  203.0.113.21:43122   1m32s ago  3.0 MiB   412 KiB   -
shop-dev      10.247.0.10  203.0.113.20:39011   18s ago    22.1 MiB  1.9 MiB   -
```

On a spoke, the same command shows its tunnel to the hub. `awg-gate check --config
gate.yaml` (no running hub needed) prints who reaches what, and who has expired.

## Health

Both roles write a report every interval, and `awg-gate health` - the image's
`HEALTHCHECK` - exits non-zero when it is stale or bad:

| Role | Healthy when |
|---|---|
| hub | the tunnel device runs and the last `gate.yaml` change was applied |
| spoke, client | a handshake with the hub within the last 3 minutes |

A spoke that is `unhealthy` has lost the hub; `depends_on: condition: service_healthy`
and any container monitor read it directly. A hub with a rejected `gate.yaml` stays healthy
(it keeps serving the last good file) and says so in `status` and in its log.

## Logs

Plain text on stderr, one line per event:

```text
level=INFO msg=applied servers=2 devices=5 rules=7 expired=carol/laptop disabled=""
level=ERROR msg="gate.yaml rejected, still serving the last good one" err="person bob: reach names \"shop\", which is not a server"
level=WARN msg="tunnel down" reason="last handshake 3m10s ago"
```

`AWG_GATE_LOG_LEVEL=debug` adds AmneziaWG's own messages. Neither level logs a private key,
a configuration file or traffic.

## Upgrades

Images are tagged by version (`0.2.0`), minor series (`0.2`) and `latest`; pin a version
and let Dependabot or Renovate raise it. Then:

```shell
docker compose pull && docker compose up -d
```

A restarted hub keeps its key and parameters: clients re-handshake within seconds. The
[changelog](https://github.com/CorneiZeR/awg-gate/blob/master/CHANGELOG.md) marks anything
that needs action.

Verify an image before running it:

```shell
cosign verify ghcr.io/corneizer/awg-gate:0.2.0 \
  --certificate-identity-regexp '^https://github.com/CorneiZeR/awg-gate/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Backups

| What | Where | Without it |
|---|---|---|
| hub key and obfuscation parameters | the hub's `data` volume | every configuration must be reissued |
| `gate.yaml` | your git repository | access must be re-granted by hand |

Issued configurations are deliberately not backed up: a lost one is replaced by issuing a
new device.

## Rotating the hub key

Rarely needed (a hub's data volume that leaked). Delete `hub.key` from the volume the way
[Obfuscation](Obfuscation.md#changing-the-parameters) deletes `params.json`, run
`hub-info --init`, and reissue every configuration.

## Capacity

AmneziaWG runs in userspace (`amneziawg-go`). On one vCPU it moves hundreds of megabits per
second - far beyond what people browsing dev sites need. Peers cost nothing while idle.
`people_subnet` /24 holds 254 devices; widen it in `gate.yaml` before issuing if you need
more.
