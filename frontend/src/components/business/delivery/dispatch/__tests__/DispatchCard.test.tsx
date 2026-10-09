/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import DispatchCard from "../DispatchCard";

// DriverAssignment uses NextUI Select/Button — stub it so card tests stay
// focused on the card logic and don't need NextUI rendering. Like the real
// DriverAssignment, the stub catches a rejected onAssign (the parent surfaces
// the toast) so failed assigns don't escape as unhandled rejections.
jest.mock("../DriverAssignment", () => ({
  DriverAssignment: ({ onAssign, tString }: { drivers: unknown[]; onAssign: (id: number) => void | Promise<void>; tString: (k: string) => string }) => (
    <div data-testid="driver-assignment-panel">
      <button onClick={() => void Promise.resolve(onAssign(5)).catch(() => {})}>{tString("actions.assign")}</button>
    </div>
  ),
}));

const etaSoon = new Date(Date.now() + 45 * 60 * 1000).toISOString();
const cutoffSoon = new Date(Date.now() + 8 * 60 * 1000).toISOString();

const baseOrder = {
  id: 1,
  code: "AI-DEL-0001",
  status: "preparing",
  customer_name: "Guest 1",
  customer_phone: "+1-555-0100",
  address: "123 Main St, Austin",
  eta: etaSoon,
  cutoff: cutoffSoon,
  fee: 18.02,
  order_total: 42.5,
  item_count: 3,
  driver: null as null | { name: string; live: boolean; eta?: string },
};

describe("DispatchCard", () => {
  it("renders ETA + fee + customer + address/phone + Advance button", () => {
    render(<DispatchCard order={baseOrder} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.getByText(/AI-DEL-0001/)).toBeInTheDocument();
    expect(screen.getByText(/Guest 1/)).toBeInTheDocument();
    expect(screen.getByText("+1-555-0100")).toBeInTheDocument();
    expect(screen.getByText(/123 Main St/)).toBeInTheDocument();
    expect(screen.getByTestId("dispatch-item-count")).toHaveTextContent(/3/);
    expect(screen.getByTestId("dispatch-order-total")).toHaveTextContent(/\$42\.50/);
    expect(screen.getByTestId("dispatch-fee")).toHaveTextContent(/\$18\.02/);
    expect(screen.getByTestId("dispatch-eta")).toHaveTextContent(/ETA/i);
    expect(screen.getByRole("button", { name: /Advance/i })).toBeInTheDocument();
  });

  it("does not offer Advance for pending/confirmed intake", () => {
    for (const status of ["pending", "confirmed"]) {
      const { unmount } = render(
        <DispatchCard
          order={{ ...baseOrder, status }}
          onAdvance={jest.fn()}
          onCancel={jest.fn()}
        />,
      );
      expect(screen.queryByRole("button", { name: /Advance/i })).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: /^Cancel$/i })).toBeInTheDocument();
      unmount();
    }
  });

  it("calls onAdvance when Advance clicked", () => {
    const onAdvance = jest.fn();
    render(<DispatchCard order={baseOrder} onAdvance={onAdvance} onCancel={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /Advance/i }));
    expect(onAdvance).toHaveBeenCalledWith(baseOrder);
  });

  it("turns ETA rose when cutoff < 15 min remain on a logistics order", () => {
    const order = { ...baseOrder, cutoff: new Date(Date.now() + 10 * 60 * 1000).toISOString() };
    render(<DispatchCard order={order} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.getByTestId("dispatch-eta")).toHaveClass("text-rose-500");
  });

  it("does NOT flag a delivered order as urgent even with a long-past cutoff", () => {
    const order = {
      ...baseOrder,
      status: "delivered",
      cutoff: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(),
    };
    render(<DispatchCard order={order} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.getByTestId("dispatch-eta")).not.toHaveClass("text-rose-500");
  });

  it("advance label tracks the real next status (in_transit -> nearby, not delivered)", () => {
    const order = { ...baseOrder, status: "in_transit" };
    render(<DispatchCard order={order} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    const advance = screen.getByRole("button", { name: /Advance/i });
    expect(advance).toHaveTextContent(/nearby/i);
    expect(advance).not.toHaveTextContent(/delivered/i);
  });

  it("uses translated next-status labels when a tString is provided", () => {
    const tString = (key: string) => {
      const map: Record<string, string> = {
        "actions.advance": "Advance to {next}",
        "filters.nearby": "Nearby",
      };
      return map[key] ?? key;
    };
    const order = { ...baseOrder, status: "in_transit" };
    render(
      <DispatchCard order={order} onAdvance={jest.fn()} onCancel={jest.fn()} tString={tString} />,
    );
    expect(screen.getByRole("button", { name: /Advance to Nearby/i })).toBeInTheDocument();
  });

  it("renders no advance button for a terminal (delivered) order", () => {
    const order = { ...baseOrder, status: "delivered" };
    render(<DispatchCard order={order} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.queryByRole("button", { name: /Advance/i })).not.toBeInTheDocument();
  });

  it("hides the Cancel button on terminal orders (delivered/cancelled/failed)", () => {
    for (const status of ["delivered", "cancelled", "failed"]) {
      const { unmount } = render(
        <DispatchCard order={{ ...baseOrder, status }} onAdvance={jest.fn()} onCancel={jest.fn()} />,
      );
      expect(screen.queryByRole("button", { name: /^Cancel$/i })).not.toBeInTheDocument();
      unmount();
    }
  });

  it("shows the Cancel button on non-terminal orders", () => {
    const onCancel = jest.fn();
    const order = { ...baseOrder, status: "pending" };
    render(<DispatchCard order={order} onAdvance={jest.fn()} onCancel={onCancel} />);
    const cancel = screen.getByRole("button", { name: /^Cancel$/i });
    expect(cancel).toBeInTheDocument();
    fireEvent.click(cancel);
    expect(onCancel).toHaveBeenCalledWith(order);
  });

  it("shows Assign driver button (not Advance) for a ready order with no driver", () => {
    const order = { ...baseOrder, status: "ready", driver: null };
    const onAssignDriver = jest.fn().mockResolvedValue(undefined);
    render(
      <DispatchCard
        order={order}
        onAdvance={jest.fn()}
        onCancel={jest.fn()}
        onAssignDriver={onAssignDriver}
        drivers={[{ id: 5, business_id: 1, name: "Maria", phone: "555-0005", status: "online" as const, is_available: true, is_active: true }]}
      />,
    );
    expect(screen.getByTestId("assign-driver-btn")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Advance/i })).not.toBeInTheDocument();
  });

  it("shows Advance button for a ready order that already has a driver", () => {
    const order = {
      ...baseOrder,
      status: "ready",
      driver: { name: "Carlos", live: false },
    };
    render(
      <DispatchCard
        order={order}
        onAdvance={jest.fn()}
        onCancel={jest.fn()}
        onAssignDriver={jest.fn()}
        drivers={[]}
      />,
    );
    expect(screen.getByRole("button", { name: /Advance/i })).toBeInTheDocument();
    expect(screen.queryByTestId("assign-driver-btn")).not.toBeInTheDocument();
  });

  it("ready+driver: advance button label resolves picked_up (not assigned)", () => {
    const tString = (key: string) => {
      const map: Record<string, string> = {
        "actions.advance": "Advance to {next}",
        "filters.picked_up": "Picked up",
        "filters.assigned": "Assigned",
      };
      return map[key] ?? key;
    };
    const order = {
      ...baseOrder,
      status: "ready",
      driver: { name: "Carlos", live: false },
    };
    render(
      <DispatchCard
        order={order}
        onAdvance={jest.fn()}
        onCancel={jest.fn()}
        tString={tString}
      />,
    );
    const btn = screen.getByRole("button", { name: /Advance to Picked up/i });
    expect(btn).toBeInTheDocument();
    expect(btn).not.toHaveTextContent(/Assigned/i);
  });

  it("ready+driver: clicking advance calls onAdvance", () => {
    const onAdvance = jest.fn();
    const order = {
      ...baseOrder,
      status: "ready",
      driver: { name: "Carlos", live: false },
    };
    render(
      <DispatchCard order={order} onAdvance={onAdvance} onCancel={jest.fn()} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Advance/i }));
    expect(onAdvance).toHaveBeenCalledWith(order);
  });

  it("assign panel passes keys to DriverAssignment without double dispatch. prefix", () => {
    const seenKeys: string[] = [];
    const tString = (key: string) => {
      seenKeys.push(key);
      const map: Record<string, string> = {
        "actions.assign": "Assign",
        "actions.selectDriver": "Select driver",
        "actions.assignDriver": "Assign driver",
        "actions.noDriversAvailable": "No drivers available",
        "assignDriver": "Assign driver",
      };
      return map[key] ?? key;
    };
    const order = { ...baseOrder, status: "ready", driver: null };
    render(
      <DispatchCard
        order={order}
        onAdvance={jest.fn()}
        onCancel={jest.fn()}
        onAssignDriver={jest.fn().mockResolvedValue(undefined)}
        drivers={[{ id: 5, business_id: 1, name: "Maria", phone: "555-0005", status: "online" as const, is_available: true, is_active: true }]}
        tString={tString}
      />,
    );
    fireEvent.click(screen.getByTestId("assign-driver-btn"));
    expect(seenKeys.some((k) => k.startsWith("dispatch."))).toBe(false);
    expect(screen.queryByText(/dispatch\.dispatch\./i)).not.toBeInTheDocument();
  });

  it("re-enables the advance button after a failed advance (DEL-OP-1)", async () => {
    const onAdvance = jest.fn().mockRejectedValue(new Error("network blip"));
    render(<DispatchCard order={baseOrder} onAdvance={onAdvance} onCancel={jest.fn()} />);
    const btn = screen.getByRole("button", { name: /Advance/i });
    fireEvent.click(btn);
    expect(onAdvance).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(btn).not.toBeDisabled());
    fireEvent.click(btn);
    expect(onAdvance).toHaveBeenCalledTimes(2);
  });

  it("re-enables the advance button after an in-column advance re-renders the same instance (DEL-OP-1)", async () => {
    const onAdvance = jest.fn().mockResolvedValue(undefined);
    const order = { ...baseOrder, status: "picked_up" };
    const { rerender } = render(
      <DispatchCard order={order} onAdvance={onAdvance} onCancel={jest.fn()} />,
    );
    const btn = screen.getByRole("button", { name: /Advance/i });
    fireEvent.click(btn);
    await waitFor(() => expect(onAdvance).toHaveBeenCalledTimes(1));
    rerender(
      <DispatchCard order={{ ...order, status: "in_transit" }} onAdvance={onAdvance} onCancel={jest.fn()} />,
    );
    const nextBtn = screen.getByRole("button", { name: /Advance/i });
    await waitFor(() => expect(nextBtn).not.toBeDisabled());
    fireEvent.click(nextBtn);
    expect(onAdvance).toHaveBeenCalledTimes(2);
  });

  it("disables the advance button while the advance is in flight (double-click guard)", async () => {
    let resolveAdvance: () => void = () => {};
    const onAdvance = jest.fn().mockImplementation(
      () => new Promise<void>((resolve) => { resolveAdvance = resolve; }),
    );
    render(<DispatchCard order={baseOrder} onAdvance={onAdvance} onCancel={jest.fn()} />);
    const btn = screen.getByRole("button", { name: /Advance/i });
    fireEvent.click(btn);
    expect(btn).toBeDisabled();
    fireEvent.click(btn);
    expect(onAdvance).toHaveBeenCalledTimes(1);
    resolveAdvance();
    await waitFor(() => expect(btn).not.toBeDisabled());
  });

  it("keeps the assign panel open when driver assignment fails (DEL-OP-4)", async () => {
    const onAssignDriver = jest.fn().mockRejectedValue(new Error("assign failed"));
    const order = { ...baseOrder, status: "ready", driver: null };
    render(
      <DispatchCard
        order={order}
        onAdvance={jest.fn()}
        onCancel={jest.fn()}
        onAssignDriver={onAssignDriver}
        drivers={[{ id: 5, business_id: 1, name: "Maria", phone: "555-0005", status: "online" as const, is_available: true, is_active: true }]}
      />,
    );
    fireEvent.click(screen.getByTestId("assign-driver-btn"));
    fireEvent.click(screen.getByText("actions.assign"));
    await waitFor(() => expect(onAssignDriver).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId("driver-assignment-panel")).toBeInTheDocument();
  });

  it("closes the assign panel when driver assignment succeeds", async () => {
    const onAssignDriver = jest.fn().mockResolvedValue(undefined);
    const order = { ...baseOrder, status: "ready", driver: null };
    render(
      <DispatchCard
        order={order}
        onAdvance={jest.fn()}
        onCancel={jest.fn()}
        onAssignDriver={onAssignDriver}
        drivers={[{ id: 5, business_id: 1, name: "Maria", phone: "555-0005", status: "online" as const, is_available: true, is_active: true }]}
      />,
    );
    fireEvent.click(screen.getByTestId("assign-driver-btn"));
    fireEvent.click(screen.getByText("actions.assign"));
    await waitFor(() =>
      expect(screen.queryByTestId("driver-assignment-panel")).not.toBeInTheDocument(),
    );
  });

  it("formats the driver ETA as a local time, not a raw ISO string", () => {
    const order = {
      ...baseOrder,
      status: "in_transit",
      eta: "2026-05-12T17:25:00Z",
      driver: { name: "Carlos", live: true, eta: "2026-05-12T16:55:00Z" },
    };
    render(<DispatchCard order={order} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.queryByText(/2026-05-12T16:55:00Z/)).toBeNull();
    const driverLine = screen.getByText(/Carlos/);
    expect(driverLine.textContent).toMatch(/\d{1,2}:\d{2}/);
    expect(driverLine.textContent).not.toMatch(/\d{4}-\d{2}-\d{2}T/);
  });
});
