/** @jest-environment jsdom */
import { render } from "@testing-library/react";
import {
  GuestTranslationProvider,
  translateKey,
} from "./GuestTranslationProvider";
import { getLocaleDirection } from "./localeRegistry";

describe("GuestTranslationProvider RTL cleanup", () => {
  afterEach(() => {
    document.documentElement.dir = "";
    document.documentElement.lang = "";
  });

  it("sets dir=rtl for Arabic and resets to ltr on unmount", () => {
    const { unmount } = render(
      <GuestTranslationProvider initialLanguage="ar">
        <div>child</div>
      </GuestTranslationProvider>,
    );

    expect(document.documentElement.dir).toBe("rtl");
    expect(document.documentElement.lang).toBe("ar");

    unmount();

    // Leaving the guest provider (e.g. in-SPA nav to a non-guest route) must
    // restore LTR so the operator/marketing UI isn't mirrored.
    expect(document.documentElement.dir).toBe("ltr");
  });

  it("drives dir from the registry direction field (not a hardcoded set)", () => {
    // ar is the only rtl entry today; the effect must agree with the registry
    // so future RTL locales (he/fa/ur) light up without touching this file.
    expect(getLocaleDirection("ar")).toBe("rtl");

    render(
      <GuestTranslationProvider initialLanguage="ar">
        <div>child</div>
      </GuestTranslationProvider>,
    );
    expect(document.documentElement.dir).toBe(getLocaleDirection("ar"));
  });

  it("sets dir=ltr for an LTR storefront locale", () => {
    render(
      <GuestTranslationProvider initialLanguage="fr">
        <div>child</div>
      </GuestTranslationProvider>,
    );

    expect(document.documentElement.dir).toBe("ltr");
    expect(document.documentElement.lang).toBe("fr");
  });
});

describe("translateKey ICU/plural brace guard (GUEST-4)", () => {
  // The guest runtime has no ICU plural support. A translation value that
  // carries ICU plural/select syntax (e.g. `{{count, plural, one {item} other
  // {items}}}`) would otherwise leak raw `{{...}}` braces into the guest UI
  // because the `{word}` interpolation regex can't resolve them. The guard
  // strips an unresolved ICU block down to a brace-free string instead.
  const icuTranslations = {
    menu: {
      itemsInCartCount: "{count} {{count, plural, one {item} other {items}}} in cart",
    },
  };

  it("never leaks raw ICU plural braces to the guest", () => {
    const out = translateKey(icuTranslations, "menu.itemsInCartCount", {
      count: 3,
    });
    expect(out).not.toMatch(/\{\{/);
    expect(out).not.toContain("plural,");
    expect(out).toContain("3");
    expect(out).toContain("in cart");
  });

  it("falls back to the 'other' plural form when an ICU block is present", () => {
    const out = translateKey(icuTranslations, "menu.itemsInCartCount", {
      count: 5,
    });
    // The runtime can't count-select, so it uses the `other` form deterministically.
    expect(out).toBe("5 items in cart");
  });

  it("leaves ordinary {placeholder} interpolation untouched", () => {
    const out = translateKey(
      { greeting: "Hello {name}" },
      "greeting",
      { name: "Ana" },
    );
    expect(out).toBe("Hello Ana");
  });

  it("returns the key when the translation is missing", () => {
    expect(translateKey({}, "missing.key")).toBe("missing.key");
  });
});
