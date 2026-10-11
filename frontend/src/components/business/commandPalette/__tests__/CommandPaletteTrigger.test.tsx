/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import CommandPaletteTrigger from "../CommandPaletteTrigger";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: () => "Search",
}));

jest.mock("../CommandPaletteProvider", () => ({
  useCommandPalette: () => ({ openPalette: jest.fn(), ready: true }),
}));

describe("CommandPaletteTrigger — iconOnly accessibility", () => {
  it("gives the icon-only trigger an accessible name", () => {
    render(<CommandPaletteTrigger iconOnly />);
    expect(screen.getByRole("button", { name: "Search" })).toBeInTheDocument();
  });

  it("keeps an accessible name when responsive CSS hides the default label", () => {
    render(<CommandPaletteTrigger />);
    const btn = screen.getByRole("button");
    expect(btn).toHaveAttribute("aria-label", "Search");
    expect(btn).toHaveAccessibleName("Search");
  });

  it("uses AA text colors for the search label and keyboard hint", () => {
    render(<CommandPaletteTrigger />);
    const button = screen.getByRole("button", { name: "Search" });

    expect(button).toHaveClass("text-ink-500");
    expect(button).not.toHaveClass("text-ink-400");
    expect(button.querySelector("kbd")).toHaveClass("text-ink-500");
  });
});
