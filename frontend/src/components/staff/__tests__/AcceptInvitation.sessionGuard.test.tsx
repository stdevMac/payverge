/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";

const mockLogout = jest.fn().mockResolvedValue(undefined);
const mockClearStaffSession = jest.fn().mockResolvedValue(undefined);
const mockAuth = {
  refreshStaffData: jest.fn(),
  isInitialized: true,
  isStaffUser: false,
  isOAuthUser: true,
  isWeb3User: false,
  oauthData: { email: "owner@resto.test" },
  staffData: null,
  walletAddress: null,
};

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  useSearchParams: () => new URLSearchParams("token=tok-123"),
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));
// Stable identity required: AcceptInvitation's load effect depends on
// localizeError. A new function per render would re-fire setStep("loading")
// forever (real useApiErrorMessage returns useCallback).
jest.mock("@/i18n/useApiErrorMessage", () => {
  const localizeError = (error: unknown) =>
    error instanceof Error ? error.message : "error";
  return {
    useApiErrorMessage: () => localizeError,
  };
});
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockAuth,
}));
jest.mock("@/utils/staffAuth", () => ({
  setStaffData: jest.fn(),
  clearStaffSession: (...args: unknown[]) => mockClearStaffSession(...args),
}));
jest.mock("@/api/auth", () => ({
  logout: (...args: unknown[]) => mockLogout(...args),
}));
jest.mock("../../../api/staff", () => ({
  getInvitationPreview: jest.fn().mockResolvedValue({
    email: "hire@resto.test",
    name: "New Hire",
    role: "server",
    business_id: 1,
    business_name: "Resto",
    business_custom_url: "resto",
    business_slug: "resto-a1b2",
    expires_at: new Date(Date.now() + 86400000).toISOString(),
  }),
  acceptInvitation: jest.fn(),
}));

import AcceptInvitation from "../AcceptInvitation";

describe("AcceptInvitation — existing session guard (P2-22)", () => {
  it("blocks the accept form behind an explicit sign-out when a session exists", async () => {
    render(<AcceptInvitation token="tok-123" />);

    // Preview loads, then the guard renders instead of the accept form.
    const signOutCta = await screen.findByRole("button", {
      name: /existingSession\.signOutCta/,
    });
    expect(screen.getByText(/owner@resto\.test/)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /form\.buttons\.accept/ }),
    ).toBeNull();

    fireEvent.click(signOutCta);
    await waitFor(() => expect(mockLogout).toHaveBeenCalledTimes(1));

    // After the explicit sign-out the normal accept form appears.
    expect(
      await screen.findByRole("button", { name: /form\.buttons\.accept/ }),
    ).toBeInTheDocument();
  });
});
