/** @jest-environment node */
import * as fs from "fs";
import * as path from "path";

/**
 * Regression-lock (Task 8 follow-up): SSEToastBridge only ever sees events
 * that handleSSEEvent's switch explicitly forwards via
 * `sseEventListenerRef.current?.(event)` — there is no default forwarding.
 * The chat.announcement toast case inside the bridge shipped as dead code
 * because the switch had no `case "chat.announcement"` forwarding it. This
 * parses page.tsx and fails if that forwarding case disappears again.
 *
 * (Source-level guard: this page has no event-routing render harness, and
 * building one is disproportionate to pinning this single routing gap.)
 */
describe("dashboard handleSSEEvent — chat.announcement forwarding", () => {
  it("forwards chat.announcement to the toast bridge listener", () => {
    const src = fs.readFileSync(path.join(__dirname, "..", "page.tsx"), "utf8");

    // The bridge's toast case must exist ...
    expect(src).toMatch(
      /case "chat\.announcement":\s*\{[^}]*newAnnouncementTitle/,
    );

    // ... and handleSSEEvent's switch must actually forward the event to it.
    // Anchor on the case label followed (within its block, allowing comment
    // lines) by the live listener-ref invocation and a break.
    const forwardingCase =
      /case "chat\.announcement":\s*\{(?:\s|\/\/[^\n]*)*(?:void\s+)?sseEventListenerRef\.current\?\.\(event\);\s*break;\s*\}/;
    expect(src).toMatch(forwardingCase);
  });
});

/**
 * Regression-lock (duplicate toasts): domain events that also produce
 * operational alerts (order.created, bill.created, reservation.new, and the
 * payment.received fallback) must NOT be toasted by SSEToastBridge — their
 * toasts are owned by OperationalAlertsProvider via alert.created frames.
 * Same source-level guard rationale as above.
 */
describe("SSEToastBridge — no duplicate toasts for alert-backed events", () => {
  const readBridge = (): string => {
    const src = fs.readFileSync(path.join(__dirname, "..", "page.tsx"), "utf8");
    const start = src.indexOf("function SSEToastBridge");
    expect(start).toBeGreaterThan(-1);
    // The bridge component ends where the next top-level declaration starts.
    const end = src.indexOf("interface BusinessDashboardProps", start);
    return src.slice(start, end === -1 ? undefined : end);
  };

  it.each(["order.created", "bill.created", "reservation.new"])(
    "has no toast case for %s",
    (eventType) => {
      expect(readBridge()).not.toContain(`case "${eventType}"`);
    },
  );

  it("payment.received only celebrates — no fallback toast", () => {
    const bridge = readBridge();
    expect(bridge).toContain('case "payment.received"');
    expect(bridge).toContain("celebrate(event.data)");
    expect(bridge).not.toContain("showSuccess");
  });

  it("keeps the chat.announcement toast (no alert counterpart)", () => {
    expect(readBridge()).toContain('case "chat.announcement"');
  });
});
