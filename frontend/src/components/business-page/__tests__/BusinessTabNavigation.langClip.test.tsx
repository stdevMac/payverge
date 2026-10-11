/** @jest-environment jsdom */
// #595: the desktop language selector mounts inline inside the sticky tab bar,
// whose outer section keeps overflow-x-clip and whose [data-desktop-tab-bar]
// keeps overflow-hidden (frozen for #442 — many tabs must never expand the
// document). An in-flow absolute dropdown was clipped to a sliver by those
// ancestors. The fix portals the locale dialog to <body>; this suite pins that
// the dialog escapes every clipping ancestor while #442's clipping stays put.
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import BusinessTabNavigation from "../BusinessTabNavigation";
import { FloatingLanguageSelectorBusiness } from "../../guest/FloatingLanguageSelectorBusiness";
import { storefrontLocales } from "../../../i18n/localeRegistry";

const mockSetLanguage = jest.fn();
const mockReplace = jest.fn();

jest.mock("next/navigation", () => ({
  usePathname: () => "/b/demo",
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => new URLSearchParams("lang=en"),
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

const settings = { primary_color: "#1a6b6a", secondary_color: "#2a8b8a" };
const t = (k: string) => k;

function renderBarWithInlineSelector() {
  return render(
    <BusinessTabNavigation
      activeTab="menu"
      onChangeTab={() => {}}
      hasDeliveryTab
      hasReservationsTab
      designSettings={settings}
      t={t}
      rightSlot={
        <FloatingLanguageSelectorBusiness
          businessId={1}
          businessLanguages={
            [
              { language_code: "en", is_default: true },
              { language_code: "es", is_default: false },
            ] as any[]
          }
          supportedLanguages={
            [
              { code: "en", name: "English", native_name: "English" },
              { code: "es", name: "Spanish", native_name: "Español" },
            ] as any[]
          }
          variant="inline"
        />
      }
    />,
  );
}

describe("inline language selector inside the sticky tab bar (#595)", () => {
  beforeEach(() => {
    mockSetLanguage.mockClear();
    mockReplace.mockClear();
    localStorage.clear();
  });

  it("portals the locale dialog out of every clipping ancestor", async () => {
    const user = userEvent.setup();
    const { container } = renderBarWithInlineSelector();

    // Trigger renders in the bar's right slot.
    const slot = container.querySelector("[data-tab-bar-right-slot]");
    expect(slot).not.toBeNull();
    const trigger = screen.getByRole("button", {
      name: /languageSelector\.changeLanguage|change language/i,
    });
    expect(slot!.contains(trigger)).toBe(true);

    await user.click(trigger);

    const dialog = await screen.findByRole("dialog", {
      name: /languageSelector\.changeLanguage|change language/i,
    });

    // The dialog must NOT be a descendant of the clipped bar or of any
    // overflow-clipping ancestor — it is portaled directly under <body>.
    const bar = container.querySelector("[data-desktop-tab-bar]");
    expect(bar).not.toBeNull();
    expect(bar!.contains(dialog)).toBe(false);
    expect(container.contains(dialog)).toBe(false);
    expect(dialog.parentElement).toBe(document.body);
    for (
      let node: HTMLElement | null = dialog.parentElement;
      node && node !== document.documentElement;
      node = node.parentElement
    ) {
      expect(node.className || "").not.toMatch(
        /\boverflow-hidden\b|\boverflow-x-clip\b/,
      );
    }

    // Floats over the bar: fixed positioning, not clipped in-flow.
    expect(dialog.className).toMatch(/\bfixed\b/);
    expect(dialog.className).not.toMatch(/\babsolute\b/);

    // Every storefront locale is present and reachable (dropdown scrolls).
    const options = screen.getAllByRole("option");
    expect(options).toHaveLength(storefrontLocales.length);
    const codes = options.map((o) => o.getAttribute("data-key"));
    expect(new Set(codes)).toEqual(new Set(storefrontLocales));
    expect(dialog.className).toMatch(/\boverflow-y-auto\b/);
  });

  it("keeps the #442 guarantee: bar and tablist clipping classes are unchanged", () => {
    const { container } = renderBarWithInlineSelector();

    const bar = container.querySelector("[data-desktop-tab-bar]");
    expect(bar?.className).toMatch(/\bmin-w-0\b/);
    expect(bar?.className).toMatch(/\boverflow-hidden\b/);

    const tablist = screen.getByRole("tablist");
    expect(tablist.className).toMatch(/\boverflow-x-auto\b/);
    expect(tablist.className).toMatch(/\boverflow-y-hidden\b/);
  });
});
