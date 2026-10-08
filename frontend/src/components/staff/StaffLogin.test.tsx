/** @jest-environment jsdom */
// StaffLogin uses NextUI (Card, Input, Button, Divider) and react-hot-toast.
// We transform NextUI / framer-motion / @react-aria / @react-stately via
// transformIgnorePatterns in jest.config.js so these tests run against the
// real component surface (no inline mock).

import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";

// Mock heavy API deps
jest.mock("../../api/staff", () => ({
  requestLoginCode: jest.fn(),
  verifyLoginCode: jest.fn(),
  getStaffGoogleAuthURL: jest.fn(),
  isMembershipSelectionResponse: (response: { membership_selection_required?: boolean }) =>
    response.membership_selection_required === true,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  toast: { error: jest.fn(), success: jest.fn() },
}));

// Mock i18n provider — by default echo the key so assertions are predictable.
// getTranslation is a jest.fn so individual tests can supply a real template
// (e.g. the {email} placeholder) to exercise interpolation in the component.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: jest.fn((key: string) => key),
}));

// Mock staffAuth utility
jest.mock("@/utils/staffAuth", () => ({
  setStaffData: jest.fn(),
}));

import StaffLogin from "./StaffLogin";
import * as StaffAPI from "../../api/staff";
import { toast } from "react-hot-toast";
import { getTranslation } from "@/i18n/SimpleTranslationProvider";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";

describe("StaffLogin inline validation (fieldErrors)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/staff/login");
  });

  it("renders without crashing", () => {
    const { container } = render(<StaffLogin />);
    expect(container).toBeTruthy();
  });

  it("renders the email input field", () => {
    render(<StaffLogin />);
    const emailInput = document.querySelector("input[type='email']");
    expect(emailInput).toBeInTheDocument();
  });

  it("renders continue button on email step", () => {
    render(<StaffLogin />);
    const buttons = screen.getAllByRole("button");
    expect(buttons.length).toBeGreaterThan(0);
  });

  it("shows toast error and marks field invalid when submitting empty email", async () => {
    render(<StaffLogin />);
    // The send-code button text is the translation key for steps.email.button
    const buttons = screen.getAllByRole("button");
    // Send button is the last button in the email step (after Google)
    const sendBtn = buttons[buttons.length - 1];

    await act(async () => {
      fireEvent.click(sendBtn);
    });

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled();
    });
  });

  it("marks email field invalid (data-invalid) after empty submit", async () => {
    render(<StaffLogin />);
    const buttons = screen.getAllByRole("button");
    const sendBtn = buttons[buttons.length - 1];

    await act(async () => {
      fireEvent.click(sendBtn);
    });

    await waitFor(() => {
      const invalidInput = document.querySelector("[data-invalid='true']");
      expect(invalidInput).toBeInTheDocument();
    });
  });

  it("does not call requestLoginCode when email is empty", async () => {
    render(<StaffLogin />);
    const buttons = screen.getAllByRole("button");
    const sendBtn = buttons[buttons.length - 1];

    await act(async () => {
      fireEvent.click(sendBtn);
    });

    expect(StaffAPI.requestLoginCode).not.toHaveBeenCalled();
  });

  it("clears prior email field errors when a valid send succeeds", async () => {
    (StaffAPI.requestLoginCode as jest.Mock).mockResolvedValueOnce({
      message: "ok",
    });
    render(<StaffLogin />);
    const buttons = screen.getAllByRole("button");
    const sendBtn = buttons[buttons.length - 1];
    // First: empty submit leaves an invalid field banner.
    await act(async () => {
      fireEvent.click(sendBtn);
    });
    await waitFor(() => {
      expect(document.querySelector("[data-invalid='true']")).toBeInTheDocument();
    });
    // Then: type a valid email and send — error must not linger into the code step.
    const emailInput = document.querySelector(
      "input[type='email']",
    ) as HTMLInputElement;
    await act(async () => {
      fireEvent.change(emailInput, { target: { value: "staff@test.com" } });
    });
    await act(async () => {
      fireEvent.click(sendBtn);
    });
    await waitFor(() => {
      expect(StaffAPI.requestLoginCode).toHaveBeenCalled();
    });
    // Code step: no email invalid marker left from the prior failed submit.
    await waitFor(() => {
      expect(document.querySelector("input[type='text']")).toBeInTheDocument();
    });
    expect(document.querySelector("[data-invalid='true']")).not.toBeInTheDocument();
  });

  it("calls requestLoginCode with valid email", async () => {
    (StaffAPI.requestLoginCode as jest.Mock).mockResolvedValueOnce({
      message: "ok",
    });
    render(<StaffLogin />);

    const emailInput = document.querySelector(
      "input[type='email']",
    ) as HTMLInputElement;

    await act(async () => {
      fireEvent.change(emailInput, { target: { value: "staff@test.com" } });
    });

    const buttons = screen.getAllByRole("button");
    const sendBtn = buttons[buttons.length - 1];

    await act(async () => {
      fireEvent.click(sendBtn);
    });

    await waitFor(() => {
      expect(StaffAPI.requestLoginCode).toHaveBeenCalledWith(
        expect.objectContaining({ email: "staff@test.com" }),
      );
    });
  });

  it("requires an authenticated business selection for a multi-business identity", async () => {
    (StaffAPI.requestLoginCode as jest.Mock).mockResolvedValueOnce({ message: "ok" });
    (StaffAPI.verifyLoginCode as jest.Mock)
      .mockResolvedValueOnce({
        message: "select",
        membership_selection_required: true,
        selection_token: "selection-proof",
        memberships: [
          { business_id: 11, business_name: "Cafe One", role: "server" },
          { business_id: 22, business_name: "Cafe Two", role: "manager" },
        ],
      })
      .mockResolvedValueOnce({
        token: "scoped-token",
        staff: { id: 8, business_id: 22, email: "staff@test.com", name: "Staff", role: "manager" },
      });

    const onLoginSuccess = jest.fn();
    render(<StaffLogin onLoginSuccess={onLoginSuccess} />);
    fireEvent.change(document.querySelector("input[type='email']") as HTMLInputElement, {
      target: { value: "staff@test.com" },
    });
    fireEvent.click(screen.getByText("staffLogin.steps.email.button"));
    await waitFor(() => expect(document.querySelector("input[type='text']")).toBeInTheDocument());

    fireEvent.change(document.querySelector("input[type='text']") as HTMLInputElement, {
      target: { value: "123456" },
    });
    fireEvent.click(screen.getByText("staffLogin.steps.code.button"));
    await waitFor(() => expect(screen.getByText("Cafe Two")).toBeInTheDocument());

    fireEvent.click(screen.getByText("Cafe Two"));
    await waitFor(() => {
      expect(StaffAPI.verifyLoginCode).toHaveBeenLastCalledWith({
        selection_token: "selection-proof",
        business_id: 22,
      });
      expect(onLoginSuccess).toHaveBeenCalledWith(expect.objectContaining({ business_id: 22, role: "manager" }));
    });
  });

  it("continues Google OAuth through the signed membership-selection fragment", async () => {
    const payload = {
      selection_token: "oauth-selection-proof",
      memberships: [
        { business_id: 11, business_name: "OAuth Cafe One", role: "server" },
        { business_id: 22, business_name: "OAuth Cafe Two", role: "manager" },
      ],
    };
    window.history.replaceState(
      {},
      "",
      `/staff/login#staff_membership_selection=${encodeURIComponent(JSON.stringify(payload))}`,
    );
    (StaffAPI.verifyLoginCode as jest.Mock).mockResolvedValueOnce({
      token: "scoped-token",
      staff: { id: 8, business_id: 22, email: "staff@test.com", name: "Staff", role: "manager" },
    });

    const onLoginSuccess = jest.fn();
    render(<StaffLogin onLoginSuccess={onLoginSuccess} />);

    await waitFor(() => expect(screen.getByText("OAuth Cafe Two")).toBeInTheDocument());
    expect(window.location.hash).toBe("");
    fireEvent.click(screen.getByText("OAuth Cafe Two"));
    await waitFor(() => {
      expect(StaffAPI.verifyLoginCode).toHaveBeenCalledWith({
        selection_token: "oauth-selection-proof",
        business_id: 22,
      });
      expect(onLoginSuccess).toHaveBeenCalledWith(expect.objectContaining({ business_id: 22 }));
    });
  });

  it("shows error for invalid email format", async () => {
    render(<StaffLogin />);
    const emailInput = document.querySelector(
      "input[type='email']",
    ) as HTMLInputElement;

    await act(async () => {
      fireEvent.change(emailInput, { target: { value: "not-an-email" } });
    });

    const buttons = screen.getAllByRole("button");
    const sendBtn = buttons[buttons.length - 1];

    await act(async () => {
      fireEvent.click(sendBtn);
    });

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled();
    });
    expect(StaffAPI.requestLoginCode).not.toHaveBeenCalled();
  });

  it("renders in modal mode without full-page wrapper", () => {
    const { container } = render(<StaffLogin isModal />);
    expect(container.querySelector(".min-h-screen")).toBeNull();
  });
});

describe("StaffLogin code-step error handling + resend cooldown", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useRealTimers();
  });

  async function reachCodeStep() {
    (StaffAPI.requestLoginCode as jest.Mock).mockResolvedValue({ message: "ok" });
    render(<StaffLogin />);
    const emailInput = document.querySelector(
      "input[type='email']",
    ) as HTMLInputElement;
    await act(async () => {
      fireEvent.change(emailInput, { target: { value: "staff@test.com" } });
    });
    const buttons = screen.getAllByRole("button");
    await act(async () => {
      fireEvent.click(buttons[buttons.length - 1]);
    });
    // Now on the code step: a code input exists.
    await waitFor(() => {
      expect(
        document.querySelector("input[type='text']"),
      ).toBeInTheDocument();
    });
  }

  async function submitCode(value: string) {
    const codeInput = document.querySelector(
      "input[type='text']",
    ) as HTMLInputElement;
    await act(async () => {
      fireEvent.change(codeInput, { target: { value } });
    });
    // The verify (Sign In) button is the first button on the code step.
    const verifyBtn = screen
      .getAllByRole("button")
      .find((b) => b.textContent?.includes("steps.code.button"));
    await act(async () => {
      fireEvent.click(verifyBtn as HTMLElement);
    });
  }

  it("keeps the user on the code step for the ambiguous 'Invalid or expired' error", async () => {
    await reachCodeStep();
    (StaffAPI.verifyLoginCode as jest.Mock).mockRejectedValueOnce(
      Object.assign(new Error("Invalid or expired login code"), {
        response: { status: 400, data: { error: "Invalid or expired login code" } },
      }),
    );

    await submitCode("123456");

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("staffLogin.messages.invalidCode");
    });
    // Still on the code step (code input present) — not bounced to email.
    expect(document.querySelector("input[type='text']")).toBeInTheDocument();
    expect(toast.error).not.toHaveBeenCalledWith("staffLogin.messages.codeExpired");
  });

  it("returns to the email step only for an unambiguous 'expired' error", async () => {
    await reachCodeStep();
    (StaffAPI.verifyLoginCode as jest.Mock).mockRejectedValueOnce(
      Object.assign(new Error("Verification code has expired"), {
        response: { status: 400, data: { error: "Verification code has expired" } },
      }),
    );

    await submitCode("123456");

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("staffLogin.messages.codeExpired");
    });
    // Bounced back to the email step (email input present again).
    await waitFor(() => {
      expect(document.querySelector("input[type='email']")).toBeInTheDocument();
    });
  });

  it("disables the resend button with a countdown after sending the code", async () => {
    await reachCodeStep();
    // On reaching the code step, a 30s cooldown starts; the resend button is
    // disabled and shows the countdown copy.
    const resendBtn = screen
      .getAllByRole("button")
      .find((b) => b.textContent?.includes("steps.code.resendIn"));
    expect(resendBtn).toBeDefined();
    expect(resendBtn).toBeDisabled();
  });
});

// The backend deliberately returns the same 200 "code sent" response whether or
// not an active staff record matches (anti-enumeration). The honest copy below
// keeps that posture but stops the toast from claiming an email was definitely
// sent, and echoes the exact (normalized) address back so a staff member who
// mistyped can spot the mistake.
describe("StaffLogin honest 'login code sent' copy", () => {
  const EMAIL_SENT_TEMPLATE =
    "If a staff account exists for {email}, we've sent a login code. Check your inbox and spam folder.";

  beforeEach(() => {
    jest.clearAllMocks();
    (getTranslation as jest.Mock).mockImplementation((key: string) =>
      key === "staffLogin.messages.emailSent" ? EMAIL_SENT_TEMPLATE : key,
    );
  });

  afterEach(() => {
    // Restore the default key-echo so other suites stay deterministic.
    (getTranslation as jest.Mock).mockImplementation((key: string) => key);
  });

  it("interpolates the normalized email into the success toast and advances to the code step", async () => {
    (StaffAPI.requestLoginCode as jest.Mock).mockResolvedValueOnce({
      message: "ok",
    });
    render(<StaffLogin />);

    const emailInput = document.querySelector(
      "input[type='email']",
    ) as HTMLInputElement;
    await act(async () => {
      // Mixed-case input proves the toast shows the normalized (lowercased) address.
      fireEvent.change(emailInput, { target: { value: "Staff.Member@Example.com" } });
    });

    const buttons = screen.getAllByRole("button");
    await act(async () => {
      fireEvent.click(buttons[buttons.length - 1]);
    });

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledTimes(1);
    });
    const msg = (toast.success as jest.Mock).mock.calls[0][0] as string;
    expect(msg).toContain("staff.member@example.com");
    // The placeholder must be fully substituted — never shown to the user raw.
    expect(msg).not.toContain("{email}");

    // Advances to the code step regardless (we can't reveal whether the email exists).
    await waitFor(() => {
      expect(document.querySelector("input[type='text']")).toBeInTheDocument();
    });
  });
});

describe("StaffLogin with email delivery off (EMAIL_PROVIDER=log)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/staff/login");
    setInstanceForTests(
      parseInstanceInfo({
        registration_mode: "invite",
        features: { email: false },
      }),
    );
  });

  afterEach(() => {
    resetInstanceCacheForTests();
  });

  it("points the operator at their administrator instead of an inbox", async () => {
    (StaffAPI.requestLoginCode as jest.Mock).mockResolvedValueOnce({
      message: "ok",
    });
    render(<StaffLogin />);
    const emailInput = document.querySelector(
      "input[type='email']",
    ) as HTMLInputElement;
    await act(async () => {
      fireEvent.change(emailInput, { target: { value: "staff@example.com" } });
    });
    const buttons = screen.getAllByRole("button");
    await act(async () => {
      fireEvent.click(buttons[buttons.length - 1]);
    });

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledTimes(1);
    });
    expect((toast.success as jest.Mock).mock.calls[0][0]).toContain(
      "messages.codeLogged",
    );
    expect(
      await screen.findByText(/steps\.code\.subtitleNoEmail/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/steps\.code\.subtitle$/)).toBeNull();
  });
});

describe("StaffLogin Google button follows the instance", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/staff/login");
  });

  afterEach(() => {
    resetInstanceCacheForTests();
  });

  it("offers Google sign-in when the instance has a Google OAuth client", () => {
    setInstanceForTests(
      parseInstanceInfo({
        registration_mode: "invite",
        features: { google_oauth: true },
      }),
    );
    render(<StaffLogin />);
    expect(screen.getByText("staffLogin.steps.email.googleButton")).toBeInTheDocument();
  });

  it("hides Google sign-in on an instance without Google OAuth", () => {
    setInstanceForTests(
      parseInstanceInfo({
        registration_mode: "invite",
        features: { google_oauth: false },
      }),
    );
    render(<StaffLogin />);
    expect(screen.queryByText("staffLogin.steps.email.googleButton")).toBeNull();
    expect(document.querySelector("input[type='email']")).toBeInTheDocument();
  });
});
