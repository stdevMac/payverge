/** @jest-environment jsdom */
/**
 * F33 / F34 — operator table surfaces must route every label through i18n so
 * es operators don't see English. We assert the keys are resolved (the mock
 * echoes the key path) rather than literal English text being rendered.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string, _l?: string, params?: Record<string, unknown>) => {
    if (!params) return key;
    let out = key;
    Object.entries(params).forEach(([k, v]) => {
      out = out.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
    });
    return out;
  },
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (n: number) => `$${(n || 0).toFixed(2)}`,
}));
jest.mock("@/api/onboarding", () => ({
  markQRPreviewed: jest.fn().mockResolvedValue({
    qr_previewed: true,
    qr_previewed_at: "2026-07-31T00:00:00Z",
  }),
}));

// Stub the canvas QR so the detail modal renders headlessly.
jest.mock("../../QRCodeWithText", () => ({
  __esModule: true,
  default: () => <div data-testid="qr" />,
}));
jest.mock("../qrDownload", () => ({
  downloadTableQR: jest.fn(),
}));

import TableDetailModal from "../TableDetailModal";
import TableRow, { TableRowData } from "../TableRow";

const NS = "businessDashboard.dashboard.tableManager";

const baseRow: TableRowData = {
  id: 1,
  name: "Table 1",
  table_code: "T01",
  is_active: true,
  status: "occupied",
  capacity: 4,
  active_bill: {
    id: 10,
    total: 50,
    physical_item_quantity: 1,
    created_at: "2026-08-11T18:00:00.000Z",
  },
  server_name: null,
  next_reservation: null,
  last_seen: null,
};

describe("F33 — TableDetailModal labels are localized", () => {
  it("renders translation keys for the QR action + footer buttons (no hardcoded EN)", () => {
    render(
      <TableDetailModal
        table={baseRow}
        open
        onClose={jest.fn()}
        businessId={1}
        onDelete={jest.fn()}
        onCustomizeQR={jest.fn()}
      />,
    );
    expect(screen.getByText(`${NS}.detailModal.downloadPng`)).toBeInTheDocument();
    expect(screen.getByText(`${NS}.detailModal.customize`)).toBeInTheDocument();
    expect(screen.getByText(`${NS}.detailModal.deleteTable`)).toBeInTheDocument();
    expect(screen.getByText(`${NS}.detailModal.close`)).toBeInTheDocument();
    // No raw English leaked.
    expect(screen.queryByText("Download PNG")).not.toBeInTheDocument();
    expect(screen.queryByText("Delete table")).not.toBeInTheDocument();
  });
});

describe("F34 — TableRow time-ago + item/items are localized", () => {
  const renderRow = (data: Partial<TableRowData>) =>
    render(
      <table>
        <tbody>
          <TableRow
            table={{ ...baseRow, ...data }}
            onSelect={jest.fn()}
            currency="USD"
          />
        </tbody>
      </table>,
    );

  it("uses the singular bill-items key for a one-item bill (L8-1)", () => {
    renderRow({
      active_bill: {
        id: 10,
        total: 50,
        physical_item_quantity: 1,
        created_at: "2026-08-11T18:00:00.000Z",
      },
    });
    expect(
      screen.getByText(new RegExp(`${NS}\\.billItemsOne`)),
    ).toBeInTheDocument();
  });

  it("uses the plural bill-items key for multiple items (L8-1)", () => {
    renderRow({
      active_bill: {
        id: 11,
        total: 50,
        physical_item_quantity: 3,
        created_at: "2026-08-11T18:00:00.000Z",
      },
    });
    expect(
      screen.getByText(new RegExp(`${NS}\\.billItemsOther`)),
    ).toBeInTheDocument();
  });

  it("localizes 'just now' for a fresh last_seen", () => {
    renderRow({ last_seen: new Date().toISOString() });
    expect(
      screen.getByText(`${NS}.timeAgo.justNow`),
    ).toBeInTheDocument();
  });

  it("localizes the minutes-ago label instead of hardcoded EN", () => {
    const tenMinAgo = new Date(Date.now() - 10 * 60000).toISOString();
    renderRow({ last_seen: tenMinAgo });
    expect(
      screen.getByText(`${NS}.timeAgo.minutesAgo`),
    ).toBeInTheDocument();
    // The old hardcoded "10 min ago" form must be gone.
    expect(screen.queryByText("10 min ago")).not.toBeInTheDocument();
  });
});
