/** @jest-environment jsdom */
import { renderHook, act, waitFor } from "@testing-library/react";

// --- mocks ---
const mockAxios = jest.fn();
jest.mock("@/api/tools/instance", () => ({
  axiosInstance: (...args: unknown[]) => mockAxios(...args),
}));

jest.mock("@/contexts/ConnectivityContext", () => ({
  useConnectivity: () => ({ isOnline: true, since: null }),
}));

const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: mockShowSuccess, showError: mockShowError }),
}));

import { useOfflineMutation } from "./useOfflineMutation";
import { enqueue, getQueue } from "@/lib/mutationQueue";

const USER = "u-1";

describe("useOfflineMutation re-drain backoff", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    localStorage.clear();
    mockAxios.mockReset();
    mockShowSuccess.mockReset();
    mockShowError.mockReset();
  });

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  it("re-drains a transiently-failed mutation on backoff instead of stranding it", async () => {
    // One queued mutation; first replay rejects (transient 500), second resolves.
    enqueue(USER, { method: "POST", url: "/orders/1/approve", body: {} });
    mockAxios
      .mockRejectedValueOnce(new Error("500"))
      .mockResolvedValueOnce({ data: {} });

    renderHook(() => useOfflineMutation(USER));

    // First drain attempt runs on mount.
    await act(async () => {
      await Promise.resolve();
    });
    // First replay failed -> item still queued (retries remain), not removed.
    expect(getQueue(USER).length).toBe(1);
    expect(mockAxios).toHaveBeenCalledTimes(1);

    // Advance past the first backoff delay -> re-drain fires and succeeds.
    await act(async () => {
      jest.advanceTimersByTime(3000);
      await Promise.resolve();
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(getQueue(USER).length).toBe(0);
    });
    expect(mockAxios).toHaveBeenCalledTimes(2);
  });
});
