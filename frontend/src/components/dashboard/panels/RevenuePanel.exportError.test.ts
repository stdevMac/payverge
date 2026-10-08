import { runRevenueExport } from "./revenueExport";
import enCommon from "@/i18n/messages/en/common.json";

/**
 * L6-7: drives the SHIPPED runRevenueExport path used by
 * RevenuePanel.handleExportData — 413 must toast range-too-large via onError.
 */
const api413 = Object.assign(new Error("request failed"), {
  status: 413,
  response: {
    status: 413,
    data: { error: "choose a period of 31 days or less" },
  },
});

describe("runRevenueExport (L6-7 shipped path)", () => {
  it("calls onError with range-too-large copy when export rejects 413", async () => {
    const onError = jest.fn();
    const result = await runRevenueExport({
      exportFn: jest.fn().mockRejectedValue(api413),
      locale: "en",
      fallback: "Could not export sales data. Try a shorter period.",
      onError,
    });
    expect(result).toBe("error");
    expect(onError).toHaveBeenCalledTimes(1);
    expect(onError).toHaveBeenCalledWith(enCommon.errors.rangeTooLarge);
  });

  it("returns ok and does not call onError when export succeeds", async () => {
    const onError = jest.fn();
    const blob = new Blob(["a,b\n"], { type: "text/csv" });
    const result = await runRevenueExport({
      exportFn: jest.fn().mockResolvedValue(blob),
      locale: "en",
      fallback: "Could not export",
      onError,
    });
    expect(result).toBe("ok");
    expect(onError).not.toHaveBeenCalled();
  });

  it("would fail if catch path skipped surfaceBackendError (non-empty message)", async () => {
    const onError = jest.fn();
    await runRevenueExport({
      exportFn: jest.fn().mockRejectedValue({ response: { status: 413, data: {} } }),
      locale: "es",
      fallback: "fallback",
      onError,
    });
    expect(onError.mock.calls[0][0].length).toBeGreaterThan(10);
  });
});
