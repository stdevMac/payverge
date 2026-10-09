/**
 * @jest-environment node
 *
 * LEGAL_TERMS_URL / LEGAL_PRIVACY_URL (GET /api/v1/instance) send /terms-and-
 * conditions and /privacy-policy to the operator's own pages instead of the
 * generic template or the upstream text.
 */

import { parseInstanceInfo, type InstanceInfo } from "@/lib/instance/instanceInfo";
import { externalLegalUrl } from "@/lib/instance/legalPage";

const mockRedirect = jest.fn((url: string) => {
  throw new Error(`NEXT_REDIRECT ${url}`);
});
jest.mock("next/navigation", () => ({
  redirect: (url: string) => mockRedirect(url),
  useRouter: () => ({ push: jest.fn() }),
}));

let mockInstance: InstanceInfo | null = null;
jest.mock("@/lib/instance/serverInstance", () => ({
  getServerInstanceInfo: async () => mockInstance,
}));

const configured = parseInstanceInfo({
  registration_mode: "invite",
  legal_terms_url: "https://eat.example.com/terms",
  legal_privacy_url: "https://eat.example.com/privacy",
  features: {},
});

describe("operator legal URLs", () => {
  beforeEach(() => mockRedirect.mockClear());

  it("parses only absolute http(s) URLs", () => {
    const relative = parseInstanceInfo({
      registration_mode: "invite",
      legal_terms_url: "/terms-and-conditions",
      legal_privacy_url: "javascript:alert(1)",
    });
    expect(relative?.legal_terms_url).toBe("");
    expect(relative?.legal_privacy_url).toBe("");
    expect(externalLegalUrl("terms", configured)).toBe("https://eat.example.com/terms");
    expect(externalLegalUrl("privacy", configured)).toBe("https://eat.example.com/privacy");
    expect(externalLegalUrl("refund", configured)).toBeNull();
    expect(externalLegalUrl("terms", null)).toBeNull();
  });

  it.each([
    ["../terms-and-conditions/page", "https://eat.example.com/terms"],
    ["../privacy-policy/page", "https://eat.example.com/privacy"],
  ])("%s redirects to the operator page", async (path, target) => {
    mockInstance = configured;
    const { default: Page } = await import(path);
    await expect(Page()).rejects.toThrow(`NEXT_REDIRECT ${target}`);
    expect(mockRedirect).toHaveBeenCalledWith(target);
  });

  it("renders the template when no operator URL is set", async () => {
    mockInstance = parseInstanceInfo({ registration_mode: "invite" });
    const { default: Page } = await import("../terms-and-conditions/page");
    await expect(Page()).resolves.toBeTruthy();
    expect(mockRedirect).not.toHaveBeenCalled();
  });
});
