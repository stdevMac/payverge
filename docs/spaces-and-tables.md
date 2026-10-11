# Spaces & Tables

Production guide for the restaurant floor-plan feature: multi-space layouts, manual editor, phone scan, draft/publish, live map, and legacy table assignment.

## What it is

Operators design one or more **spaces** (main dining room, patio, bar, rooftop, …). Each space has:

- A **draft layout** document (JSON) edited in the canvas
- An optional **published layout** used by Live Map and operational surfaces
- **Tables** that remain the existing `tables` entity (QR codes, bills, reservations) — never a parallel table system
- Optional **regions** (zones) and structural **elements** (walls, doors, bar, …)

Scan flow lets a phone capture a rough draft (web keyframes today; RoomPlan JSON when a native iOS capture app posts structured payloads). Results land as **draft only** — never auto-publish.

## Architecture

```
┌──────────────────┐     hybrid JWT + RBAC      ┌────────────────────────────┐
│  Dashboard FE    │ ─────────────────────────► │  /api/v1/inside/.../spaces │
│  Tables → Spaces │                            │  layout draft/publish      │
│  SpaceEditorPage │ ◄── SSE space.scan.updated │  scan-sessions             │
└────────┬─────────┘                            └─────────────┬──────────────┘
         │                                                    │
         │ QR / URL                                           │ enqueue
         ▼                                                    ▼
┌──────────────────┐   opaque token (+ pair)    ┌────────────────────────────┐
│ /space-scan/[tok]│ ─────────────────────────► │  /api/v1/space-scan/:token │
│ SpaceScanClient  │   connect / upload / done  │  public, rate-limited      │
└──────────────────┘                            └─────────────┬──────────────┘
                                                              │
                                                              ▼
                                                ┌────────────────────────────┐
                                                │ SpaceScanWorker            │
                                                │ RoomPlan / Keyframe        │
                                                │ processors → draft layout  │
                                                │ ArtifactStore (S3|.local)  │
                                                └────────────────────────────┘
```

### Backend packages

| Path | Role |
|------|------|
| `backend/schema/genesis/current_schema.sql` | Schema (spaces, regions, elements, scan sessions/uploads, audit, table layout columns, combinations) |
| `backend/internal/database/space*.go` | Models + business-scoped accessors |
| `backend/internal/spaces/` | Domain service: validate, draft concurrency, publish, discard, duplicate, safe delete, assign legacy tables |
| `backend/internal/spaces/geometry/` | Canonical mm coords, seat layout, boundary validation |
| `backend/internal/spaces/scan/` | `RoomPlanProcessor`, `KeyframeProcessor`, `ArtifactStore` |
| `backend/internal/server/space_handlers.go` | CRUD + layout endpoints |
| `backend/internal/server/space_scan_handlers.go` | Operator scan sessions + public phone routes |
| `backend/internal/services/space_scan_worker.go` | Async processing + SSE |
| `backend/internal/services/space_scan_retention_janitor.go` | Delete raw artifacts after `retain_raw_until` |

### Frontend

| Path | Role |
|------|------|
| `frontend/src/api/spaces.ts` | Typed client (inside + public scan) |
| `frontend/src/components/business/spaces/` | Overview, create modal, unassigned panel, scan modal/review |
| `frontend/src/components/business/spaces/editor/` | Canvas editor (draft autosave, publish, regions, tools) |
| `frontend/src/components/business/tables/TablesLiveMap.tsx` | Published layout + live table status |
| `frontend/src/app/space-scan/[token]/` | Mobile capture UI |
| `frontend/src/i18n/messages/{en,es,es-ar}/spacesTables.json` | Operator copy |
| Tables tab deep-links | `?tablesView=spaces\|live` and optional `?spaceId=` |

### Permissions

Reuses existing table RBAC (no new keys):

| Action | Permission |
|--------|------------|
| List/read spaces & layouts, poll scan | `tables:read` |
| Create space / duplicate | `tables:create` |
| Patch, reorder, archive, draft write, publish, assign, scan | `tables:write` |
| Soft-delete space | `tables:delete` |

SSE event `space.scan.updated` is scoped to `tables:read` in `internal/events/sse_permissions.go`.

## API surface

### Inside (auth + subscription + RBAC)

```
GET    /api/v1/inside/businesses/:id/spaces
GET    /api/v1/inside/businesses/:id/spaces/summary
POST   /api/v1/inside/businesses/:id/spaces
POST   /api/v1/inside/businesses/:id/spaces/reorder
GET    /api/v1/inside/businesses/:id/spaces/:spaceId
PATCH  /api/v1/inside/businesses/:id/spaces/:spaceId
POST   /api/v1/inside/businesses/:id/spaces/:spaceId/duplicate
POST   /api/v1/inside/businesses/:id/spaces/:spaceId/archive
DELETE /api/v1/inside/businesses/:id/spaces/:spaceId
GET    /api/v1/inside/businesses/:id/spaces/:spaceId/layout/draft
PUT    /api/v1/inside/businesses/:id/spaces/:spaceId/layout/draft
POST   /api/v1/inside/businesses/:id/spaces/:spaceId/layout/validate
POST   /api/v1/inside/businesses/:id/spaces/:spaceId/layout/publish
POST   /api/v1/inside/businesses/:id/spaces/:spaceId/layout/discard
GET    /api/v1/inside/businesses/:id/spaces/:spaceId/layout/published
POST   /api/v1/inside/businesses/:id/spaces/:spaceId/tables/assign
POST   /api/v1/inside/businesses/:id/spaces/:spaceId/scan-sessions
GET    /api/v1/inside/businesses/:id/scan-sessions/:sessionId
POST   /api/v1/inside/businesses/:id/scan-sessions/:sessionId/cancel
POST   /api/v1/inside/businesses/:id/scan-sessions/:sessionId/retry-process
```

Draft writes and publish use **optimistic concurrency** (`expected_revision`). Mismatch → `409` with `code: revision_conflict`.

**Draft validation** allows scan/editor **candidate tables** (`table_id: 0`). Overlap and out-of-bounds placements are **soft warnings** (they do not block autosave). Hard failures (zero size, bad polygons, invalid shapes) still return `422 layout_invalid`.

**Publish** materializes candidates into real `tables` rows (QR codes assigned), requires linked IDs, and syncs `capacity` plus reservable/combinable/accessible flags onto the relational table row — **in one transaction** with draft rewrite + published layout + position sync (failure rolls back; no half-published success, no orphan tables on CAS conflict). Soft geometry warnings remain non-blocking.

Safe delete/archive returns `409` with `code: has_dependencies` when open bills or active reservations exist on tables in the space. Tables and QR codes are never hard-deleted; they are unassigned from the space.

### Public phone-scan (rate limited)

```
GET  /api/v1/space-scan/:token
POST /api/v1/space-scan/:token/connect
POST /api/v1/space-scan/:token/status
POST /api/v1/space-scan/:token/uploads
POST /api/v1/space-scan/:token/complete-upload
GET  /api/v1/space-scan/:token/result
```

- Token is crypto/rand base64url; **only SHA-256 hash** is stored.
- Token returned **once** at session create; pair code also returned once.
- Connect accepts: (1) valid pair code, or (2) logged-in staff/owner with business access (optional JWT/cookie hydrate — no abort on missing token so pair code still works).
- Token alone never grants account access or business IDs in public meta responses.
- Upload kinds with processors: `roomplan_json`, `keyframes`, `metadata`. `video`/`depth` are **rejected at upload** (`unsupported_upload_kind`) until a processor ships.
- Token stays valid only during the short `review_ready` window (not past `expires_at`). `GET /result` returns 410 when expired. Operator apply (`POST /space-scan/:token/apply-review` or authenticated `.../scan-sessions/:id/complete`) **completes and revokes** the token so layout is no longer readable.
- Worker draft-apply failures set `error_code=draft_apply_failed` on the session (still `review_ready` with `result_layout_json` for re-apply).

## Layout document (schema v1)

Canonical coordinates: origin top-left, X right, Y down, units **millimeters**, rotation degrees clockwise from +X.

```json
{
  "schema_version": 1,
  "width_mm": 12000,
  "height_mm": 8000,
  "measurement_unit": "m",
  "boundary": { "points_mm": [{"x":0,"y":0}, ...], "closed": true },
  "regions": [{ "name": "Bar", "color": "#1a6b6a", "polygon_mm": [...] }],
  "elements": [{ "element_type": "wall", "x_mm": 0, "y_mm": 0, "width_mm": 100, "height_mm": 8000 }],
  "tables": [{
    "table_id": 42,
    "name": "T1",
    "x_mm": 1000, "y_mm": 1200, "width_mm": 800, "height_mm": 800,
    "rotation_deg": 0, "shape": "square",
    "min_capacity": 2, "max_capacity": 4, "visible_seat_count": 4
  }],
  "meta": { "source": "keyframe", "approximate": true, "confidence": 0.55 }
}
```

On **publish**, relational table placement columns (`space_id`, `pos_*_mm`, shape, capacities, `layout_published`) are synced for maps and reservation grouping.

## Environment variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `SPACE_SCAN_SESSION_TTL_MINUTES` | `30` | Opaque token / session lifetime |
| `SPACE_SCAN_MAX_UPLOAD_BYTES` | `26214400` (25 MiB) | Max upload part size |
| `SPACE_SCAN_RAW_RETENTION_DAYS` | `7` | Days before raw S3/.local cleanup (`0` disables janitor) |
| `SPACE_SCAN_MAX_FRAMES` | `120` | Max keyframes accepted per upload |

Documented in root `.env.example`.

**Artifact storage:** protected S3 when configured; otherwise local fallback under `.local/space-scans/` (gitignored) via the same `ArtifactStore` interface — suitable for local/dev without AWS.

## Operator usage

### Create and draw

1. Open **Tables** → **Spaces & Tables** (`?tablesView=spaces`).
2. **Create space** → name, type, floor, unit → **Draw manually**.
3. Editor: place boundary, tables, walls/regions; autosave drafts (~800ms debounce).
4. **Publish** when ready (validates layout; soft warnings for overlap/capacity are non-blocking).
5. **Discard** reverts draft to last published.

### Phone scan (web)

1. Create or open a space → **Scan**.
2. Desktop shows QR + pair code; phone opens `/space-scan/{token}`.
3. Connect with pair code **or** existing staff/owner session cookies.
4. Capture guided keyframes (camera). Web path is an **approximate room outline** — not metric RoomPlan and **not automatic table detection**.
5. Upload → processing worker → **review_ready** draft (SSE + poll). Review any candidates, open the editor to place tables, then publish.

### RoomPlan / native iOS (external prerequisite)

True Apple RoomPlan capture requires a **native iOS app** (not shipped in this web monorepo). When available, post `upload_kind=roomplan_json` payloads; `RoomPlanProcessor` maps structured floors/walls/objects into a draft. Web operators always use the keyframe path.

### Legacy tables

Existing QR tables without a space appear under **Unassigned**. Assign to a space (IDs and QR codes preserved), then place them on the draft canvas.

### Live map

When any published space exists, Tables → Live View offers **List | Map**. Map shows published layout with live table statuses; click opens existing table detail. Multi-space switcher + region/status filters.

### Reservations

`ReservationTablePicker` groups options by space when `space_id` is set — soft enhancement; unassigned tables still appear.

### Onboarding

Optional setup step **layout** after tables exist (`spacesTables.onboarding.*`). Never required for go-live (`required_done` / `all_done` ignore layout).

## Compatibility

| Surface | Behavior |
|---------|----------|
| Existing tables | Nullable layout columns; create/update APIs remain name/capacity/QR-only |
| QR guest flow | Unchanged — still keyed by `table_code` |
| Bills / orders | Unchanged; open bills block space archive/delete |
| Reservations | Unchanged; active reservations block space archive/delete; optional space grouping in picker |
| Soft delete space | Unassigns tables (`space_id` SET NULL via FK / explicit unassign); does not delete table rows |

## Tests (reference)

```bash
# Backend domain + handlers
cd backend
go test ./internal/spaces/... ./internal/server/ -count=1 -run 'Space|Geometry|Layout|Scan'

# Frontend unit
cd frontend
npx jest --watchman=false --runInBand --testPathPatterns='spaces|space-scan|SpacesOverview|SpaceEditor|SpaceScan|ReservationTablePicker|spaces\.test'
```

## Related docs

- Codemap notes: `docs/CODEMAPS/frontend.md`, `docs/CODEMAPS/backend.md`
- Agent guide: `Agents.md` (RBAC, migrations policy)
- Env template: `.env.example` (`SPACE_SCAN_*`)
