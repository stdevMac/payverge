/** @jest-environment jsdom */
const mockPush = jest.fn();

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    refreshStaffData: jest.fn(),
    refreshSession: jest.fn().mockResolvedValue(true),
  }),
}));
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush, refresh: jest.fn() }),
}));
jest.mock("@/api/auth", () => ({
  authAPI: {
    login: jest.fn(),
    register: jest.fn(),
    getGoogleAuthURL: jest.fn(),
    resendVerification: jest.fn(),
  },
}));
// AuthModal probes GET /platform/registration-mode; answer with the backend
// default (invite) instead of reaching for a real server.
jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn().mockResolvedValue({ data: { registration_mode: "invite" } }),
  },
}));
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    if (key === "authModal.verificationSentKeepGoing") {
      return "We sent a verification link to {{email}} — you can keep going and verify later.";
    }
    return key.replace(/^authModal\./, "");
  },
}));
jest.mock("@/components/staff/StaffLogin", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("@nextui-org/react", () => {
  const React = jest.requireActual("react");
  const Pass = ({ children }: { children?: React.ReactNode }) =>
    React.createElement("div", null, children);
  return {
    Modal: ({
      children,
      isOpen,
    }: {
      children: React.ReactNode;
      isOpen: boolean;
    }) => (isOpen ? React.createElement("div", null, children) : null),
    ModalContent: ({
      children,
    }: {
      children: (onClose: () => void) => React.ReactNode;
    }) =>
      React.createElement(
        "div",
        null,
        typeof children === "function" ? children(() => {}) : children,
      ),
    ModalHeader: Pass,
    ModalBody: Pass,
    Divider: () => React.createElement("hr"),
    Input: React.forwardRef(function MockInput(
      { placeholder, type = "text", value, onChange, endContent }: any,
      ref: any,
    ) {
      return React.createElement(
        "span",
        null,
        React.createElement("input", {
          ref,
          placeholder,
          type,
          value,
          onChange,
        }),
        endContent ?? null,
      );
    }),
    Button: ({ children, type = "button", onPress, onClick }: any) =>
      React.createElement(
        "button",
        { type, onClick: onPress ?? onClick },
        children,
      ),
  };
});

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AuthModal } from "./AuthModal";
import { authAPI } from "@/api/auth";
import toast from "react-hot-toast";

describe("AuthModal — session at registration keeps signups in the funnel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Invite mode only shows the sign-up form for an invite link.
    window.history.replaceState({}, "", "/?invite_code=test-invite");
  });

  it("fires onSuccess and does NOT navigate to /verify-email when register returns a token", async () => {
    (authAPI.register as jest.Mock).mockResolvedValue({
      success: true,
      token: "jwt-token",
      verification_required: true,
      email: "founder@example.com",
      message: "Account created.",
    });
    const onSuccess = jest.fn();

    render(
      <AuthModal isOpen onClose={jest.fn()} defaultTab="signup" onSuccess={onSuccess} />,
    );

    fireEvent.change(screen.getByPlaceholderText("enterName"), {
      target: { value: "Founder" },
    });
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "founder@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("createPasswordPlaceholder"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    await waitFor(() => expect(onSuccess).toHaveBeenCalled());

    // No ejection to the verification page.
    expect(mockPush).not.toHaveBeenCalledWith("/verify-email");
    expect(mockPush).not.toHaveBeenCalledWith(
      expect.stringContaining("/verify-email"),
    );

    // An inline "keep going, verify later" notice is surfaced with the email.
    expect(toast.success).toHaveBeenCalledWith(
      "We sent a verification link to founder@example.com — you can keep going and verify later.",
    );
  });

  it("sends the active operator locale so the verification email localizes", async () => {
    (authAPI.register as jest.Mock).mockResolvedValue({
      success: true,
      token: "jwt-token",
      verification_required: true,
      email: "founder@example.com",
      message: "Account created.",
    });

    render(
      <AuthModal isOpen onClose={jest.fn()} defaultTab="signup" onSuccess={jest.fn()} />,
    );

    fireEvent.change(screen.getByPlaceholderText("enterName"), {
      target: { value: "Founder" },
    });
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "founder@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("createPasswordPlaceholder"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    await waitFor(() =>
      expect(authAPI.register).toHaveBeenCalledWith({
        email: "founder@example.com",
        password: "password123",
        name: "Founder",
        invite_code: "test-invite",
        language: "en",
      }),
    );
  });
});
