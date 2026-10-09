import {
  spacesApi,
  publicSpaceScanApi,
  parseLayoutDocument,
  layoutTableStats,
} from "@/api/spaces";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
  },
}));

const mocked = axiosInstance as unknown as {
  get: jest.Mock;
  post: jest.Mock;
  patch: jest.Mock;
  put: jest.Mock;
  delete: jest.Mock;
};

const sampleSpace = {
  id: 7,
  business_id: 42,
  name: "Main Dining",
  space_type: "indoor",
  floor_level: 0,
  sort_order: 0,
  measurement_unit: "m",
  status: "draft",
  layout_schema_version: 1,
  draft_revision: 1,
  published_revision: 0,
  has_unpublished_changes: false,
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
};

describe("spacesApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("lists spaces without archived by default", async () => {
    mocked.get.mockResolvedValue({ data: { spaces: [sampleSpace] } });
    const out = await spacesApi.list(42);
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/spaces", {
      params: undefined,
    });
    expect(out).toHaveLength(1);
    expect(out[0].name).toBe("Main Dining");
  });

  it("lists spaces with include_archived", async () => {
    mocked.get.mockResolvedValue({ data: { spaces: [] } });
    await spacesApi.list("42", { includeArchived: true });
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/spaces", {
      params: { include_archived: "true" },
    });
  });

  it("fetches summary + unassigned tables", async () => {
    mocked.get.mockResolvedValue({
      data: {
        summary: {
          total_spaces: 2,
          draft_spaces: 1,
          published_spaces: 1,
          archived_spaces: 0,
          unassigned_tables: 3,
          assigned_tables: 10,
        },
        unassigned_tables: [{ id: 1, name: "T1" }],
      },
    });
    const out = await spacesApi.summary(42);
    expect(mocked.get).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/summary",
    );
    expect(out.summary.total_spaces).toBe(2);
    expect(out.unassigned_tables).toHaveLength(1);
  });

  it("creates a space and returns the created row", async () => {
    mocked.post.mockResolvedValue({ data: sampleSpace });
    const out = await spacesApi.create(42, {
      name: "Main Dining",
      space_type: "indoor",
      measurement_unit: "m",
    });
    expect(mocked.post).toHaveBeenCalledWith("/inside/businesses/42/spaces", {
      name: "Main Dining",
      space_type: "indoor",
      measurement_unit: "m",
    });
    expect(out.id).toBe(7);
  });

  it("gets a space with tables", async () => {
    mocked.get.mockResolvedValue({
      data: { space: sampleSpace, tables: [{ id: 9, name: "A1" }] },
    });
    const out = await spacesApi.get(42, 7);
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/spaces/7");
    expect(out.space.id).toBe(7);
    expect(out.tables[0].id).toBe(9);
  });

  it("patches space metadata", async () => {
    mocked.patch.mockResolvedValue({
      data: { ...sampleSpace, name: "Patio" },
    });
    const out = await spacesApi.patch(42, 7, { name: "Patio" });
    expect(mocked.patch).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7",
      { name: "Patio" },
    );
    expect(out.name).toBe("Patio");
  });

  it("duplicates a space", async () => {
    mocked.post.mockResolvedValue({
      data: { ...sampleSpace, id: 8, name: "Main Dining (copy)" },
    });
    const out = await spacesApi.duplicate(42, 7, { name: "Main Dining (copy)" });
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/duplicate",
      { name: "Main Dining (copy)" },
    );
    expect(out.id).toBe(8);
  });

  it("reorders spaces with wrapped items payload", async () => {
    mocked.post.mockResolvedValue({ data: { spaces: [sampleSpace] } });
    const out = await spacesApi.reorder(42, [
      { id: 7, sort_order: 0 },
      { id: 8, sort_order: 1 },
    ]);
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/reorder",
      {
        items: [
          { id: 7, sort_order: 0 },
          { id: 8, sort_order: 1 },
        ],
      },
    );
    expect(out).toHaveLength(1);
  });

  it("archives a space", async () => {
    mocked.post.mockResolvedValue({
      data: { space: { ...sampleSpace, status: "archived" }, status: "archived" },
    });
    const out = await spacesApi.archive(42, 7);
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/archive",
    );
    expect(out.status).toBe("archived");
  });

  it("deletes a space", async () => {
    mocked.delete.mockResolvedValue({
      data: { deleted: true, space_id: 7 },
    });
    const out = await spacesApi.remove(42, 7);
    expect(mocked.delete).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7",
    );
    expect(out.deleted).toBe(true);
  });

  it("gets and puts layout draft", async () => {
    mocked.get.mockResolvedValue({
      data: {
        space_id: 7,
        draft_revision: 2,
        has_unpublished_changes: true,
        layout: { schema_version: 1, tables: [] },
        measurement_unit: "m",
      },
    });
    const draft = await spacesApi.getLayoutDraft(42, 7);
    expect(mocked.get).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/layout/draft",
    );
    expect(draft.draft_revision).toBe(2);

    mocked.put.mockResolvedValue({
      data: {
        space: sampleSpace,
        draft_revision: 3,
        has_unpublished_changes: true,
        validation: { valid: true },
      },
    });
    const saved = await spacesApi.putLayoutDraft(42, 7, {
      expected_revision: 2,
      layout: { schema_version: 1, width_mm: 10000, height_mm: 8000 },
    });
    expect(mocked.put).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/layout/draft",
      {
        expected_revision: 2,
        layout: { schema_version: 1, width_mm: 10000, height_mm: 8000 },
      },
    );
    expect(saved.draft_revision).toBe(3);
  });

  it("validates, publishes, discards, and loads published layout", async () => {
    mocked.post.mockResolvedValueOnce({ data: { valid: true, issues: [] } });
    const v = await spacesApi.validateLayout(42, 7);
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/layout/validate",
      {},
    );
    expect(v.valid).toBe(true);

    mocked.post.mockResolvedValueOnce({
      data: { space: sampleSpace, validation: { valid: true } },
    });
    await spacesApi.publishLayout(42, 7, { expected_revision: 3 });
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/layout/publish",
      { expected_revision: 3 },
    );

    mocked.post.mockResolvedValueOnce({ data: { space: sampleSpace } });
    await spacesApi.discardLayout(42, 7, { expected_revision: 3 });
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/layout/discard",
      { expected_revision: 3 },
    );

    mocked.get.mockResolvedValue({
      data: {
        space_id: 7,
        published_revision: 3,
        status: "published",
        layout: {},
        measurement_unit: "m",
      },
    });
    const pub = await spacesApi.getLayoutPublished(42, 7);
    expect(pub.status).toBe("published");
  });

  it("assigns tables to a space", async () => {
    mocked.post.mockResolvedValue({ data: { tables: [{ id: 1 }] } });
    const out = await spacesApi.assignTables(42, 7, { table_ids: [1, 2] });
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/tables/assign",
      { table_ids: [1, 2] },
    );
    expect(out.tables).toHaveLength(1);
  });

  it("creates and manages scan sessions", async () => {
    mocked.post.mockResolvedValueOnce({
      data: {
        session: { id: 99, status: "waiting_for_phone", progress_pct: 0 },
        token: "tok_abc",
        pair_code: "123456",
      },
    });
    const created = await spacesApi.createScanSession(42, 7, {
      calibration_mm: 1000,
    });
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/spaces/7/scan-sessions",
      { calibration_mm: 1000 },
    );
    expect(created.token).toBe("tok_abc");

    mocked.get.mockResolvedValue({
      data: {
        session: { id: 99, status: "processing" },
        uploads: [],
      },
    });
    const detail = await spacesApi.getScanSession(42, 99);
    expect(mocked.get).toHaveBeenCalledWith(
      "/inside/businesses/42/scan-sessions/99",
    );
    expect(detail.session.id).toBe(99);

    mocked.post.mockResolvedValueOnce({
      data: { session: { id: 99, status: "cancelled" } },
    });
    await spacesApi.cancelScanSession(42, 99);
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/scan-sessions/99/cancel",
    );

    mocked.post.mockResolvedValueOnce({
      data: { session: { id: 99, status: "processing" }, enqueued: true },
    });
    const retry = await spacesApi.retryProcessScanSession(42, 99);
    expect(retry.enqueued).toBe(true);
  });
});

describe("publicSpaceScanApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("hits public token routes for meta, connect, upload, complete", async () => {
    mocked.get.mockResolvedValueOnce({
      data: {
        status: "waiting_for_phone",
        expires_at: "2026-07-01T01:00:00Z",
        progress_pct: 0,
        business_name: "Cafe",
      },
    });
    const meta = await publicSpaceScanApi.getMeta("tok123");
    expect(mocked.get).toHaveBeenCalledWith("/space-scan/tok123");
    expect(meta.status).toBe("waiting_for_phone");

    mocked.post.mockResolvedValueOnce({
      data: { status: "phone_connected", progress_pct: 5 },
    });
    await publicSpaceScanApi.connect("tok123", { pair_code: "111222" });
    expect(mocked.post).toHaveBeenCalledWith(
      "/space-scan/tok123/connect",
      { pair_code: "111222" },
    );

    mocked.post.mockResolvedValueOnce({
      data: { upload: { id: 1, is_complete: true } },
    });
    await publicSpaceScanApi.upload("tok123", {
      upload_kind: "keyframes",
      checksum_sha256: "abc",
      payload: { frames: [] },
      idempotency_key: "idem-1",
    });
    expect(mocked.post).toHaveBeenCalledWith(
      "/space-scan/tok123/uploads",
      expect.objectContaining({
        upload_kind: "keyframes",
        checksum_sha256: "abc",
      }),
      expect.objectContaining({
        headers: expect.objectContaining({ "Idempotency-Key": "idem-1" }),
      }),
    );

    mocked.post.mockResolvedValueOnce({
      data: { status: "processing", enqueued: true },
    });
    const done = await publicSpaceScanApi.completeUpload("tok123");
    expect(done.enqueued).toBe(true);
  });
});

describe("parseLayoutDocument / layoutTableStats", () => {
  it("parses empty and string layouts", () => {
    expect(parseLayoutDocument(null)).toBeNull();
    expect(parseLayoutDocument("{}")?.schema_version).toBe(1);
    expect(parseLayoutDocument({ schema_version: 1, width_mm: 5 })?.width_mm).toBe(
      5,
    );
  });

  it("sums table seats from layout", () => {
    const stats = layoutTableStats({
      schema_version: 1,
      tables: [
        {
          table_id: 1,
          x_mm: 0,
          y_mm: 0,
          width_mm: 1000,
          height_mm: 1000,
          shape: "round",
          max_capacity: 4,
        },
        {
          table_id: 2,
          x_mm: 2000,
          y_mm: 0,
          width_mm: 1200,
          height_mm: 800,
          shape: "rectangle",
          visible_seat_count: 6,
        },
      ],
    });
    expect(stats.tableCount).toBe(2);
    expect(stats.maxSeats).toBe(10);
  });
});
