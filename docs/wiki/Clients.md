# Clients

A person gets one file per device, `alice-laptop.conf`, and often the same configuration
as a QR code, `alice-laptop.png`. Any AmneziaWG-capable client takes it.

| Platform | App | Import |
|---|---|---|
| iOS, iPadOS | **AmneziaVPN** or **AmneziaWG** (App Store) | scan the QR code, or open the file |
| Android | **AmneziaVPN** or **AmneziaWG** (Google Play, GitHub releases) | scan the QR code, or import the file |
| macOS | **AmneziaVPN** or **AmneziaWG** | import the file |
| Windows | **AmneziaVPN** or **AmneziaWG** | import the file |
| Linux | **AmneziaVPN**, the awg-gate image, or `amneziawg-tools` | see below |

In AmneziaVPN: **+** (add a connection) → *file with connection settings* or *QR code*.
In AmneziaWG: **+** → *import from file* or *scan from QR code*. Give the tunnel a name you
will recognise.

The configuration uses the hub's [obfuscation profile](Obfuscation.md). Profile `v1` works
with every Amnezia client released since 2024; profile `v3` needs AmneziaWG 3 support
(AmneziaVPN 5, AmneziaWG for Android 3). An app that is too old fails to import or never
connects - update it.

## Is it working

```shell
ping 10.247.0.1                         # the hub answers whenever the tunnel is up
ping 10.247.0.10                        # a server you may reach
curl -I https://app.dev.example.com     # its site
```

The app shows a *latest handshake* a few seconds after connecting. Only the access
network goes through the tunnel; everything else - browsing, calls - goes the usual way,
so leaving the tunnel on all day costs nothing.

## With a commercial VPN

- **Desktop**: both can run at once. If the commercial VPN has a *kill switch* or
  *block traffic outside the VPN*, allow AmneziaVPN/AmneziaWG in its split tunnelling (or
  *allow LAN/other tunnels*); otherwise it drops the AmneziaWG packets. Inside a commercial
  VPN the tunnel to the hub simply rides through it - the default MTU of 1280 leaves room.
- **Phone**: iOS and Android run one VPN at a time. Switching to the access network turns
  the commercial one off, and with it the protection of the rest of the phone's traffic.
  AmneziaWG is made to get through DPI on its own, so the access network does not need the
  commercial VPN to reach the hub.

## Linux

With Docker, the awg-gate image runs the configuration in the host's network namespace:

```shell
docker run -d --name awg-gate --restart unless-stopped --network host \
  --cap-drop ALL --cap-add NET_ADMIN --device /dev/net/tun --read-only --tmpfs /run \
  -v "$HOME/.config/awg-gate:/config:ro" \
  ghcr.io/corneizer/awg-gate:0.1.0 up --config /config/alice-laptop.conf
```

Without Docker, install [amneziawg-tools](https://github.com/amnezia-vpn/amneziawg-tools)
(and the kernel module or `amneziawg-go`) and run `awg-quick up ./alice-laptop.conf`.

## Names do not resolve on one network

On mobile data it works, at home `app.dev.example.com` resolves to nothing: the home
router refuses public names with private answers. See
[Cloudflare → resolvers that refuse private answers](Cloudflare.md#resolvers-that-refuse-private-answers).
