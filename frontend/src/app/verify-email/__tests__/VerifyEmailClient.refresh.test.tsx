/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

let mockSearchParamsValue = new URLSearchParams("token=tok_valid");
jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: jest.fn(), back: jest.fn(), push: jest.fn() }),
  useSearchParams: () => mockSearchParamsValue,
}));

const mockVerifyEmail = jest.fn();
const mockResendVerification = jest.fn();
jest.mock("@/api/auth", () => ({
  authAPI: {
    verifyEmail: (...args: unknown[]) => mockVerifyEmail(...args),
    resendVerification: (...args: unknown[]) => mockResendVerification(...args),
  },
}));

const mockRefreshSession = jest.fn().mockResolvedValue(undefined);
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ refreshSession: mockRefreshSession }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

import VerifyEmailClient from "../VerifyEmailClient";

describe("VerifyEmailClient — provider refresh on verify success", () => {
  beforeEach(() => {
    mockVerifyEmail.mockReset();
    mockRefreshSession.mockClear();
  });

  it("calls refreshSession() after a successful verification so the emailVerified banner clears in-tab", async () => {
    mockVerifyEmail.mockResolvedValue({ message: "Email verified successfully" });
    render(<VerifyEmailClient />);

    await waitFor(() =>
      expect(
        screen.getByText("verifyEmail.messages.successDefault"),
      ).toBeInTheDocument(),
    );
    expect(mockRefreshSession).toHaveBeenCalledTimes(1);
  });

  it("does NOT refresh on verification failure", async () => {
    mockVerifyEmail.mockRejectedValue(new Error("Invalid or expired verification token"));
    render(<VerifyEmailClient />);

    await waitFor(() =>
      expect(screen.getByText("verifyEmail.headings.error")).toBeInTheDocument(),
    );
    expect(mockRefreshSession).not.toHaveBeenCalled();
  });
});
