/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { OrderRow } from "../OrderRow";
import type { DeliveryOrder } from "@/api/delivery";

// Mock NextUI to plain elements so the chip/button text is queryable.
jest.mock("@nextui-org/react", () => ({
  Chip: ({ children }: any) => <span data-testid="status-chip">{children}</span>,
  Button: ({ children, "aria-label": ariaLabel }: any) => (
    <button type="button" aria-label={ariaLabel}>
      {children}
    </button>
  ),
  Select: ({ children }: any) => <div>{children}</div>,
  SelectItem: ({ children }: any) => <div>{children}</div>,
}));

jest.mock("../../../operational-alerts/OperationalAlertClaimStatus", () => ({
  __esModule: true,
  default: () => null,
}));

// tString humanizes via the dispatch.filters.* map; advance interpolates {next}.
const tString = (key: string, vars?: Record<string, string>) => {
  const map: Record<string, string> = {
    "dispatch.filters.picked_up": "Picked up",
    "dispatch.filters.nearby": "Nearby",
    "dispatch.filters.in_transit": "In transit",
    "dispatch.actions.advance": "Advance to {next}",
    "dispatch.actions.cancel": "Cancel",
    "dispatch.actions.assignDriver": "Assign driver",
    "dispatch.actions.assign": "Assign",
    "dispatch.actions.selectDriver": "Select driver",
    "dispatch.order.noEta": "No ETA",
    "dispatch.order.cutoff": "Cutoff",
    "dispatch.order.fee": "Fee",
  };
  let val = map[key] ?? key;
  if (vars) {
    Object.entries(vars).forEach(([k, v]) => {
      val = val.replace(`{${k}}`, v);
    });
  }
  return val;
};

function makeOrder(status: string): DeliveryOrder {
  return {
    id: 1,
    business_id: 1,
    delivery_number: "DEL-9",
    delivery_type: "in_house",
    status,
    customer_name: "Guest",
    delivery_address: { city: "Austin", country: "US" },
    delivery_fee: 4,
    created_at: "2026-01-01T10:00:00Z",
    updated_at: "2026-01-01T10:00:00Z",
  } as unknown as DeliveryOrder;
}

describe("OrderRow i18n", () => {
  it("renders a humanized translated status chip, not raw snake_case", () => {
    render(
      <OrderRow
        order={makeOrder("picked_up")}
        drivers={[]}
        onAdvance={jest.fn()}
        onAssignDriver={jest.fn()}
        onCancel={jest.fn()}
        tString={tString}
      />,
    );
    const chip = screen.getByTestId("status-chip");
    expect(chip).toHaveTextContent("Picked up");
    expect(chip).not.toHaveTextContent("picked_up");
  });

  it("advance label interpolates the translated next status, not the enum", () => {
    render(
      <OrderRow
        order={makeOrder("picked_up")}
        drivers={[]}
        onAdvance={jest.fn()}
        onAssignDriver={jest.fn()}
        onCancel={jest.fn()}
        tString={tString}
      />,
    );
    // getNextStatus(picked_up) === "in_transit"
    expect(
      screen.getByRole("button", { name: /Advance to In transit order DEL-9/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Advance to in_transit/ }),
    ).not.toBeInTheDocument();
  });
});

describe("OrderRow business-timezone correctness (R17)", () => {
  // Only estimated_delivery_time is set (cutoff_at stays unset → "No ETA"), so the
  // sole H:MM token in the row is the ETA; the $4.00 fee uses a dot, not a colon.
  const etaOrder = () =>
    ({
      ...makeOrder("picked_up"),
      // 16:00 UTC → 13:00 in America/Argentina/Buenos_Aires (UTC-3, no DST in July)
      estimated_delivery_time: "2026-07-11T16:00:00Z",
    }) as DeliveryOrder;

  it("renders the ETA on the business clock, not the device/UTC clock", () => {
    const { container } = render(
      <OrderRow
        order={etaOrder()}
        drivers={[]}
        onAdvance={jest.fn()}
        onAssignDriver={jest.fn()}
        onCancel={jest.fn()}
        tString={tString}
        locale="en"
        businessTimezone="America/Argentina/Buenos_Aires"
      />,
    );
    // Business time is 13:00 → "01:00 PM"; the raw UTC 16:00 → "04:00 PM" must not leak.
    expect(container.textContent).toMatch(/1:00/);
    expect(container.textContent).not.toMatch(/4:00/);
  });

  it("falls back to UTC (never device TZ) when businessTimezone is null", () => {
    const { container } = render(
      <OrderRow
        order={etaOrder()}
        drivers={[]}
        onAdvance={jest.fn()}
        onAssignDriver={jest.fn()}
        onCancel={jest.fn()}
        tString={tString}
        locale="en"
        businessTimezone={null}
      />,
    );
    // UTC fallback → 16:00 → "04:00 PM"; the Buenos Aires 13:00 ("1:00") must not appear.
    expect(container.textContent).toMatch(/4:00/);
    expect(container.textContent).not.toMatch(/1:00/);
  });
});
