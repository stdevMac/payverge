/**
 * Spaces & Tables API client.
 *
 * Mirrors backend routes under /api/v1/inside/businesses/:id/spaces and
 * scan-sessions (see backend/cmd/app/main.go + space_handlers.go /
 * space_scan_handlers.go). Response shapes match RestaurantSpace,
 * SpacesSummary, LayoutDocument, SpaceScanSession, etc.
 */

import { axiosInstance } from "@/api/tools/instance";

// ─── Status / unit / type unions ────────────────────────────────────────────

type SpaceStatus = "draft" | "published" | "archived";

export type SpaceMeasurementUnit = "m" | "ft";

/** Common space types; backend accepts free-form strings (default "indoor"). */
export type SpaceType =
  | "indoor"
  | "outdoor"
  | "patio"
  | "rooftop"
  | "bar"
  | "private"
  | "terrace"
  | (string & {});

export type ScanStatus =
  | "waiting_for_phone"
  | "phone_connected"
  | "scanning"
  | "uploading"
  | "processing"
  | "review_ready"
  | "failed"
  | "expired"
  | "cancelled"
  | "completed"
  | (string & {});

type ScanUploadKind =
  | "roomplan_json"
  | "keyframes"
  | "video"
  | "depth"
  | "metadata"
  | (string & {});

export type TableShape =
  | "round"
  | "square"
  | "rectangle"
  | "oval"
  | "bar"
  | "custom"
  | ""
  | (string & {});

export type LayoutElementType =
  | "wall"
  | "door"
  | "window"
  | "column"
  | "bar"
  | "counter"
  | "entrance"
  | "stairs"
  | "service_station"
  | "restroom"
  | "divider"
  | "obstacle"
  | "label"
  | (string & {});

// ─── Core models ────────────────────────────────────────────────────────────

/** Millimeter point in layout/boundary polygons (origin top-left, X right, Y down). */
interface LayoutPoint {
  x: number;
  y: number;
}

/** RestaurantSpace — floor-plan space owned by a business. */
export interface Space {
  id: number;
  business_id: number;
  name: string;
  space_type: SpaceType;
  floor_level: number;
  sort_order: number;
  measurement_unit: SpaceMeasurementUnit;
  status: SpaceStatus;
  width_mm?: number | null;
  height_mm?: number | null;
  boundary_json?: unknown;
  draft_layout_json?: unknown;
  published_layout_json?: unknown;
  layout_schema_version: number;
  draft_revision: number;
  published_revision: number;
  has_unpublished_changes: boolean;
  scan_source?: string | null;
  archived_at?: string | null;
  created_at: string;
  updated_at: string;
  deleted_at?: string | null;
}

// ─── Layout document (draft/published JSON) ─────────────────────────────────

export interface LayoutBoundary {
  points_mm: LayoutPoint[];
  closed?: boolean;
}

export interface LayoutRegion {
  id?: number;
  name: string;
  color?: string;
  purpose?: string;
  polygon_mm: LayoutPoint[];
}

export interface LayoutElement {
  id?: number;
  element_type: LayoutElementType;
  name?: string;
  geometry?: Record<string, unknown>;
  x_mm?: number;
  y_mm?: number;
  width_mm?: number;
  height_mm?: number;
  rotation_deg?: number;
  z_index?: number;
  meta?: Record<string, unknown>;
}

export interface LayoutTable {
  table_id: number;
  /** Stable candidate id before materialization (scan/editor). */
  client_key?: string;
  name?: string;
  x_mm: number;
  y_mm: number;
  width_mm: number;
  height_mm: number;
  rotation_deg?: number;
  shape: TableShape;
  min_capacity?: number;
  max_capacity?: number;
  visible_seat_count?: number;
  region_id?: number;
  is_reservable?: boolean;
  is_combinable?: boolean;
  is_accessible?: boolean;
}

/** Version-1 draft/published layout JSON shape (spaces.LayoutDocument). */
export interface LayoutDocument {
  schema_version?: number;
  width_mm?: number;
  height_mm?: number;
  measurement_unit?: SpaceMeasurementUnit | string;
  boundary?: LayoutBoundary;
  regions?: LayoutRegion[];
  elements?: LayoutElement[];
  tables?: LayoutTable[];
  meta?: Record<string, unknown>;
}

interface LayoutValidationIssue {
  code: string;
  message: string;
  path?: string;
}

export interface LayoutValidationResult {
  valid: boolean;
  issues?: LayoutValidationIssue[];
}

// ─── Summary / scan ─────────────────────────────────────────────────────────

/** Aggregate counts for the Spaces overview strip. */
export interface SpaceSummary {
  total_spaces: number;
  draft_spaces: number;
  published_spaces: number;
  archived_spaces: number;
  unassigned_tables: number;
  assigned_tables: number;
}

/** Minimal table row as returned by unassigned_tables / assign responses. */
export interface SpaceTableRef {
  id: number;
  business_id: number;
  name: string;
  table_code: string;
  capacity: number;
  is_active: boolean;
  space_id?: number | null;
  [key: string]: unknown;
}

export interface SpacesSummaryResponse {
  summary: SpaceSummary;
  unassigned_tables: SpaceTableRef[];
}

export interface SpaceDetailResponse {
  space: Space;
  tables: SpaceTableRef[];
}

export interface LayoutDraftResponse {
  space_id: number;
  draft_revision: number;
  has_unpublished_changes: boolean;
  layout: LayoutDocument | Record<string, unknown> | null;
  measurement_unit: SpaceMeasurementUnit | string;
  width_mm?: number | null;
  height_mm?: number | null;
}

export interface LayoutPublishedResponse {
  space_id: number;
  published_revision: number;
  status: SpaceStatus | string;
  layout: LayoutDocument | Record<string, unknown> | null;
  measurement_unit: SpaceMeasurementUnit | string;
  width_mm?: number | null;
  height_mm?: number | null;
}

export interface PutDraftResponse {
  space: Space;
  draft_revision: number;
  has_unpublished_changes: boolean;
  validation: LayoutValidationResult;
}

export interface PublishLayoutResponse {
  space: Space;
  validation: LayoutValidationResult;
}

export interface DiscardLayoutResponse {
  space: Space;
}

export interface ScanSession {
  id: number;
  business_id: number;
  space_id?: number | null;
  created_by_user_id?: number | null;
  created_by_staff_id?: number | null;
  /** Only token prefix is exposed on the session row (raw token returned once). */
  token_prefix: string;
  status: ScanStatus;
  expires_at: string;
  connected_at?: string | null;
  completed_at?: string | null;
  progress_pct: number;
  progress_message?: string | null;
  error_code?: string | null;
  error_message?: string | null;
  device_meta_json?: unknown;
  calibration_mm?: number | null;
  idempotency_key?: string | null;
  result_layout_json?: unknown;
  retain_raw_until?: string | null;
  created_at: string;
  updated_at: string;
}

interface ScanUpload {
  id: number;
  session_id: number;
  business_id: number;
  upload_kind: ScanUploadKind;
  content_type?: string | null;
  byte_size?: number | null;
  checksum_sha256?: string | null;
  s3_key?: string | null;
  format_version: number;
  part_index: number;
  is_complete: boolean;
  idempotency_key?: string | null;
  created_at: string;
}

export interface CreateScanSessionResponse {
  session: ScanSession;
  /** Raw opaque token — returned once on create (null on idempotent replay). */
  token?: string | null;
  /** 6-digit pair code — returned once on create. */
  pair_code?: string | null;
  expires_at?: string;
  idempotent?: boolean;
  message?: string;
}

export interface ScanSessionDetailResponse {
  session: ScanSession;
  uploads: ScanUpload[];
}

// ─── Request bodies ─────────────────────────────────────────────────────────

export interface CreateSpaceInput {
  name: string;
  space_type?: SpaceType;
  floor_level?: number;
  sort_order?: number;
  measurement_unit?: SpaceMeasurementUnit;
}

export interface PatchSpaceInput {
  name?: string;
  space_type?: SpaceType;
  floor_level?: number;
  sort_order?: number;
  measurement_unit?: SpaceMeasurementUnit;
}

export interface SpaceSortEntry {
  id: number;
  sort_order: number;
}

export interface PutDraftInput {
  expected_revision: number;
  layout: LayoutDocument | Record<string, unknown>;
}

export interface PublishLayoutInput {
  expected_revision: number;
}

export interface AssignTablesInput {
  table_ids?: number[];
}

export interface CreateScanSessionInput {
  idempotency_key?: string;
  calibration_mm?: number;
}

export interface DuplicateSpaceInput {
  name?: string;
}

// ─── Error codes (structured API errors) ────────────────────────────────────

// ─── Helpers ────────────────────────────────────────────────────────────────

const base = (businessId: number | string) =>
  `/inside/businesses/${businessId}`;

/** Best-effort parse of layout JSON (string or object) into LayoutDocument. */
export function parseLayoutDocument(
  raw: unknown,
): LayoutDocument | null {
  if (raw == null) return null;
  if (typeof raw === "string") {
    const trimmed = raw.trim();
    if (!trimmed || trimmed === "{}" || trimmed === "null") {
      return { schema_version: 1 };
    }
    try {
      return JSON.parse(trimmed) as LayoutDocument;
    } catch {
      return null;
    }
  }
  if (typeof raw === "object") {
    return raw as LayoutDocument;
  }
  return null;
}

/** Table + seat counts derived from a layout document (fallback when relational counts unavailable). */
export function layoutTableStats(layout: LayoutDocument | null | undefined): {
  tableCount: number;
  maxSeats: number;
} {
  const tables = layout?.tables ?? [];
  let maxSeats = 0;
  for (const t of tables) {
    const seats =
      t.max_capacity ?? t.visible_seat_count ?? t.min_capacity ?? 0;
    maxSeats += seats;
  }
  return { tableCount: tables.length, maxSeats };
}

// ─── API ────────────────────────────────────────────────────────────────────

export const spacesApi = {
  // ── Spaces CRUD ─────────────────────────────────────────────────────────

  /** GET /businesses/:id/spaces */
  list: async (
    businessId: number | string,
    opts?: { includeArchived?: boolean },
  ): Promise<Space[]> => {
    const res = await axiosInstance.get(base(businessId) + "/spaces", {
      params: opts?.includeArchived ? { include_archived: "true" } : undefined,
    });
    return res.data.spaces ?? [];
  },

  /** GET /businesses/:id/spaces/summary */
  summary: async (
    businessId: number | string,
  ): Promise<SpacesSummaryResponse> => {
    const res = await axiosInstance.get(base(businessId) + "/spaces/summary");
    return {
      summary: res.data.summary,
      unassigned_tables: res.data.unassigned_tables ?? [],
    };
  },

  /** POST /businesses/:id/spaces */
  create: async (
    businessId: number | string,
    input: CreateSpaceInput,
  ): Promise<Space> => {
    const res = await axiosInstance.post(base(businessId) + "/spaces", input);
    return res.data;
  },

  /** GET /businesses/:id/spaces/:spaceId */
  get: async (
    businessId: number | string,
    spaceId: number,
  ): Promise<SpaceDetailResponse> => {
    const res = await axiosInstance.get(
      base(businessId) + "/spaces/" + spaceId,
    );
    return {
      space: res.data.space,
      tables: res.data.tables ?? [],
    };
  },

  /** PATCH /businesses/:id/spaces/:spaceId */
  patch: async (
    businessId: number | string,
    spaceId: number,
    input: PatchSpaceInput,
  ): Promise<Space> => {
    const res = await axiosInstance.patch(
      base(businessId) + "/spaces/" + spaceId,
      input,
    );
    return res.data;
  },

  /** POST /businesses/:id/spaces/:spaceId/duplicate */
  duplicate: async (
    businessId: number | string,
    spaceId: number,
    input?: DuplicateSpaceInput,
  ): Promise<Space> => {
    const res = await axiosInstance.post(
      base(businessId) + "/spaces/" + spaceId + "/duplicate",
      input ?? {},
    );
    return res.data;
  },

  /** POST /businesses/:id/spaces/reorder — accepts { items } or bare array. */
  reorder: async (
    businessId: number | string,
    items: SpaceSortEntry[],
  ): Promise<Space[]> => {
    const res = await axiosInstance.post(
      base(businessId) + "/spaces/reorder",
      { items },
    );
    return res.data.spaces ?? [];
  },

  /** POST /businesses/:id/spaces/:spaceId/archive */
  archive: async (
    businessId: number | string,
    spaceId: number,
  ): Promise<{ space: Space; status: string }> => {
    const res = await axiosInstance.post(
      base(businessId) + "/spaces/" + spaceId + "/archive",
    );
    return res.data;
  },

  /**
   * DELETE /businesses/:id/spaces/:spaceId
   * Soft-delete; 409 has_dependencies when open bills/reservations exist.
   */
  remove: async (
    businessId: number | string,
    spaceId: number,
  ): Promise<{ deleted: boolean; space_id: number }> => {
    const res = await axiosInstance.delete(
      base(businessId) + "/spaces/" + spaceId,
    );
    return res.data;
  },

  // ── Layout ──────────────────────────────────────────────────────────────

  /** GET .../layout/draft */
  getLayoutDraft: async (
    businessId: number | string,
    spaceId: number,
  ): Promise<LayoutDraftResponse> => {
    const res = await axiosInstance.get(
      base(businessId) + "/spaces/" + spaceId + "/layout/draft",
    );
    return res.data;
  },

  /** PUT .../layout/draft (optimistic concurrency via expected_revision) */
  putLayoutDraft: async (
    businessId: number | string,
    spaceId: number,
    input: PutDraftInput,
  ): Promise<PutDraftResponse> => {
    const res = await axiosInstance.put(
      base(businessId) + "/spaces/" + spaceId + "/layout/draft",
      input,
    );
    return res.data;
  },

  /** POST .../layout/validate */
  validateLayout: async (
    businessId: number | string,
    spaceId: number,
    layout?: LayoutDocument | Record<string, unknown>,
  ): Promise<LayoutValidationResult> => {
    const res = await axiosInstance.post(
      base(businessId) + "/spaces/" + spaceId + "/layout/validate",
      layout != null ? { layout } : {},
    );
    return res.data;
  },

  /** POST .../layout/publish */
  publishLayout: async (
    businessId: number | string,
    spaceId: number,
    input: PublishLayoutInput,
  ): Promise<PublishLayoutResponse> => {
    const res = await axiosInstance.post(
      base(businessId) + "/spaces/" + spaceId + "/layout/publish",
      input,
    );
    return res.data;
  },

  /** POST .../layout/discard */
  discardLayout: async (
    businessId: number | string,
    spaceId: number,
    input: PublishLayoutInput,
  ): Promise<DiscardLayoutResponse> => {
    const res = await axiosInstance.post(
      base(businessId) + "/spaces/" + spaceId + "/layout/discard",
      input,
    );
    return res.data;
  },

  /** GET .../layout/published */
  getLayoutPublished: async (
    businessId: number | string,
    spaceId: number,
  ): Promise<LayoutPublishedResponse> => {
    const res = await axiosInstance.get(
      base(businessId) + "/spaces/" + spaceId + "/layout/published",
    );
    return res.data;
  },

  /** POST .../tables/assign — empty table_ids assigns all unassigned. */
  assignTables: async (
    businessId: number | string,
    spaceId: number,
    input?: AssignTablesInput,
  ): Promise<{ tables: SpaceTableRef[] }> => {
    const res = await axiosInstance.post(
      base(businessId) + "/spaces/" + spaceId + "/tables/assign",
      input ?? {},
    );
    return { tables: res.data.tables ?? [] };
  },

  // ── Scan sessions ───────────────────────────────────────────────────────

  /** POST .../spaces/:spaceId/scan-sessions */
  createScanSession: async (
    businessId: number | string,
    spaceId: number,
    input?: CreateScanSessionInput,
  ): Promise<CreateScanSessionResponse> => {
    const res = await axiosInstance.post(
      base(businessId) + "/spaces/" + spaceId + "/scan-sessions",
      input ?? {},
    );
    return res.data;
  },

  /** GET /businesses/:id/scan-sessions/:sessionId */
  getScanSession: async (
    businessId: number | string,
    sessionId: number,
  ): Promise<ScanSessionDetailResponse> => {
    const res = await axiosInstance.get(
      base(businessId) + "/scan-sessions/" + sessionId,
    );
    return {
      session: res.data.session,
      uploads: res.data.uploads ?? [],
    };
  },

  /** POST /businesses/:id/scan-sessions/:sessionId/cancel */
  cancelScanSession: async (
    businessId: number | string,
    sessionId: number,
  ): Promise<{ session: ScanSession; status?: string }> => {
    const res = await axiosInstance.post(
      base(businessId) + "/scan-sessions/" + sessionId + "/cancel",
    );
    return res.data;
  },

  /** POST /businesses/:id/scan-sessions/:sessionId/retry-process */
  retryProcessScanSession: async (
    businessId: number | string,
    sessionId: number,
  ): Promise<{ session: ScanSession; enqueued?: boolean }> => {
    const res = await axiosInstance.post(
      base(businessId) + "/scan-sessions/" + sessionId + "/retry-process",
    );
    return res.data;
  },

  /** POST /businesses/:id/scan-sessions/:sessionId/complete — revoke token after review. */
  completeScanSession: async (
    businessId: number | string,
    sessionId: number,
  ): Promise<{ status: string; completed: boolean }> => {
    const res = await axiosInstance.post(
      base(businessId) + "/scan-sessions/" + sessionId + "/complete",
    );
    return res.data;
  },
};

// ─── Public phone-scan token routes (no account access from token alone) ────

export interface PublicScanSessionMeta {
  status: ScanStatus;
  expires_at: string;
  progress_pct: number;
  business_name?: string;
  space_name?: string;
}

export interface PublicScanConnectInput {
  pair_code?: string;
  device_meta?: Record<string, unknown>;
}

export interface PublicScanConnectResponse {
  status: ScanStatus;
  expires_at: string;
  progress_pct: number;
}

export interface PublicScanStatusInput {
  status: "scanning" | "uploading";
  progress_pct?: number;
  progress_message?: string;
}

export interface PublicScanUploadInput {
  upload_kind: ScanUploadKind;
  content_type?: string;
  checksum_sha256?: string;
  part_index?: number;
  /** JSON payload (keyframes, roomplan, metadata). */
  payload?: unknown;
  /** Base64 binary alternative. */
  payload_base64?: string;
  /** Optional Idempotency-Key header value. */
  idempotency_key?: string;
}

export interface PublicScanUploadResponse {
  upload: ScanUpload;
  idempotent?: boolean;
}

export interface PublicScanCompleteResponse {
  session?: ScanSession;
  status: ScanStatus | string;
  enqueued?: boolean;
}

export interface PublicScanResultResponse {
  status: ScanStatus | string;
  layout: LayoutDocument | Record<string, unknown> | null;
  /** Current space draft_revision for strict CAS on apply-review. */
  draft_revision?: number;
  draft_apply_failed?: boolean;
  error_code?: string | null;
  error_message?: string | null;
  expires_at?: string;
}

export interface PublicScanApplyReviewResponse {
  status: ScanStatus | string;
  draft_revision?: number;
  completed?: boolean;
  validation?: LayoutValidationResult;
}

/** SHA-256 hex of a string or ArrayBuffer (browser SubtleCrypto). */
export async function sha256Hex(
  data: string | ArrayBuffer | Uint8Array,
): Promise<string> {
  let bytes: ArrayBuffer;
  if (typeof data === "string") {
    bytes = new TextEncoder().encode(data).buffer;
  } else if (data instanceof Uint8Array) {
    bytes = data.buffer.slice(
      data.byteOffset,
      data.byteOffset + data.byteLength,
    ) as ArrayBuffer;
  } else {
    bytes = data;
  }
  const hash = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(hash))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

/**
 * Public (token) space-scan endpoints. Uses axios with credentials so a
 * logged-in staff cookie can be attempted on connect; pair_code remains the
 * primary phone auth path (token alone never grants account access).
 */
export const publicSpaceScanApi = {
  /** GET /space-scan/:token */
  getMeta: async (token: string): Promise<PublicScanSessionMeta> => {
    const res = await axiosInstance.get(
      `/space-scan/${encodeURIComponent(token)}`,
    );
    return res.data;
  },

  /** POST /space-scan/:token/connect */
  connect: async (
    token: string,
    input?: PublicScanConnectInput,
  ): Promise<PublicScanConnectResponse> => {
    const res = await axiosInstance.post(
      `/space-scan/${encodeURIComponent(token)}/connect`,
      input ?? {},
    );
    return res.data;
  },

  /** POST /space-scan/:token/status */
  updateStatus: async (
    token: string,
    input: PublicScanStatusInput,
  ): Promise<{ status: ScanStatus; progress_pct: number }> => {
    const res = await axiosInstance.post(
      `/space-scan/${encodeURIComponent(token)}/status`,
      input,
    );
    return res.data;
  },

  /** POST /space-scan/:token/uploads — JSON body with checksum + optional Idempotency-Key. */
  upload: async (
    token: string,
    input: PublicScanUploadInput,
  ): Promise<PublicScanUploadResponse> => {
    const headers: Record<string, string> = {};
    if (input.idempotency_key) {
      headers["Idempotency-Key"] = input.idempotency_key;
    }
    const body: Record<string, unknown> = {
      upload_kind: input.upload_kind,
      content_type: input.content_type ?? "application/json",
      part_index: input.part_index ?? 0,
    };
    if (input.checksum_sha256) {
      body.checksum_sha256 = input.checksum_sha256;
    }
    if (input.payload_base64 != null) {
      body.payload_base64 = input.payload_base64;
    } else if (input.payload !== undefined) {
      body.payload = input.payload;
    }
    const res = await axiosInstance.post(
      `/space-scan/${encodeURIComponent(token)}/uploads`,
      body,
      { headers },
    );
    return res.data;
  },

  /** POST /space-scan/:token/complete-upload — starts real backend processing. */
  completeUpload: async (
    token: string,
  ): Promise<PublicScanCompleteResponse> => {
    const res = await axiosInstance.post(
      `/space-scan/${encodeURIComponent(token)}/complete-upload`,
      {},
    );
    return res.data;
  },

  /** GET /space-scan/:token/result — only when review_ready (short TTL window). */
  getResult: async (token: string): Promise<PublicScanResultResponse> => {
    const res = await axiosInstance.get(
      `/space-scan/${encodeURIComponent(token)}/result`,
    );
    return res.data;
  },

  /**
   * POST /space-scan/:token/apply-review — persists filtered layout to the space
   * draft and completes/revokes the session (mobile + optional desktop path).
   * expected_revision is required (strict CAS; server rejects <= 0).
   */
  applyReview: async (
    token: string,
    layout: LayoutDocument | Record<string, unknown>,
    expectedRevision: number,
  ): Promise<PublicScanApplyReviewResponse> => {
    const res = await axiosInstance.post(
      `/space-scan/${encodeURIComponent(token)}/apply-review`,
      { layout, expected_revision: expectedRevision },
    );
    return res.data;
  },
};
