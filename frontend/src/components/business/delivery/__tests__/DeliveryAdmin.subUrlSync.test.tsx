/** @jest-environment jsdom */
/**
 * Issue #717 — the Delivery sub-tab pill and `?sub=` fall out of sync.
 *
 * `useUrlState` is scoped with `activeWhen: { key: "tab", values: ["delivery"] }`,
 * and its `setValue` silently returns when that owner param has not settled on
 * delivery yet. The rail flipped optimistically on the click, the URL write was
 * dropped, and nothing ever re-drove it — so the highlighted pill and `?sub=`
 * disagreed for the rest of the session (a reload landed on the other sub-tab).
 *
 * The real `useUrlState` runs here; only `next/navigation` is stubbed.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

const mockReplace = jest.fn();
const mockPush = jest.fn();
let mockSearch = new URLSearchParams();

jest.mock("next/navigation", () => ({
  useSearchParams: () => mockSearch,
  usePathname: () => "/business/demo/dashboard",
  useRouter: () => ({ replace: mockReplace, push: mockPush }),
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
jest.mock("../DriverPerformance", () => ({
  __esModule: true,
  default: () => <div data-testid="driver-performance">DriverPerformance</div>,
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

import DeliveryAdmin from "../DeliveryAdmin";

const dispatchPill = () =>
  screen.getByRole("tab", {
    name: /deliverySettings\.dispatch\.tabs\.dispatch/,
  });

describe("DeliveryAdmin sub-tab URL sync (#717)", () => {
  beforeEach(() => {
    mockReplace.mockClear();
    mockPush.mockClear();
  });

  it("re-drives a dropped ?sub= write once the owner tab param settles", () => {
    // The dashboard has already swapped to Delivery optimistically, but the
    // router has not committed `?tab=delivery` yet.
    mockSearch = new URLSearchParams("tab=orders&sub=history");
    const { rerender } = render(<DeliveryAdmin businessId={1} />);
    expect(screen.getByTestId("delivery-history")).toBeInTheDocument();

    fireEvent.click(dispatchPill());

    // The rail moved to Despacho...
    expect(screen.getByTestId("dispatch-console")).toBeInTheDocument();
    expect(dispatchPill()).toHaveAttribute("aria-selected", "true");
    // ...but the URL write was dropped while `?tab=` was still on orders.
    expect(mockReplace).not.toHaveBeenCalled();

    // The owner param lands. `?sub=` must catch up to the rendered sub-tab.
    mockSearch = new URLSearchParams("tab=delivery&sub=history");
    rerender(<DeliveryAdmin businessId={1} />);

    expect(mockReplace).toHaveBeenCalled();
    const hrefs = mockReplace.mock.calls.map((call) => String(call[0]));
    expect(hrefs.some((href) => href.includes("sub=dispatch"))).toBe(true);
    expect(screen.getByTestId("dispatch-console")).toBeInTheDocument();
    expect(dispatchPill()).toHaveAttribute("aria-selected", "true");
  });

  it("lets an external URL change win over a pending optimistic sub-tab", () => {
    mockSearch = new URLSearchParams("tab=delivery&sub=history");
    const { rerender } = render(<DeliveryAdmin businessId={1} />);

    fireEvent.click(dispatchPill());
    expect(screen.getByTestId("dispatch-console")).toBeInTheDocument();

    // Back button / cross-rail nav lands on drivers instead.
    mockReplace.mockClear();
    mockSearch = new URLSearchParams("tab=delivery&sub=drivers");
    rerender(<DeliveryAdmin businessId={1} />);

    expect(screen.getByTestId("drivers-manager")).toBeInTheDocument();
    expect(dispatchPill()).toHaveAttribute("aria-selected", "false");
    const hrefs = mockReplace.mock.calls.map((call) => String(call[0]));
    expect(hrefs.some((href) => href.includes("sub=dispatch"))).toBe(false);
  });
});
