/** @jest-environment jsdom */
/**
 * Round 4 audit — Task 5 coverage.
 *
 * Smoke-tests the shared StatusChip primitive that replaced per-screen
 * colored-pill-only status rendering. We verify three things for every kind:
 *
 *   1. A representative status renders a readable label (color alone is no
 *      longer the only signal).
 *   2. The tone's Tailwind class lands on the pill element.
 *   3. `labelOverride` wins over the internal English default so callers that
 *      already have an i18n helper can keep non-English rendering intact.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { StatusChip } from "../StatusChip";

const TONE_CLASS = {
  success: "border-emerald-200",
  warn: "border-amber-200",
  danger: "border-rose-200",
  info: "border-brand/20",
  neutral: "border-ink-200",
} as const;

function pill(status: string) {
  // The pill is always a <span> with data-status attribute — easy to target
  // without relying on fragile DOM structure.
  return document.querySelector(`[data-status="${status}"]`) as HTMLElement | null;
}

describe("StatusChip", () => {
  describe("kind=bill", () => {
    it("renders open -> info tone with 'Open' label", () => {
      render(<StatusChip kind="bill" status="open" />);
      expect(screen.getByText("Open")).toBeTruthy();
      expect(pill("open")?.className).toContain(TONE_CLASS.info);
    });

    it("renders paid -> success tone with 'Paid' label", () => {
      render(<StatusChip kind="bill" status="paid" />);
      expect(screen.getByText("Paid")).toBeTruthy();
      expect(pill("paid")?.className).toContain(TONE_CLASS.success);
    });

    it("renders closed -> neutral tone with 'Closed' label", () => {
      render(<StatusChip kind="bill" status="closed" />);
      expect(screen.getByText("Closed")).toBeTruthy();
      expect(pill("closed")?.className).toContain(TONE_CLASS.neutral);
    });
  });

  describe("kind=order", () => {
    it("renders approved -> info tone", () => {
      render(<StatusChip kind="order" status="approved" />);
      expect(screen.getByText("Approved")).toBeTruthy();
      expect(pill("approved")?.className).toContain(TONE_CLASS.info);
    });

    it("renders in_kitchen -> warn tone with 'In Kitchen' label", () => {
      render(<StatusChip kind="order" status="in_kitchen" />);
      expect(screen.getByText("In Kitchen")).toBeTruthy();
      expect(pill("in_kitchen")?.className).toContain(TONE_CLASS.warn);
    });

    it("renders ready -> success tone", () => {
      render(<StatusChip kind="order" status="ready" />);
      expect(screen.getByText("Ready")).toBeTruthy();
      expect(pill("ready")?.className).toContain(TONE_CLASS.success);
    });
  });

  describe("kind=table", () => {
    it("renders available -> success tone", () => {
      render(<StatusChip kind="table" status="available" />);
      expect(screen.getByText("Available")).toBeTruthy();
      expect(pill("available")?.className).toContain(TONE_CLASS.success);
    });

    it("renders occupied -> danger tone", () => {
      render(<StatusChip kind="table" status="occupied" />);
      expect(screen.getByText("Occupied")).toBeTruthy();
      expect(pill("occupied")?.className).toContain(TONE_CLASS.danger);
    });

    it("renders reserved -> warn tone", () => {
      render(<StatusChip kind="table" status="reserved" />);
      expect(screen.getByText("Reserved")).toBeTruthy();
      expect(pill("reserved")?.className).toContain(TONE_CLASS.warn);
    });
  });

  describe("kind=reservation", () => {
    it("renders confirmed -> success tone", () => {
      render(<StatusChip kind="reservation" status="confirmed" />);
      expect(screen.getByText("Confirmed")).toBeTruthy();
      expect(pill("confirmed")?.className).toContain(TONE_CLASS.success);
    });

    it("renders pending -> warn tone", () => {
      render(<StatusChip kind="reservation" status="pending" />);
      expect(screen.getByText("Pending")).toBeTruthy();
      expect(pill("pending")?.className).toContain(TONE_CLASS.warn);
    });

    it("renders cancelled -> danger tone", () => {
      render(<StatusChip kind="reservation" status="cancelled" />);
      expect(screen.getByText("Cancelled")).toBeTruthy();
      expect(pill("cancelled")?.className).toContain(TONE_CLASS.danger);
    });

    it("renders late -> warn tone", () => {
      render(<StatusChip kind="reservation" status="late" />);
      expect(screen.getByText("Late")).toBeTruthy();
      expect(pill("late")?.className).toContain(TONE_CLASS.warn);
    });
  });

  describe("generic tone/label API", () => {
    it("renders the label with the tone classes, bypassing kind/status lookup", () => {
      render(<StatusChip tone="warn" label="Trial" />);
      const el = screen.getByText("Trial");
      expect(el).toBeTruthy();
      expect(el.className).toContain(TONE_CLASS.warn);
      // The generic escape hatch carries no domain kind/status attributes.
      expect(el.getAttribute("data-status-tone")).toBe("warn");
      expect(el.getAttribute("data-status")).toBeNull();
      expect(el.getAttribute("data-status-kind")).toBeNull();
    });

    it("renders an info-tone label", () => {
      render(<StatusChip tone="info" label="Active" />);
      const el = screen.getByText("Active");
      expect(el.className).toContain(TONE_CLASS.info);
    });

    it("appends className on the generic chip without clobbering tone classes", () => {
      render(<StatusChip tone="danger" label="Past due" className="extra-x" />);
      const el = screen.getByText("Past due");
      expect(el.className).toContain("extra-x");
      expect(el.className).toContain(TONE_CLASS.danger);
    });

    it("leaves the kind/status path unchanged when tone is not provided", () => {
      render(<StatusChip kind="bill" status="voided" />);
      expect(screen.getByText("Voided")).toBeTruthy();
      expect(pill("voided")?.className).toContain(TONE_CLASS.danger);
    });
  });

  describe("fallbacks", () => {
    it("labelOverride wins over the internal English default", () => {
      render(
        <StatusChip
          kind="bill"
          status="open"
          labelOverride="Abierta"
        />,
      );
      expect(screen.getByText("Abierta")).toBeTruthy();
      // And the internal default is suppressed.
      expect(screen.queryByText("Open")).toBeNull();
    });

    it("unknown status falls back to neutral tone with 'Unknown' label", () => {
      render(<StatusChip kind="bill" status="some_weird_status" />);
      expect(screen.getByText("Unknown")).toBeTruthy();
      expect(pill("some_weird_status")?.className).toContain(TONE_CLASS.neutral);
    });

    it("renders partial bills as warning status instead of unknown", () => {
      render(<StatusChip kind="bill" status="partial" />);
      expect(screen.getByText("Partial")).toBeTruthy();
      expect(pill("partial")?.className).toContain(TONE_CLASS.warn);
    });

    it("appends className onto the pill without clobbering tone classes", () => {
      render(
        <StatusChip
          kind="table"
          status="available"
          className="custom-extra-class"
        />,
      );
      const el = pill("available");
      expect(el?.className).toContain("custom-extra-class");
      expect(el?.className).toContain(TONE_CLASS.success);
    });
  });
});
