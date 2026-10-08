/**
 * Parity guard for the ⌘K command palette's TAB_REGISTRY.
 *
 * tabAccess.ts:4-8 states the invariant: the palette and the sidebar must never
 * drift — a tab the sidebar hides can never be offered by the palette, and vice
 * versa. Concretely, the palette's TAB_REGISTRY keys must be exactly the union
 * of the sidebar's PRIMARY_TABS and SECONDARY_TABS: no missing tab (unreachable
 * via ⌘K) and no extra tab (a dead palette row pointing at a nonexistent panel).
 *
 * This test failed before "marketing" / "schedule" were added and "fiscal" was
 * dropped from the registry; it now pins that alignment in place.
 */
import { TAB_REGISTRY_KEYS } from "../CommandPaletteProvider";
import { PRIMARY_TABS, SECONDARY_TABS } from "../../sidebar/sidebarConfig";

describe("command palette TAB_REGISTRY parity", () => {
  it("covers exactly the sidebar's PRIMARY ∪ SECONDARY tabs (no missing, no extra)", () => {
    const registry = new Set(TAB_REGISTRY_KEYS);
    const sidebar = new Set<string>([...PRIMARY_TABS, ...SECONDARY_TABS]);

    // Missing: sidebar tabs the palette cannot reach.
    const missing = [...sidebar].filter((k) => !registry.has(k));
    // Extra: palette rows that no sidebar tab backs.
    const extra = [...registry].filter((k) => !sidebar.has(k));

    expect(missing).toEqual([]);
    expect(extra).toEqual([]);
    // Set-equality (order-independent), matching the invariant exactly.
    expect(registry).toEqual(sidebar);
  });

  it("has no duplicate keys in the registry", () => {
    expect(new Set(TAB_REGISTRY_KEYS).size).toBe(TAB_REGISTRY_KEYS.length);
  });

  it("surfaces marketing, schedule, and the restored fiscal row", () => {
    expect(TAB_REGISTRY_KEYS).toContain("marketing");
    expect(TAB_REGISTRY_KEYS).toContain("schedule");
    // Task 32: fiscal is a first-class Setup tab again.
    expect(TAB_REGISTRY_KEYS).toContain("fiscal");
  });
});
