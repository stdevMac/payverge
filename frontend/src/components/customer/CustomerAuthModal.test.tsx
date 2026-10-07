/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

import CustomerAuthModal from "@/components/customer/CustomerAuthModal";
import { useCustomerAuth } from "@/contexts/CustomerAuthContext";

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: jest.fn(),
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) =>
      ({
        "customerAuth.login.title": "Sign in",
        "customerAuth.login.subtitle": "Welcome back",
        "customerAuth.login.email": "Email",
        "customerAuth.login.emailPlaceholder": "you@example.com",
        "customerAuth.login.password": "Password",
        "customerAuth.login.passwordPlaceholder": "Password",
        "customerAuth.login.noAccount": "No account?",
        "customerAuth.login.register": "Register",
        "customerAuth.login.button": "Sign in",
        "customerAuth.login.signingIn": "Signing in",
        "customerAuth.validation.emailRequired": "Email is required.",
        "customerAuth.validation.emailInvalid": "Enter a valid email address.",
        "customerAuth.validation.passwordRequired": "Password is required.",
        "customerAuth.register.nameRequired": "Name is required",
      })[key] ?? key,
  }),
}));

jest.mock("@nextui-org/react", () => ({
  Modal: ({
    children,
    isOpen,
  }: {
    children: React.ReactNode;
    isOpen: boolean;
  }) => (isOpen ? <div>{children}</div> : null),
  ModalContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ModalHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ModalBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ModalFooter: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Divider: () => <hr />,
  Input: (jest.requireActual("react") as typeof import("react")).forwardRef<
    HTMLInputElement,
    {
      label: string;
      type?: string;
      value: string;
      onValueChange?: (value: string) => void;
      onChange?: (event: React.ChangeEvent<HTMLInputElement>) => void;
      placeholder?: string;
      autoComplete?: string;
      endContent?: React.ReactNode;
      required?: boolean;
      isInvalid?: boolean;
      errorMessage?: React.ReactNode;
      "aria-required"?: boolean | "true" | "false";
    }
  >(function MockInput(
    {
      label,
      type = "text",
      value,
      onValueChange,
      onChange,
      placeholder,
      autoComplete,
      endContent,
      required,
      isInvalid,
      errorMessage,
      "aria-required": ariaRequired,
    },
    ref,
  ) {
    return (
      <label>
        <span>{label}</span>
        <input
          ref={ref}
          aria-label={label}
          type={type}
          value={value}
          placeholder={placeholder}
          autoComplete={autoComplete}
          required={required}
          aria-required={ariaRequired}
          aria-invalid={isInvalid || undefined}
          onChange={(event) => {
            onChange?.(event);
            onValueChange?.(event.target.value);
          }}
        />
        {endContent}
        {isInvalid && errorMessage ? <p role="alert">{errorMessage}</p> : null}
      </label>
    );
  }),
  Button: ({
    children,
    type = "button",
    onPress,
    isLoading,
  }: {
    children: React.ReactNode;
    type?: "button" | "submit";
    onPress?: () => void;
    isLoading?: boolean;
  }) => (
    <button type={type} disabled={isLoading} onClick={onPress}>
      {children}
    </button>
  ),
}));

const mockedUseCustomerAuth = useCustomerAuth as jest.MockedFunction<
  typeof useCustomerAuth
>;

describe("CustomerAuthModal", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("waits for parent success handling before closing", async () => {
    const login = jest
      .fn()
      .mockResolvedValue({ id: 42, email: "guest@test.dev" });
    const register = jest.fn();
    mockedUseCustomerAuth.mockReturnValue({
      login,
      register,
      customer: null,
      customerId: null,
      isAuthenticated: false,
      isLoading: false,
      logout: jest.fn(),
      refreshAuth: jest.fn(),
    } as any);
    const onClose = jest.fn();
    let resolveSuccess: (() => void) | undefined;
    const onSuccess = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSuccess = resolve;
        }),
    );

    render(
      <CustomerAuthModal
        isOpen
        onClose={onClose}
        onSuccess={onSuccess}
        defaultMode="login"
      />,
    );

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: " Guest@Test.Dev " },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => {
      expect(login).toHaveBeenCalledWith("guest@test.dev", "password123");
    });
    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalledWith(42, {
        id: 42,
        email: "guest@test.dev",
      });
    });
    expect(onClose).not.toHaveBeenCalled();

    await act(async () => {
      resolveSuccess?.();
    });

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
  });

  function mountWith(mode: "login" | "register") {
    const login = jest.fn().mockReturnValue(new Promise(() => {}));
    const register = jest.fn().mockReturnValue(new Promise(() => {}));
    mockedUseCustomerAuth.mockReturnValue({
      login,
      register,
      customer: null,
      customerId: null,
      isAuthenticated: false,
      isLoading: false,
      logout: jest.fn(),
      refreshAuth: jest.fn(),
    } as any);
    return {
      ...render(
        <CustomerAuthModal isOpen onClose={jest.fn()} defaultMode={mode} />,
      ),
      login,
      register,
    };
  }

  it("sets autoComplete on email and password for password-manager autofill", () => {
    mountWith("login");
    expect(screen.getByLabelText("Email")).toHaveAttribute(
      "autocomplete",
      "email",
    );
    expect(screen.getByLabelText("Password")).toHaveAttribute(
      "autocomplete",
      "current-password",
    );
  });

  it("uses new-password autoComplete in register mode", () => {
    mountWith("register");
    expect(
      screen.getByLabelText("customerAuth.register.password"),
    ).toHaveAttribute("autocomplete", "new-password");
  });

  it("toggles password visibility via the eye button", () => {
    mountWith("login");
    const password = screen.getByLabelText("Password");
    expect(password).toHaveAttribute("type", "password");
    fireEvent.click(
      screen.getByRole("button", { name: "customerAuth.login.showPassword" }),
    );
    expect(password).toHaveAttribute("type", "text");
  });

  it("announces the signup password requirement and focuses a short password", () => {
    const { register } = mountWith("register");

    const password = screen.getByLabelText("customerAuth.register.password");
    expect(screen.getByRole("status")).toHaveTextContent(
      "customerAuth.register.passwordPlaceholder",
    );

    fireEvent.change(screen.getByLabelText("customerAuth.register.name"), {
      target: { value: "Ada" },
    });
    fireEvent.change(screen.getByLabelText("customerAuth.register.email"), {
      target: { value: "ada@example.com" },
    });
    fireEvent.change(password, { target: { value: "short" } });
    fireEvent.submit(password.closest("form") as HTMLFormElement);

    expect(password).toHaveFocus();
    expect(register).not.toHaveBeenCalled();
  });

  it("shows mode-aware loading copy ('creating') in register mode", () => {
    mountWith("register");
    fireEvent.change(screen.getByLabelText("customerAuth.register.name"), {
      target: { value: "Ada" },
    });
    fireEvent.change(screen.getByLabelText("customerAuth.register.email"), {
      target: { value: "ada@example.com" },
    });
    fireEvent.change(screen.getByLabelText("customerAuth.register.password"), {
      target: { value: "password123" },
    });
    const form = screen
      .getByLabelText("customerAuth.register.email")
      .closest("form");
    fireEvent.submit(form as HTMLFormElement);
    expect(
      screen.getByText("customerAuth.register.creating"),
    ).toBeInTheDocument();
  });

  it("disables native constraint bubbles so validation can be localized", () => {
    mountWith("login");
    const form = screen.getByLabelText("Email").closest("form");
    expect(form).toHaveAttribute("novalidate");
  });

  it("shows a localized email error on empty sign-in and does not call login", () => {
    const { login } = mountWith("login");
    const form = screen
      .getByLabelText("Email")
      .closest("form") as HTMLFormElement;

    fireEvent.submit(form);

    expect(screen.getByText("Email is required.")).toBeInTheDocument();
    expect(login).not.toHaveBeenCalled();
  });

  it("shows a localized invalid-email error instead of a native type=email bubble", () => {
    const { login } = mountWith("login");
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "not-an-email" },
    });
    fireEvent.submit(
      screen.getByLabelText("Email").closest("form") as HTMLFormElement,
    );

    expect(
      screen.getByText("Enter a valid email address."),
    ).toBeInTheDocument();
    expect(login).not.toHaveBeenCalled();
  });

  it("clears stale sign-in validity when switching to an untouched signup form", () => {
    const { login, register } = mountWith("login");
    const email = screen.getByLabelText("Email") as HTMLInputElement;
    const form = email.closest("form") as HTMLFormElement;

    email.setCustomValidity("Please fill out this field.");
    expect(email.validationMessage).toBe("Please fill out this field.");
    fireEvent.submit(form);
    expect(screen.getByText("Email is required.")).toBeInTheDocument();
    expect(login).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Register" }));

    const signupEmail = screen.getByLabelText(
      "customerAuth.register.email",
    ) as HTMLInputElement;
    expect(signupEmail.validationMessage).toBe("");
    expect(
      screen.queryByText("Please fill out this field."),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Email is required.")).not.toBeInTheDocument();
    expect(screen.queryByText("Name is required")).not.toBeInTheDocument();
    expect(signupEmail).not.toHaveAttribute("aria-invalid", "true");
    expect(register).not.toHaveBeenCalled();
  });

  it("validates empty signup only after a signup submit, with localized copy", () => {
    const { register } = mountWith("login");
    fireEvent.submit(
      screen.getByLabelText("Email").closest("form") as HTMLFormElement,
    );
    fireEvent.click(screen.getByRole("button", { name: "Register" }));

    expect(screen.queryByText("Email is required.")).not.toBeInTheDocument();

    fireEvent.submit(
      screen
        .getByLabelText("customerAuth.register.email")
        .closest("form") as HTMLFormElement,
    );

    expect(screen.getByText("Name is required")).toBeInTheDocument();
    expect(register).not.toHaveBeenCalled();
  });
});
