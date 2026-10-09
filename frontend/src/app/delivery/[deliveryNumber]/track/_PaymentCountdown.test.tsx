/** @jest-environment jsdom */
import { render, screen, act } from "@testing-library/react";
import { PaymentCountdown } from "./_PaymentCountdown";

describe("PaymentCountdown", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("renders without throwing and fires onExpired once when expiresAt is already in the past (TDZ regression)", () => {
    const onExpired = jest.fn();
    const past = new Date(Date.now() - 60_000).toISOString();

    // Pre-fix this render threw ReferenceError: Cannot access 'id' before
    // initialization (first tick ran before `const id = setInterval` existed).
    expect(() => render(<PaymentCountdown expiresAt={past} onExpired={onExpired} />)).not.toThrow();

    expect(screen.getByTestId("payment-countdown")).toHaveTextContent("00:00");
    expect(onExpired).toHaveBeenCalledTimes(1);

    // Advancing time must not re-fire (fires at most once per expiresAt).
    act(() => {
      jest.advanceTimersByTime(5_000);
    });
    expect(onExpired).toHaveBeenCalledTimes(1);
  });

  it("counts down and fires onExpired exactly once at zero for a future expiry", () => {
    const onExpired = jest.fn();
    const future = new Date(Date.now() + 3_000).toISOString();
    render(<PaymentCountdown expiresAt={future} onExpired={onExpired} />);

    expect(onExpired).not.toHaveBeenCalled();
    act(() => {
      jest.advanceTimersByTime(4_000);
    });
    expect(screen.getByTestId("payment-countdown")).toHaveTextContent("00:00");
    expect(onExpired).toHaveBeenCalledTimes(1);
  });

  it("formats multi-day windows as Xd Yh instead of raw minutes:seconds", () => {
    const onExpired = jest.fn();
    // ~57d 10h remaining (demo seed payment windows)
    const future = new Date(Date.now() + ((57 * 24 + 10) * 60 + 23) * 60_000).toISOString();
    render(<PaymentCountdown expiresAt={future} onExpired={onExpired} />);
    expect(screen.getByTestId("payment-countdown")).toHaveTextContent("57d 10h");
    expect(screen.getByTestId("payment-countdown")).not.toHaveTextContent(/^\d{3,}:/);
    expect(onExpired).not.toHaveBeenCalled();
  });
});

