/** @jest-environment jsdom */
/**
 * Regression for the delivery header status chip: DeliveryAdmin's chip comes
 * from useDeliveryStatus while the actual on/off toggle lives inside the
 * Configuration sub-tab (DeliverySettings → DeliveryToggle). A successful
 * toggle must propagate up so the chip flips in place instead of staying
 * stale until the tab remounts.
 */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));

// Stable references — DeliverySettings' load effect depends on showError, so
// fresh fns per render would loop the fetch forever.
jest.mock("@/contexts/ToastContext", () => {
  const toastFns = {
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  };
  return { useToast: () => toastFns };
});

jest.mock("@/api/delivery", () => ({
  deliveryApi: {
    getDeliverySettings: jest.fn(() =>
      Promise.resolve({
        delivery_enabled: true,
        zones: [],
        external_partner_links: [],
      }),
    ),
    updateDeliverySettings: jest.fn(() => Promise.resolve({})),
    getBusinessDeliveries: jest.fn(() =>
      Promise.resolve({ deliveries: [], total: 0, has_more: false }),
    ),
  },
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "USD" })),
}));

// DeliverySettings reads the shared business row through React Query
// (useDeliveryBusiness). Stub the hook so this suite needs no QueryClient.
jest.mock("@/components/business/delivery/useDeliveryQueries", () => ({
  useDeliveryBusiness: () => ({
    data: { default_currency: "USD", timezone: "UTC" },
    isLoading: false,
  }),
}));

// The settings form sections are irrelevant to status syncing — stub them so
// this test exercises only DeliveryAdmin ↔ DeliverySettings ↔ DeliveryToggle.
jest.mock("@/components/business/delivery/ConfigurationSection", () => ({
  ConfigurationSection: () => <div data-testid="configuration-section" />,
}));
// Stub the component but keep the module's pure helpers: DeliverySettings
// imports `ensureZoneClientKeys` from here (L3-38), and a wholesale stub made
// it `undefined` — the load threw, the catch swallowed it, and settings stayed
// at the `delivery_enabled: false` default.
jest.mock("@/components/business/delivery/ZonesSection", () => ({
  ...jest.requireActual("@/components/business/delivery/ZonesSection"),
  ZonesSection: () => <div data-testid="zones-section" />,
}));
jest.mock("@/components/business/delivery/HoursSection", () => ({
  HoursSection: () => <div data-testid="hours-section" />,
}));
jest.mock("@/components/business/delivery/PartnersSection", () => ({
  PartnersSection: () => <div data-testid="partners-section" />,
}));
jest.mock("@/components/business/delivery/CustomerNoteSection", () => ({
  CustomerNoteSection: () => <div data-testid="customer-note-section" />,
}));
jest.mock("@/components/business/delivery/DispatchConsole", () => ({
  __esModule: true,
  default: () => <div data-testid="dispatch-console" />,
}));
jest.mock("@/components/business/delivery/DriversManager", () => ({
  __esModule: true,
  default: () => <div data-testid="drivers-manager" />,
}));
jest.mock("@/components/business/delivery/DriverPerformance", () => ({
  __esModule: true,
  default: () => <div data-testid="driver-performance" />,
}));

import DeliveryAdmin from "@/components/business/delivery/DeliveryAdmin";
import {
  DeliveryToggle,
  useDeliveryStatus,
} from "@/components/business/DeliveryToggle";
import { deliveryApi } from "@/api/delivery";

const updateDeliverySettings = deliveryApi.updateDeliverySettings as jest.Mock;
const getDeliverySettings = deliveryApi.getDeliverySettings as jest.Mock;

function deferred<T = unknown>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function DeliveryStatusProbe({ businessId }: { businessId: number }) {
  const { loading } = useDeliveryStatus(businessId);
  return <div>{loading ? "loading" : "loaded"}</div>;
}

describe("Delivery status async cleanup", () => {
  let consoleErrorSpy: jest.SpyInstance;

  beforeEach(() => {
    consoleErrorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
  });

  it("does not log if useDeliveryStatus rejects after unmount", async () => {
    const pending = deferred();
    getDeliverySettings.mockReturnValueOnce(pending.promise);

    const { unmount } = render(<DeliveryStatusProbe businessId={1} />);
    unmount();

    await act(async () => {
      pending.reject(new Error("late delivery status failure"));
      await pending.promise.catch(() => undefined);
    });

    expect(consoleErrorSpy).not.toHaveBeenCalled();
  });

  it("does not log if DeliveryToggle status load rejects after unmount", async () => {
    const pending = deferred();
    getDeliverySettings.mockReturnValueOnce(pending.promise);

    const { unmount } = render(
      <DeliveryToggle businessId={1} variant="button" />,
    );
    unmount();

    await act(async () => {
      pending.reject(new Error("late toggle status failure"));
      await pending.promise.catch(() => undefined);
    });

    expect(consoleErrorSpy).not.toHaveBeenCalled();
  });
});

describe("DeliveryAdmin status chip sync", () => {
  beforeEach(() => {
    getDeliverySettings.mockImplementation(() =>
      Promise.resolve({
        delivery_enabled: true,
        zones: [],
        external_partner_links: [],
      }),
    );
  });

  it("flips the header chip in place when delivery is disabled from the Configuration tab", async () => {
    render(<DeliveryAdmin businessId={1} />);

    // Header chip resolves from useDeliveryStatus.
    expect(
      await screen.findByText("deliverySettings.focused.status.on"),
    ).toBeInTheDocument();

    // Disable via the toggle inside the (real) DeliverySettings sub-tab.
    fireEvent.click(await screen.findByText("deliveryToggle.button.disable"));
    fireEvent.click(
      await screen.findByText("deliveryToggle.disableModal.buttons.confirm"),
    );
    await waitFor(() => expect(updateDeliverySettings).toHaveBeenCalled());

    // The shared status must reach the shell header — chip flips to off and
    // the activation card takes the toggle's place, no remount required.
    expect(
      await screen.findByText("deliverySettings.focused.status.off"),
    ).toBeInTheDocument();
    expect(screen.queryByText("deliverySettings.focused.status.on")).toBeNull();
    expect(
      screen.getByText("deliveryToggle.activationCard.title"),
    ).toBeInTheDocument();
  });
});
