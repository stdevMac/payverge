/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Providers } from "./providers";
import { useToast } from "@/contexts/ToastContext";

let mockPathname = "/business/casa/dashboard";
let mockPwaIdentity: { key: string; roleType: "owner" | "staff" } | null = {
  key: "user:19",
  roleType: "owner",
};
const mockClaimFloatingPrompt = jest.fn<() => void, []>(() => jest.fn());

jest.mock("next/navigation", () => ({
  usePathname: () => mockPathname,
}));

jest.mock("@nextui-org/react", () => ({
  NextUIProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

jest.mock("@/context/ThemeContext", () => ({
  ThemeProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  HybridAuthProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="hybrid-auth-provider">{children}</div>
  ),
}));

jest.mock("@/providers/PwaInstallProvider", () => ({
  PwaInstallProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="pwa-provider">{children}</div>
  ),
  usePwaInstall: () => ({
    identity: mockPwaIdentity,
    claimFloatingPrompt: mockClaimFloatingPrompt,
  }),
}));

jest.mock("@/components/pwa/PwaInstallSurface", () => {
  const ReactActual = jest.requireActual<typeof React>("react");
  function MockPwaInstallSurface({
    allowFloatingPrompt,
  }: {
    allowFloatingPrompt: boolean;
  }) {
    ReactActual.useEffect(() => {
      if (!allowFloatingPrompt) return;
      return mockClaimFloatingPrompt();
    }, [allowFloatingPrompt]);
    return (
      <div
        data-testid="pwa-surface"
        data-floating-allowed={String(allowFloatingPrompt)}
      />
    );
  }
  return {
    __esModule: true,
    default: MockPwaInstallSurface,
  };
});

jest.mock("@/components/auth/AuthGate", () => ({
  __esModule: true,
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

function ToastConsumer() {
  useToast();
  return <div>toast ready</div>;
}

describe("Providers", () => {
  beforeEach(() => {
    mockPathname = "/business/casa/dashboard";
    mockPwaIdentity = { key: "user:19", roleType: "owner" };
    mockClaimFloatingPrompt.mockClear();
  });
  it("makes ToastContext available to shop pages that call useToast", () => {
    render(
      <Providers>
        <ToastConsumer />
      </Providers>,
    );

    expect(screen.getByText("toast ready")).toBeInTheDocument();
  });

  it("mounts the PWA provider inside auth and the global install surface", () => {
    render(
      <Providers>
        <div>child</div>
      </Providers>,
    );

    const authProvider = screen.getByTestId("hybrid-auth-provider");
    const pwaProvider = screen.getByTestId("pwa-provider");
    expect(authProvider).toContainElement(pwaProvider);
    expect(pwaProvider).toContainElement(screen.getByTestId("pwa-surface"));
  });

  it.each([
    ["owner dashboard", "/business/casa/dashboard", "owner"],
    ["nested owner dashboard", "/business/casa/dashboard/bills", "owner"],
    ["staff home", "/staff/home", "staff"],
    ["nested staff home", "/staff/home/schedule", "staff"],
  ] as const)("allows the floating card on %s", (_name, pathname, roleType) => {
    mockPathname = pathname;
    mockPwaIdentity = { key: `${roleType}:trusted`, roleType };

    render(<Providers>child</Providers>);

    expect(screen.getByTestId("pwa-surface")).toHaveAttribute(
      "data-floating-allowed",
      "true",
    );
    expect(mockClaimFloatingPrompt).toHaveBeenCalledTimes(1);
  });

  it.each([
    "/",
    "/privacy-policy",
    "/menu/casa",
    "/customer/account",
    "/admin",
    "/staff/login",
    "/staff/invitation/token",
    "/business/register",
    "/business/register/dashboard",
    "/business/casa/menu",
  ])("blocks the floating card on %s", (pathname) => {
    mockPathname = pathname;

    render(<Providers>child</Providers>);

    expect(screen.getByTestId("pwa-surface")).toHaveAttribute(
      "data-floating-allowed",
      "false",
    );
    expect(mockClaimFloatingPrompt).not.toHaveBeenCalled();
  });

  it("blocks the floating card without a trusted identity", () => {
    mockPwaIdentity = null;

    render(<Providers>child</Providers>);

    expect(screen.getByTestId("pwa-surface")).toHaveAttribute(
      "data-floating-allowed",
      "false",
    );
  });

  it("does not allow an identity on the other role's shell", () => {
    mockPathname = "/staff/home";
    mockPwaIdentity = { key: "user:19", roleType: "owner" };

    render(<Providers>child</Providers>);

    expect(screen.getByTestId("pwa-surface")).toHaveAttribute(
      "data-floating-allowed",
      "false",
    );
  });
});
