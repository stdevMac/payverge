/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

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
    useUserStore: Object.assign(() => ({ user: null }), {
      getState: () => ({ setUser }),
    }),
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

const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;

async function flushStepReveal() {
  await act(async () => {
    await new Promise<void>((resolve) => {
      requestAnimationFrame(() => resolve());
    });
  });
}

beforeEach(() => {
  mockReplace.mockClear();
  mockPush.mockClear();
  mockSearchParamsValue = new URLSearchParams();
  HTMLElement.prototype.scrollIntoView = jest.fn();
  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
});

afterEach(() => {
  HTMLElement.prototype.scrollIntoView = originalScrollIntoView;
});

describe("business/register — step heading/progress stays in view", () => {
  it("scrolls the progress region and focuses the step heading after a step change", async () => {
    mockSearchParamsValue = new URLSearchParams("step=auth");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });
    await flushStepReveal();

    const region = screen.getByTestId("register-step-context");
    const scrollIntoView = jest.spyOn(region, "scrollIntoView");
    scrollIntoView.mockClear();

    const backBtn = screen.getByRole("button", {
      name: /businessRegister\.navigation\.back/i,
    });
    await act(async () => {
      fireEvent.click(backBtn);
    });
    await flushStepReveal();

    expect(
      screen.getByText("businessRegister.businessInfo.title"),
    ).toBeInTheDocument();
    expect(scrollIntoView).toHaveBeenCalled();
    expect(scrollIntoView).toHaveBeenCalledWith(
      expect.objectContaining({ block: "start" }),
    );
    expect(
      screen.getByRole("heading", {
        name: "businessRegister.businessInfo.title",
      }),
    ).toHaveFocus();

    const pushed = String(mockPush.mock.calls.at(-1)?.[0] ?? "");
    expect(pushed).toContain("step=business");
  });

  it("reveals the heading/progress when landing on a URL-persisted step", async () => {
    mockSearchParamsValue = new URLSearchParams("step=business");
    await act(async () => {
      render(<BusinessRegisterPage />);
    });
    await flushStepReveal();

    const heading = await screen.findByRole("heading", {
      name: "businessRegister.businessInfo.title",
    });
    const region = screen.getByTestId("register-step-context");

    await waitFor(() => {
      expect(region.scrollIntoView).toHaveBeenCalled();
    });
    expect(heading).toHaveFocus();
    expect(mockPush).not.toHaveBeenCalled();
  });
});
