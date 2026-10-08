/** @jest-environment jsdom */
/**
 * Issue #715 — in-app navigation to Despacho renders "could not load the
 * deliveries" while a hard reload of the same URL works, and Reintentar never
 * recovers. Two shipped defects are covered here:
 *
 *  1. The board fetches with an unresolved business id (the dashboard renders
 *     tabs before the business row lands, so the numeric id is briefly 0).
 *     Every sibling delivery read gates on `businessId > 0`; the board did not,
 *     so the id-0 request failed and latched the error board.
 *  2. Reintentar fired a bare `load(false)` with no AbortSignal. An unsignalled
 *     load cannot be recognised as cancelled — `isDispatchLoadCanceled` has no
 *     signal to test — so an in-app abort of the retry surfaced as a board
 *     failure instead of being ignored.
 */
import React from "react";
import { render, screen, act } from "@testing-library/react";
import DispatchConsole from "../DispatchConsole";
import * as deliveryApiModule from "@/api/delivery";

jest.mock("@/api/delivery", () => ({
  ...jest.requireActual("@/api/delivery"),
  deliveryApi: {
    getBusinessDeliveries: jest.fn(),
    getAvailableDrivers: jest.fn(),
    getDeliveryOrder: jest.fn(),
    updateDeliveryOrderStatus: jest.fn(),
    assignDriver: jest.fn(),
    cancelDeliveryOrder: jest.fn(),
    claimDeliveryOrder: jest.fn(),
    releaseDeliveryOrder: jest.fn(),
  },
}));
jest.mock("../useDeliveryQueries", () => ({
  useDeliveryBusiness: () => ({
    data: { default_currency: "USD", timezone: "UTC" },
    isLoading: false,
  }),
}));
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));
jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ retriesExhausted: false, reconnect: jest.fn() }),
}));

const api = deliveryApiModule.deliveryApi as jest.Mocked<
  typeof deliveryApiModule.deliveryApi
>;

const emptyPage = { deliveries: [], total: 0, has_more: false };

const flush = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe("DispatchConsole board load (#715)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("does not fetch or fail the board while the business id is unresolved", async () => {
    (api.getBusinessDeliveries as jest.Mock).mockRejectedValue(
      new Error("business not found"),
    );
    (api.getAvailableDrivers as jest.Mock).mockRejectedValue(
      new Error("business not found"),
    );

    render(<DispatchConsole businessId={0} />);
    await flush();

    expect(api.getBusinessDeliveries).not.toHaveBeenCalled();
    expect(api.getAvailableDrivers).not.toHaveBeenCalled();
    expect(screen.queryByTestId("dispatch-load-error")).toBeNull();
  });

  it("retries through the effect-owned abort scope so an aborted retry is not a board failure", async () => {
    (api.getBusinessDeliveries as jest.Mock).mockRejectedValue(
      new Error("network"),
    );
    (api.getAvailableDrivers as jest.Mock).mockRejectedValue(
      new Error("network"),
    );

    const { rerender } = render(<DispatchConsole businessId={7} />);
    expect(await screen.findByTestId("dispatch-load-error")).toBeInTheDocument();

    // The retry hangs: we hold its rejection until after the board has been
    // re-scoped to another business id (an in-app abort).
    const pendingRejects: Array<(err: unknown) => void> = [];
    (api.getBusinessDeliveries as jest.Mock).mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          pendingRejects.push(reject);
        }),
    );
    (api.getAvailableDrivers as jest.Mock).mockImplementation(
      () => new Promise(() => {}),
    );

    await act(async () => {
      screen.getByText("deliverySettings.dispatch.retry").click();
    });
    await flush();
    expect(pendingRejects.length).toBeGreaterThan(0);

    // The retry must be issued inside the effect's AbortController, not as a
    // bare unsignalled load.
    const retryCall = (api.getBusinessDeliveries as jest.Mock).mock.calls.at(
      -1,
    ) as [number, { signal?: AbortSignal }];
    expect(retryCall[1].signal).toBeInstanceOf(AbortSignal);

    // In-app abort: the board is re-scoped, which aborts the retry.
    (api.getBusinessDeliveries as jest.Mock).mockResolvedValue(emptyPage);
    (api.getAvailableDrivers as jest.Mock).mockResolvedValue([]);
    rerender(<DispatchConsole businessId={9} />);
    await flush();

    // The aborted retry rejects late; it must never repaint the error board.
    await act(async () => {
      pendingRejects.forEach((reject) => reject(new Error("network")));
      await Promise.resolve();
      await Promise.resolve();
    });
    await flush();

    expect(screen.queryByTestId("dispatch-load-error")).toBeNull();
  });
});
