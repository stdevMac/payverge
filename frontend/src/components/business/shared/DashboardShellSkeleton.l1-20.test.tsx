/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import DashboardShellSkeleton from "./DashboardShellSkeleton";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: () => "Loading dashboard",
}));

jest.mock("./DashboardTabLoadingSkeleton", () => ({
  __esModule: true,
  default: () => <div data-testid="tab-skeleton" />,
}));

function visualRoot(container: HTMLElement): HTMLElement {
  const root = container.querySelector<HTMLElement>(
    '[data-skeleton-inert="true"]',
  );
  if (!root) throw new Error("skeleton visual root not found");
  return root;
}

describe("DashboardShellSkeleton L1-20", () => {
  it("is inert and pointer-events-none so pre-ready chrome cannot eat clicks", () => {
    const { container } = render(<DashboardShellSkeleton />);
    const root = visualRoot(container);
    expect(root).toHaveClass("pointer-events-none");
    // useEffect applies the real HTML inert attribute after mount.
    expect(root.hasAttribute("inert")).toBe(true);
  });

  // R2-7: an inert subtree is dropped from the accessibility tree, so a
  // role="status" / aria-live region nested inside it never announces. The
  // announcement has to be a sibling of the inert chrome, not a child of it.
  it("keeps the live region outside the inert subtree", () => {
    const { container } = render(<DashboardShellSkeleton />);
    const status = screen.getByRole("status");

    expect(status).toHaveTextContent("Loading dashboard");
    expect(status.getAttribute("aria-live")).toBe("polite");
    expect(status.getAttribute("aria-busy")).toBe("true");

    const root = visualRoot(container);
    expect(root.contains(status)).toBe(false);
    expect(status.closest("[inert]")).toBeNull();
    expect(status.closest('[aria-hidden="true"]')).toBeNull();
  });

  it("hides the purely decorative chrome from assistive tech", () => {
    const { container } = render(<DashboardShellSkeleton />);
    const root = visualRoot(container);
    expect(root.getAttribute("aria-hidden")).toBe("true");
    expect(root.getAttribute("role")).toBeNull();
  });
});
