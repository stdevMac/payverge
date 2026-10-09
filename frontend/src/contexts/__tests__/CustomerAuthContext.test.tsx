/** @jest-environment jsdom */

import React from "react";
import { render, screen, act, waitFor } from "@testing-library/react";
import {
  CustomerAuthProvider,
  useCustomerAuth,
} from "../CustomerAuthContext";
import { crmAPI, Customer } from "@/api/crm";
import { axiosInstance } from "@/api/tools/instance";
import { startTokenRefreshTimer, type TokenRefreshOptions } from "@/utils/tokenRefresh";
import {
  CUSTOMER_SESSION_HINT_KEY,
  refreshCustomerAuthSession,
  type RefreshCustomerAuthResult,
} from "@/utils/refreshCustomerAuth";
import { setSentryIdentity } from "@/lib/sentry/reporting";
import { getPublicConfig } from "@/config/publicConfig";

jest.mock("@/api/crm", () => ({
  crmAPI: {
    getProfile: jest.fn(),
    login: jest.fn(),
    logout: jest.fn(),
    register: jest.fn(),
  },
}));

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

jest.mock("@/utils/tokenRefresh", () => ({
  startTokenRefreshTimer: jest.fn(() => jest.fn()),
}));

jest.mock("@/utils/refreshCustomerAuth", () => ({
  CUSTOMER_SESSION_EXPIRED_EVENT: "customer:session-expired",
  CUSTOMER_SESSION_HINT_KEY: "payverge_had_customer_session",
  refreshCustomerAuthSession: jest.fn(),
}));

jest.mock("@/lib/sentry/reporting", () => ({
  clearSentryIdentity: jest.fn(),
  setSentryIdentity: jest.fn(),
}));

const mockCrmAPI = crmAPI as jest.Mocked<typeof crmAPI>;
const mockAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;
const mockStartTokenRefreshTimer = startTokenRefreshTimer as jest.MockedFunction<
  typeof startTokenRefreshTimer
>;
const mockRefreshCustomerAuthSession =
  refreshCustomerAuthSession as jest.MockedFunction<
    typeof refreshCustomerAuthSession
  >;
const mockSetSentryIdentity = setSentryIdentity as jest.MockedFunction<
  typeof setSentryIdentity
>;

const mockCustomer: Customer = {
  id: 42,
  email: "test@example.com",
  name: "Test User",
  is_active: true,
  email_verified: true,
  created_at: "2026-04-02T00:00:00Z",
  updated_at: "2026-04-02T00:00:00Z",
};

function TestConsumer() {
  const { customer, customerId, isAuthenticated, loading } = useCustomerAuth();
  return (
    <div>
      <span data-testid="loading">{String(loading)}</span>
      <span data-testid="is-authenticated">{String(isAuthenticated)}</span>
      <span data-testid="customer-id">{customerId ?? "null"}</span>
      <span data-testid="customer-email">{customer?.email ?? "none"}</span>
    </div>
  );
}

function LoginConsumer() {
  const { login } = useCustomerAuth();
  return (
    <button
      onClick={() => {
        login("test@example.com", "password").catch(() => {});
      }}
    >
      Login
    </button>
  );
}

function LogoutConsumer() {
  const { logout } = useCustomerAuth();
  return <button onClick={() => { logout().catch(() => {}); }}>Logout</button>;
}

function RefreshConsumer() {
  const { refreshCustomer } = useCustomerAuth();
  return (
    <button onClick={() => { refreshCustomer().catch(() => {}); }}>
      Refresh customer
    </button>
  );
}

describe("CustomerAuthProvider", () => {
  beforeEach(() => {
    jest.resetAllMocks();
    localStorage.clear();
    mockAxios.get.mockResolvedValue({ data: { authenticated: false } });
    mockAxios.post.mockResolvedValue({ data: { success: true } });
    mockRefreshCustomerAuthSession.mockResolvedValue({ ok: true });
    mockStartTokenRefreshTimer.mockImplementation(() => jest.fn());
  });

  describe("initial loading state", () => {
    it("hydrates a customer session from session-info before loading the profile", async () => {
      mockAxios.get.mockResolvedValue({
        data: {
          authenticated: true,
          type: "customer",
          customer_id: mockCustomer.id,
          email: mockCustomer.email,
        },
      });
      mockCrmAPI.getProfile.mockResolvedValue(mockCustomer);

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      // Initially loading should be true
      expect(screen.getByTestId("loading").textContent).toBe("true");

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      expect(screen.getByTestId("customer-email").textContent).toBe("test@example.com");
      expect(mockAxios.get).toHaveBeenCalledWith("/customer/session-info", {
        _skipAuthRefresh: true,
      });
      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBe("1");
    });

    it("uses the customer refresh cookie on remount when access has expired", async () => {
      localStorage.setItem(CUSTOMER_SESSION_HINT_KEY, "1");
      mockAxios.get
        .mockResolvedValueOnce({ data: { authenticated: false } })
        .mockResolvedValueOnce({
          data: {
            authenticated: true,
            type: "customer",
            customer_id: mockCustomer.id,
            email: mockCustomer.email,
          },
        });
      mockCrmAPI.getProfile.mockResolvedValue(mockCustomer);
      mockRefreshCustomerAuthSession.mockResolvedValue({ ok: true });

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });
      expect(mockRefreshCustomerAuthSession).toHaveBeenCalledTimes(1);
      expect(mockAxios.get).toHaveBeenCalledTimes(2);
      expect(mockCrmAPI.getProfile).toHaveBeenCalledTimes(1);
      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBe("1");
    });

    it("uses the HttpOnly refresh cookie when the local session hint is unavailable", async () => {
      const getItem = jest
        .spyOn(Storage.prototype, "getItem")
        .mockImplementation(() => {
          throw new DOMException("storage disabled", "SecurityError");
        });
      mockAxios.get
        .mockResolvedValueOnce({ data: { authenticated: false } })
        .mockResolvedValueOnce({
          data: {
            authenticated: true,
            type: "customer",
            customer_id: mockCustomer.id,
            email: mockCustomer.email,
          },
        });
      mockCrmAPI.getProfile.mockResolvedValue(mockCustomer);

      try {
        render(
          <CustomerAuthProvider>
            <TestConsumer />
          </CustomerAuthProvider>,
        );

        await waitFor(() => {
          expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
        });
        expect(mockRefreshCustomerAuthSession).toHaveBeenCalledTimes(1);
        expect(mockAxios.get).toHaveBeenCalledTimes(2);
        expect(mockCrmAPI.getProfile).toHaveBeenCalledTimes(1);
      } finally {
        getItem.mockRestore();
      }

      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBe("1");
    });

    it("does not commit customer recovery after the provider unmounts", async () => {
      localStorage.setItem(CUSTOMER_SESSION_HINT_KEY, "1");
      let resolveRefresh!: (result: RefreshCustomerAuthResult) => void;
      mockRefreshCustomerAuthSession.mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveRefresh = resolve;
          }),
      );
      mockAxios.get
        .mockResolvedValueOnce({ data: { authenticated: false } })
        .mockResolvedValueOnce({
          data: {
            authenticated: true,
            type: "customer",
            customer_id: mockCustomer.id,
            email: mockCustomer.email,
          },
        });
      mockCrmAPI.getProfile.mockResolvedValue(mockCustomer);

      const { unmount } = render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );
      await waitFor(() => {
        expect(mockRefreshCustomerAuthSession).toHaveBeenCalledTimes(1);
      });

      unmount();
      await act(async () => {
        resolveRefresh({ ok: true });
        for (let tick = 0; tick < 6; tick += 1) {
          await Promise.resolve();
        }
      });

      expect(mockAxios.get).toHaveBeenCalledTimes(1);
      expect(mockCrmAPI.getProfile).not.toHaveBeenCalled();
      expect(mockSetSentryIdentity).not.toHaveBeenCalled();
      expect(mockStartTokenRefreshTimer).not.toHaveBeenCalled();
    });

    it("does not refresh when the local customer session hint is absent", async () => {

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      expect(screen.getByTestId("customer-id").textContent).toBe("null");
      expect(mockCrmAPI.getProfile).not.toHaveBeenCalled();
      expect(mockRefreshCustomerAuthSession).not.toHaveBeenCalled();
    });

    it("clears the customer hint when bootstrap refresh proves the session dead", async () => {
      localStorage.setItem(CUSTOMER_SESSION_HINT_KEY, "1");
      mockRefreshCustomerAuthSession.mockResolvedValue({
        ok: false,
        status: 401,
        code: "AUTH_TOKEN_INVALID",
        alreadyRotated: false,
        sessionDead: true,
      });

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );
      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(mockRefreshCustomerAuthSession).toHaveBeenCalledTimes(1);
      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBeNull();
      expect(mockCrmAPI.getProfile).not.toHaveBeenCalled();
    });

    it("does not probe CRM when another auth type is active", async () => {
      mockAxios.get.mockResolvedValue({
        data: {
          authenticated: true,
          type: "user",
          user_id: 7,
          email: "staff@example.com",
        },
      });

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      expect(mockCrmAPI.getProfile).not.toHaveBeenCalled();
    });

    it("clears any prior session when session-info returns 401 (session is dead)", async () => {
      mockAxios.get.mockRejectedValue({
        status: 401,
        response: { status: 401, data: { error: "session expired" } },
      });

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      expect(screen.getByTestId("customer-id").textContent).toBe("null");
      expect(mockCrmAPI.getProfile).not.toHaveBeenCalled();
    });

    it("preserves a live customer state and hint when bootstrap receives a policy 403", async () => {
      let rejectBootstrap!: (reason: unknown) => void;
      mockAxios.get.mockImplementationOnce(
        () =>
          new Promise((_resolve, reject) => {
            rejectBootstrap = reject;
          }),
      );
      mockCrmAPI.login.mockResolvedValue({
        customer: mockCustomer,
        token: "tok123",
      });
      const stopTimer = jest.fn();
      mockStartTokenRefreshTimer.mockReturnValue(stopTimer);

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <LoginConsumer />
        </CustomerAuthProvider>,
      );

      await act(async () => {
        screen.getByText("Login").click();
      });
      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });
      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBe("1");

      await act(async () => {
        rejectBootstrap({
          status: 403,
          response: { status: 403, data: { code: "origin_not_allowed" } },
        });
        await Promise.resolve();
      });
      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      expect(screen.getByTestId("customer-id").textContent).toBe("42");
      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBe("1");
      expect(stopTimer).not.toHaveBeenCalled();
    });

    it("ignores a stale bootstrap 401 after a newer login succeeds", async () => {
      let rejectBootstrap!: (reason: unknown) => void;
      mockAxios.get.mockImplementationOnce(
        () =>
          new Promise((_resolve, reject) => {
            rejectBootstrap = reject;
          }),
      );
      mockCrmAPI.login.mockResolvedValue({
        customer: mockCustomer,
        token: "tok123",
      });
      const stopTimer = jest.fn();
      mockStartTokenRefreshTimer.mockReturnValue(stopTimer);

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <LoginConsumer />
        </CustomerAuthProvider>,
      );

      await act(async () => {
        screen.getByText("Login").click();
      });
      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      await act(async () => {
        rejectBootstrap({
          status: 401,
          response: { status: 401, data: { code: "AUTH_TOKEN_INVALID" } },
        });
        await Promise.resolve();
      });
      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      expect(screen.getByTestId("customer-id").textContent).toBe("42");
      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBe("1");
      expect(stopTimer).not.toHaveBeenCalled();
    });

    it("keeps loading=false but does not error-clear on a 500 from getProfile (transient)", async () => {
      mockAxios.get.mockResolvedValue({
        data: {
          authenticated: true,
          type: "customer",
          customer_id: mockCustomer.id,
          email: mockCustomer.email,
        },
      });
      mockCrmAPI.getProfile.mockRejectedValue({
        status: 500,
        response: { status: 500, data: { error: "boom" } },
      });

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      // hasCustomerSessionRef was set true before getProfile threw; a 5xx must
      // NOT force-clear — the optimistic session is preserved for retry.
      // is-authenticated is driven by `customer` state, which never populated,
      // so it reads false here on its own.
      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");

      // Load-bearing: clearCustomerState is internal, but it flips
      // hasCustomerSessionRef to false. The session-expired handler is a no-op
      // when that ref is false and DOES run (setting isSessionExpired=true,
      // which mounts SessionTimeoutWarning) when it is true. Since the 5xx must
      // NOT have cleared, the optimistic session is still considered live, so
      // dispatching customer:session-expired now surfaces the expiry dialog. If the
      // code had wrongly cleared on the 500, the ref would already be false and
      // this dispatch would be ignored — no dialog — letting us observe the
      // difference without reaching into the internal callback.
      // SessionTimeoutWarning (the only button-rendering child here) is unmounted
      // while the session is live and un-expired.
      expect(screen.queryByRole("button")).not.toBeInTheDocument();
      act(() => {
        window.dispatchEvent(new Event("customer:session-expired"));
      });

      await waitFor(() => {
        expect(screen.getByRole("button")).toBeInTheDocument();
      });
    });

    it("clears a hydrated session when getProfile returns 401", async () => {
      mockAxios.get.mockResolvedValue({
        data: {
          authenticated: true,
          type: "customer",
          customer_id: mockCustomer.id,
          email: mockCustomer.email,
        },
      });
      mockCrmAPI.getProfile.mockRejectedValue({
        status: 401,
        response: { status: 401, data: { error: "expired" } },
      });

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      expect(screen.getByTestId("customer-id").textContent).toBe("null");
    });
  });

  describe("login()", () => {
    it("updates context state after successful login and returns the customer", async () => {
      mockCrmAPI.getProfile.mockRejectedValue(new Error("Not authenticated"));
      mockCrmAPI.login.mockResolvedValue({ customer: mockCustomer, token: "tok123" });

      let returnedCustomer: typeof mockCustomer | null = null;

      function LoginCapture() {
        const { login } = useCustomerAuth();
        return (
          <button
            onClick={() => {
              login("test@example.com", "password").then((c) => {
                returnedCustomer = c as typeof mockCustomer;
              }).catch(() => {});
            }}
          >
            Login
          </button>
        );
      }

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <LoginCapture />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");

      await act(async () => {
        screen.getByText("Login").click();
      });

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      expect(screen.getByTestId("customer-email").textContent).toBe("test@example.com");
      expect(screen.getByTestId("customer-id").textContent).toBe("42");
      expect(returnedCustomer).toEqual(mockCustomer);
    });

    it("refresh timer uses the typed customer refresh client", async () => {
      mockCrmAPI.login.mockResolvedValue({ customer: mockCustomer, token: "tok123" });

      render(
        <CustomerAuthProvider>
          <LoginConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(mockAxios.get).toHaveBeenCalledWith("/customer/session-info", {
          _skipAuthRefresh: true,
        });
      });

      await act(async () => {
        screen.getByText("Login").click();
      });

      await waitFor(() => {
        expect(mockStartTokenRefreshTimer).toHaveBeenCalled();
      });

      const options = mockStartTokenRefreshTimer.mock.calls.at(-1)?.[0] as
        | TokenRefreshOptions
        | undefined;
      expect(options).toBeDefined();
      await expect(options!.refreshFn()).resolves.toEqual({ ok: true });

      expect(mockRefreshCustomerAuthSession).toHaveBeenCalledWith(
        getPublicConfig().apiUrl,
      );
      expect(mockAxios.post).not.toHaveBeenCalled();
    });

    it.each<[
      string,
      RefreshCustomerAuthResult,
    ]>([
      [
        "rotated",
        {
          ok: false,
          status: 401,
          code: "AUTH_REFRESH_ROTATED",
          alreadyRotated: true,
          sessionDead: false,
        },
      ],
      [
        "transient",
        {
          ok: false,
          status: 503,
          alreadyRotated: false,
          sessionDead: false,
        },
      ],
      [
        "dead",
        {
          ok: false,
          status: 401,
          code: "AUTH_TOKEN_INVALID",
          alreadyRotated: false,
          sessionDead: true,
        },
      ],
    ])("passes the %s typed refresh outcome through to the timer", async (_label, result) => {
      mockCrmAPI.login.mockResolvedValue({ customer: mockCustomer, token: "tok123" });
      mockRefreshCustomerAuthSession.mockResolvedValue(result);

      render(
        <CustomerAuthProvider>
          <LoginConsumer />
        </CustomerAuthProvider>,
      );
      await waitFor(() => expect(screen.getByText("Login")).toBeInTheDocument());
      await act(async () => { screen.getByText("Login").click(); });
      await waitFor(() => expect(mockStartTokenRefreshTimer).toHaveBeenCalled());

      const options = mockStartTokenRefreshTimer.mock.calls.at(-1)?.[0] as TokenRefreshOptions;
      await expect(options.refreshFn()).resolves.toEqual(result);
    });

    it("throws and does not update state on login failure", async () => {
      mockCrmAPI.getProfile.mockRejectedValue(new Error("Not authenticated"));
      mockCrmAPI.login.mockRejectedValue(new Error("Invalid credentials"));

      let caughtError: Error | null = null;

      function LoginCapture() {
        const { login } = useCustomerAuth();
        return (
          <button
            onClick={() => {
              login("bad@example.com", "wrong").catch((e: Error) => {
                caughtError = e;
              });
            }}
          >
            Login
          </button>
        );
      }

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <LoginCapture />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      await act(async () => {
        screen.getByText("Login").click();
      });

      await waitFor(() => {
        expect(caughtError).not.toBeNull();
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
    });
  });

  describe("register()", () => {
    it("calls register then login, updates state, and returns the customer", async () => {
      mockCrmAPI.getProfile.mockRejectedValue(new Error("Not authenticated"));
      mockCrmAPI.register.mockResolvedValue({ success: true });
      mockCrmAPI.login.mockResolvedValue({ customer: mockCustomer, token: "tok123" });

      let returnedCustomer: typeof mockCustomer | null = null;

      function RegisterCapture() {
        const { register } = useCustomerAuth();
        return (
          <button
            onClick={() => {
              register("test@example.com", "password", "Test User").then((c) => {
                returnedCustomer = c as typeof mockCustomer;
              }).catch(() => {});
            }}
          >
            Register
          </button>
        );
      }

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <RegisterCapture />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      await act(async () => {
        screen.getByText("Register").click();
      });

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      expect(mockCrmAPI.register).toHaveBeenCalledWith("test@example.com", "password", "Test User");
      expect(mockCrmAPI.login).toHaveBeenCalledWith("test@example.com", "password");
      expect(screen.getByTestId("customer-email").textContent).toBe("test@example.com");
      expect(returnedCustomer).toEqual(mockCustomer);
    });

    it("throws and does not update state on register failure", async () => {
      mockCrmAPI.getProfile.mockRejectedValue(new Error("Not authenticated"));
      mockCrmAPI.register.mockRejectedValue(new Error("Email already taken"));

      let caughtError: Error | null = null;

      function RegisterCapture() {
        const { register } = useCustomerAuth();
        return (
          <button
            onClick={() => {
              register("taken@example.com", "password", "Test User").catch((e: Error) => {
                caughtError = e;
              });
            }}
          >
            Register
          </button>
        );
      }

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <RegisterCapture />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      await act(async () => {
        screen.getByText("Register").click();
      });

      await waitFor(() => {
        expect(caughtError).not.toBeNull();
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      expect(mockCrmAPI.login).not.toHaveBeenCalled();
    });
  });

  describe("logout()", () => {
    it("clears customer state after logout", async () => {
      mockAxios.get.mockResolvedValue({
        data: {
          authenticated: true,
          type: "customer",
          customer_id: mockCustomer.id,
          email: mockCustomer.email,
        },
      });
      mockCrmAPI.getProfile.mockResolvedValue(mockCustomer);
      mockCrmAPI.logout.mockResolvedValue(undefined);

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <LogoutConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      await act(async () => {
        screen.getByText("Logout").click();
      });

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      });

      expect(screen.getByTestId("customer-id").textContent).toBe("null");
      expect(screen.getByTestId("customer-email").textContent).toBe("none");
    });

    it("still clears state when logout API call fails", async () => {
      mockAxios.get.mockResolvedValue({
        data: {
          authenticated: true,
          type: "customer",
          customer_id: mockCustomer.id,
          email: mockCustomer.email,
        },
      });
      mockCrmAPI.getProfile.mockResolvedValue(mockCustomer);
      mockCrmAPI.logout.mockRejectedValue(new Error("Network error"));

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <LogoutConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      await act(async () => {
        screen.getByText("Logout").click();
      });

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      });
    });
  });

  describe("refreshCustomer()", () => {
    it.each([
      ["network", new TypeError("offline")],
      ["server", { status: 503, response: { status: 503 } }],
    ])("preserves a proven customer session on a transient %s failure", async (_label, error) => {
      mockAxios.get.mockResolvedValue({
        data: { authenticated: true, type: "customer", customer_id: mockCustomer.id },
      });
      mockCrmAPI.getProfile
        .mockResolvedValueOnce(mockCustomer)
        .mockRejectedValueOnce(error);

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <RefreshConsumer />
        </CustomerAuthProvider>,
      );
      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      await act(async () => { screen.getByText("Refresh customer").click(); });
      await waitFor(() => expect(mockCrmAPI.getProfile).toHaveBeenCalledTimes(2));

      expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      expect(screen.getByTestId("customer-id").textContent).toBe("42");
    });

    it("preserves customer state, hint, and timer when profile refresh returns 403", async () => {
      mockAxios.get.mockResolvedValue({
        data: { authenticated: true, type: "customer", customer_id: mockCustomer.id },
      });
      mockCrmAPI.getProfile
        .mockResolvedValueOnce(mockCustomer)
        .mockRejectedValueOnce({
          status: 403,
          response: { status: 403, data: { code: "origin_not_allowed" } },
        });
      const stopTimer = jest.fn();
      mockStartTokenRefreshTimer.mockReturnValue(stopTimer);

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <RefreshConsumer />
        </CustomerAuthProvider>,
      );
      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      await act(async () => {
        screen.getByText("Refresh customer").click();
      });
      await waitFor(() => expect(mockCrmAPI.getProfile).toHaveBeenCalledTimes(2));

      expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      expect(screen.getByTestId("customer-id").textContent).toBe("42");
      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBe("1");
      expect(mockStartTokenRefreshTimer).toHaveBeenCalledTimes(1);
      expect(stopTimer).not.toHaveBeenCalled();
    });

    it("clears a proven customer session when profile refresh returns 401", async () => {
      mockAxios.get.mockResolvedValue({
        data: { authenticated: true, type: "customer", customer_id: mockCustomer.id },
      });
      mockCrmAPI.getProfile
        .mockResolvedValueOnce(mockCustomer)
        .mockRejectedValueOnce({ status: 401, response: { status: 401 } });

      render(
        <CustomerAuthProvider>
          <TestConsumer />
          <RefreshConsumer />
        </CustomerAuthProvider>,
      );
      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      await act(async () => { screen.getByText("Refresh customer").click(); });
      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      });
      expect(localStorage.getItem(CUSTOMER_SESSION_HINT_KEY)).toBeNull();
    });
  });

  describe("session-expired event", () => {
    it("clears customer state when customer:session-expired is dispatched", async () => {
      mockAxios.get.mockResolvedValue({
        data: {
          authenticated: true,
          type: "customer",
          customer_id: mockCustomer.id,
          email: mockCustomer.email,
        },
      });
      mockCrmAPI.getProfile.mockResolvedValue(mockCustomer);

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      act(() => {
        window.dispatchEvent(new Event("customer:session-expired"));
      });

      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      });

      expect(screen.getByTestId("customer-id").textContent).toBe("null");
    });

    it("ignores customer:session-expired when there is no customer session", async () => {
      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading").textContent).toBe("false");
      });

      act(() => {
        window.dispatchEvent(new Event("customer:session-expired"));
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("false");
      expect(screen.getByTestId("customer-id").textContent).toBe("null");
    });

    it("does not clear a customer session for an owner auth expiry event", async () => {
      mockAxios.get.mockResolvedValue({
        data: {
          authenticated: true,
          type: "customer",
          customer_id: mockCustomer.id,
          email: mockCustomer.email,
        },
      });
      mockCrmAPI.getProfile.mockResolvedValue(mockCustomer);

      render(
        <CustomerAuthProvider>
          <TestConsumer />
        </CustomerAuthProvider>,
      );
      await waitFor(() => {
        expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      });

      act(() => {
        window.dispatchEvent(new Event("auth:session-expired"));
      });

      expect(screen.getByTestId("is-authenticated").textContent).toBe("true");
      expect(screen.getByTestId("customer-id").textContent).toBe("42");
    });
  });

  describe("useCustomerAuth()", () => {
    it("throws when used outside of CustomerAuthProvider", () => {
      // Suppress expected console.error from React error boundary
      const consoleSpy = jest.spyOn(console, "error").mockImplementation(() => {});

      function BareConsumer() {
        useCustomerAuth();
        return null;
      }

      expect(() => render(<BareConsumer />)).toThrow(
        "useCustomerAuth must be used within a CustomerAuthProvider",
      );

      consoleSpy.mockRestore();
    });
  });
});
