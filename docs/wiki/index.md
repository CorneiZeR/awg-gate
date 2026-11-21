# awg-gate

An [AmneziaWG](https://docs.amnezia.org/documentation/amnezia-wg/) access network for
reaching private servers - dev and staging environments, admin panels, internal tools -
and nothing else.

```
  alice's laptop ─┐                          ┌─ shop-dev   10.247.0.10
  alice's phone  ─┤   AmneziaWG, udp/443     │
  bob's laptop   ─┼──────────►  hub  ◄───────┼─ blog-dev   10.247.0.11
                  │         10.247.0.1       │
       people 10.247.1.0/24      access rules: who reaches which server, on which ports
```

A dev environment put on the internet "with a password" leaks: its names are in
Certificate Transparency logs within minutes, basic auth collides with the
`Authorization` header the API's own tokens use, and an identity-aware proxy ties the
team to one vendor and names your identity provider on every login redirect. awg-gate
decides access at the network instead: the servers answer only addresses of the access
network, and only the people `gate.yaml` lets through reach them.

## Where to start

| You want to | Read |
|---|---|
| understand the design and its limits | [Concepts](Concepts.md), [Security model](Security-model.md) |
| set it up end to end | [Quick start](Quick-start.md) |
| run the hub | [Hub](Hub.md) |
| connect a server and close its sites to everyone else | [Servers](Servers.md), [Traefik, nginx, Caddy](Services.md) |
| move names and certificates in Cloudflare | [Cloudflare](Cloudflare.md) |
| give someone access, or take it away | [People and devices](People.md) |
| connect from a laptop or a phone | [Clients](Clients.md) |
| choose how hard the traffic is to recognise | [Obfuscation](Obfuscation.md) |
| keep it running | [Operations](Operations.md), [Troubleshooting](Troubleshooting.md) |
| look a setting up | [gate.yaml](Gate-yaml.md), [CLI](CLI.md), [Docker](Docker.md) |
| work on awg-gate itself | [Development](Development.md) |
