/** @jest-environment jsdom */

import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import AppLaunchPage from "./page";

const mockReplace = jest.fn();
const mockRouter = { replace: mockReplace };
jest.mock("next/navigation", () => ({ useRouter: () => mockRouter }));

interface MockStaffData {
  business_id: number;
  business_slug?: string;
}

let mockAuth: {
  isInitialized: boolean;
  isLoading: boolean;
  staffData: MockStaffData | null;
} = {
  isInitialized: true,
  isLoading: false,
  staffData: null,
};
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockAuth,
}));

type MockIdentity = { key: string; roleType: "owner" | "staff" };
let mockIdentity: MockIdentity | null = {
  key: "user:19",
  roleType: "owner",
};
const mockGetRememberedDashboardPath = jest.fn(
  () => "/business/casa/dashboard" as string | null,
);
jest.mock("@/providers/PwaInstallProvider", () => ({
  usePwaInstall: () => ({
    identity: mockIdentity,
    getRememberedDashboardPath: mockGetRememberedDashboardPath,
  }),
}));

const mockGetMyBusinesses = jest.fn();
jest.mock("@/api/business", () => ({
  getMyBusinesses: () => mockGetMyBusinesses(),
}));

const mockTrackEvent = jest.fn();
jest.mock("@/utils/analytics", () => ({
  trackPwaEvent: (...args: unknown[]) => mockTrackEvent(...args),
}));

const mockBrowserPlatformSnapshot = jest.fn();
const mockDetectPwaPlatform = jest.fn();
jest.mock("@/pwa/platform", () => ({
  browserPlatformSnapshot: () => mockBrowserPlatformSnapshot(),
  detectPwaPlatform: (...args: unknown[]) => mockDetectPwaPlatform(...args),
}));

let mockLocale = "en";
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale }),
  getTranslation: (_key: string, locale: string) =>
    locale === "es" ? "Abriendo Payverge" : "Opening Payverge",
}));

jest.mock("@nextui-org/react", () => ({
  Spinner: ({ "aria-label": ariaLabel }: { "aria-label": string }) => (
    <div role="progressbar" aria-label={ariaLabel} />
  ),
}));

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe("/app", () => {
  beforeEach(() => {
    mockReplace.mockReset();
    mockGetMyBusinesses.mockReset();
    mockGetMyBusinesses.mockResolvedValue([{ id: 42, business_id: "casa" }]);
    mockGetRememberedDashboardPath.mockReset();
    mockGetRememberedDashboardPath.mockReturnValue("/business/casa/dashboard");
    mockTrackEvent.mockReset();
    mockBrowserPlatformSnapshot.mockReset();
    mockBrowserPlatformSnapshot.mockReturnValue({
      userAgent: "Mozilla/5.0",
      platform: "MacIntel",
      maxTouchPoints: 0,
      standalone: false,
      displayModeStandalone: false,
    });
    mockDetectPwaPlatform.mockReset();
    mockDetectPwaPlatform.mockReturnValue("unavailable");
    mockLocale = "en";
    mockAuth = {
      isInitialized: true,
      isLoading: false,
      staffData: null,
    };
    mockIdentity = { key: "user:19", roleType: "owner" };
  });

  it("waits for auth initialization and renders an accessible localized status", () => {
    mockAuth = {
      isInitialized: false,
      isLoading: true,
      staffData: null,
    };
    mockLocale = "es";

    render(<AppLaunchPage />);

    expect(mockReplace).not.toHaveBeenCalled();
    expect(mockGetMyBusinesses).not.toHaveBeenCalled();
    expect(mockTrackEvent).not.toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite");
    expect(
      screen.getByRole("progressbar", { name: "Abriendo Payverge" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Abriendo Payverge")).toBeInTheDocument();
  });

  it("validates once and opens the remembered owner business", async () => {
    const view = render(<AppLaunchPage />);

    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/business/casa/dashboard"),
    );
    view.rerender(<AppLaunchPage />);

    expect(mockGetMyBusinesses).toHaveBeenCalledTimes(1);
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(mockTrackEvent).not.toHaveBeenCalledWith(
      "pwa_standalone_launched",
      expect.anything(),
    );
  });

  it("reports one owner launch from a standalone display context", async () => {
    const standaloneSnapshot = {
      userAgent: "Mozilla/5.0",
      platform: "MacIntel",
      maxTouchPoints: 0,
      standalone: false,
      displayModeStandalone: true,
    };
    mockBrowserPlatformSnapshot.mockReturnValue(standaloneSnapshot);
    mockDetectPwaPlatform.mockReturnValue("installed");
    const view = render(<AppLaunchPage />);

    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/business/casa/dashboard"),
    );
    view.rerender(<AppLaunchPage />);

    expect(mockBrowserPlatformSnapshot).toHaveBeenCalledTimes(1);
    expect(mockDetectPwaPlatform).toHaveBeenCalledWith(standaloneSnapshot);
    expect(
      mockTrackEvent.mock.calls.filter(
        ([eventName]) => eventName === "pwa_standalone_launched",
      ),
    ).toEqual([
      [
        "pwa_standalone_launched",
        { role_type: "owner", destination: "business" },
      ],
    ]);
  });

  it("uses current staff auth data without calling the owner API", async () => {
    mockIdentity = { key: "staff:7:42", roleType: "staff" };
    mockAuth = {
      isInitialized: true,
      isLoading: false,
      staffData: { business_id: 42, business_slug: "casa" },
    };

    render(<AppLaunchPage />);

    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/business/casa/dashboard"),
    );
    expect(mockGetMyBusinesses).not.toHaveBeenCalled();
    expect(mockGetRememberedDashboardPath).not.toHaveBeenCalled();
    expect(mockTrackEvent).not.toHaveBeenCalledWith(
      "pwa_standalone_launched",
      expect.anything(),
    );
  });

  it("falls back when no PWA identity is available", async () => {
    mockIdentity = null;

    render(<AppLaunchPage />);

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/dashboard"));
    expect(mockGetMyBusinesses).not.toHaveBeenCalled();
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_launch_resolution_error", {
      role_type: "unknown",
      destination: "chooser",
    });
    expect(
      mockTrackEvent.mock.calls.filter(
        ([eventName]) => eventName === "pwa_launch_resolution_error",
      ),
    ).toHaveLength(1);
  });

  it("keeps missing-identity analytics non-gating", async () => {
    mockIdentity = null;
    mockTrackEvent.mockImplementation(() => {
      throw new Error("analytics unavailable");
    });

    render(<AppLaunchPage />);

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/dashboard"));
  });

  it("falls back when remembered-path storage throws", async () => {
    mockGetRememberedDashboardPath.mockImplementation(() => {
      throw new Error("storage blocked");
    });

    render(<AppLaunchPage />);

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/dashboard"));
    expect(mockGetMyBusinesses).not.toHaveBeenCalled();
  });

  it("opens the chooser without reporting an error when an owner has no remembered path", async () => {
    mockGetRememberedDashboardPath.mockReturnValue(null);

    render(<AppLaunchPage />);

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/dashboard"));
    expect(mockTrackEvent).not.toHaveBeenCalledWith(
      "pwa_launch_resolution_error",
      expect.anything(),
    );
  });

  it("reports a current owner resolution error once before chooser fallback", async () => {
    mockGetRememberedDashboardPath.mockReturnValue(
      "/business/revoked/dashboard",
    );

    render(<AppLaunchPage />);

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/dashboard"));
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_launch_resolution_error", {
      role_type: "owner",
      destination: "chooser",
    });
    expect(
      mockTrackEvent.mock.calls.filter(
        ([eventName]) => eventName === "pwa_launch_resolution_error",
      ),
    ).toHaveLength(1);
    const errorOrder = mockTrackEvent.mock.invocationCallOrder.find(
      (_order, index) =>
        mockTrackEvent.mock.calls[index]?.[0] === "pwa_launch_resolution_error",
    );
    expect(errorOrder).toBeLessThan(mockReplace.mock.invocationCallOrder[0]);
  });

  it("reports an unsafe staff slug while launching the current numeric business", async () => {
    mockIdentity = { key: "staff:7:42", roleType: "staff" };
    mockAuth = {
      isInitialized: true,
      isLoading: false,
      staffData: { business_id: 42, business_slug: "../other" },
    };

    render(<AppLaunchPage />);

    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/business/42/dashboard"),
    );
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_launch_resolution_error", {
      role_type: "staff",
      destination: "business",
    });
  });

  it("navigates even when analytics throws", async () => {
    mockDetectPwaPlatform.mockReturnValue("installed");
    mockTrackEvent.mockImplementation(() => {
      throw new Error("analytics unavailable");
    });

    render(<AppLaunchPage />);

    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/business/casa/dashboard"),
    );
  });

  it("keeps resolution-error analytics non-gating", async () => {
    mockGetRememberedDashboardPath.mockReturnValue(
      "/business/revoked/dashboard",
    );
    mockTrackEvent.mockImplementation(() => {
      throw new Error("analytics unavailable");
    });

    render(<AppLaunchPage />);

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/dashboard"));
  });

  it("ignores a stale owner resolution after identity changes", async () => {
    mockDetectPwaPlatform.mockReturnValue("installed");
    const ownerBusinesses = deferred<{ id: number; business_id?: string }[]>();
    mockGetMyBusinesses.mockReturnValue(ownerBusinesses.promise);
    const view = render(<AppLaunchPage />);

    mockIdentity = { key: "staff:7:9", roleType: "staff" };
    mockAuth = {
      isInitialized: true,
      isLoading: false,
      staffData: { business_id: 9, business_slug: "new-cafe" },
    };
    view.rerender(<AppLaunchPage />);
    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/business/new-cafe/dashboard"),
    );

    await act(async () => {
      ownerBusinesses.resolve([]);
      await ownerBusinesses.promise;
    });
    expect(mockReplace).not.toHaveBeenCalledWith("/business/casa/dashboard");
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(
      mockTrackEvent.mock.calls.filter(
        ([eventName]) => eventName === "pwa_standalone_launched",
      ),
    ).toEqual([
      [
        "pwa_standalone_launched",
        { role_type: "staff", destination: "business" },
      ],
    ]);
    expect(mockTrackEvent).not.toHaveBeenCalledWith(
      "pwa_launch_resolution_error",
      expect.anything(),
    );
  });

  it("does not navigate after unmount while validation is pending", async () => {
    mockDetectPwaPlatform.mockReturnValue("installed");
    const ownerBusinesses = deferred<{ id: number; business_id?: string }[]>();
    mockGetMyBusinesses.mockReturnValue(ownerBusinesses.promise);
    const view = render(<AppLaunchPage />);
    view.unmount();

    await act(async () => {
      ownerBusinesses.reject(new Error("network unavailable"));
      await expect(ownerBusinesses.promise).rejects.toThrow(
        "network unavailable",
      );
    });
    expect(mockReplace).not.toHaveBeenCalled();
    expect(mockTrackEvent).not.toHaveBeenCalled();
  });
});
