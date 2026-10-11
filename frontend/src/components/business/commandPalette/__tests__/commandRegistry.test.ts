import {
  buildNavCommands,
  buildRecordCommands,
  scoreCommand,
  rankCommands,
  type NavTabDef,
  type RecordDef,
  type Command,
} from "../commandRegistry";
import type { AccessState } from "../tabAccess";

const FULL: AccessState = {
  loading: false,
  isSuspended: false,
};

const TABS: NavTabDef[] = [
  { key: "overview", label: "Overview", description: "Quick stats", iconKey: "Home" },
  { key: "bills", label: "Bills", description: "Open tabs", iconKey: "Receipt", keywords: ["checks", "tabs"] },
  { key: "kitchen", label: "Kitchen", description: "KDS", iconKey: "ChefHat", keywords: ["kds"] },
  { key: "reservations", label: "Reservations", description: "Bookings", iconKey: "Calendar" },
  { key: "ai-waiter", label: "AI Waiter", description: "Assistant", iconKey: "Bot" },
  { key: "settings", label: "Settings", description: "Configure", iconKey: "Settings" },
];

describe("buildNavCommands", () => {
  it("returns one nav command per visible tab, in input order", () => {
    const cmds = buildNavCommands(TABS, { allowedTabs: [], isStaffUser: false, access: FULL });
    expect(cmds.map((c) => c.tabKey)).toEqual([
      "overview",
      "bills",
      "kitchen",
      "reservations",
      "ai-waiter",
      "settings",
    ]);
    expect(cmds.every((c) => c.kind === "nav" && c.group === "navigate")).toBe(true);
  });

  it("restricts to allowedTabs for staff (no off-limits navigation offered)", () => {
    const cmds = buildNavCommands(TABS, {
      allowedTabs: ["bills", "kitchen"],
      isStaffUser: true,
      access: FULL,
    });
    expect(cmds.map((c) => c.tabKey).sort()).toEqual(["bills", "kitchen"]);
  });

  it("locks every tab but settings while suspended, and hides them for staff", () => {
    const suspended: AccessState = { ...FULL, isSuspended: true };
    const owner = buildNavCommands(TABS, { allowedTabs: [], isStaffUser: false, access: suspended });
    expect(owner.find((c) => c.tabKey === "ai-waiter")?.locked).toBe(true);
    expect(owner.find((c) => c.tabKey === "settings")?.locked).toBe(false);

    const staff = buildNavCommands(TABS, { allowedTabs: [], isStaffUser: true, access: suspended });
    expect(staff.map((c) => c.tabKey)).toEqual(["settings"]);
  });
});

describe("scoreCommand", () => {
  const bills = buildNavCommands(TABS, { allowedTabs: [], isStaffUser: false, access: FULL }).find(
    (c) => c.tabKey === "bills",
  ) as Command;
  const kitchen = buildNavCommands(TABS, { allowedTabs: [], isStaffUser: false, access: FULL }).find(
    (c) => c.tabKey === "kitchen",
  ) as Command;

  it("returns null when the query does not match", () => {
    expect(scoreCommand("zzzzz", bills)).toBeNull();
  });

  it("matches on a keyword alias", () => {
    // "kds" is only a keyword of Kitchen, not in its label.
    expect(scoreCommand("kds", kitchen)).not.toBeNull();
  });

  it("ranks a prefix match above a scattered subsequence", () => {
    const prefix = scoreCommand("bil", bills)!;
    const scattered = scoreCommand("bls", bills)!; // subsequence b-l-s
    expect(prefix).toBeGreaterThan(scattered);
  });
});

describe("rankCommands", () => {
  const all = buildNavCommands(TABS, { allowedTabs: [], isStaffUser: false, access: FULL });

  it("returns every command unchanged for an empty query", () => {
    expect(rankCommands("", all)).toHaveLength(all.length);
    expect(rankCommands("   ", all).map((c) => c.id)).toEqual(all.map((c) => c.id));
  });

  it("drops non-matches and orders the best match first", () => {
    const ranked = rankCommands("res", all);
    expect(ranked[0].tabKey).toBe("reservations");
    expect(ranked.some((c) => c.tabKey === "bills")).toBe(false);
  });

  it("surfaces the exact-prefix tab ahead of incidental matches", () => {
    // "set" is a prefix of Settings; it also appears scattered elsewhere.
    const ranked = rankCommands("set", all);
    expect(ranked[0].tabKey).toBe("settings");
  });
});

describe("buildRecordCommands", () => {
  const RECORDS: RecordDef[] = [
    {
      id: "bill:1",
      tabKey: "bills",
      navSpec: "bills?billId=1",
      label: "Bill #1 · Window",
      iconKey: "Receipt",
      keywords: ["1", "window"],
    },
    {
      id: "menu:burger",
      tabKey: "menu",
      navSpec: "menu?menuSearch=Burger",
      label: "Burger",
      iconKey: "Utensils",
      keywords: ["burger", "dish"],
    },
    {
      id: "staff:9",
      tabKey: "staff",
      navSpec: "staff?sub=people&staffSearch=Ada",
      label: "Ada Lovelace",
      iconKey: "Users",
    },
  ];

  it("indexes records and preserves navSpec deep links", () => {
    const cmds = buildRecordCommands(RECORDS, {
      allowedTabs: [],
      isStaffUser: false,
      access: FULL,
    });
    expect(cmds).toHaveLength(3);
    expect(cmds.every((c) => c.kind === "record" && c.group === "records")).toBe(true);
    expect(cmds.find((c) => c.id === "record:bill:1")?.navSpec).toBe("bills?billId=1");
  });

  it("respects staff allowedTabs so off-limits records never surface", () => {
    const cmds = buildRecordCommands(RECORDS, {
      allowedTabs: ["bills"],
      isStaffUser: true,
      access: FULL,
    });
    expect(cmds.map((c) => c.tabKey)).toEqual(["bills"]);
  });

  it("is searchable by record label via rankCommands", () => {
    const cmds = buildRecordCommands(RECORDS, {
      allowedTabs: [],
      isStaffUser: false,
      access: FULL,
    });
    const ranked = rankCommands("burger", cmds);
    expect(ranked[0]?.label).toBe("Burger");
  });
});
