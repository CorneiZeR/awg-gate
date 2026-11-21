# Obfuscation

AmneziaWG changes what WireGuard looks like on the wire; the cryptography is WireGuard's.
The hub draws its parameters once, on its first start, from the `profile` in `gate.yaml`,
and keeps them in its data volume. Every issued configuration carries them; a client with
different values cannot complete a handshake.

## Profiles

| | `v1` (default) | `v3` |
|---|---|---|
| junk packets before each handshake (`Jc`, `Jmin`, `Jmax`) | 4-8 packets of 40-70 bytes | same |
| handshake paddings (`S1`, `S2`) | 15-150 bytes, sizes never equal | same |
| cookie and transport paddings (`S3`, `S4`) | - | 15-64 and 12-32 bytes |
| message headers (`H1`-`H4`) | four distinct fixed values | four non-overlapping ranges, a fresh value per packet |
| header protection (`HeaderProtectionKey`) | - | yes |
| clients | every AmneziaVPN and AmneziaWG client since 2024 | AmneziaWG 3: AmneziaVPN 5, AmneziaWG for Android 3 and later |

Choose `v3` when every client you support has AmneziaWG 3; it leaves no fixed value in any
packet. Choose `v1` (the default) when someone may be on an older app. Both profiles run the
full end-to-end test.

Junk packets stay below 70 bytes so they never exceed an MTU and fragment - fragmented
UDP is itself a signal.

## MTU

Issued configurations set `MTU = 1280`, the IPv6 minimum every path carries. It leaves room
for the AmneziaWG overhead, `v3`'s transport padding, and a commercial VPN the tunnel may
run inside. On a clean path you may raise `mtu` (up to 1420) for a little throughput;
symptoms of too high a value are pages that stall half-loaded while `ping` works.

## Changing the parameters

The parameters - and the profile - are part of every issued configuration. Changing them
means reissuing every configuration, servers included. When that is the intent:

```shell
docker compose stop hub
docker run --rm -v awg-gate_data:/data alpine rm /data/params.json   # the key stays
$EDITOR config/gate.yaml                                             # profile: v3
docker compose run --rm hub hub-info --init > hub.json
docker compose up -d
```

then issue every server and device again. The hub refuses to start when `gate.yaml` names a
profile other than the stored one, so a typo cannot do this by accident.

## What obfuscation does not do

It hides that this is WireGuard; it does not hide that it is *a* UDP flow between a device
and the hub's address. A censor that blocks the hub's IP outright blocks it; move the hub
(change the `vpn` DNS record) or reach it through a commercial VPN on a desktop.
