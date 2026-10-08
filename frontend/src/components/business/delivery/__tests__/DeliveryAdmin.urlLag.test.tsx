/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

const urlState = { sub: "history" as string };

jest.mock("@/hooks/useUrlState", () => ({
  useUrlState: () => [
    urlState.sub,
    (_next: string) => {
      // Simulate router.replace lag: URL stays on Historial after the click.
    },
  ],
}));

jest.mock("@/components/business/DeliverySettings", () => ({
  __esModule: true,
  default: () => <div data-testid="delivery-settings">DeliverySettings</div>,
}));
jest.mock("../DispatchConsole", () => ({
  __esModule: true,
  default: () => <div data-testid="dispatch-console">DispatchConsole</div>,
}));
jest.mock("../DriversManager", () => ({
  __esModule: true,
  default: () => <div data-testid="drivers-manager">DriversManager</div>,
}));
jest.mock("../DeliveryHistory", () => ({
  __esModule: true,
  default: () => <div data-testid="delivery-history">DeliveryHistory</div>,
}));
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
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));
jest.mock("@/components/business/DeliveryToggle", () => ({
  useDeliveryStatus: () => ({
    enabled: true,
    loading: false,
    setEnabled: jest.fn(),
  }),
}));
jest.mock("@/components/business/DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked-view">Locked</div>,
}));
jest.mock("@/components/ui/spinners/PrimarySpinner", () => ({
  PrimarySpinner: () => <div data-testid="spinner">Loading</div>,
}));

import DeliveryAdmin from "../DeliveryAdmin";

describe("DeliveryAdmin URL lag (#717)", () => {
  it("highlights and mounts Dispatch immediately even if ?sub= stays on history", () => {
    urlState.sub = "history";
    render(<DeliveryAdmin businessId={1} />);
    expect(screen.getByTestId("delivery-history")).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.dispatch/,
      }),
    );

    expect(screen.getByTestId("dispatch-console")).toBeInTheDocument();
    expect(screen.queryByTestId("delivery-history")).not.toBeInTheDocument();
    expect(
      screen.getByRole("tab", {
        name: /deliverySettings\.dispatch\.tabs\.dispatch/,
      }),
    ).toHaveAttribute("aria-selected", "true");
  });
});
