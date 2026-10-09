/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import "@testing-library/jest-dom";

const mockPush = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush, replace: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es" }),
  getTranslation: (key: string) => key,
}));
jest.mock("@/i18n/useApiErrorMessage", () => {
  const localizeError = (error: unknown) =>
    error instanceof Error ? error.message : "error";
  return {
    useApiErrorMessage: () => localizeError,
  };
});
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    refreshStaffData: jest.fn(),
    isInitialized: true,
    isStaffUser: false,
    isOAuthUser: false,
    isWeb3User: false,
    oauthData: null,
    staffData: null,
    walletAddress: null,
  }),
}));
jest.mock("@/utils/staffAuth", () => ({
  setStaffData: jest.fn(),
  clearStaffSession: jest.fn(),
}));
jest.mock("@/api/auth", () => ({
  logout: jest.fn(),
}));

jest.mock("../../../api/staff", () => ({
  getInvitationPreview: jest.fn(),
  acceptInvitation: jest.fn(),
}));

import AcceptInvitation from "../AcceptInvitation";
import * as StaffAPI from "../../../api/staff";

// NextUI's Button calls useLayoutEffect, which React's server renderer warns
// about. That warning is about hydration, not about this assertion, and it
// would otherwise bury the real output.
const withoutSsrLayoutEffectNoise = (fn: () => string): string => {
  const real = console.error;
  console.error = (...args: unknown[]) => {
    if (typeof args[0] === "string" && args[0].includes("useLayoutEffect does nothing on the server")) {
      return;
    }
    real(...(args as []));
  };
  try {
    return fn();
  } finally {
    console.error = real;
  }
};

describe("AcceptInvitation — missing token (#881)", () => {
  // The mount effect already flips step to "error", and render() flushes
  // effects — so a post-render assertion passes with or without the fix. What
  // the QA screenshot actually showed is the FIRST paint: a spinner that never
  // resolves. renderToStaticMarkup runs no effects, so it is exactly that
  // first paint.
  it("paints the permanent error, not a spinner, on the very first render", () => {
    const html = withoutSsrLayoutEffectNoise(() =>
      renderToStaticMarkup(<AcceptInvitation />),
    );

    expect(html).toContain("staffInvitation.messages.expiredTitle");
    expect(html).toContain("staffInvitation.actions.backToLogin");
    expect(html).not.toContain("staffInvitation.loading.validating");
  });

  it("never asks the API to preview a token that is not there", () => {
    render(<AcceptInvitation />);

    expect(
      screen.queryByText("staffInvitation.loading.validating"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "staffInvitation.actions.backToLogin",
      }),
    ).toBeInTheDocument();
    expect(StaffAPI.getInvitationPreview).not.toHaveBeenCalled();
  });

  it("treats a whitespace-only ?token= the same as no token", () => {
    const html = withoutSsrLayoutEffectNoise(() =>
      renderToStaticMarkup(<AcceptInvitation token="   " />),
    );

    expect(html).toContain("staffInvitation.messages.expiredTitle");
    expect(html).not.toContain("staffInvitation.loading.validating");
  });
});
