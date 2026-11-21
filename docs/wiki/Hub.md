# Hub

## Where it runs

On a small VPS of its own, not on one of the servers it gives access to. The hub sees every
connection's endpoints and holds the network's rules; a project's box is the wrong place
for that, and a hub there would fall with the box. Requirements:

- a public IPv4 address (not behind CGNAT) and inbound UDP on one port;
- Linux with Docker Engine and Compose v2, and `/dev/net/tun` (every common VPS kernel has
  it; awg-gate runs AmneziaWG in userspace, so no kernel module is needed);
- 1 vCPU and 512 MB RAM; the container idles at a few megabytes.

## The port

`443/udp` by default: it is what HTTP/3 (QUIC) uses, so it is rarely blocked and draws no
attention. If a network blocks it, publish another port - `"51443:443/udp"` - and set
`endpoint: vpn.example.com:51443`; the container keeps listening on `listen_port`.

Allow the port in the provider's firewall. A port Docker publishes bypasses UFW (Docker's
own chains come first), so `ufw` neither opens nor closes it.

## Install

[`deploy/hub/compose.yaml`](https://github.com/CorneiZeR/awg-gate/blob/master/deploy/hub/compose.yaml):

```shell
mkdir -p /opt/awg-gate/config && cd /opt/awg-gate
curl -fsSLO https://raw.githubusercontent.com/CorneiZeR/awg-gate/master/deploy/hub/compose.yaml
curl -fsSL -o config/gate.yaml https://raw.githubusercontent.com/CorneiZeR/awg-gate/master/deploy/hub/gate.example.yaml
$EDITOR config/gate.yaml                                # endpoint, and any defaults to change
docker compose run --rm hub hub-info --init > hub.json  # draws the hub key and parameters, once
docker compose up -d
```

What the compose file sets, and why:

| Setting | Why |
|---|---|
| `cap_add: [NET_ADMIN]`, `cap_drop: [ALL]` | creating the tunnel, addresses and nftables rules is all it needs |
| `devices: /dev/net/tun` | the userspace tunnel |
| `sysctls: net.ipv4.ip_forward=1` | the hub forwards between peers; set inside the container's namespace only, not on the host |
| `./config:/config:ro` - a **directory** | gate.yaml is replaced on every edit; a single-file bind mount would keep the old inode |
| `data:/data` | the hub key and obfuscation parameters - **back this up** |
| `read_only`, `tmpfs: /run`, `no-new-privileges` | nothing in the image needs to write anywhere else |

The hub refuses to start without IP forwarding, and with a `gate.yaml` that does not
validate.

## gate.yaml is the access list

Keep it in a private git repository; every change is a reviewed commit. Add and remove
entries with `awg-gate issue` and `awg-gate revoke` (see [People](People.md)); they keep
your comments and validate the result. Deploy it by copying it into `config/` - `scp`,
`rsync`, an Ansible `copy` task, a CI job.

The hub re-reads the file every 30 seconds (`--interval`) and on `SIGHUP`:

```shell
docker compose kill -s HUP hub
```

A file that does not validate is **rejected**: the hub logs why, keeps serving the last
good one and `awg-gate status` shows `gate.yaml REJECTED`. Changes to `interface`,
`listen_port`, `profile`, `mtu`, `network` or `hub_address` need a restart; the hub warns
and applies the rest.

## hub.json

`awg-gate hub-info` prints the hub's public key, endpoint, subnets and obfuscation
parameters - everything `issue` needs, and no private key. It is not a credential, but keep
it out of public places: the parameters are what makes the hub's traffic unrecognisable.
Regenerate it any time:

```shell
docker compose run --rm hub hub-info > hub.json
```

## Backups and moving

The `data` volume (two small files) is the hub's identity. With it, a hub moves to another
VPS by restoring the volume, copying `config/`, starting the container and pointing the
DNS name at the new IP; every issued configuration keeps working. Without it, every
configuration has to be reissued.

```shell
docker run --rm -v awg-gate_data:/data -v "$PWD:/backup" alpine tar czf /backup/hub-data.tgz -C /data .
```

The archive holds the hub's private key: store it like one.

## Host networking

The hub works in its own network namespace (the default above), which keeps its rules and
forwarding away from the host. `network_mode: host` also works - every rule awg-gate writes
matches only the tunnel interface, and its chains never drop the host's own traffic - but
then the host itself needs `net.ipv4.ip_forward=1`.
