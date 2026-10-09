import {
  filterSlashItems,
  insertSlashSelection,
  isSlashDraft,
  slashQueryFromValue,
} from "../slashPaletteUtils";

describe("slashPaletteUtils (L4-12)", () => {
  it("extracts the query after leading /", () => {
    expect(slashQueryFromValue("/plan")).toBe("plan");
    expect(slashQueryFromValue("  /why was")).toBe("why");
    expect(slashQueryFromValue("hello")).toBeNull();
    expect(isSlashDraft("/x")).toBe(true);
    expect(isSlashDraft("x")).toBe(false);
  });

  it("filters items by query substring", () => {
    const items = ["Plan 7-day AOV", "Why was Tuesday slow?", "Retention"];
    expect(filterSlashItems(items, "")).toEqual(items);
    expect(filterSlashItems(items, "plan")).toEqual(["Plan 7-day AOV"]);
    expect(filterSlashItems(items, "tue")).toEqual(["Why was Tuesday slow?"]);
  });

  it("inserts selection without sending (replaces slash draft)", () => {
    expect(insertSlashSelection("/plan", "Plan 7-day AOV")).toBe(
      "Plan 7-day AOV",
    );
    expect(insertSlashSelection("  /x", "Hello")).toBe("  Hello");
  });
});
