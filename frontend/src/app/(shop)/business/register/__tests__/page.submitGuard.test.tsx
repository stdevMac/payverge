/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, waitFor, fireEvent } from "@testing-library/react";

const mockReplace = jest.fn();
let mockSearchParamsValue = new URLSearchParams();

// Stable router object — Next's useRouter returns a stable identity, and the
// page's URL-init effect depends on it; a fresh object per render would
// re-fire that effect and clobber programmatic step changes.
const mockRouter = { replace: mockReplace, back: jest.fn(), push: jest.fn() };

jest.mock("next/navigation", () => ({
  useRouter: () => mockRouter,
  useSearchParams: () => mockSearchParamsValue,
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: undefined, isConnected: false }),
}));

// Fully-authenticated OAuth principal so the auth guard inside handleSubmit
// does not bounce to the auth modal — the only thing missing is the country.
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isOAuthUser: true,
    isWeb3User: false,
    isStaffUser: false,
    oauthData: { email: "owner@example.com", userId: "user-1" },
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

const mockCreateBusiness = jest.fn();
const mockGetMyBusinesses = jest.fn();
jest.mock("@/api/business", () => ({
  createBusiness: (...args: unknown[]) => mockCreateBusiness(...args),
  // MIN-3: page redirects owners with existing businesses; default empty list
  // so submit-guard tests stay on the wizard unless a case overrides.
  getMyBusinesses: (...args: unknown[]) => mockGetMyBusinesses(...args),
}));

import BusinessRegisterPage from "../page";

beforeEach(() => {
  mockReplace.mockClear();
  mockCreateBusiness.mockClear();
  mockGetMyBusinesses.mockReset();
  mockGetMyBusinesses.mockResolvedValue([]);
  mockSearchParamsValue = new URLSearchParams();
  try {
    localStorage.clear();
    sessionStorage.clear();
  } catch {}
});

describe("business/register — workspace step revalidates the draft", () => {
  it("cannot create a workspace when required country is missing", async () => {
    // A restored draft that filled everything EXCEPT the country must not
    // slip through isStepValid and seed USD/UTC defaults.
    localStorage.setItem(
      "payverge_registration_draft",
      JSON.stringify({
        name: "Café Rio",
        owner_name: "Mara",
        email: "owner@example.com",
        address: { street: "", city: "", state: "", postal_code: "", country: "" },
      }),
    );
    mockSearchParamsValue = new URLSearchParams(
      "step=business",
    );

    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    expect(
      screen.getByText("businessRegister.businessInfo.title"),
    ).toBeInTheDocument();
    const submit = await screen.findByText("businessRegister.navigation.submit");
    await act(async () => {
      fireEvent.click(submit);
    });
    expect(mockCreateBusiness).not.toHaveBeenCalled();
  });

  // MIN-3: authenticated owners who already own a venue skip the register wizard.
  it("redirects to the first owned business dashboard when getMyBusinesses is non-empty", async () => {
    mockGetMyBusinesses.mockResolvedValue([
      { id: 7, business_id: "cafe-luna", name: "Café Luna" },
    ]);

    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    await waitFor(() => {
      expect(mockGetMyBusinesses).toHaveBeenCalled();
      expect(mockReplace).toHaveBeenCalledWith(
        "/business/cafe-luna/dashboard",
      );
    });
    expect(mockCreateBusiness).not.toHaveBeenCalled();
  });

  // Dashboard "Add business" links here with ?new=1: the owner deliberately
  // wants another venue, so neither bounce them nor tell them to go back to
  // the dashboard to do what they are already doing.
  it("keeps an owner with ?new=1 on the wizard without the go-to-dashboard banner", async () => {
    mockGetMyBusinesses.mockResolvedValue([
      { id: 7, business_id: "cafe-luna", name: "Café Luna" },
    ]);
    mockSearchParamsValue = new URLSearchParams("new=1&step=business");

    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    expect(mockReplace).not.toHaveBeenCalledWith(
      "/business/cafe-luna/dashboard",
    );
    expect(screen.queryByText(/alreadyHaveAccount/)).toBeNull();
  });

  // A signed-in user with no venue is here to create one: no banner sending
  // them back to the dashboard.
  it("does not tell a signed-in user without a venue to go to the dashboard", async () => {
    await act(async () => {
      render(<BusinessRegisterPage />);
    });

    expect(screen.queryByText(/alreadyHaveAccount/)).toBeNull();
  });
});
