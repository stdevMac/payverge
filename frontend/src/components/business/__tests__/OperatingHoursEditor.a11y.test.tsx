/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});

import OperatingHoursEditor from "@/components/business/OperatingHoursEditor";

// NextUI's Switch renders the role="switch" element as the <input> and places
// the composed aria-label on the wrapping <label>, so the computed accessible
// name resolves through that label. Assert on the accessible NAME (RTL's a11y
// name computation via getByRole({ name })), not the raw aria-label attribute
// on the input — that is the load-bearing screen-reader property.
const accessibleName = (sw: HTMLElement): string =>
  (sw.getAttribute("aria-label") ||
    sw.closest("label")?.getAttribute("aria-label") ||
    "").trim();

describe("OperatingHoursEditor — a11y (A11Y-7)", () => {
  it("each day's open/closed switch has an accessible name including the day", () => {
    render(<OperatingHoursEditor businessId={1} hours={[]} onHoursChange={jest.fn()} />);
    // Every switch must expose a non-empty accessible name (no nameless switches).
    const switches = screen.getAllByRole("switch");
    expect(switches.length).toBeGreaterThan(0);
    switches.forEach((sw) => {
      expect(accessibleName(sw).length).toBeGreaterThan(0);
    });
    // At least one switch is named for Monday (getDayName(1)) — day context present.
    expect(screen.getAllByRole("switch", { name: /monday/i }).length).toBeGreaterThan(0);
    expect(screen.getAllByLabelText(/monday/i).length).toBeGreaterThan(0);
  });
});
