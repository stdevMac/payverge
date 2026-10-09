/** @jest-environment node */
/**
 * L6-8 — Personalizado date commit/debounce policy.
 * RED first when helpers missing; GREEN once PaymentHistory uses them so
 * keystroke intermediates never become query deps.
 */
import fs from "node:fs";
import path from "node:path";
import {
  commitDateRange,
  isCompleteIsoDate,
  DATE_RANGE_DEBOUNCE_MS,
} from "./paymentHistoryDateCommit";

const PAYMENT_HISTORY_SRC = fs.readFileSync(
  path.resolve(__dirname, "./PaymentHistory.tsx"),
  "utf-8",
);

describe("isCompleteIsoDate (L6-8)", () => {
  it("accepts empty and full calendar dates", () => {
    expect(isCompleteIsoDate("")).toBe(true);
    expect(isCompleteIsoDate("2026-03-15")).toBe(true);
  });

  it("rejects intermediate year keystrokes the audit observed (garbage years)", () => {
    // Typing "2026" segment-by-segment can surface 0002 / 0020 / 0202-style values
    // via CalendarDate.toString() before the year is finished.
    expect(isCompleteIsoDate("0002-01-01")).toBe(false);
    expect(isCompleteIsoDate("0020-01-01")).toBe(false);
    expect(isCompleteIsoDate("0202-01-01")).toBe(false);
    expect(isCompleteIsoDate("202")).toBe(false);
    expect(isCompleteIsoDate("2026-0")).toBe(false);
    expect(isCompleteIsoDate("2026-03")).toBe(false);
  });
});

describe("commitDateRange (L6-8)", () => {
  it("does not advance committed start on incomplete intermediate values", () => {
    const prev = { start: "", end: "" };
    // Three intermediate years while typing 2026 — must not become the query.
    const steps = ["0002-01-01", "0020-01-01", "0202-01-01", "2026-01-01"];
    let committed = prev;
    const committedSnapshots: string[] = [];
    for (const start of steps) {
      committed = commitDateRange({ start, end: "" }, committed);
      committedSnapshots.push(committed.start);
    }
    // Only the final complete year commits; intermediates keep previous ("").
    expect(committedSnapshots).toEqual(["", "", "", "2026-01-01"]);
  });

  it("keeps previous committed value while the other side is still typing", () => {
    const committed = commitDateRange(
      { start: "0002-01-01", end: "2026-12-31" },
      { start: "2026-01-01", end: "2026-06-01" },
    );
    expect(committed).toEqual({ start: "2026-01-01", end: "2026-12-31" });
  });
});

describe("debounce constant mirrors search (L6-8)", () => {
  it("uses 350ms so date and search share one cadence", () => {
    expect(DATE_RANGE_DEBOUNCE_MS).toBe(350);
  });
});

/**
 * Call-count gate: if raw intermediates drove the query (unfixed), each step
 * would change deps. With commitDateRange, only complete commits change deps.
 */
describe("call-count under intermediate keystrokes (L6-8)", () => {
  it("only complete commits produce a new query signature", () => {
    const fetchSignatures: string[] = [];
    let committed = { start: "", end: "" };
    // Typing start year only; end stays empty until fully set.
    const startKeystrokes = [
      "0002-01-01",
      "0020-01-01",
      "0202-01-01",
      "2026-01-01", // complete start
    ];
    for (const start of startKeystrokes) {
      const next = commitDateRange({ start, end: "" }, committed);
      const sig = `${next.start}|${next.end}`;
      if (sig !== `${committed.start}|${committed.end}`) {
        fetchSignatures.push(sig);
      }
      committed = next;
    }
    // Unfixed would fetch per intermediate year; fixed is one commit.
    expect(fetchSignatures).toEqual(["2026-01-01|"]);
  });
});

describe("PaymentHistory.tsx wires date commit (L6-8)", () => {
  it("debounces date input into a committed range (not raw onChange → fetch)", () => {
    expect(PAYMENT_HISTORY_SRC).toMatch(
      /from\s+["']\.\/paymentHistoryDateCommit["']/,
    );
    expect(PAYMENT_HISTORY_SRC).toMatch(/DATE_RANGE_DEBOUNCE_MS/);
    expect(PAYMENT_HISTORY_SRC).toMatch(/commitDateRange/);
    // Raw picker state must not be the sole dateRange driving the query.
    expect(PAYMENT_HISTORY_SRC).toMatch(/dateRangeInput/);
  });
});
