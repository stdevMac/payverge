/** @jest-environment jsdom */

import React, { StrictMode, useEffect } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { PwaInstallProvider, usePwaInstall } from "./PwaInstallProvider";

const mockTrackEvent = jest.fn();
jest.mock("@/utils/analytics", () => ({
  trackPwaEvent: (...args: unknown[]) => mockTrackEvent(...args),
}));

let mockLocale = "en";
jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale }),
}));

const authenticatedOwner = {
  isInitialized: true,
  isStaffUser: false,
  staffData: null as null | { id: number; business_id: number },
  isOAuthUser: true,
  oauthData: { userId: 19 } as null | { userId: number },
  isWeb3User: false,
  walletAddress: null as string | null,
};
let mockAuth = { ...authenticatedOwner };
jest.mock("./HybridAuthProvider", () => ({ useAuth: () => mockAuth }));

let mockStandalone = false;
let mockPlatformState: "installed" | "manual-install" | "unavailable" =
  "unavailable";
jest.mock("@/pwa/platform", () => ({
  browserPlatformSnapshot: () => ({
    userAgent: "Mozilla/5.0 Chrome/126 Safari/537.36",
    platform: "Linux armv8l",
    maxTouchPoints: 5,
    standalone: mockStandalone,
    displayModeStandalone: mockStandalone,
  }),
  detectPwaPlatform: () => (mockStandalone ? "installed" : mockPlatformState),
}));

interface MockWorker {
  postMessage: jest.Mock;
}

interface MockRegistration {
  installing: MockWorker | null;
  waiting: MockWorker | null;
  active: MockWorker | null;
}

const mockRegister = jest.fn();
const mockAddServiceWorkerListener = jest.fn();
const mockRemoveServiceWorkerListener = jest.fn();
const mockInstallingPostMessage = jest.fn();
const mockWaitingPostMessage = jest.fn();
const mockActivePostMessage = jest.fn();
let mockRegistration: MockRegistration;
let mockReadyPromise: Promise<MockRegistration>;
let mockControllerChangeListeners = new Set<EventListener>();

function installServiceWorkerContainer() {
  mockControllerChangeListeners = new Set();
  mockAddServiceWorkerListener.mockImplementation(
    (eventName: string, listener: EventListener) => {
      if (eventName === "controllerchange") {
        mockControllerChangeListeners.add(listener);
      }
    },
  );
  mockRemoveServiceWorkerListener.mockImplementation(
    (eventName: string, listener: EventListener) => {
      if (eventName === "controllerchange") {
        mockControllerChangeListeners.delete(listener);
      }
    },
  );
  Object.defineProperty(navigator, "serviceWorker", {
    configurable: true,
    value: {
      register: mockRegister,
      get ready() {
        return mockReadyPromise;
      },
      addEventListener: mockAddServiceWorkerListener,
      removeEventListener: mockRemoveServiceWorkerListener,
    },
  });
}

const mockRequestNotificationPermission = jest.fn();
Object.defineProperty(window, "Notification", {
  configurable: true,
  value: { requestPermission: mockRequestNotificationPermission },
});

interface NativeInstallEvent extends Event {
  prompt: jest.Mock<Promise<void>, []>;
  userChoice: Promise<{
    outcome: "accepted" | "dismissed";
    platform: string;
  }>;
}

function nativeInstallEvent(
  outcome: "accepted" | "dismissed" = "accepted",
): NativeInstallEvent {
  const event = new Event("beforeinstallprompt", {
    cancelable: true,
  }) as NativeInstallEvent;
  event.prompt = jest.fn().mockResolvedValue(undefined);
  event.userChoice = Promise.resolve({ outcome, platform: "web" });
  return event;
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

let lastInstallRequest: Promise<void> | null = null;

function Probe({
  allowFloatingPrompt = true,
}: {
  allowFloatingPrompt?: boolean;
}) {
  const pwa = usePwaInstall();
  const { claimFloatingPrompt } = pwa;

  useEffect(() => {
    if (!allowFloatingPrompt) return;
    return claimFloatingPrompt();
  }, [allowFloatingPrompt, claimFloatingPrompt]);

  return (
    <div>
      <span data-testid="state">{pwa.state}</span>
      <span data-testid="visible">{String(pwa.shouldShowFloatingPrompt)}</span>
      <span data-testid="help">{pwa.helpMode ?? "none"}</span>
      <button
        onClick={() => pwa.recordDashboardVisit("/business/casa/dashboard")}
      >
        visit
      </button>
      <button onClick={() => pwa.recordDashboardVisit("/staff/home")}>
        staff visit
      </button>
      <button onClick={() => pwa.recordDashboardVisit("/staff/login")}>
        invalid staff visit
      </button>
      <button
        onClick={() => {
          lastInstallRequest = pwa.requestInstall();
        }}
      >
        install
      </button>
      <button onClick={pwa.dismissForSevenDays}>dismiss</button>
      <button onClick={pwa.confirmManualInstall}>manual done</button>
    </div>
  );
}

function renderProvider(node: React.ReactNode = <Probe />) {
  return render(<PwaInstallProvider>{node}</PwaInstallProvider>);
}

function dispatchNativePrompt(outcome: "accepted" | "dismissed" = "accepted") {
  const event = nativeInstallEvent(outcome);
  act(() => window.dispatchEvent(event));
  return event;
}

describe("PwaInstallProvider", () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    mockTrackEvent.mockReset();
    mockRegister.mockReset();
    mockAddServiceWorkerListener.mockReset();
    mockRemoveServiceWorkerListener.mockReset();
    mockInstallingPostMessage.mockReset();
    mockWaitingPostMessage.mockReset();
    mockActivePostMessage.mockReset();
    mockRegistration = {
      installing: { postMessage: mockInstallingPostMessage },
      waiting: { postMessage: mockWaitingPostMessage },
      active: { postMessage: mockActivePostMessage },
    };
    mockReadyPromise = Promise.resolve(mockRegistration);
    mockRegister.mockResolvedValue(mockRegistration);
    installServiceWorkerContainer();
    lastInstallRequest = null;
    mockRequestNotificationPermission.mockClear();
    mockLocale = "en";
    mockStandalone = false;
    mockPlatformState = "unavailable";
    mockAuth = { ...authenticatedOwner, oauthData: { userId: 19 } };
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 1024,
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("registers the worker once under Strict Mode without blocking children", async () => {
    render(
      <StrictMode>
        <PwaInstallProvider>
          <Probe />
        </PwaInstallProvider>
      </StrictMode>,
    );

    expect(screen.getByTestId("state")).toHaveTextContent("unavailable");
    await waitFor(() => expect(mockRegister).toHaveBeenCalledWith("/sw.js"));
    expect(mockRegister).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(mockActivePostMessage).toHaveBeenCalledWith({
        type: "SET_OFFLINE_LOCALE",
        locale: "en",
      }),
    );
    expect(mockInstallingPostMessage).toHaveBeenCalledWith({
      type: "SET_OFFLINE_LOCALE",
      locale: "en",
    });
    expect(mockWaitingPostMessage).toHaveBeenCalledWith({
      type: "SET_OFFLINE_LOCALE",
      locale: "en",
    });
    expect(mockAddServiceWorkerListener).toHaveBeenCalledWith(
      "controllerchange",
      expect.any(Function),
    );
    expect(mockRequestNotificationPermission).not.toHaveBeenCalled();
  });

  it("resends locale to a replacement worker and cleans up its listener", async () => {
    const view = renderProvider();
    await waitFor(() => expect(mockActivePostMessage).toHaveBeenCalled());
    const replacementPostMessage = jest.fn();
    mockReadyPromise = Promise.resolve({
      installing: null,
      waiting: null,
      active: { postMessage: replacementPostMessage },
    });

    act(() => {
      for (const listener of mockControllerChangeListeners) {
        listener(new Event("controllerchange"));
      }
    });
    await waitFor(() =>
      expect(replacementPostMessage).toHaveBeenCalledWith({
        type: "SET_OFFLINE_LOCALE",
        locale: "en",
      }),
    );

    view.unmount();
    expect(mockRemoveServiceWorkerListener).toHaveBeenCalledWith(
      "controllerchange",
      expect.any(Function),
    );
    expect(mockControllerChangeListeners.size).toBe(0);
  });

  it("keeps children rendered but invalidates install when worker registration rejects", async () => {
    const registrationGate = deferred<MockRegistration>();
    const readyGate = deferred<MockRegistration>();
    mockRegister.mockReturnValue(registrationGate.promise);
    mockReadyPromise = readyGate.promise;
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");
    const view = renderProvider();
    const event = dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("state")).toHaveTextContent("native-installable");
    expect(screen.getByTestId("visible")).toHaveTextContent("true");
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(1));
    await act(async () => {
      registrationGate.reject(new Error("registration failed"));
      readyGate.reject(new Error("ready failed"));
      await Promise.resolve();
    });
    expect(screen.getByTestId("state")).toHaveTextContent("unavailable");
    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(screen.getByTestId("help")).toHaveTextContent("none");

    mockPlatformState = "manual-install";
    mockAuth = {
      ...authenticatedOwner,
      oauthData: { userId: 20 },
    };
    view.rerender(
      <PwaInstallProvider>
        <Probe />
      </PwaInstallProvider>,
    );
    expect(screen.getByTestId("state")).toHaveTextContent("unavailable");
    act(() => screen.getByRole("button", { name: "install" }).click());
    await expect(lastInstallRequest).resolves.toBeUndefined();

    expect(event.prompt).not.toHaveBeenCalled();
    expect(screen.getByTestId("help")).toHaveTextContent("unsupported");
    expect(mockTrackEvent).toHaveBeenCalledWith(
      "pwa_service_worker_registration_error",
      expect.objectContaining({ role_type: "owner" }),
    );
  });

  it("keeps install unavailable when serviceWorker capability is absent", async () => {
    Reflect.deleteProperty(navigator, "serviceWorker");
    mockPlatformState = "manual-install";
    renderProvider();

    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "install" }).click());
    await expect(lastInstallRequest).resolves.toBeUndefined();

    expect(screen.getByTestId("state")).toHaveTextContent("unavailable");
    expect(screen.getByTestId("help")).toHaveTextContent("unsupported");
    expect(mockRegister).not.toHaveBeenCalled();
  });

  it("keeps standalone installed when serviceWorker capability is absent", () => {
    Reflect.deleteProperty(navigator, "serviceWorker");
    mockStandalone = true;
    renderProvider();

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(screen.getByTestId("visible")).toHaveTextContent("false");
  });

  it("does not downgrade standalone when worker registration rejects", async () => {
    const registrationGate = deferred<MockRegistration>();
    mockRegister.mockReturnValue(registrationGate.promise);
    mockStandalone = true;
    renderProvider();

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(1));
    await act(async () => {
      registrationGate.reject(new Error("registration failed"));
      await Promise.resolve();
    });

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(screen.getByTestId("visible")).toHaveTextContent("false");
  });

  it("accepts appinstalled as final truth after registration failure", async () => {
    const registrationGate = deferred<MockRegistration>();
    mockRegister.mockReturnValue(registrationGate.promise);
    renderProvider();
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(1));
    await act(async () => {
      registrationGate.reject(new Error("registration failed"));
      await Promise.resolve();
    });
    expect(screen.getByTestId("state")).toHaveTextContent("unavailable");

    act(() => window.dispatchEvent(new Event("appinstalled")));

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(localStorage.getItem("payverge:pwa:user:19:complete")).toBe("1");
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_appinstalled_observed", {
      platform_path: "installed",
      device_category: "tablet",
      role_type: "owner",
    });
  });

  it("keeps persisted completion installed without serviceWorker capability", () => {
    localStorage.setItem("payverge:pwa:user:19:complete", "1");
    Reflect.deleteProperty(navigator, "serviceWorker");
    renderProvider();

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(screen.getByTestId("visible")).toHaveTextContent("false");
  });

  it("does not downgrade persisted completion when registration rejects", async () => {
    const registrationGate = deferred<MockRegistration>();
    mockRegister.mockReturnValue(registrationGate.promise);
    localStorage.setItem("payverge:pwa:user:19:complete", "1");
    renderProvider();

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(1));
    await act(async () => {
      registrationGate.reject(new Error("registration failed"));
      await Promise.resolve();
    });

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(screen.getByTestId("visible")).toHaveTextContent("false");
  });

  it("shows only after a resolved visit in a later session", () => {
    const first = renderProvider();
    act(() => screen.getByRole("button", { name: "visit" }).click());
    expect(screen.getByTestId("visible")).toHaveTextContent("false");

    first.unmount();
    sessionStorage.clear();
    renderProvider();
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("true");
    expect(sessionStorage.getItem("payverge:pwa:user:19:card-shown")).toBe("1");
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_install_card_viewed", {
      platform_path: "native-installable",
      device_category: "tablet",
      role_type: "owner",
    });
  });

  it("shows an eligible native install invitation on desktop", () => {
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 1440,
    });
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");

    renderProvider();
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("true");
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_install_card_viewed", {
      platform_path: "native-installable",
      device_category: "desktop",
      role_type: "owner",
    });
  });

  it("does not consume or track an eligible prompt before a gated surface claims it", () => {
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");
    renderProvider(<Probe allowFloatingPrompt={false} />);
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(
      sessionStorage.getItem("payverge:pwa:user:19:card-shown"),
    ).toBeNull();
    expect(
      mockTrackEvent.mock.calls.filter(
        ([eventName]) => eventName === "pwa_install_card_viewed",
      ),
    ).toHaveLength(0);
  });

  it("claims an eligible prompt exactly once across route transitions", () => {
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");
    const view = renderProvider(<Probe allowFloatingPrompt />);
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("true");
    expect(sessionStorage.getItem("payverge:pwa:user:19:card-shown")).toBe("1");

    view.rerender(
      <PwaInstallProvider>
        <Probe allowFloatingPrompt={false} />
      </PwaInstallProvider>,
    );
    expect(screen.getByTestId("visible")).toHaveTextContent("false");

    view.rerender(
      <PwaInstallProvider>
        <Probe allowFloatingPrompt />
      </PwaInstallProvider>,
    );
    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(
      mockTrackEvent.mock.calls.filter(
        ([eventName]) => eventName === "pwa_install_card_viewed",
      ),
    ).toHaveLength(1);
  });

  it("keeps an eligible claimed prompt idempotent under Strict Mode", () => {
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");
    render(
      <StrictMode>
        <PwaInstallProvider>
          <Probe allowFloatingPrompt />
        </PwaInstallProvider>
      </StrictMode>,
    );
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("true");
    expect(sessionStorage.getItem("payverge:pwa:user:19:card-shown")).toBe("1");
    expect(
      mockTrackEvent.mock.calls.filter(
        ([eventName]) => eventName === "pwa_install_card_viewed",
      ),
    ).toHaveLength(1);
  });

  it("establishes staff eligibility only from staff home without remembering it as a business path", () => {
    mockAuth = {
      ...authenticatedOwner,
      isStaffUser: true,
      staffData: { id: 7, business_id: 42 },
      isOAuthUser: false,
      oauthData: null,
    };
    const first = renderProvider();
    act(() => screen.getByRole("button", { name: "staff visit" }).click());
    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(
      localStorage.getItem("payverge:pwa:staff:7:42:last-dashboard"),
    ).toBeNull();

    first.unmount();
    sessionStorage.clear();
    renderProvider();
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "staff visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("true");
  });

  it("does not establish staff eligibility from arbitrary staff paths", () => {
    mockAuth = {
      ...authenticatedOwner,
      isStaffUser: true,
      staffData: { id: 7, business_id: 42 },
      isOAuthUser: false,
      oauthData: null,
    };
    localStorage.setItem("payverge:pwa:staff:7:42:eligible", "1");
    renderProvider();
    dispatchNativePrompt();

    act(() =>
      screen.getByRole("button", { name: "invalid staff visit" }).click(),
    );

    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(
      sessionStorage.getItem("payverge:pwa:staff:7:42:dashboard-session"),
    ).toBeNull();
  });

  it("captures and consumes the native prompt once without requesting notifications", async () => {
    renderProvider();
    const event = dispatchNativePrompt("dismissed");

    expect(event.defaultPrevented).toBe(true);
    expect(screen.getByTestId("state")).toHaveTextContent("native-installable");
    await act(async () =>
      screen.getByRole("button", { name: "install" }).click(),
    );
    await act(async () =>
      screen.getByRole("button", { name: "install" }).click(),
    );

    expect(event.prompt).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("help")).toHaveTextContent("unsupported");
    const outcomeCall = mockTrackEvent.mock.calls.find(
      ([eventName]) => eventName === "pwa_native_prompt_outcome",
    );
    expect(outcomeCall?.[1]).toEqual({
      platform_path: "native-installable",
      device_category: "tablet",
      role_type: "owner",
      outcome: "dismissed",
    });
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_install_action_clicked", {
      platform_path: "native-installable",
      device_category: "tablet",
      role_type: "owner",
    });
    expect(mockRequestNotificationPermission).not.toHaveBeenCalled();
  });

  it("preserves a native prompt captured before initial auth resolution", async () => {
    mockAuth = {
      ...authenticatedOwner,
      isInitialized: false,
      isOAuthUser: false,
      oauthData: null,
    };
    const view = renderProvider();
    const event = dispatchNativePrompt();
    expect(screen.getByTestId("state")).toHaveTextContent("native-installable");

    mockAuth = { ...authenticatedOwner, oauthData: { userId: 19 } };
    view.rerender(
      <PwaInstallProvider>
        <Probe />
      </PwaInstallProvider>,
    );

    expect(screen.getByTestId("state")).toHaveTextContent("native-installable");
    await act(async () =>
      screen.getByRole("button", { name: "install" }).click(),
    );
    expect(event.prompt).toHaveBeenCalledTimes(1);
  });

  it("does not let a pending native prompt undo appinstalled", async () => {
    const promptGate = deferred<void>();
    const choiceGate = deferred<{
      outcome: "accepted" | "dismissed";
      platform: string;
    }>();
    const event = nativeInstallEvent("dismissed");
    event.prompt = jest.fn(() => promptGate.promise);
    event.userChoice = choiceGate.promise;
    renderProvider();
    act(() => window.dispatchEvent(event));
    act(() => screen.getByRole("button", { name: "install" }).click());

    act(() => window.dispatchEvent(new Event("appinstalled")));
    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    await act(async () => {
      promptGate.resolve(undefined);
      choiceGate.resolve({ outcome: "dismissed", platform: "web" });
      await lastInstallRequest;
    });

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(
      localStorage.getItem("payverge:pwa:user:19:snoozed-until"),
    ).toBeNull();
  });

  it("does not let a pending prompt overwrite a newer identity", async () => {
    const promptGate = deferred<void>();
    const choiceGate = deferred<{
      outcome: "accepted" | "dismissed";
      platform: string;
    }>();
    const event = nativeInstallEvent("dismissed");
    event.prompt = jest.fn(() => promptGate.promise);
    event.userChoice = choiceGate.promise;
    const view = renderProvider();
    act(() => window.dispatchEvent(event));
    act(() => screen.getByRole("button", { name: "install" }).click());

    mockPlatformState = "manual-install";
    mockAuth = {
      ...authenticatedOwner,
      oauthData: { userId: 20 },
    };
    view.rerender(
      <PwaInstallProvider>
        <Probe />
      </PwaInstallProvider>,
    );
    expect(screen.getByTestId("state")).toHaveTextContent("manual-install");

    await act(async () => {
      promptGate.resolve(undefined);
      choiceGate.resolve({ outcome: "dismissed", platform: "web" });
      await lastInstallRequest;
    });

    expect(screen.getByTestId("state")).toHaveTextContent("manual-install");
    expect(screen.getByTestId("help")).toHaveTextContent("none");
    expect(
      localStorage.getItem("payverge:pwa:user:19:snoozed-until"),
    ).toBeNull();
    expect(
      localStorage.getItem("payverge:pwa:user:20:snoozed-until"),
    ).toBeNull();
  });

  it("keeps a newer native prompt usable while an older prompt is pending", async () => {
    const promptGate = deferred<void>();
    const choiceGate = deferred<{
      outcome: "accepted" | "dismissed";
      platform: string;
    }>();
    const oldEvent = nativeInstallEvent("accepted");
    oldEvent.prompt = jest.fn(() => promptGate.promise);
    oldEvent.userChoice = choiceGate.promise;
    renderProvider();
    act(() => window.dispatchEvent(oldEvent));
    act(() => screen.getByRole("button", { name: "install" }).click());

    const newerEvent = dispatchNativePrompt("accepted");
    await act(async () => {
      promptGate.resolve(undefined);
      choiceGate.resolve({ outcome: "accepted", platform: "web" });
      await lastInstallRequest;
    });
    expect(screen.getByTestId("state")).toHaveTextContent("native-installable");

    await act(async () =>
      screen.getByRole("button", { name: "install" }).click(),
    );
    expect(oldEvent.prompt).toHaveBeenCalledTimes(1);
    expect(newerEvent.prompt).toHaveBeenCalledTimes(1);
  });

  it("keeps appinstalled authoritative while userChoice is pending", async () => {
    const promptGate = deferred<void>();
    const choiceGate = deferred<{
      outcome: "accepted" | "dismissed";
      platform: string;
    }>();
    const event = nativeInstallEvent("dismissed");
    event.prompt = jest.fn(() => promptGate.promise);
    event.userChoice = choiceGate.promise;
    renderProvider();
    act(() => window.dispatchEvent(event));
    act(() => screen.getByRole("button", { name: "install" }).click());
    await act(async () => {
      promptGate.resolve(undefined);
      await Promise.resolve();
    });

    act(() => window.dispatchEvent(new Event("appinstalled")));
    await act(async () => {
      choiceGate.resolve({ outcome: "dismissed", platform: "web" });
      await lastInstallRequest;
    });

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(
      localStorage.getItem("payverge:pwa:user:19:snoozed-until"),
    ).toBeNull();
  });

  it("keeps a newer identity authoritative while userChoice is pending", async () => {
    const promptGate = deferred<void>();
    const choiceGate = deferred<{
      outcome: "accepted" | "dismissed";
      platform: string;
    }>();
    const event = nativeInstallEvent("dismissed");
    event.prompt = jest.fn(() => promptGate.promise);
    event.userChoice = choiceGate.promise;
    const view = renderProvider();
    act(() => window.dispatchEvent(event));
    act(() => screen.getByRole("button", { name: "install" }).click());
    await act(async () => {
      promptGate.resolve(undefined);
      await Promise.resolve();
    });

    mockPlatformState = "manual-install";
    mockAuth = {
      ...authenticatedOwner,
      oauthData: { userId: 20 },
    };
    view.rerender(
      <PwaInstallProvider>
        <Probe />
      </PwaInstallProvider>,
    );
    await act(async () => {
      choiceGate.resolve({ outcome: "dismissed", platform: "web" });
      await lastInstallRequest;
    });

    expect(screen.getByTestId("state")).toHaveTextContent("manual-install");
    expect(screen.getByTestId("help")).toHaveTextContent("none");
    expect(
      localStorage.getItem("payverge:pwa:user:19:snoozed-until"),
    ).toBeNull();
    expect(
      localStorage.getItem("payverge:pwa:user:20:snoozed-until"),
    ).toBeNull();
  });

  it("keeps a newer native event usable while old userChoice is pending", async () => {
    const promptGate = deferred<void>();
    const choiceGate = deferred<{
      outcome: "accepted" | "dismissed";
      platform: string;
    }>();
    const oldEvent = nativeInstallEvent("accepted");
    oldEvent.prompt = jest.fn(() => promptGate.promise);
    oldEvent.userChoice = choiceGate.promise;
    renderProvider();
    act(() => window.dispatchEvent(oldEvent));
    act(() => screen.getByRole("button", { name: "install" }).click());
    await act(async () => {
      promptGate.resolve(undefined);
      await Promise.resolve();
    });

    const newerEvent = dispatchNativePrompt("accepted");
    await act(async () => {
      choiceGate.resolve({ outcome: "accepted", platform: "web" });
      await lastInstallRequest;
    });
    expect(screen.getByTestId("state")).toHaveTextContent("native-installable");
    await act(async () =>
      screen.getByRole("button", { name: "install" }).click(),
    );

    expect(oldEvent.prompt).toHaveBeenCalledTimes(1);
    expect(newerEvent.prompt).toHaveBeenCalledTimes(1);
  });

  it("ignores a stale prompt rejection after identity changes", async () => {
    const promptGate = deferred<void>();
    const event = nativeInstallEvent("accepted");
    event.prompt = jest.fn(() => promptGate.promise);
    const view = renderProvider();
    act(() => window.dispatchEvent(event));
    act(() => screen.getByRole("button", { name: "install" }).click());

    mockPlatformState = "manual-install";
    mockAuth = {
      ...authenticatedOwner,
      oauthData: { userId: 20 },
    };
    view.rerender(
      <PwaInstallProvider>
        <Probe />
      </PwaInstallProvider>,
    );
    await act(async () => {
      promptGate.reject(new Error("stale prompt failure"));
      await lastInstallRequest;
    });

    expect(screen.getByTestId("state")).toHaveTextContent("manual-install");
    expect(screen.getByTestId("help")).toHaveTextContent("none");
  });

  it("opens manual instructions on iOS Safari", async () => {
    mockPlatformState = "manual-install";
    renderProvider();

    expect(screen.getByTestId("state")).toHaveTextContent("manual-install");
    await act(async () =>
      screen.getByRole("button", { name: "install" }).click(),
    );

    expect(screen.getByTestId("help")).toHaveTextContent("manual-install");
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_ios_instructions_viewed", {
      platform_path: "manual-install",
      device_category: "tablet",
      role_type: "owner",
    });
  });

  it("opens manual instructions even when analytics throws", async () => {
    mockPlatformState = "manual-install";
    mockTrackEvent.mockImplementation(() => {
      throw new Error("analytics unavailable");
    });
    renderProvider();

    act(() => screen.getByRole("button", { name: "install" }).click());
    await act(async () => {
      await expect(lastInstallRequest).resolves.toBeUndefined();
    });

    expect(screen.getByTestId("help")).toHaveTextContent("manual-install");
  });

  it("runs the native prompt even when analytics throws", async () => {
    mockTrackEvent.mockImplementation(() => {
      throw new Error("analytics unavailable");
    });
    renderProvider();
    const event = dispatchNativePrompt("accepted");

    act(() => screen.getByRole("button", { name: "install" }).click());
    await act(async () => {
      await expect(lastInstallRequest).resolves.toBeUndefined();
    });

    expect(event.prompt).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("state")).toHaveTextContent("unavailable");
  });

  it("marks explicit manual completion as installed", async () => {
    mockPlatformState = "manual-install";
    renderProvider();
    await act(async () =>
      screen.getByRole("button", { name: "install" }).click(),
    );
    act(() => screen.getByRole("button", { name: "manual done" }).click());

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(localStorage.getItem("payverge:pwa:user:19:complete")).toBe("1");
  });

  it("treats standalone display mode as installed", () => {
    mockStandalone = true;
    renderProvider();

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(screen.getByTestId("visible")).toHaveTextContent("false");
  });

  it("does not let a later browser prompt override installed truth", () => {
    mockStandalone = true;
    renderProvider();

    dispatchNativePrompt();

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
  });

  it("records appinstalled completion with sanitized analytics", () => {
    renderProvider();
    dispatchNativePrompt();
    act(() => window.dispatchEvent(new Event("appinstalled")));

    expect(screen.getByTestId("state")).toHaveTextContent("installed");
    expect(localStorage.getItem("payverge:pwa:user:19:complete")).toBe("1");
    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_appinstalled_observed", {
      platform_path: "installed",
      device_category: "tablet",
      role_type: "owner",
    });
  });

  it("never promotes installation without an authenticated operator identity", () => {
    mockAuth = {
      ...authenticatedOwner,
      isOAuthUser: false,
      oauthData: null,
    };
    renderProvider();
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(
      mockTrackEvent.mock.calls.some(
        ([eventName]) => eventName === "pwa_install_card_viewed",
      ),
    ).toBe(false);
  });

  it("clears only the prior identity launch path when auth disappears", () => {
    localStorage.setItem(
      "payverge:pwa:user:19:last-dashboard",
      "/business/casa/dashboard",
    );
    localStorage.setItem(
      "payverge:pwa:user:19:snoozed-until",
      String(Date.now() + 60_000),
    );
    const view = renderProvider();

    mockAuth = {
      ...authenticatedOwner,
      isOAuthUser: false,
      oauthData: null,
    };
    view.rerender(
      <PwaInstallProvider>
        <Probe />
      </PwaInstallProvider>,
    );

    expect(
      localStorage.getItem("payverge:pwa:user:19:last-dashboard"),
    ).toBeNull();
    expect(
      localStorage.getItem("payverge:pwa:user:19:snoozed-until"),
    ).not.toBeNull();
  });

  it("drops retained install capability when the authenticated identity logs out", async () => {
    const view = renderProvider();
    const event = dispatchNativePrompt();
    expect(screen.getByTestId("state")).toHaveTextContent("native-installable");

    mockAuth = {
      ...authenticatedOwner,
      isOAuthUser: false,
      oauthData: null,
    };
    view.rerender(
      <PwaInstallProvider>
        <Probe />
      </PwaInstallProvider>,
    );
    mockAuth = {
      ...authenticatedOwner,
      oauthData: { userId: 20 },
    };
    view.rerender(
      <PwaInstallProvider>
        <Probe />
      </PwaInstallProvider>,
    );
    await act(async () =>
      screen.getByRole("button", { name: "install" }).click(),
    );

    expect(event.prompt).not.toHaveBeenCalled();
    expect(screen.getByTestId("help")).toHaveTextContent("unsupported");
  });

  it("does not throw when auth-transition launch-path cleanup is blocked", () => {
    localStorage.setItem(
      "payverge:pwa:user:19:last-dashboard",
      "/business/casa/dashboard",
    );
    const view = renderProvider();
    const realRemoveItem = Storage.prototype.removeItem;
    jest.spyOn(Storage.prototype, "removeItem").mockImplementation(function (
      this: Storage,
      storageKey: string,
    ) {
      if (this === localStorage && storageKey.endsWith(":last-dashboard")) {
        throw new Error("blocked");
      }
      return realRemoveItem.call(this, storageKey);
    });
    mockAuth = {
      ...authenticatedOwner,
      isOAuthUser: false,
      oauthData: null,
    };

    expect(() =>
      view.rerender(
        <PwaInstallProvider>
          <Probe />
        </PwaInstallProvider>,
      ),
    ).not.toThrow();
  });

  it("fails closed when localStorage is blocked", () => {
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");
    const realRemoveItem = Storage.prototype.removeItem;
    jest.spyOn(Storage.prototype, "removeItem").mockImplementation(function (
      this: Storage,
      storageKey: string,
    ) {
      if (
        this === localStorage &&
        storageKey === "payverge:pwa:storage-probe"
      ) {
        throw new Error("blocked");
      }
      return realRemoveItem.call(this, storageKey);
    });
    renderProvider();
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(
      mockTrackEvent.mock.calls.some(
        ([eventName]) => eventName === "pwa_install_card_viewed",
      ),
    ).toBe(false);
  });

  it("fails closed when the once-per-session card marker cannot be written", () => {
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");
    const realSetItem = Storage.prototype.setItem;
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(function (
      this: Storage,
      storageKey: string,
      value: string,
    ) {
      if (this === sessionStorage && storageKey.endsWith(":card-shown")) {
        throw new Error("blocked");
      }
      return realSetItem.call(this, storageKey, value);
    });
    renderProvider();
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());

    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(
      mockTrackEvent.mock.calls.some(
        ([eventName]) => eventName === "pwa_install_card_viewed",
      ),
    ).toBe(false);
  });

  it("dismisses and snoozes the card even when analytics throws", () => {
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");
    renderProvider();
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());
    expect(screen.getByTestId("visible")).toHaveTextContent("true");
    mockTrackEvent.mockImplementation(() => {
      throw new Error("analytics unavailable");
    });

    expect(() =>
      act(() => screen.getByRole("button", { name: "dismiss" }).click()),
    ).not.toThrow();

    expect(screen.getByTestId("visible")).toHaveTextContent("false");
    expect(
      localStorage.getItem("payverge:pwa:user:19:snoozed-until"),
    ).not.toBeNull();
  });

  it("tracks card dismissal with categorical properties", () => {
    localStorage.setItem("payverge:pwa:user:19:eligible", "1");
    renderProvider();
    dispatchNativePrompt();
    act(() => screen.getByRole("button", { name: "visit" }).click());
    mockTrackEvent.mockClear();

    act(() => screen.getByRole("button", { name: "dismiss" }).click());

    expect(mockTrackEvent).toHaveBeenCalledWith("pwa_install_card_dismissed", {
      platform_path: "native-installable",
      device_category: "tablet",
      role_type: "owner",
    });
  });
});
