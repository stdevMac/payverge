/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import SpecialFeaturesEditor from "@/components/business/SpecialFeaturesEditor";
import type { BusinessSpecialFeature } from "@/api/business";
import fs from "node:fs";
import path from "node:path";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

const features = [
  {
    id: 1,
    business_id: 1,
    title: "Free WiFi",
    description: "",
    icon: "wifi",
    display_order: 0,
    is_active: true,
  },
  {
    id: 2,
    business_id: 1,
    title: "Parking",
    description: "",
    icon: "car",
    display_order: 1,
    is_active: true,
  },
  {
    id: 3,
    business_id: 1,
    title: "Live Music",
    description: "",
    icon: "music",
    display_order: 2,
    is_active: true,
  },
] as unknown as BusinessSpecialFeature[];

describe("SpecialFeaturesEditor — icons resolve", () => {
  it("renders a resolved UI icon for every configured feature (data-testid + data-icon)", () => {
    render(
      <SpecialFeaturesEditor
        businessId={1}
        features={features}
        onFeaturesChange={jest.fn()}
      />,
    );

    // Three icon selects (aria-label "Icon").
    const iconSelects = screen.getAllByLabelText(/^icon$/i);
    expect(iconSelects).toHaveLength(3);

    for (const f of features) {
      // Product must surface the selected icon key in the trigger, not a blank
      // unset state. data-testid / data-icon were added for this assertion.
      const wrap = screen.getByTestId(`feature-icon-${f.icon}`);
      expect(wrap).toBeTruthy();
      const glyph = wrap.querySelector(`[data-icon="${f.icon}"]`);
      expect(glyph).not.toBeNull();
      // Lucide SVGs render as <svg>; a real glyph is not an empty wrapper.
      expect(glyph?.tagName.toLowerCase()).toBe("svg");
    }
  });

  it("does not allow clearing the icon select to an empty key", () => {
    render(
      <SpecialFeaturesEditor
        businessId={1}
        features={features}
        onFeaturesChange={jest.fn()}
      />,
    );

    const iconSelects = screen.getAllByLabelText(/^icon$/i);
    for (const select of iconSelects) {
      expect(select.querySelector('[data-key=""]')).toBeNull();
    }

    const src = fs.readFileSync(
      path.resolve(__dirname, "../SpecialFeaturesEditor.tsx"),
      "utf-8",
    );
    expect(src).toMatch(/disallowEmptySelection/);
    expect(src).toMatch(/selectedIcon\.trim\(\)\s*!==\s*["']["']/);
  });
});
