/** @jest-environment jsdom */

import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";

const mockGetMeta = jest.fn();
const mockConnect = jest.fn();

jest.mock("@/api/spaces", () => {
  const actual = jest.requireActual("@/api/spaces");
  return {
    ...actual,
    publicSpaceScanApi: {
      getMeta: (...a: unknown[]) => mockGetMeta(...a),
      connect: (...a: unknown[]) => mockConnect(...a),
      updateStatus: jest.fn(),
      upload: jest.fn(),
      completeUpload: jest.fn(),
      getResult: jest.fn(),
    },
  };
});

jest.mock("@/api/auth/sessionInfo", () => ({
  getSessionInfo: jest.fn().mockResolvedValue({ authenticated: false }),
}));

jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(""),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

import SpaceScanClient from "../SpaceScanClient";

describe("SpaceScanClient unsupported device", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // No camera API
    Object.defineProperty(navigator, "mediaDevices", {
      value: undefined,
      configurable: true,
    });
    mockGetMeta.mockResolvedValue({
      status: "waiting_for_phone",
      expires_at: new Date(Date.now() + 3600_000).toISOString(),
      progress_pct: 0,
      business_name: "Test Bistro",
      space_name: "Main",
    });
  });

  it("shows unsupported fallback with manual editor deep-link", async () => {
    await act(async () => {
      render(<SpaceScanClient token="tok_abc" />);
    });

    await waitFor(() => {
      expect(mockGetMeta).toHaveBeenCalledWith("tok_abc");
    });

    expect(
      await screen.findByTestId("space-scan-unsupported"),
    ).toBeInTheDocument();
    const link = screen.getByTestId("space-scan-manual-editor-link");
    // Operator hub at /(shop)/dashboard — real route that lists businesses.
    expect(link).toHaveAttribute("href", "/dashboard");
    expect(link.getAttribute("href")).toMatch(/^\/(dashboard|staff|business)/);
    expect(
      screen.queryByTestId("space-scan-start-capture"),
    ).not.toBeInTheDocument();
  });
});
