/** @jest-environment jsdom */

import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import {
  SimpleTranslationProvider,
  useSimpleLocale,
} from "../SimpleTranslationProvider";

describe("SimpleTranslationProvider locale persistence", () => {
  beforeEach(() => {
    window.localStorage.clear();
    document.cookie = "payverge_locale=; Max-Age=0; Path=/";
    window.history.replaceState(null, "", "/");
  });

  it("persists a locale choice to both local storage and a SameSite cookie", () => {
    const wrapper = ({ children }: PropsWithChildren) => (
      <SimpleTranslationProvider initialLocale="en">
        {children}
      </SimpleTranslationProvider>
    );
    const { result } = renderHook(() => useSimpleLocale(), { wrapper });

    act(() => result.current.setLocale("es-AR"));

    expect(window.localStorage.getItem("locale")).toBe("es-AR");
    expect(document.cookie).toContain("payverge_locale=es-AR");
  });

  it("keeps the request-derived initial locale authoritative over stale local storage", async () => {
    // An operator auth surface: its request locale comes from the operator
    // cookie / Accept-Language, so it is the operator's locale to persist.
    // (The instance root is a guest path and never persists; see below.)
    window.history.replaceState(null, "", "/register");
    window.localStorage.setItem("locale", "en");
    const wrapper = ({ children }: PropsWithChildren) => (
      <SimpleTranslationProvider initialLocale="es-AR">
        {children}
      </SimpleTranslationProvider>
    );
    const { result } = renderHook(() => useSimpleLocale(), { wrapper });

    await waitFor(() => expect(result.current.locale).toBe("es-AR"));
    expect(window.localStorage.getItem("locale")).toBe("es-AR");
  });

  it("uses local storage only as a legacy fallback when no request locale exists", async () => {
    // jsdom defaults to `/`, which is an English-stable unprefixed marketing
    // path (#37). Use a non-marketing surface so the legacy localStorage
    // fallback is actually reachable when initialLocale is omitted.
    window.history.replaceState(null, "", "/dashboard");
    window.localStorage.setItem("locale", "es");
    const wrapper = ({ children }: PropsWithChildren) => (
      <SimpleTranslationProvider>{children}</SimpleTranslationProvider>
    );
    const { result } = renderHook(() => useSimpleLocale(), { wrapper });

    await waitFor(() => expect(result.current.locale).toBe("es"));
  });

  it("writes the operator locale cookie when seeded from an /es path (#402)", async () => {
    window.history.replaceState(null, "", "/es");
    const wrapper = ({ children }: PropsWithChildren) => (
      <SimpleTranslationProvider initialLocale="es">
        {children}
      </SimpleTranslationProvider>
    );
    renderHook(() => useSimpleLocale(), { wrapper });

    await waitFor(() => {
      expect(document.cookie).toContain("payverge_locale=es");
    });
    expect(window.localStorage.getItem("locale")).toBe("es");
  });

  it("writes the operator locale cookie when seeded from /es-ar", async () => {
    window.history.replaceState(null, "", "/es-ar/privacy-policy");
    const wrapper = ({ children }: PropsWithChildren) => (
      <SimpleTranslationProvider initialLocale="es-AR">
        {children}
      </SimpleTranslationProvider>
    );
    renderHook(() => useSimpleLocale(), { wrapper });

    await waitFor(() => {
      expect(document.cookie).toContain("payverge_locale=es-AR");
    });
  });

  it("keeps an explicit English pick on the operator dashboard over Accept-Language (#617)", async () => {
    window.history.replaceState(null, "", "/business/demo/dashboard");
    window.localStorage.setItem("locale", "en");
    const wrapper = ({ children }: PropsWithChildren) => (
      <SimpleTranslationProvider initialLocale="es-AR">
        {children}
      </SimpleTranslationProvider>
    );
    const { result } = renderHook(() => useSimpleLocale(), { wrapper });

    expect(result.current.locale).toBe("en");
    await waitFor(() => expect(result.current.locale).toBe("en"));
    expect(document.cookie).toContain("payverge_locale=en");
    expect(window.localStorage.getItem("locale")).toBe("en");
  });

  it("does not persist an Accept-Language SSR locale over an empty store on /business (#617)", async () => {
    window.history.replaceState(null, "", "/business/demo/dashboard?tab=inventory");
    const wrapper = ({ children }: PropsWithChildren) => (
      <SimpleTranslationProvider initialLocale="es-AR">
        {children}
      </SimpleTranslationProvider>
    );
    const { result } = renderHook(() => useSimpleLocale(), { wrapper });

    await waitFor(() => expect(result.current.locale).toBe("es-AR"));
    expect(window.localStorage.getItem("locale")).toBeNull();
  });

  it.each(["/", "/b/demo-bistro", "/t/T1/menu", "/terms-and-conditions"])(
    "does not overwrite the operator's stored locale from guest/public path %s",
    async (path) => {
      // Operator signs out (full load of "/") or previews the storefront: the
      // request locale there is the guest language / English path contract.
      window.history.replaceState(null, "", path);
      window.localStorage.setItem("locale", "es");
      document.cookie = "payverge_locale=es; Path=/";
      const wrapper = ({ children }: PropsWithChildren) => (
        <SimpleTranslationProvider initialLocale="en">
          {children}
        </SimpleTranslationProvider>
      );
      const { result } = renderHook(() => useSimpleLocale(), { wrapper });

      await waitFor(() => expect(result.current.locale).toBe("en"));
      expect(window.localStorage.getItem("locale")).toBe("es");
      expect(document.cookie).toContain("payverge_locale=es");
      expect(document.cookie).not.toContain("payverge_locale=en");
    },
  );

  it("ignores stale localStorage on unprefixed public page paths (#37)", async () => {
    window.history.replaceState(null, "", "/terms-and-conditions");
    window.localStorage.setItem("locale", "es");
    const wrapper = ({ children }: PropsWithChildren) => (
      <SimpleTranslationProvider>{children}</SimpleTranslationProvider>
    );
    const { result } = renderHook(() => useSimpleLocale(), { wrapper });

    await waitFor(() => expect(result.current.locale).toBe("en"));
  });
});
