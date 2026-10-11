/** @jest-environment jsdom */
/**
 * #650 — US fiscal settings + Spanish operator chrome must not show AR
 * Factura/Recibo types or AFIP identity fields. Previous tests stubbed
 * English strings, so they never saw the es Factura / ID fiscal copy.
 */
import React from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import * as fiscalApi from "@/api/fiscal";
import FiscalDashboard from "./FiscalDashboard";

const mockLocale = { current: "es" };

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({
      locale: mockLocale.current,
      setLocale: jest.fn(),
    }),
  };
});

jest.mock("@/api/fiscal");

function settings(overrides: Partial<fiscalApi.FiscalSettings> = {}) {
  return {
    id: 1,
    business_id: 86,
    country: "US",
    provider: "demo",
    mode: "manual" as const,
    environment: "sandbox" as const,
    tax_id: "",
    tax_condition: "",
    point_of_sale: 0,
    setup_status: "validated",
    ...overrides,
  };
}

function receipt(overrides: Partial<fiscalApi.FiscalReceipt> = {}) {
  return {
    id: 9,
    business_id: 86,
    settings_id: 1,
    bill_id: 123,
    payment_id: null,
    alternative_payment_id: null,
    country: "US",
    provider: "demo",
    action: "issue",
    receipt_type: "invoice",
    receipt_number: "0002",
    provider_receipt_id: null,
    auth_code: null,
    auth_expires_at: null,
    qr_payload: null,
    qr_image_path: null,
    pdf_path: null,
    customer_doc_type: null,
    customer_doc_number: null,
    total_amount_cents: 1804,
    tip_amount_cents: 0,
    currency: "USD",
    status: "authorized" as const,
    error_code: null,
    error_message: null,
    issued_at: "2026-08-20T12:00:00Z",
    created_at: "2026-08-20T12:00:00Z",
    updated_at: "2026-08-20T12:00:00Z",
    ...overrides,
  };
}

describe("FiscalDashboard US + es labels (#650)", () => {
  beforeEach(() => {
    mockLocale.current = "es";
    jest.clearAllMocks();
    (fiscalApi.listReceiptDelivery as jest.Mock).mockResolvedValue([]);
  });

  it("shows Invoice/Receipt (not Factura/Recibo) and hides AFIP fields for country=US locale=es", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(settings());
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({ receipt_type: "invoice", receipt_number: "0002" }),
      receipt({
        id: 10,
        receipt_type: "receipt",
        receipt_number: "0003",
      }),
    ]);

    render(<FiscalDashboard businessId={86} />);

    const table = await screen.findByRole("table");
    expect(
      within(table).getAllByText("Invoice").length,
    ).toBeGreaterThanOrEqual(1);
    expect(within(table).getByText("Receipt")).toBeInTheDocument();
    expect(within(table).queryByText("Factura")).not.toBeInTheDocument();
    expect(within(table).queryByText("Factura A")).not.toBeInTheDocument();
    expect(within(table).queryByText("Factura B")).not.toBeInTheDocument();
    expect(within(table).queryByText("Recibo")).not.toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: /editar configuración/i }),
    );
    expect(screen.queryByText("ID fiscal")).not.toBeInTheDocument();
    expect(screen.queryByText("Punto de venta")).not.toBeInTheDocument();
    expect(screen.queryByText("Condición fiscal")).not.toBeInTheDocument();
  });

  it("still uses Factura labels and AFIP identity fields when country=AR locale=es", async () => {
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(
      settings({
        country: "AR",
        provider: "arca",
        tax_id: "20123456789",
        tax_condition: "responsable_inscripto",
        point_of_sale: 1,
      }),
    );
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([
      receipt({
        country: "AR",
        provider: "arca",
        receipt_type: "factura_b",
        currency: "ARS",
      }),
    ]);

    render(<FiscalDashboard businessId={86} />);

    const table = await screen.findByRole("table");
    expect(within(table).getByText("Factura B")).toBeInTheDocument();
    expect(within(table).getByText("Factura")).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: /editar configuración/i }),
    );
    expect(screen.getByText("ID fiscal")).toBeInTheDocument();
    expect(screen.getByText("Punto de venta")).toBeInTheDocument();
    expect(screen.getByText("Condición fiscal")).toBeInTheDocument();
  });
});
