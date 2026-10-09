/** @jest-environment jsdom */
import fs from "fs";
import path from "path";
import React from "react";
import { render } from "@testing-library/react";

import DashboardTabShell from "./DashboardTabShell";
import DashboardTabLoadingSkeleton from "./DashboardTabLoadingSkeleton";
import { AnalyticsSkeleton } from "../AnalyticsSkeleton";
import { CashRegisterSkeleton } from "../CashRegisterSkeleton";
import { CounterSkeleton } from "../CounterSkeleton";
import { InventorySkeleton } from "../InventorySkeleton";
import { ReservationsSkeleton } from "../ReservationsSkeleton";
import { SettingsSkeleton } from "../SettingsSkeleton";
import { TablesSkeleton } from "../TablesSkeleton";
import { TeamSkeleton } from "../TeamSkeleton";
import { MarketingSkeleton } from "../Marketing/MarketingSkeleton";
import { CRMCustomersSkeleton } from "../crm/CRMCustomersSkeleton";
import { MenuLoadingSkeleton } from "../MenuBuilder/MenuLoadingSkeleton";
import { BillsSkeleton } from "../BillsSkeleton";
import { KitchenSkeleton } from "../KitchenSkeleton";
import { DeliverySkeleton } from "../delivery/DeliverySkeleton";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

/**
 * S-9: `DashboardTabShell` keeps the real PageHeader (and sub-tab rail)
 * mounted while loading, and drops the skeleton into the *content slot only*.
 * A skeleton that still paints its own page container or its own header
 * placeholder therefore renders a second grey title bar directly under the
 * live title, inside a second centered canvas — the exact double-chrome the
 * shell was built to delete.
 *
 * The rule: anything handed to `loading` owns no page chrome.
 */

/** Root-level classes that mean "I am a page canvas", not tab content. */
const PAGE_CHROME_CLASS = /^(mx-auto|max-w-\S+|p-[0-9]|(sm|md|lg):p-[0-9])$/;

function chromeClassesOn(el: Element): string[] {
  return Array.from(el.classList).filter((c) => PAGE_CHROME_CLASS.test(c));
}

const SKELETONS: Array<[string, React.ReactElement]> = [
  ["AnalyticsSkeleton", <AnalyticsSkeleton key="a" />],
  ["CashRegisterSkeleton", <CashRegisterSkeleton key="b" />],
  ["CounterSkeleton", <CounterSkeleton key="c" />],
  ["InventorySkeleton", <InventorySkeleton key="d" />],
  ["ReservationsSkeleton", <ReservationsSkeleton key="e" />],
  ["SettingsSkeleton", <SettingsSkeleton key="f" />],
  ["TablesSkeleton", <TablesSkeleton key="g" />],
  ["TeamSkeleton", <TeamSkeleton key="h" />],
  ["MarketingSkeleton", <MarketingSkeleton key="i" />],
  ["CRMCustomersSkeleton", <CRMCustomersSkeleton key="j" />],
  ["MenuLoadingSkeleton", <MenuLoadingSkeleton key="k" />],
  ["BillsSkeleton", <BillsSkeleton key="l" />],
  ["KitchenSkeleton", <KitchenSkeleton key="m" />],
  ["DeliverySkeleton", <DeliverySkeleton key="n" />],
];

describe("loading-slot skeletons own no page container", () => {
  it.each(SKELETONS)("%s renders content-only geometry", (_name, element) => {
    const { container } = render(element);
    const root = container.firstElementChild;
    expect(root).toBeTruthy();
    // Empty array reads better than a boolean when this fails — Jest prints
    // exactly which classes have to go.
    expect(chromeClassesOn(root!)).toEqual([]);
  });
});

describe("loading-slot skeletons paint no second page header", () => {
  it.each(SKELETONS)("%s has no header placeholder", (_name, element) => {
    const { container } = render(element);
    expect(container.querySelector("[data-skeleton-page-header]")).toBeNull();
  });

  it.each(["default", "overview", "table"] as const)(
    "DashboardTabLoadingSkeleton drops chrome for the %s variant in a shell",
    (variant) => {
      const { container } = render(
        <DashboardTabShell header={{ title: "Inventory" }}>
          <DashboardTabLoadingSkeleton variant={variant} withPageChrome={false} />
        </DashboardTabShell>,
      );
      expect(container.querySelector("[data-skeleton-page-header]")).toBeNull();
      const skeleton = container.querySelector('[role="status"]');
      expect(skeleton).toBeTruthy();
      expect(chromeClassesOn(skeleton!)).toEqual([]);
    },
  );

  // Proves the marker above is load-bearing: standalone (no shell) the
  // skeleton IS the page, so it keeps its container and header placeholder.
  it("keeps chrome when rendered standalone", () => {
    const { container } = render(
      <DashboardTabLoadingSkeleton variant="overview" />,
    );
    expect(container.querySelector("[data-skeleton-page-header]")).toBeTruthy();
    const root = container.firstElementChild!;
    expect(chromeClassesOn(root)).toContain("mx-auto");
  });
});

/**
 * The DOM tests above cover the shared skeleton components. Consumers can
 * reintroduce the same defect at the call site by wrapping the skeleton in
 * their own centered canvas before handing it to `loading` — that shape lives
 * in the consumer file, so it needs a consumer-level guard.
 */
describe("shell consumers hand the loading slot bare content", () => {
  const SRC = path.join(__dirname, "..", "..", "..");

  function shellConsumers(): string[] {
    const out: string[] = [];
    const walk = (dir: string) => {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          walk(full);
        } else if (
          entry.name.endsWith(".tsx") &&
          !entry.name.includes(".test.")
        ) {
          const src = fs.readFileSync(full, "utf8");
          if (/import DashboardTabShell from/.test(src)) out.push(full);
        }
      }
    };
    walk(path.join(SRC, "components"));
    return out.sort();
  }

  /** Balanced source from `lines[start]` until its opening brace/paren closes. */
  function balancedFrom(lines: string[], start: number, open: string): string {
    const close = open === "{" ? "}" : ")";
    let depth = 0;
    const chunk: string[] = [];
    for (let j = start; j < lines.length; j += 1) {
      chunk.push(lines[j]);
      depth +=
        (lines[j].split(open).length - 1) - (lines[j].split(close).length - 1);
      if (depth <= 0) break;
    }
    return chunk.join("\n");
  }

  /**
   * Every `loading={...}` JSX prop body in a file, brace-balanced — plus the
   * body of any `const <name> = (…)` the prop hands off to. DeliverySettings
   * builds its skeleton as `loadingView` above the JSX, so a call-site-only
   * scan would read clean while the canvas is one identifier away.
   */
  function loadingBlocks(src: string): string[] {
    const blocks: string[] = [];
    const lines = src.split("\n");
    lines.forEach((line, i) => {
      if (!/^\s*loading=\{/.test(line)) return;
      const block = balancedFrom(lines, i, "{");
      blocks.push(block);
      for (const ref of block.matchAll(/\b([a-z]\w*(?:View|Skeleton|Node))\b/g)) {
        const declared = lines.findIndex((l) =>
          new RegExp(`^\\s*const ${ref[1]} = \\(`).test(l),
        );
        if (declared >= 0) blocks.push(balancedFrom(lines, declared, "("));
      }
    });
    return blocks;
  }

  const consumers = shellConsumers();

  it("finds the shell consumers to scan", () => {
    // A refactor that moves or renames the shell must not silently empty this
    // suite into a green no-op.
    expect(consumers.length).toBeGreaterThan(15);
  });

  it.each(consumers.map((f) => [path.relative(SRC, f), f]))(
    "%s wraps no centered canvas around its skeleton",
    (_rel, file) => {
      for (const block of loadingBlocks(fs.readFileSync(file, "utf8"))) {
        expect(block).not.toMatch(/mx-auto/);
        expect(block).not.toMatch(/p-4 (sm|md):p-6/);
      }
    },
  );

  it.each(consumers.map((f) => [path.relative(SRC, f), f]))(
    "%s turns off page chrome on DashboardTabLoadingSkeleton",
    (_rel, file) => {
      for (const block of loadingBlocks(fs.readFileSync(file, "utf8"))) {
        if (!block.includes("DashboardTabLoadingSkeleton")) continue;
        expect(block).toMatch(/withPageChrome=\{false\}/);
      }
    },
  );

  // The mirror-image defect: instead of drawing a second header, the tab
  // returns its skeleton *ahead* of the shell and the real header vanishes
  // for the whole load. Shell contract rule 1 — "the page header never
  // disappears" — so a tab that owns a shell must route loading through it.
  it.each(consumers.map((f) => [path.relative(SRC, f), f]))(
    "%s routes loading through the shell, not an early return",
    (_rel, file) => {
      const src = fs.readFileSync(file, "utf8");
      const earlyReturns = src.match(/return\s+<\w*Skeleton\b/g) ?? [];
      expect(earlyReturns).toEqual([]);
    },
  );
});
