/** @jest-environment jsdom */
import React from "react";
import { act, render, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// BusinessOverview now reads the menu-item count via a React Query, so it needs
// a QueryClientProvider. Retries off + gcTime 0 keeps the test deterministic.
const renderWithQuery = (ui: React.ReactElement) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));
jest.mock("@/api/analytics", () => ({
  analyticsApi: { getDashboardSummary: jest.fn() },
}));
jest.mock("@/api/business", () => ({
  businessApi: { getBusinessTables: jest.fn() },
  getMenu: jest.fn(),
}));
jest.mock("@/api/inventory", () => ({ inventoryApi: { getSummary: jest.fn() } }));
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    protected: { getAllPlugins: jest.fn() },
    business: { getBusinessPlugins: jest.fn() },
  },
}));
jest.mock("@/api/directorConsole", () => ({
  getDirectorProactiveInsights: jest.fn(),
}));
// BusinessOverview now loads proactive insights via useProactiveInsights, which
// subscribes to the shared SSE stream. Stub the hook so the test doesn't open a
// real EventSource.
jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));
jest.mock("@/components/common/CurrencyConverter", () => ({
  __esModule: true,
  CurrencyPrice: () => <span />,
}));
jest.mock("../overview/ProactiveInsights", () => ({ __esModule: true, default: () => null }));
jest.mock("../ReservationsTodayCard", () => ({ __esModule: true, default: () => null }));
jest.mock("../staff/LiveTableGrid", () => ({ __esModule: true, default: () => null }));
jest.mock("../onboarding/OnboardingHub", () => ({ __esModule: true, default: () => null }));

import BusinessOverview from "@/components/business/BusinessOverview";
import { analyticsApi } from "@/api/analytics";
import { businessApi, getMenu } from "@/api/business";
import { inventoryApi } from "@/api/inventory";
import { pluginAPI } from "@/api/plugins";
import { getDirectorProactiveInsights } from "@/api/directorConsole";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const fakeBusiness = { id: 42, name: "Verge", default_currency: "USD", display_currency: "USD" } as any;

// Deferred barrier: record each invocation synchronously, then keep every
// response pending until the test releases the shared barrier. This makes the
// concurrency assertion independent of wall-clock scheduling.
function createDeferredBarrier() {
  let releaseBarrier!: () => void;
  const barrier = new Promise<void>((resolve) => {
    releaseBarrier = resolve;
  });
  const invocations: string[] = [];
  const pending: Promise<unknown>[] = [];

  return {
    invocations,
    defer<T>(label: string, value: T) {
      return jest.fn(() => {
        invocations.push(label);
        const response = barrier.then(() => value);
        pending.push(response);
        return response;
      });
    },
    async release() {
      releaseBarrier();
      await Promise.all(pending);
    },
  };
}

it("dispatches menu, inventory, and plugin fetches concurrently", async () => {
  const barrier = createDeferredBarrier();
  (useBusinessAccess as jest.Mock).mockReturnValue({
    access: null, loading: false, hasAccess: true, isSuspended: false, aiConfigured: false,
  });
  (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue({});
  (businessApi.getBusinessTables as jest.Mock).mockResolvedValue({ tables: [] });
  (getDirectorProactiveInsights as jest.Mock).mockResolvedValue({ insights: [] });

  (getMenu as jest.Mock).mockImplementation(
    barrier.defer("menu", { parsed_categories: [] }),
  );
  (inventoryApi.getSummary as jest.Mock).mockImplementation(
    barrier.defer("inventory", {
      out_of_stock_items: 0,
      low_stock_items: 0,
      settings: {},
    }),
  );
  (pluginAPI.protected.getAllPlugins as jest.Mock).mockImplementation(
    barrier.defer("platform plugins", { plugins: [] }),
  );
  (pluginAPI.business.getBusinessPlugins as jest.Mock).mockImplementation(
    barrier.defer("business plugins", { plugins: [] }),
  );

  try {
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);

    await waitFor(() => {
      expect(getMenu).toHaveBeenCalled();
      expect(inventoryApi.getSummary).toHaveBeenCalled();
      expect(pluginAPI.protected.getAllPlugins).toHaveBeenCalled();
      expect(pluginAPI.business.getBusinessPlugins).toHaveBeenCalled();
      expect(barrier.invocations).toEqual(
        expect.arrayContaining([
          "menu",
          "inventory",
          "platform plugins",
          "business plugins",
        ]),
      );
    });

    // No deferred response can resolve before this point, so every invocation
    // above necessarily happened before the barrier was released.
  } finally {
    await act(async () => {
      await barrier.release();
    });
  }
});
