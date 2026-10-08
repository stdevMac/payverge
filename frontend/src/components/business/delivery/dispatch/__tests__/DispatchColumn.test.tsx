/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import DispatchColumn from "../DispatchColumn";

describe("DispatchColumn", () => {
  it("renders title + count + cards", () => {
    const orders = [
      { id: 1, code: "AI-DEL-0001", status: "pending", customer_name: "G1", cutoff: new Date(Date.now()+9e5).toISOString(), fee: 18.02 },
    ];
    render(<DispatchColumn title="Pending" orders={orders} onAdvance={jest.fn()} onCancel={jest.fn()} />);
    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(screen.getByText("(1)")).toBeInTheDocument();
    expect(screen.getByText(/AI-DEL-0001/)).toBeInTheDocument();
  });
});
