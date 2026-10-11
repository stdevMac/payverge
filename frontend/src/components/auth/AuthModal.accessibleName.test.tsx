/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import { computeAccessibleName } from "dom-accessibility-api";
import { AuthModal } from "./AuthModal";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    refreshStaffData: jest.fn(),
    refreshSession: jest.fn().mockResolvedValue(true),
  }),
}));
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn(), refresh: jest.fn() }),
}));
jest.mock("@/api/auth", () => ({
  authAPI: {
    login: jest.fn(),
    register: jest.fn(),
    getGoogleAuthURL: jest.fn(),
    resendVerification: jest.fn(),
  },
}));
// AuthModal probes GET /platform/registration-mode. These synchronous name
// checks keep the probe pending: the form stays in its default layout (invite
// field shown) and no update lands after the assertions.
jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(() => new Promise(() => {})) },
}));
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/components/staff/StaffLogin", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@nextui-org/react", () => {
  const actual = jest.requireActual("@nextui-org/react");
  const React = jest.requireActual("react");
  const Pass = ({ children }: { children?: React.ReactNode }) =>
    React.createElement("div", null, children);
  return {
    ...actual,
    Modal: ({ children, isOpen }: { children: React.ReactNode; isOpen: boolean }) =>
      isOpen ? React.createElement("div", null, children) : null,
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
  };
});

function renderModal(
  tab: "signin" | "signup",
  locale: "en" | "es" | "es-AR" = "en",
) {
  return render(
    <SimpleTranslationProvider initialLocale={locale}>
      <AuthModal isOpen onClose={jest.fn()} defaultTab={tab} />
    </SimpleTranslationProvider>,
  );
}

function passwordName(container: HTMLElement): string {
  const input = container.querySelector('input[type="password"]');
  if (!input) throw new Error("missing password input");
  return computeAccessibleName(input);
}

describe("AuthModal shipped field wiring (#394)", () => {
  it("sign-in Email and Password each have one accessible name", () => {
    const { container } = renderModal("signin");
    expect(screen.getByRole("textbox", { name: /^Email$/ })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: /Email Email/ })).toBeNull();
    expect(passwordName(container)).toBe("Password");
  });

  it("sign-up Full Name, Invite code, Email, and Password each have one name", () => {
    const { container } = renderModal("signup");
    expect(screen.getByRole("textbox", { name: /^Full Name$/ })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /^Invite code$/ })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /^Email$/ })).toBeInTheDocument();
    expect(passwordName(container)).toBe("Password");
  });

  it("Spanish sign-up fields keep a single name", () => {
    const { container } = renderModal("signup", "es");
    expect(screen.getByRole("textbox", { name: /^Nombre completo$/ })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /^Código de invitación$/ })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /^Correo$/ })).toBeInTheDocument();
    expect(passwordName(container)).toBe("Contraseña");
  });

  it("Argentine Spanish invite code stays a single name after switching tabs", () => {
    renderModal("signin", "es-AR");
    fireEvent.click(screen.getByRole("button", { name: "Registrarse" }));
    expect(
      screen.getByRole("textbox", { name: /^Código de invitación$/ }),
    ).toBeInTheDocument();
  });
});
