# syntax=docker/dockerfile:1.12

# awg-gate: the CLI and daemons, plus AmneziaWG's userspace implementation and its `awg` tool.
#
# Everything is compiled here, in CI, never on a server: the image carries no compiler.
# amneziawg-go is built from this repository's go.mod (it is a `tool` dependency there), so it
# links the same patched golang.org/x modules awg-gate does, and Dependabot raises both.

# Base images are written into each FROM, pinned by digest, so Dependabot can raise them.

# Go builds cross-compile on the build machine's own architecture: fast, no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS go-build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/awg-gate ./cmd/awg-gate \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/amneziawg-go github.com/amnezia-vpn/amneziawg-go/v3

# The `awg` tool is C; it is built for the target platform, against the same musl the image runs.
FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS tools-build
# The tag names the release; the commit is what is actually trusted - a moved tag fails the build.
ARG AWG_TOOLS_REF=v3.1.20260812
ARG AWG_TOOLS_COMMIT=ee0f0a9aa34ff0a0da4b3433b9512781cfe02843
RUN apk add --no-cache build-base linux-headers git
RUN git clone --depth 1 --branch "${AWG_TOOLS_REF}" https://github.com/amnezia-vpn/amneziawg-tools.git /awg-tools \
 && test "$(git -C /awg-tools rev-parse HEAD)" = "${AWG_TOOLS_COMMIT}" \
 && make -C /awg-tools/src -j"$(nproc)" wg \
 && install -m 0755 /awg-tools/src/wg /out-awg

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
ARG VERSION=dev
LABEL org.opencontainers.image.title="awg-gate" \
      org.opencontainers.image.description="An AmneziaWG access network for reaching private servers: one hub, servers as spokes, access per person" \
      org.opencontainers.image.source="https://github.com/CorneiZeR/awg-gate" \
      org.opencontainers.image.documentation="https://corneizer.github.io/awg-gate/" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
# Only what the daemons call: `ip` for addresses and routes, `nft` for the access rules.
RUN apk upgrade --no-cache && apk add --no-cache iproute2-minimal nftables
COPY --from=go-build /out/awg-gate /out/amneziawg-go /usr/bin/
COPY --from=tools-build /out-awg /usr/bin/awg
COPY LICENSE THIRD-PARTY-NOTICES.md /usr/share/doc/awg-gate/
# The binaries must answer before the image is published at all.
RUN awg-gate version && awg --version && test -x /usr/bin/amneziawg-go
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 CMD ["awg-gate", "health"]
ENTRYPOINT ["awg-gate"]
CMD ["help"]
