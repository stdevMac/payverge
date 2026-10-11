/** @jest-environment jsdom */
// Public demo (DEMO_MODE): the sign-in tab offers one-click demo entries.
// Mock heavy transitive deps so importing AuthModal (for the pure helper) does
// not pull in wagmi/ESM modules Jest cannot transform.
const mockRefreshStaffData = jest.fn().mockResolvedValue(undefined);
const mockRefreshSession = jest.fn().mockResolvedValue(true);
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    refreshStaffData: mockRefreshStaffData,
    refreshSession: mockRefreshSession,
  }),
}));
const mockDemoLogin = jest.fn();
jest.mock("@/api/demo", () => ({
  demoLogin: (...args: unknown[]) => mockDemoLogin(...args),
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
import { AuthModal } from "./AuthModal";
import { resetRegistrationModeCacheForTests } from "@/api/registrationMode";
import { resetInstanceCacheForTests, setInstanceForTests } from "@/hooks/useInstance";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import { SESSION_HINT_KEY } from "@/utils/refreshAuth";

function demoInstance(mode: boolean) {
  return parseInstanceInfo({
    registration_mode: "closed",
    features: { google_oauth: false },
    demo: { enabled: true, mode, reset_utc: "03:00" },
  });
}

beforeEach(() => {
  resetRegistrationModeCacheForTests();
  mockInstanceGet.mockReset();
  mockInstanceGet.mockReturnValue(new Promise(() => {}));
  mockDemoLogin.mockReset();
  mockRouterPush.mockReset();
  mockRefreshSession.mockClear();
  mockRefreshStaffData.mockClear();
});

afterEach(() => resetInstanceCacheForTests());

describe("AuthModal public demo", () => {
  it("has no demo entries on a normal install", () => {
    setInstanceForTests(demoInstance(false));
    render(<AuthModal isOpen onClose={jest.fn()} />);
    expect(screen.queryByRole("button", { name: "Enter demo as Owner" })).toBeNull();
  });

  it("enters as owner: hydrates the session and opens the dashboard", async () => {
    setInstanceForTests(demoInstance(true));
    mockDemoLogin.mockResolvedValue({ kind: "owner", redirect: "/dashboard" });
    const onClose = jest.fn();
    render(<AuthModal isOpen onClose={onClose} />);
    fireEvent.click(screen.getByRole("button", { name: "Enter demo as Owner" }));
    await waitFor(() => expect(mockRouterPush).toHaveBeenCalledWith("/dashboard"));
    expect(mockRefreshSession).toHaveBeenCalled();
    expect(localStorage.getItem(SESSION_HINT_KEY)).toBe("1");
    expect(onClose).toHaveBeenCalled();
  });

  it("enters as owner from a host's sign-in gate: hands over to onSuccess", async () => {
    setInstanceForTests(demoInstance(true));
    mockDemoLogin.mockResolvedValue({ kind: "owner", redirect: "/dashboard" });
    const onSuccess = jest.fn();
    render(<AuthModal isOpen onClose={jest.fn()} onSuccess={onSuccess} />);
    fireEvent.click(screen.getByRole("button", { name: "Enter demo as Owner" }));
    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
    expect(mockRefreshSession).toHaveBeenCalled();
    expect(mockRouterPush).not.toHaveBeenCalled();
  });

  it("enters as owner while already on the dashboard: reloads it with the new session", async () => {
    setInstanceForTests(demoInstance(true));
    mockDemoLogin.mockResolvedValue({ kind: "owner", redirect: "/dashboard" });
    window.history.pushState({}, "", "/dashboard");
    const errSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    try {
      render(<AuthModal isOpen onClose={jest.fn()} />);
      fireEvent.click(screen.getByRole("button", { name: "Enter demo as Owner" }));
      await waitFor(() => expect(mockRefreshSession).toHaveBeenCalled());
      // jsdom cannot navigate: the reload surfaces as its "not implemented" error.
      await waitFor(() =>
        expect(errSpy.mock.calls.some((c) => String(c[0]?.message ?? c[0]).includes("navigation"))).toBe(true),
      );
      expect(mockRouterPush).not.toHaveBeenCalled();
    } finally {
      errSpy.mockRestore();
      window.history.pushState({}, "", "/");
    }
  });

  it("enters as kitchen staff: opens that venue's staff dashboard", async () => {
    setInstanceForTests(demoInstance(true));
    mockDemoLogin.mockResolvedValue({
      kind: "staff",
      staff: { id: 3, name: "Tomás", email: "t@x", role: "kitchen", business_id: 42 },
    });
    render(<AuthModal isOpen onClose={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Enter demo as Staff (kitchen)" }));
    await waitFor(() => expect(mockRouterPush).toHaveBeenCalledWith("/business/42/dashboard"));
    expect(mockRefreshStaffData).toHaveBeenCalled();
    expect(mockDemoLogin).toHaveBeenCalledWith("kitchen");
  });
});
