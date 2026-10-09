/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }) }));

import DesignCustomization from "@/components/business/DesignCustomization";
import type { BusinessDesignSettings } from "@/api/business";

const settings: BusinessDesignSettings = {
  primary_color: "#1a6b6a", secondary_color: "#2a8b8a", font_family: "Inter", theme: "light",
  menu_layout: "grid", show_images: true, show_descriptions: true, header_style: "banner",
  corner_radius: "medium", shadow_intensity: "subtle", background_pattern: "none",
  pattern_opacity: 0.1, hero_layout: "centered", section_density: "comfortable",
};

it("does not render its own floating Unsaved Changes card (parent SaveBar owns dirty state)", () => {
  render(
    <DesignCustomization
      businessId={1}
      designSettings={settings}
      onDesignSettingsChange={jest.fn()}
    />,
  );
  // Change a color preset to dirty the panel.
  fireEvent.click(screen.getByText(/forest green/i));

  expect(screen.queryByText(/unsaved changes/i)).not.toBeInTheDocument();
});
