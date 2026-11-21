# Troubleshooting

Start with `awg-gate status` on the hub: it shows which peers have a recent handshake.

## No handshake

The client shows no *latest handshake*; the device is missing from the hub's status or
shows `never`.

| Cause | Check |
|---|---|
| UDP to the hub is blocked | the provider's firewall allows the published port; `nc -u -v vpn.example.com 443` from outside proves little (UDP), so try another network (mobile data) |
| `endpoint` points elsewhere | `dig +short vpn.example.com` gives the hub's IP; the record is **DNS only** |
| the device is not in `gate.yaml` on the hub | `awg-gate check` on the deployed file; the hub log after the last change (`applied ... devices=`) |
| the person has expired | `awg-gate check` lists `expired, not admitted` |
| a plain WireGuard client | the WireGuard app cannot speak AmneziaWG; use AmneziaVPN or AmneziaWG |
| an app too old for the profile | profile `v3` needs AmneziaWG 3 clients ([Obfuscation](Obfuscation.md)) |
| the configuration is from another hub | its `PublicKey` in `[Peer]` differs from `awg-gate hub-info`'s |
| a commercial VPN's kill switch | allow AmneziaVPN/AmneziaWG in its split tunnelling ([Clients](Clients.md#with-a-commercial-vpn)) |

## Handshake, but the site does not open

| Cause | Check |
|---|---|
| the person does not reach that server | `awg-gate check`: is `alice/laptop → shop-dev` listed? |
| the port is not in the server's `ports` | the same table's PORTS column |
| the spoke drops the port | `AWG_GATE_ALLOW_PORTS` on the server includes it |
| the spoke is down | `awg-gate status` on the server: healthy, recent handshake |
| a host service behind UFW | `ufw allow in on awg0 to any port 443 proto tcp` ([Servers](Servers.md#ufw-and-host-services)) |
| the service listens on `127.0.0.1` only | it must listen on all addresses or on the tunnel address |
| the proxy answers 403 | its allowlist names `10.247.0.0/16` (your `network`), and judges the peer, not `X-Forwarded-For` |

`ping 10.247.0.1` (the hub) and `ping 10.247.0.10` (the server) from the device narrow it
down: the first fails - the tunnel; the second fails - the rules or the spoke; both work -
the proxy or the service.

## The name does not resolve

Resolves on mobile data, not at home or in the office: the local resolver drops private
answers - see [Cloudflare](Cloudflare.md#resolvers-that-refuse-private-answers). Resolves
nowhere: the record is missing or still proxied.

## Certificates stopped renewing

HTTP-01 cannot reach a private address. Switch to DNS-01 ([Cloudflare](Cloudflare.md#certificates-by-dns-01)).
A token that fails with an authentication error usually lacks `Zone · Zone · Read`, or its
IP filter misses the address the server calls out from (often its IPv6).

## Pages stall half-loaded, ping works

An MTU too high for the path. Keep `mtu: 1280` (the default) and reissue if it was raised.

## Containers that will not start

| Message | Fix |
|---|---|
| `amneziawg-go exited before it was ready (is /dev/net/tun passed in and NET_ADMIN granted?)` | `devices: ["/dev/net/tun:/dev/net/tun"]` and `cap_add: [NET_ADMIN]` |
| `IPv4 forwarding is off in this network namespace` | the hub's `sysctls: net.ipv4.ip_forward: "1"` |
| `gate.yaml asks for profile v3 but this hub was set up with v1` | intended? see [Obfuscation](Obfuscation.md#changing-the-parameters); otherwise fix `profile` |
| `lockdown ... looks like a host's` | `--lockdown` is for a sidecar; on a host use `AWG_GATE_ALLOW_PORTS` |
| `nft: ...` | the kernel lacks nf_tables (very old kernels, some OpenVZ VPS); use a KVM VPS |
| `gate.yaml: ... field expries not found` | a typo in a key: every key is checked, see [gate.yaml](Gate-yaml.md) |

## A change to gate.yaml does nothing

The hub reads `/config/gate.yaml` from a mounted **directory**. With a single-file bind
mount, the container keeps seeing the file it started with after the file is replaced.
Check `docker compose exec hub cat /config/gate.yaml`, and the hub's log for `rejected`.
