/**
 * @jest-environment jsdom
 */
import React from "react";
import { render } from "@testing-library/react";
import { FloatingLanguageSelectorBusiness } from "../FloatingLanguageSelectorBusiness";

// Verify the component actually WIRES resolveGuestInitialLanguage into the guest
// provider: on mount it must auto-select the browser-detected language among the
// business's enabled languages and push it via setLanguage. (The resolver itself
// is unit-tested in i18n/__tests__/localeRegistry.test.ts; this guards the
// component integration that was previously untested.)
const mockSetLanguage = jest.fn();
jest.mock("next/navigation", () => ({
  usePathname: () => "/b/demo",
  useRouter: () => ({ replace: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));
jest.mock("../../../i18n/GuestTranslationProvider", () => {
  const actual = jest.requireActual("../../../i18n/GuestTranslationProvider");
  return {
    ...actual,
    useGuestTranslation: () => ({
      t: (k: string) => k,
      currentLanguage: "en",
      setLanguage: mockSetLanguage,
      availableLanguages: actual.GUEST_SUPPORTED_LANGUAGES,
      setBusinessId: jest.fn(),
    }),
  };
});

function setBrowserLanguages(langs: string[]) {
  Object.defineProperty(window.navigator, "language", {
    value: langs[0],
    configurable: true,
  });
  Object.defineProperty(window.navigator, "languages", {
    value: langs,
    configurable: true,
  });
}

const baseProps = {
  businessId: 1,
  supportedLanguages: [],
  variant: "floating" as const,
};

beforeEach(() => {
  mockSetLanguage.mockClear();
  localStorage.clear();
});

test("auto-selects es-AR for an Argentine browser when the business offers it", () => {
  setBrowserLanguages(["es-AR", "es", "en"]);
  render(
    <FloatingLanguageSelectorBusiness
      {...baseProps}
      businessLanguages={[
        { language_code: "en", is_default: true } as any,
        { language_code: "es-AR", is_default: false } as any,
      ]}
    />,
  );
  expect(mockSetLanguage).toHaveBeenCalledWith("es-AR");
});

test("keeps es-AR when the business only enabled en/es (does not collapse to EN or generic es)", () => {
  setBrowserLanguages(["es-AR", "es"]);
  render(
    <FloatingLanguageSelectorBusiness
      {...baseProps}
      businessLanguages={[
        { language_code: "en", is_default: true } as any,
        { language_code: "es", is_default: false } as any,
      ]}
    />,
  );
  expect(mockSetLanguage).toHaveBeenCalledWith("es-AR");
});

test("a saved es-AR preference wins over an English browser when the business only offers en/es", () => {
  localStorage.setItem("guest-language-1", "es-AR");
  setBrowserLanguages(["en-US", "en"]);
  render(
    <FloatingLanguageSelectorBusiness
      {...baseProps}
      businessLanguages={[
        { language_code: "en", is_default: true } as any,
        { language_code: "es", is_default: false } as any,
      ]}
    />,
  );
  expect(mockSetLanguage).toHaveBeenCalledWith("es-AR");
});

test("a saved per-business preference overrides browser detection", () => {
  localStorage.setItem("guest-language-1", "en");
  setBrowserLanguages(["es-AR", "es"]);
  render(
    <FloatingLanguageSelectorBusiness
      {...baseProps}
      businessLanguages={[
        { language_code: "en", is_default: false } as any,
        { language_code: "es-AR", is_default: true } as any,
      ]}
    />,
  );
  expect(mockSetLanguage).toHaveBeenCalledWith("en");
});
