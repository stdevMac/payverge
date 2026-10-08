/** @jest-environment jsdom */
/**
 * D1 / L1-25: KDS tickets must not render "Table Table 1" when the stored
 * table name already embeds the type word. Asserts the mounted location chip
 * on KitchenDisplayMode (audit surface), not only formatEntityName helper math.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

type GetTranslationArgs = Parameters<
  typeof import("@/i18n/getTranslation").getTranslation
>;

const mockGetTranslation = jest.fn((...args: GetTranslationArgs) => args[0]);

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  getTranslation: (...args: GetTranslationArgs) => mockGetTranslation(...args),
}));

jest.mock("@/api/orders", () => ({
  updateOrderStatus: jest.fn(),
  parseOrderItems: (items: string) => JSON.parse(items || "[]"),
}));

jest.mock(
  "@/components/business/operational-alerts/EnableAlertSoundButton",
  () => ({ __esModule: true, default: () => null }),
);
jest.mock(
  "@/components/business/operational-alerts/OperationalAlertClaimStatus",
  () => ({ __esModule: true, default: () => null }),
);

import KitchenDisplayMode from "@/components/business/KitchenDisplayMode";
import type { Order } from "@/api/orders";

const baseOrder = (
  id: number,
  bill: Record<string, unknown> | undefined,
): Order =>
  ({
    id,
    bill_id: 10,
    business_id: 1,
    order_number: `K-${id}`,
    status: "approved",
    items: JSON.stringify([
      {
        id: `i${id}`,
        menu_item_name: "Soup",
        quantity: 1,
        price: 10,
        subtotal: 10,
      },
    ]),
    currency: "USD",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    bill,
  }) as unknown as Order;

function renderKds(
  tableName: string,
  locationTableLabel: string,
  locale: "en" | "es" = "en",
) {
  mockGetTranslation.mockImplementation((key: string) => {
    if (key === "kitchenDisplay.location.table") return locationTableLabel;
    if (key === "kitchenDisplay.location.counter") {
      return locationTableLabel === "Mesa" ? "Mostrador" : "Counter";
    }
    if (key === "kitchenDisplay.location.delivery") {
      return locationTableLabel === "Mesa" ? "Entrega" : "Delivery";
    }
    if (key === "kitchenDisplay.billPrefix") return "Bill {billId}";
    return key;
  });

  return render(
    <KitchenDisplayMode
      orders={[
        baseOrder(1, {
          id: 5,
          table_id: 1,
          table_name: tableName,
        }),
      ]}
      onExit={jest.fn()}
      onRefresh={jest.fn()}
      businessId={1}
      locale={locale as never}
      onOrderStatusChange={jest.fn()}
    />,
  );
}

describe("KitchenDisplayMode L1-25 table label DOM (D1)", () => {
  beforeEach(() => {
    mockGetTranslation.mockReset();
  });

  it("renders Table 1 once for English seed names (never Table Table 1)", () => {
    renderKds("Table 1", "Table", "en");

    expect(screen.getByText("Table 1")).toBeInTheDocument();
    expect(screen.queryByText("Table Table 1")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/Table\s+Table\s+1/);
  });

  it("localizes English seed Table 9 to Mesa 9 under Spanish labels", () => {
    renderKds("Table 9", "Mesa", "es");

    expect(screen.getByText("Mesa 9")).toBeInTheDocument();
    expect(screen.queryByText("Table 9")).not.toBeInTheDocument();
    expect(screen.queryByText("Mesa Table 9")).not.toBeInTheDocument();
  });

  it("does not double-prefix when name already starts with Mesa", () => {
    renderKds("Mesa 2", "Mesa", "es");
    expect(screen.getByText("Mesa 2")).toBeInTheDocument();
    expect(screen.queryByText("Mesa Mesa 2")).not.toBeInTheDocument();
  });

  it("does not double-prefix custom names that already start with the label word", () => {
    // Not an English seed (no trailing digits-only). De-dupe must keep "Table Vista".
    renderKds("Table Vista", "Table", "en");
    expect(screen.getByText("Table Vista")).toBeInTheDocument();
    expect(screen.queryByText("Table Table Vista")).not.toBeInTheDocument();
  });

  it("KitchenDisplayMode routes location through getBillLocationLabel (wired)", () => {
    const fs = require("fs") as typeof import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(
      path.join(
        process.cwd(),
        "src/components/business/KitchenDisplayMode.tsx",
      ),
      "utf8",
    );
    expect(src).toMatch(/getBillLocationLabel/);
    expect(src).not.toMatch(/T-\$\{/);
  });
});
