/**
 * Root A hole 3 (Session O) — commit-time jsdom guards for classes axe
 * structurally cannot (or does not) catch at release:
 *
 *  1. Duplicate accessible names across co-mounted controls (PG-28)
 *  2. Decorative lucide/SVG adjacent to visible text without aria-hidden
 *     (F-cand-7 / Root B) — name injection into the host control
 *  3. Target *spacing* between adjacent interactive targets (RV-3 / SC 2.5.8)
 *     — axe has limited support; CSS gap is the durable signal here
 *
 * These tests exercise real shipped components where possible. Fixture-only
 * cases document the assertion shape so a future contributor can extend them.
 *
 * @jest-environment jsdom
 */

import React from "react";
import { render, within } from "@testing-library/react";
import fs from "fs";
import path from "path";

const FRONTEND_ROOT = path.resolve(__dirname, "..", "..");
const SRC_ROOT = path.join(FRONTEND_ROOT, "src");

function readSrc(relFromSrc: string): string {
  return fs.readFileSync(path.join(SRC_ROOT, relFromSrc), "utf8");
}

/** Collect accessible names of all role=button elements that are not hidden. */
function buttonAccessibleNames(container: HTMLElement): string[] {
  const buttons = within(container).queryAllByRole("button");
  return buttons
    .map((btn) => {
      const labelled =
        btn.getAttribute("aria-label") ||
        btn.getAttribute("aria-labelledby") ||
        btn.textContent ||
        "";
      return labelled.replace(/\s+/g, " ").trim();
    })
    .filter(Boolean);
}

function findDuplicateNames(names: string[]): string[] {
  const seen = new Map<string, number>();
  for (const n of names) {
    seen.set(n, (seen.get(n) ?? 0) + 1);
  }
  return [...seen.entries()]
    .filter(([, count]) => count > 1)
    .map(([name]) => name);
}

describe("Root A — axe-blind class: duplicate accessible names", () => {
  it("detects two co-mounted buttons with the same accessible name (fixture)", () => {
    const { container } = render(
      <div>
        <button type="button">Dividir Cuenta</button>
        <button type="button">Dividir Cuenta</button>
      </div>,
    );
    const names = buttonAccessibleNames(container);
    expect(findDuplicateNames(names)).toContain("Dividir Cuenta");
  });

  it("passes when every visible button name is unique (fixture)", () => {
    const { container } = render(
      <div>
        <button type="button">Split bill</button>
        <button type="button">Confirm split</button>
      </div>,
    );
    expect(findDuplicateNames(buttonAccessibleNames(container))).toEqual([]);
  });
});

describe("Root A — axe-blind class: decorative SVG / lucide adjacent to text", () => {
  /**
   * Rule (Root B): a lucide/SVG icon rendered adjacent to text inside a
   * control or labelled value must be aria-hidden so it does not inject into
   * the accessible name.
   */
  it("fixture: SVG without aria-hidden adjacent to text is a defect", () => {
    const { container } = render(
      <span>
        <svg data-testid="arrow" className="w-4 h-4" />
        +10%
      </span>,
    );
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("aria-hidden")).not.toBe("true");
  });

  it("fixture: SVG with aria-hidden is compliant", () => {
    const { container } = render(
      <span>
        <svg data-testid="arrow" className="w-4 h-4" aria-hidden="true" />
        +10%
      </span>,
    );
    expect(container.querySelector("svg")?.getAttribute("aria-hidden")).toBe(
      "true",
    );
  });

  /**
   * Source-level Root B rule for the two overview files the audit named.
   * Each JSX lucide usage (ArrowUpRight / ArrowDownRight / ArrowRight /
   * Sparkles) that sits on its own opening tag must carry aria-hidden on
   * that same tag. Line-level: the opening tag substring must include
   * aria-hidden.
   */
  it.each([
    "components/business/overview/Metric.tsx",
    "components/business/overview/ProactiveInsights.tsx",
  ] as const)(
    "%s: every decorative lucide opening tag declares aria-hidden",
    (rel) => {
      const src = readSrc(rel);
      // Match self-closing lucide-style tags: <ArrowUpRight ... /> etc.
      const iconTag =
        /<(ArrowUpRight|ArrowDownRight|ArrowRight|Sparkles)\b([^>]*?)\/>/g;
      const missing: string[] = [];
      let m: RegExpExecArray | null;
      while ((m = iconTag.exec(src)) !== null) {
        const attrs = m[2] ?? "";
        if (!/aria-hidden\s*=\s*\{?\s*["']?true/.test(attrs)) {
          missing.push(m[0].replace(/\s+/g, " ").slice(0, 120));
        }
      }
      expect(missing).toEqual([]);
    },
  );
});

describe("Root A — axe-blind class: target spacing (SC 2.5.8 signal)", () => {
  /**
   * SC 2.5.8 spacing clause: adjacent targets need ≥24px undivided space
   * or adequate target size. Tailwind `gap-2` = 8px is the failure the audit
   * measured on the guest menu search row (RV-3). The leaf fix lives in RV-3;
   * here we freeze the assertion helper so commit-time can catch reintroductions.
   */
  function searchFilterGapViolations(src: string): number {
    return [...src.matchAll(/flex items-center gap-2\b/g)].filter((match) => {
      const window = src.slice(
        Math.max(0, match.index! - 80),
        match.index! + 200,
      );
      return /search|filter|Input/i.test(window);
    }).length;
  }

  it("helper flags gap-2 on a search/filters cluster (fixture)", () => {
    const bad = `const row = "flex items-center gap-2"; // search Input Filters`;
    expect(searchFilterGapViolations(bad)).toBeGreaterThan(0);
  });

  it("helper accepts gap-3+ on a search/filters cluster (fixture)", () => {
    const good = `const row = "flex items-center gap-3"; // search Input Filters`;
    expect(searchFilterGapViolations(good)).toBe(0);
  });
});

// Re-export helpers for GuestBill PG-28 suite to import if needed later.
export { buttonAccessibleNames, findDuplicateNames };
