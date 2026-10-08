/** @jest-environment jsdom */
/**
 * #618 — SENTADOS must never paint a sentence-cased missing-key leaf.
 * Force aging.* lookups through sentenceCaseLeaf (the real getTranslation
 * fallback) and assert the cell still shows a seated-ago time or the
 * hard empty fallback — never "Open check warning" / "Seated unknown".
 */
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  const { sentenceCaseLeaf } = jest.requireActual("@/i18n/sentenceCaseLeaf");
  return {
    ...actual,
    getTranslation: (
      key: string,
      locale?: string,
      params?: Record<string, string | number>,
    ) => {
      if (String(key).includes("aging")) {
        return sentenceCaseLeaf(String(key));
      }
      return actual.getTranslation(key, locale, params);
    },
  };
});

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: React.ReactNode;
    [key: string]: unknown;
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

import { sentenceCaseLeaf } from "@/i18n/sentenceCaseLeaf";
import TableRow from "../TableRow";

const OPEN_CHECK_LEAF = sentenceCaseLeaf("aging.openCheckWarning");
const SEATED_UNKNOWN_LEAF = sentenceCaseLeaf("aging.seatedUnknown");

function renderOccupied(overrides: {
  created_at?: string | null;
  last_seen?: string | null;
} = {}) {
  const createdAt =
    overrides.created_at !== undefined
      ? overrides.created_at
      : new Date(Date.now() - 130 * 60 * 1000).toISOString();
  render(
    <table>
      <tbody>
        <TableRow
          table={{
            id: 9,
            name: "Table 9",
            table_code: "AI-T09",
            is_active: true,
            status: "occupied",
            capacity: 4,
            active_bill: {
              id: 9,
              total: 408.55,
              physical_item_quantity: 0,
              created_at: createdAt,
            },
            server_name: null,
            next_reservation: null,
            last_seen:
              overrides.last_seen !== undefined
                ? overrides.last_seen
                : new Date(Date.now() - 12 * 60 * 1000).toISOString(),
          }}
          onSelect={jest.fn()}
          currency="USD"
        />
      </tbody>
    </table>,
  );
  return screen.getByTestId("table-seated-age");
}

function expectNoAgingLeaf(el: HTMLElement) {
  expect(OPEN_CHECK_LEAF).toBe("Open check warning");
  expect(SEATED_UNKNOWN_LEAF).toBe("Seated unknown");
  expect(el.textContent).not.toBe(OPEN_CHECK_LEAF);
  expect(el.textContent).not.toBe(SEATED_UNKNOWN_LEAF);
  expect(el.getAttribute("title") || "").not.toBe(OPEN_CHECK_LEAF);
  expect(el.getAttribute("title") || "").not.toBe(SEATED_UNKNOWN_LEAF);
  expect(el.getAttribute("aria-label") || "").not.toBe(OPEN_CHECK_LEAF);
}

describe("TableRow seated column — never an aging leaf (#618)", () => {
  it("shows seated-ago time for a long-open check when aging keys are missing", () => {
    const seated = renderOccupied();
    expect(seated.textContent).toMatch(/h ago|min ago|d ago|just now|\d+h/i);
    expectNoAgingLeaf(seated);
  });

  it("uses last_seen when created_at is missing, never an aging leaf", () => {
    const seated = renderOccupied({ created_at: null });
    expect(seated.textContent).toMatch(/12 min ago/i);
    expectNoAgingLeaf(seated);
  });

  it("uses last_seen when created_at is a year-1 sentinel", () => {
    const seated = renderOccupied({
      created_at: "0001-01-01T00:00:00Z",
    });
    expect(seated.textContent).toMatch(/12 min ago/i);
    expectNoAgingLeaf(seated);
  });

  it("falls back to the hard empty mark when every aging key is a leaf and no time exists", () => {
    const seated = renderOccupied({ created_at: null, last_seen: null });
    expect(seated.textContent).toBe("—");
    expectNoAgingLeaf(seated);
  });
});
