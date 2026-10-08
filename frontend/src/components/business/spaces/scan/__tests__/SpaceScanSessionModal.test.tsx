/** @jest-environment jsdom */

import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";

const mockCreate = jest.fn();
const mockGet = jest.fn();
const mockCancel = jest.fn();
const mockRetry = jest.fn();

jest.mock("@/api/spaces", () => {
  const actual = jest.requireActual("@/api/spaces");
  return {
    ...actual,
    spacesApi: {
      createScanSession: (...a: unknown[]) => mockCreate(...a),
      getScanSession: (...a: unknown[]) => mockGet(...a),
      cancelScanSession: (...a: unknown[]) => mockCancel(...a),
      retryProcessScanSession: (...a: unknown[]) => mockRetry(...a),
      getLayoutDraft: jest.fn(),
      putLayoutDraft: jest.fn(),
    },
  };
});

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));

jest.mock("qrcode", () => ({
  __esModule: true,
  default: {
    toDataURL: jest.fn().mockResolvedValue("data:image/png;base64,qr"),
  },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

import SpaceScanSessionModal from "../SpaceScanSessionModal";

const t = (key: string) => key;

describe("SpaceScanSessionModal", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    Object.defineProperty(navigator, "userAgent", {
      value: "Mozilla/5.0 (Macintosh; Intel Mac OS X)",
      configurable: true,
    });
    mockCreate.mockResolvedValue({
      session: {
        id: 9,
        business_id: 1,
        space_id: 2,
        token_prefix: "abc",
        status: "waiting_for_phone",
        expires_at: new Date(Date.now() + 3600_000).toISOString(),
        progress_pct: 0,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      token: "tok_test_token_value_long",
      pair_code: "654321",
    });
    mockGet.mockResolvedValue({
      session: {
        id: 9,
        status: "waiting_for_phone",
        progress_pct: 0,
      },
      uploads: [],
    });
  });

  it("creates a session and shows QR + pair code + statuses", async () => {
    await act(async () => {
      render(
        <SpaceScanSessionModal
          isOpen
          onOpenChange={jest.fn()}
          businessId={1}
          spaceId={2}
          t={t}
          onReviewReady={jest.fn()}
        />,
      );
    });

    await waitFor(() => {
      expect(mockCreate).toHaveBeenCalledWith(1, 2, expect.any(Object));
    });

    expect(await screen.findByTestId("space-scan-pairing")).toBeInTheDocument();
    expect(screen.getByTestId("space-scan-qr")).toBeInTheDocument();
    expect(screen.getByTestId("space-scan-pair-code")).toHaveTextContent(
      "654321",
    );
    expect(screen.getByTestId("space-scan-status")).toHaveAttribute(
      "data-status",
      "waiting_for_phone",
    );
    expect(screen.getByTestId("space-scan-url").textContent).toContain(
      "/space-scan/tok_test_token_value_long",
    );
  });

  it("surfaces draft_apply_failed banner when review session carries error_code", async () => {
    // Create returns review_ready with error fields so auto-open path runs
    // immediately and merges getScanSession (not a 2s poll theater path).
    mockCreate.mockResolvedValue({
      session: {
        id: 9,
        business_id: 1,
        space_id: 2,
        token_prefix: "abc",
        status: "review_ready",
        progress_pct: 100,
        error_code: "draft_apply_failed",
        error_message: "scan ready but draft apply failed — re-apply from review",
        expires_at: new Date(Date.now() + 3600_000).toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        result_layout_json: {
          schema_version: 1,
          width_mm: 2000,
          height_mm: 2000,
          tables: [],
        },
      },
      token: "tok_test_token_value_long",
      pair_code: "654321",
    });
    mockGet.mockResolvedValue({
      session: {
        id: 9,
        business_id: 1,
        space_id: 2,
        status: "review_ready",
        progress_pct: 100,
        error_code: "draft_apply_failed",
        error_message: "scan ready but draft apply failed — re-apply from review",
        result_layout_json: {
          schema_version: 1,
          width_mm: 2000,
          height_mm: 2000,
          tables: [],
        },
      },
      uploads: [],
    });

    await act(async () => {
      render(
        <SpaceScanSessionModal
          isOpen
          onOpenChange={jest.fn()}
          businessId={1}
          spaceId={2}
          t={t}
          onReviewReady={jest.fn()}
        />,
      );
    });

    // Auto-open review on review_ready merges getScanSession (error_code path).
    await waitFor(() => {
      expect(mockGet).toHaveBeenCalledWith(1, 9);
    });

    expect(
      await screen.findByTestId("space-scan-draft-apply-failed"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("scan ready but draft apply failed — re-apply from review"),
    ).toBeInTheDocument();
  });
});
