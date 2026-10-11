/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import * as fiscalApi from "@/api/fiscal";

import FiscalDashboard, { isValidEmitterTaxId } from "./FiscalDashboard";

const I18N_STUB: Record<string, string> = {
  "fiscal.serviceUnavailable":
    "Electronic invoicing is switched off on the server right now.",
  "fiscal.dashboard.errorTitle": "Could not load invoice data.",
  "fiscal.dashboard.unexpectedError": "Unexpected invoice API error",
  "fiscal.dashboard.tryAgain": "Try again",
  "fiscal.status.titleSetup": "Invoice setup",
  "fiscal.summary.lastIssued": "Last invoice",
  "fiscal.summary.refresh": "Refresh",
  "fiscal.summary.editSetup": "Edit setup",
  "fiscal.summary.sandboxBannerPrefix": "Sandbox",
  "fiscal.summary.sandboxBanner":
    "Invoices issued here are simulated — not legally valid tax invoices.",
  "fiscal.summary.demoBannerPrefix": "Demo",
  "fiscal.summary.demoBanner":
    "These invoices are practice documents only — not legally valid tax invoices.",
  "fiscal.setup.title": "Invoice setup",
  "fiscal.setup.description":
    "Update country, tax profile, or issuance mode. Changes apply to new paid bills.",
  "fiscal.setup.emptyDescription":
    "Set this up once so Payverge can queue official invoices for paid bills.",
  "fiscal.setup.compareModes": "Compare modes",
  "fiscal.setup.environmentDemoLocked":
    "Demo tenants stay in Sandbox. Production is not available for the demo provider.",
  "fiscal.setup.fields.country": "Country",
  "fiscal.setup.fields.provider": "Provider",
  "fiscal.setup.fields.mode": "Invoice mode",
  "fiscal.setup.fields.environment": "Environment",
  "fiscal.setup.fields.taxId": "Tax ID",
  "fiscal.setup.fields.taxCondition": "Tax condition",
  "fiscal.setup.fields.pointOfSale": "Point of sale",
  "fiscal.setup.fields.pointOfSaleHelp":
    "Whole number assigned by your provider for this location.",
  "fiscal.setup.actions.save": "Save invoice setup",
  "fiscal.setup.actions.saving": "Saving invoice setup",
  "fiscal.setup.actions.cancel": "Cancel",
  "fiscal.setup.saveSuccess": "Invoice setup saved",
  "fiscal.setup.saveError": "Could not save invoice setup.",
  "fiscal.setup.invalidPointOfSale": "Point of sale must be a whole number.",
  "fiscal.modes.off.title": "Off",
  "fiscal.modes.manual.title": "Manual",
  "fiscal.modes.automatic_non_blocking.title": "Automatic",
  "fiscal.modes.off.description":
    "Payverge will not issue invoices and the manual button is disabled.",
  "fiscal.modes.manual.description":
    "Issue invoices yourself from the history when you're ready.",
  "fiscal.modes.automatic_non_blocking.description":
    "Paid bills queue invoices in the background. Bills close even if the provider is slow.",
  "fiscal.history.title": "Invoice history",
  "fiscal.history.description":
    "Invoices queued for paid bills — including pending, authorized, and failed.",
  "fiscal.history.emptyTitle": "No invoices yet",
  "fiscal.history.emptyDescription":
    "Paid bills will appear here as soon as Payverge issues an invoice for them.",
  "fiscal.history.noMatch": "No invoices match this filter.",
  "fiscal.history.unnumbered": "Awaiting number",
  "fiscal.history.filters.all": "All",
  "fiscal.history.filters.authorized": "Authorized",
  "fiscal.history.filters.pending": "Pending",
  "fiscal.history.filters.attention": "Needs attention",
  "fiscal.history.issue.billId": "Bill ID to issue",
  "fiscal.history.issue.placeholder": "e.g. 1234",
  "fiscal.history.issue.button": "Issue invoice",
  "fiscal.history.issue.issuing": "Issuing",
  "fiscal.history.issue.success": "Invoice issue queued",
  "fiscal.history.issue.error": "Could not issue invoice.",
  "fiscal.history.issue.alreadyIssued":
    "This bill already has an invoice. Open it from the Invoices list instead of issuing a second one.",
  "fiscal.history.retry.button": "Retry",
  "fiscal.history.retry.success": "Invoice retry queued",
  "fiscal.history.retry.error": "Could not retry invoice.",
  "fiscal.history.resend.button": "Re-send",
  "fiscal.history.resend.resending": "Re-sending",
  "fiscal.history.resend.success": "Invoice re-send queued",
  "fiscal.history.resend.error": "Could not re-send invoice.",
  "fiscal.history.creditNote.button": "Credit note",
  "fiscal.history.creditNote.crediting": "Issuing",
  "fiscal.history.creditNote.success": "Credit note queued",
  "fiscal.history.creditNote.error": "Could not issue the credit note.",
  "fiscal.history.creditNote.confirmTitle": "Issue a credit note?",
  "fiscal.history.creditNote.confirmDescription":
    "This queues an official credit note that cancels invoice #{{bill}}.",
  "fiscal.history.creditNote.reasonLabel": "Reason (optional)",
  "fiscal.history.creditNote.reasonPlaceholder": "e.g. Order cancelled",
  "fiscal.history.creditNote.confirm": "Issue credit note",
  "fiscal.history.creditNote.cancel": "Cancel",
  "fiscal.history.cae": "CAE",
  "fiscal.credentials.title": "AFIP credentials",
  "fiscal.credentials.description": "Upload the certificate and private key.",
  "fiscal.credentials.certLabel": "Certificate (.crt or .pem)",
  "fiscal.credentials.keyLabel": "Private key (.key)",
  "fiscal.credentials.upload": "Upload credentials",
  "fiscal.credentials.uploading": "Uploading credentials",
  "fiscal.credentials.uploadSuccess": "Credentials uploaded.",
  "fiscal.credentials.uploadError": "Could not upload credentials.",
  "fiscal.credentials.missingFiles":
    "Select both the certificate and the private key before uploading.",
  "fiscal.credentials.validate": "Validate",
  "fiscal.credentials.validating": "Validating with AFIP",
  "fiscal.credentials.validateSuccess": "Credentials validated.",
  "fiscal.credentials.validateError": "Could not run the validation.",
  "fiscal.credentials.validationFailed": "AFIP rejected the credentials:",
  "fiscal.credentials.fingerprint": "Fingerprint",
  "fiscal.credentials.expiresAt": "Expires",
  "fiscal.credentials.keyNeverStored":
    "For your security, the private key is never displayed after upload.",
  "fiscal.credentials.status.label": "Setup status",
  "fiscal.credentials.status.draft": "Credentials needed",
  "fiscal.credentials.status.credentials_set": "Pending validation",
  "fiscal.credentials.status.ready": "Ready",
  "fiscal.credentials.status.validated": "Validated",
  "fiscal.region.title": "E-invoicing",
  "fiscal.region.unavailableTitle":
    "E-invoicing isn't available in your region yet",
  "fiscal.region.unavailableDescription":
    "Payverge currently issues official electronic invoices for Argentina (AFIP/ARCA). E-invoicing is not available in {country} yet.",
  "fiscal.region.demoTitle": "Demo fiscal environment",
  "fiscal.region.demoDescription":
    "This business issues simulated DEMO invoices only.",
  "fiscal.region.demoPathToLive":
    "Path to live: change country to Argentina and complete AFIP credentials.",
  "fiscal.region.unavailablePathToLive":
    "Path to live: when your country is supported, upload credentials.",
  "fiscal.history.columns.billId": "Bill",
  "fiscal.history.columns.receipt": "Invoice",
  "fiscal.history.columns.amount": "Amount",
  "fiscal.history.columns.status": "Status",
  "fiscal.history.columns.error": "Error",
  "fiscal.history.columns.actions": "Actions",
  "fiscal.common.notSet": "Not set",
  "fiscal.common.none": "—",
  "fiscal.labels.AR": "Argentina",
  "fiscal.labels.AE": "United Arab Emirates",
  "fiscal.labels.arca": "ARCA",
  "fiscal.labels.edicom": "EDICOM",
  "fiscal.labels.off": "Off",
  "fiscal.labels.manual": "Manual",
  "fiscal.labels.automatic_non_blocking": "Automatic",
  "fiscal.labels.draft": "Draft",
  "fiscal.labels.failed_retryable": "Failed retryable",
  "fiscal.labels.authorized": "Authorized",
  "fiscal.labels.pending": "Pending",
  "fiscal.labels.rejected": "Rejected",
  "fiscal.labels.invoice_a": "Invoice A",
  "fiscal.labels.receipt": "Receipt",
  "fiscal.labels.invoice": "Invoice",
  "fiscal.labels.factura_b": "Factura B",
  "fiscal.labels.sandbox": "Sandbox",
  "fiscal.labels.production": "Production",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string, _locale?: string, params?: Record<string, string | number>) => {
    let value = I18N_STUB[key] ?? key;
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        value = value.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
      }
    }
    return value;
  },
}));

jest.mock("@/api/fiscal");

/**
 * The shared axiosInstance interceptor rejects with a plain Error that copies
 * `.status` and `.response.{status,data}` across but carries no isAxiosError
 * flag (see utils/apiError). Build that exact shape so the dashboard's error
 * handling is exercised the way it runs in the app.
 */
function sanitizedApiError(status: number, data: unknown) {
  const err = new Error(`Request failed with status code ${status}`) as Error & {
    status?: number;
    response?: { status: number; data: unknown };
  };
  err.status = status;
  err.response = { status, data };
  return err;
}

function settings(overrides: Partial<fiscalApi.FiscalSettings> = {}) {
  return {
    id: 1,
    business_id: 42,
    country: "AR",
    provider: "arca",
    mode: "automatic_non_blocking" as const,
    environment: "sandbox" as const,
    tax_id: "20123456789",
    tax_condition: "responsable_inscripto",
    point_of_sale: 1,
    setup_status: "draft",
    ...overrides,
  };
}

function receipt(overrides: Partial<fiscalApi.FiscalReceipt> = {}) {
  return {
    id: 9,
    business_id: 42,
    settings_id: 1,
    bill_id: 123,
    payment_id: null,
    alternative_payment_id: null,
    country: "AR",
    provider: "arca",
    action: "issue",
    receipt_type: "invoice_a",
    receipt_number: null,
    provider_receipt_id: null,
    auth_code: null,
    auth_expires_at: null,
    qr_payload: null,
    qr_image_path: null,
    pdf_path: null,
    customer_doc_type: null,
    customer_doc_number: null,
    total_amount_cents: 125050,
    tip_amount_cents: 0,
    currency: "ARS",
    status: "pending" as const,
    error_code: null,
    error_message: null,
    issued_at: null,
    created_at: "2026-05-20T12:00:00Z",
    updated_at: "2026-05-20T12:00:00Z",
    ...overrides,
  };
}

describe("FiscalDashboard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (fiscalApi.listReceiptDelivery as jest.Mock).mockResolvedValue([]);
  });

  // F-RBAC (FE): credit notes (tax reversal) and AFIP credential upload are
  // owner-only on the backend. The dashboard hides those controls for non-owners
  // (canManageSensitive=false) so managers do not see actions that would 403.
  it("hides credit-note and credential controls when canManageSensitive is false", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({ status: "authorized" }),
    ]);

    render(<FiscalDashboard businessId={42} canManageSensitive={false} />);

    await screen.findByText("Invoice history");
    expect(screen.queryByText("Credit note")).not.toBeInTheDocument();
    expect(screen.queryByText("AFIP credentials")).not.toBeInTheDocument();
  });

  it("shows credit-note and credential controls for owners (default)", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({ status: "authorized" }),
    ]);

    render(<FiscalDashboard businessId={42} />);

    expect(await screen.findByText("Credit note")).toBeInTheDocument();
    expect(screen.getByText("AFIP credentials")).toBeInTheDocument();
  });

  // F7: a "validated" setup_status must read as a green success badge, not the
  // amber warning fallback. The seed sets setup_status="validated".
  it("shows a success (not warning) badge for a validated setup status", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({
        setup_status: "validated",
        credentials_fingerprint: "AB:CD:EF",
      }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    const badge = await screen.findByText("Validated");
    // StatusBadge lives on the span carrying the tone classes.
    expect(badge).toHaveClass("text-emerald-700");
    expect(badge).not.toHaveClass("text-amber-800");
  });

  // F6: a US/demo business has no AFIP/ARCA credential flow — the Argentina
  // certificate + private-key upload form must not render; a neutral region
  // card is shown instead.
  it("hides the AFIP credential form for a demo-provider business", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({ country: "US", provider: "demo", setup_status: "validated" }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    // Neutral demo state, not the AFIP certificate uploader.
    expect(
      await screen.findByText("Demo fiscal environment"),
    ).toBeInTheDocument();
    expect(screen.queryByText("AFIP credentials")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Certificate (.crt or .pem)"),
    ).not.toBeInTheDocument();
  });

  it("explains demo sandbox honestly and does not promise Production from Edit setup", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({
        country: "US",
        provider: "demo",
        environment: "sandbox",
        setup_status: "validated",
      }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    const banner = await screen.findByTestId("fiscal-sandbox-banner");
    expect(banner).toHaveAttribute("data-demo-provider", "true");
    expect(banner).toHaveTextContent(/practice documents only/i);
    expect(banner).not.toHaveTextContent(/Switch to Production/i);
    expect(
      screen.getByText(/Path to live: change country to Argentina/i),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /edit setup/i }));
    expect(await screen.findByTestId("fiscal-env-demo-locked")).toBeInTheDocument();
    const env = screen.getByTestId(
      "fiscal-environment-select",
    ) as HTMLSelectElement;
    expect(env).toBeDisabled();
    expect(within(env).queryByText("Production")).not.toBeInTheDocument();
    expect(screen.queryByText("Tax ID")).not.toBeInTheDocument();
    expect(screen.queryByText("Tax condition")).not.toBeInTheDocument();
    expect(screen.queryByText("Point of sale")).not.toBeInTheDocument();
  });

  it("remaps leftover AFIP letter types on a US venue history row", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({ country: "US", provider: "demo", setup_status: "validated" }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({
        receipt_type: "factura_b",
        country: "US",
        status: "authorized",
        receipt_number: "0001",
      }),
    ]);

    render(<FiscalDashboard businessId={42} />);
    expect(await screen.findByText("Receipt")).toBeInTheDocument();
    expect(screen.queryByText("Factura B")).not.toBeInTheDocument();
    expect(screen.queryByText("Factura")).not.toBeInTheDocument();
    expect(screen.queryByText("Recibo")).not.toBeInTheDocument();
  });

  it("labels stored US invoice/receipt types with US names, not AFIP Factura/Recibo", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({ country: "US", provider: "demo", setup_status: "validated" }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({
        receipt_type: "invoice",
        country: "US",
        status: "authorized",
        receipt_number: "0002",
      }),
    ]);

    render(<FiscalDashboard businessId={42} />);
    const table = await screen.findByRole("table", { name: "Invoice history" });
    const typeCell = within(table).getAllByText("Invoice").find(
      (el) => el.tagName.toLowerCase() === "span",
    );
    expect(typeCell).toBeTruthy();
    expect(within(table).queryByText("Factura")).not.toBeInTheDocument();
    expect(within(table).queryByText("Factura A")).not.toBeInTheDocument();
    expect(within(table).queryByText("Recibo")).not.toBeInTheDocument();
  });

  it("shows the unavailable-region card for an unsupported non-demo country", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({ country: "US", provider: "", setup_status: "draft" }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    expect(
      await screen.findByText("E-invoicing isn't available in your region yet"),
    ).toBeInTheDocument();
    expect(screen.queryByText("AFIP credentials")).not.toBeInTheDocument();
  });

  it("offers exactly the three implemented invoice modes", async () => {
    // Default fixture mode is "automatic_non_blocking".
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    await user.click(
      await screen.findByRole("button", { name: "Edit setup" }),
    );

    // The mode picker offers off / manual / automatic and nothing else.
    const modeSelect = await screen.findByRole("combobox", {
      name: "Invoice mode",
    });
    const optionValues = within(modeSelect)
      .getAllByRole("option")
      .map((o) => (o as HTMLOptionElement).value);
    expect(optionValues).toEqual(["off", "manual", "automatic_non_blocking"]);
  });

  it("renders only the setup form when no fiscal settings exist", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValueOnce(null);
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValueOnce([]);

    render(<FiscalDashboard businessId={42} />);

    expect(
      await screen.findByRole("heading", { name: "Invoice setup" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Country")).toBeInTheDocument();
    expect(screen.getByLabelText("Invoice mode")).toBeInTheDocument();
    expect(screen.getByLabelText("Tax ID")).toBeInTheDocument();
    // No history card, no summary bar, no edit-setup button before save
    expect(screen.queryByText("Invoice history")).not.toBeInTheDocument();
    expect(screen.queryByText("Edit setup")).not.toBeInTheDocument();
  });

  it("saves invoice setup, hides the form, and shows the summary bar", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock)
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(settings({ country: "AR", provider: "arca" }));
    (fiscalApi.listReceipts as jest.Mock)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([]);
    (fiscalApi.updateSettings as jest.Mock).mockResolvedValueOnce(
      settings({ country: "AR", provider: "arca" }),
    );

    render(<FiscalDashboard businessId={42} />);

    await user.selectOptions(
      await screen.findByLabelText("Invoice mode"),
      "manual",
    );
    await user.clear(screen.getByLabelText("Tax ID"));
    await user.type(screen.getByLabelText("Tax ID"), "30123456789");
    await user.selectOptions(
      screen.getByLabelText("Tax condition"),
      "responsable_inscripto",
    );
    await user.clear(screen.getByLabelText("Point of sale"));
    await user.type(screen.getByLabelText("Point of sale"), "9");
    await user.click(
      screen.getByRole("button", { name: "Save invoice setup" }),
    );

    await waitFor(() => {
      expect(fiscalApi.updateSettings).toHaveBeenCalledWith(42, {
        country: "AR",
        provider: "arca",
        mode: "manual",
        environment: "sandbox",
        tax_id: "30123456789",
        tax_condition: "responsable_inscripto",
        point_of_sale: 9,
      });
    });
    // After save: form collapses, summary bar + history appear
    expect(
      await screen.findByRole("button", { name: "Edit setup" }),
    ).toBeInTheDocument();
    expect(screen.getByText("No invoices yet")).toBeInTheDocument();
  });

  it("disables the UAE option in the country picker", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValueOnce(null);
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValueOnce([]);

    render(<FiscalDashboard businessId={42} />);

    const select = (await screen.findByLabelText(
      "Country",
    )) as HTMLSelectElement;
    const uaeOption = Array.from(select.options).find((o) => o.value === "AE");
    expect(uaeOption).toBeDefined();
    expect(uaeOption?.disabled).toBe(true);
  });

  // L6-22 (edit-path residue): a business whose saved country sits outside
  // COUNTRY_OPTIONS — the "US"/demo business — had that country coerced to "AR"
  // the moment form state was built. Opening "Edit setup" and pressing save,
  // without ever touching the country picker, then rewrote the business to
  // Argentina/ARCA. Only an explicit operator change may move the country.
  it("round-trips an unsupported saved country through edit and save", async () => {
    const user = userEvent.setup();
    const saved = settings({
      country: "US",
      provider: "demo",
      tax_condition: "",
      mode: "manual" as const,
    });
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(saved);
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.updateSettings as jest.Mock).mockResolvedValue(saved);

    render(<FiscalDashboard businessId={42} />);

    await user.click(await screen.findByRole("button", { name: "Edit setup" }));

    // The picker shows the business's real country, not a silent "Argentina".
    const countrySelect = (await screen.findByLabelText(
      "Country",
    )) as HTMLSelectElement;
    expect(countrySelect.value).toBe("US");

    await user.click(screen.getByRole("button", { name: "Save invoice setup" }));

    await waitFor(() => {
      expect(fiscalApi.updateSettings).toHaveBeenCalledWith(
        42,
        expect.objectContaining({ country: "US", provider: "demo" }),
      );
    });
  });

  it("still moves the country when the operator explicitly picks one", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({
        country: "US",
        provider: "demo",
        tax_condition: "",
        mode: "manual" as const,
      }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.updateSettings as jest.Mock).mockResolvedValue(settings());

    render(<FiscalDashboard businessId={42} />);

    await user.click(await screen.findByRole("button", { name: "Edit setup" }));
    await user.selectOptions(await screen.findByLabelText("Country"), "AR");
    await user.click(screen.getByRole("button", { name: "Save invoice setup" }));

    await waitFor(() => {
      expect(fiscalApi.updateSettings).toHaveBeenCalledWith(
        42,
        expect.objectContaining({ country: "AR", provider: "arca" }),
      );
    });
  });

  it("does not save invalid point of sale values", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    await user.click(
      await screen.findByRole("button", { name: "Edit setup" }),
    );
    const pointOfSaleInput = await screen.findByLabelText("Point of sale");
    await user.clear(pointOfSaleInput);
    await user.type(pointOfSaleInput, "10e1");

    expect(pointOfSaleInput).toHaveAttribute("aria-invalid", "true");
    expect(
      screen.getByRole("button", { name: "Save invoice setup" }),
    ).toBeDisabled();
    expect(fiscalApi.updateSettings).not.toHaveBeenCalled();
  });

  it("issues an invoice for a paid bill and refreshes history", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([receipt({ bill_id: 321 })]);
    (fiscalApi.issueReceipt as jest.Mock).mockResolvedValueOnce({
      ok: true,
      business_id: 42,
    });

    render(<FiscalDashboard businessId={42} />);

    await user.type(await screen.findByLabelText("Bill ID to issue"), "321");
    await user.click(screen.getByRole("button", { name: "Issue invoice" }));

    await waitFor(() => {
      expect(fiscalApi.issueReceipt).toHaveBeenCalledWith(42, 321);
    });
    expect(await screen.findByText("#321")).toBeInTheDocument();
    expect(screen.getByText("Invoice issue queued")).toBeInTheDocument();
  });

  // #906: while the server-side fiscal containment control is off, every fiscal
  // write answers 503 and the backend's global 5xx sanitizer has already
  // replaced the body with one constant English envelope. The operator console
  // used to print that English sentence verbatim — in every locale — because the
  // safe-detail helper accepts any short backend string. Answer the status with
  // our own localized copy instead.
  it("shows localized copy, not the sanitized English 5xx body, when issuing is switched off", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.issueReceipt as jest.Mock).mockRejectedValueOnce(
      sanitizedApiError(503, {
        error: "Service temporarily unavailable",
        code: "SERVICE_UNAVAILABLE",
      }),
    );

    render(<FiscalDashboard businessId={42} />);

    await user.type(await screen.findByLabelText("Bill ID to issue"), "321");
    await user.click(screen.getByRole("button", { name: "Issue invoice" }));

    expect(
      await screen.findByText(
        "Electronic invoicing is switched off on the server right now.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Service temporarily unavailable"),
    ).not.toBeInTheDocument();
  });

  // The credential/validate slot renders through a different state setter, and
  // it 503s from the same control — it must get the same localized answer.
  it("shows localized copy when credential validation is switched off", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({
        setup_status: "credentials_set",
        credentials_fingerprint: "AB:CD:EF",
      }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.validateFiscalSettings as jest.Mock).mockRejectedValueOnce(
      sanitizedApiError(503, {
        error: "Service temporarily unavailable",
        code: "SERVICE_UNAVAILABLE",
      }),
    );

    render(<FiscalDashboard businessId={42} />);

    await user.click(await screen.findByRole("button", { name: "Validate" }));

    expect(
      await screen.findByText(
        "Electronic invoicing is switched off on the server right now.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Service temporarily unavailable"),
    ).not.toBeInTheDocument();
  });

  // The 503 branch must stay narrow: a real, specific backend rejection is
  // better copy than any generic string we could substitute for it.
  it("still surfaces a specific backend rejection on a non-503 issue failure", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.issueReceipt as jest.Mock).mockRejectedValueOnce(
      sanitizedApiError(400, {
        error: "bill_id is required",
        code: "VALIDATION_FIELD_INVALID",
        params: { field: "bill_id" },
      }),
    );

    render(<FiscalDashboard businessId={42} />);

    await user.type(await screen.findByLabelText("Bill ID to issue"), "321");
    await user.click(screen.getByRole("button", { name: "Issue invoice" }));

    expect(await screen.findByText("bill_id is required")).toBeInTheDocument();
    expect(
      screen.queryByText(
        "Electronic invoicing is switched off on the server right now.",
      ),
    ).not.toBeInTheDocument();
  });

  // #907: the "Bill ID to issue" field bypasses the invoice picker, which is the
  // only place already-invoiced bills are marked. The backend used to swallow
  // that duplicate on its per-bill idempotency key and answer 202 {"ok":true},
  // so the console printed "Invoice issue queued" for a factura that was never
  // queued. It now answers 409 + fiscal_receipt_already_issued, and the console
  // must say so in the operator's language instead of echoing the English body.
  it("says the bill is already invoiced when issuing one that has a live receipt", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.issueReceipt as jest.Mock).mockRejectedValueOnce(
      sanitizedApiError(409, {
        error: "This bill already has an invoice.",
        code: "fiscal_receipt_already_issued",
      }),
    );

    render(<FiscalDashboard businessId={42} />);

    await user.type(await screen.findByLabelText("Bill ID to issue"), "1704");
    await user.click(screen.getByRole("button", { name: "Issue invoice" }));

    expect(
      await screen.findByText(
        "This bill already has an invoice. Open it from the Invoices list instead of issuing a second one.",
      ),
    ).toBeInTheDocument();
    // The backend body is the untranslated English sentence — it must not be
    // what the operator reads.
    expect(
      screen.queryByText("This bill already has an invoice."),
    ).not.toBeInTheDocument();
    // Never the success banner, and never the generic failure copy.
    expect(screen.queryByText("Invoice issue queued")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Could not issue invoice."),
    ).not.toBeInTheDocument();
  });

  // The 404 "bill not found" branch used to be the first thing checked. A 409
  // carries no "not found" text, but this pins the ordering so a future edit
  // cannot route an already-invoiced bill into the wrong explanation.
  it("does not mistake an already-invoiced bill for a missing one", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.issueReceipt as jest.Mock).mockRejectedValueOnce(
      sanitizedApiError(409, {
        error: "This bill already has an invoice.",
        code: "fiscal_receipt_already_issued",
      }),
    );

    render(<FiscalDashboard businessId={42} />);

    await user.type(await screen.findByLabelText("Bill ID to issue"), "1704");
    await user.click(screen.getByRole("button", { name: "Issue invoice" }));

    await screen.findByText(
      "This bill already has an invoice. Open it from the Invoices list instead of issuing a second one.",
    );
    expect(
      screen.queryByText(
        "We couldn’t find a paid bill with that ID. Open the Bills tab to confirm the number.",
      ),
    ).not.toBeInTheDocument();
  });

  it("hides the manual issue form when mode is off", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({ mode: "off" }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    expect(await screen.findByText("No invoices yet")).toBeInTheDocument();
    expect(screen.queryByLabelText("Bill ID to issue")).not.toBeInTheDocument();
  });

  it("Edit setup toggles the form open and Cancel collapses it back", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    expect(
      await screen.findByRole("button", { name: "Edit setup" }),
    ).toBeInTheDocument();
    // form is collapsed
    expect(screen.queryByLabelText("Tax ID")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Edit setup" }));
    expect(screen.getByLabelText("Tax ID")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByLabelText("Tax ID")).not.toBeInTheDocument();
  });

  it("filters the history table by status", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({
        id: 1,
        bill_id: 100,
        status: "authorized",
        receipt_number: "A-0001-1",
      }),
      receipt({
        id: 2,
        bill_id: 200,
        status: "failed_retryable",
        error_message: "Provider timed out",
      }),
    ]);

    render(<FiscalDashboard businessId={42} />);

    expect(await screen.findByText("#100")).toBeInTheDocument();
    expect(screen.getByText("#200")).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: /^Needs attention 1$/ }),
    );

    expect(screen.queryByText("#100")).not.toBeInTheDocument();
    expect(screen.getByText("#200")).toBeInTheDocument();
  });

  it("retries only retryable invoices", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({
        id: 9,
        status: "failed_retryable",
        error_message: "Provider timed out",
      }),
      receipt({
        id: 10,
        bill_id: 124,
        receipt_number: "A-0001-00000044",
        status: "authorized",
      }),
    ]);
    (fiscalApi.retryReceipt as jest.Mock).mockResolvedValueOnce({
      ok: true,
      business_id: 42,
    });

    render(<FiscalDashboard businessId={42} />);

    const retryButtons = await screen.findAllByRole("button", {
      name: "Retry",
    });
    expect(retryButtons).toHaveLength(1);
    await user.click(retryButtons[0]);

    await waitFor(() => {
      expect(fiscalApi.retryReceipt).toHaveBeenCalledWith(42, 9);
    });
    expect(screen.getByText("Invoice retry queued")).toBeInTheDocument();
  });

  it("re-sends only authorized invoices", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({
        id: 9,
        status: "failed_retryable",
        error_message: "Provider timed out",
      }),
      receipt({
        id: 10,
        bill_id: 124,
        receipt_number: "A-0001-00000044",
        status: "authorized",
      }),
    ]);
    (fiscalApi.resendFiscalReceipt as jest.Mock).mockResolvedValueOnce({
      ok: true,
      business_id: 42,
    });

    render(<FiscalDashboard businessId={42} />);

    const resendButtons = await screen.findAllByRole("button", {
      name: "Re-send",
    });
    expect(resendButtons).toHaveLength(1); // only the authorized receipt
    await user.click(resendButtons[0]);

    await waitFor(() => {
      expect(fiscalApi.resendFiscalReceipt).toHaveBeenCalledWith(42, 10);
    });
    expect(screen.getByText("Invoice re-send queued")).toBeInTheDocument();
  });

  it("uploads credentials, then enables and runs validation", async () => {
    const user = userEvent.setup();
    // Two getSettings calls happen: initial load, then the post-upload reload.
    // Validation updates state directly from validateFiscalSettings (no reload),
    // so no third getSettings is queued — leaving an unconsumed mockResolvedValueOnce
    // would bleed into the next test (clearAllMocks does not drain the once-queue).
    (fiscalApi.getSettings as jest.Mock)
      .mockResolvedValueOnce(settings({ setup_status: "draft" }))
      .mockResolvedValueOnce(
        settings({
          setup_status: "credentials_set",
          credentials_fingerprint: "AB:CD:EF",
          credentials_expires_at: "2027-01-01T00:00:00Z",
        }),
      );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.uploadFiscalCredentials as jest.Mock).mockResolvedValueOnce({
      fingerprint: "AB:CD:EF",
      expires_at: "2027-01-01T00:00:00Z",
      setup_status: "credentials_set",
    });
    (fiscalApi.validateFiscalSettings as jest.Mock).mockResolvedValueOnce(
      settings({ setup_status: "ready" }),
    );

    render(<FiscalDashboard businessId={42} />);

    // Validate is disabled until credentials are uploaded.
    const validateBtn = await screen.findByRole("button", { name: "Validate" });
    expect(validateBtn).toBeDisabled();

    const cert = new File(["cert"], "cert.crt", { type: "application/x-pem-file" });
    const key = new File(["key"], "private.key", { type: "application/x-pem-file" });
    await user.upload(screen.getByLabelText("Certificate (.crt or .pem)"), cert);
    await user.upload(screen.getByLabelText("Private key (.key)"), key);
    await user.click(screen.getByRole("button", { name: "Upload credentials" }));

    await waitFor(() => {
      expect(fiscalApi.uploadFiscalCredentials).toHaveBeenCalledWith(
        42,
        cert,
        key,
      );
    });
    // Fingerprint surfaces after the reload, Validate becomes enabled.
    expect(await screen.findByText(/AB:CD:EF/)).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Validate" })).toBeEnabled(),
    );

    await user.click(screen.getByRole("button", { name: "Validate" }));
    await waitFor(() => {
      expect(fiscalApi.validateFiscalSettings).toHaveBeenCalledWith(42);
    });
    expect(
      await screen.findByText("Credentials validated."),
    ).toBeInTheDocument();
  });

  it("shows the AFIP validation error inline when validation fails", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({
        setup_status: "credentials_set",
        credentials_fingerprint: "AB:CD:EF",
      }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.validateFiscalSettings as jest.Mock).mockResolvedValueOnce(
      settings({
        setup_status: "credentials_set",
        credentials_fingerprint: "AB:CD:EF",
        last_validation_error: "WSAA login rejected: cert not authorized",
      }),
    );

    render(<FiscalDashboard businessId={42} />);

    await user.click(await screen.findByRole("button", { name: "Validate" }));

    expect(
      await screen.findByText("WSAA login rejected: cert not authorized"),
    ).toBeInTheDocument();
  });

  it("offers a credit note only on authorized receipts and confirms before crediting", async () => {
    const user = userEvent.setup();
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({ setup_status: "ready" }),
    );
    (fiscalApi.listReceipts as jest.Mock)
      .mockResolvedValueOnce([
        receipt({
          id: 5,
          bill_id: 500,
          status: "authorized",
          receipt_number: "A-0001-5",
          auth_code: "71234567890123",
        }),
        receipt({ id: 6, bill_id: 600, status: "pending" }),
      ])
      .mockResolvedValueOnce([
        receipt({
          id: 5,
          bill_id: 500,
          status: "credited",
          receipt_number: "A-0001-5",
          auth_code: "71234567890123",
        }),
        receipt({ id: 6, bill_id: 600, status: "pending" }),
      ]);
    (fiscalApi.creditFiscalReceipt as jest.Mock).mockResolvedValueOnce({
      ok: true,
      business_id: 42,
    });

    render(<FiscalDashboard businessId={42} />);

    // Only the authorized receipt gets a Credit note button.
    const creditButtons = await screen.findAllByRole("button", {
      name: "Credit note",
    });
    expect(creditButtons).toHaveLength(1);
    // CAE is shown for the authorized receipt.
    expect(screen.getByText(/71234567890123/)).toBeInTheDocument();

    await user.click(creditButtons[0]);

    // Confirmation dialog appears; type an optional reason and confirm.
    await user.type(
      await screen.findByLabelText("Reason (optional)"),
      "Order cancelled",
    );
    await user.click(screen.getByRole("button", { name: "Issue credit note" }));

    await waitFor(() => {
      expect(fiscalApi.creditFiscalReceipt).toHaveBeenCalledWith(
        42,
        5,
        "Order cancelled",
      );
    });
    expect(screen.getByText("Credit note queued")).toBeInTheDocument();
  });

  it("names the unsupported country in the empty state (L6-22)", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({ country: "BR", provider: "", setup_status: "draft" }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);

    render(<FiscalDashboard businessId={42} />);

    expect(
      await screen.findByText("E-invoicing isn't available in your region yet"),
    ).toBeInTheDocument();
    expect(screen.getByText(/not available in BR/i)).toBeInTheDocument();
  });

  it("keeps invoice setup visible when receipts fail (#616)", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockRejectedValue(
      new Error("network"),
    );

    render(<FiscalDashboard businessId={42} />);

    expect(await screen.findByText("Could not load invoice data.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit setup" })).toBeInTheDocument();
  });

});

describe("isValidEmitterTaxId", () => {
  it("requires exactly 11 digits for the arca provider", () => {
    expect(isValidEmitterTaxId("30123456789", "arca")).toBe(true);
    expect(isValidEmitterTaxId("30-12345678-9", "arca")).toBe(true);
    expect(isValidEmitterTaxId("123", "arca")).toBe(false);
    expect(isValidEmitterTaxId("301234567890", "arca")).toBe(false);
  });

  it("allows empty while drafting and other providers unchanged", () => {
    expect(isValidEmitterTaxId("", "arca")).toBe(true);
    expect(isValidEmitterTaxId("anything", "edicom")).toBe(true);
  });
});
