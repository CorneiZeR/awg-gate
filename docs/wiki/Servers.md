# Servers

A server joins the access network as a **spoke**: a tunnel to the hub and an address in
`servers_subnet`. Two ways to run it:

| | Host spoke | Sidecar |
|---|---|---|
| runs in | the host's network namespace | one service's namespace |
| reachable over the tunnel | anything on the host: a host nginx, published container ports | that one service |
| typical use | a dev server with Traefik or nginx in front of several apps | one container without a proxy: an internal tool, a database console |
| what closes it to the internet | the proxy's allowlist ([recipes](Services.md)) | `--lockdown` |

## Issue it

On the machine you administer from (see [People](People.md) for the alias):

```shell
awg-gate issue server shop-dev --hub-info hub.json --gate gate.yaml --ports 443,80
```

`--ports` is optional: the hub then lets people reach only those TCP/UDP ports on it
(plus ping). Without it, people reach every port the server answers on its tunnel address.
Deploy the updated `gate.yaml` to the hub, and `shop-dev.conf` to the server - it holds the
server's private key.

## Host spoke

[`deploy/spoke/compose.yaml`](https://github.com/CorneiZeR/awg-gate/blob/master/deploy/spoke/compose.yaml):

```shell
mkdir -p /opt/awg-gate/config && cd /opt/awg-gate
curl -fsSLO https://raw.githubusercontent.com/CorneiZeR/awg-gate/master/deploy/spoke/compose.yaml
install -m 600 /path/to/shop-dev.conf config/awg.conf
docker compose up -d
```

The host gets an `awg0` interface with `10.247.0.10/32` and a route to `people_subnet`
through it. Requests from people arrive with their own source addresses
(`10.247.1.x`), so a proxy's allowlist and its access logs see who is who.

### Limit the ports

`AWG_GATE_ALLOW_PORTS: "80,443"` (the compose file's default) drops everything else that
comes over the tunnel - whether a host process or a published container port would answer
it. Leave it set: without it, every port the host listens on (a database published on
`0.0.0.0`, an exporter) is reachable by everyone with access to the server. It is a second
fence behind the hub's `ports`, kept on the server itself.

### UFW and host services

Ports **published by Docker** (a Traefik container's `443`) are reached over the tunnel
with no UFW rule - Docker's chains run before UFW's. A **host** service (nginx installed
from packages) is behind UFW, which by default drops what arrives on `awg0`:

```shell
ufw allow in on awg0 to any port 443 proto tcp
ufw allow in on awg0 to any port 80 proto tcp
```

### What must not use the public names

Once `app.dev.example.com` resolves to `10.247.0.10`, anything on the server that calls it
by name - server-side rendering fetching its own API, a worker calling a webhook URL, a
health check - would go to the tunnel address from a Docker network and be refused. Point
internal calls at service names (`http://back:8000`) as production should anyway.

### Never lockdown on a host

`--lockdown` drops every new connection that does not come over the tunnel - on a host
that is SSH, the CI deploy and the sites. `awg-gate up` refuses it when it sees `docker0`.

## Sidecar

[`deploy/sidecar/compose.yaml`](https://github.com/CorneiZeR/awg-gate/blob/master/deploy/sidecar/compose.yaml):
the service shares the tunnel container's network namespace and is reached at the tunnel
address on its own port:

```yaml
services:
  awg-gate:
    image: ghcr.io/corneizer/awg-gate:0.2.0
    command: ["up", "--lockdown"]
    volumes: ["./config:/config:ro"]
    devices: ["/dev/net/tun:/dev/net/tun"]
    cap_drop: [ALL]
    cap_add: [NET_ADMIN]
    read_only: true
    tmpfs: ["/run"]
  app:
    image: your/internal-tool
    network_mode: service:awg-gate
    depends_on: {awg-gate: {condition: service_healthy}}
```

With `--lockdown` nothing that does not come through the tunnel gets in: do not publish
ports for it. The service still makes outbound connections normally.

## Check it

```shell
docker compose exec awg-gate awg-gate status   # up on awg0, healthy; the hub's last handshake
```

From a person's device on the network: `ping 10.247.0.10` (allowed whenever the person
reaches the server), then the site. The container is `healthy` only while the tunnel has a
recent handshake with the hub, so `depends_on: condition: service_healthy` and monitoring
both mean "the tunnel works", not "the process runs".

If the hub's name does not resolve at boot - DNS not up yet, the hub's record moved - the
spoke keeps the tunnel and retries the name every 15 seconds until it connects.

## Remove a server

```shell
awg-gate revoke server shop-dev --gate gate.yaml
```

It leaves every person's `reach` too. Deploy `gate.yaml`, then stop the spoke and delete
`config/awg.conf` on the server.
