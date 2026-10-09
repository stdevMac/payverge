/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import FloatingLanguageSwitcher from "../FloatingLanguageSwitcher";

let mockPathname = "/";
jest.mock("next/navigation", () => ({
  usePathname: () => mockPathname,
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => undefined }),
  getTranslation: (k: string) => k,
}));

describe("FloatingLanguageSwitcher position", () => {
  it.each([
    "/",
    "/pricing",
    "/features",
    "/privacy-policy",
    "/terms-and-conditions",
    "/refund",
    "/account",
    "/admin",
    "/staff/login",
    "/business/register",
    "/dashboard",
  ])("renders nothing on %s because the route owns its chrome", (pathname) => {
    mockPathname = pathname;
    const { container } = render(<FloatingLanguageSwitcher />);
    expect(container.firstChild).toBeNull();
  });

  it.each(["/forgot-password", "/reset-password", "/verify-email"])(
    "renders a top-right control on standalone no-chrome route %s",
    (pathname) => {
      mockPathname = pathname;
      const { container } = render(<FloatingLanguageSwitcher />);
      const root = container.firstChild as HTMLElement;
      expect(root).not.toBeNull();
      expect(root.className).toMatch(/top-4/);
      expect(root.className).toMatch(/right-4/);
      expect(root.className).not.toMatch(/md:hidden/);
    },
  );

  it("renders nothing on the operator dashboard — sidebar footer owns the switcher (IMP-28)", () => {
    mockPathname = "/business/42/dashboard";
    const { container } = render(<FloatingLanguageSwitcher />);
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing on nested operator dashboard subpaths (IMP-28)", () => {
    mockPathname = "/business/mara-ai-lounge/dashboard/orders";
    const { container } = render(<FloatingLanguageSwitcher />);
    expect(container.firstChild).toBeNull();
  });
});
