# CLI

One binary, `awg-gate`, is the image's entrypoint. Flags come **after** the positional
names (`issue person alice laptop --gate gate.yaml`). Every daemon flag also reads an
`AWG_GATE_*` environment variable, named below; a flag given wins.

## hub

Runs the hub.

| Flag | Variable | Default | |
|---|---|---|---|
| `--config` | `AWG_GATE_CONFIG` | `/config/gate.yaml` | the access list |
| `--data` | `AWG_GATE_DATA` | `/data` | hub key and obfuscation parameters; created on first start |
| `--interval` | | `30s` | how often `gate.yaml` and end dates are re-checked; `SIGHUP` re-reads at once |

## up

Runs one tunnel end from an issued configuration: a server's spoke, or a Linux client.

| Flag | Variable | Default | |
|---|---|---|---|
| `--config` | `AWG_GATE_CONFIG` | `/config/awg.conf` | the configuration issued by `issue` |
| `--interface` | `AWG_GATE_INTERFACE` | `awg0` | |
| `--allow-ports` | `AWG_GATE_ALLOW_PORTS` | all | comma-separated TCP/UDP ports reachable over the tunnel |
| `--lockdown` | `AWG_GATE_LOCKDOWN` | off | sidecar only: nothing but the tunnel comes in |

Takes any WireGuard-format configuration with IPv4 addresses; refuses one whose
`AllowedIPs` would route everything (`0.0.0.0/0`).

## hub-info

```shell
awg-gate hub-info [--init] [--config F] [--data D] > hub.json
```

Prints what clients need from a hub as JSON - public key, endpoint, subnets, MTU,
obfuscation parameters - and no private key. `--init` creates the hub key and parameters
when they are missing (what the hub's first start would do).

## issue

```shell
awg-gate issue person NAME DEVICE --hub-info hub.json [--gate gate.yaml] [--reach a,b|*] [--expires YYYY-MM-DD] [--address IP] [--out DIR] [--qr | --json]
awg-gate issue server NAME        --hub-info hub.json [--gate gate.yaml] [--ports 443,80]   [--address IP] [--out DIR] [--json]
```

Generates a key pair locally and writes the configuration, mode `600`, into `--out`
(default `.`): `NAME-DEVICE.conf` and `NAME-DEVICE.png` for a person, `NAME.conf` for a
server. Never overwrites a file.

With `--gate`, adds the entry to that `gate.yaml` (picking the next free address unless
`--address` is given) and validates it. Without it, `--address` is required and the YAML
to add is printed. `--reach` and `--expires` apply to a new person only.

`--json` prints, instead of the text, one JSON object for a script that keeps `gate.yaml`'s
source elsewhere:

```json
{
  "kind": "person",
  "name": "alice",
  "device": "phone",
  "address": "10.247.1.11",
  "public_key": "...",
  "files": ["./alice-phone.conf", "./alice-phone.png"]
}
```

`ports` is added for a server given `--ports`, and `gate` names the file when `--gate`
filed the entry too. `files` hold the private key: deliver them and delete them.

`AWG_GATE_HUB_INFO` and `AWG_GATE_GATE` set `--hub-info` and `--gate`.

## revoke

```shell
awg-gate revoke person NAME [DEVICE] [--gate gate.yaml]
awg-gate revoke server NAME          [--gate gate.yaml]
```

Removes the entry from `gate.yaml`, keeping the rest of the file as it was. A person left
without devices is removed; a removed server leaves every `reach` list.

## disable, enable

```shell
awg-gate disable person NAME [DEVICE] [--gate gate.yaml]
awg-gate enable  person NAME [DEVICE] [--gate gate.yaml]
```

`disable` sets `disabled: true` on the person, or on one device: the entry, its keys and
addresses stay, the hub leaves it out. `enable` removes the key again, and the
configurations already issued connect as before. See [People](People.md#pause-someone).

## check

```shell
awg-gate check [--config gate.yaml]
```

Validates the file and prints every device → server rule with its ports, and the devices
left out because they expired or are disabled. Exit status 1 on an invalid file: use it in CI.

## status

Inside a running hub or spoke container: health, and every peer's address, endpoint, last
handshake and traffic; on the hub, with the names from `gate.yaml`.

## health

Exit status 0 when the running daemon's last report is fresh and good. The image's
`HEALTHCHECK`.

## version

Prints the version.

## Environment

| Variable | |
|---|---|
| `AWG_GATE_LOG_LEVEL` | `debug` for AmneziaWG's own messages; default `info` |
