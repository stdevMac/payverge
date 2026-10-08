/** @jest-environment jsdom */
jest.mock("@/api/index", () => ({
  axiosInstance: {
    post: jest.fn().mockResolvedValue({ data: {} }),
  },
}));

jest.mock("@/lib/analytics/consentGate", () => ({
  ...jest.requireActual("@/lib/analytics/consentGate"),
  hasAnalyticsConsent: jest.fn(),
}));

import { trackPageView } from "../analytics";
import { axiosInstance } from "@/api/index";
import { hasAnalyticsConsent } from "@/lib/analytics/consentGate";

const mockHasConsent = hasAnalyticsConsent as jest.Mock;

describe("trackPageView", () => {
  beforeEach(() => {
    (axiosInstance.post as jest.Mock).mockClear();
    mockHasConsent.mockClear();
    // Decline-by-default: no stored consent. Each test that needs consent opts in explicitly.
    mockHasConsent.mockReturnValue(false);
    sessionStorage.clear();
  });

  it("POSTs to /analytics/page-view with the expected payload shape", async () => {
    // Grant analytics consent for this test.
    mockHasConsent.mockReturnValue(true);

    await trackPageView("/foo");

    const postMock = axiosInstance.post as jest.Mock;
    expect(postMock).toHaveBeenCalled();

    // Find the page-view call (first one, since there's no prior page to emit
    // a duration event for).
    const pageViewCall = postMock.mock.calls.find(
      (call) => call[0] === "/analytics/page-view",
    );
    expect(pageViewCall).toBeDefined();

    const [url, body, config] = pageViewCall!;
    expect(url).toBe("/analytics/page-view");
    expect(config).toEqual(expect.objectContaining({ _skipErrorToast: true }));
    expect(body).toMatchObject({
      page: "/foo",
      session_id: expect.any(String),
      referrer: expect.any(String),
      user_agent: expect.any(String),
      device_type: expect.any(String),
      browser: expect.any(String),
      os: expect.any(String),
      screen_width: expect.any(Number),
      screen_height: expect.any(Number),
      locale: expect.any(String),
    });
  });

  it("does NOT POST when analytics consent has not been granted (regression guard for consent gate)", async () => {
    // consent is false (default in beforeEach) — the gate must drop the event.
    mockHasConsent.mockReturnValue(false);

    await trackPageView("/no-consent-page");

    expect(axiosInstance.post).not.toHaveBeenCalled();
  });

  it("swallows errors from the underlying POST (fire-and-forget)", async () => {
    // Grant consent so the POST path is actually exercised and can throw.
    mockHasConsent.mockReturnValue(true);

    (axiosInstance.post as jest.Mock).mockRejectedValueOnce(
      new Error("network down"),
    );
    const errSpy = jest.spyOn(console, "error").mockImplementation(() => {});

    await expect(trackPageView("/bar")).resolves.toBeUndefined();

    errSpy.mockRestore();
  });
});
