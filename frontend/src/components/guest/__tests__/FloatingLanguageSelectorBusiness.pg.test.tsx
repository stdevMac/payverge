/** @jest-environment jsdom */
// PG-4 / PG-5 / PG-6: pill agrees with locale, picker updates URL, outside click closes.
import React from "react";
import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FloatingLanguageSelectorBusiness } from "../FloatingLanguageSelectorBusiness";

const mockSetLanguage = jest.fn();
const mockReplace = jest.fn();
const mockNavState = {
  searchParams: new URLSearchParams("lang=ja"),
  currentLanguage: "ja",
  pathname: "/b/demo",
};

jest.mock("next/navigation", () => ({
  usePathname: () => mockNavState.pathname,
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => mockNavState.searchParams,
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

const langs = [
  { language_code: "en", is_default: true },
  { language_code: "es", is_default: false },
  { language_code: "ja", is_default: false },
] as any[];

const supported = [
  { code: "en", name: "English", native_name: "English" },
  { code: "es", name: "Spanish", native_name: "Español" },
  { code: "ja", name: "Japanese", native_name: "日本語" },
] as any[];

describe("FloatingLanguageSelectorBusiness PG-4/5/6", () => {
  beforeEach(() => {
    mockSetLanguage.mockClear();
    mockReplace.mockClear();
    localStorage.clear();
    mockNavState.searchParams = new URLSearchParams("lang=ja");
    mockNavState.currentLanguage = "ja";
    mockNavState.pathname = "/b/demo";
  });

  it("shows ES-AR (not EN) when the active guest locale is Argentine Spanish", async () => {
    mockNavState.searchParams = new URLSearchParams("lang=es-AR");
    mockNavState.currentLanguage = "es-AR";
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={[
          { language_code: "en", is_default: true },
          { language_code: "es", is_default: false },
        ] as any[]}
        supportedLanguages={
          [
            { code: "en", name: "English", native_name: "English" },
            { code: "es", name: "Spanish", native_name: "Español" },
          ] as any[]
        }
        variant="inline"
      />,
    );
    expect(screen.getByText("ES-AR")).toBeInTheDocument();
    expect(screen.queryByText("EN")).not.toBeInTheDocument();
    await waitFor(() => {
      expect(mockSetLanguage).toHaveBeenCalledWith("es-AR");
    });
  });

  it("PG-4: pill displays the provider language (ja), not a stuck ES default", async () => {
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="inline"
      />,
    );
    expect(screen.getByText("JA")).toBeInTheDocument();
    await waitFor(() => {
      expect(mockSetLanguage).toHaveBeenCalledWith("ja");
    });
  });

  it("PG-5: selecting a language updates ?lang= so sticky URL cannot re-defeat the pick", async () => {
    const user = userEvent.setup();
    mockNavState.currentLanguage = "ja";
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="inline"
      />,
    );

    await user.click(
      screen.getByRole("button", { name: /languageSelector\.changeLanguage|change language/i }),
    );
    const esOption = await screen.findByText("Español");
    await user.click(esOption);

    await waitFor(() => {
      expect(mockSetLanguage).toHaveBeenCalledWith("es");
      expect(mockReplace).toHaveBeenCalled();
    });
    const replaceArg = mockReplace.mock.calls.find((c) =>
      String(c[0]).includes("lang=es"),
    );
    expect(replaceArg).toBeTruthy();
  });

  it("PG-6: outside mousedown on a NextUI-style storefront card closes the dropdown", async () => {
    const user = userEvent.setup();
    render(
      <div>
        <FloatingLanguageSelectorBusiness
          businessId={1}
          businessLanguages={langs}
          supportedLanguages={supported}
          variant="inline"
        />
        {/* NextUI roots use data-slot="base" site-wide — must NOT count as "inside menu". */}
        <button type="button" data-slot="base" data-testid="storefront-card">
          Menu card
        </button>
      </div>,
    );

    await user.click(
      screen.getByRole("button", { name: /languageSelector\.changeLanguage|change language/i }),
    );
    expect(await screen.findByText("Español")).toBeInTheDocument();

    // Real outside path: mousedown on a sibling storefront control, not document.body.
    fireEvent.mouseDown(screen.getByTestId("storefront-card"));

    await waitFor(() => {
      expect(screen.queryByText("Español")).not.toBeInTheDocument();
    });
  });

  it("names the language dialog and focuses the selected locale on open (#399)", async () => {
    const user = userEvent.setup();
    mockNavState.currentLanguage = "es";
    mockNavState.searchParams = new URLSearchParams("lang=es");
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="inline"
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: /languageSelector\.changeLanguage|change language/i,
      }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: /languageSelector\.changeLanguage|change language/i,
    });
    expect(dialog).toBeInTheDocument();

    const selected = screen.getByRole("option", { selected: true });
    expect(selected).toHaveTextContent(/Español|es/i);
    await waitFor(() => {
      expect(selected).toHaveFocus();
    });
    const tabbable = screen
      .getAllByRole("option")
      .filter((item) => item.getAttribute("tabindex") === "0");
    expect(tabbable).toHaveLength(1);
  });

  it("keeps #reservations on the URL when switching language (#401)", async () => {
    const user = userEvent.setup();
    window.history.pushState({}, "", "/b/demo?lang=ja#reservations");
    mockNavState.currentLanguage = "ja";
    mockNavState.searchParams = new URLSearchParams("lang=ja");
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="inline"
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: /languageSelector\.changeLanguage|change language/i,
      }),
    );
    await user.click(await screen.findByText("English"));

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalled();
    });
    const replaceArg = mockReplace.mock.calls.find((c) =>
      String(c[0]).includes("lang=en"),
    );
    expect(String(replaceArg?.[0])).toContain("#reservations");
  });

  it("keeps the floating trigger after a locale change (#474)", async () => {
    mockNavState.searchParams = new URLSearchParams();
    mockNavState.currentLanguage = "ja";
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="floating"
      />,
    );

    // Plan 1.11: the trigger mounts immediately — the 1s setTimeout
    // materialization is gone; entrance is a CSS animation instead.
    const trigger = screen.getByRole("button", {
      name: /languageSelector\.changeLanguage|change language/i,
    });
    expect(trigger).toBeInTheDocument();

    fireEvent.click(trigger);
    fireEvent.click(await screen.findByText("Español"));

    jest.useFakeTimers();
    await act(async () => {
      jest.advanceTimersByTime(500);
    });
    jest.useRealTimers();

    expect(
      screen.getByRole("button", {
        name: /languageSelector\.changeLanguage|change language/i,
      }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("roves focus across options with ArrowDown, End, Home, and wrapping ArrowUp", async () => {
    const user = userEvent.setup();
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="inline"
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: /languageSelector\.changeLanguage|change language/i,
      }),
    );

    const options = await screen.findAllByRole("option");
    const selectedIndex = options.findIndex(
      (option) => option.getAttribute("aria-selected") === "true",
    );
    expect(selectedIndex).toBeGreaterThanOrEqual(0);
    await waitFor(() => {
      expect(options[selectedIndex]).toHaveFocus();
    });

    const listbox = screen.getByRole("listbox");
    fireEvent.keyDown(listbox, { key: "ArrowDown" });
    expect(document.activeElement).toBe(
      options[(selectedIndex + 1) % options.length],
    );

    fireEvent.keyDown(listbox, { key: "End" });
    expect(document.activeElement).toBe(options[options.length - 1]);

    fireEvent.keyDown(listbox, { key: "Home" });
    expect(document.activeElement).toBe(options[0]);

    fireEvent.keyDown(listbox, { key: "ArrowUp" });
    expect(document.activeElement).toBe(options[options.length - 1]);
  });

  it("floating variant carries the CSS entrance class (no setTimeout gate)", () => {
    mockNavState.searchParams = new URLSearchParams();
    const { container } = render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="floating"
      />,
    );
    const root = container.firstElementChild as HTMLElement;
    expect(root.className).toMatch(/storefront-lang-pill-in/);
    // Immediate visibility: no opacity-0 / visibility gate left over from the
    // old delayed materialization.
    expect(root.className).not.toMatch(/\binvisible\b/);
  });

  it("shows the floating trigger immediately after remount when ?lang= is set (#474)", () => {
    mockNavState.searchParams = new URLSearchParams("lang=es");
    mockNavState.currentLanguage = "es";
    const { unmount } = render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="floating"
      />,
    );
    expect(
      screen.getByRole("button", {
        name: /languageSelector\.changeLanguage|change language/i,
      }),
    ).toBeInTheDocument();
    unmount();
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={langs}
        supportedLanguages={supported}
        variant="floating"
      />,
    );
    expect(
      screen.getByRole("button", {
        name: /languageSelector\.changeLanguage|change language/i,
      }),
    ).toBeInTheDocument();
  });

  it("lists es-AR in the storefront picker even when the business only enabled en/es", async () => {
    const user = userEvent.setup();
    mockNavState.currentLanguage = "es-AR";
    mockNavState.searchParams = new URLSearchParams("lang=es-AR");
    render(
      <FloatingLanguageSelectorBusiness
        businessId={1}
        businessLanguages={[
          { language_code: "en", is_default: true },
          { language_code: "es", is_default: false },
        ] as any[]}
        supportedLanguages={supported}
        variant="inline"
      />,
    );

    expect(screen.getByText("ES-AR")).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", {
        name: /languageSelector\.changeLanguage|change language/i,
      }),
    );
    expect(screen.getAllByText("es-AR", { exact: true }).length).toBeGreaterThan(
      0,
    );
  });

  it("honors /es/b/{slug} over a saved English per-business pick (#860)", async () => {
    mockNavState.pathname = "/es/b/parrilla-quebracho-azul";
    mockNavState.searchParams = new URLSearchParams("");
    mockNavState.currentLanguage = "en";
    localStorage.setItem("guest-language-142", "en");

    render(
      <FloatingLanguageSelectorBusiness
        businessId={142}
        businessLanguages={[
          { language_code: "en", is_default: true },
          { language_code: "es", is_default: false },
        ] as any[]}
        supportedLanguages={supported}
        variant="inline"
      />,
    );

    await waitFor(() => {
      expect(mockSetLanguage).toHaveBeenCalledWith("es");
    });
    expect(mockSetLanguage).not.toHaveBeenCalledWith("en");
  });
});
