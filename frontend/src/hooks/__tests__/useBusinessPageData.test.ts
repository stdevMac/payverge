/** @jest-environment jsdom */
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

import { useBusinessPageData, normalizeExternalPartnerLinks } from "@/hooks/useBusinessPageData";
import { guestDeliveryApi } from "@/api/delivery";
import { getBusinessGoogleDetails } from "@/api/googleReviews";
import { getBusinessMenuByCustomUrl } from "@/api/publicBusiness";
import { asDollars } from "@/types/money";
import { logError } from "@/utils/errorLogger";

jest.mock("@/api/delivery");
jest.mock("@/api/publicBusiness", () => ({
  getBusinessMenuByCustomUrl: jest
    .fn()
    .mockResolvedValue({ categories: [], offers: [], bundles: [] }),
}));
jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getSettings: jest
      .fn()
      .mockResolvedValue({ enabled: false, external_partner_links: [] }),
  },
}));
jest.mock("@/api/googleReviews", () => ({
  getBusinessGoogleDetails: jest.fn(),
}));
jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

const mockGet = guestDeliveryApi.getSettings as jest.MockedFunction<
  typeof guestDeliveryApi.getSettings
>;
const mockGetGoogleDetails = getBusinessGoogleDetails as jest.MockedFunction<
  typeof getBusinessGoogleDetails
>;
const mockGetBusinessMenu = getBusinessMenuByCustomUrl as jest.MockedFunction<
  typeof getBusinessMenuByCustomUrl
>;
const mockLogError = logError as jest.MockedFunction<typeof logError>;

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return React.createElement(QueryClientProvider, { client: qc }, children);
};

const baseSettings = {
  business_id: 1,
  delivery_enabled: false,
  in_house_delivery_enabled: false,
  third_party_enabled: false,
  external_partner_links: [],
  zones: [],
  partner_fallback_available: false,
  payment_mode: "cash_on_delivery" as const,
  online_payment_available: false,
  flat_delivery_fee: asDollars(0),
  free_delivery_minimum: asDollars(0),
  minimum_order_amount: asDollars(0),
  estimated_prep_time: 0,
  max_concurrent_deliveries: 0,
  delivery_hours_same_as_business: true,
  auto_assign_drivers: false,
};

const validPartner = {
  name: "Talabat",
  url: "https://talabat.com/x",
  provider_key: "talabat",
};

describe("useBusinessPageData — es-AR menu language", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGet.mockResolvedValue(baseSettings);
  });

  it("requests the guest menu with es-AR (does not collapse to en)", async () => {
    mockGetBusinessMenu.mockResolvedValueOnce({
      categories: [],
      offers: [],
      bundles: [],
    });

    renderHook(() => useBusinessPageData(1, "biz", "es-AR", false, ""), {
      wrapper,
    });

    await waitFor(() => {
      expect(mockGetBusinessMenu).toHaveBeenCalledWith("biz", "es-AR");
    });
  });
});

describe("useBusinessPageData — feature tab readiness", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("stays unready until delivery and reservation settings settle", async () => {
    let resolveDelivery: (value: typeof baseSettings) => void = () => {};
    mockGet.mockReturnValue(
      new Promise<typeof baseSettings>((resolve) => {
        resolveDelivery = resolve;
      }),
    );

    const { result } = renderHook(
      () => useBusinessPageData(1, "biz", "en", false, ""),
      { wrapper },
    );

    expect(result.current.featureTabsReady).toBe(false);

    await act(async () => {
      resolveDelivery(baseSettings);
    });

    await waitFor(() => expect(result.current.featureTabsReady).toBe(true));
    expect(result.current.deliverySettingsLoading).toBe(false);
    expect(result.current.reservationSettingsLoading).toBe(false);
  });
});

describe("useBusinessPageData — delivery flag truth table", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it.each([
    {
      delivery: false,
      inHouse: true,
      thirdParty: true,
      links: [validPartner],
      directExpected: false,
      partnersExpected: 0,
    },
    {
      delivery: true,
      inHouse: false,
      thirdParty: false,
      links: [],
      directExpected: false,
      partnersExpected: 0,
    },
    {
      delivery: true,
      inHouse: true,
      thirdParty: false,
      links: [],
      directExpected: true,
      partnersExpected: 0,
    },
    {
      delivery: true,
      inHouse: false,
      thirdParty: true,
      links: [validPartner],
      directExpected: false,
      partnersExpected: 1,
    },
    {
      delivery: true,
      inHouse: true,
      thirdParty: true,
      links: [validPartner],
      directExpected: true,
      partnersExpected: 1,
    },
  ])(
    "d=$delivery h=$inHouse 3p=$thirdParty -> direct=$directExpected partners=$partnersExpected",
    async (tc) => {
      mockGet.mockResolvedValueOnce({
        ...baseSettings,
        delivery_enabled: tc.delivery,
        in_house_delivery_enabled: tc.inHouse,
        third_party_enabled: tc.thirdParty,
        external_partner_links: tc.links,
      });

      const { result } = renderHook(
        () => useBusinessPageData(1, "biz", "en", false, ""),
        { wrapper },
      );

      await waitFor(() => expect(mockGet).toHaveBeenCalled());
      await waitFor(() => expect(result.current.deliverySettings).not.toBeNull());

      expect(result.current.deliveryEnabled).toBe(tc.directExpected);
      expect(result.current.deliveryPartnerLinks).toHaveLength(tc.partnersExpected);
    },
  );

  it("does not log decorative Google rating failures", async () => {
    mockGet.mockResolvedValueOnce(baseSettings);
    mockGetGoogleDetails.mockRejectedValueOnce(new Error("place details failed"));

    const { result } = renderHook(
      () => useBusinessPageData(1, "biz", "en", true, "place-id"),
      { wrapper },
    );

    await waitFor(() => expect(mockGetGoogleDetails).toHaveBeenCalledWith("biz"));
    await waitFor(() => expect(result.current.deliverySettings).not.toBeNull());

    expect(result.current.googleRating).toBeNull();
    expect(mockLogError).not.toHaveBeenCalledWith(
      expect.anything(),
      "useBusinessPageData",
      "loadGoogleRating",
    );
  });
});

describe("useBusinessPageData — authoritative menu orderability", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGet.mockResolvedValue(baseSettings);
  });

  it("preserves the backend item_orderability projection", async () => {
    mockGetBusinessMenu.mockResolvedValueOnce({
      categories: [
        {
          name: "Mains",
          description: "",
          items: [
            {
              id: "steak",
              name: "Steak Plate",
              description: "",
              price: asDollars(25),
              is_available: true,
            },
          ],
        },
      ],
      menu: {
        item_orderability: {
          steak: { state: "inventory_out", orderable: false },
        },
      },
      offers: [],
      bundles: [],
    });

    const { result } = renderHook(
      () => useBusinessPageData(1, "biz", "en", false, ""),
      { wrapper },
    );

    await waitFor(() =>
      expect(result.current.itemOrderability).toEqual({
        steak: { state: "inventory_out", orderable: false },
      }),
    );
  });

  it("excludes an inventory-blocked item from popular items even when its manual flag is true", async () => {
    mockGetBusinessMenu.mockResolvedValueOnce({
      categories: [
        {
          name: "Mains",
          description: "",
          items: [
            {
              id: "steak",
              name: "Steak Plate",
              description: "",
              price: asDollars(25),
              is_available: true,
            },
          ],
        },
      ],
      menu: {
        item_orderability: {
          steak: { state: "inventory_out", orderable: false },
        },
      },
      offers: [],
      bundles: [],
    });

    const { result } = renderHook(
      () => useBusinessPageData(1, "biz", "en", false, ""),
      { wrapper },
    );

    await waitFor(() => expect(mockGetBusinessMenu).toHaveBeenCalled());
    await waitFor(() => expect(result.current.menuLoading).toBe(false));
    expect(result.current.popularItems).toEqual([]);
  });

  it("refreshes the authoritative projection after the guest cache interval", async () => {
    jest.useFakeTimers();
    mockGetBusinessMenu
      .mockResolvedValueOnce({
        categories: [],
        menu: { item_orderability: { steak: { state: "available", orderable: true } } },
        offers: [],
        bundles: [],
      })
      .mockResolvedValueOnce({
        categories: [],
        menu: { item_orderability: { steak: { state: "inventory_out", orderable: false } } },
        offers: [],
        bundles: [],
      });

    try {
      const { result } = renderHook(
        () => useBusinessPageData(1, "biz", "en", false, ""),
        { wrapper },
      );

      await act(async () => {
        await Promise.resolve();
      });
      expect(mockGetBusinessMenu).toHaveBeenCalledTimes(1);

      await act(async () => {
        jest.advanceTimersByTime(60_000);
        await Promise.resolve();
      });
      await act(async () => {
        jest.advanceTimersByTime(0);
        await Promise.resolve();
      });

      expect(mockGetBusinessMenu).toHaveBeenCalledTimes(2);
      expect(result.current.itemOrderability.steak).toEqual({
        state: "inventory_out",
        orderable: false,
      });
    } finally {
      jest.useRealTimers();
    }
  });

  it("does not schedule menu refetch when pollMenu is false", async () => {
    jest.useFakeTimers();
    mockGetBusinessMenu.mockResolvedValue({
      categories: [],
      offers: [],
      bundles: [],
    });

    try {
      renderHook(
        () =>
          useBusinessPageData(1, "biz", "en", false, "", {
            pollMenu: false,
          }),
        { wrapper },
      );

      await act(async () => {
        await Promise.resolve();
      });
      expect(mockGetBusinessMenu).toHaveBeenCalledTimes(1);

      await act(async () => {
        jest.advanceTimersByTime(15_000);
        await Promise.resolve();
      });

      expect(mockGetBusinessMenu).toHaveBeenCalledTimes(1);
    } finally {
      jest.useRealTimers();
    }
  });

  it("does not mark an initial failed fetch as an authoritative empty snapshot", async () => {
    mockGetBusinessMenu.mockRejectedValueOnce(new Error("temporary network failure"));

    const { result } = renderHook(
      () => useBusinessPageData(1, "biz", "en", false, ""),
      { wrapper },
    );

    await waitFor(() => expect(mockGetBusinessMenu).toHaveBeenCalled());
    await waitFor(() => expect(result.current.menuLoading).toBe(false));
    expect(result.current.menuSnapshotAuthoritative).toBe(false);
  });

  it("retains the last authoritative projection when a background refresh fails", async () => {
    jest.useFakeTimers();
    mockGetBusinessMenu
      .mockResolvedValueOnce({
        categories: [],
        menu: { item_orderability: { steak: { state: "available", orderable: true } } },
        offers: [],
        bundles: [],
      })
      .mockRejectedValueOnce(new Error("temporary refresh failure"));

    try {
      const { result } = renderHook(
        () => useBusinessPageData(1, "biz", "en", false, ""),
        { wrapper },
      );
      await act(async () => {
        await Promise.resolve();
      });
      await act(async () => {
        jest.advanceTimersByTime(0);
        await Promise.resolve();
      });
      expect(result.current.menuSnapshotAuthoritative).toBe(true);

      await act(async () => {
        jest.advanceTimersByTime(60_000);
        await Promise.resolve();
      });
      await act(async () => {
        jest.advanceTimersByTime(0);
        await Promise.resolve();
      });

      expect(mockGetBusinessMenu).toHaveBeenCalledTimes(2);
      expect(result.current.menuSnapshotAuthoritative).toBe(true);
      expect(result.current.itemOrderability.steak).toEqual({
        state: "available",
        orderable: true,
      });
    } finally {
      jest.useRealTimers();
    }
  });
});

describe("normalizeExternalPartnerLinks — unit tests (Task 7)", () => {
  it("drops links with empty url", () => {
    const result = normalizeExternalPartnerLinks([
      { name: "OpenTable", url: "", provider_key: "opentable" },
    ]);
    expect(result).toEqual([]);
  });

  it("drops bare OpenTable/Resy homepages without a venue path", () => {
    const result = normalizeExternalPartnerLinks([
      { name: "OpenTable", url: "https://www.opentable.com", provider_key: "opentable" },
      { name: "OpenTable", url: "https://www.opentable.com/", provider_key: "opentable" },
      { name: "Resy", url: "https://resy.com", provider_key: "resy" },
    ]);
    expect(result).toEqual([]);
  });

  it("drops links with no provider_key and no icon_url", () => {
    const result = normalizeExternalPartnerLinks([
      { name: "Resy", url: "https://resy.com/x" },
    ]);
    expect(result).toEqual([]);
  });

  it("keeps a fully-configured operator link", () => {
    const result = normalizeExternalPartnerLinks([
      { name: "Uber Eats", url: "https://www.ubereats.com/store/foo", provider_key: "ubereats" },
    ]);
    expect(result).toHaveLength(1);
    expect(result[0].url).toBe("https://www.ubereats.com/store/foo");
  });

  it("drops obvious demo-slug placeholder urls", () => {
    const result = normalizeExternalPartnerLinks([
      { name: "OpenTable", url: "https://www.opentable.com/r/acme-demo", provider_key: "opentable" },
      { name: "Resy", url: "https://resy.com/cities/foo/acme-demo", provider_key: "resy" },
    ]);
    expect(result).toEqual([]);
  });

  it("does not block a real slug that starts with demo- (only trailing -demo is blocked)", () => {
    const result = normalizeExternalPartnerLinks([
      { name: "Demo Kitchen", url: "https://www.opentable.com/r/demo-kitchen", provider_key: "opentable" },
    ]);
    expect(result).toHaveLength(1);
  });

  it("drops a -demo slug with a trailing query string", () => {
    const result = normalizeExternalPartnerLinks([
      { name: "UberEats", url: "https://www.ubereats.com/store/mara-core-kitchen-demo?utm_source=test", provider_key: "ubereats" },
    ]);
    expect(result).toEqual([]);
  });
});
