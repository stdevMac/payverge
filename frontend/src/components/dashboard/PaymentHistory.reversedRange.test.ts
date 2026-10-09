import {
  customRangeClientError,
  isReversedCustomRange,
  paymentHistoryListError,
  periodTabsValueForCustom,
} from "./paymentHistoryRange";
import enApiErrors from "@/i18n/locales/en/apiErrors.json";

/**
 * L6-9: drives the SHIPPED paymentHistoryRange helpers used by PaymentHistory
 * (client gate, period chip blanking, server error surface).
 */
describe("isReversedCustomRange", () => {
  it("detects start after end", () => {
    expect(isReversedCustomRange("2026-08-10", "2026-08-01")).toBe(true);
  });

  it("accepts start equal or before end", () => {
    expect(isReversedCustomRange("2026-08-01", "2026-08-10")).toBe(false);
    expect(isReversedCustomRange("2026-08-01", "2026-08-01")).toBe(false);
  });
});

describe("customRangeClientError (L6-9 client gate)", () => {
  const msg = "Start date must be on or before the end date.";

  it("returns reversed message when custom is active and start > end", () => {
    expect(customRangeClientError(true, "2026-08-10", "2026-08-01", msg)).toBe(
      msg,
    );
  });

  it("returns null when range is valid or custom is off", () => {
    expect(customRangeClientError(true, "2026-08-01", "2026-08-10", msg)).toBeNull();
    expect(customRangeClientError(false, "2026-08-10", "2026-08-01", msg)).toBeNull();
  });
});

describe("periodTabsValueForCustom (L6-9 chip)", () => {
  it("uses a non-option sentinel while Personalizado is active (not empty string)", () => {
    // Empty string made NextUI Tabs auto-select the first chip and clear custom.
    expect(periodTabsValueForCustom(true, "month")).toBe("__custom__");
    expect(periodTabsValueForCustom(true, "quarter")).toBe("__custom__");
    expect(periodTabsValueForCustom(true, "month")).not.toBe("month");
  });

  it("keeps the preset key when custom is off", () => {
    expect(periodTabsValueForCustom(false, "month")).toBe("month");
    expect(periodTabsValueForCustom(false, "year")).toBe("year");
  });
});

describe("paymentHistoryListError (L6-9 server surface)", () => {
  it("maps a coded 400 via surfaceBackendError", () => {
    const err = {
      response: {
        status: 400,
        data: {
          code: "VALIDATION_INVALID_INPUT",
          error: "invalid date range",
        },
      },
    };
    expect(paymentHistoryListError(err, "en")).toBe(
      enApiErrors.VALIDATION_INVALID_INPUT,
    );
  });

  it("surfaces product-safe domain copy for uncoded 400", () => {
    const err = {
      response: {
        status: 400,
        data: { error: "end date must be after start date" },
      },
    };
    expect(paymentHistoryListError(err, "en")).toBe(
      "end date must be after start date",
    );
  });
});
