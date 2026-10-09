/** @jest-environment jsdom */
/**
 * ShellChrome decides whether to render the marketing TopMenu + Footer
 * around a page. /admin remains chrome-less (full viewport for admin
 * workflows). Per-business operator dashboards keep the TopMenu but
 * suppress the marketing Footer — the Product / Company / "Built with ❤️"
 * block has no place on an operator surface. Locale switching lives in
 * TopMenu (header) on marketing pages and in DashboardSidebar on operator
 * dashboards (IMP-28).
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import ShellChrome from "../ShellChrome";

// usePathname is the only Next hook we need; stub it per-test.
const mockPathname = jest.fn();
jest.mock("next/navigation", () => ({
  usePathname: () => mockPathname(),
}));

// Stub the three chrome pieces with identifiable test components — we
// don't need their real implementations, only to know whether they
// rendered.
jest.mock("@/components/ui/top-menu/TopMenuLazy", () => ({
  __esModule: true,
  default: () => <div data-testid="top-menu" />,
}));
jest.mock("@/components/Footer", () => ({
  Footer: () => <div data-testid="public-footer" />,
}));

describe("ShellChrome", () => {
  beforeEach(() => {
    mockPathname.mockReset();
  });

  it("renders TopMenu + Footer on public pages", () => {
    mockPathname.mockReturnValue("/terms-and-conditions");
    render(
      <ShellChrome>
        <p>content</p>
      </ShellChrome>,
    );
    expect(screen.getByTestId("top-menu")).toBeInTheDocument();
    expect(screen.getByTestId("public-footer")).toBeInTheDocument();
  });

  it("suppresses Footer on /business/<id>/dashboard but keeps TopMenu", () => {
    mockPathname.mockReturnValue("/business/42/dashboard");
    render(
      <ShellChrome>
        <p>content</p>
      </ShellChrome>,
    );
    expect(screen.getByTestId("top-menu")).toBeInTheDocument();
    expect(screen.queryByTestId("public-footer")).toBeNull();
  });

  it("suppresses Footer on nested dashboard subpaths", () => {
    mockPathname.mockReturnValue("/business/mara-ai-lounge/dashboard/orders");
    render(
      <ShellChrome>
        <p>content</p>
      </ShellChrome>,
    );
    expect(screen.getByTestId("top-menu")).toBeInTheDocument();
    expect(screen.queryByTestId("public-footer")).toBeNull();
  });

  it("suppresses Footer on the root portfolio dashboard", () => {
    mockPathname.mockReturnValue("/dashboard");
    render(
      <ShellChrome>
        <p>content</p>
      </ShellChrome>,
    );
    expect(screen.getByTestId("top-menu")).toBeInTheDocument();
    expect(screen.queryByTestId("public-footer")).toBeNull();
  });

  it("suppresses chrome on /admin routes", () => {
    mockPathname.mockReturnValue("/admin/users");
    render(
      <ShellChrome>
        <p>content</p>
      </ShellChrome>,
    );
    expect(screen.queryByTestId("top-menu")).toBeNull();
    expect(screen.queryByTestId("public-footer")).toBeNull();
  });

  it("renders chrome on the marketing /business listing page (no /dashboard segment)", () => {
    mockPathname.mockReturnValue("/business/mara-ai-lounge");
    render(
      <ShellChrome>
        <p>content</p>
      </ShellChrome>,
    );
    expect(screen.getByTestId("top-menu")).toBeInTheDocument();
    expect(screen.getByTestId("public-footer")).toBeInTheDocument();
  });

  it("suppresses Footer on /account but keeps TopMenu", () => {
    mockPathname.mockReturnValue("/account");
    render(
      <ShellChrome>
        <p>content</p>
      </ShellChrome>,
    );
    expect(screen.getByTestId("top-menu")).toBeInTheDocument();
    expect(screen.queryByTestId("public-footer")).toBeNull();
  });

  it("suppresses Footer on nested /account paths", () => {
    mockPathname.mockReturnValue("/account/privacy");
    render(
      <ShellChrome>
        <p>content</p>
      </ShellChrome>,
    );
    expect(screen.getByTestId("top-menu")).toBeInTheDocument();
    expect(screen.queryByTestId("public-footer")).toBeNull();
  });
});
