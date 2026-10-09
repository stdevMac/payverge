/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import PaymentCelebration, { type MoneyMoment } from "../PaymentCelebration";

// Snap count-up to its target so the rendered amount is deterministic.
beforeAll(() => {
  (window as any).matchMedia = (query: string) => ({
    matches: query.includes("reduce"),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
    onchange: null,
  });
});

const t = (key: string) => key.split(".").pop() as string;

const moment: MoneyMoment = { id: "m1", amount: 25.5, method: "card", billId: 3 };

describe("PaymentCelebration", () => {
  it("renders nothing with no moments", () => {
    const { container } = render(
      <PaymentCelebration moments={[]} currency="USD" t={t} onDismiss={jest.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("announces the payment in a polite live region with the real amount", () => {
    render(<PaymentCelebration moments={[moment]} currency="USD" t={t} onDismiss={jest.fn()} />);
    const region = screen.getByRole("status");
    expect(region).toHaveAttribute("aria-live", "polite");
    // Amount snapped to target ($25.50).
    expect(screen.getByTestId("money-moment-amount").textContent).toContain("25");
    // Method + bill metadata rendered (leaf keys via the t stub).
    expect(screen.getByText(/methodCard/)).toBeInTheDocument();
    expect(screen.getByText(/bill #3/i)).toBeInTheDocument();
  });

  it("dismisses a card via its close button", () => {
    const onDismiss = jest.fn();
    render(<PaymentCelebration moments={[moment]} currency="USD" t={t} onDismiss={onDismiss} />);
    fireEvent.click(screen.getByRole("button", { name: "dismiss" }));
    expect(onDismiss).toHaveBeenCalledWith("m1");
  });

  it("stacks multiple celebrations", () => {
    const moments: MoneyMoment[] = [
      moment,
      { id: "m2", amount: 8, method: "crypto", billId: null },
    ];
    render(<PaymentCelebration moments={moments} currency="USD" t={t} onDismiss={jest.fn()} />);
    expect(screen.getAllByTestId("money-moment-amount")).toHaveLength(2);
  });
});
