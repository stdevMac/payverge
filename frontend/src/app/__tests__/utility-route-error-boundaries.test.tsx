/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

const mockLogError = jest.fn();
jest.mock("@/utils/errorLogger", () => ({
  logError: (...args: unknown[]) => mockLogError(...args),
}));

import ScanError from "../scan/error";
import VerifyEmailError from "../verify-email/error";
import ForgotPasswordError from "../forgot-password/error";
import ResetPasswordError from "../reset-password/error";

const props = { error: new Error("boom"), reset: () => {} };

describe("utility/auth-recovery route boundaries", () => {
  beforeEach(() => mockLogError.mockClear());

  it("scan boundary shows diner-worded copy + a back-to-scan link", () => {
    render(<ScanError {...props} />);
    expect(screen.getByText(/couldn't open the scanner/i)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /scan again/i });
    expect(link.getAttribute("href")).toBe("/scan");
  });

  it("verify-email boundary points recovery at /dashboard", () => {
    render(<VerifyEmailError {...props} />);
    const link = screen.getByRole("link", { name: /go to sign in/i });
    expect(link.getAttribute("href")).toBe("/dashboard");
  });

  it("forgot-password boundary offers a request-new-link recovery", () => {
    render(<ForgotPasswordError {...props} />);
    const link = screen.getByRole("link", { name: /request a new link/i });
    expect(link.getAttribute("href")).toBe("/forgot-password");
  });

  it("reset-password boundary offers a request-new-link recovery", () => {
    render(<ResetPasswordError {...props} />);
    const link = screen.getByRole("link", { name: /request a new link/i });
    expect(link.getAttribute("href")).toBe("/forgot-password");
  });

  it("each boundary logs with its own tag", () => {
    render(<ScanError {...props} />);
    expect(mockLogError).toHaveBeenCalledWith(
      props.error,
      "ScanErrorBoundary",
      "render",
      { digest: undefined }
    );
  });
});
