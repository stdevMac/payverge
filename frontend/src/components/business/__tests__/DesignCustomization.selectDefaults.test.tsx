/** @jest-environment jsdom */
import React from "react";
import fs from "node:fs";
import path from "node:path";
import { render, screen, within } from "@testing-library/react";
import DesignCustomization from "@/components/business/DesignCustomization";
import type { BusinessDesignSettings } from "@/api/business";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));

const base: BusinessDesignSettings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
  font_family: "Inter",
  theme: "light",
  menu_layout: "grid",
  show_images: true,
  show_descriptions: true,
  header_style: "banner",
  corner_radius: "medium",
  shadow_intensity: "subtle",
  background_pattern: "none",
  pattern_opacity: 0.1,
  hero_layout: "" as any, // missing / empty from legacy rows
  section_density: "comfortable",
};

const DESIGN_SRC = fs.readFileSync(
  path.resolve(__dirname, "../DesignCustomization.tsx"),
  "utf-8",
);

describe("DesignCustomization — select defaults", () => {
  it("Hero layout select has a non-empty default when settings are blank", () => {
    render(
      <DesignCustomization
        businessId={1}
        designSettings={base}
        onDesignSettingsChange={jest.fn()}
      />,
    );
    // Label is "Hero layout" (en). The trigger must not show an empty value.
    const label = screen.getByText(/Hero layout/i);
    const field = label.closest("div") ?? label.parentElement!;
    const trigger = within(field as HTMLElement).getByRole("button");
    expect(trigger.textContent?.trim()).not.toBe("");
    // Default is "Centered" (en) via selectedKeys={[hero_layout || "centered"]}.
    expect(trigger.textContent).toMatch(/centered/i);
  });

  it("hardens design selects: disallowEmptySelection + no empty SelectItem keys + reject empty updates", () => {
    // Parent SelectItem keys were already non-empty; the defect we lock is
    // operators being able to clear a design enum to "". Color text inputs
    // legitimately use value="" while typing — never assert on bare [value=""].
    expect(DESIGN_SRC).toMatch(/disallowEmptySelection/);
    expect(DESIGN_SRC).not.toMatch(/SelectItem\s+key=["']["']/);
    expect(DESIGN_SRC).not.toMatch(/SelectItem\s+key=\{""\}/);
    // updateSetting must refuse empty string so NextUI clear cannot blank enums.
    expect(DESIGN_SRC).toMatch(
      /typeof value === ["']string["']\s*&&\s*value\.trim\(\) === ["']["']/,
    );
    // Runtime: opened listboxes must not expose an empty option key.
    const { container } = render(
      <DesignCustomization
        businessId={1}
        designSettings={{ ...base, hero_layout: "centered" }}
        onDesignSettingsChange={jest.fn()}
      />,
    );
    const emptySelectItems = container.querySelectorAll(
      '[role="option"][data-key=""], [role="option"][value=""], li[data-key=""]',
    );
    expect(emptySelectItems.length).toBe(0);
  });
});
