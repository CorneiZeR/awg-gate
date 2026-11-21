# gate.yaml

The hub's whole desired state. Every key is checked: a key the hub does not know is an
error, so a misspelt `expries:` can never grant access forever. A file that fails any rule
below is not applied at all; a running hub keeps the last good one.

```yaml
endpoint: vpn.example.com:443

servers:
  shop-dev:
    address: 10.247.0.10
    public_key: q2bH7...=
  blog-dev:
    address: 10.247.0.11
    public_key: Xc0Lm...=
    ports: [443, 80]

people:
  alice:
    reach: [shop-dev, blog-dev]
    expires: 2027-03-31
    devices:
      laptop: {address: 10.247.1.10, public_key: 8kQ2v...=}
      phone:  {address: 10.247.1.11, public_key: Pz1aD...=}
  bob:
    reach: ["*"]
    devices:
      laptop: {address: 10.247.1.20, public_key: Rt5sW...=}
```

## Top level

| Key | Default | Meaning |
|---|---|---|
| `endpoint` | **required** | `host:port` clients dial: the hub's public name (or IP) and the published UDP port |
| `listen_port` | `443` | UDP port inside the container |
| `interface` | `awg0` | tunnel interface name, 1-15 characters |
| `profile` | `v1` | obfuscation profile, `v1` or `v3` - see [Obfuscation](Obfuscation.md); fixed at the hub's first start |
| `mtu` | `1280` | 1280-1420; written into issued configurations |
| `keepalive` | `25` | seconds, 0-600; written into issued configurations (0: none) |
| `network` | `10.247.0.0/16` | the whole access network |
| `servers_subnet` | `10.247.0.0/24` | servers' addresses; inside `network` |
| `people_subnet` | `10.247.1.0/24` | devices' addresses; inside `network`, not overlapping `servers_subnet` |
| `hub_address` | first address of `servers_subnet` | the hub's own address |
| `servers` | - | map of server name → server |
| `people` | - | map of person name → person |

`network`, `servers_subnet`, `people_subnet` and `hub_address` are written into issued
configurations: decide them before issuing the first one.

## Servers

| Key | Required | Meaning |
|---|---|---|
| `address` | yes | a host address in `servers_subnet` |
| `public_key` | yes | the spoke's public key, base64 |
| `ports` | no | the only TCP/UDP ports people may reach (ping is always allowed to those who reach it); every port when unset |

## People

| Key | Required | Meaning |
|---|---|---|
| `reach` | no | server names, or `["*"]` for every server, present and future; empty reaches nothing |
| `expires` | no | `YYYY-MM-DD`: the last day of access, UTC, inclusive |
| `disabled` | no | `true` keeps the person in the file - keys and addresses reserved - but off the hub; removing it lets the same configurations connect again |
| `devices` | yes, at least one | map of device name → `{address, public_key}`, and `disabled: true` for one device alone; addresses in `people_subnet` |

## Rules the hub enforces

- Names of servers, people and devices: 1-32 characters, lowercase letters, digits and
  inner `-`.
- Every address is a host address (not the subnet's first or last) in the right subnet,
  and used once - the hub's own and a disabled device's included.
- Every public key is a valid 32-byte key and used once.
- `reach` names existing servers; `"*"` stands alone.
- Subnets are IPv4, `/30` or larger, inside `network`, without host bits, not overlapping.

`awg-gate check --config gate.yaml` applies exactly these rules without a hub; run it in
the CI of the repository that holds the file.

## Editing

`awg-gate issue`, `revoke`, `disable` and `enable` with `--gate gate.yaml` edit the
file in place: comments, order and formatting of the rest survive, and the result is
validated before it is written. Hand edits are just as fine.
