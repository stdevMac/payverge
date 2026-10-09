/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { useReservationStatus } from "../ReservationToggle";

let resolveSettings: (value: { enabled: boolean }) => void = () => {};

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getSettings: jest.fn(
      () =>
        new Promise<{ enabled: boolean }>((resolve) => {
          resolveSettings = resolve;
        }),
    ),
  },
}));

function Probe({ locked }: { locked: boolean }) {
  const { enabled, loading } = useReservationStatus(85, locked);
  return (
    <div>
      <span data-testid="enabled">{String(enabled)}</span>
      <span data-testid="loading">{String(loading)}</span>
    </div>
  );
}

describe("useReservationStatus first-load gate", () => {
  it("keeps loading true after unlock until settings return enabled", async () => {
    const { rerender } = render(<Probe locked />);
    expect(screen.getByTestId("enabled")).toHaveTextContent("false");
    expect(screen.getByTestId("loading")).toHaveTextContent("false");

    rerender(<Probe locked={false} />);
    // Unknown on this paint — not "paused" — even before the fetch effect runs.
    expect(screen.getByTestId("loading")).toHaveTextContent("true");
    expect(screen.getByTestId("enabled")).toHaveTextContent("false");

    resolveSettings({ enabled: true });
    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
    });
    expect(screen.getByTestId("enabled")).toHaveTextContent("true");
  });

  it("starts loading when first mounted unlocked, before settings resolve", () => {
    render(<Probe locked={false} />);
    expect(screen.getByTestId("loading")).toHaveTextContent("true");
    expect(screen.getByTestId("enabled")).toHaveTextContent("false");
  });
});
