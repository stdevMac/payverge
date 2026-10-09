/** @jest-environment jsdom */

import { render } from "@testing-library/react";
import { SimpleTranslationProvider } from "../SimpleTranslationProvider";
import { GuestTranslationProvider } from "../GuestTranslationProvider";

// <html lang>/<dir> belong to GuestTranslationProvider on every guest path and
// to SimpleTranslationProvider everywhere else.

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("SimpleTranslationProvider <html lang>", () => {
  it.each(["/delivery/12/track", "/reservations/ABC123", "/scan", "/"])(
    "leaves lang untouched on guest path %s",
    (path) => {
      window.history.replaceState(null, "", path);
      document.documentElement.lang = "fr";
      render(
        <SimpleTranslationProvider initialLocale="es">
          <span />
        </SimpleTranslationProvider>,
      );
      expect(document.documentElement.lang).toBe("fr");
    },
  );

  it("writes lang on an operator path", () => {
    window.history.replaceState(null, "", "/dashboard");
    render(
      <SimpleTranslationProvider initialLocale="es">
        <span />
      </SimpleTranslationProvider>,
    );
    expect(document.documentElement.lang).toBe("es");
  });
});

describe("GuestTranslationProvider unmount", () => {
  it("hands lang and dir back to the operator locale", () => {
    window.history.replaceState(null, "", "/b/acme-bistro");
    const { rerender } = render(
      <SimpleTranslationProvider initialLocale="es">
        <GuestTranslationProvider initialLanguage="ar" preferInitialLanguage>
          <span />
        </GuestTranslationProvider>
      </SimpleTranslationProvider>,
    );
    expect(document.documentElement.lang).toBe("ar");
    expect(document.documentElement.dir).toBe("rtl");

    // In-SPA nav to the dashboard: the guest provider unmounts.
    window.history.replaceState(null, "", "/dashboard");
    rerender(
      <SimpleTranslationProvider initialLocale="es">
        <span />
      </SimpleTranslationProvider>,
    );
    expect(document.documentElement.lang).toBe("es");
    expect(document.documentElement.dir).toBe("ltr");
  });
});
