# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89
# Builder: cgo-free static Go build (CGO_ENABLED=0).
FROM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Go build tags. Empty by default: the published image is Apache-2.0 code plus
# the LGPL-3.0-or-later go-ethereum library packages it links (see the license
# label below), and it links no GPL code. GO_TAGS=whatsapp compiles in the
# WhatsApp channel, which pulls go.mau.fi/libsignal (GPL-3.0) into /app/server;
# never publish such an image without honouring GPL-3.0. See
# docs/self-hosting/whatsapp.md.
ARG GO_TAGS=""

# Static, stripped binaries for distroless runtime.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -tags "$GO_TAGS" -trimpath -ldflags="-s -w" \
    -o /out/server ./cmd/app/ \
 && CGO_ENABLED=0 GOOS=linux go build \
    -tags "$GO_TAGS" -trimpath -ldflags="-s -w" \
    -o /out/healthcheck ./cmd/healthcheck \
 && CGO_ENABLED=0 GOOS=linux go build \
    -tags "$GO_TAGS" -trimpath -ldflags="-s -w" \
    -o /out/email-smoke ./cmd/email-smoke

# Pre-create runtime-writable directories. distroless has no shell so we cannot
# `mkdir` in the final stage; we COPY a pre-built tree owned by nonroot.
# /app/images must exist because cmd/app creates ./images at startup.
RUN mkdir -p /out/data /out/images /out/uploads

# /out/storage becomes /data/storage, the STORAGE_DRIVER=local default
# (STORAGE_DIR). Docker seeds a fresh named volume mounted there with this
# directory's owner and mode (nonroot, 0700).
RUN mkdir -p /out/storage && chmod 0700 /out/storage

# Runtime: distroless static, runs as user nonroot (uid/gid 65532) by default.
# Eliminates the entire Debian/glibc/openssl/etc. attack surface from the image.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

# OCI metadata (D-8). LICENSE and NOTICE in this directory are byte-identical
# copies of the repository root files (scripts/ci/d10_hygiene_contract.test.mjs
# checks this), so a plain `docker build backend` ships them too. The image
# also links LGPL-3.0-or-later code (go-ethereum library packages),
# so LICENSES/ carries the verbatim LGPL-3.0 and the GPL-3.0 it incorporates.
LABEL org.opencontainers.image.title="payverge-backend" \
      org.opencontainers.image.licenses="Apache-2.0 AND LGPL-3.0-or-later" \
      org.opencontainers.image.source="https://github.com/stdevMac/payverge" \
      org.opencontainers.image.vendor="Payverge"

WORKDIR /app

COPY --chown=nonroot:nonroot LICENSE NOTICE /usr/share/doc/payverge/
COPY --chown=nonroot:nonroot LICENSES/ /usr/share/doc/payverge/LICENSES/
COPY --from=builder --chown=nonroot:nonroot /out/server      /app/server
COPY --from=builder --chown=nonroot:nonroot /out/healthcheck /app/healthcheck
COPY --from=builder --chown=nonroot:nonroot /out/email-smoke /app/email-smoke
COPY --from=builder --chown=nonroot:nonroot /app/email       /app/email
COPY --from=builder --chown=nonroot:nonroot /app/migrations  /app/migrations
COPY --from=builder --chown=nonroot:nonroot /out/storage     /data/storage
COPY --from=builder --chown=nonroot:nonroot /out/data        /app/data
COPY --from=builder --chown=nonroot:nonroot /out/images      /app/images
COPY --from=builder --chown=nonroot:nonroot /out/uploads     /app/uploads

USER nonroot:nonroot

EXPOSE 8080

# --start-period covers genesis bootstrap and migrations on first boot: failed
# probes in that window do not count toward --retries. --start-interval is not
# used because it needs Docker Engine 25 or newer.
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=90s \
  CMD ["/app/healthcheck"]

ENTRYPOINT ["/app/server"]
