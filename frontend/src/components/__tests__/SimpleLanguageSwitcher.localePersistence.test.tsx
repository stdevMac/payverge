/** @jest-environment jsdom */

/**
 * #617 — an explicit operator language pick must win for the session.
 *
 * The dashboard reads its language from SimpleTranslationProvider, and
 * HybridAuthProvider seeds that provider from the account's saved
 * `language_selected` whenever localStorage["locale"] is empty
 * (resolveBackendLocaleSeed). On operator app paths the provider is
 * contractually forbidden from writing that key, so the store is empty by
 * default — which means the ONLY thing standing between an English login and
 * the account's Spanish/voseo preference is the switcher persisting the pick.
 *
 * Nothing here is mocked except the user store (an unrelated network
 * dependency): the real provider, the real switcher and the real
 * operatorLocaleBridge guard all run.
 */

import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

jest.mock("@/store/useUserStore", () => ({
  useUserStore: (selector: (s: { user: null }) => unknown) =>
    selector({ user: null }),
}));

import SimpleLanguageSwitcher from "../SimpleLanguageSwitcher";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";
import { resolveBackendLocaleSeed } from "@/i18n/operatorLocaleBridge";

/** The dashboard surface #617 was filed against. */
const OPERATOR_PATH = "/business/demo-admin-8-ai-pro/dashboard";

function renderSwitcher(initialLocale: "en" | "es" | "es-AR") {
  return render(
    <SimpleTranslationProvider initialLocale={initialLocale}>
      <SimpleLanguageSwitcher />
    </SimpleTranslationProvider>,
  );
}

async function pickLanguage(visibleName: string) {
  const user = userEvent.setup();
  await user.click(screen.getAllByRole("button")[0]);
  const option = await screen.findByRole("menuitem", { name: visibleName });
  await user.click(option);
}

beforeEach(() => {
  window.localStorage.clear();
  document.cookie = "payverge_locale=; Max-Age=0; Path=/";
  window.history.replaceState(null, "", OPERATOR_PATH);
});

describe("operator language pick persists on the dashboard (#617)", () => {
  it("records a pick that matches the language already on screen", async () => {
    renderSwitcher("en");

    await pickLanguage("English");

    await waitFor(() =>
      expect(window.localStorage.getItem("locale")).toBe("en"),
    );
    expect(document.cookie).toContain("payverge_locale=en");
  });

  it("stops the account language from overriding that pick at sign-in", async () => {
    // Control: with nothing picked, the backend preference seeds the dashboard.
    expect(
      resolveBackendLocaleSeed({
        saved: "es-AR",
        storedLocale: window.localStorage.getItem("locale"),
      }),
    ).toBe("es-AR");

    renderSwitcher("en");
    await pickLanguage("English");

    await waitFor(() =>
      expect(
        resolveBackendLocaleSeed({
          saved: "es-AR",
          storedLocale: window.localStorage.getItem("locale"),
        }),
      ).toBeNull(),
    );
  });

  it("still records a pick that changes the language", async () => {
    renderSwitcher("en");

    await pickLanguage("Español");

    await waitFor(() =>
      expect(window.localStorage.getItem("locale")).toBe("es"),
    );
    expect(document.cookie).toContain("payverge_locale=es");
  });
});
