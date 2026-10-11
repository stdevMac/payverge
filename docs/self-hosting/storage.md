# Storage

Payverge stores two kinds of objects:

- **Public objects.** Menu photos, logos, partner icons, AI-generated dish images
  and demo assets. Guests and operators load them in the browser.
- **Protected objects.** Contracts, fiscal receipt PDFs, accounting attachments,
  space-scan artifacts and menu-extraction uploads. They are only ever returned
  by authenticated API endpoints, never by a public URL.

The `STORAGE_DRIVER` setting picks where both live:

| Driver | Needs | Use it when |
|---|---|---|
| `local` (default) | A writable directory (a Docker volume) | Single-server installs. No third-party account. |
| `s3` | An S3-compatible bucket: AWS S3, MinIO, Cloudflare R2, Garage, … | Several backend replicas, hosts without a persistent disk, or a CDN in front of images. |

Menus, tables, orders, bills, the KDS and payments do not depend on storage. If
the storage directory is unusable, the backend still starts, logs an error, and
only uploads fail.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `STORAGE_DRIVER` | `local` | `local` or `s3`. Any other value refuses to start. |
| `STORAGE_DIR` | `/data/storage` in production mode, `./data/storage` otherwise | Root directory of the local driver. |
| `PUBLIC_URL` | empty | Public origin of the install, e.g. `https://pos.example.com`. When it is empty, media URLs are relative (`/media/<key>`). When it is set, they become `${PUBLIC_URL}/media/<key>`. `APP_BASE_URL` is never used for media URLs. A value that is not an absolute `http(s)` origin is ignored with a startup warning. |
| `S3_FORCE_PATH_STYLE` | `false` | `s3` driver: address objects as `http://host/bucket/key`. Required by MinIO and Garage. |
| `S3_BUCKET`, `AWS_ACCESS_KEY`, `AWS_SECRET_KEY` | none | `s3` driver: public bucket and credentials. |
| `AWS_REGION` | `us-east-1` | `s3` driver region. For an R2 endpoint it is coerced to `auto`. |
| `S3_ENDPOINT` | AWS | `s3` driver: API endpoint for a non-AWS service, e.g. `http://minio:9000` or `https://<account>.r2.cloudflarestorage.com`. |
| `S3_PUBLIC_BASE_URL` | empty | `s3` driver, optional: a base that serves the public bucket directly, either a CDN or custom domain or the public-read bucket's own URL. New uploads are stored as `<S3_PUBLIC_BASE_URL>/<key>`. When it is empty, public objects are served through the backend at `/media/<key>`, so the bucket never needs to be public. See [Upgrading an `s3` install](#upgrading-an-s3-install). |
| `S3_PROTECTED_BUCKET`, `AWS_PROTECTED_ACCESS_KEY`, `AWS_PROTECTED_SECRET_KEY`, `S3_PROTECTED_ENDPOINT` | empty | `s3` driver, optional: a separate private bucket for protected objects. Credentials and endpoint fall back to the public ones. When it is empty, protected objects go into `S3_BUCKET` under the `protected/` prefix. |
| `S3_PROTECTED_BASE_URL` | empty | Cosmetic prefix for stored protected location strings. It does **not** make anything public, and the startup probe checks it. |

The `S3_*` and `AWS_*` values are ignored with `STORAGE_DRIVER=local`. If
`S3_BUCKET` is set while the driver is `local`, the backend logs a warning
because new uploads go to the local directory.

## Public URLs and the `/media` route

The backend serves public objects at `GET /media/<key>` (and `HEAD`), whatever
the driver. Point your reverse proxy's `/media/*` at the backend next to
`/api/*`:

```caddyfile
pos.example.com {
	handle /api/* {
		reverse_proxy backend:8080
	}
	handle /media/* {
		reverse_proxy backend:8080
	}
	handle {
		reverse_proxy frontend:3000
	}
}
```

The Next.js frontend also proxies `/media/*` to the backend when
`BACKEND_INTERNAL_URL` is set. The root `docker-compose.yml` sets it to
`http://backend:8080` by default, so a relative `/media/<key>` URL works when
only the frontend port is published. Keep it set even behind a reverse proxy:
the Next.js image optimizer fetches `/media/...` through that proxy. Outside
Docker, start the frontend with `BACKEND_INTERNAL_URL=http://localhost:8080`.

If neither the proxy nor a `/media` route reaches the backend, every uploaded
image returns `404`.

What `/media` does:

- **Public store only.** Keys in protected key spaces get `404`, exactly like a
  missing object: `protected/`, `storage-probe/`, `fiscal-receipts/`,
  `ledger/`, `ai/menu-extraction/`, `space-scans/` and
  `businesses/<id>/contracts/`. Directories are never listed.
- **Strict keys.** Allowed characters are `A-Z a-z 0-9 - _ . ~ ! ( ) + , = @ /`.
  Empty segments, `.`/`..` segments and dot-leading segments are refused, as is
  anything over 1024 bytes.
- **Inline types.** PNG, JPEG, WebP, GIF, AVIF, PDF, MP4 and WebM are served
  inline with their real `Content-Type`. Anything else (SVG, HTML, unknown) is
  sent as an `application/octet-stream` attachment.
- **Headers.** Every response carries `X-Content-Type-Options: nosniff`, a
  sandboxing `Content-Security-Policy` and
  `Cross-Origin-Resource-Policy: cross-origin`.
- **Caching.**
  - Upload keys carry a random 16-hex or UUID component, so they get
    `Cache-Control: public, max-age=31536000, immutable`.
  - Other keys, such as demo assets, get `public, max-age=3600`.
  - Responses carry an `ETag` (sha256 of the content on the local driver) and
    answer `If-None-Match` with `304`.
  - The local driver also supports `Range` requests.
- **Its own rate limit.** `/media` is exempt from the per-IP API rate limiter,
  because a menu page loads dozens of images and must not use up the API
  budget. It has a separate per-IP token bucket instead, shared by `GET` and
  `HEAD`. Requests past it get `429` with `Retry-After` and `no-store`.
  - `MEDIA_RATE_LIMIT_REQUESTS_PER_MINUTE` defaults to 1800 (30/s), and
    `MEDIA_RATE_LIMIT_BURST` defaults to 600.
  - `0` disables it. Do that only when a CDN or your proxy already throttles
    `/media/*`.
  - The defaults are generous on purpose, because a restaurant's Wi-Fi shows
    every guest phone as one IP.
  - The frontend server fetches `/media` on its own behalf, for image-optimizer
    cache misses and server renders. Those requests carry no
    `X-Forwarded-For`, so they all come from the frontend container's address.
    A loopback or private peer that sends no forwarding headers therefore gets
    its own bucket with 10 times the rate and burst: 18000/min and 6000 with
    the defaults.
  - The limit keys on the client IP, so `TRUSTED_PROXIES` must cover your
    proxy and the frontend container. Otherwise every relayed guest shares the
    frontend's per-IP bucket. The deploy compose default
    (`EDGE_SUBNET`, `172.30.0.0/24`, plus `127.0.0.1`) covers both.
  - Sizing for image-heavy menus: one cold menu page costs the frontend about
    dishes x image widths optimizer misses, and each miss is one `/media`
    request. Raise `MEDIA_RATE_LIMIT_BURST` if you serve menus with several
    hundred photos to large NATed rooms, or put a CDN in front and set `0`.
    A `429` on `/media` in your proxy or backend access logs means it binds.

Keep `PUBLIC_URL` empty unless something needs absolute URLs. Media URLs are
written into database rows. A relative `/media/<key>` keeps working when the
install moves to another domain. An absolute `${PUBLIC_URL}/media/<key>` keeps
pointing at the old host until you rewrite the rows.

Stored URLs survive driver switches. A row that holds `/media/<key>` or
`${PUBLIC_URL}/media/<key>` keeps working after you move from `local` to `s3`,
because `/media` reads from whichever store is configured. Rows written with an
`S3_PUBLIC_BASE_URL` keep pointing at that CDN.

## Local driver

Layout under `STORAGE_DIR`:

```
public/objects/<key>        object bytes (files 0600, directories 0700)
public/meta/<key>.json      content type, disposition, sha256 ETag
public/tmp/                 staging for atomic writes (temp file + rename)
protected/objects/<key>
protected/meta/<key>.json
protected/tmp/
```

- **Atomic writes.** A crash never leaves a half-written object behind.
- **Startup check.** The directory is checked for writability at startup.
- **Unique protected keys.** Uploads of protected files always get a unique
  key, so re-uploading `contract.pdf` never overwrites an earlier one.
- **Symlinks stay inside.** Each store resolves paths without leaving its own
  directory, through Go's `os.Root`. A symlink inside the tree that points
  outside it is treated as a missing object: `404`, never written through,
  never deleted through. That covers a link from `public/` into `protected/`,
  any absolute link, and any link loop. `STORAGE_DIR` itself may be a symlink
  or a mount point.

**Docker.** The image creates `/data/storage` owned by the non-root user
(uid/gid 65532). The root `docker-compose.yml` mounts the named volume
`backend_storage` there. A fresh named volume inherits that ownership. If you
bind-mount a host directory instead, make it writable by 65532:

```bash
sudo install -d -o 65532 -g 65532 -m 0700 /srv/payverge/storage
# compose: - /srv/payverge/storage:/data/storage
```

**Outside Docker.** `make run` uses `./data/storage` relative to `backend/`;
it is gitignored. Set `STORAGE_DIR` to put it elsewhere.

The local driver serves a single backend instance. Run several replicas only
with a shared filesystem mounted at `STORAGE_DIR`, or switch to `s3`.

### Backup and restore

Every object is a plain file, so a backup is just a copy of the directory. Keep
the `meta/` sidecars: they hold the content types.

```bash
# Backup (the volume name is <compose project>_backend_storage)
docker run --rm \
  -v payverge_backend_storage:/data/storage:ro \
  -v "$PWD":/backup \
  alpine tar czf /backup/payverge-storage-$(date +%F).tgz -C /data storage

# Restore (with the backend stopped)
docker compose stop backend
docker run --rm \
  -v payverge_backend_storage:/data/storage \
  -v "$PWD":/backup \
  alpine sh -c 'tar xzf /backup/payverge-storage-YYYY-MM-DD.tgz -C /data && chown -R 65532:65532 /data/storage'
docker compose start backend
```

Back up the database and the storage volume together. Database rows reference
object keys.

## S3-compatible driver

Set `STORAGE_DRIVER=s3` plus the bucket settings. Startup fails fast on an
incomplete configuration, such as a missing bucket, credentials or region.

### MinIO or Garage (self-hosted)

```dotenv
STORAGE_DRIVER=s3
S3_ENDPOINT=http://minio:9000
S3_FORCE_PATH_STYLE=true
AWS_REGION=us-east-1
S3_BUCKET=payverge
AWS_ACCESS_KEY=...
AWS_SECRET_KEY=...
# Optional: a second, private bucket. Otherwise protected objects use the
# "protected/" prefix of S3_BUCKET.
S3_PROTECTED_BUCKET=payverge-protected
```

Keep both buckets **private**: no anonymous read policy. Public objects reach
browsers through `/media`.

### Cloudflare R2

```dotenv
STORAGE_DRIVER=s3
S3_ENDPOINT=https://<account_id>.r2.cloudflarestorage.com
S3_BUCKET=payverge-public
S3_PROTECTED_BUCKET=payverge-protected
AWS_ACCESS_KEY=...
AWS_SECRET_KEY=...
# Optional: serve public images straight from an R2 custom domain
S3_PUBLIC_BASE_URL=https://images.example.com
```

Never attach a public custom domain or `r2.dev` URL to the protected bucket.

### AWS S3

Leave `S3_ENDPOINT` empty and set `AWS_REGION`. Turn on Block Public Access for
the protected bucket.

### Protected-exposure probe

With the `s3` driver, the backend checks that protected objects are not
publicly readable before it serves traffic:

1. It writes a throwaway `storage-probe/<random>.txt` object to the protected
   store.
2. It fetches that object **anonymously** from the bucket's own URL, from
   `S3_PROTECTED_BASE_URL`, and, in shared-bucket mode, from
   `S3_PUBLIC_BASE_URL`.
3. It deletes the object.

A `2xx` response means contracts, fiscal receipts and ledger attachments are
world-readable:

- In **production mode** the backend refuses to start:
  `storage: protected objects are anonymously readable via <url>`.
- Otherwise it logs a warning.

If the probe cannot run at all (upload denied, network error), the result is
inconclusive. That case is only logged.

This is the only object startup writes. If the protected bucket has object
lock or a policy that denies `DeleteObject`, each restart leaves one
`storage-probe/<random>.txt` behind; add a lifecycle rule that expires the
`storage-probe/` prefix after a day.

To fix an exposure:

- Remove the public bucket policy, ACL or custom domain from the protected
  bucket. In shared-bucket mode, from the `protected/` prefix.
- Or give protected objects their own private bucket with `S3_PROTECTED_BUCKET`.

### Upgrading an `s3` install

Before pluggable storage, an empty `S3_PUBLIC_BASE_URL` meant "store the
bucket's own object URL", such as `https://<bucket>.<endpoint-host>/<key>` on
iDrive e2. That only works with an anonymously readable bucket. Now an empty
`S3_PUBLIC_BASE_URL` means "serve through the backend at `/media/<key>`", which
also works with a private bucket.

Existing rows keep their direct bucket URLs and keep working. Only new uploads
change shape. To keep storing direct bucket URLs and skip the proxy hop, set
`S3_PUBLIC_BASE_URL` to the bucket's own base URL:

| Addressing | `S3_PUBLIC_BASE_URL` |
|---|---|
| Custom endpoint, virtual-hosted (iDrive e2, default) | `https://<bucket>.<endpoint-host>` |
| Custom endpoint, `S3_FORCE_PATH_STYLE=true` | `<S3_ENDPOINT>/<bucket>` |
| AWS, no `S3_ENDPOINT` | `https://<bucket>.s3.<region>.amazonaws.com` |

You do not have to work this out by hand. When `S3_PUBLIC_BASE_URL` is empty,
startup lists one page of the public bucket and requests one existing object
anonymously (a `HEAD`; nothing is written). If that succeeds, the backend logs a
warning with the exact value to set. A private bucket produces no warning,
because `/media` is then the only option. An empty bucket (a fresh install has
no old URLs to keep) and credentials without `s3:ListBucket` skip the check.

In shared-bucket mode, the protected-exposure probe also checks
`S3_PUBLIC_BASE_URL`. If the bucket policy makes the whole bucket readable, the
`protected/` prefix is readable too and the probe fails. Give protected objects
their own private bucket with `S3_PROTECTED_BUCKET` first.

## Moving between drivers

**local to s3.** Copy each store's `objects/` tree into its bucket, keeping the
keys, then switch the driver:

```bash
aws s3 sync /data/storage/public/objects/    s3://payverge/                  --endpoint-url "$S3_ENDPOINT"
aws s3 sync /data/storage/protected/objects/ s3://payverge-protected/        --endpoint-url "$S3_ENDPOINT"
# shared-bucket mode instead: s3://payverge/protected/
```

Existing `/media/...` URLs keep resolving. Content types come from the file
extension during the copy. Add `--content-type` per extension if your tool does
not guess them.

**s3 to local.** Download the buckets into `public/objects/` and
`protected/objects/`. Rows that store CDN URLs (`S3_PUBLIC_BASE_URL`) still
point at the CDN until you rewrite them. Objects without a `meta/` sidecar are
served with a type guessed from the extension and a weak ETag.

## Upload API notes

- `POST /api/v1/inside/businesses/:id/uploads` stores public files under
  `businesses/<id>/[folder]/`. The optional `folder` form field must be one to
  three slug segments (`[A-Za-z0-9][A-Za-z0-9_-]{0,63}`); `contracts` is
  reserved.
- `DELETE /api/v1/inside/businesses/:id/uploads` accepts a key or any URL form
  (`/media/<key>`, a CDN URL, a legacy S3 URL):
  - Keys that are not clean are refused with `400`, including `..`, `.`, empty
    segments and a leading `/`.
  - Keys outside `businesses/<id>/` are refused with `403`, and so are
    protected keys.
- `POST /api/v1/inside/businesses/:id/uploads/protected` stores each upload
  under a unique key, so a re-upload with the same file name never overwrites
  an earlier one. These objects are never addressable through `/media`.

### Objects left behind

Storage only grows unless something deletes objects. Today:

- **Ledger attachments.** Deleting one removes the database row first, then
  the object, best-effort. If the object delete fails, the row is already gone
  and the object is unreachable. The backend logs
  `[accounting] attachment <id> object cleanup failed (orphan key <key>)`, and
  you can remove that key by hand: `rm STORAGE_DIR/protected/objects/<key>`
  plus its `meta/<key>.json`, or `aws s3 rm` on the protected bucket. The
  other order would be worse: a row pointing at an object that is gone.
- **Replaced public uploads.** A replaced or removed menu photo or logo stays
  in storage unless the dashboard calls
  `DELETE /api/v1/inside/businesses/:id/uploads` for it.
- **No sweep.** There is no tool yet that lists objects no row references.

## See also

- Every storage variable, with defaults: [configuration.md](configuration.md#file-storage).
- What the nightly backup includes (local volume and bundled MinIO, not an
  external bucket): [backups.md](backups.md#what-is-covered).
- Images that upload but do not show: [troubleshooting.md](troubleshooting.md#uploads-and-images).
