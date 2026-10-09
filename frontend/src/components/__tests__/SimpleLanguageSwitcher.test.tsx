/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

// The locale value the mocked provider returns for a given test. Mutable so
// each case can drive a different (or missing) locale without re-mocking.
let mockLocale: unknown = "en";
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale, setLocale: () => undefined }),
  getTranslation: (k: string) => k,
}));

jest.mock("@/store/useUserStore", () => ({
  useUserStore: (selector: (s: { user: null }) => unknown) =>
    selector({ user: null }),
}));

import SimpleLanguageSwitcher from "../SimpleLanguageSwitcher";

beforeEach(() => {
  mockLocale = "en";
});

// The visible LABEL only: the flag span is aria-hidden and would otherwise
// satisfy "non-empty" even with the text label deleted (the exact L9 bug).
function labelText(): string {
  const button = screen.getByRole("button");
  const visible = Array.from(button.querySelectorAll("span")).filter(
    (el) => el.getAttribute("aria-hidden") !== "true",
  );
  return visible.map((el) => el.textContent ?? "").join("").trim();
}

describe("SimpleLanguageSwitcher — pill always shows a visible label", () => {
  it("renders the registered display name for a known operator locale (en)", () => {
    mockLocale = "en";
    render(<SimpleLanguageSwitcher />);
    expect(labelText()).toBe("English");
  });

  it("renders a short es-AR header label so 1024px chrome does not clip (#668)", () => {
    mockLocale = "es-AR";
    render(<SimpleLanguageSwitcher />);
    expect(labelText()).toBe("Español (AR)");
    expect(labelText()).not.toBe("Español (Argentina)");
    const button = screen.getByRole("button");
    expect(button.className).toMatch(/whitespace-nowrap/);
    expect(button.className).toMatch(/shrink-0/);
    expect(button.className).toMatch(/overflow-visible/);
  });

  it("compact es-AR trigger fits the 1024px header rail (#668)", () => {
    mockLocale = "es-AR";
    render(<SimpleLanguageSwitcher compact />);
    expect(labelText()).toBe("ES-AR");
    expect(labelText()).not.toMatch(/Argentina/);
    const button = screen.getByRole("button");
    expect(button.className).toMatch(/overflow-visible/);
  });

  it("renders a non-empty label if the provider contract is violated (undefined locale)", () => {
    mockLocale = undefined;
    render(<SimpleLanguageSwitcher />);
    expect(labelText()).toBe("EN");
  });

  it("renders a non-empty label when the locale is an empty string", () => {
    mockLocale = "";
    render(<SimpleLanguageSwitcher />);
    expect(labelText()).toBe("EN");
  });

  it("falls back to the upper-cased code for a locale missing from languageNames", () => {
    mockLocale = "xx";
    render(<SimpleLanguageSwitcher />);
    expect(labelText()).toBe("XX");
  });

  it("compact variant still exposes a non-empty accessible label", () => {
    mockLocale = undefined;
    render(<SimpleLanguageSwitcher compact />);
    const button = screen.getByRole("button");
    expect(button.getAttribute("aria-label")).toContain("EN");
  });
});

// #593 — sidebar footer compact trigger was an aria-hidden flag emoji with
// no visible language signal. Match the guest selector / Help row: Globe +
// ISO code (EN / ES / ES-AR), never a flag as the only cue.
describe("SimpleLanguageSwitcher — compact trigger is Globe + locale code", () => {
  it.each([
    ["en", "EN"],
    ["es", "ES"],
    ["es-AR", "ES-AR"],
  ] as const)(
    "shows locale %s as visible %s",
    (locale, code) => {
      mockLocale = locale;
      render(<SimpleLanguageSwitcher compact />);
      expect(labelText()).toBe(code);
    },
  );

  it("uses a Globe icon and does not render a flag emoji in the trigger", () => {
    mockLocale = "en";
    render(<SimpleLanguageSwitcher compact />);
    const button = screen.getByRole("button");
    expect(button.querySelector("svg.lucide-globe")).not.toBeNull();
    expect(button.textContent ?? "").not.toMatch(/🇺🇸|🇪🇸|🇦🇷|🌐/);
  });

  it("falls back to a visible EN code when the provider locale is missing", () => {
    mockLocale = undefined;
    render(<SimpleLanguageSwitcher compact />);
    expect(labelText()).toBe("EN");
  });
});
