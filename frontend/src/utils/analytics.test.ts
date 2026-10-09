/** @jest-environment jsdom */
import { hasAnalyticsConsent } from "@/lib/analytics/consentGate";
import { axiosInstance } from "@/api/index";

jest.mock("@/lib/analytics/consentGate", () => ({
  ...jest.requireActual("@/lib/analytics/consentGate"),
  hasAnalyticsConsent: jest.fn(),
}));

jest.mock("@/api/index", () => ({
  axiosInstance: { post: jest.fn().mockResolvedValue({ data: {} }) },
}));

import {
  getAnalyticsTracker,
  PWA_ANALYTICS_EVENT_NAMES,
  trackEvent,
  trackPwaEvent,
  type PwaAnalyticsProps,
} from "./analytics";

const mockedHasConsent = hasAnalyticsConsent as jest.Mock;
const mockedPost = (axiosInstance as unknown as { post: jest.Mock }).post;

describe("AnalyticsTracker consent gate", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("does NOT POST a page view when analytics consent is absent (decline-by-default)", async () => {
    mockedHasConsent.mockReturnValue(false);
    await getAnalyticsTracker().trackPageView("/pricing");
    expect(mockedPost).not.toHaveBeenCalled();
  });

  it("does NOT POST an interaction without consent", async () => {
    mockedHasConsent.mockReturnValue(false);
    await getAnalyticsTracker().trackInteraction({
      event_type: "click",
      event_category: "button",
      event_label: "cta",
    });
    expect(mockedPost).not.toHaveBeenCalled();
  });

  it("does NOT POST a conversion without consent", async () => {
    mockedHasConsent.mockReturnValue(false);
    await getAnalyticsTracker().trackConversion({ conversion_type: "signup" });
    expect(mockedPost).not.toHaveBeenCalled();
  });

  it("DOES POST a page view once analytics consent is granted", async () => {
    mockedHasConsent.mockReturnValue(true);
    await getAnalyticsTracker().trackPageView("/pricing");
    expect(mockedPost).toHaveBeenCalledWith(
      "/analytics/page-view",
      expect.objectContaining({ page: "/pricing" }),
      expect.objectContaining({ _skipErrorToast: true }),
    );
  });
});

describe("PWA analytics privacy boundary", () => {
  beforeEach(async () => {
    jest.clearAllMocks();
    mockedPost.mockResolvedValue({ data: {} });
    mockedHasConsent.mockReturnValue(true);
    window.history.replaceState({}, "", "/business/private-slug/dashboard");

    await getAnalyticsTracker().trackPageView(
      "/business/private-slug/dashboard",
    );
    mockedPost.mockClear();
  });

  it("posts PWA events against a normalized page with categorical properties only", () => {
    trackPwaEvent("pwa_install_card_viewed", {
      platform_path: "native-installable",
      device_category: "mobile",
      role_type: "owner",
      business_id: "private-slug",
      path: "/business/private-slug/dashboard",
    } as unknown as PwaAnalyticsProps);

    expect(mockedPost).toHaveBeenCalledTimes(1);
    expect(mockedPost).toHaveBeenCalledWith(
      "/analytics/interaction",
      expect.objectContaining({
        page: "/app",
        event_type: "pwa_install_card_viewed",
        event_category: "product",
        event_label: "pwa_install_card_viewed",
      }),
      expect.objectContaining({ _skipErrorToast: true }),
    );

    const payload = mockedPost.mock.calls[0]?.[1] as Record<string, unknown>;
    expect(JSON.parse(String(payload.event_value))).toEqual({
      platform_path: "native-installable",
      device_category: "mobile",
      role_type: "owner",
    });
    expect(JSON.stringify(payload)).not.toContain("private-slug");
    expect(payload).not.toHaveProperty("pageOverride");
  });

  it("keeps the typed event inventory complete", () => {
    expect(PWA_ANALYTICS_EVENT_NAMES).toEqual([
      "pwa_service_worker_registration_error",
      "pwa_appinstalled_observed",
      "pwa_install_card_dismissed",
      "pwa_install_action_clicked",
      "pwa_ios_instructions_viewed",
      "pwa_native_prompt_outcome",
      "pwa_install_card_viewed",
      "pwa_standalone_launched",
      "pwa_launch_resolution_error",
    ]);
  });

  it("posts launch-resolution errors without business identifiers", () => {
    trackPwaEvent("pwa_launch_resolution_error", {
      role_type: "owner",
      destination: "chooser",
      business_id: "private-slug",
    } as unknown as PwaAnalyticsProps);

    expect(mockedPost).toHaveBeenCalledWith(
      "/analytics/interaction",
      expect.objectContaining({
        page: "/app",
        event_type: "pwa_launch_resolution_error",
        event_label: "pwa_launch_resolution_error",
        event_value: JSON.stringify({
          role_type: "owner",
          destination: "chooser",
        }),
      }),
      expect.objectContaining({ _skipErrorToast: true }),
    );
    expect(JSON.stringify(mockedPost.mock.calls[0]?.[1])).not.toContain(
      "private-slug",
    );
  });

  it("keeps generic product events on the current page", () => {
    trackEvent("generic_product_event", { role_type: "owner" });

    expect(mockedPost).toHaveBeenCalledWith(
      "/analytics/interaction",
      expect.objectContaining({
        page: "/business/private-slug/dashboard",
        event_type: "generic_product_event",
      }),
      expect.objectContaining({ _skipErrorToast: true }),
    );
  });

  it("does not post PWA events without analytics consent", () => {
    mockedHasConsent.mockReturnValue(false);

    trackPwaEvent("pwa_install_action_clicked", {
      platform_path: "manual-install",
      device_category: "tablet",
      role_type: "staff",
    });

    expect(mockedPost).not.toHaveBeenCalled();
  });
});

describe("analytics session id is lazy until consent", () => {
  const SESSION_KEY = "analytics_session_id";

  beforeEach(() => {
    mockedHasConsent.mockReturnValue(false);
    mockedPost.mockClear();
    mockedPost.mockResolvedValue({ data: {} });
    sessionStorage.clear();
  });

  it("does not write analytics_session_id or post when consent is absent", async () => {
    const tracker = getAnalyticsTracker();
    expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();

    await tracker.trackPageView("/pricing");
    tracker.trackClick("cta");

    expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();
    expect(mockedPost).not.toHaveBeenCalled();
  });

  it("creates the session id after consent is granted and posts that session_id", async () => {
    await getAnalyticsTracker().trackPageView("/ignored");
    expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();

    mockedHasConsent.mockReturnValue(true);
    mockedPost.mockClear();
    await getAnalyticsTracker().trackPageView("/pricing");

    const raw = sessionStorage.getItem(SESSION_KEY);
    expect(raw).not.toBeNull();
    const { id } = JSON.parse(raw as string) as { id: string };
    expect(id.length).toBeGreaterThan(0);
    expect(mockedPost).toHaveBeenCalledWith(
      "/analytics/page-view",
      expect.objectContaining({ page: "/pricing", session_id: id }),
      expect.objectContaining({ _skipErrorToast: true }),
    );
  });

  it("removes a stored session id once consent is withdrawn and an event is attempted", async () => {
    mockedHasConsent.mockReturnValue(true);
    await getAnalyticsTracker().trackPageView("/pricing");
    expect(sessionStorage.getItem(SESSION_KEY)).not.toBeNull();

    mockedHasConsent.mockReturnValue(false);
    mockedPost.mockClear();
    await getAnalyticsTracker().trackPageView("/after-decline");
    getAnalyticsTracker().trackClick("cta");

    expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();
    expect(mockedPost).not.toHaveBeenCalled();
  });

  it("removes the stored session id when consent is declined, without a tracking call", async () => {
    mockedHasConsent.mockReturnValue(true);
    await getAnalyticsTracker().trackPageView("/pricing");
    expect(sessionStorage.getItem(SESSION_KEY)).not.toBeNull();

    mockedHasConsent.mockReturnValue(false);
    window.dispatchEvent(new Event("payverge:consent-change"));

    expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();
  });
});
