/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { ComingSoonBadge } from "../ComingSoonBadge";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string, locale: string) => {
    if (key === "common.comingSoonBadge" && locale === "en") return "Coming soon";
    if (key === "common.comingSoonBadge" && locale === "es") return "Próximamente";
    return key;
  },
}));

describe("ComingSoonBadge", () => {
  it("renders the localized label", () => {
    render(<ComingSoonBadge />);
    expect(screen.getByText("Coming soon")).toBeTruthy();
  });
});
