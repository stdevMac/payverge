/** @jest-environment jsdom */
import React from "react";
import { render, screen, act } from "@testing-library/react";

const mockReplace = jest.fn();
let mockSearchParamsValue = new URLSearchParams();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace, back: jest.fn(), push: jest.fn() }),
  useSearchParams: () => mockSearchParamsValue,
}));

// Bare Wagmi connection: wallet is connected, but there is NO backend session.
jest.mock("wagmi", () => ({
  useAccount: () => ({
    address: "0x1111111111111111111111111111111111111111",
    isConnected: true,
  }),
}));

// Session-less auth provider: a connected wallet does not equal a verified
// principal until SIWE/OAuth/email resolves a `user`.
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
  mockSearchParamsValue = new URLSearchParams();
  try {
    localStorage.clear();
  } catch {}
});

describe("business/register — wallet connect does not skip the auth step", () => {
  it("mounting at ?step=auth with a bare wallet keeps the auth step (no auto-skip)", async () => {
    mockSearchParamsValue = new URLSearchParams(
      "step=auth",
    );
    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    // The auth step renders (getTranslation passthrough → raw keys).
    expect(
      screen.getByText("businessRegister.authStep.title"),
    ).toBeInTheDocument();
    // A bare wallet connection must not auto-skip the auth step.
    expect(
      mockReplace.mock.calls.map((c) => String(c[0])),
    ).not.toContainEqual(expect.stringContaining("step=business"));
  });
});
