/** @jest-environment jsdom */
import { renderHook, act } from "@testing-library/react";
import { asDollars } from "@/types/money";

const mockGenerateMenuImage = jest.fn();
const mockRegenerateMenuItemImage = jest.fn();
const mockEnhanceMenuItemImage = jest.fn();
const mockAddMenuCategory = jest.fn();
const mockUpdateMenuCategory = jest.fn();
const mockAddMenuItem = jest.fn();
const mockUpdateMenuItem = jest.fn();

const mockToastSuccess = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: (...a: unknown[]) => mockToastSuccess(...a),
    error: jest.fn(),
  },
}));

jest.mock("../../../../../api/business", () => ({
  businessApi: {
    generateMenuImage: (...a: unknown[]) => mockGenerateMenuImage(...a),
    regenerateMenuItemImage: (...a: unknown[]) =>
      mockRegenerateMenuItemImage(...a),
    enhanceMenuItemImage: (...a: unknown[]) => mockEnhanceMenuItemImage(...a),
    addMenuCategory: (...a: unknown[]) => mockAddMenuCategory(...a),
    updateMenuCategory: (...a: unknown[]) => mockUpdateMenuCategory(...a),
    addMenuItem: (...a: unknown[]) => mockAddMenuItem(...a),
    updateMenuItem: (...a: unknown[]) => mockUpdateMenuItem(...a),
  },
}));

import { useMenuMutations } from "../useMenuMutations";

const dailyLimitError = {
  response: {
    status: 429,
    data: {
      code: "image_daily_limit_reached",
      daily_limit: 500,
      resets_in_seconds: 21600,
    },
  },
};

const plain500Error = {
  response: {
    status: 500,
    data: { error: "internal error" },
  },
};

// A 429 from the generic rate limiter, with no `code`. The fair-use banner must
// discriminate on `data.code`, never on the 429 status: matching on status would
// render the banner with a 0-image limit that resets in 0 seconds and swallow the
// generic error toast, because a zeroed DailyLimitInfo object is still truthy.
const bareRateLimit429 = {
  response: {
    status: 429,
    data: { message: "too many requests" },
  },
};

const mockSetError = jest.fn();

function renderMutations() {
  return renderHook(() =>
    useMenuMutations({
      businessId: 7,
      menu: [],
      menuVersion: 1,
      setMenuVersion: jest.fn(),
      setMenu: jest.fn(),
      loadMenu: jest.fn().mockResolvedValue(undefined),
      setError: mockSetError,
      defaultCurrency: "USD",
      tString: (key: string) => key,
    }),
  );
}

beforeEach(() => jest.clearAllMocks());

// ── handleGenerateBreakdown ──────────────────────────────────────────────────

it("handleGenerateBreakdown: 429 daily-limit populates state and does not setError", async () => {
  mockGenerateMenuImage.mockRejectedValueOnce(dailyLimitError);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGenerateBreakdown("Burger", "Beef");
  });

  expect(result.current.dailyLimitReached).toEqual({
    dailyLimit: 500,
    resetsInSeconds: 21600,
  });
  expect(mockSetError).not.toHaveBeenCalled();
});

it("handleGenerateBreakdown: plain 500 leaves state null and calls setError", async () => {
  mockGenerateMenuImage.mockRejectedValueOnce(plain500Error);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGenerateBreakdown("Burger", "Beef");
  });

  expect(result.current.dailyLimitReached).toBeNull();
  expect(mockSetError).toHaveBeenCalled();
});

it("handleGenerateBreakdown: forwards dietary tags and uses description as ingredients (#588)", async () => {
  mockGenerateMenuImage.mockResolvedValueOnce({ url: "https://x/breakdown.jpg" });
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGenerateBreakdown(
      "Harvest Bowl",
      "Roasted vegetables, grains, herbs",
      ["vegetarian"],
    );
  });

  expect(mockGenerateMenuImage).toHaveBeenCalledWith(7, {
    name: "Harvest Bowl",
    description: "Roasted vegetables, grains, herbs",
    ingredients: "Roasted vegetables, grains, herbs",
    dietary_tags: ["vegetarian"],
  });
});

it("handleGenerateBreakdown: subsequent success clears dailyLimitReached", async () => {
  mockGenerateMenuImage
    .mockRejectedValueOnce(dailyLimitError)
    .mockResolvedValueOnce({ url: "https://x/generated.png" });
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGenerateBreakdown("Burger", "Beef");
  });
  expect(result.current.dailyLimitReached).toEqual({
    dailyLimit: 500,
    resetsInSeconds: 21600,
  });

  await act(async () => {
    await result.current.handleGenerateBreakdown("Burger", "Beef");
  });
  expect(result.current.dailyLimitReached).toBeNull();
});

// ── handleGeneratePhoto ──────────────────────────────────────────────────────

it("handleGeneratePhoto: 429 daily-limit populates state and does not setError", async () => {
  mockRegenerateMenuItemImage.mockRejectedValueOnce(dailyLimitError);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGeneratePhoto("Burger", "Beef");
  });

  expect(result.current.dailyLimitReached).toEqual({
    dailyLimit: 500,
    resetsInSeconds: 21600,
  });
  expect(mockSetError).not.toHaveBeenCalled();
});

it("handleGeneratePhoto: plain 500 leaves state null and calls setError", async () => {
  mockRegenerateMenuItemImage.mockRejectedValueOnce(plain500Error);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGeneratePhoto("Burger", "Beef");
  });

  expect(result.current.dailyLimitReached).toBeNull();
  expect(mockSetError).toHaveBeenCalled();
});

it("handleGeneratePhoto: subsequent success clears dailyLimitReached", async () => {
  mockRegenerateMenuItemImage
    .mockRejectedValueOnce(dailyLimitError)
    .mockResolvedValueOnce({ url: "https://x/photo.png", credit: "c1" });
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGeneratePhoto("Burger", "Beef");
  });
  expect(result.current.dailyLimitReached).toEqual({
    dailyLimit: 500,
    resetsInSeconds: 21600,
  });

  await act(async () => {
    await result.current.handleGeneratePhoto("Burger", "Beef");
  });
  expect(result.current.dailyLimitReached).toBeNull();
});

// ── handleEnhancePhoto ───────────────────────────────────────────────────────

it("handleEnhancePhoto: 429 daily-limit populates state and does not setError", async () => {
  mockEnhanceMenuItemImage.mockRejectedValueOnce(dailyLimitError);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleEnhancePhoto(
      "Burger",
      "Beef",
      "https://x/a.png",
    );
  });

  expect(result.current.dailyLimitReached).toEqual({
    dailyLimit: 500,
    resetsInSeconds: 21600,
  });
  expect(mockSetError).not.toHaveBeenCalled();
});

it("handleEnhancePhoto: plain 500 leaves state null and calls setError", async () => {
  mockEnhanceMenuItemImage.mockRejectedValueOnce(plain500Error);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleEnhancePhoto(
      "Burger",
      "Beef",
      "https://x/a.png",
    );
  });

  expect(result.current.dailyLimitReached).toBeNull();
  expect(mockSetError).toHaveBeenCalled();
});

it("handleEnhancePhoto: subsequent success clears dailyLimitReached", async () => {
  mockEnhanceMenuItemImage
    .mockRejectedValueOnce(dailyLimitError)
    .mockResolvedValueOnce({ url: "https://x/photo.png", credit: "c1" });
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleEnhancePhoto(
      "Burger",
      "Beef",
      "https://x/a.png",
    );
  });
  expect(result.current.dailyLimitReached).toEqual({
    dailyLimit: 500,
    resetsInSeconds: 21600,
  });

  await act(async () => {
    await result.current.handleEnhancePhoto(
      "Burger",
      "Beef",
      "https://x/a.png",
    );
  });
  expect(result.current.dailyLimitReached).toBeNull();
});

// ── the code-vs-status discriminator ─────────────────────────────────────────
// readDailyLimit must key off `data.code`, not the 429 status. Matching on
// status shipped undefended once already in this campaign. Every path that can
// raise the fair-use banner needs its own bare-429 test: without one, swapping
// the predicate to `status === 429` leaves the whole suite green.

it("handleGenerateBreakdown: a bare rate-limiter 429 is not the fair-use limit", async () => {
  mockGenerateMenuImage.mockRejectedValueOnce(bareRateLimit429);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGenerateBreakdown("Burger", "Beef");
  });

  expect(result.current.dailyLimitReached).toBeNull();
  expect(mockSetError).toHaveBeenCalled();
});

it("handleGeneratePhoto: a bare rate-limiter 429 is not the fair-use limit", async () => {
  mockRegenerateMenuItemImage.mockRejectedValueOnce(bareRateLimit429);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleGeneratePhoto("Burger", "Beef");
  });

  expect(result.current.dailyLimitReached).toBeNull();
  expect(mockSetError).toHaveBeenCalled();
});

it("handleEnhancePhoto: a bare rate-limiter 429 is not the fair-use limit", async () => {
  mockEnhanceMenuItemImage.mockRejectedValueOnce(bareRateLimit429);
  const { result } = renderMutations();

  await act(async () => {
    await result.current.handleEnhancePhoto(
      "Burger",
      "Beef",
      "https://x/a.png",
    );
  });

  expect(result.current.dailyLimitReached).toBeNull();
  expect(mockSetError).toHaveBeenCalled();
});

it("manual menu writes expose dropped fields and retry only after confirmation", async () => {
  const report = {
    dropped_allergens: 1,
    dropped_dietary_tags: 0,
    dropped_items: 0,
    retained: [],
    dropped: [{
      category_index: 0,
      category_name: "Mains",
      item_index: 0,
      item_name: "Taco",
      field: "allergens",
      value: "unknown",
      reason: "unknown_enum_value",
    }],
  };
  mockAddMenuItem
    .mockResolvedValueOnce({
      message: "review",
      requires_confirmation: true,
      sanitization: report,
    })
    .mockResolvedValueOnce({
      message: "saved",
      version: 2,
      requires_confirmation: false,
      sanitization: report,
      item: { id: "item-2", name: "Taco", price: 12, is_available: true },
    });
  const setMenuVersion = jest.fn();
  const setMenu = jest.fn();
  const { result } = renderHook(() =>
    useMenuMutations({
      businessId: 7,
      menu: [{ id: "cat-1", name: "Mains", description: "", items: [] }],
      menuVersion: 1,
      setMenuVersion,
      setMenu,
      loadMenu: jest.fn().mockResolvedValue(undefined),
      setError: mockSetError,
      defaultCurrency: "USD",
      tString: (key: string) => key,
    }),
  );

  act(() => result.current.setSelectedCategoryIndex(0));
  await act(async () => {
    await result.current.handleAddItem({
      name: "Taco",
      description: "",
      price: asDollars(12),
      is_available: true,
      allergens: ["unknown"],
    });
  });

  expect(result.current.sanitizationReview).toEqual({ kind: "add-item", report });
  expect(mockAddMenuItem).toHaveBeenCalledTimes(1);
  expect(setMenuVersion).not.toHaveBeenCalled();
  expect(setMenu).not.toHaveBeenCalled();

  let confirmedKind: string | null = null;
  await act(async () => {
    confirmedKind = await result.current.confirmPendingSanitization();
  });

  expect(confirmedKind).toBe("add-item");
  expect(mockAddMenuItem).toHaveBeenLastCalledWith(
    7,
    0,
    expect.objectContaining({ name: "Taco" }),
    1,
    "cat-1",
    true,
  );
  expect(result.current.sanitizationReview).toBeNull();
  expect(setMenuVersion).toHaveBeenCalledWith(2);
  expect(setMenu).toHaveBeenCalledTimes(1);
});

function applyMenuUpdater(
  setMenu: jest.Mock,
  current: Array<{ id?: string; name: string; items: Array<Record<string, unknown>> }>,
) {
  const updater = setMenu.mock.calls.at(-1)?.[0];
  if (typeof updater !== "function") {
    throw new Error("expected setMenu to receive a functional updater");
  }
  return updater(current);
}

it("handleToggleEightySix flips availability without opening edit", async () => {
  const steak = {
    id: "steak",
    name: "Steak Plate",
    description: "",
    price: asDollars(42),
    is_available: true,
    currency: "USD",
  };
  const menu = [
    { id: "cat-1", name: "Mains", description: "", items: [steak] },
  ];
  const setMenu = jest.fn();
  const setMenuVersion = jest.fn();
  const loadMenu = jest.fn().mockResolvedValue(undefined);
  mockUpdateMenuItem.mockResolvedValueOnce({
    version: 4,
    requires_confirmation: false,
    item: { ...steak, is_available: false },
  });

  const { result } = renderHook(() =>
    useMenuMutations({
      businessId: 7,
      menu,
      menuVersion: 3,
      setMenuVersion,
      setMenu,
      loadMenu,
      setError: mockSetError,
      defaultCurrency: "USD",
      tString: (key: string) => key,
    }),
  );

  let ok = false;
  await act(async () => {
    ok = Boolean(await result.current.handleToggleEightySix(0, 0));
  });

  expect(ok).toBe(true);
  expect(mockUpdateMenuItem).toHaveBeenCalledWith(
    7,
    0,
    0,
    expect.objectContaining({ id: "steak", is_available: false }),
    3,
    "cat-1",
    "steak",
  );
  expect(setMenuVersion).toHaveBeenCalledWith(4);
  const patched = applyMenuUpdater(setMenu, menu);
  expect(patched[0].items[0].is_available).toBe(false);
  expect(loadMenu).toHaveBeenCalled();
  expect(mockToastSuccess).toHaveBeenCalledWith(
    expect.stringContaining("eightySix.markSuccess"),
  );
  expect(result.current.editingItem).toBeNull();
});

it("handleToggleEightySix restores a manually 86'd dish", async () => {
  const steak = {
    id: "steak",
    name: "Steak Plate",
    description: "",
    price: asDollars(42),
    is_available: false,
    currency: "USD",
  };
  const menu = [
    { id: "cat-1", name: "Mains", description: "", items: [steak] },
  ];
  const setMenu = jest.fn();
  mockUpdateMenuItem.mockResolvedValueOnce({
    version: 5,
    requires_confirmation: false,
    item: { ...steak, is_available: true },
  });

  const { result } = renderHook(() =>
    useMenuMutations({
      businessId: 7,
      menu,
      menuVersion: 4,
      setMenuVersion: jest.fn(),
      setMenu,
      loadMenu: jest.fn().mockResolvedValue(undefined),
      setError: mockSetError,
      defaultCurrency: "USD",
      tString: (key: string) => key,
    }),
  );

  await act(async () => {
    await result.current.handleToggleEightySix(0, 0);
  });

  expect(mockUpdateMenuItem).toHaveBeenCalledWith(
    7,
    0,
    0,
    expect.objectContaining({ id: "steak", is_available: true }),
    4,
    "cat-1",
    "steak",
  );
  const patched = applyMenuUpdater(setMenu, menu);
  expect(patched[0].items[0].is_available).toBe(true);
  expect(mockToastSuccess).toHaveBeenCalledWith(
    expect.stringContaining("eightySix.restoreSuccess"),
  );
});

it("handleToggleEightySix pins the 86 on an inventory-blocked dish [#727]", async () => {
  // Live venue 142 payload: inventory pulled demo-bife, so the operator menu
  // now reports is_available:false (same as guest) while the operator's own
  // switch — manual_available — is still on. Toggling has to read the stored
  // switch: reading the effective flag would send is_available:true and
  // "restore" a dish nobody had 86'd, changing nothing the guest can see.
  const bife = {
    id: "demo-bife",
    name: "Bife de chorizo",
    description: "",
    price: asDollars(240),
    is_available: false,
    manual_available: true,
    orderability_state: "inventory_out" as const,
    inventory_status: "out_of_stock",
    currency: "ARS",
  };
  const menu = [
    { id: "principales", name: "Principales", description: "", items: [bife] },
  ];
  const setMenu = jest.fn();
  mockUpdateMenuItem.mockResolvedValueOnce({
    version: 9,
    requires_confirmation: false,
    item: { ...bife, manual_available: false },
  });

  const { result } = renderHook(() =>
    useMenuMutations({
      businessId: 142,
      menu,
      menuVersion: 8,
      setMenuVersion: jest.fn(),
      setMenu,
      loadMenu: jest.fn().mockResolvedValue(undefined),
      setError: mockSetError,
      defaultCurrency: "ARS",
      tString: (key: string) => key,
    }),
  );

  await act(async () => {
    await result.current.handleToggleEightySix(0, 0);
  });

  expect(mockUpdateMenuItem).toHaveBeenCalledWith(
    142,
    0,
    0,
    expect.objectContaining({
      id: "demo-bife",
      is_available: false,
      manual_available: false,
    }),
    8,
    "principales",
    "demo-bife",
  );
  const patched = applyMenuUpdater(setMenu, menu);
  expect(patched[0].items[0].manual_available).toBe(false);
  expect(patched[0].items[0].is_available).toBe(false);
  expect(mockToastSuccess).toHaveBeenCalledWith(
    expect.stringContaining("eightySix.markSuccess"),
  );
});

it("handleToggleEightySix restores a hand-pulled dish that is also out of stock [#727]", async () => {
  // Both flags off: the operator 86'd it AND inventory is empty. Untoggling
  // must clear only the manual switch — the response still reports the dish
  // unsellable, and the card keeps its inventory badge.
  const bife = {
    id: "demo-bife",
    name: "Bife de chorizo",
    description: "",
    price: asDollars(240),
    is_available: false,
    manual_available: false,
    orderability_state: "inventory_out" as const,
    currency: "ARS",
  };
  const menu = [
    { id: "principales", name: "Principales", description: "", items: [bife] },
  ];
  const setMenu = jest.fn();
  mockUpdateMenuItem.mockResolvedValueOnce({
    version: 10,
    requires_confirmation: false,
    // Backend keeps is_available false: inventory still blocks the dish.
    item: { ...bife, manual_available: true },
  });

  const { result } = renderHook(() =>
    useMenuMutations({
      businessId: 142,
      menu,
      menuVersion: 9,
      setMenuVersion: jest.fn(),
      setMenu,
      loadMenu: jest.fn().mockResolvedValue(undefined),
      setError: mockSetError,
      defaultCurrency: "ARS",
      tString: (key: string) => key,
    }),
  );

  await act(async () => {
    await result.current.handleToggleEightySix(0, 0);
  });

  expect(mockUpdateMenuItem).toHaveBeenCalledWith(
    142,
    0,
    0,
    expect.objectContaining({ manual_available: true, is_available: true }),
    9,
    "principales",
    "demo-bife",
  );
  expect(mockToastSuccess).toHaveBeenCalledWith(
    expect.stringContaining("eightySix.restoreSuccess"),
  );
});
