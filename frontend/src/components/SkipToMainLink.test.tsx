/** @jest-environment jsdom */
import React from "react";
import { act, render, screen } from "@testing-library/react";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";
import { SkipToMainLink } from "./SkipToMainLink";

describe("SkipToMainLink (#26)", () => {
  it("uses Spanish copy when the operator locale is es or es-AR", () => {
    const { rerender } = render(
      <SimpleTranslationProvider initialLocale="es">
        <SkipToMainLink />
      </SimpleTranslationProvider>,
    );
    expect(
      screen.getByRole("link", { name: "Saltar al contenido principal" }),
    ).toHaveAttribute("href", "#main-content");

    rerender(
      <SimpleTranslationProvider initialLocale="es-AR">
        <SkipToMainLink />
      </SimpleTranslationProvider>,
    );
    expect(
      screen.getByRole("link", { name: "Saltar al contenido principal" }),
    ).toBeInTheDocument();
  });

  it("keeps English on en operator pages", () => {
    render(
      <SimpleTranslationProvider initialLocale="en">
        <SkipToMainLink />
      </SimpleTranslationProvider>,
    );
    expect(
      screen.getByRole("link", { name: "Skip to main content" }),
    ).toBeInTheDocument();
  });

  // jsdom's pathname is "/", the instance root, which is a diner surface: the
  // guest catalog loads asynchronously.
  it("switches to Spanish after an in-app guest es-AR change on an unprefixed route", async () => {
    render(
      <SimpleTranslationProvider initialLocale="en">
        <SkipToMainLink />
      </SimpleTranslationProvider>,
    );
    expect(
      screen.getByRole("link", { name: "Skip to main content" }),
    ).toBeInTheDocument();

    act(() => {
      window.dispatchEvent(
        new CustomEvent("guestLanguageChange", {
          detail: { language: "es-AR", businessId: 1 },
        }),
      );
    });

    expect(
      await screen.findByRole("link", { name: "Saltar al contenido principal" }),
    ).toBeInTheDocument();
  });
});
