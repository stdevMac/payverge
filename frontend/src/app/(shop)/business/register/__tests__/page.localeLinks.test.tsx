/** @jest-environment jsdom */
/**
 * #720 — the signup wizard's escape hatch leads to the operator dashboard
 * ("/" serves a venue page) and keeps the operator locale via ?lang=, so an
 * es / es-AR operator who backs out of the wizard is not flipped to English.
 */
import React from "react";
import { render, screen, act } from "@testing-library/react";

const mockReplace = jest.fn();
const mockPush = jest.fn();
let mockSearchParamsValue = new URLSearchParams();
let mockLocale = "en";

const mockRouter = { replace: mockReplace, back: jest.fn(), push: mockPush };

jest.mock("next/navigation", () => ({
  useRouter: () => mockRouter,
  useSearchParams: () => mockSearchParamsValue,
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: undefined, isConnected: false }),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isOAuthUser: false,
    isWeb3User: false,
    isStaffUser: false,
    oauthData: null,
    isLoading: false,
    isInitialized: true,
  }),
}));

jest.mock("@/store/useUserStore", () => {
  const setUser = jest.fn();
  return {
    useUserStore: Object.assign(
      () => ({ user: null }),
      { getState: () => ({ setUser }) },
    ),
  };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useAnalytics", () => ({
  usePageTracking: () => undefined,
  useClickTracking: () => jest.fn(),
  useConversionTracking: () => jest.fn(),
}));

jest.mock("@/api/users/profile", () => ({
  getUserProfile: jest.fn().mockResolvedValue(null),
}));

import BusinessRegisterPage from "../page";

async function renderFirstStep(locale: string) {
  mockLocale = locale;
  mockSearchParamsValue = new URLSearchParams("step=business");
  await act(async () => {
    render(<BusinessRegisterPage />);
  });
}

function hrefsMatching(pattern: RegExp): string[] {
  return screen
    .getAllByRole("link")
    .map((el) => el.getAttribute("href") || "")
    .filter((href) => pattern.test(href));
}

beforeEach(() => {
  mockReplace.mockClear();
  mockPush.mockClear();
  mockLocale = "en";
  mockSearchParamsValue = new URLSearchParams();
  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
});

describe("business/register escape hatch goes to the dashboard and keeps the locale (#720)", () => {
  it("links English operators to the bare dashboard", async () => {
    await renderFirstStep("en");
    expect(hrefsMatching(/^\/dashboard/)).toEqual(["/dashboard"]);
    expect(hrefsMatching(/^\/(es(-ar)?)?$/)).toEqual([]);
  });

  it("keeps Spanish via ?lang=es", async () => {
    await renderFirstStep("es");
    expect(hrefsMatching(/^\/dashboard/)).toEqual(["/dashboard?lang=es"]);
  });

  it("keeps Argentine Spanish via ?lang=es-ar", async () => {
    await renderFirstStep("es-AR");
    expect(hrefsMatching(/^\/dashboard/)).toEqual(["/dashboard?lang=es-ar"]);
  });

  it("offers no link to the removed custom-intake form", async () => {
    await renderFirstStep("es-AR");
    expect(hrefsMatching(/custom-intake/)).toEqual([]);
  });

  it("treats a stale ?plan=custom as the ordinary first step instead of bouncing away", async () => {
    mockLocale = "es-AR";
    mockSearchParamsValue = new URLSearchParams("plan=custom");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });
    expect(mockReplace).not.toHaveBeenCalledWith(
      expect.stringContaining("custom-intake"),
    );
  });
});
