/** @jest-environment jsdom */
/**
 * Task 21 / findings 26, 27, 63 — the register page has one job.
 *
 * Step 1 must present a single primary CTA and no Back control. There is
 * no plan step: the funnel opens on the venue basics.
 */
import React from "react";
import { render, screen, act } from "@testing-library/react";

const mockReplace = jest.fn();
const mockPush = jest.fn();
let mockSearchParamsValue = new URLSearchParams();

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
  useSimpleLocale: () => ({ locale: "en" }),
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

beforeEach(() => {
  mockReplace.mockClear();
  mockPush.mockClear();
  mockSearchParamsValue = new URLSearchParams();
  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
});

describe("business/register — one job (step 1)", () => {
  it("renders exactly one primary CTA on step 1", async () => {
    mockSearchParamsValue = new URLSearchParams("step=business");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    const primaries = screen.getAllByTestId("register-primary-cta");
    expect(primaries).toHaveLength(1);
    expect(primaries[0]).toHaveTextContent(/businessRegister\.navigation\.next/i);
  });

  it("does not render a Back control on step 1 of the funnel", async () => {
    mockSearchParamsValue = new URLSearchParams("step=business");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    expect(
      screen.queryByRole("button", {
        name: /businessRegister\.navigation\.back/i,
      }),
    ).toBeNull();
  });

  it("opens on the business step for a bare URL and ignores stale ?plan=", async () => {
    mockSearchParamsValue = new URLSearchParams("plan=ai_pro&billing=annual");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "businessRegister.businessInfo.title",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/planStep/)).toBeNull();
    expect(mockReplace).not.toHaveBeenCalledWith(
      expect.stringContaining("plan="),
    );
  });
});
