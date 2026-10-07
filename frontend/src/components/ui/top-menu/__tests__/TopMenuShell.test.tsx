/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";
import type { Locale } from "@/i18n/localeRegistry";
import {
  HEADER_SIGN_IN_CLASS,
  HEADER_STAFF_LINK_CLASS,
  DESKTOP_NAV_CLASS,
  MOBILE_ONLY_CLASS,
  TopMenuInstantChrome,
} from "../TopMenuShell";

let mockPathname = "/";

jest.mock("next/navigation", () => ({
  usePathname: () => mockPathname,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: ({ src, alt }: { src: string; alt: string }) => {
    const React = jest.requireActual("react");
    return React.createElement("img", { src, alt });
  },
}));

function renderChrome(locale: Locale = "en") {
  return render(
    <SimpleTranslationProvider initialLocale={locale}>
      <TopMenuInstantChrome />
    </SimpleTranslationProvider>,
  );
}

describe("TopMenuInstantChrome", () => {
  beforeEach(() => {
    mockPathname = "/";
  });

  it("renders logo and public nav links immediately", () => {
    renderChrome("en");
    expect(screen.getByAltText("Payverge")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /home/i })).toHaveAttribute("href", "/");
    // The marketing site is gone: no sales links in the header.
    for (const name of [/features/i, /^ai$/i, /pricing/i, /contact/i, /blog/i]) {
      expect(screen.queryByRole("link", { name })).toBeNull();
    }
    expect(screen.getByRole("link", { name: /staff login/i })).toHaveAttribute(
      "href",
      "/staff/login",
    );
    expect(screen.getByRole("link", { name: /sign in/i })).toHaveAttribute(
      "href",
      "/dashboard?auth=signin",
    );
  });

  // N-2: pre-hydration public nav must not flash English on lang=es.
  // Revert-proof against hardcoded English labels in TopMenuInstantChrome.
  // Sign-in goes to /dashboard (no locale-prefixed variant): "/" is the
  // venue page now, and the operator locale cookie carries the language.
  it("renders Spanish public nav when operator locale is es", () => {
    renderChrome("es");
    expect(screen.getByRole("link", { name: /inicio/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /acceso personal/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /iniciar sesión/i })).toHaveAttribute(
      "href",
      "/dashboard?auth=signin",
    );
    expect(screen.getByRole("link", { name: /acceso personal/i })).toHaveAttribute(
      "href",
      "/es/staff/login",
    );
    expect(screen.queryByRole("link", { name: /^home$/i })).not.toBeInTheDocument();
  });

  // #811 refined #668: the clipped-looking "Personal" staff label is restored
  // to the es wording ("Acceso Personal", matching the footer); the rail keeps
  // fitting because signIn stays short ("Ingresar").
  it("uses nowrap es-AR header CTAs so 1024px chrome does not clip (#668/#811)", () => {
    renderChrome("es-AR");
    const signIn = screen.getByRole("link", { name: "Ingresar" });
    expect(signIn).toHaveAttribute("href", "/dashboard?auth=signin");
    expect(signIn.className).toBe(HEADER_SIGN_IN_CLASS);
    expect(signIn.className).toMatch(/whitespace-nowrap/);
    expect(signIn.className).toMatch(/shrink-0/);
    const staff = screen.getByRole("link", { name: "Acceso Personal" });
    expect(staff).toHaveAttribute("href", "/es-ar/staff/login");
    expect(staff.className).toBe(HEADER_STAFF_LINK_CLASS);
    expect(staff.className).toMatch(/whitespace-nowrap/);
    expect(screen.queryByRole("link", { name: /iniciar sesión/i })).toBeNull();
    expect(screen.queryByText("Español (Argentina)")).toBeNull();
    expect(screen.getByTestId("header-auth-rail").className).toMatch(
      /hidden md:flex/,
    );
  });

  it("shows the single Home link from md up and the drawer below md", () => {
    expect(DESKTOP_NAV_CLASS).toBe("hidden md:flex items-center");
    expect(MOBILE_ONLY_CLASS).toBe("md:hidden");

    const { container } = renderChrome("es-AR");
    const desktopNav = container.querySelector(".hidden.md\\:flex");
    expect(desktopNav).not.toBeNull();
  });

  it("never flashes Sign in / Staff Login on operator dashboard routes", () => {
    mockPathname = "/business/demo-admin-8-ai-pro/dashboard";
    renderChrome("en");
    expect(screen.queryByRole("link", { name: /sign in/i })).toBeNull();
    expect(screen.queryByRole("link", { name: /staff login/i })).toBeNull();
    expect(screen.queryByRole("link", { name: /features/i })).toBeNull();
    expect(screen.queryByRole("link", { name: /pricing/i })).toBeNull();
    expect(screen.getByAltText("Payverge")).toBeInTheDocument();
  });
});
