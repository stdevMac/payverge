import {
  fetchPrinters,
  createPrinter,
  fetchJobs,
  createBillPrintJob,
  claimPrintJob,
  renewPrintLease,
  markPresented,
  confirmPrinted,
  retryPrintJob,
  formatPrintJobStatus,
  testPrint,
  isTerminalPrintJobStatus,
} from "./print";

const originalFetch = global.fetch;

beforeEach(() => {
  global.fetch = jest.fn();
});

afterAll(() => {
  global.fetch = originalFetch;
});

describe("printers api client", () => {
  it("GETs /inside/businesses/:id/printers", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [] }),
    });
    await fetchPrinters(42);
    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringMatching(/\/inside\/businesses\/42\/printers$/),
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("POSTs body to create a printer", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 1, name: "p" }),
    });
    await createPrinter(42, {
      name: "p",
      role: "bill",
      transport: "browser",
      paper_width_mm: 80,
    });
    const call = (global.fetch as jest.Mock).mock.calls[0];
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body)).toEqual({
      name: "p",
      role: "bill",
      transport: "browser",
      paper_width_mm: 80,
    });
  });

  it("rejects when fetch returns !ok", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 500,
      text: async () => "boom",
    });
    await expect(fetchJobs(42)).rejects.toThrow(/500/);
  });

  it("throws the backend's message, status and code instead of raw JSON", async () => {
    const body = {
      error: "Disabled in the public demo: test prints.",
      code: "DEMO_MODE_FORBIDDEN",
      params: { reason: "demo_mode", kind: "write" },
    };
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 403,
      text: async () => JSON.stringify(body),
    });
    const err = await testPrint(42, 7).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(Error);
    expect((err as Error).message).toBe(body.error);
    expect((err as Error).message).not.toContain("{");
    expect(err).toMatchObject({
      status: 403,
      code: "DEMO_MODE_FORBIDDEN",
      response: { status: 403, data: body },
    });
  });

  it("falls back to the status when the error body is not JSON", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 502,
      text: async () => "<html>Bad Gateway</html>",
    });
    await expect(fetchJobs(42)).rejects.toThrow("HTTP 502");
  });

  it("filters the pending browser queue by the selected printer", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [] }),
    });

    await fetchJobs(42, {
      status: "routed",
      transport: "browser",
      printer_id: 7,
    });

    expect((global.fetch as jest.Mock).mock.calls[0][0]).toMatch(
      /status=routed&transport=browser&printer_id=7/,
    );
  });

  it("POSTs the operator language when creating a bill print job", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 9 }),
    });
    await createBillPrintJob(42, 7, "es-ar");
    const call = (global.fetch as jest.Mock).mock.calls[0];
    expect(call[0]).toMatch(/\/inside\/businesses\/42\/print\/jobs$/);
    expect(JSON.parse(call[1].body)).toEqual({
      kind: "bill",
      source_type: "bill",
      source_id: 7,
      language: "es-ar",
    });
  });

  it("omits language when none is given (backend falls back to en)", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 9 }),
    });
    await createBillPrintJob(42, 7);
    const call = (global.fetch as jest.Mock).mock.calls[0];
    expect(JSON.parse(call[1].body)).toEqual({
      kind: "bill",
      source_type: "bill",
      source_id: 7,
    });
  });

  it("POSTs claim with client_id and AbortSignal", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ job: null, pending_count: 4 }),
    });
    const ac = new AbortController();
    const result = await claimPrintJob(
      42,
      "11111111-1111-4111-8111-111111111111",
      7,
      {
        signal: ac.signal,
      },
    );
    expect(result).toEqual({ job: null, pending_count: 4 });
    expect(result.pending_count).toBe(4);
    const call = (global.fetch as jest.Mock).mock.calls[0];
    expect(call[0]).toMatch(/\/inside\/businesses\/42\/print\/jobs\/claim$/);
    expect(JSON.parse(call[1].body)).toEqual({
      client_id: "11111111-1111-4111-8111-111111111111",
      printer_id: 7,
    });
    expect(call[1].signal).toBe(ac.signal);
  });

  it("POSTs renew / presented / confirm / retry lease methods", async () => {
    (global.fetch as jest.Mock).mockResolvedValue({
      ok: true,
      json: async () => ({ ok: true }),
    });
    const client = "22222222-2222-4222-8222-222222222222";
    await renewPrintLease(1, 9, client);
    await markPresented(1, 9, client);
    await confirmPrinted(1, 9, client);
    await retryPrintJob(1, 9, client);
    const urls = (global.fetch as jest.Mock).mock.calls.map(
      (c: unknown[]) => c[0] as string,
    );
    expect(urls.some((u) => u.endsWith("/print/jobs/9/renew"))).toBe(true);
    expect(urls.some((u) => u.endsWith("/print/jobs/9/presented"))).toBe(true);
    expect(urls.some((u) => u.endsWith("/print/jobs/9/confirm"))).toBe(true);
    expect(urls.some((u) => u.endsWith("/print/jobs/9/retry"))).toBe(true);
  });

  it("formats known and unknown statuses safely", () => {
    expect(formatPrintJobStatus("failed_retryable")).toMatch(/retry/i);
    expect(formatPrintJobStatus("failed_permanent")).toMatch(/permanent/i);
    expect(formatPrintJobStatus("future_state_xyz")).toBe("future_state_xyz");
    expect(formatPrintJobStatus(undefined)).toBe("unknown");
    expect(isTerminalPrintJobStatus("printed")).toBe(true);
    expect(isTerminalPrintJobStatus("failed_permanent")).toBe(true);
    expect(isTerminalPrintJobStatus("printing")).toBe(false);
  });
});
