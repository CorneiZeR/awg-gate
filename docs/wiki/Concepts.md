# Concepts

## The shape: one hub, spokes and people

```
                               ┌──────────── hub ────────────┐
  person device ── tunnel ───► │ awg0 10.247.0.1             │ ◄── tunnel ── server (spoke)
  10.247.1.x                   │ nftables: forward only what │              10.247.0.x
                               │ gate.yaml grants            │
                               └─────────────────────────────┘
```

- The **hub** is a small VPS of its own with a public IP. It is the only thing the
  internet can reach: one UDP port.
- A **server** (spoke) keeps a tunnel open to the hub and gets an address in
  `servers_subnet`. Nothing on the server has to be public for people to reach it.
- A **person** has one or more **devices**; each device has its own key and address in
  `people_subnet`. One configuration per device reaches every server the person may reach.

Every packet between a person and a server crosses the hub, and the hub forwards it only
when a rule allows it:

| From → to | Allowed |
|---|---|
| a device → a server in its person's `reach` | yes, on the server's `ports` (all when unset) |
| a server → a device | only replies; a server never opens a connection to a person |
| a device → another device | never |
| a server → another server | never (a spoke does not even route the servers' subnet) |
| anything → the internet through the hub | never: the hub is not an exit node |
| a device → the hub | ping only |

Hub-and-spoke rather than a mesh because access is decided in one place, by one file,
reviewed like code. A mesh (Tailscale, Netbird, Nebula) needs a coordination service and
agents that each enforce policy; here the servers need nothing but a tunnel.

## Why AmneziaWG

WireGuard is easy to recognise on the wire - fixed message types and sizes - and is
blocked by DPI in several countries. [AmneziaWG](https://docs.amnezia.org/documentation/amnezia-wg/)
is a WireGuard fork that adds junk packets, paddings and custom message headers, so the
handshake looks like noise. It keeps WireGuard's cryptography and its *cryptokey routing*:
a packet from a peer is accepted only with a source address that peer owns, which is what
makes per-address rules on the hub trustworthy. A plain WireGuard client cannot talk to an
AmneziaWG hub at all (the end-to-end test proves it).

Clients exist for every platform: AmneziaVPN and AmneziaWG for iOS, Android, macOS and
Windows, and Linux through this image or the AmneziaWG tools. See [Clients](Clients.md).

## Split tunnel, no DNS

An issued configuration routes exactly one subnet - a person's device routes
`servers_subnet`, a server routes `people_subnet` - and carries **no `DNS =` line**:

- the person's other traffic never touches the hub, which keeps it from becoming a VPN
  provider with the bandwidth, logging and liability that brings;
- names resolve through the normal public DNS. A dev name such as `app.dev.example.com`
  has an A record pointing at the server's tunnel address, `10.247.0.10`: anyone can look
  it up, only people on the access network can connect to it;
- because the names are real public names, certificates come from Let's Encrypt as usual
  (by DNS-01) and every device trusts them: no private CA to install on phones.

[Cloudflare](Cloudflare.md) shows the records and the certificate setup.

## The address plan

The default network is `10.247.0.0/16`: servers in `10.247.0.0/24` (the hub takes
`10.247.0.1`), people in `10.247.1.0/24`. It stays clear of Docker's `172.16.0.0/12`,
Tailscale's `100.64.0.0/10`, the `10.8.0.0/24` OpenVPN setups habitually use and the
`192.168.x` and `10.0.x` ranges of home routers. Change it in [gate.yaml](Gate-yaml.md)
before issuing anything; addresses are written into issued configurations.

## What stays public

Taking a dev environment off the internet does not take the server off it:

- **SSH and CI deploys** keep using the server's public address.
- **Certificates** are issued by DNS-01, which needs no inbound connection.
- **Webhooks** a third party calls (payment providers, a Telegram bot in webhook mode)
  cannot reach a private address. Switch the dev bot to polling, point the provider at a
  separate public endpoint for that one path, or leave that integration to staging.

## Why not basic auth, Cloudflare Access, an identity proxy

- **Basic auth** occupies the `Authorization` header, which an API's own Bearer tokens and
  API keys use; a dev API behind it stops working the way production works.
- **Cloudflare Access / an identity-aware proxy** ties every developer to one vendor,
  redirects every request through a login page that names your identity provider, and
  still exposes the origin to whatever reaches it by IP.
- **A VPN** leaves the application untouched - the same headers, tokens, Swagger, admin -
  and closes the environment to everyone who is not on it.
