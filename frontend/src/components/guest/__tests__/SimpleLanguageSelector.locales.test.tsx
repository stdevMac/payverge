/** @jest-environment jsdom */
// PG-14: /t language surface must offer the full guest storefront locale set.
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SimpleLanguageSelector } from "../SimpleLanguageSelector";
import { GUEST_SUPPORTED_LANGUAGES } from "@/i18n/GuestTranslationProvider";

const mockSetLanguage = jest.fn();
const mockReplace = jest.fn();
const mockNavState = { currentLanguage: "en" };

jest.mock("next/navigation", () => ({
  usePathname: () => "/t/TABLE-1",
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("../../../api/bills", () => ({
  getBusinessByTableCode: jest.fn(async () => ({
    business: { id: 9 },
    // Business only enables EN/ES — the picker must still list all storefront locales.
    business_languages: [
      { language_code: "en", is_default: true },
      { language_code: "es", is_default: false },
    ],
    supported_languages: [
      { code: "en", name: "English", native_name: "English" },
      { code: "es", name: "Spanish", native_name: "Español" },
    ],
  })),
}));

jest.mock("../../../i18n/GuestTranslationProvider", () => {
  const actual = jest.requireActual("../../../i18n/GuestTranslationProvider");
  return {
    ...actual,
    useGuestTranslation: () => ({
      t: (k: string) => k,
      get currentLanguage() {
        return mockNavState.currentLanguage;
      },
      setLanguage: mockSetLanguage,
      availableLanguages: actual.GUEST_SUPPORTED_LANGUAGES,
      setBusinessId: jest.fn(),
    }),
  };
});

describe("SimpleLanguageSelector full guest locale set (PG-14)", () => {
  beforeEach(() => {
    mockSetLanguage.mockClear();
    mockReplace.mockClear();
    localStorage.clear();
    mockNavState.currentLanguage = "en";
  });

  it("lists every storefront locale even when the business only offers EN/ES", async () => {
    const user = userEvent.setup();
    render(<SimpleLanguageSelector tableCode="TABLE-1" />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /languageSelector\.changeLanguage|change language/i }),
      ).toBeInTheDocument();
    });

    await user.click(
      screen.getByRole("button", { name: /languageSelector\.changeLanguage|change language/i }),
    );

    const storefrontCount = Object.keys(GUEST_SUPPORTED_LANGUAGES).length;
    expect(storefrontCount).toBeGreaterThanOrEqual(21);

    // Each storefront code appears as a mono chip button in the modal.
    for (const code of Object.keys(GUEST_SUPPORTED_LANGUAGES)) {
      expect(
        screen.getAllByText(code, { exact: true }).length,
      ).toBeGreaterThanOrEqual(1);
    }
  });
});

describe("SimpleLanguageSelector es-AR pill", () => {
  it("shows ES-AR (not EN) when the guest locale is Argentine Spanish", async () => {
    mockNavState.currentLanguage = "es-AR";
    render(<SimpleLanguageSelector tableCode="TABLE-1" />);
    await waitFor(() => {
      expect(screen.getByText("ES-AR")).toBeInTheDocument();
    });
    expect(screen.queryByText("EN")).not.toBeInTheDocument();
  });

  it("keeps the table hash when changing language (#401)", async () => {
    const user = userEvent.setup();
    window.history.pushState({}, "", "/t/TABLE-1?lang=en#menu");
    mockNavState.currentLanguage = "en";
    render(<SimpleLanguageSelector tableCode="TABLE-1" />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: /languageSelector\.changeLanguage|change language/i,
        }),
      ).toBeInTheDocument();
    });
    await user.click(
      screen.getByRole("button", {
        name: /languageSelector\.changeLanguage|change language/i,
      }),
    );
    const esChip = await screen.findByText("es", { exact: true });
    await user.click(esChip.closest("button")!);

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalled();
    });
    const replaceArg = mockReplace.mock.calls.find((c) =>
      String(c[0]).includes("lang=es"),
    );
    expect(String(replaceArg?.[0])).toContain("#menu");
  });
});
