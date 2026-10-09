/** @jest-environment node */
/**
 * L3-12 — global bill poll is allowlist-gated.
 *
 * HISTORY: this file previously characterized the *unfixed* denylist
 * `POLL_PAUSED_TABS = {settings, business-page, printers, plugins}`
 * via a source regex (see git history). That characterization locked the
 * defect (menu still polled). Fixing L3-12 correctly turned that assertion red;
 * it was replaced here with an allowlist policy + call-count gate. This is not
 * a regression of a product spec — it is the intentional retirement of a
 * characterization lock.
 */
import fs from "node:fs";
import path from "node:path";
import {
  POLL_ACTIVE_TABS,
  shouldScheduleGlobalBillPoll,
} from "../pollPolicy";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../page.tsx"),
  "utf-8",
);

describe("dashboard 60s poll is allowlist-gated (L3-12 / Root C)", () => {
  it("exports an allowlist of operational tabs that need global polling", () => {
    // Operational floors that consume globalBills / globalOrders / reservations.
    for (const tab of [
      "overview",
      "bills",
      "cash-register",
      "kitchen",
      "counter",
      "tables",
      "reservations",
      "delivery",
    ]) {
      expect(shouldScheduleGlobalBillPoll(tab)).toBe(true);
    }
  });

  it("does not schedule the global poll on Menú or pure-config rails", () => {
    for (const tab of [
      "menu",
      "analytics",
      "accounting",
      "fiscal",
      "settings",
      "business-page",
      "printers",
      "plugins",
      "staff",
      "inventory",
      "marketing",
      "director-console",
      "ai-waiter",
    ]) {
      expect(shouldScheduleGlobalBillPoll(tab)).toBe(false);
    }
  });

  it("page.tsx uses the allowlist helper (not the old POLL_PAUSED_TABS denylist)", () => {
    // Replaced characterization of POLL_PAUSED_TABS — must not reappear.
    expect(SOURCE).not.toMatch(/POLL_PAUSED_TABS/);
    expect(SOURCE).toMatch(/shouldScheduleGlobalBillPoll\s*\(\s*visibleActiveTab\s*\)/);
    expect(SOURCE).toMatch(/from\s+["']\.\/pollPolicy["']/);
  });

  it("on the menu rail, N minutes of wall-clock produce zero loadGlobalBills calls", () => {
    jest.useFakeTimers();
    const loadGlobalBills = jest.fn();
    const POLL_MS = 60_000;
    const WALL_MS = 5 * 60_000; // 5 minutes

    // Mirror the page.tsx effect body: only schedule when the policy allows.
    function scheduleForTab(tab: string) {
      if (!shouldScheduleGlobalBillPoll(tab)) return () => {};
      const id = setInterval(() => {
        loadGlobalBills();
      }, POLL_MS);
      return () => clearInterval(id);
    }

    const cancel = scheduleForTab("menu");
    jest.advanceTimersByTime(WALL_MS);
    cancel();

    expect(loadGlobalBills).toHaveBeenCalledTimes(0);
    // Control: bills rail does schedule.
    loadGlobalBills.mockClear();
    const cancelBills = scheduleForTab("bills");
    jest.advanceTimersByTime(WALL_MS);
    cancelBills();
    expect(loadGlobalBills).toHaveBeenCalledTimes(5);

    jest.useRealTimers();
    // Keep the allowlist set itself honest (not empty / accidental).
    expect(POLL_ACTIVE_TABS.size).toBeGreaterThanOrEqual(5);
  });
});
