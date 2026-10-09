/** @jest-environment jsdom */
const mockPush = jest.fn();
const mockRefreshSession = jest.fn().mockResolvedValue(true);

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    refreshStaffData: jest.fn(),
    refreshSession: mockRefreshSession,
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
  getTranslation: (key: string) => key.replace(/^authModal\./, ""),
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
import toast from "react-hot-toast";
import { AuthModal } from "./AuthModal";
import { authAPI } from "@/api/auth";
import { SESSION_HINT_KEY } from "@/utils/refreshAuth";

// P0-1: authAPI.register/login set the httpOnly session cookie but the
// provider stays anonymous on public surfaces (no session hint → bootstrap
// skipped). AuthModal must set the hint AND hydrate the provider via
// refreshSession() BEFORE handing control back, or the register funnel's
// checkout guard loops back to this modal forever.
describe("AuthModal — email/password auth hydrates the provider session", () => {
  beforeEach(() => {
    // Invite mode only shows the sign-up form for an invite link.
    window.history.replaceState({}, "", "/?invite_code=test-invite");
    jest.clearAllMocks();
    mockRefreshSession.mockResolvedValue(true);
    try {
      localStorage.clear();
    } catch {}
  });

  function fillAndSubmitSignup() {
    fireEvent.change(screen.getByPlaceholderText("enterName"), {
      target: { value: "Founder" },
    });
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "founder@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("createPasswordPlaceholder"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);
  }

  it("register success sets the session hint and awaits refreshSession before onSuccess", async () => {
    (authAPI.register as jest.Mock).mockResolvedValue({
      success: true,
      token: "jwt-token",
      email: "founder@example.com",
    });
    const onSuccess = jest.fn();

    render(
      <AuthModal isOpen onClose={jest.fn()} defaultTab="signup" onSuccess={onSuccess} />,
    );
    fillAndSubmitSignup();

    await waitFor(() => expect(onSuccess).toHaveBeenCalled());

    expect(mockRefreshSession).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem(SESSION_HINT_KEY)).toBe("1");
    // The provider must be hydrated BEFORE the funnel advances a step.
    expect(mockRefreshSession.mock.invocationCallOrder[0]).toBeLessThan(
      onSuccess.mock.invocationCallOrder[0],
    );
  });

  it("login success sets the session hint and awaits refreshSession before onSuccess", async () => {
    (authAPI.login as jest.Mock).mockResolvedValue({
      success: true,
      token: "jwt-token",
      email: "founder@example.com",
    });
    const onSuccess = jest.fn();

    render(
      <AuthModal isOpen onClose={jest.fn()} defaultTab="signin" onSuccess={onSuccess} />,
    );
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "founder@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("enterPassword"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    await waitFor(() => expect(onSuccess).toHaveBeenCalled());

    expect(mockRefreshSession).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem(SESSION_HINT_KEY)).toBe("1");
    expect(mockRefreshSession.mock.invocationCallOrder[0]).toBeLessThan(
      onSuccess.mock.invocationCallOrder[0],
    );
  });

  it("login does not close or navigate when refreshSession reports hydration failure", async () => {
    (authAPI.login as jest.Mock).mockResolvedValue({
      success: true,
      token: "jwt-token",
      email: "founder@example.com",
    });
    mockRefreshSession.mockResolvedValueOnce(false);
    const onSuccess = jest.fn();
    const onClose = jest.fn();

    render(
      <AuthModal isOpen onClose={onClose} defaultTab="signin" onSuccess={onSuccess} />,
    );
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "founder@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("enterPassword"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    await waitFor(() => expect(mockRefreshSession).toHaveBeenCalled());
    expect(onSuccess).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(mockPush).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
    expect(screen.getByText("genericAuthError")).toBeTruthy();
  });

  it("register does not close or navigate when refreshSession reports hydration failure", async () => {
    (authAPI.register as jest.Mock).mockResolvedValue({
      success: true,
      token: "jwt-token",
      email: "founder@example.com",
    });
    mockRefreshSession.mockResolvedValueOnce(false);
    const onSuccess = jest.fn();
    const onClose = jest.fn();

    render(
      <AuthModal isOpen onClose={onClose} defaultTab="signup" onSuccess={onSuccess} />,
    );
    fillAndSubmitSignup();

    await waitFor(() => expect(mockRefreshSession).toHaveBeenCalled());
    expect(onSuccess).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(mockPush).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
    expect(screen.getByText("genericAuthError")).toBeTruthy();
  });

  it("login does not close or navigate when refreshSession throws", async () => {
    (authAPI.login as jest.Mock).mockResolvedValue({
      success: true,
      token: "jwt-token",
      email: "founder@example.com",
    });
    mockRefreshSession.mockRejectedValueOnce(new Error("hydrate exploded"));
    const onSuccess = jest.fn();
    const onClose = jest.fn();

    render(
      <AuthModal isOpen onClose={onClose} defaultTab="signin" onSuccess={onSuccess} />,
    );
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "founder@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("enterPassword"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    await waitFor(() => expect(mockRefreshSession).toHaveBeenCalled());
    expect(onSuccess).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(mockPush).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
    expect(screen.getByText("genericAuthError")).toBeTruthy();
  });
});
