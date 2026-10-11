/** @jest-environment jsdom */
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const mockUseAuth = jest.fn();
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockUseAuth(),
}));
jest.mock("@/api/auth", () => ({
  authAPI: { resendVerification: jest.fn() },
}));
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

jest.mock("@/i18n/operatorChromeCatalog", () => ({
  getChromeTranslation: (key: string) => key.replace(/^authModal\./, ""),
}));
jest.mock("@nextui-org/react", () => {
  const React = jest.requireActual("react");
  return {
    Button: ({ children, onPress, isLoading }: any) =>
      React.createElement(
        "button",
        { type: "button", onClick: onPress, "data-loading": !!isLoading },
        children,
      ),
  };
});

import VerifyEmailBanner from "../VerifyEmailBanner";
import { authAPI } from "@/api/auth";
import toast from "react-hot-toast";

const unverified = {
  isOAuthUser: true,
  emailVerified: false,
  oauthData: { userId: 1, email: "founder@example.com", role: "user" },
};

describe("VerifyEmailBanner", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    try {
      sessionStorage.clear();
    } catch {}
  });

  it("renders for an authenticated user with an unverified email", () => {
    mockUseAuth.mockReturnValue(unverified);
    render(<VerifyEmailBanner />);
    expect(screen.getByText("verifyBannerText")).toBeInTheDocument();
  });

  it("does not render when the email is verified", () => {
    mockUseAuth.mockReturnValue({ ...unverified, emailVerified: true });
    const { container } = render(<VerifyEmailBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("does not render for non-user (web3/staff) sessions", () => {
    mockUseAuth.mockReturnValue({ ...unverified, isOAuthUser: false });
    const { container } = render(<VerifyEmailBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("calls the resend API with the session email", async () => {
    (authAPI.resendVerification as jest.Mock).mockResolvedValue({
      message: "ok",
    });
    mockUseAuth.mockReturnValue(unverified);
    render(<VerifyEmailBanner />);

    fireEvent.click(screen.getByText("verifyBannerResend"));

    await waitFor(() =>
      expect(authAPI.resendVerification).toHaveBeenCalledWith(
        "founder@example.com",
      ),
    );
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("verifyBannerResent"),
    );
  });

  it("hides after dismiss and persists dismissal in sessionStorage", () => {
    mockUseAuth.mockReturnValue(unverified);
    const { container } = render(<VerifyEmailBanner />);
    fireEvent.click(screen.getByLabelText("verifyBannerDismiss"));
    expect(container).toBeEmptyDOMElement();
    expect(sessionStorage.getItem("payverge_verify_banner_dismissed")).toBe(
      "1",
    );
  });
});
