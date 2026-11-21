# People and devices

## The operator's alias

Everything below runs the image on your machine, in the directory holding `gate.yaml` and
`hub.json`:

```shell
alias awg-gate='docker run --rm -it -v "$PWD:/work" -w /work --user "$(id -u):$(id -g)" ghcr.io/corneizer/awg-gate:0.2.0'
```

Key pairs are generated here, on your machine. `gate.yaml` receives the public key; the
private key goes only into the configuration you hand over. The hub never holds a private
key of anyone's.

## Give someone access

```shell
awg-gate issue person alice laptop --hub-info hub.json --gate gate.yaml \
  --reach shop-dev,blog-dev --expires 2027-03-31
```

- `alice` is the person, `laptop` the device. Names are lowercase letters, digits and `-`.
- `--reach` and `--expires` apply to a **new** person; for one already in `gate.yaml`
  they are ignored and their entry is used. `--reach '*'` grants every server.
- The next free address in `people_subnet` is picked (`--address` to choose one).
- It writes `alice-laptop.conf` and `alice-laptop.png` (the same configuration as a QR
  code) with mode `600`, and refuses to overwrite an existing file. Add `--qr` to draw the
  code in the terminal.

Then deploy `gate.yaml` to the hub (see [Hub](Hub.md#gateyaml-is-the-access-list)) and
deliver the file.

### Delivering the configuration

It is a credential: whoever has it is Alice on the network. Hand it over the way you hand
over a password - a one-time share from your password manager (Bitwarden Send,
1Password), not a chat message or an e-mail attachment. Then delete your copies:

```shell
rm alice-laptop.conf alice-laptop.png
```

Nothing is lost: a lost configuration is replaced by issuing a new device, not recovered.

### One device, one configuration

A phone and a laptop are two devices with two keys. Never put one configuration on two
devices: the hub follows a key to its last address, and the two would knock each other
off. Adding a device:

```shell
awg-gate issue person alice phone --hub-info hub.json --gate gate.yaml
```

## Change what someone reaches

Edit `gate.yaml`: the person's `reach` list, a server's `ports`, the `expires` date. Their
configuration does not change - it routes the whole servers' subnet, and the hub decides.
`awg-gate check --config gate.yaml` prints who reaches what before you deploy it.

## End dates

`expires: 2027-03-31` is the **last day** of access, in UTC: the hub drops the person's
devices when that day ends (re-checked every 30 seconds, no timer to set up). The entry
stays in `gate.yaml`, so extending access is changing the date. `awg-gate check` lists
expired devices.

## Pause someone

Somebody who works with you now and then does not need a new configuration every time:

```shell
awg-gate disable person alice --gate gate.yaml        # every device
awg-gate disable person alice phone --gate gate.yaml  # one device
awg-gate enable person alice --gate gate.yaml         # back, with the same configuration
```

`disabled: true` keeps the entry, its keys and its addresses, but the hub leaves it out -
established connections stop as with `revoke`. Clearing it lets the configuration they
already have connect again. A disabled device's address stays reserved, so nobody else is
ever given it. `awg-gate check` lists disabled devices.

## Take access away

```shell
awg-gate revoke person alice phone --gate gate.yaml   # a lost or replaced device
awg-gate revoke person alice --gate gate.yaml         # the person, every device
```

Deploy `gate.yaml` and send `SIGHUP` (or wait up to 30 seconds): the hub removes the keys
and their rules, and established connections stop at once - the end-to-end test checks it.

**Leaving the team** is `revoke person`, in the same change that closes their other
accounts. A temporary contractor gets `--expires` up front, so forgetting is not an
outcome.

### A device that may be compromised

`revoke` the device, then issue a new one. If the configuration may have leaked together
with `hub.json` - which is not secret but makes traffic recognisable - consider a new
obfuscation parameter set ([Obfuscation](Obfuscation.md#changing-the-parameters)).

## Where this fits an existing onboarding

If people already live in a list - an Ansible inventory, Terraform, an HR export - generate
`gate.yaml` from it, or run `awg-gate issue`/`revoke` from the same playbook that creates
mailboxes and accounts. `issue --json` (without `--gate`) prints the new device's name,
address and public key as one JSON object, for a script to file in that list; whoever is
leaving or pausing becomes `disabled: true` in the generated file, so their configurations
work again the day they are back. `gate.yaml` stays the one thing the hub reads, and `awg-gate check`
in CI keeps a broken one from reaching it.
