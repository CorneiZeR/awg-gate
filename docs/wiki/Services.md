# Traefik, nginx, Caddy

A server on the access network is still on the internet. What makes a site reachable
**only** through the tunnel is its proxy admitting only the access network,
`10.247.0.0/16` (your `network`). Requests from people arrive over the tunnel with their
own addresses, `10.247.1.x`; anything else is a stranger.

Use the whole `network`, not `people_subnet`: it keeps working if you later split people
into several subnets.

## Traefik

An `ipAllowList` middleware on every router of the environment. With Docker labels:

```yaml
services:
  back:
    labels:
      - traefik.enable=true
      - traefik.http.routers.back.rule=Host(`api.dev.example.com`)
      - traefik.http.routers.back.entrypoints=websecure
      - traefik.http.routers.back.tls.certresolver=le
      - traefik.http.middlewares.vpn-only.ipallowlist.sourcerange=10.247.0.0/16
      - traefik.http.routers.back.middlewares=vpn-only
```

Or once, in the dynamic configuration, and referenced as `vpn-only@file`:

```yaml
http:
  middlewares:
    vpn-only:
      ipAllowList:
        sourceRange: ["10.247.0.0/16"]
```

- A request from anywhere else gets **403**. The end-to-end test runs exactly this
  middleware on Traefik 3.7 and checks both sides.
- The middleware judges the TCP peer. Do **not** add `ipStrategy` to it: that would make
  it read `X-Forwarded-For`, which a client writes itself.
- Production and dev on one Traefik: put the middleware on the dev routers only. If dev
  routers used a "Cloudflare only" allowlist before, `vpn-only` replaces it.
- Traefik published as `ports: ["443:443"]` is fine: traffic from the tunnel reaches it
  through Docker's DNAT with the person's address intact.
- Certificates: HTTP-01 and TLS-ALPN-01 cannot reach a private address; use DNS-01, see
  [Cloudflare](Cloudflare.md#traefik).

## nginx

```nginx
server {
    listen 443 ssl;
    server_name app.dev.example.com;

    allow 10.247.0.0/16;
    deny  all;

    # ... ssl_certificate, locations, proxy_pass as before
}
```

- `deny all` answers 403; the end-to-end test runs this block on nginx 1.30.
- If the server block trusted Cloudflare's `CF-Connecting-IP` (`set_real_ip_from` +
  `real_ip_header`), remove that for the dev hosts: dev is no longer behind Cloudflare,
  and `allow`/`deny` must judge the real peer.
- Binding to the tunnel address instead - `listen 10.247.0.10:443 ssl;` - makes the site
  invisible rather than forbidden, but nginx then cannot start before the tunnel is up
  (or needs `net.ipv4.ip_nonlocal_bind=1`). The allowlist is the simpler fence.
- nginx installed on the host sits behind UFW: allow it on `awg0`, see
  [Servers](Servers.md#ufw-and-host-services).

## Caddy

```caddyfile
app.dev.example.com {
    @outside not remote_ip 10.247.0.0/16
    respond @outside 403

    reverse_proxy back:8000
    tls {
        dns cloudflare {env.CF_API_TOKEN}
    }
}
```

`remote_ip` judges the TCP peer (`client_ip` would honour trusted proxies' headers).
Certificates by DNS-01 need a Caddy build with the
[`caddy-dns/cloudflare`](https://github.com/caddy-dns/cloudflare) module. This recipe is
not part of the end-to-end test.

## No proxy: a sidecar

A single service without a proxy - an internal tool, an admin console - runs behind a
[sidecar](Servers.md#sidecar) with `--lockdown`: it is reachable at the tunnel address and
not at all from anywhere else.

## What the application sees

Nothing changes for it: the same `Host`, the same headers, the same `Authorization`
tokens and API keys, Swagger and the admin as in production. `ALLOWED_HOSTS`, CORS and
CSRF origins stay as they were - the names did not change, only where they point.
