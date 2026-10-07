/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});

import SpecialFeaturesEditor from "@/components/business/SpecialFeaturesEditor";
import type { BusinessSpecialFeature } from "@/api/business";

const features = [
  { id: 1, business_id: 1, title: "Free WiFi", description: "", icon: "wifi", display_order: 0, is_active: true },
] as unknown as BusinessSpecialFeature[];

it("toggling the per-feature Visible switch flips is_active in the emitted features", () => {
  const onFeaturesChange = jest.fn();
  render(
    <SpecialFeaturesEditor businessId={1} features={features} onFeaturesChange={onFeaturesChange} />,
  );
  const toggle = screen.getByRole("switch", { name: /visible to customers/i });
  fireEvent.click(toggle);
  const emitted = onFeaturesChange.mock.calls.at(-1)[0];
  expect(emitted[0].is_active).toBe(false);
});
