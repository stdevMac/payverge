/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { computeAccessibleName } from "dom-accessibility-api";
import {
  announcedSwitchName,
  labelledByTargetsHaveText,
} from "@/components/ui/namedControl";

let mockLocale = "en";
// logError is fire-and-forget (void) and POSTs to the API; unmocked, its
// console fallback lands after this file's teardown and fails the run.
jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: mockLocale, setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));

import DesignCustomization from "@/components/business/DesignCustomization";
import type { BusinessDesignSettings } from "@/api/business";

const settings: BusinessDesignSettings = {
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
  hero_layout: "centered",
  section_density: "comfortable",
} as unknown as BusinessDesignSettings;

function renderDC() {
  return render(
    <DesignCustomization
      businessId={1}
      designSettings={settings}
      onDesignSettingsChange={jest.fn()}
      customUrl="demo"
    />,
  );
}

describe("DesignCustomization — a11y + i18n polish (Track P / Task P1)", () => {
  beforeEach(() => {
    mockLocale = "en";
  });

  it("A11Y-4: gives each color input an accessible name", () => {
    renderDC();
    // The visible label "Primary Color" fronts BOTH a color picker and a text
    // input, so each must carry its own aria-label to be individually nameable.
    expect(
      screen.getByRole("textbox", { name: /primary color/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /secondary color/i }),
    ).toBeInTheDocument();
    // The native <input type="color"> elements are not exposed as a role, but
    // they must each carry an aria-label naming the swatch.
    const swatches = document.querySelectorAll('input[type="color"]');
    expect(swatches.length).toBe(2);
    swatches.forEach((el) => {
      expect(el.getAttribute("aria-label")).toBeTruthy();
    });
    expect(
      (document.querySelector('input[type="color"][value="#1a6b6a"]') as HTMLElement)
        ?.getAttribute("aria-label"),
    ).toMatch(/primary color/i);
    expect(
      (document.querySelector('input[type="color"][value="#2a8b8a"]') as HTMLElement)
        ?.getAttribute("aria-label"),
    ).toMatch(/secondary color/i);
  });

  it("I18N-9: routes the font-option labels through the translation layer", () => {
    renderDC();
    // The font-family <select> options must use translated labels (resolved via
    // t("fontOptions.*")), not the hardcoded English literals from the
    // FONT_OPTIONS array. NextUI may render the option text in both the trigger
    // and the hidden listbox, so allow >=1 match.
    expect(screen.getAllByText("Sans (DM Sans)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Serif (DM Serif Display)").length).toBeGreaterThan(0);
    // The literal label key must resolve to a non-key string (proves it is
    // routed through getTranslation, not echoing the raw key back).
    expect(
      screen.queryByText("fontOptions.sans"),
    ).toBeNull();
  });

  it("IA-14: uses the brand color token, not text-green-600", () => {
    const { container } = renderDC();
    expect(container.querySelector(".text-green-600")).toBeNull();
    // The Content card icon now uses the brand token.
    expect(container.querySelector(".text-brand")).not.toBeNull();
  });

  it("names content switches and style selects by purpose plus value (#446)", () => {
    mockLocale = "en";
    renderDC();
    const images = screen.getByRole("switch", { name: "Show Item Images" });
    const descriptions = screen.getByRole("switch", {
      name: "Show Descriptions",
    });
    expect(announcedSwitchName(images)).toBe("Show Item Images, on");
    expect(announcedSwitchName(descriptions)).toBe("Show Descriptions, on");
    expect(labelledByTargetsHaveText(images)).toBe(true);
    expect(images.getAttribute("aria-describedby")).toBeTruthy();

    const font = screen.getByRole("button", {
      name: "Font Family, Sans (DM Sans)",
    });
    const menu = screen.getByRole("button", { name: "Menu layout, Grid" });
    const header = screen.getByRole("button", {
      name: "Header style, Banner",
    });
    expect(computeAccessibleName(font)).toBe("Font Family, Sans (DM Sans)");
    expect(computeAccessibleName(menu)).toBe("Menu layout, Grid");
    expect(computeAccessibleName(header)).toBe("Header style, Banner");
    expect(computeAccessibleName(font)).not.toBe("Sans (DM Sans)");
    expect(
      screen.getByRole("button", { name: "Corner Radius, Medium" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Shadow Intensity, Subtle" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Section density, Comfortable" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Hero layout, Centered" }),
    ).toBeInTheDocument();
    expect(labelledByTargetsHaveText(font)).toBe(true);
  });

  it("names Look & feel switches and selects in Spanish (#446)", () => {
    mockLocale = "es";
    renderDC();
    expect(
      screen.getByRole("switch", { name: "Mostrar Imágenes" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "Familia de Fuente, Sans (DM Sans)",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Diseño del menú, Cuadrícula" }),
    ).toBeInTheDocument();
  });
});
