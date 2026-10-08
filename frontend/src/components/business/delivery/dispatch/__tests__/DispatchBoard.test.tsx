/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import DispatchBoard from "../DispatchBoard";

const cutoffSoon = new Date(Date.now() + 9e5).toISOString();
const etaSoon = new Date(Date.now() + 2.7e6).toISOString();

const orders = [
  {
    id: 1,
    code: "AI-DEL-0001",
    status: "preparing",
    customer_name: "G1",
    eta: etaSoon,
    cutoff: cutoffSoon,
    fee: 18.02,
  },
  {
    id: 2,
    code: "AI-DEL-0002",
    status: "delivered",
    customer_name: "G2",
    eta: etaSoon,
    cutoff: cutoffSoon,
    fee: 18.04,
  },
];

describe("DispatchBoard — logistics columns", () => {
  it("renders intake + logistics columns including awaiting payment", () => {
    render(<DispatchBoard orders={orders} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.getByText("Awaiting payment")).toBeInTheDocument();
    expect(screen.getByText("Preparing")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("Out for delivery")).toBeInTheDocument();
    expect(screen.getByText("Delivered")).toBeInTheDocument();
  });

  it("places pending/confirmed orders in the awaiting payment column", () => {
    const intake = [
      {
        id: 9,
        code: "AI-DEL-PENDING",
        status: "pending",
        customer_name: "Pending Guest",
        fee: 4.99,
      },
      {
        id: 10,
        code: "AI-DEL-CONFIRMED",
        status: "confirmed",
        customer_name: "Pay Guest",
        fee: 4.99,
        awaiting_payment: true,
      },
    ];
    render(<DispatchBoard orders={intake} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.getByText("AI-DEL-PENDING")).toBeInTheDocument();
    expect(screen.getByText("AI-DEL-CONFIRMED")).toBeInTheDocument();
    expect(screen.getByText("Awaiting payment")).toBeInTheDocument();
  });

  it("maps picked_up into the out_for_delivery column", () => {
    const activeOrders = [
      {
        id: 10,
        code: "AI-DEL-PICKED",
        status: "picked_up",
        customer_name: "Picked Guest",
        eta: etaSoon,
        cutoff: cutoffSoon,
        fee: 5,
      },
    ];
    render(<DispatchBoard orders={activeOrders} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.getByText("AI-DEL-PICKED")).toBeInTheDocument();
    expect(screen.getByText("Out for delivery")).toBeInTheDocument();
  });

  it("maps assigned/in_transit/nearby into the out_for_delivery column", () => {
    const activeOrders = [
      { id: 10, code: "AI-DEL-ASSIGNED", status: "assigned", customer_name: "Assigned Guest", fee: 5 },
      { id: 11, code: "AI-DEL-INTRANSIT", status: "in_transit", customer_name: "Transit Guest", fee: 5 },
      { id: 12, code: "AI-DEL-NEARBY", status: "nearby", customer_name: "Nearby Guest", fee: 5 },
    ];
    render(<DispatchBoard orders={activeOrders} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.getByText("AI-DEL-ASSIGNED")).toBeInTheDocument();
    expect(screen.getByText("AI-DEL-INTRANSIT")).toBeInTheDocument();
    expect(screen.getByText("AI-DEL-NEARBY")).toBeInTheDocument();
  });

  // issue 238 — five 200px lanes expand flex ancestors; dashboard overflow-hidden
  // then clips Out for delivery / Delivered with no working scroller.
  it("keeps Out for delivery / Delivered reachable via a min-width-safe scroller", () => {
    render(<DispatchBoard orders={orders} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    const scroller = screen.getByTestId("dispatch-board-scroll");
    expect(scroller.className).toMatch(/overflow-x-auto/);
    expect(scroller.className).toMatch(/min-w-0/);
    expect(scroller.className).toMatch(/\bw-full\b/);
    expect(scroller.className).toMatch(/scrollbar-thin/);
    expect(screen.getByTestId("dispatch-column-out_for_delivery").className).toMatch(
      /shrink-0/,
    );
    expect(screen.getByTestId("dispatch-column-delivered").className).toMatch(
      /min-w-\[200px\]/,
    );
    expect(screen.getByText("Out for delivery")).toBeInTheDocument();
    expect(screen.getByText("Delivered")).toBeInTheDocument();
    expect(
      screen.getByText("Scroll sideways for Out for delivery and Delivered"),
    ).toBeInTheDocument();
  });

  it("board filter: renders only cancelled orders in a single column", () => {
    const mixed = [
      { id: 1, code: "AI-DEL-PREP", status: "preparing", customer_name: "Prep Guest", fee: 5 },
      { id: 2, code: "AI-DEL-CANCEL", status: "cancelled", customer_name: "Cancelled Guest", fee: 5 },
    ];
    render(
      <DispatchBoard
        orders={mixed}
        onAdvance={jest.fn()}
        onCancel={jest.fn()}
        boardFilter="cancelled"
      />,
    );
    expect(screen.getByText("AI-DEL-CANCEL")).toBeInTheDocument();
    expect(screen.queryByText("AI-DEL-PREP")).not.toBeInTheDocument();
    expect(screen.queryByText("Preparing")).not.toBeInTheDocument();
  });
});
