# Toolchain image for a source-built backup sidecar: pg_dump 15 + aws-cli for
# off-host upload.
#
# The backup script itself (backup-db.sh) is bind-mounted read-only by the
# compose file, so this image only needs rebuilding when the toolchain
# changes, not when the script changes.
#
# Built from the same PostgreSQL image, pinned by digest, that
# deploy/docker-compose.yml runs, so pg_dump always matches the server
# (pg_dump refuses a newer server). Dependabot (docker, /backend/scripts)
# proposes digest bumps; scripts/ci/workflowcontract keeps the two equal.
#
#   docker build -f backend/scripts/backup.Dockerfile backend/scripts
FROM postgres:18.6-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873

RUN apk add --no-cache \
    bash \
    gzip \
    coreutils \
    aws-cli \
    ca-certificates

# pg_dump and aws need no privileges; the dump directory is a volume the
# compose service makes writable.
USER nobody

# The compose service supplies the real entrypoint loop; this default just
# makes ad-hoc `docker run` debugging pleasant.
ENTRYPOINT ["/bin/sh"]
