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
FROM postgres:15.19-alpine@sha256:f7d23353e1b15400d22ebe31189f4d314b87a4c129cc400c8c2d8d4ca127bf81

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
