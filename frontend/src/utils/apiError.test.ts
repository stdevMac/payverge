import {
  apiErrorDetail,
  errMessage,
  getApiErrorStatus,
  getApiErrorData,
  getApiErrorMessage,
  getApiErrorCode,
  getLocalizedApiError,
  getSafeApiErrorMessage,
  isApiNetworkError,
  isNonSessionAuthFailure,
  isRawValidatorDump,
} from "@/utils/apiError";

// The shared axiosInstance interceptor rejects with a SANITIZED plain Error
// that copies status/code/response but carries NO `isAxiosError` flag.
const sanitized = (
  status: number | undefined,
  data?: { error?: string; code?: string },
) =>
  Object.assign(new Error("request failed"), {
    status,
    code: data?.code,
    response: status === undefined ? undefined : { status, data },
  });

// A raw AxiosError (e.g. pre-interceptor / direct axios use) keys on
// `isAxiosError: true`.
const rawAxios = (
  status: number | undefined,
  data?: { error?: string; code?: string },
) =>
  Object.assign(new Error("request failed"), {
    isAxiosError: true,
    response: status === undefined ? undefined : { status, data },
  });

describe("apiError helpers", () => {
  describe("on the sanitized plain-Error shape", () => {
    const err = sanitized(409, { error: "Bill is not open", code: "bill_not_open" });

    it("reads the HTTP status", () => {
      expect(getApiErrorStatus(err)).toBe(409);
    });
    it("reads the response body", () => {
      expect(getApiErrorData(err)).toEqual({ error: "Bill is not open", code: "bill_not_open" });
    });
    it("reads the backend error string", () => {
      expect(getApiErrorMessage(err)).toBe("Bill is not open");
      expect(apiErrorDetail(err)).toBe("Bill is not open");
    });
    it("reads the backend code", () => {
      expect(getApiErrorCode(err)).toBe("bill_not_open");
    });
    it("is not a network error (a response is present)", () => {
      expect(isApiNetworkError(err)).toBe(false);
    });
  });

  describe("on the raw AxiosError shape", () => {
    const err = rawAxios(404, { error: "Not found", code: "missing" });

    it("reads status/data/message/code structurally", () => {
      expect(getApiErrorStatus(err)).toBe(404);
      expect(getApiErrorMessage(err)).toBe("Not found");
      expect(getApiErrorCode(err)).toBe("missing");
    });
  });

  describe("network failures (no response)", () => {
    it("flags an axios error with no response as a network error", () => {
      expect(isApiNetworkError(rawAxios(undefined))).toBe(true);
      expect(getApiErrorStatus(rawAxios(undefined))).toBeUndefined();
    });
  });

  describe("non-axios throws", () => {
    const err = new TypeError("boom");
    it("is NOT a network error and yields no api fields", () => {
      expect(isApiNetworkError(err)).toBe(false);
      expect(getApiErrorStatus(err)).toBeUndefined();
      expect(getApiErrorData(err)).toBeUndefined();
      expect(getApiErrorMessage(err)).toBeUndefined();
      expect(getApiErrorCode(err)).toBeUndefined();
    });
    it("errMessage still returns the Error message", () => {
      expect(errMessage(err)).toBe("boom");
    });
  });

  it("does not treat RBAC/authorization codes as a dead session", () => {
    expect(isNonSessionAuthFailure("AUTH_INSUFFICIENT_ROLE")).toBe(true);
    expect(isNonSessionAuthFailure("BIZ_NOT_OWNER")).toBe(true);
    expect(isNonSessionAuthFailure("BIZ_STAFF_NO_ACCESS")).toBe(true);
    expect(isNonSessionAuthFailure("AUTH_SESSION_UNKNOWN")).toBe(false);
    expect(isNonSessionAuthFailure("AUTH_TOKEN_EXPIRED")).toBe(false);
    expect(isNonSessionAuthFailure(undefined)).toBe(false);
  });

  it("tolerates null/undefined/primitive throws", () => {
    expect(getApiErrorStatus(null)).toBeUndefined();
    expect(getApiErrorData(undefined)).toBeUndefined();
    expect(getApiErrorMessage("a string")).toBeUndefined();
    expect(isApiNetworkError(42)).toBe(false);
  });

  describe("isRawValidatorDump / getLocalizedApiError gin dumps (FIND-031)", () => {
    it("detects gin Key/Field validation dumps", () => {
      expect(
        isRawValidatorDump(
          "Key: 'InviteStaffRequest.Email' Error:Field validation for 'Email' failed on the 'required' tag",
        ),
      ).toBe(true);
      expect(isRawValidatorDump("Bill is not open")).toBe(false);
    });

    it("maps gin dumps to validation catalog copy instead of raw Key: text", () => {
      const err = sanitized(400, {
        error:
          "Key: 'AddCustomerRequest.Name' Error:Field validation for 'Name' failed on the 'required' tag",
        code: "VALIDATION_INVALID_INPUT",
      });
      const msg = getLocalizedApiError(err, "en");
      expect(msg).not.toContain("Key:");
      expect(msg).not.toContain("Field validation");
      expect(msg.toLowerCase()).toMatch(/check|form|try again|invalid|required/);
    });

    it("getSafeApiErrorMessage never returns gin dumps", () => {
      const err = sanitized(400, {
        error:
          "Key: 'X.Y' Error:Field validation for 'Y' failed on the 'required' tag",
      });
      expect(getSafeApiErrorMessage(err, "fallback")).toBe("fallback");
      expect(
        getSafeApiErrorMessage(
          sanitized(400, { error: "zone name is required" }),
          "fallback",
        ),
      ).toBe("zone name is required");
    });

    // D-3: an allowlisted public 5xx (503 ai_not_configured) carries
    // error === code and a server-authored message; show the message.
    it("getSafeApiErrorMessage surfaces the ai_not_configured message, not the code", () => {
      const err = Object.assign(new Error("request failed"), {
        status: 503,
        response: {
          status: 503,
          data: {
            error: "ai_not_configured",
            code: "ai_not_configured",
            message: "AI features are not configured on this server.",
          },
        },
      });
      expect(getSafeApiErrorMessage(err, "fallback")).toBe(
        "AI features are not configured on this server.",
      );
    });

    // FIND-055: payment/wallet paths need Error.message; axios transport text must not win.
    it("getSafeApiErrorMessage prefers wallet Error.message over fallback", () => {
      expect(
        getSafeApiErrorMessage(
          new Error("User rejected the request"),
          "Payment failed",
        ),
      ).toBe("User rejected the request");
    });

    it("getSafeApiErrorMessage rejects axios status-code transport text", () => {
      const err = new Error("Request failed with status code 400");
      (err as { response?: { status: number } }).response = { status: 400 };
      expect(getSafeApiErrorMessage(err, "Payment failed")).toBe(
        "Payment failed",
      );
    });

    it("apiErrorDetail never returns gin dumps (FIND-034 residual admin paths)", () => {
      const dump = sanitized(400, {
        error:
          "Key: 'SuspendRequest.Reason' Error:Field validation for 'Reason' failed on the 'required' tag",
      });
      // Callers use `apiErrorDetail(err) || fallback` — undefined forces fallback.
      expect(apiErrorDetail(dump)).toBeUndefined();
      expect(apiErrorDetail(dump) || "Failed to suspend business").toBe(
        "Failed to suspend business",
      );
      expect(
        apiErrorDetail(
          sanitized(400, { error: "Business is already suspended" }),
        ),
      ).toBe("Business is already suspended");
    });
  });
});
