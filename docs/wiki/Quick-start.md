# Quick start

From nothing to a dev site only the team can open: a hub, one server, one person.
Names used below - replace them with yours:

| | |
|---|---|
| hub's public name | `vpn.example.com` |
| the dev server | `shop-dev`, public IP `203.0.113.20` |
| its site | `app.dev.example.com` |
| the person | `alice`, on a laptop |

## 1. The hub

A VPS with a public IPv4 address, Docker and Compose. 1 vCPU and 512 MB are plenty.

In Cloudflare, add **`vpn.example.com` A → the hub's IP, DNS only** (grey cloud) - see
[Cloudflare](Cloudflare.md#the-hub). In the VPS provider's firewall allow `443/udp` (and
SSH).

```shell
mkdir -p /opt/awg-gate/config && cd /opt/awg-gate
curl -fsSLO https://raw.githubusercontent.com/CorneiZeR/awg-gate/master/deploy/hub/compose.yaml
curl -fsSL -o config/gate.yaml https://raw.githubusercontent.com/CorneiZeR/awg-gate/master/deploy/hub/gate.example.yaml
$EDITOR config/gate.yaml          # endpoint: vpn.example.com:443
docker compose run --rm hub hub-info --init > hub.json
docker compose up -d
docker compose logs hub      # "applied servers=0 devices=0 rules=0"
```

`hub-info --init` draws the hub's key and obfuscation parameters into the `data` volume
and prints what clients need. Copy `hub.json` and `config/gate.yaml` to the machine you
administer from; keep `gate.yaml` in a private git repository.

## 2. Issue the server and the person

On your machine, in the directory with `gate.yaml` and `hub.json`:

```shell
alias awg-gate='docker run --rm -it -v "$PWD:/work" -w /work --user "$(id -u):$(id -g)" ghcr.io/corneizer/awg-gate:0.2.0'

awg-gate issue server shop-dev --hub-info hub.json --gate gate.yaml --ports 443,80
awg-gate issue person alice laptop --hub-info hub.json --gate gate.yaml --reach shop-dev
```

Each command generates a key pair **on your machine**, adds the public half to `gate.yaml`
with the next free address, and writes the private configuration: `shop-dev.conf`, and
`alice-laptop.conf` with `alice-laptop.png`. The hub never sees a private key.

Put the new `gate.yaml` on the hub (`scp gate.yaml hub:/opt/awg-gate/config/`). The hub
re-reads it within 30 seconds; `docker compose kill -s HUP hub` applies it at once.

## 3. Connect the server

On `shop-dev`:

```shell
mkdir -p /opt/awg-gate/config && cd /opt/awg-gate
curl -fsSLO https://raw.githubusercontent.com/CorneiZeR/awg-gate/master/deploy/spoke/compose.yaml
install -m 600 /path/to/shop-dev.conf config/awg.conf
docker compose up -d
docker compose exec awg-gate awg-gate status    # "up on awg0, healthy"
ip -4 addr show awg0                            # 10.247.0.10/32
```

## 4. Close the site to everyone else

The site must answer only the access network. With Traefik, one middleware on its routers:

```yaml
labels:
  - traefik.http.middlewares.vpn-only.ipallowlist.sourcerange=10.247.0.0/16
  - traefik.http.routers.app.middlewares=vpn-only
```

With nginx, two lines in its `server` block: `allow 10.247.0.0/16; deny all;`.
[Traefik, nginx, Caddy](Services.md) has the full recipes.

## 5. DNS and the certificate

In Cloudflare: **`app.dev.example.com` A → `10.247.0.10`, DNS only**, and a scoped API
token for DNS-01 so the server keeps getting certificates without being reachable.
[Cloudflare](Cloudflare.md) walks through both.

## 6. Alice connects

Alice installs **AmneziaVPN** (or AmneziaWG), imports `alice-laptop.conf` or scans
`alice-laptop.png`, and switches it on. Then:

```shell
ping 10.247.0.1                   # the hub answers
curl -I https://app.dev.example.com
```

From anywhere else, `https://app.dev.example.com` resolves to `10.247.0.10` and connects
nowhere. Deliver the file the way you deliver a password, and delete your copy.
