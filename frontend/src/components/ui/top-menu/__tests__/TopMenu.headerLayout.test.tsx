/** @jest-environment jsdom */
/**
 * #668 — es-AR marketing header at 1024px.
 *
 * The public chrome must keep the locale control and primary auth CTAs on
 * one unclipped line. Center nav moves into the drawer until `xl` so
 * "Español (Argentina)" / "Acceso Personal" / "Iniciar sesión" cannot blow
 * the 1024px rail.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";
import { TopMenu } from "../TopMenu";
import {
  HEADER_SIGN_IN_CLASS,
  HEADER_STAFF_LINK_CLASS,
  MOBILE_ONLY_CLASS,
} from "../TopMenuShell";

let mockPathname = "/es-ar";

jest.mock("next/navigation", () => ({
  usePathname: () => mockPathname,
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ isConnected: false, address: undefined }),
}));

jest.mock("@/store/useUserStore", () => ({
  useUserStore: () => ({ user: null }),
}));

jest.mock("@/utils/auth", () => ({
  isAdmin: () => false,
}));

jest.mock("@/hooks", () => ({
  useLogout: () => ({ logout: jest.fn() }),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isWeb3User: false,
    isStaffUser: false,
    isOAuthUser: false,
    oauthData: null,
    staffData: null,
    isLoading: false,
    isInitialized: true,
  }),
}));

jest.mock("@/providers/PwaInstallProvider", () => ({
  usePwaInstall: () => ({
    state: "unavailable",
    identity: null,
    requestInstall: jest.fn(),
  }),
}));

jest.mock("@/components/ui/web3-button/Web3Button", () => ({
  Web3Button: () => null,
}));

jest.mock("@/components/auth/AuthModal", () => ({
  AuthModal: () => null,
}));

jest.mock("@/components/SimpleLanguageSwitcherLazy", () => ({
  __esModule: true,
  default: ({ compact }: { compact?: boolean }) => (
    <button type="button" data-testid={compact ? "lang-compact" : "lang-full"}>
      {compact ? "ES-AR" : "Español (AR)"}
    </button>
  ),
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: ({ src, alt }: { src: string; alt: string }) => {
    const React = jest.requireActual("react");
    return React.createElement("img", { src, alt });
  },
}));

function renderEsArHeader() {
  mockPathname = "/es-ar";
  return render(
    <SimpleTranslationProvider initialLocale="es-AR">
      <TopMenu onReady={jest.fn()} />
    </SimpleTranslationProvider>,
  );
}

describe("TopMenu es-AR header layout (#668)", () => {
  beforeEach(() => {
    mockPathname = "/es-ar";
    delete document.documentElement.dataset.operatorAccessError;
  });

  it("uses compact locale chrome below xl and never paints Español (Argentina)", () => {
    renderEsArHeader();
    const compactWrap = screen.getByTestId("header-locale-compact");
    const fullWrap = screen.getByTestId("header-locale-full");
    expect(compactWrap.className).toBe("xl:hidden");
    expect(fullWrap.className).toBe("hidden xl:inline-flex");
    expect(screen.getByTestId("lang-compact")).toHaveTextContent("ES-AR");
    expect(screen.getByTestId("lang-full")).toHaveTextContent("Español (AR)");
    expect(screen.queryByText("Español (Argentina)")).toBeNull();
  });

  // #811 refined #668: staff label matches es and the footer ("Acceso
  // Personal") instead of the clipped-looking "Personal"; signIn stays short.
  it("keeps primary auth CTAs on one line with short es-AR sign-in copy", () => {
    renderEsArHeader();
    const signIn = screen.getByRole("button", { name: "Ingresar" });
    expect(signIn.className).toBe(HEADER_SIGN_IN_CLASS);
    expect(signIn.className).toMatch(/whitespace-nowrap/);
    expect(signIn.className).toMatch(/shrink-0/);
    const staff = screen.getByRole("link", { name: "Acceso Personal" });
    expect(staff.className).toBe(HEADER_STAFF_LINK_CLASS);
    expect(staff.className).toMatch(/whitespace-nowrap/);
    expect(screen.queryByRole("button", { name: /iniciar sesión/i })).toBeNull();
  });

  it("keeps the hamburger for phones only now the center nav is just Home", () => {
    renderEsArHeader();
    const hamburger = screen.getByRole("button", {
      name: "Alternar menú",
    });
    expect(hamburger.className).toMatch(
      new RegExp(`(?:^|\\s)${MOBILE_ONLY_CLASS}(?:\\s|$)`),
    );
  });
});
