/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import DeliveryAdmin from "../DeliveryAdmin";
import { resetTestUrl } from "@/test/nextNavigationMock";

// L3-43: the sub-tab lives in `?sub=` via useUrlState, so the static
// next/navigation mock in jest.setup.js can never advance the view. Swap in the
// stateful double that actually round-trips router writes back into
// useSearchParams.
jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);

// Mock DeliverySettings to avoid deep render
jest.mock("@/components/business/DeliverySettings", () => ({
  __esModule: true,
  default: () => <div data-testid="delivery-settings">DeliverySettings</div>,
}));

// Mock DispatchConsole
jest.mock("../DispatchConsole", () => ({
  __esModule: true,
  default: () => <div data-testid="dispatch-console">DispatchConsole</div>,
}));

// Mock DriversManager
jest.mock("../DriversManager", () => ({
  __esModule: true,
  default: () => <div data-testid="drivers-manager">DriversManager</div>,
}));

jest.mock("../DeliveryHistory", () => ({
  __esModule: true,
  default: () => <div data-testid="delivery-history">DeliveryHistory</div>,
}));

// Mock translation
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/components/business/premium", () => {
  const actual = jest.requireActual("@/components/business/premium");
  return {
    ...actual,
    DashboardTabTransition: ({ children }: { children: React.ReactNode }) => (
      <>{children}</>
    ),
  };
});

// Mock tier hook — grant access by default
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));

// Mock locked view and spinner
jest.mock("@/components/business/DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked-view">Locked</div>,
}));

jest.mock("@/components/ui/spinners/PrimarySpinner", () => ({
  PrimarySpinner: () => <div data-testid="spinner">Loading</div>,
}));

describe("DeliveryAdmin", () => {
  beforeEach(() => resetTestUrl("/business/1/dashboard?tab=delivery"));

  it("renders delivery tabs", () => {
    render(<DeliveryAdmin businessId={1} />);

    // Tab labels come from mocked translation (returns the key).
    expect(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.configuration/,
      })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.dispatch/,
      })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.drivers/,
      })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.performance/,
      })
    ).toBeInTheDocument();
  });

  it("shows Configuration content by default", () => {
    render(<DeliveryAdmin businessId={1} />);
    expect(screen.getByTestId("delivery-settings")).toBeInTheDocument();
  });

  it("switching to Dispatch tab shows DispatchConsole", async () => {
    render(<DeliveryAdmin businessId={1} />);

    const dispatchTab = screen.getByRole("tab", {
      name: /deliverySettings\.dispatch\.tabs\.dispatch/,
    });
    fireEvent.click(dispatchTab);

    await waitFor(() =>
      expect(screen.getByTestId("dispatch-console")).toBeInTheDocument()
    );
  });

  it("keeps the highlighted pill in sync with the rendered sub-tab", async () => {
    render(<DeliveryAdmin businessId={1} />);

    fireEvent.click(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.history/,
      }),
    );
    await waitFor(() =>
      expect(screen.getByTestId("delivery-history")).toBeInTheDocument(),
    );
    expect(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.history/,
      }),
    ).toHaveAttribute("aria-selected", "true");
    expect(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.dispatch/,
      }),
    ).toHaveAttribute("aria-selected", "false");

    fireEvent.click(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.dispatch/,
      }),
    );
    await waitFor(() =>
      expect(screen.getByTestId("dispatch-console")).toBeInTheDocument(),
    );
    expect(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.dispatch/,
      }),
    ).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByTestId("delivery-history")).not.toBeInTheDocument();
  });

  it("switching to Drivers tab shows DriversManager", async () => {
    render(<DeliveryAdmin businessId={1} />);

    const driversTab = screen.getByRole("tab", {
      name: /deliverySettings\.dispatch\.tabs\.drivers/,
    });
    fireEvent.click(driversTab);

    await waitFor(() =>
      expect(screen.getByTestId("drivers-manager")).toBeInTheDocument()
    );
  });
});
