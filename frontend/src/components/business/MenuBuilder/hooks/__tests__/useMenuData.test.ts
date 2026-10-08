/** @jest-environment jsdom */
import { renderHook, waitFor, act } from "@testing-library/react";

import { businessApi } from "@/api/business";
import { translateEntireMenu, getTranslationStatus } from "@/api/currency";
import { useMenuData } from "../useMenuData";
import { clearLastGoodOperatorMenu } from "../lastGoodOperatorMenu";

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn(),
    getMenu: jest.fn(),
  },
}));

jest.mock("@/api/currency", () => ({
  getSupportedLanguages: jest.fn().mockResolvedValue([]),
  getBusinessLanguages: jest.fn().mockResolvedValue([]),
  updateBusinessLanguages: jest.fn(),
  translateEntireMenu: jest
    .fn()
    .mockResolvedValue({ job_id: "job-1", status: "processing" }),
  getTranslationStatus: jest
    .fn()
    .mockResolvedValue({ status: "completed", progress: 0, total: 0 }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

const mockGetBusiness = businessApi.getBusiness as jest.Mock;
const mockGetMenu = businessApi.getMenu as jest.Mock;

describe("useMenuData orderability", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearLastGoodOperatorMenu();
    mockGetBusiness.mockResolvedValue({ id: 7, default_currency: "USD" });
    mockGetMenu.mockResolvedValue({
      version: 3,
      categories: [
        {
          name: "Mains",
          description: "",
          items: [
            {
              id: "steak",
              name: "Steak Plate",
              description: "",
              price: 25,
              is_available: true,
            },
          ],
        },
      ],
      item_orderability: {
        steak: { state: "inventory_out", orderable: false },
      },
    });
  });

  it("preserves item_orderability returned by the operator menu endpoint", async () => {
    const { result } = renderHook(() => useMenuData(7));

    await waitFor(() => expect(mockGetMenu).toHaveBeenCalled());
    await waitFor(() =>
      expect(result.current.itemOrderability).toEqual({
        steak: { state: "inventory_out", orderable: false },
      }),
    );
  });
});

// §3.7 fix 1: "Sync all" must start the async translate JOB (not the synchronous
// per-string endpoint) and poll the job status endpoint for progress.
describe("useMenuData keeps last-good catalog on fetch flake (#773)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearLastGoodOperatorMenu();
    mockGetBusiness.mockResolvedValue({ id: 7, default_currency: "USD" });
    const currency = require("@/api/currency");
    currency.getBusinessLanguages.mockResolvedValue([
      { language_code: "en", is_default: true },
    ]);
  });

  it("does not wipe a successful catalog when a later getMenu fails", async () => {
    mockGetMenu
      .mockResolvedValueOnce({
        version: 6,
        parsed_categories: [
          { name: "Mains", items: [{ id: "steak", name: "Steak", price: 25 }] },
        ],
        item_orderability: {},
      })
      .mockRejectedValueOnce(new Error("network blip"));

    const { result } = renderHook(() => useMenuData(7));
    await waitFor(() => expect(result.current.menu[0]?.name).toBe("Mains"));
    expect(result.current.loadFailed).toBe(false);

    await act(async () => {
      await result.current.loadMenu("en");
    });

    expect(result.current.menu[0]?.name).toBe("Mains");
    expect(result.current.menu).toHaveLength(1);
    expect(result.current.loadFailed).toBe(true);
    expect(result.current.error).toBeNull();
  });

  it("marks loadFailed without inventing an empty catalog on first-fetch fail", async () => {
    mockGetMenu.mockRejectedValue(new Error("network blip"));

    const { result } = renderHook(() => useMenuData(7));
    await waitFor(() => expect(result.current.loadFailed).toBe(true));
    expect(result.current.menu).toEqual([]);
    expect(result.current.error).toBeNull();
  });
});

describe("useMenuData sync-all uses the async translate job", () => {
  const mockTranslateEntireMenu = translateEntireMenu as jest.Mock;
  const mockGetTranslationStatus = getTranslationStatus as jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();
    clearLastGoodOperatorMenu();
    (businessApi.getBusiness as jest.Mock).mockResolvedValue({
      id: 7,
      default_currency: "USD",
    });
    (businessApi.getMenu as jest.Mock).mockResolvedValue({
      version: 1,
      parsed_categories: [],
      item_orderability: {},
    });
    // Two configured languages: default en + target es.
    const currency = require("@/api/currency");
    currency.getBusinessLanguages.mockResolvedValue([
      { language_code: "en", is_default: true },
      { language_code: "es", is_default: false },
    ]);
    mockTranslateEntireMenu.mockResolvedValue({
      job_id: "job-xyz",
      status: "processing",
    });
    mockGetTranslationStatus.mockResolvedValue({
      status: "completed",
      progress: 10,
      total: 10,
    });
  });

  it("starts the job with the target language codes and does NOT call the sync endpoint", async () => {
    const { result } = renderHook(() => useMenuData(7));
    await waitFor(() => expect(businessApi.getMenu).toHaveBeenCalled());

    await act(async () => {
      await result.current.handleSyncAllTranslations();
    });

    expect(mockTranslateEntireMenu).toHaveBeenCalledWith(7, ["es"]);
    expect(mockGetTranslationStatus).toHaveBeenCalledWith("job-xyz");
  });
});

describe("useMenuData ignores stale language responses", () => {
  const currency = require("@/api/currency");

  beforeEach(() => {
    jest.clearAllMocks();
    clearLastGoodOperatorMenu();
    mockGetBusiness.mockResolvedValue({ id: 7, default_currency: "USD" });
    currency.getBusinessLanguages.mockResolvedValue([
      { language_code: "en", is_default: true },
      { language_code: "de", is_default: false },
    ]);
  });

  it("does not apply an older Deutsch payload after switching back to English", async () => {
    let resolveDeutsch: (value: unknown) => void = () => undefined;
    const deutschMenu = new Promise((resolve) => {
      resolveDeutsch = resolve;
    });

    mockGetMenu.mockImplementation((_id: number, language?: string) => {
      if (language === "de") {
        return deutschMenu;
      }
      return Promise.resolve({
        language: language || "en",
        version: 1,
        parsed_categories: [{ name: "Mains", items: [] }],
        item_orderability: {},
      });
    });

    const { result } = renderHook(() => useMenuData(7));
    await waitFor(() => expect(result.current.languagesLoaded).toBe(true));
    await waitFor(() => expect(result.current.menu[0]?.name).toBe("Mains"));

    await act(async () => {
      const pendingDeutsch = result.current.loadMenu("de");
      const pendingEnglish = result.current.loadMenu("en");
      resolveDeutsch({
        language: "de",
        version: 2,
        parsed_categories: [{ name: "Hauptgerichte", items: [] }],
        item_orderability: {},
      });
      await Promise.all([pendingDeutsch, pendingEnglish]);
    });

    expect(result.current.menu[0]?.name).toBe("Mains");
    expect(result.current.menu[0]?.name).not.toBe("Hauptgerichte");
  });
});
