/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "Dashboard.tsx"),
  "utf-8",
);

describe("Dashboard lazy-loads chart panels to keep chart.js off first paint (P-4)", () => {
  it("does not statically import the chart-bearing panels", () => {
    expect(SOURCE).not.toMatch(/^import\s+RevenuePanel\s+from\s+["']\.\/panels\/RevenuePanel["'];?\s*$/m);
    expect(SOURCE).not.toMatch(/^import\s+ServiceTipsPanel\s+from\s+["']\.\/panels\/ServiceTipsPanel["'];?\s*$/m);
  });

  it("loads RevenuePanel and ServiceTipsPanel via next/dynamic with ssr:false", () => {
    expect(SOURCE).toMatch(
      /const\s+RevenuePanel\s*=\s*dynamic\(\s*\(\)\s*=>\s*import\(\s*["']\.\/panels\/RevenuePanel["']\s*\)[\s\S]*?ssr:\s*false/,
    );
    expect(SOURCE).toMatch(
      /const\s+ServiceTipsPanel\s*=\s*dynamic\(\s*\(\)\s*=>\s*import\(\s*["']\.\/panels\/ServiceTipsPanel["']\s*\)[\s\S]*?ssr:\s*false/,
    );
  });

  it("keeps the non-chart panels statically imported", () => {
    // TodayLivePanel/MenuPanel/PaymentHistory carry no chart.js, so they stay
    // eager — only the chart panels are deferred.
    expect(SOURCE).toMatch(/^import\s+TodayLivePanel\s+from\s+["']\.\/panels\/TodayLivePanel["'];?\s*$/m);
    expect(SOURCE).toMatch(/^import\s+MenuPanel\s+from\s+["']\.\/panels\/MenuPanel["'];?\s*$/m);
  });
});
