/** @jest-environment jsdom */
/**
 * #817 — Overview TODAY'S BRIEFINGS, same defect as the Director card.
 *
 * Business 86 has two open checks: bill 761 (~7.39 days) and bill 1143
 * (~1.16 days). The title "{count} bills open longer than {duration}" carries
 * a single age, so it either smears 7 days onto tonight's check or, once the
 * backend narrowed `count` to the duration bucket (f6648da6e), silently drops
 * 1143 and briefs "1 bill". `stale_count` already ships the real total.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import ProactiveInsights from "./ProactiveInsights";

jest.mock("next/link", () => {
  const MockLink = ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  );
  MockLink.displayName = "MockLink";
  return MockLink;
});

const COPY: Record<string, string> = {
  "proactive.types.stale_open_bills.title_one":
    "1 bill open longer than {duration}",
  "proactive.types.stale_open_bills.title_other":
    "{count} bills open longer than {duration}",
  "proactive.types.stale_open_bills.title_mixed":
    "{count} bills open — the oldest {duration}",
  "proactive.types.stale_open_bills.summary":
    "Check if guests left or need a reminder to close out.",
  "proactive.duration.days_one": "{count} day",
  "proactive.duration.days_other": "{count} days",
};

const t = (key: string, params?: Record<string, string | number>) => {
  let value = COPY[key] ?? key;
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      value = value.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
    });
  }
  return value;
};

const DAY = 24 * 60;

function renderInsight(params: Record<string, unknown>) {
  render(
    <ProactiveInsights
      insights={[
        {
          id: "stale-open-bills",
          type: "stale_open_bills",
          params,
          cta: { tab: "bills" },
        },
      ]}
      t={t}
      onOpenConsole={() => {}}
    />,
  );
}

describe("#817 Overview stale-bills title with mixed ages", () => {
  it("briefs the real total and pins the age to the oldest check", () => {
    renderInsight({
      count: 1,
      stale_count: 2,
      oldest_minutes: Math.round(7.39 * DAY),
      threshold_minutes: 120,
    });
    expect(screen.getByText("2 bills open — the oldest 7 days")).toBeInTheDocument();
    expect(screen.queryByText(/2 bills open longer than/)).not.toBeInTheDocument();
    expect(screen.queryByText(/^1 bill/)).not.toBeInTheDocument();
  });

  it("keeps the plain title when every stale bill is in the bucket", () => {
    renderInsight({ count: 2, stale_count: 2, oldest_minutes: 7 * DAY });
    expect(screen.getByText("2 bills open longer than 7 days")).toBeInTheDocument();
  });

  it("keeps the plain title when the backend omits stale_count", () => {
    renderInsight({ count: 2, oldest_minutes: 7 * DAY });
    expect(screen.getByText("2 bills open longer than 7 days")).toBeInTheDocument();
  });

  it("keeps the singular title for a lone stale bill", () => {
    renderInsight({ count: 1, stale_count: 1, oldest_minutes: 7 * DAY });
    expect(screen.getByText("1 bill open longer than 7 days")).toBeInTheDocument();
  });
});

describe("#817 operator-locale contract", () => {
  it("en/es carry title_mixed so the stub above cannot drift from shipped copy", async () => {
    const fs = await import("fs");
    const path = require("path") as typeof import("path");
    for (const locale of ["en", "es"]) {
      const raw = fs.readFileSync(
        path.join(__dirname, "..", "..", "..", "i18n", "messages", locale, "businessDashboard.json"),
        "utf8",
      );
      const node = JSON.parse(raw).overview.proactive.types.stale_open_bills;
      expect(typeof node.title_mixed).toBe("string");
      // The whole point of the key is that it names both numbers separately.
      expect(node.title_mixed).toContain("{count}");
      expect(node.title_mixed).toContain("{duration}");
      // ...and that it never repeats the "open longer than {duration}" claim,
      // which is exactly the smear #817 reported.
      expect(node.title_mixed).not.toMatch(/longer than|desde hace m[aá]s de/);
    }
    // en is the source of truth for the stub used in this file.
    const en = JSON.parse(
      fs.readFileSync(
        path.join(__dirname, "..", "..", "..", "i18n", "messages", "en", "businessDashboard.json"),
        "utf8",
      ),
    ).overview.proactive.types.stale_open_bills;
    expect(COPY["proactive.types.stale_open_bills.title_mixed"]).toBe(en.title_mixed);
    expect(COPY["proactive.types.stale_open_bills.title_other"]).toBe(en.title_other);
    expect(COPY["proactive.types.stale_open_bills.title_one"]).toBe(en.title_one);
  });
});
