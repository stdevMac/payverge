/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams("token=used-token"),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/auth", () => ({
  authAPI: {
    verifyEmail: jest.fn(),
    resendVerification: jest.fn(),
  },
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ refreshSession: jest.fn().mockResolvedValue(undefined) }),
}));

import { authAPI } from "@/api/auth";
import VerifyEmailClient from "./VerifyEmailClient";

describe("VerifyEmailClient — already-verified link (P2-15)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders SUCCESS with the localized already-verified message, no resend form", async () => {
    // mockResolvedValue (not Once): React 18 Strict Mode double-invokes effects.
    (authAPI.verifyEmail as jest.Mock).mockResolvedValue({
      message: "Email already verified",
      already_verified: true,
    });

    render(<VerifyEmailClient />);

    expect(
      await screen.findByText("verifyEmail.headings.success"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("verifyEmail.messages.alreadyVerified"),
    ).toBeInTheDocument();
    // The resend form is an error-state affordance — must not show on success.
    expect(
      screen.queryByText("verifyEmail.resend.submit"),
    ).not.toBeInTheDocument();
  });

  it("renders the plain success message on a first-time verification", async () => {
    (authAPI.verifyEmail as jest.Mock).mockResolvedValue({
      message: "Email verified successfully",
    });

    render(<VerifyEmailClient />);

    expect(
      await screen.findByText("verifyEmail.headings.success"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("verifyEmail.messages.successDefault"),
    ).toBeInTheDocument();
  });
});
