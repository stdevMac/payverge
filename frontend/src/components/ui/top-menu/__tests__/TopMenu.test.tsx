/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { StaffData } from "@/utils/staffAuth";
import { TopMenu } from "../TopMenu";

let mockPathname = "/";
let mockSearchParams = new URLSearchParams();
// Features the instance probe reports as OFF (unknown stays "on").
const mockOffFeatures: string[] = [];
jest.mock("@/hooks/useInstance", () => ({
  useInstance: () => ({
    instance: null,
    loading: false,
    isError: false,
    productName: "Payverge",
    isOff: (feature: string) => mockOffFeatures.includes(feature),
    isOn: (feature: string) => !mockOffFeatures.includes(feature),
  }),
}));
type MockAuthState = {
  isWeb3User: boolean;
  isStaffUser: boolean;
  isOAuthUser: boolean;
  oauthData: { email: string; role?: string } | null;
  staffData: StaffData | null;
  isLoading: boolean;
  isInitialized: boolean;
};
const signedOutAuthState: MockAuthState = {
  isWeb3User: false,
  isStaffUser: false,
  isOAuthUser: false,
  oauthData: null,
  staffData: null,
  isLoading: false,
  isInitialized: true,
};
let mockAuthState: MockAuthState = signedOutAuthState;
let mockIsConnected = false;
let mockAddress: `0x${string}` | undefined;
jest.mock("next/navigation", () => ({
  usePathname: () => mockPathname,
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  useSearchParams: () => mockSearchParams,
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({
    isConnected: mockIsConnected,
    address: mockAddress,
  }),
}));
jest.mock("@/store", () => ({
  StoreMenu: {},
}));
let mockIsAdmin = false;
jest.mock("@/store/useUserStore", () => ({
  useUserStore: () => ({
    user: mockIsAdmin ? { role: "admin" } : null,
  }),
}));
jest.mock("@/utils/auth", () => ({
  isAdmin: () => mockIsAdmin,
}));
jest.mock("@/hooks", () => ({
  useLogout: () => ({ logout: jest.fn() }),
}));
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockAuthState,
}));
const mockRequestInstall = jest.fn<Promise<void>, []>();
let mockInstallState = "native-installable";
let mockInstallIdentity: { key: string; roleType: "owner" | "staff" } | null =
  null;
jest.mock("@/providers/PwaInstallProvider", () => ({
  usePwaInstall: () => ({
    state: mockInstallState,
    identity: mockInstallIdentity,
    requestInstall: mockRequestInstall,
  }),
}));
const mockClearStaffSession = jest.fn<Promise<void>, []>();
const mockRedirectToStaffLogin = jest.fn();
jest.mock("@/utils/staffAuth", () => ({
  clearStaffSession: () => mockClearStaffSession(),
  redirectToStaffLogin: () => mockRedirectToStaffLogin(),
  getRoleColor: () => "#000",
}));
const mockStaffLogout = jest.fn();
jest.mock("@/api/staffProfile", () => ({
  staffLogout: () => mockStaffLogout(),
}));
jest.mock("@/components/ui/web3-button/Web3Button", () => ({
  Web3Button: () => null,
}));
jest.mock("@/components/auth/AuthModal", () => ({
  AuthModal: ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div role="dialog" data-testid="top-menu-auth-modal" /> : null,
}));
jest.mock("@/components/SimpleLanguageSwitcherLazy", () => ({
  __esModule: true,
  default: () => <div data-testid="top-menu-language-switcher" />,
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));
jest.mock("next/image", () => ({
  __esModule: true,
  default: ({ src, alt }: { src: string; alt: string }) => {
    const React = jest.requireActual("react");
    return React.createElement("img", { src, alt });
  },
}));

describe("TopMenu right rail", () => {
  beforeEach(() => {
    mockPathname = "/";
    mockSearchParams = new URLSearchParams();
    mockClearStaffSession.mockReset();
    mockClearStaffSession.mockResolvedValue(undefined);
    mockRedirectToStaffLogin.mockReset();
    mockStaffLogout.mockReset();
    mockAuthState = signedOutAuthState;
    mockIsAdmin = false;
    mockIsConnected = false;
    mockAddress = undefined;
    mockInstallState = "native-installable";
    mockInstallIdentity = null;
    mockRequestInstall.mockReset();
    mockRequestInstall.mockResolvedValue(undefined);
    delete document.documentElement.dataset.operatorAccessError;
  });

  const signInOwner = () => {
    mockPathname = "/business/42/dashboard";
    mockAuthState = {
      ...signedOutAuthState,
      isOAuthUser: true,
      oauthData: { email: "owner@payverge.test", role: "user" },
    };
    mockInstallIdentity = { key: "user:19", roleType: "owner" };
  };

  it("renders the sign-in + staff entries via i18n keys (no hardcoded English)", () => {
    render(<TopMenu onReady={jest.fn()} />);
    // The mock getTranslation echoes the key, so localized labels resolve to
    // their namespaced keys. The previously-hardcoded literals must be gone.
    expect(screen.queryByText(/^Login$/)).toBeNull();
    expect(screen.queryByText(/^Get Started$/i)).toBeNull();
    // Sign-in opens the AuthModal in place — it is a button, not a route.
    expect(
      screen.getByRole("button", { name: "navigation.signIn" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "navigation.staffLogin" }),
    ).toBeTruthy();
  });

  it("leaves dashboard locale switching to the dashboard sidebar", () => {
    mockPathname = "/business/42/dashboard";
    render(<TopMenu onReady={jest.fn()} />);
    expect(screen.queryByTestId("top-menu-language-switcher")).toBeNull();
  });

  it("renders the authenticated profile dropdown trigger as a named button", () => {
    mockPathname = "/business/42/dashboard";
    mockAuthState = {
      ...signedOutAuthState,
      isStaffUser: true,
      staffData: {
        id: 7,
        business_id: 42,
        name: "Operator",
        email: "operator@payverge.test",
        role: "manager" as const,
        is_active: true,
      },
    };

    render(<TopMenu onReady={jest.fn()} />);

    expect(
      screen.getByRole("button", { name: "navigation.profileActionsAria" }),
    ).toBeInTheDocument();
  });

  it("public register route nav contains no Admin link (even for admins)", () => {
    mockPathname = "/business/register";
    mockIsAdmin = true;
    mockAuthState = {
      ...signedOutAuthState,
      isOAuthUser: true,
      oauthData: { email: "admin@payverge.test", role: "admin" },
    };

    render(<TopMenu onReady={jest.fn()} />);

    expect(screen.queryByRole("link", { name: "navigation.admin" })).toBeNull();
    expect(screen.queryByText("navigation.admin")).toBeNull();
  });

  it("demotes Platform Admin out of the primary desktop nav beside Dashboard", async () => {
    mockPathname = "/business/42/dashboard";
    mockIsAdmin = true;
    mockAuthState = {
      ...signedOutAuthState,
      isOAuthUser: true,
      oauthData: { email: "admin@payverge.test", role: "admin" },
    };

    render(<TopMenu onReady={jest.fn()} />);

    // Primary center rail must not advertise Admin next to Dashboard (#62).
    expect(screen.queryByRole("link", { name: "navigation.admin" })).toBeNull();

    // Still reachable from the profile menu.
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.profileActionsAria" }),
    );
    expect(
      await screen.findByRole("menuitem", { name: /navigation\.admin/i }),
    ).toBeInTheDocument();
  });

  it("offers installation from the authenticated desktop account menu and guards rapid activation", async () => {
    signInOwner();
    let rejectInstall!: (reason?: unknown) => void;
    mockRequestInstall.mockImplementation(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectInstall = reject;
        }),
    );

    render(
      <React.StrictMode>
        <TopMenu onReady={jest.fn()} />
      </React.StrictMode>,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.profileActionsAria" }),
    );
    const installAction = await screen.findByText("pwa.account.label");
    act(() => {
      fireEvent.click(installAction);
      fireEvent.click(installAction);
    });

    expect(mockRequestInstall).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "navigation.profileActionsAria" }),
      ).toBeInTheDocument(),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.profileActionsAria" }),
    );
    const pendingAction = await screen.findByText("pwa.account.label");
    const installMenuItem = pendingAction.closest('[role="menuitem"]');
    expect(installMenuItem).toHaveAttribute("aria-disabled", "true");
    expect(installMenuItem).toHaveAttribute("aria-busy", "true");
    rejectInstall(new Error("prompt failed"));
    await waitFor(() => {
      expect(installMenuItem).toHaveAttribute("aria-busy", "false");
    });
  });

  it("offers installation in the authenticated mobile drawer with disabled busy state", async () => {
    signInOwner();
    let resolveInstall!: () => void;
    let drawerPresentWhenInstallStarts: boolean | null = null;
    mockRequestInstall.mockImplementation(() => {
      drawerPresentWhenInstallStarts = Boolean(
        screen.queryByRole("dialog", {
          name: "navigation.toggleMenuAria",
        }),
      );
      return new Promise<void>((resolve) => {
        resolveInstall = resolve;
      });
    });

    render(
      <React.StrictMode>
        <TopMenu onReady={jest.fn()} />
      </React.StrictMode>,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.toggleMenuAria" }),
    );
    const installAction = screen.getByRole("button", {
      name: "pwa.account.label",
    });
    act(() => {
      fireEvent.click(installAction);
      fireEvent.click(installAction);
    });

    expect(mockRequestInstall).toHaveBeenCalledTimes(1);
    expect(drawerPresentWhenInstallStarts).toBe(false);
    expect(
      screen.queryByRole("dialog", { name: "navigation.toggleMenuAria" }),
    ).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "navigation.toggleMenuAria" }),
    );
    const pendingAction = screen.getByRole("button", {
      name: "pwa.account.label",
    });
    expect(pendingAction).toBeDisabled();
    expect(pendingAction).toHaveAttribute("aria-busy", "true");
    fireEvent.click(pendingAction);
    expect(mockRequestInstall).toHaveBeenCalledTimes(1);

    resolveInstall();
    await waitFor(() =>
      expect(pendingAction).not.toHaveAttribute("aria-busy", "true"),
    );
  });

  it("never offers installation for a connected wallet without a trusted identity", async () => {
    mockIsConnected = true;
    mockAddress = "0xabc";
    mockPathname = "/business/casa/dashboard";

    render(<TopMenu onReady={jest.fn()} />);
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.profileActionsAria" }),
    );
    expect(await screen.findByText("navigation.account")).toBeInTheDocument();
    expect(screen.queryByText("pwa.account.label")).toBeNull();

    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "navigation.toggleMenuAria" }),
      ).toBeInTheDocument(),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.toggleMenuAria" }),
    );
    expect(
      screen.queryByRole("button", { name: "pwa.account.label" }),
    ).toBeNull();
  });

  it.each(["staff", "web3"] as const)(
    "offers installation for a trusted authenticated %s identity",
    async (identityType) => {
      mockPathname = "/business/casa/dashboard";
      if (identityType === "staff") {
        mockAuthState = {
          ...signedOutAuthState,
          isStaffUser: true,
          staffData: {
            id: 7,
            business_id: 42,
            name: "Operator",
            email: "operator@payverge.test",
            role: "manager" as const,
            is_active: true,
          },
        };
        mockInstallIdentity = {
          key: "staff:7:42",
          roleType: "staff",
        };
      } else {
        mockIsConnected = true;
        mockAddress = "0xabc";
        mockAuthState = { ...signedOutAuthState, isWeb3User: true };
        mockInstallIdentity = {
          key: "web3:0xabc",
          roleType: "owner",
        };
      }

      render(<TopMenu onReady={jest.fn()} />);
      fireEvent.click(
        screen.getByRole("button", { name: "navigation.profileActionsAria" }),
      );

      expect(await screen.findByText("pwa.account.label")).toBeInTheDocument();
    },
  );

  it("hides install actions only in standalone mode", async () => {
    signInOwner();
    mockInstallState = "installed";

    render(<TopMenu onReady={jest.fn()} />);
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.profileActionsAria" }),
    );
    expect(await screen.findByText("navigation.account")).toBeInTheDocument();
    expect(screen.queryByText("pwa.account.label")).toBeNull();

    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "navigation.toggleMenuAria" }),
      ).toBeInTheDocument(),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.toggleMenuAria" }),
    );
    expect(
      screen.queryByRole("button", { name: "pwa.account.label" }),
    ).toBeNull();
  });

  it("does not render the public Home/Install hamburger on operator access error", () => {
    signInOwner();
    document.documentElement.dataset.operatorAccessError = "1";

    render(<TopMenu onReady={jest.fn()} />);

    expect(
      screen.queryByRole("button", { name: "navigation.toggleMenuAria" }),
    ).toBeNull();
    expect(screen.queryByRole("link", { name: "navigation.home" })).toBeNull();
    expect(screen.queryByText("pwa.account.label")).toBeNull();
  });
});

describe("TopMenu ?auth dialog ownership", () => {
  beforeEach(() => {
    mockPathname = "/";
    mockSearchParams = new URLSearchParams();
    mockAuthState = signedOutAuthState;
    mockIsAdmin = false;
    mockIsConnected = false;
    mockAddress = undefined;
    mockInstallIdentity = null;
  });

  it("leaves /dashboard?auth=signin to the dashboard page's own dialog", () => {
    mockPathname = "/dashboard";
    mockSearchParams = new URLSearchParams("auth=signin");
    render(<TopMenu onReady={jest.fn()} />);
    expect(screen.queryByTestId("top-menu-auth-modal")).toBeNull();
  });

  it("opens from ?auth elsewhere and closes once a session exists", () => {
    mockPathname = "/business/register";
    mockSearchParams = new URLSearchParams("auth=signin");
    const view = render(<TopMenu onReady={jest.fn()} />);
    expect(screen.getByTestId("top-menu-auth-modal")).toBeInTheDocument();

    // Demo one-click / email sign-in lands: the provider reports a session
    // while ?auth is still in the URL.
    mockAuthState = {
      ...signedOutAuthState,
      isOAuthUser: true,
      oauthData: { email: "owner@payverge.test", role: "user" },
    };
    view.rerender(<TopMenu onReady={jest.fn()} />);
    expect(screen.queryByTestId("top-menu-auth-modal")).toBeNull();
  });

  it("closes the header sign-in dialog once a session exists", () => {
    const view = render(<TopMenu onReady={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "navigation.signIn" }));
    expect(screen.getByTestId("top-menu-auth-modal")).toBeInTheDocument();

    mockAuthState = {
      ...signedOutAuthState,
      isOAuthUser: true,
      oauthData: { email: "owner@payverge.test", role: "user" },
    };
    view.rerender(<TopMenu onReady={jest.fn()} />);
    expect(screen.queryByTestId("top-menu-auth-modal")).toBeNull();
  });
});

describe("TopMenu staff logout", () => {
  beforeEach(() => {
    mockPathname = "/business/42/dashboard";
    mockSearchParams = new URLSearchParams();
    mockIsAdmin = false;
    mockIsConnected = false;
    mockInstallIdentity = null;
    mockClearStaffSession.mockReset();
    mockClearStaffSession.mockResolvedValue(undefined);
    mockRedirectToStaffLogin.mockReset();
    mockStaffLogout.mockReset();
    mockAuthState = {
      ...signedOutAuthState,
      isStaffUser: true,
      staffData: {
        id: 7,
        business_id: 42,
        name: "Operator",
        email: "operator@payverge.test",
        role: "manager" as const,
        is_active: true,
      },
    };
  });

  it("sends a single /staff/logout and then leaves with a full page load", async () => {
    render(<TopMenu onReady={jest.fn()} />);
    fireEvent.click(
      screen.getByRole("button", { name: "navigation.profileActionsAria" }),
    );
    fireEvent.click(await screen.findByText("navigation.logout"));

    await waitFor(() => expect(mockRedirectToStaffLogin).toHaveBeenCalledTimes(1));
    expect(mockClearStaffSession).toHaveBeenCalledTimes(1);
    expect(mockStaffLogout).not.toHaveBeenCalled();
    expect(mockClearStaffSession.mock.invocationCallOrder[0]).toBeLessThan(
      mockRedirectToStaffLogin.mock.invocationCallOrder[0],
    );
  });
});

describe("TopMenu public chrome without the marketing site", () => {
  beforeEach(() => {
    mockPathname = "/business/register";
    mockAuthState = signedOutAuthState;
    mockIsAdmin = false;
    mockIsConnected = false;
    delete document.documentElement.dataset.operatorAccessError;
  });

  it("never links into the removed sales pages but keeps sign-in and staff login", () => {
    render(<TopMenu onReady={jest.fn()} />);
    const salesHrefs = screen
      .queryAllByRole("link", { hidden: true })
      .map((link) => link.getAttribute("href"))
      .filter((h) =>
        ["/features", "/ai-features", "/pricing", "/contact", "/blog"].includes(
          h ?? "",
        ),
      );
    expect(salesHrefs).toEqual([]);
    expect(
      screen.getByRole("button", { name: "navigation.signIn" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "navigation.staffLogin" }),
    ).toBeTruthy();
  });
});
