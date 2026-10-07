/** @jest-environment jsdom */
/**
 * #776 — ES KDS leftover tickets must show operator-locale allergen chips
 * (Gluten / Sésamo) and remapped guest note chrome, not GLUTEN / SESAME
 * or "Table 1 - Additional Items".
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/api/orders", () => ({
  updateOrderStatus: jest.fn(),
  parseOrderItems: (items: unknown) =>
    Array.isArray(items) ? items : JSON.parse((items as string) || "[]"),
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

const harvestBowl = {
  id: "hb-1",
  menu_item_id: "harvest-bowl",
  menu_item_name: "Harvest Bowl",
  quantity: 1,
  price: 14,
  subtotal: 14,
};

function renderEsKds() {
  const order = {
    id: 1132,
    bill_id: 10,
    business_id: 1,
    order_number: "K-1132",
    status: "in_kitchen",
    notes: "Table 1 - Additional Items",
    items: [harvestBowl],
    currency: "USD",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as Order;

  return render(
    <KitchenDisplayMode
      orders={[order]}
      onExit={jest.fn()}
      onRefresh={jest.fn()}
      businessId={1}
      locale={"es" as never}
      allergensByItemId={{ "harvest-bowl": ["gluten", "sesame"] }}
      onOrderStatusChange={jest.fn()}
    />,
  );
}

describe("KitchenDisplayMode allergen chips + note chrome (#776)", () => {
  it("renders Gluten / Sésamo and remaps leftover English note chrome", () => {
    renderEsKds();

    expect(screen.getByText("Gluten")).toBeInTheDocument();
    expect(screen.getByText("Sésamo")).toBeInTheDocument();
    expect(screen.queryByText("GLUTEN")).not.toBeInTheDocument();
    expect(screen.queryByText("SESAME")).not.toBeInTheDocument();
    expect(screen.queryByText("gluten")).not.toBeInTheDocument();
    expect(screen.queryByText("sesame")).not.toBeInTheDocument();

    const chips = screen.getAllByTestId("kitchen-allergen-chip");
    expect(chips).toHaveLength(2);
    for (const chip of chips) {
      expect(chip.className).not.toMatch(/uppercase/);
    }

    // Seed "Table 1" prefixes are localized too now (#776 follow-up).
    expect(screen.getByTestId("kds-ticket-note")).toHaveTextContent(
      "Mesa 1 - Artículos adicionales",
    );
    expect(screen.queryByText("Table 1 - Additional Items")).not.toBeInTheDocument();
    expect(screen.queryByText(/Additional Items/)).not.toBeInTheDocument();
  });
});
