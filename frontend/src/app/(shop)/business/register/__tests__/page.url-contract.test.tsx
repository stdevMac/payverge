/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, fireEvent } from "@testing-library/react";

// --- Mocks ---
// Names MUST start with `mock` so jest.mock() factories can reference them
// (Jest hoists mocks above the file; only mock-prefixed identifiers are allowed
// to be referenced from inside the factory).
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

// Import under test AFTER mocks
import BusinessRegisterPage from "../page";

beforeEach(() => {
  mockReplace.mockClear();
  mockPush.mockClear();
  mockSearchParamsValue = new URLSearchParams();
  try {
    localStorage.clear();
  } catch {}
});

describe("business/register URL contract — restore on mount", () => {
  it("mounting with ?step=business renders the business step", async () => {
    mockSearchParamsValue = new URLSearchParams("step=business");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });
    expect(screen.getByText("businessRegister.businessInfo.title")).toBeInTheDocument();
  });

  it("mounting with ?step=auth renders the sign-in step", async () => {
    mockSearchParamsValue = new URLSearchParams("step=auth");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });
    expect(screen.getByText("businessRegister.authStep.title")).toBeInTheDocument();
  });

  it("mounting with ?step=invalid falls back to the business step", async () => {
    mockSearchParamsValue = new URLSearchParams("step=bogus");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });
    expect(screen.getByText("businessRegister.businessInfo.title")).toBeInTheDocument();
  });

  it("a stale ?step=plan link lands on the business step", async () => {
    mockSearchParamsValue = new URLSearchParams("step=plan&plan=ai_pro");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });
    expect(screen.getByText("businessRegister.businessInfo.title")).toBeInTheDocument();
  });
});

describe("business/register URL contract — write on transition", () => {
  it("clicking Back on the auth step pushes ?step=business", async () => {
    mockSearchParamsValue = new URLSearchParams("step=auth");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });
    const backBtn = screen.getByRole("button", { name: /businessRegister\.navigation\.back/i });
    await act(async () => {
      fireEvent.click(backBtn);
    });
    expect(mockPush).toHaveBeenCalled();
    const lastCall = String(mockPush.mock.calls.at(-1)?.[0] ?? "");
    expect(lastCall).toContain("step=business");
  });
});

describe("business/register URL contract — browser Back", () => {
  it("Back to the bare URL (no ?step=) resets to the business step", async () => {
    mockSearchParamsValue = new URLSearchParams("step=auth");
    const view = render(<BusinessRegisterPage />);
    await act(async () => {});
    expect(screen.getByText("businessRegister.authStep.title")).toBeInTheDocument();

    // Browser Back: URL loses the step param entirely.
    mockSearchParamsValue = new URLSearchParams();
    await act(async () => {
      view.rerender(<BusinessRegisterPage />);
    });
    expect(screen.getByText("businessRegister.businessInfo.title")).toBeInTheDocument();
  });
});
