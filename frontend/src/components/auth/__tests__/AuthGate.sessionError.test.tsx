/**
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import AuthGate, { shouldWallSessionError } from "../AuthGate";

const mockReplace = jest.fn();
const mockRetrySessionBootstrap = jest.fn();
// /business/* is operator-protected and not render-through (unlike /dashboard).
let mockPathname = "/business/cafe/dashboard";
let mockSearchParams = new URLSearchParams("tab=settings");
let mockAuth = {
  isInitialized: true,
  isLoading: false,
  isWeb3User: false,
  isStaffUser: false,
  isOAuthUser: false,
  sessionInfoError: false,
  retrySessionBootstrap: mockRetrySessionBootstrap,
};

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace }),
  usePathname: () => mockPathname,
  useSearchParams: () => mockSearchParams,
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockAuth,
}));

jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

jest.mock("@/i18n/operatorChromeCatalog", () => ({
  getChromeTranslation: (key: string) => key,
}));

jest.mock("@nextui-org/react", () => ({
  Spinner: ({ "aria-label": ariaLabel }: { "aria-label"?: string }) => (
    <div role="status" aria-label={ariaLabel} />
  ),
}));

describe("shouldWallSessionError — fail-open on a session blip (#621)", () => {
  it("does not wall when a prior session hint exists", () => {
    expect(
      shouldWallSessionError({
        sessionInfoError: true,
        authenticated: false,
        hasSessionHint: true,
      }),
    ).toBe(false);
  });

  it("still walls when there is no session hint", () => {
    expect(
      shouldWallSessionError({
        sessionInfoError: true,
        authenticated: false,
        hasSessionHint: false,
      }),
    ).toBe(true);
  });

  it("never walls an authenticated principal", () => {
    expect(
      shouldWallSessionError({
        sessionInfoError: true,
        authenticated: true,
        hasSessionHint: false,
      }),
    ).toBe(false);
  });
});

describe("AuthGate — sessionInfoError retriable state (L6-42 / #4)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    mockPathname = "/business/cafe/dashboard";
    mockSearchParams = new URLSearchParams("tab=settings");
    mockAuth = {
      isInitialized: true,
      isLoading: false,
      isWeb3User: false,
      isStaffUser: false,
      isOAuthUser: false,
      sessionInfoError: false,
      retrySessionBootstrap: mockRetrySessionBootstrap,
    };
  });

  it("renders a retriable error card instead of redirecting when sessionInfoError is true", () => {
    mockAuth = { ...mockAuth, sessionInfoError: true };

    render(
      <AuthGate>
        <div data-testid="protected-child">secret</div>
      </AuthGate>,
    );

    expect(screen.getByText("common.sessionCheckFailed")).toBeTruthy();
    expect(screen.getByRole("button", { name: "common.retry" })).toBeTruthy();
    expect(screen.queryByTestId("protected-child")).toBeNull();
    expect(mockReplace).not.toHaveBeenCalled();
    expect(mockRetrySessionBootstrap).toHaveBeenCalled();
  });

  it("fail-opens protected chrome when a session hint exists", () => {
    window.localStorage.setItem("payverge_had_session", "1");
    mockAuth = { ...mockAuth, sessionInfoError: true };

    render(
      <AuthGate>
        <div data-testid="protected-child">secret</div>
      </AuthGate>,
    );

    expect(screen.getByTestId("protected-child")).toBeTruthy();
    expect(screen.queryByText("common.sessionCheckFailed")).toBeNull();
    window.localStorage.removeItem("payverge_had_session");
  });

  it.each(["crm", "analytics"])(
    "fail-opens %s first paint instead of the session wall (#621)",
    (tab) => {
      window.localStorage.setItem("payverge_had_session", "1");
      mockSearchParams = new URLSearchParams(`tab=${tab}`);
      mockAuth = { ...mockAuth, sessionInfoError: true };

      render(
        <AuthGate>
          <div data-testid={`${tab}-child`}>{tab}</div>
        </AuthGate>,
      );

      expect(screen.getByTestId(`${tab}-child`)).toBeTruthy();
      expect(screen.queryByText("common.sessionCheckFailed")).toBeNull();
      window.localStorage.removeItem("payverge_had_session");
    },
  );

  it("invokes retrySessionBootstrap when the retry button is clicked", () => {
    mockAuth = { ...mockAuth, sessionInfoError: true };

    render(
      <AuthGate>
        <div>child</div>
      </AuthGate>,
    );

    fireEvent.click(screen.getByRole("button", { name: "common.retry" }));
    expect(mockRetrySessionBootstrap).toHaveBeenCalled();
  });

  it("still redirects unauthenticated users when initialized without sessionInfoError", () => {
    mockAuth = { ...mockAuth, sessionInfoError: false };

    render(
      <AuthGate>
        <div>child</div>
      </AuthGate>,
    );

    expect(mockReplace).toHaveBeenCalled();
    const dest = mockReplace.mock.calls[0]?.[0] as string;
    expect(dest.startsWith("/dashboard?redirect=")).toBe(true);
  });
});

describe("AuthGate — redirect preserves query string (L6-42 / #6)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPathname = "/business/cafe/dashboard";
    mockSearchParams = new URLSearchParams("tab=settings");
    mockAuth = {
      isInitialized: true,
      isLoading: false,
      isWeb3User: false,
      isStaffUser: false,
      isOAuthUser: false,
      sessionInfoError: false,
      retrySessionBootstrap: mockRetrySessionBootstrap,
    };
  });

  it("includes the current search string in the redirect target, encoded once", () => {
    render(
      <AuthGate>
        <div>child</div>
      </AuthGate>,
    );

    expect(mockReplace).toHaveBeenCalledTimes(1);
    const dest = mockReplace.mock.calls[0]?.[0] as string;
    // Full path+query must be the redirect value, encoded exactly once.
    expect(dest).toBe(
      `/dashboard?redirect=${encodeURIComponent("/business/cafe/dashboard?tab=settings")}`,
    );
    // Must not double-encode.
    expect(dest).not.toContain(encodeURIComponent(encodeURIComponent("?")));
  });
});
