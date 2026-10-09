import fs from "fs";
import path from "path";

/**
 * Root A: DashboardLayout wraps all dashboard chrome in `relative z-[1]`,
 * creating a stacking context capped at z-index 1. The global top menu is
 * root-level `fixed … z-50`, so every overlay rendered inside the dashboard
 * (KDS z-50, kiosk z-[60], sanitization z-[200], NextUI modals at z-50)
 * paints UNDER the top menu regardless of its internal z.
 *
 * Fix: do not create a z-index stacking context on the main content column.
 * Decorative backgrounds already paint under later siblings without z-[1].
 */
describe("Root A — dashboard stacking context", () => {
  const layoutSrc = fs.readFileSync(
    path.join(
      __dirname,
      "..",
      "components/business/DashboardLayout.tsx",
    ),
    "utf8",
  );

  it("does not wrap the live dashboard content column in relative z-[1]", () => {
    // The shell content column that holds sidebar + tabs + full-screen overlays
    // must not form a z-[1] stacking context (that traps child fixed layers).
    // Match the post-spacer content flex column specifically.
    expect(layoutSrc).not.toMatch(
      /data-testid="dashboard-shell"[\s\S]{0,1500}className="relative z-\[1\] flex h-full flex-col"/,
    );
  });

  it("keeps a relative flex column for the shell content (without z-index)", () => {
    expect(layoutSrc).toMatch(
      /data-testid="dashboard-shell"[\s\S]{0,1500}className="relative flex h-full flex-col"/,
    );
  });
});
