/** @jest-environment jsdom */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { SimpleTranslationProvider } from "./SimpleTranslationProvider";
import { chromeCopyLocale, useChromeLocale } from "./useChromeLocale";

function Probe() {
  const locale = useChromeLocale();
  return <div data-testid="chrome-locale">{locale}</div>;
}

describe("chromeCopyLocale", () => {
  it("keeps operator locales and maps other Spanish tags to es", () => {
    expect(chromeCopyLocale("es-AR")).toBe("es-AR");
    expect(chromeCopyLocale("es")).toBe("es");
    expect(chromeCopyLocale("es-MX")).toBe("es");
    expect(chromeCopyLocale("en")).toBe("en");
    expect(chromeCopyLocale("fr")).toBe("en");
  });
});

describe("useChromeLocale (#26 guest-table in-app switch)", () => {
  beforeEach(() => {
    document.cookie = "payverge_guest_locale=; Max-Age=0; Path=/";
  });

  it("stays on the operator locale until a guest language event fires", () => {
    render(
      <SimpleTranslationProvider initialLocale="en">
        <Probe />
      </SimpleTranslationProvider>,
    );
    expect(screen.getByTestId("chrome-locale")).toHaveTextContent("en");
  });

  it("reads the guest cookie on a table route so reload stays Spanish", async () => {
    window.history.pushState({}, "", "/t/MF6PVR6S97");
    document.cookie = "payverge_guest_locale=es-AR; Path=/";

    render(
      <SimpleTranslationProvider initialLocale="en">
        <Probe />
      </SimpleTranslationProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("chrome-locale")).toHaveTextContent("es-AR");
    });
  });

  it("follows guestLanguageChange to es-AR without an /es prefix", () => {
    render(
      <SimpleTranslationProvider initialLocale="en">
        <Probe />
      </SimpleTranslationProvider>,
    );

    act(() => {
      window.dispatchEvent(
        new CustomEvent("guestLanguageChange", {
          detail: { language: "es-AR", businessId: 1 },
        }),
      );
    });

    expect(screen.getByTestId("chrome-locale")).toHaveTextContent("es-AR");
  });
});
