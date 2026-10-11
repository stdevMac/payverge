/**
 * @jest-environment jsdom
 */
import {
  __errorBoundaryCopyForTests,
  resolveErrorBoundaryCopy,
} from "./errorBoundaryCopy";

describe("resolveErrorBoundaryCopy", () => {
  const originalNavigatorDescriptor = Object.getOwnPropertyDescriptor(
    window,
    "navigator",
  );

  afterEach(() => {
    // Reset query string without redefining non-configurable window.location.
    window.history.replaceState(null, "", window.location.pathname);
    document.documentElement.lang = "";
    if (originalNavigatorDescriptor) {
      Object.defineProperty(window, "navigator", originalNavigatorDescriptor);
    }
  });

  function mockSearch(search: string) {
    const path = window.location.pathname || "/";
    window.history.replaceState(null, "", search ? `${path}${search}` : path);
  }

  function mockNavigatorLanguage(language: string) {
    Object.defineProperty(window, "navigator", {
      configurable: true,
      value: { language },
    });
  }

  it("returns French copy when ?lang=fr is present", () => {
    mockSearch("?lang=fr");
    document.documentElement.lang = "en";
    mockNavigatorLanguage("de");

    const copy = resolveErrorBoundaryCopy();
    expect(copy.title).toBe(__errorBoundaryCopyForTests.fr.title);
    expect(copy.retry).toBe(__errorBoundaryCopyForTests.fr.retry);
  });

  it("prefers ?lang= over document.lang and navigator.language", () => {
    mockSearch("?lang=ja");
    document.documentElement.lang = "fr";
    mockNavigatorLanguage("de");

    expect(resolveErrorBoundaryCopy().title).toBe(
      __errorBoundaryCopyForTests.ja.title,
    );
  });

  it("uses document.documentElement.lang when no ?lang=", () => {
    mockSearch("");
    document.documentElement.lang = "de";
    mockNavigatorLanguage("fr");

    expect(resolveErrorBoundaryCopy().title).toBe(
      __errorBoundaryCopyForTests.de.title,
    );
  });

  it("resolves es-AR (voseo) before generic es", () => {
    mockSearch("?lang=es-AR");
    expect(resolveErrorBoundaryCopy().body).toBe(
      __errorBoundaryCopyForTests["es-AR"].body,
    );
    expect(resolveErrorBoundaryCopy().body).toContain("Disculpá");
  });

  it("falls back to generic Spanish for es-* regional tags", () => {
    mockSearch("?lang=es-MX");
    expect(resolveErrorBoundaryCopy().title).toBe(
      __errorBoundaryCopyForTests.es.title,
    );
  });

  it("falls back to English for unknown locales", () => {
    mockSearch("?lang=zz");
    expect(resolveErrorBoundaryCopy().title).toBe(
      __errorBoundaryCopyForTests.en.title,
    );
  });

  it("includes table-specific reassurance body", () => {
    mockSearch("?lang=en");
    const copy = resolveErrorBoundaryCopy();
    expect(copy.tableBody).toContain("Your table");
    expect(copy.tableBody).not.toBe(copy.body);
  });

  it("keeps table 404 copy distinct from storefront custom-URL copy", () => {
    mockSearch("?lang=en");
    const copy = resolveErrorBoundaryCopy();
    expect(copy.tableNotFoundTitle).toMatch(/table/i);
    expect(copy.tableNotFoundBody).not.toMatch(/custom URL/i);
    expect(copy.tableNotFoundBody).not.toBe(copy.notFoundBody);
  });
});
