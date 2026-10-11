/** @jest-environment jsdom */
import { renderHook } from "@testing-library/react";
import type { DragEndEvent } from "@dnd-kit/core";

const mockReorderMenu = jest.fn().mockResolvedValue({ version: 2 });
jest.mock("../../../../../api/business", () => ({
  businessApi: {
    reorderMenu: (...a: unknown[]) => mockReorderMenu(...a),
  },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: Object.assign(jest.fn(), { error: jest.fn(), success: jest.fn() }),
}));

import { useMenuDragDrop } from "../useMenuDragDrop";

// Common plumbing the hook now requires (CAS version + reload-on-failure).
const mockSetMenuVersion = jest.fn();
const mockLoadMenu = jest.fn().mockResolvedValue(undefined);
const dragDropDeps = {
  menuVersion: 1,
  setMenuVersion: mockSetMenuVersion,
  loadMenu: mockLoadMenu,
  currentViewLanguage: "en",
};

type Cat = {
  id?: string;
  name: string;
  description: string;
  items: { id?: string; name: string; price: number }[];
  sort_order?: number;
};

// Generalized drag-event builder. `type` defaults to "item" so existing item
// tests read unchanged; pass `type: "category"` (and a categoryIndex) to build a
// category→category drag. A mixed pair (active item / over category) is built by
// giving the two sides different types.
function makeDragEvent(
  active: { categoryIndex: number; itemIndex?: number; type?: string },
  over: { categoryIndex: number; itemIndex?: number; type?: string },
): DragEndEvent {
  return {
    active: {
      id: "a",
      data: { current: { type: "item", ...active } },
    },
    over: {
      id: "b",
      data: { current: { type: "item", ...over } },
    },
  } as unknown as DragEndEvent;
}

// Category-drag convenience wrapper — same shape, `type: "category"` on both
// sides so the hook takes the category branch.
function makeCategoryDragEvent(
  activeIndex: number,
  overIndex: number,
): DragEndEvent {
  return makeDragEvent(
    { categoryIndex: activeIndex, type: "category" },
    { categoryIndex: overIndex, type: "category" },
  );
}

beforeEach(() => jest.clearAllMocks());

it("moves the correct item by index even when two items share a name (F21)", () => {
  // Two "Small" items in the same category — name-based resolution would have
  // moved the first match for both source and target.
  const menu: Cat[] = [
    {
      id: "cat-drinks",
      name: "Drinks",
      description: "",
      items: [
        { name: "Small", price: 1 }, // index 0
        { name: "Medium", price: 2 }, // index 1
        { name: "Small", price: 3 }, // index 2 (duplicate name, different price)
      ],
    },
  ];

  let captured: Cat[] | null = null;
  const setMenu = (updater: unknown) => {
    captured = (updater as (m: Cat[]) => Cat[])(menu);
  };

  const { result } = renderHook(() =>
    useMenuDragDrop({ businessId: 1, menu: menu as never, setMenu: setMenu as never, ...dragDropDeps }),
  );

  // Drag the THIRD item (index 2, the second "Small") to position 0.
  result.current.handleDragEnd(
    makeDragEvent({ categoryIndex: 0, itemIndex: 2 }, { categoryIndex: 0, itemIndex: 0 }),
  );

  expect(captured).not.toBeNull();
  const newItems = captured![0].items;
  // The moved item is the price-3 "Small", now first.
  expect(newItems[0].price).toBe(3);
  expect(newItems.map((i) => i.price)).toEqual([3, 1, 2]);
  // Granular reorder: only the single item move is sent (scope item + from/to).
  expect(mockReorderMenu).toHaveBeenCalledWith(1, {
    scope: "item",
    category_id: "cat-drinks",
    from: 2,
    to: 0,
    version: 1,
  });
});

it("reorders items whose name contains a colon (F22)", () => {
  const menu: Cat[] = [
    {
      id: "cat-lunch",
      name: "Lunch: 12-3pm",
      description: "",
      items: [
        { name: "Combo: Burger & Fries", price: 10 }, // index 0
        { name: "Salad", price: 5 }, // index 1
      ],
    },
  ];

  let captured: Cat[] | null = null;
  const setMenu = (updater: unknown) => {
    captured = (updater as (m: Cat[]) => Cat[])(menu);
  };

  const { result } = renderHook(() =>
    useMenuDragDrop({ businessId: 1, menu: menu as never, setMenu: setMenu as never, ...dragDropDeps }),
  );

  // Drag the colon-named combo (index 0) to position 1.
  result.current.handleDragEnd(
    makeDragEvent({ categoryIndex: 0, itemIndex: 0 }, { categoryIndex: 0, itemIndex: 1 }),
  );

  expect(captured).not.toBeNull();
  expect(captured![0].items.map((i) => i.name)).toEqual([
    "Salad",
    "Combo: Burger & Fries",
  ]);
  expect(mockReorderMenu).toHaveBeenCalledWith(1, {
    scope: "item",
    category_id: "cat-lunch",
    from: 0,
    to: 1,
    version: 1,
  });
});

it("persists the reorder under CAS (passes the current version) and syncs the bumped version (R3-MB-1/MB-4)", async () => {
  const menu: Cat[] = [
    {
      id: "cat-drinks",
      name: "Drinks",
      description: "",
      items: [
        { name: "Small", price: 1 },
        { name: "Medium", price: 2 },
      ],
    },
  ];

  const setMenu = (updater: unknown) => {
    (updater as (m: Cat[]) => Cat[])(menu);
  };

  const { result } = renderHook(() =>
    useMenuDragDrop({ businessId: 7, menu: menu as never, setMenu: setMenu as never, ...dragDropDeps }),
  );

  result.current.handleDragEnd(
    makeDragEvent({ categoryIndex: 0, itemIndex: 0 }, { categoryIndex: 0, itemIndex: 1 }),
  );

  // reorderMenu must carry the current version (CAS guard) and the single move.
  expect(mockReorderMenu).toHaveBeenCalledWith(7, {
    scope: "item",
    category_id: "cat-drinks",
    from: 0,
    to: 1,
    version: 1,
  });

  // Let the resolved save promise settle so setMenuVersion runs.
  await Promise.resolve();
  await Promise.resolve();
  expect(mockSetMenuVersion).toHaveBeenCalledWith(2);
});

it("reloads the current view on a failed reorder save (R3-MB-4)", async () => {
  mockReorderMenu.mockRejectedValueOnce(new Error("409"));
  const menu: Cat[] = [
    {
      id: "cat-drinks",
      name: "Drinks",
      description: "",
      items: [
        { name: "Small", price: 1 },
        { name: "Medium", price: 2 },
      ],
    },
  ];
  const setMenu = (updater: unknown) => {
    (updater as (m: Cat[]) => Cat[])(menu);
  };

  const { result } = renderHook(() =>
    useMenuDragDrop({ businessId: 7, menu: menu as never, setMenu: setMenu as never, ...dragDropDeps }),
  );

  result.current.handleDragEnd(
    makeDragEvent({ categoryIndex: 0, itemIndex: 0 }, { categoryIndex: 0, itemIndex: 1 }),
  );

  await Promise.resolve();
  await Promise.resolve();
  expect(mockLoadMenu).toHaveBeenCalledWith("en");
});

it("reorders categories and persists the arrayMoved order under CAS (category drag)", () => {
  const menu: Cat[] = [
    { name: "Starters", description: "", items: [] }, // index 0
    { name: "Mains", description: "", items: [] }, // index 1
    { name: "Desserts", description: "", items: [] }, // index 2
  ];

  let captured: Cat[] | null = null;
  const setMenu = (updater: unknown) => {
    captured = (updater as (m: Cat[]) => Cat[])(menu);
  };

  const { result } = renderHook(() =>
    useMenuDragDrop({ businessId: 9, menu: menu as never, setMenu: setMenu as never, ...dragDropDeps }),
  );

  // Drag "Desserts" (index 2) to the front (index 0).
  result.current.handleDragEnd(makeCategoryDragEvent(2, 0));

  expect(captured).not.toBeNull();
  expect(captured!.map((c) => c.name)).toEqual(["Desserts", "Starters", "Mains"]);
  // Granular reorder: only the category from/to is sent, under the CAS version.
  expect(mockReorderMenu).toHaveBeenCalledWith(9, {
    scope: "category",
    category_id: undefined,
    from: 2,
    to: 0,
    version: 1,
  });
});

it("no-ops a category drag with the same old/new index (no save)", () => {
  const menu: Cat[] = [
    { name: "Starters", description: "", items: [] },
    { name: "Mains", description: "", items: [] },
  ];

  const setMenu = jest.fn();

  const { result } = renderHook(() =>
    useMenuDragDrop({ businessId: 9, menu: menu as never, setMenu: setMenu as never, ...dragDropDeps }),
  );

  result.current.handleDragEnd(makeCategoryDragEvent(1, 1));

  expect(setMenu).not.toHaveBeenCalled();
  expect(mockReorderMenu).not.toHaveBeenCalled();
});

it("ignores a mixed category/item drag pair (no save)", () => {
  const menu: Cat[] = [
    { name: "Starters", description: "", items: [{ name: "Soup", price: 4 }] },
    { name: "Mains", description: "", items: [] },
  ];

  const setMenu = jest.fn();

  const { result } = renderHook(() =>
    useMenuDragDrop({ businessId: 9, menu: menu as never, setMenu: setMenu as never, ...dragDropDeps }),
  );

  // active is a category, over is an item — neither the category branch (needs
  // over.type === "category") nor the item branch (needs active.type === "item")
  // fires, so nothing is moved or saved.
  result.current.handleDragEnd(
    makeDragEvent(
      { categoryIndex: 0, type: "category" },
      { categoryIndex: 1, itemIndex: 0, type: "item" },
    ),
  );

  expect(setMenu).not.toHaveBeenCalled();
  expect(mockReorderMenu).not.toHaveBeenCalled();
});
