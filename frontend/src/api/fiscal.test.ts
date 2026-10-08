import {
  creditFiscalReceipt,
  fiscalApi,
  getSettings,
  issueReceipt,
  listReceipts,
  resendFiscalReceipt,
  retryReceipt,
  updateSettings,
  uploadFiscalCredentials,
  validateFiscalSettings,
} from "@/api/fiscal";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    put: jest.fn(),
    post: jest.fn(),
  },
}));

describe("fiscalApi", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads fiscal settings from the inside business route", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { settings: { id: 7, mode: "manual" } },
    });

    const result = await getSettings(42);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/settings",
      { _skipErrorToast: true },
    );
    expect(result).toEqual({ id: 7, mode: "manual" });
  });

  it("updates fiscal settings with a JSON body", async () => {
    (axiosInstance.put as jest.Mock).mockResolvedValue({
      data: { id: 7, mode: "automatic_non_blocking" },
    });

    await updateSettings(42, {
      country: "AR",
      provider: "arca",
      mode: "automatic_non_blocking",
      environment: "sandbox",
      tax_id: "20123456789",
      tax_condition: "monotributo",
      point_of_sale: 1,
    });

    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/settings",
      {
        country: "AR",
        provider: "arca",
        mode: "automatic_non_blocking",
        environment: "sandbox",
        tax_id: "20123456789",
        tax_condition: "monotributo",
        point_of_sale: 1,
      },
    );
  });

  it("lists fiscal receipts with an optional status query", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { items: [{ id: 9, status: "failed_retryable" }] },
    });

    const result = await listReceipts(42, "failed_retryable");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts",
      { params: { status: "failed_retryable" }, _skipErrorToast: true },
    );
    expect(result).toEqual([{ id: 9, status: "failed_retryable" }]);
  });

  it("lists fiscal receipts without params when status is omitted", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { items: [] },
    });

    await fiscalApi.listReceipts(42);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts",
      { _skipErrorToast: true },
    );
  });

  it("issues a fiscal receipt for a bill", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { ok: true, business_id: 42 },
    });

    await issueReceipt(42, 24);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts/issue",
      { bill_id: 24 },
    );
  });

  it("retries a fiscal receipt", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { ok: true, business_id: 42 },
    });

    await retryReceipt(42, 11);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts/11/retry",
      {},
    );
  });

  it("re-sends a fiscal receipt", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { ok: true, business_id: 42 },
    });

    await resendFiscalReceipt(42, 11);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts/11/resend",
      {},
    );
  });

  it("uploads fiscal credentials as multipart certificate + private_key files", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        fingerprint: "AB:CD:EF",
        expires_at: "2027-01-01T00:00:00Z",
        setup_status: "credentials_set",
      },
    });

    const cert = new File(["cert-pem"], "cert.crt", {
      type: "application/x-x509-ca-cert",
    });
    const key = new File(["key-pem"], "private.key", {
      type: "application/x-pem-file",
    });

    const result = await uploadFiscalCredentials(42, cert, key);

    expect(axiosInstance.post).toHaveBeenCalledTimes(1);
    const [url, body, config] = (axiosInstance.post as jest.Mock).mock.calls[0];
    expect(url).toBe("/inside/businesses/42/fiscal/credentials");
    expect(body).toBeInstanceOf(FormData);
    expect((body as FormData).get("certificate")).toBe(cert);
    expect((body as FormData).get("private_key")).toBe(key);
    expect(config).toEqual({
      headers: { "Content-Type": "multipart/form-data" },
    });
    expect(result).toEqual({
      fingerprint: "AB:CD:EF",
      expires_at: "2027-01-01T00:00:00Z",
      setup_status: "credentials_set",
    });
  });

  it("validates fiscal settings and unwraps the settings payload", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { settings: { id: 7, setup_status: "ready" } },
    });

    const result = await validateFiscalSettings(42);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/validate",
      {},
    );
    expect(result).toEqual({ id: 7, setup_status: "ready" });
  });

  it("credits a fiscal receipt with a required reason", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { ok: true, business_id: 42 },
    });

    await creditFiscalReceipt(42, 11, "Cliente devolvió el plato");

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts/11/credit",
      { reason: "Cliente devolvió el plato" },
    );
  });

  it("credits a fiscal receipt with a partial amount", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { ok: true, business_id: 42 },
    });

    await fiscalApi.creditFiscalReceipt(42, 12, "partial refund", 500);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts/12/credit",
      { reason: "partial refund", amount_cents: 500 },
    );
  });

  it("propagates axios errors", async () => {
    const err = new Error("server exploded");
    (axiosInstance.get as jest.Mock).mockRejectedValue(err);

    await expect(getSettings(42)).rejects.toThrow("server exploded");
  });

  it("listReceiptsPage always sends page and unwraps top-level ReceiptsPage", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        receipts: [
          {
            id: 11,
            business_id: 42,
            settings_id: 1,
            bill_id: 99,
            payment_id: null,
            alternative_payment_id: null,
            country: "AR",
            provider: "arca",
            action: "invoice",
            receipt_type: "factura_c",
            receipt_number: "0001-00000042",
            provider_receipt_id: null,
            auth_code: null,
            auth_expires_at: null,
            qr_payload: null,
            qr_image_path: null,
            pdf_path: null,
            customer_doc_type: null,
            customer_doc_number: null,
            total_amount_cents: 12550,
            tip_amount_cents: 250,
            currency: "ARS",
            status: "authorized",
            error_code: null,
            error_message: null,
            issued_at: "2026-06-22T12:00:00Z",
            created_at: "2026-06-22T12:00:00Z",
            updated_at: "2026-06-22T12:00:00Z",
            delivery: [
              { task_id: 7, channel: "email", status: "succeeded" },
              { task_id: 8, channel: "print", status: "pending" },
            ],
            needs_attention: false,
          },
        ],
        total: 1,
        page: 1,
        page_size: 20,
        total_pages: 1,
      },
    });

    const page = await fiscalApi.listReceiptsPage(42, {
      start: "2026-06-21",
      end: "2026-07-20",
      status: "authorized",
      needs_attention: true,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts",
      {
        params: {
          start: "2026-06-21",
          end: "2026-07-20",
          status: "authorized",
          needs_attention: "true",
          page: 1,
        },
        _skipErrorToast: true,
      },
    );
    expect(page.receipts).toHaveLength(1);
    expect(page.receipts[0].delivery).toEqual([
      { task_id: 7, channel: "email", status: "succeeded" },
      { task_id: 8, channel: "print", status: "pending" },
    ]);
    expect(page.receipts[0].needs_attention).toBe(false);
    expect(page.total).toBe(1);
    expect(page.page).toBe(1);
    expect(page.page_size).toBe(20);
    expect(page.total_pages).toBe(1);
  });

  it("listReceiptsPage forwards explicit page and page_size", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        receipts: [],
        total: 0,
        page: 2,
        page_size: 10,
        total_pages: 0,
      },
    });

    await fiscalApi.listReceiptsPage(42, {
      page: 2,
      page_size: 10,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts",
      {
        params: {
          page: 2,
          page_size: 10,
        },
        _skipErrorToast: true,
      },
    );
  });

  it("listIssuableBills requests q and unwraps bills (L6-21)", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        bills: [
          {
            bill_id: 99,
            bill_number: "ISSUABLE-NEW",
            table_label: "Patio 3",
            closed_at: "2026-06-22T18:00:00Z",
            total_amount: 125.5,
            currency: "ARS",
          },
        ],
      },
    });

    const items = await fiscalApi.listIssuableBills(42, "Patio");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/issuable-bills",
      { params: { q: "Patio" } },
    );
    expect(items).toEqual([
      {
        bill_id: 99,
        bill_number: "ISSUABLE-NEW",
        table_label: "Patio 3",
        closed_at: "2026-06-22T18:00:00Z",
        total_amount: 125.5,
        currency: "ARS",
      },
    ]);
  });

  it("listIssuableBills ignores the retired items-only envelope", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { items: [{ bill_id: 7 }] },
    });
    await expect(fiscalApi.listIssuableBills(42)).resolves.toEqual([]);
  });

  it("listIssuableBills omits empty q", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { bills: [] } });

    await fiscalApi.listIssuableBills(42);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/issuable-bills",
      { params: {} },
    );
  });

  // `receiptsExportUrl` and its server route are deleted (decision #16). Fiscal
  // receipts export is the client `exportLocalizedCsv` path in
  // InvoicesTab.handleExportCsv, which honors the receipt_type/needs_attention
  // filters the server export ignored and walks the full paginated range.
  // Pinned server-side by TestFiscalReceiptsServerExportDeletedSourceGate.
  it("does not expose a server-side receipts CSV export builder", () => {
    expect(
      (fiscalApi as Record<string, unknown>).receiptsExportUrl,
    ).toBeUndefined();
  });
});
