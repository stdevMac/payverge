/** @jest-environment jsdom */
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MoneyMomentsProvider, useMoneyMoments } from "../MoneyMomentsProvider";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key.split(".").pop(),
}));

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

function Harness({ payload }: { payload: Record<string, unknown> }) {
  const { celebrate } = useMoneyMoments();
  return (
    <button type="button" onClick={() => celebrate(payload)}>
      pay
    </button>
  );
}

describe("MoneyMomentsProvider", () => {
  it("shows a celebration for a real payment", () => {
    render(
      <MoneyMomentsProvider currency="USD" durationMs={6000}>
        <Harness payload={{ amount: 25.5, method: "card", bill_id: 3 }} />
      </MoneyMomentsProvider>,
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("pay"));
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.getByTestId("money-moment-amount").textContent).toContain("25");
  });

  it("ignores a phantom payment (amount <= 0)", () => {
    render(
      <MoneyMomentsProvider>
        <Harness payload={{ amount: 0, method: "card" }} />
      </MoneyMomentsProvider>,
    );
    fireEvent.click(screen.getByText("pay"));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("auto-dismisses after the configured duration", () => {
    jest.useFakeTimers();
    try {
      render(
        <MoneyMomentsProvider durationMs={4000}>
          <Harness payload={{ amount: 12, method: "crypto" }} />
        </MoneyMomentsProvider>,
      );
      fireEvent.click(screen.getByText("pay"));
      expect(screen.getByRole("status")).toBeInTheDocument();
      act(() => {
        jest.advanceTimersByTime(4000);
      });
      expect(screen.queryByRole("status")).not.toBeInTheDocument();
    } finally {
      jest.useRealTimers();
    }
  });

  it("caps the number of simultaneous celebrations", () => {
    render(
      <MoneyMomentsProvider maxVisible={2} durationMs={60000}>
        <Harness payload={{ amount: 5, method: "cash" }} />
      </MoneyMomentsProvider>,
    );
    const button = screen.getByText("pay");
    fireEvent.click(button);
    fireEvent.click(button);
    fireEvent.click(button);
    expect(screen.getAllByTestId("money-moment-amount")).toHaveLength(2);
  });
});
