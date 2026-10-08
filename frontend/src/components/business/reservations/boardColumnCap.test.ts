/** @jest-environment node */
import {
  capBoardColumnItems,
  RESERVATION_BOARD_COLUMN_CAP,
} from "./boardColumnCap";

describe("capBoardColumnItems", () => {
  it("returns all items when under the cap", () => {
    const items = Array.from({ length: 10 }, (_, i) => i);
    expect(capBoardColumnItems(items, { expanded: false })).toEqual({
      visible: items,
      hiddenCount: 0,
      total: 10,
    });
  });

  it("caps at RESERVATION_BOARD_COLUMN_CAP by default", () => {
    const items = Array.from({ length: 40 }, (_, i) => i);
    const sliced = capBoardColumnItems(items, { expanded: false });
    expect(sliced.visible).toHaveLength(RESERVATION_BOARD_COLUMN_CAP);
    expect(sliced.hiddenCount).toBe(40 - RESERVATION_BOARD_COLUMN_CAP);
    expect(sliced.total).toBe(40);
  });

  it("shows all items when expanded", () => {
    const items = Array.from({ length: 40 }, (_, i) => i);
    const sliced = capBoardColumnItems(items, { expanded: true });
    expect(sliced.visible).toHaveLength(40);
    expect(sliced.hiddenCount).toBe(0);
  });

  it("applies the same cap to completed columns", () => {
    const completed = Array.from({ length: 30 }, (_, i) => `c${i}`);
    const sliced = capBoardColumnItems(completed, {
      expanded: false,
      cap: RESERVATION_BOARD_COLUMN_CAP,
    });
    expect(sliced.visible).toHaveLength(25);
    expect(sliced.hiddenCount).toBe(5);
  });
});
