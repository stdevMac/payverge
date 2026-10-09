/** @jest-environment jsdom */
/**
 * #775: KDS En Cocina cards must put a real text separator between the
 * order number and bill label. Flex gap is visual-only and smashes innerText.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  getTranslation: (key: string) => {
    if (key === "kitchenDisplay.billPrefix") return "Cuenta n.º {billId}";
    if (key === "kitchenDisplay.columns.in_kitchen") return "En Cocina";
    return key;
  },
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
import { kitchenTicketIdentityText } from "@/components/business/kitchenTicketIdentity";

const leftover = (orderNumber: string, billId: number): Order =>
  ({
    id: billId,
    bill_id: billId,
    business_id: 1,
    order_number: orderNumber,
    status: "in_kitchen",
    items: JSON.stringify([
      {
        id: `i${billId}`,
        menu_item_name: "Steak",
        quantity: 1,
        price: 24,
        options: [],
        special_requests: "",
        subtotal: 24,
      },
    ]),
    currency: "USD",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }) as Order;

const leftovers: Order[] = [
  leftover("G86-36604192", 1132),
  leftover("G86-36604193", 1134),
  leftover("G86-36604194", 1141),
];

function identityText(el: HTMLElement): string {
  return (el.innerText || el.textContent || "").replace(/\u00a0/g, " ");
}

it("keeps KDS in-kitchen identities as readable tokens (#775)", () => {
  render(
    <KitchenDisplayMode
      orders={leftovers}
      onExit={jest.fn()}
      onRefresh={jest.fn()}
      businessId={1}
      locale={"es" as never}
      onOrderStatusChange={jest.fn()}
    />,
  );

  expect(screen.getByText("En Cocina")).toBeInTheDocument();

  const identities = screen.getAllByTestId("kds-ticket-identity");
  expect(identities).toHaveLength(3);

  const expected = leftovers.map((order) =>
    kitchenTicketIdentityText(
      order.order_number,
      `Cuenta n.º ${order.bill_id}`,
    ),
  );

  const actual = identities.map((el) => identityText(el));
  expect(actual).toEqual(expected);
  for (const text of actual) {
    expect(text).toMatch(/#G86-\d+ · Cuenta n\.º \d+/);
    expect(text).not.toMatch(/#G86-\d+Cuenta/);
  }
});
