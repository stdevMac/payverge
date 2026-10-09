/** @jest-environment jsdom */
// Mock heavy transitive deps so importing AuthModal (for the pure helper) does
// not pull in wagmi/ESM modules Jest cannot transform.
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    refreshStaffData: jest.fn(),
    refreshSession: jest.fn().mockResolvedValue(true),
  }),
}));
const mockRouterPush = jest.fn();
const mockRouterRefresh = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockRouterPush, refresh: mockRouterRefresh }),
}));
jest.mock("@/api/auth", () => ({
  authAPI: {
    login: jest.fn(),
    register: jest.fn(),
    getGoogleAuthURL: jest.fn(),
    resendVerification: jest.fn(),
  },
}));
// The registration-mode probe goes through the shared axios instance; the
// real @/api/registrationMode module runs against this stub.
const mockInstanceGet = jest.fn();
jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: (...args: unknown[]) => mockInstanceGet(...args) },
}));
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
// Identity-ish translation: return the bare key so assertions are stable.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key.replace(/^authModal\./, ""),
}));
// StaffLogin pulls in more deps; stub it out for the email-auth tests.
jest.mock("@/components/staff/StaffLogin", () => ({
  __esModule: true,
  default: () => null,
}));

// Lightweight NextUI stand-ins so the modal renders fast/deterministically in
// jsdom (the real Modal + framer-motion render is slow enough to time out).
jest.mock("@nextui-org/react", () => {
  const React = jest.requireActual("react");
  const Pass = ({ children }: { children?: React.ReactNode }) =>
    React.createElement("div", null, children);
  const Input = React.forwardRef(
    ({ placeholder, label, type = "text", value, onChange, endContent, autoComplete, required, description }: any, ref: any) =>
      React.createElement(
        "span",
        null,
        React.createElement("input", {
          ref,
          placeholder,
          "aria-label": label,
          type,
          value,
          onChange,
          autoComplete,
          required,
        }),
        description ? React.createElement("small", null, description) : null,
        endContent ?? null,
      ),
  );
  Input.displayName = "MockNextUIInput";
  return {
    Modal: ({ children, isOpen }: { children: React.ReactNode; isOpen: boolean }) =>
      isOpen ? React.createElement("div", null, children) : null,
    ModalContent: ({ children }: { children: (onClose: () => void) => React.ReactNode }) =>
      React.createElement("div", null, typeof children === "function" ? children(() => {}) : children),
    ModalHeader: Pass,
    ModalBody: Pass,
    Divider: () => React.createElement("hr"),
    Input,
    Button: ({ children, type = "button", onPress, onClick, "aria-describedby": describedBy }: any) =>
      React.createElement(
        "button",
        { type, onClick: onPress ?? onClick, "aria-describedby": describedBy },
        children,
      ),
  };
});

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AuthModal, readVerificationError } from "./AuthModal";
import { authAPI } from "@/api/auth";
import { resetRegistrationModeCacheForTests } from "@/api/registrationMode";

beforeEach(() => {
  resetRegistrationModeCacheForTests();
  mockInstanceGet.mockReset();
  // Suites that predate REGISTRATION_MODE never see the probe answer, so the
  // form keeps its permissive default (invite field shown, optional) and no
  // state update lands outside act(). Mode tests opt in below.
  mockInstanceGet.mockReturnValue(new Promise(() => {}));
});

function mockRegistrationMode(mode: string) {
  mockInstanceGet.mockResolvedValue({ data: { registration_mode: mode } });
}

function fillSignup(email = "alex@example.com") {
  fireEvent.change(screen.getByPlaceholderText("enterName"), {
    target: { value: "Alex Owner" },
  });
  const emailInput = screen.getByPlaceholderText("enterEmail");
  fireEvent.change(emailInput, { target: { value: email } });
  fireEvent.change(screen.getByPlaceholderText("createPasswordPlaceholder"), {
    target: { value: "strong-password" },
  });
  return emailInput.closest("form") as HTMLFormElement;
}

describe("readVerificationError", () => {
  it("reads verification fields from params", () => {
    const data = {
      error: "Email not verified. We sent you a new verification email.",
      code: "AUTH_FORBIDDEN",
      params: { requires_email_verification: true, verification_url: "/verify-email?token=abc" },
    };
    expect(readVerificationError(data)).toEqual({
      requiresVerification: true,
      message: "Email not verified. We sent you a new verification email.",
      verificationUrl: "/verify-email?token=abc",
    });
  });

  it("falls back to top-level fields (pre-migration body)", () => {
    const data = {
      error: "Email not verified.",
      requires_email_verification: true,
      verification_url: "/verify-email?token=xyz",
    };
    expect(readVerificationError(data)).toEqual({
      requiresVerification: true,
      message: "Email not verified.",
      verificationUrl: "/verify-email?token=xyz",
    });
  });

  it("returns requiresVerification=false for ordinary errors", () => {
    expect(
      readVerificationError({ error: "Invalid email or password", code: "AUTH_TOKEN_INVALID" })
        .requiresVerification,
    ).toBe(false);
  });
});

describe("AuthModal open-redirect guard (#296)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/");
    try {
      localStorage.clear();
    } catch {
      /* jsdom */
    }
  });

  it("does not navigate to an absolute redirectUrl after login", async () => {
    (authAPI.login as jest.Mock).mockResolvedValue({
      success: true,
      token: "session",
    });

    render(
      <AuthModal
        isOpen
        onClose={jest.fn()}
        defaultTab="signin"
        redirectUrl="https://evil.example/phish"
      />,
    );

    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "diner@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("enterPassword"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    await waitFor(() => {
      expect(mockRouterPush).toHaveBeenCalledWith("/dashboard");
    });
    expect(mockRouterPush).not.toHaveBeenCalledWith(
      "https://evil.example/phish",
    );
  });

  it("forwards a safe dashboard deep-link to forgot-password (#403)", () => {
    const dest = "/business/1/dashboard?tab=accounting&sub=invoices";
    render(
      <AuthModal
        isOpen
        onClose={jest.fn()}
        defaultTab="signin"
        redirectUrl={dest}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "forgotPassword" }));
    expect(mockRouterPush).toHaveBeenCalledWith(
      `/forgot-password?redirect=${encodeURIComponent(dest)}`,
    );
  });

  it("reads the current ?redirect= query when the prop is absent (#403)", () => {
    window.history.replaceState(
      {},
      "",
      "/dashboard?redirect=%2Fbusiness%2F1%2Fdashboard%3Ftab%3Daccounting%26sub%3Dinvoices",
    );
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signin" />);
    fireEvent.click(screen.getByRole("button", { name: "forgotPassword" }));
    expect(mockRouterPush).toHaveBeenCalledWith(
      `/forgot-password?redirect=${encodeURIComponent("/business/1/dashboard?tab=accounting&sub=invoices")}`,
    );
  });

  it("does not forward an external redirect to forgot-password (#403)", () => {
    render(
      <AuthModal
        isOpen
        onClose={jest.fn()}
        defaultTab="signin"
        redirectUrl="https://evil.example/phish"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "forgotPassword" }));
    expect(mockRouterPush).toHaveBeenCalledWith("/forgot-password");
  });
});

describe("AuthModal silent-failure guard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/");
  });

  it("surfaces a generic error when sign-in returns 200 with success:false", async () => {
    (authAPI.login as jest.Mock).mockResolvedValue({ success: false });

    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signin" />);

    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "diner@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("enterPassword"), {
      target: { value: "password123" },
    });
    // The email/password form owns the submit button (type=submit). Submit the
    // form directly to avoid colliding with the "signIn" tab toggle button.
    const form = emailInput.closest("form");
    expect(form).not.toBeNull();
    fireEvent.submit(form as HTMLFormElement);

    await waitFor(() => {
      expect(screen.getByText("genericAuthError")).toBeInTheDocument();
    });
  });

  it("shows a resend-verification nudge when sign-in fails on an unverified email", async () => {
    (authAPI.login as jest.Mock).mockRejectedValue({
      response: {
        data: {
          error: "Email not verified. Check your inbox.",
          code: "AUTH_FORBIDDEN",
          params: {
            requires_email_verification: true,
            verification_url: "/verify-email?token=abc",
          },
        },
      },
    });
    (authAPI.resendVerification as jest.Mock).mockResolvedValue({
      message: "sent",
    });

    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signin" />);

    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "diner@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("enterPassword"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    // Specific message + a resend action, not the generic failure.
    const resendBtn = await screen.findByRole("button", {
      name: "resendVerification",
    });
    expect(screen.getByText("Email not verified. Check your inbox.")).toBeInTheDocument();
    expect(screen.queryByText("genericAuthError")).not.toBeInTheDocument();

    fireEvent.click(resendBtn);
    await waitFor(() =>
      expect(authAPI.resendVerification).toHaveBeenCalledWith(
        "diner@example.com",
      ),
    );
  });

  it("announces the live signup requirement and focuses a short password", async () => {
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);

    fireEvent.change(screen.getByPlaceholderText("enterName"), {
      target: { value: "Alex Owner" },
    });
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "alex@example.com" } });
    const passwordInput = screen.getByPlaceholderText("createPasswordPlaceholder");
    fireEvent.change(passwordInput, { target: { value: "short" } });

    expect(screen.getByRole("status")).toHaveTextContent("passwordRequirement");
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    await waitFor(() => expect(passwordInput).toHaveFocus());
    expect(screen.getByRole("alert")).toHaveTextContent("passwordTooShort");
    expect(authAPI.register).not.toHaveBeenCalled();
  });

  it("prefills and submits the launch invite from an invite link", async () => {
    window.history.replaceState({}, "", "/?invite_code=cohort-ALPHA");
    (authAPI.register as jest.Mock).mockResolvedValue({ success: false });

    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);

    const inviteInput = await screen.findByPlaceholderText("enterInviteCode");
    expect(inviteInput).toHaveValue("cohort-ALPHA");
    fireEvent.change(screen.getByPlaceholderText("enterName"), {
      target: { value: "Alex Owner" },
    });
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "alex@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("createPasswordPlaceholder"), {
      target: { value: "strong-password" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);

    await waitFor(() =>
      expect(authAPI.register).toHaveBeenCalledWith(
        expect.objectContaining({ invite_code: "cohort-ALPHA" }),
      ),
    );
  });
});

describe("AuthModal login transport error (issue 363)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/");
  });

  async function submitSignIn() {
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signin" />);
    const emailInput = screen.getByPlaceholderText("enterEmail");
    fireEvent.change(emailInput, { target: { value: "diner@example.com" } });
    fireEvent.change(screen.getByPlaceholderText("enterPassword"), {
      target: { value: "password123" },
    });
    fireEvent.submit(emailInput.closest("form") as HTMLFormElement);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  }

  it("surfaces one honest transport error for net::ERR_FAILED, not genericAuthError", async () => {
    const toast = require("react-hot-toast").default;
    (authAPI.login as jest.Mock).mockRejectedValue({
      message: "net::ERR_FAILED",
      code: "ERR_FAILED",
    });

    await submitSignIn();

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("networkError");
    expect(screen.queryByText("genericAuthError")).not.toBeInTheDocument();
    expect(screen.queryByText("tooManySignInAttempts")).not.toBeInTheDocument();
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("recognizes the interceptor's sanitized network copy as a transport failure", async () => {
    const toast = require("react-hot-toast").default;
    (authAPI.login as jest.Mock).mockRejectedValue({
      message:
        "Unable to connect to the server. Please check your internet connection and try again.",
    });

    await submitSignIn();

    expect(await screen.findByRole("alert")).toHaveTextContent("networkError");
    expect(screen.queryByText("genericAuthError")).not.toBeInTheDocument();
    expect(toast.error).not.toHaveBeenCalled();
  });
});

describe("AuthModal REGISTRATION_MODE", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/");
  });

  it("reads the mode from the public platform probe", async () => {
    mockRegistrationMode("invite");
    window.history.replaceState({}, "", "/?invite_code=cohort-ALPHA");
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);
    expect(await screen.findByText("inviteCodeHint")).toBeInTheDocument();
    expect(mockInstanceGet).toHaveBeenCalledWith(
      "/platform/registration-mode",
      expect.objectContaining({ _skipErrorToast: true }),
    );
  });

  it("invite: an invite link opens the form; the code is required and explained", async () => {
    mockRegistrationMode("invite");
    window.history.replaceState({}, "", "/?invite_code=cohort-ALPHA");
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);

    await waitFor(() =>
      expect(screen.getByPlaceholderText("enterInviteCode")).toBeRequired(),
    );
    expect(screen.getByText("inviteCodeHint")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("enterInviteCode")).toHaveValue("cohort-ALPHA");
    expect(screen.getByRole("button", { name: "createAccount" })).toBeInTheDocument();
  });

  it("invite without a link: no public form, an ask-your-admin notice instead", async () => {
    mockRegistrationMode("invite");
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);

    expect(await screen.findByText("inviteOnlyBody")).toBeInTheDocument();
    expect(screen.getByText("inviteOnlyTitle")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("enterName")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "signUp" })).not.toBeInTheDocument();

    // A code delivered out of band still works.
    fireEvent.click(screen.getByRole("button", { name: "haveInviteCode" }));
    expect(screen.getByPlaceholderText("enterInviteCode")).toBeRequired();
    expect(screen.getByRole("button", { name: "createAccount" })).toBeInTheDocument();
  });

  it("invite: sign-in offers no sign-up tab, only the ask-your-admin hint", async () => {
    mockRegistrationMode("invite");
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signin" />);

    expect(await screen.findByText(/inviteOnlySignInHint/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "signUp" })).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText("enterPassword")).toBeInTheDocument();
  });

  it("open: no invite field and signup submits without one", async () => {
    mockRegistrationMode("open");
    (authAPI.register as jest.Mock).mockResolvedValue({ success: false });
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);

    await waitFor(() =>
      expect(screen.queryByPlaceholderText("enterInviteCode")).not.toBeInTheDocument(),
    );
    fireEvent.submit(fillSignup());

    await waitFor(() => expect(authAPI.register).toHaveBeenCalledTimes(1));
    expect((authAPI.register as jest.Mock).mock.calls[0][0].invite_code).toBeUndefined();
  });

  it("closed: signup is replaced by a sign-in-only notice", async () => {
    mockRegistrationMode("closed");
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);

    expect(await screen.findByText("registrationClosedBody")).toBeInTheDocument();
    expect(screen.getByText("registrationClosedTitle")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("enterName")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "createAccount" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "signUp" })).not.toBeInTheDocument();
    expect(screen.queryByText("continueWithGoogle")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "signInInstead" }));

    expect(screen.getByPlaceholderText("enterEmail")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("enterPassword")).toBeInTheDocument();
    expect(screen.queryByText("registrationClosedBody")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "signUp" })).not.toBeInTheDocument();
  });

  it("closed: sign-in keeps working for existing accounts", async () => {
    mockRegistrationMode("closed");
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signin" />);

    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "signUp" })).not.toBeInTheDocument(),
    );
    expect(screen.getByPlaceholderText("enterEmail")).toBeInTheDocument();
    expect(screen.getByText("continueWithGoogle")).toBeInTheDocument();
    // ADM-3: Google stays for existing accounts but says it cannot sign up.
    const hint = screen.getByText("googleExistingAccountsOnly");
    expect(screen.getByText("continueWithGoogle").closest("button")).toHaveAttribute(
      "aria-describedby",
      hint.id,
    );
  });

  it("invite/open: the Google button carries no existing-accounts-only hint", async () => {
    mockRegistrationMode("invite");
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signin" />);
    await waitFor(() => expect(mockInstanceGet).toHaveBeenCalled());
    await screen.findByText(/inviteOnlySignInHint/);
    expect(screen.getByText("continueWithGoogle")).toBeInTheDocument();
    expect(screen.queryByText("googleExistingAccountsOnly")).not.toBeInTheDocument();
  });

  it("a registration_closed refusal switches the open form to the notice", async () => {
    mockRegistrationMode("invite");
    (authAPI.register as jest.Mock).mockRejectedValue({
      response: {
        status: 403,
        data: {
          error: "Registration is closed on this instance",
          code: "FORBIDDEN",
          params: { reason: "registration_closed" },
        },
      },
    });
    window.history.replaceState({}, "", "/?invite_code=cohort-ALPHA");
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);
    expect(await screen.findByText("inviteCodeHint")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("enterInviteCode"), {
      target: { value: "cohort-ALPHA" },
    });
    fireEvent.submit(fillSignup());

    expect(await screen.findByText("registrationClosedBody")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("unknown mode (probe failed) keeps the invite field optional", async () => {
    mockInstanceGet.mockRejectedValue(new Error("offline"));
    render(<AuthModal isOpen onClose={jest.fn()} defaultTab="signup" />);

    await waitFor(() => expect(mockInstanceGet).toHaveBeenCalled());
    const invite = screen.getByPlaceholderText("enterInviteCode");
    expect(invite).not.toBeRequired();
    expect(screen.queryByText("inviteCodeHint")).not.toBeInTheDocument();
  });
});
