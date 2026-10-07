/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { CookieConsent } from "../CookieConsent";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";
import * as i18n from "@/i18n/OperatorLocaleProvider";

const cases: Array<{
  locale: "en" | "es" | "es-AR";
  title: RegExp;
  accept: RegExp;
  customize: RegExp;
  decline: RegExp;
}> = [
  {
    locale: "en",
    title: /We value your privacy/i,
    accept: /Accept all/i,
    customize: /Customize/i,
    decline: /Decline non-essential/i,
  },
  {
    locale: "es",
    title: /Tu privacidad nos importa/i,
    accept: /Aceptar todas/i,
    customize: /Personalizar/i,
    decline: /Rechazar las no esenciales/i,
  },
  {
    locale: "es-AR",
    title: /Tu privacidad nos importa/i,
    accept: /Aceptar todas/i,
    customize: /Personalizar/i,
    decline: /Rechazar las no esenciales/i,
  },
];

describe("CookieConsent localized banner", () => {
  beforeEach(() => localStorage.clear());

  it.each(cases)(
    "renders stacked unclipped actions in $locale",
    async ({ locale, title, accept, customize, decline }) => {
      jest
        .spyOn(i18n, "useSimpleLocale")
        .mockReturnValue({ locale, setLocale: () => {} } as any);
      render(
        <CookieConsentProvider>
          <CookieConsent />
        </CookieConsentProvider>,
      );
      expect(await screen.findByText(title)).toBeInTheDocument();
      // The localized Accept control is present in every locale (proves the
      // namespace resolved, not the leaf fallback).
      const acceptBtn = screen.getByRole("button", { name: accept });
      expect(acceptBtn).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: customize }),
      ).toBeInTheDocument();
      expect(screen.getByRole("button", { name: decline })).toBeInTheDocument();

      const card = screen.getByTestId("cookie-consent-card");
      expect(card.className).not.toMatch(/24dvh|200px/);
      expect(card.className).not.toMatch(/overflow-y-auto/);
      expect(acceptBtn.className).toMatch(/w-full/);
      expect(acceptBtn.closest("[class*='overflow-y-auto']")).toBeNull();
    },
  );
});
