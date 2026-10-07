/** @jest-environment jsdom */
/**
 * PG-5 on /t: after the diner picks a language, sticky ?lang= must not re-apply.
 * Production setLanguage is not useCallback-stable; if the load effect depends
 * on it, every pick re-runs the effect and re-seeds the original URL lang.
 */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SimpleLanguageSelector } from "../SimpleLanguageSelector";

const mockSetLanguage = jest.fn();
const mockReplace = jest.fn();
const mockNavState = {
  searchParams: new URLSearchParams("lang=ja"),
};

jest.mock("next/navigation", () => ({
  usePathname: () => "/t/TABLE-1",
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => mockNavState.searchParams,
}));

jest.mock("../../../api/bills", () => ({
  getBusinessByTableCode: jest.fn(async () => ({
    business: { id: 9 },
    business_languages: [
      { language_code: "en", is_default: true },
      { language_code: "es", is_default: false },
      { language_code: "ja", is_default: false },
    ],
    supported_languages: [
      { code: "en", name: "English", native_name: "English" },
      { code: "es", name: "Spanish", native_name: "Español" },
      { code: "ja", name: "Japanese", native_name: "日本語" },
    ],
  })),
}));

jest.mock("../../../i18n/GuestTranslationProvider", () => {
  const actual = jest.requireActual("../../../i18n/GuestTranslationProvider");
  return {
    ...actual,
    useGuestTranslation: () => ({
      t: (k: string) => k,
      currentLanguage: "ja",
      // New function identity every render — mirrors production setLanguage
      // (not wrapped in useCallback).
      setLanguage: (lang: string) => {
        mockSetLanguage(lang);
      },
      availableLanguages: actual.GUEST_SUPPORTED_LANGUAGES,
      setBusinessId: jest.fn(),
    }),
  };
});

describe("SimpleLanguageSelector sticky ?lang= precedence (PG-5)", () => {
  beforeEach(() => {
    mockSetLanguage.mockClear();
    mockReplace.mockClear();
    localStorage.clear();
    mockNavState.searchParams = new URLSearchParams("lang=ja");
  });

  it("after picking es, does not re-apply sticky ?lang=ja when setLanguage identity changes", async () => {
    const user = userEvent.setup();
    render(<SimpleLanguageSelector tableCode="TABLE-1" />);

    // Initial resolve from ?lang=ja
    await waitFor(() => {
      expect(mockSetLanguage).toHaveBeenCalledWith("ja");
    });

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
    // Click the row button that contains the es chip (not just the chip span).
    const esRow = esChip.closest("button");
    expect(esRow).toBeTruthy();
    await user.click(esRow!);

    await waitFor(() => {
      expect(mockSetLanguage).toHaveBeenCalledWith("es");
    });

    const callsAfterPick = mockSetLanguage.mock.calls.length;

    // Flush microtasks / effect re-runs that unstable setLanguage can trigger.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // Sticky ?lang=ja must not re-win after the diner picked es.
    const afterPick = mockSetLanguage.mock.calls.slice(
      mockSetLanguage.mock.calls.findIndex((c) => c[0] === "es"),
    );
    expect(afterPick.some((c) => c[0] === "ja")).toBe(false);
    expect(mockSetLanguage.mock.calls[mockSetLanguage.mock.calls.length - 1][0]).toBe(
      "es",
    );
    // Effect must not keep thrashing after the pick.
    expect(mockSetLanguage.mock.calls.length).toBe(callsAfterPick);
  });
});
