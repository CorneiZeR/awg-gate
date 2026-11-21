# Cloudflare

What changes in Cloudflare when an environment moves behind the access network: two kinds
of DNS records, and certificates issued by DNS-01 with a narrowly scoped API token. Nothing
else in Cloudflare is involved: the tunnel does not go through Cloudflare's proxy, and the
dev sites no longer do either.

The same applies to any DNS provider with an API that Let's Encrypt clients support; the
steps below are Cloudflare's.

## The hub

| Type | Name | Content | Proxy status | TTL |
|---|---|---|---|---|
| A | `vpn` | the hub's public IPv4 | **DNS only** (grey cloud) | Auto |

**Never proxied:** Cloudflare's proxy carries HTTP(S), not a UDP tunnel, so an orange cloud
would hand clients an address that does not answer. Using a name rather than the IP in
`endpoint` lets the hub move to another VPS by changing this one record; spokes re-resolve
it whenever they lose the hub.

## The environment's names

Each site of the environment points at its server's **tunnel address**:

| Type | Name | Content | Proxy status |
|---|---|---|---|
| A | `app.dev` | `10.247.0.10` | **DNS only** |
| A | `api.dev` | `10.247.0.10` | **DNS only** |

Or a single wildcard per server, `*.dev` → `10.247.0.10`.

- **DNS only**: a proxied record would make Cloudflare connect to `10.247.0.10` itself,
  which it cannot (error 1002). It must resolve to the private address on every device.
- Switching an existing proxied record: edit it in place - content to the tunnel address,
  proxy off. While the change propagates (minutes; Cloudflare's own TTL is 5 minutes for
  proxied records), some clients still reach the old public origin; close it with the
  proxy allowlist ([Services](Services.md)) **before** you change DNS.
- The server's public IP no longer appears in DNS for these names. SSH and CI deploys
  keep using it directly (or through their own record).
- An `AAAA` record for a dev name must go: the access network is IPv4, and a client that
  prefers IPv6 would try the public address first.

### What becomes public, and why it does not matter

The names and the private address are visible to anyone who asks DNS, and the names were
already in Certificate Transparency logs the moment a certificate was issued. Knowing that
`app.dev.example.com` is `10.247.0.10` lets nobody connect to it. A wildcard certificate
(`*.dev.example.com`) keeps individual names out of CT logs if you prefer not to list them.

### Resolvers that refuse private answers

Some resolvers drop public names that resolve to private addresses (*DNS rebinding
protection*): OpenWrt and other dnsmasq routers (`stop-dns-rebind`), FRITZ!Box, pfSense
and OPNsense (Unbound `private-address`), some ISP resolvers and filtering services. The
symptom: the site works on mobile data, not on one home or office network, and
`nslookup app.dev.example.com` returns nothing there.

The client configurations carry no DNS server on purpose, so the device's own resolver is
used. Fix it on that network:

- **OpenWrt**: *Network → DHCP and DNS → Filter → Domain whitelist*: `example.com`
  (`rebind_domain`).
- **FRITZ!Box**: *Home Network → Network → Network Settings → DNS rebind protection*: add
  the domain.
- **Unbound** (pfSense, OPNsense): `private-domain: "example.com"`.
- Or point the device at a public resolver (1.1.1.1, 8.8.8.8).

## Certificates by DNS-01

Let's Encrypt cannot reach a private address, so HTTP-01 and TLS-ALPN-01 stop working the
moment the names point at `10.247.0.10`. DNS-01 proves control of the name by a TXT record
the server writes through Cloudflare's API - no inbound connection at all.

### The token

*Cloudflare dashboard → My Profile → API Tokens → Create Token → Create Custom Token*:

| Field | Value |
|---|---|
| Token name | `acme-dns01 shop-dev` |
| Permissions | **Zone · DNS · Edit** and **Zone · Zone · Read** |
| Zone Resources | **Include · Specific zone · `example.com`** |
| Client IP Address Filtering | **Is in** · the server's public IPv4 **and** IPv6, if it has one |
| TTL | no end date (renewals run every 60 days; rotate it on your own schedule) |

- One token per server, scoped to one zone: a leaked token can edit that zone's DNS and
  nothing else, and only from that server's addresses.
- `Zone · Zone · Read` lets the ACME client find the zone ID; without it Traefik and
  certbot fail with an authentication error that does not say why.
- The IP filter applies to whichever address the server *calls out* from - include its
  IPv6 if it has one, or the API refuses calls made over IPv6.
- Store it as a secret (CI variable, vault), never in the repository.

If the zone holds production too and a token that can rewrite every record is too much,
delegate the dev subdomain to a zone of its own (`dev.example.com`, a separate Cloudflare
zone with NS records in the parent) and scope the token to that zone only.

### Traefik

```yaml
# static configuration
certificatesResolvers:
  le:
    acme:
      email: ops@example.com
      storage: /letsencrypt/acme.json
      dnsChallenge:
        provider: cloudflare
        resolvers: ["1.1.1.1:53", "1.0.0.1:53"]
```

```yaml
# the Traefik service
environment:
  CF_DNS_API_TOKEN_FILE: /run/secrets/cf_dns_api_token
secrets: [cf_dns_api_token]
```

`resolvers` makes Traefik check propagation against Cloudflare's resolvers rather than the
server's. A wildcard for a whole server:

```yaml
- traefik.http.routers.back.tls.domains[0].main=dev.example.com
- traefik.http.routers.back.tls.domains[0].sans=*.dev.example.com
```

### certbot

```shell
apt install python3-certbot-dns-cloudflare
install -m 600 /dev/null /etc/letsencrypt/cloudflare.ini
echo "dns_cloudflare_api_token = <token>" > /etc/letsencrypt/cloudflare.ini
certbot certonly --dns-cloudflare --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
  -d app.dev.example.com -d api.dev.example.com
```

A certificate issued before by `--nginx` or `--webroot` renews with that method and will
fail now: reissue it once with `--dns-cloudflare` (as above); certbot records the new
method for renewals.

### Caddy

`tls { dns cloudflare {env.CF_API_TOKEN} }`, with a Caddy built with
`caddy-dns/cloudflare` - see [Services](Services.md#caddy).

## Moving an existing environment: the order

1. Hub running, server connected, your own device on the network: `ping 10.247.0.10`.
2. The token created; the proxy issues a test certificate by DNS-01 (Let's Encrypt staging).
3. The proxy admits only `10.247.0.0/16` on the environment's routers. From the network
   the site still works - through its public IP and Cloudflare for now.
4. In Cloudflare: each name → the tunnel address, **DNS only**; remove dev `AAAA` records.
5. Remove what only existed for Cloudflare's proxy: a "Cloudflare only" allowlist on those
   routers, UFW rules admitting Cloudflare's ranges to 80/443 (if nothing else needs them),
   `real_ip` trust of `CF-Connecting-IP` for those hosts.
6. Check from outside the network: the names resolve to `10.247.0.10` and nothing
   connects; the server's public IP answers 403 (or nothing) for those `Host`s.
7. Webhooks that a third party called on dev: switch them to polling or a separate public
   endpoint ([Concepts](Concepts.md#what-stays-public)).
