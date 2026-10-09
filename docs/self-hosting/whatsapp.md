# WhatsApp channel (optional, GPL-3.0 build)

Payverge is Apache-2.0, and the backend image the project publishes contains
**no WhatsApp code**. The WhatsApp AI-waiter channel is opt-in at two levels:

1. **Build time**: compile the backend with the `whatsapp` Go build tag. This
   links GPL-3.0 code into the binary (see [Licensing](#licensing)).
2. **Run time**: set `WHATSAPP_ENABLED=true` on the backend.

The channel runs only when both are true. Most self-hosters do not need it.

## What the `whatsapp` build tag does

The integration uses [whatsmeow](https://github.com/tulir/whatsmeow), which
depends on `go.mau.fi/libsignal`. All code that imports `go.mau.fi/*` sits
behind `//go:build whatsapp`.

| | Default build (no tag) | `-tags whatsapp` |
|---|---|---|
| WhatsApp files compiled | `internal/services/whatsapp_stub.go` | `whatsapp_manager.go`, `whatsapp_client.go` |
| `go.mau.fi/*` linked | no | yes (whatsmeow, util, libsignal) |
| `services.WhatsAppBuilt` | `false` | `true` |
| Licence of the binary | Apache-2.0 plus permissive deps | **GPL-3.0** (whole binary) |

When the tag is absent, the stub keeps the rest of the backend compiling.
`NewWhatsAppManager` returns `ErrWhatsAppNotBuilt`, and every manager method
is an inert no-op.

### Behaviour by build and `WHATSAPP_ENABLED`

| Build | `WHATSAPP_ENABLED` | Manager and sessions | `POST …/whatsapp/connect`, `…/disconnect` and `GET …/whatsapp/qr` | `GET …/whatsapp/status` | Dashboard card | Startup log |
|---|---|---|---|---|---|---|
| default | unset or anything else | off | not mounted (404) | `{"status":"disconnected","enabled":false,"built":false,"requested":false}` | hidden | info: `WhatsApp integration not compiled in` |
| default | `true` | off | not mounted (404) | same, with `"requested":true` | hidden | **warn**: `WHATSAPP_ENABLED=true is ignored: this binary was built without the whatsapp Go build tag …` |
| `whatsapp` | unset or anything else | off | not mounted (404) | `{"status":"disconnected","enabled":false,"built":true,"requested":false}` | shown, with a "not turned on" note and no actions | info: `WhatsApp integration disabled` |
| `whatsapp` | `true` | starts, restores saved sessions | mounted | live status, `"enabled":true,"built":true,"requested":true` | shown, with connect, the pairing QR and disconnect | info: `WhatsApp integration enabled` |
| `whatsapp` | `true`, but the manager failed to start | off | mounted, but they answer 500 | `{"status":"disconnected","enabled":false,"built":true,"requested":true}` | shown, with a "failed to start, check the backend logs" note and no actions | **warn**: `Failed to initialize WhatsApp manager: …` |

`WHATSAPP_ENABLED` accepts only the value `true`, in any letter case and with
surrounding spaces trimmed. Values such as `1`, `yes` and `on` leave the
channel off.

The status route is mounted in every build. The AI-waiter dashboard reads its
`built` field and hides the WhatsApp card when it is `false`. It also hides the
card when the field is missing (an older backend) or when the first request
fails. `requested` mirrors `WHATSAPP_ENABLED`, so the card can tell "not turned
on" (`requested` is `false`) apart from "turned on but failed to start"
(`requested` is `true` and `enabled` is `false`).

## Licensing

| Module | Licence | Linked in the default build? |
|---|---|---|
| `go.mau.fi/whatsmeow` | MPL-2.0 | no |
| `go.mau.fi/util` | MPL-2.0 | no |
| `go.mau.fi/libsignal` | **GPL-3.0** | no |

The Payverge source code stays Apache-2.0 in every case. Apache-2.0 code can be
combined with GPL-3.0 code, so building the tagged binary is allowed.

The **tagged binary**, and any image that contains it, is a combined work that
includes GPL-3.0 code. Distributing it brings the GPL-3.0 obligations with it.

### Distribution caveat

The obligations attach when you **convey** (distribute) the tagged binary or an
image that contains it. Examples of conveying:

- pushing it to a public registry;
- handing it to a customer or another company;
- shipping it on hardware.

In each case you must:

- offer the Corresponding Source of the whole binary under GPL-3.0;
- keep the licence notices;
- not add further restrictions.

Building the image and running it on your own servers for your own restaurant
does not, by itself, convey it. libsignal is GPL-3.0, not AGPL-3.0, so letting
guests talk to the bot over the network is not distribution.

This is a summary, not legal advice. If you plan to distribute a tagged build,
or are unsure whether you do, ask counsel.

What the project guarantees:

- The project's release workflow builds its images with the empty `GO_TAGS`
  default.
- `scripts/ci/workflowcontract/gpl_free_contract_test.go` fails if any
  workflow or `docker-compose*.yml` sets `GO_TAGS` to include `whatsapp`, or
  if `backend/Dockerfile` stops defaulting to `GO_TAGS=""`.
- `make check-gpl-free` (see [Licence gate](#licence-gate-for-contributors))
  proves that the default dependency graph contains no `go.mau.fi` package.

**Never push a whatsapp-tagged image to a public registry under the Payverge
name.** If you need one across machines, keep it in a private registry that
only you can pull from.

## Building a WhatsApp-enabled backend

### Docker

`backend/Dockerfile` takes a `GO_TAGS` build argument. It is empty by default.

```bash
# From the repository root.
docker build --build-arg GO_TAGS=whatsapp -t payverge-backend:whatsapp ./backend
```

With the development compose stack, `docker-compose.yml` builds the backend
from `./backend` and passes `GO_TAGS` from the root `.env` as a build argument
(`GO_TAGS=${GO_TAGS:-}`, empty by default). Put both switches in the root
`.env`:

```bash
GO_TAGS=whatsapp
WHATSAPP_ENABLED=true
```

Then build and start the stack:

```bash
docker compose --env-file .env up -d --build
```

Because the tag comes from `.env`, later `up --build` runs keep it. Remove
`GO_TAGS` from `.env` (or set it empty) and rebuild to go back to the
GPL-free image.

Do not hard-code `GO_TAGS: whatsapp` into a tracked compose file. The licence
contract test rejects any value other than the empty `${GO_TAGS:-}`
pass-through, so that no one publishes such an image by accident.

A compose stack that pulls a pre-built backend image has no build step, so
`GO_TAGS` does nothing there. To use WhatsApp with one:

1. Build the tagged image yourself
   (`docker build --build-arg GO_TAGS=whatsapp ./backend`).
2. Push it to a **private** registry.
3. Point the backend service's `image` at that digest.
4. Set `WHATSAPP_ENABLED=true` in the root `.env`.

### From source

```bash
cd backend
go build -tags whatsapp -o bin/app ./cmd/app
```

`make build` and `make run` in `backend/` use the default tags. Pass
`-tags whatsapp` to `go build`, `go run` or `go test` yourself.

### Turning it on

Add this to the root `.env` (the only env file compose reads), then recreate
the backend container (`docker compose --env-file .env up -d backend`):

```bash
WHATSAPP_ENABLED=true
```

`docker-compose.yml` forwards `WHATSAPP_ENABLED` to the backend, defaulting
to `false`. A `workflowcontract` test fails if it stops forwarding it.

Leave `WHATSAPP_STORE_NAME` unset. Despite its name, the backend passes it to
whatsmeow's `sqlstore.New` as a database connection string. With the default
value (`payverge_whatsapp_store`) that open fails, and the manager falls back to
the backend's own Postgres pool. That fallback is the supported setup: session
data lives in the Payverge database (see [Database](#database)).

Next, open **Business → AI Waiter → Overview**. The WhatsApp channel card shows
a **Connect WhatsApp** button. Connecting, disconnecting and reading the
pairing code require the `settings:write` permission, and the business must not be
suspended or closed by the server administrator.

Pairing works like this:

1. **Connect WhatsApp** calls `POST …/whatsapp/connect`. The backend waits up
   to 20 seconds for whatsmeow's first pairing code and returns it as
   `qr_code`. That stays under the dashboard's 30-second request timeout. The card draws it as a QR code. If whatsmeow ends the attempt
   without a code, the route answers 502; if no code arrives in time, it
   answers 504. Either way, try again.
2. On the phone, open WhatsApp → **Linked devices** → **Link a device**, and
   scan the QR code.
3. whatsmeow rotates the code while nobody has scanned it (about 60 seconds
   for the first, then about every 20 seconds). While the status is
   `pairing`, the card reads the current code from `GET …/whatsapp/qr` every
   3 seconds and redraws the QR. That route returns
   `{"status":"pairing","qr_code":"…"}`, or no `qr_code` once pairing has
   ended. A network error or 5xx keeps the last code and retries. After 5
   failures in a row the card stops, shows an error and re-reads the status.
   A 401, 403 or 404 stops the polling at once.
   These background reads never raise the dashboard's global error toast.
4. After a successful scan the status moves to `connected`. If whatsmeow runs
   out of codes before anyone scans, the attempt expires and the status
   returns to `disconnected`. The same happens when whatsmeow ends the attempt
   any other way without a paired device: a failed pair, an outdated-client
   rejection, or an unexpected connection state. The backend drops the
   half-open connection and nothing is saved. Press **Connect WhatsApp** again
   for a fresh code.

Viewers who only have `settings:read` see the channel status but no QR code.

### Verifying the build

- **Startup log.** Look for `WhatsApp integration enabled`. If you see the
  `WHATSAPP_ENABLED=true is ignored` warning, the image was built without the
  tag.
- **Status route.** `GET /api/v1/inside/businesses/<id>/whatsapp/status`
  returns `"built": true`, `"requested": true` and, once the manager is
  running, `"enabled": true`. `"enabled": false` alongside
  `"requested": true` means the manager failed to start; check the backend log
  for `Failed to initialize WhatsApp manager`.
- **Binary metadata.** Build info survives `-s -w`, so this works on the
  shipped binary:

  ```bash
  docker create --name wa-check payverge-backend:whatsapp
  docker cp wa-check:/app/server ./server-whatsapp && docker rm wa-check
  go version -m ./server-whatsapp | grep -E 'tags=|go.mau.fi'
  ```

  A default image shows no `-tags=whatsapp` setting and no `go.mau.fi` lines.

## Database

`whatsapp_business_devices` is part of the genesis baseline (every build, empty
by default). The `whatsmeow_*` session tables are not in the schema at all.
They are created at runtime by whatsmeow's sqlstore on first start of a build
compiled with `-tags whatsapp`.

Once a device is paired, these tables hold Signal-protocol identity and session
keys. Treat database dumps and backups from a WhatsApp-enabled instance as
secrets.

## Licence gate for contributors

```bash
make check-gpl-free
```

This runs `scripts/ci/check-gpl-free_test.sh`, the gate's own contract test,
and then `scripts/ci/check-gpl-free.sh`. The gate checks three things:

1. `go list -deps` in the Docker build configuration (`CGO_ENABLED=0
   GOOS=linux`, default tags) contains no package under `go.mau.fi` or
   `github.com/ethereum/go-ethereum/cmd`. It checks `./cmd/app`,
   `./cmd/email-smoke`, `./cmd/healthcheck` and `./...`. On failure it names
   the importing package that needs a build tag.
2. `-tags whatsapp` still links `go.mau.fi` into `./cmd/app`. This proves the
   tag split has not silently stopped wiring the integration.
3. `go build -tags whatsapp ./...` compiles. Set
   `CHECK_GPL_FREE_SKIP_TAGGED_BUILD=1` to skip this step on quick local
   re-runs.

Two notes on licences in the dependency graph:

- **go-ethereum.** `go-licenses` reports it as GPL-3.0 because it reads the
  repository-root `COPYING` file. The library packages Payverge links are
  LGPL-3.0. The GPL `cmd/` tree is on the denylist.
- **golang/freetype.** It is dual-licensed; Payverge elects the FreeType
  License.

When you add WhatsApp code:

- Put anything that imports `go.mau.fi/*` in a file with `//go:build whatsapp`.
- Add the matching inert behaviour to `whatsapp_stub.go` (`//go:build
  !whatsapp`).
- Gate runtime behaviour on `services.WhatsAppAvailable()`. It returns true
  only for a tagged build with `WHATSAPP_ENABLED=true`, and it is the value to
  publish as the instance feature flag.
- Run the tests in both configurations:

  ```bash
  go test -short ./internal/services ./internal/server ./internal/aicontract ./cmd/app
  go test -short -tags whatsapp ./internal/services ./internal/server ./internal/aicontract ./cmd/app
  ```

The AI contract matrix (`internal/aicontract`) lists WhatsApp scenarios in
`tagGatedScenarioIDs` for default builds, which skips them there. Tagged builds
register and run the real runners. Tests on both sides fail if that split
drifts.

## See also

- WhatsApp and quota variables: [configuration.md](configuration.md#telegram-whatsapp-and-web-push).
- Building the images yourself: [deploy/README.md](../../deploy/README.md#build-from-source).
