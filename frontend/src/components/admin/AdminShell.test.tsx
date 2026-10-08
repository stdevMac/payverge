/** @jest-environment jsdom */

import { render, screen } from "@testing-library/react";
import { AdminShell } from "./AdminShell";

jest.mock("next/navigation", () => ({
  usePathname: () => "/admin",
}));
jest.mock("@/components/SimpleLanguageSwitcherLazy", () => ({
  __esModule: true,
  default: () => <div data-testid="admin-language-switcher" />,
}));

describe("AdminShell locale control", () => {
  it("renders an explicit language switcher in chromeless admin UI", () => {
    render(<AdminShell><p>content</p></AdminShell>);
    expect(screen.getByTestId("admin-language-switcher")).toBeInTheDocument();
  });
});

describe("AdminShell nav", () => {
  it("lists the instance sections and no billing console", () => {
    render(<AdminShell><p>content</p></AdminShell>);
    expect(screen.getAllByRole("link", { name: /Businesses/ }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("link", { name: /Plugins/ }).length).toBeGreaterThan(0);
    expect(screen.queryByRole("link", { name: /Stripe/ })).not.toBeInTheDocument();
  });
});
