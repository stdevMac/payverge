# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89
# Use an official Node runtime as the base image.
# node:20.18.0-alpine pinned the (now-EOL) 20.18.0 patch, leaving 23 HIGH/CRITICAL
# CVEs in the Alpine base. node:22-alpine is the current LTS with a clean base.
FROM node:26.10.0-alpine@sha256:0b36e8c136b94cd4fcf02188228e76c31ad5872eef3fec8cbd2eee500cfd9e80 AS deps

# Install dependencies only when needed
WORKDIR /app
ENV NPM_CONFIG_AUDIT=false \
    NPM_CONFIG_FUND=false \
    NPM_CONFIG_FETCH_RETRIES=5 \
    NPM_CONFIG_FETCH_RETRY_MINTIMEOUT=20000 \
    NPM_CONFIG_FETCH_RETRY_MAXTIMEOUT=120000

# Copy package files
COPY package.json package-lock.json* ./

# Install dependencies with minimal footprint
RUN --mount=type=cache,target=/root/.npm \
    apk add --no-cache --virtual .build-deps \
    python3 \
    make \
    g++ \
    linux-headers && \
    npm ci --only=production && \
    apk del .build-deps && \
    npm cache clean --force

# Builder stage
FROM node:26.10.0-alpine@sha256:0b36e8c136b94cd4fcf02188228e76c31ad5872eef3fec8cbd2eee500cfd9e80 AS builder

# Add build arguments
ARG NODE_ENV=production

# Set environment variables
ENV NODE_ENV=$NODE_ENV
ENV NEXT_TELEMETRY_DISABLED=1
ENV NEXT_OUTPUT_STANDALONE=1
ENV NPM_CONFIG_AUDIT=false \
    NPM_CONFIG_FUND=false \
    NPM_CONFIG_FETCH_RETRIES=5 \
    NPM_CONFIG_FETCH_RETRY_MINTIMEOUT=20000 \
    NPM_CONFIG_FETCH_RETRY_MAXTIMEOUT=120000
# Increase Node memory limit to prevent heap out of memory errors
ENV NODE_OPTIONS="--max-old-space-size=4096"
# Runtime-class settings (PUBLIC_URL, API_URL, SUPPORT_EMAIL, NETWORK,
# PUBLIC_RPC_URL, FRONTEND_SENTRY_*, VAPID_PUBLIC_KEY, MEDIA_ORIGINS, ...) are
# read from the CONTAINER environment at request time and published to the
# browser through window.__PAYVERGE_ENV__ (src/config/publicConfig.ts), so one
# image serves any origin. The NEXT_PUBLIC_* ARGs below are an optional legacy
# fallback only: they are inlined at build time and lose to any runtime value.
# Leave them empty for a portable image. See docs/self-hosting/frontend-config.md.
ARG NEXT_PUBLIC_MAINTENANCE_MODE
ENV NEXT_PUBLIC_MAINTENANCE_MODE=$NEXT_PUBLIC_MAINTENANCE_MODE
ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
ARG NEXT_PUBLIC_PUBLIC_URL
ENV NEXT_PUBLIC_PUBLIC_URL=$NEXT_PUBLIC_PUBLIC_URL
ARG NEXT_PUBLIC_SUPPORT_EMAIL
ENV NEXT_PUBLIC_SUPPORT_EMAIL=$NEXT_PUBLIC_SUPPORT_EMAIL
ARG NEXT_PUBLIC_NETWORK
ENV NEXT_PUBLIC_NETWORK=$NEXT_PUBLIC_NETWORK
ARG NEXT_PUBLIC_RPC_URL
ENV NEXT_PUBLIC_RPC_URL=$NEXT_PUBLIC_RPC_URL
ARG NEXT_PUBLIC_SENTRY_DSN
ENV NEXT_PUBLIC_SENTRY_DSN=$NEXT_PUBLIC_SENTRY_DSN
ARG NEXT_PUBLIC_SENTRY_ENVIRONMENT
ENV NEXT_PUBLIC_SENTRY_ENVIRONMENT=$NEXT_PUBLIC_SENTRY_ENVIRONMENT
ARG NEXT_PUBLIC_SENTRY_ENABLED
ENV NEXT_PUBLIC_SENTRY_ENABLED=$NEXT_PUBLIC_SENTRY_ENABLED
ARG NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE
ENV NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE=$NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE
ARG NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE
ENV NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE=$NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE
ARG NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE
ENV NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE=$NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE
ARG NEXT_PUBLIC_VAPID_PUBLIC_KEY
ENV NEXT_PUBLIC_VAPID_PUBLIC_KEY=$NEXT_PUBLIC_VAPID_PUBLIC_KEY
# Build-time by nature: these describe the image itself and stay inlined.
ARG NEXT_PUBLIC_RELEASE_SHA
ENV NEXT_PUBLIC_RELEASE_SHA=$NEXT_PUBLIC_RELEASE_SHA
ARG NEXT_PUBLIC_VERSION
ENV NEXT_PUBLIC_VERSION=$NEXT_PUBLIC_VERSION
ARG NEXT_PUBLIC_BUILD_TIMESTAMP
ENV NEXT_PUBLIC_BUILD_TIMESTAMP=$NEXT_PUBLIC_BUILD_TIMESTAMP
ARG NEXT_PUBLIC_SENTRY_RELEASE
ENV NEXT_PUBLIC_SENTRY_RELEASE=$NEXT_PUBLIC_SENTRY_RELEASE
# Sentry source-map upload targeting (non-secret). The auth token is NOT an ARG
# or ENV — it's injected as a build secret on the build step below so it never
# lands in an image layer.
ARG SENTRY_ORG
ENV SENTRY_ORG=$SENTRY_ORG
ARG SENTRY_FRONTEND_PROJECT
ENV SENTRY_FRONTEND_PROJECT=$SENTRY_FRONTEND_PROJECT

WORKDIR /app

# Copy package files and install ALL dependencies (including dev).
# NODE_ENV is set to production above for the build output, but npm ci
# must see NODE_ENV!=production to include devDependencies (eslint, types).
COPY package.json package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm \
    apk add --no-cache --virtual .build-deps \
    python3 \
    make \
    g++ \
    linux-headers && \
    NODE_ENV=development npm ci && \
    apk del .build-deps

# Copy source code (use .dockerignore to exclude unnecessary files)
COPY . .

# Build the application. SENTRY_AUTH_TOKEN is mounted only for this command so
# it can upload source maps without ever becoming a persisted image-layer env.
# required=false keeps local/CI builds without the secret working (upload no-ops).
RUN --mount=type=secret,id=sentry_auth_token,required=false \
    if [ -s /run/secrets/sentry_auth_token ]; then \
      export SENTRY_AUTH_TOKEN="$(cat /run/secrets/sentry_auth_token)"; \
    fi; \
    npm run build

# Production image
FROM node:26.10.0-alpine@sha256:0b36e8c136b94cd4fcf02188228e76c31ad5872eef3fec8cbd2eee500cfd9e80 AS runner

# Drop npm/npx. They're build-only and bundle their own copy of picomatch
# (HIGH CVE) that isn't reachable at runtime but still shows up in any
# vulnerability scanner. Next.js standalone runs via `node server.js` and
# has no npm dependency at runtime.
RUN rm -rf /usr/local/lib/node_modules/npm \
           /usr/local/bin/npm \
           /usr/local/bin/npx

# Create non-root user for security
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

# OCI metadata (D-8). LICENSE and NOTICE in this directory are byte-identical
# copies of the repository root files (scripts/ci/d10_hygiene_contract.test.mjs
# checks this), so a plain `docker build frontend` ships them too. The image
# also links LGPL-3.0-or-later code (the sharp-libvips shared libraries),
# so LICENSES/ carries the verbatim LGPL-3.0 and the GPL-3.0 it incorporates.
LABEL org.opencontainers.image.title="payverge-frontend" \
      org.opencontainers.image.licenses="Apache-2.0 AND LGPL-3.0-or-later" \
      org.opencontainers.image.source="https://github.com/stdevMac/payverge" \
      org.opencontainers.image.vendor="Payverge"

# Set working directory
WORKDIR /app

COPY LICENSE NOTICE /usr/share/doc/payverge/
COPY LICENSES/ /usr/share/doc/payverge/LICENSES/

# Set environment to production
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

# Copy built standalone application from builder stage. Next.js standalone output
# includes the runtime dependency subset; copying full node_modules here makes
# local/prod image assembly much slower and bloats the runtime image.
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

# Switch to non-root user
USER nextjs

# Expose port
EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME=0.0.0.0

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD wget -q --spider http://127.0.0.1:3000/api/health || exit 1

# Start the application
CMD ["node", "server.js"]
