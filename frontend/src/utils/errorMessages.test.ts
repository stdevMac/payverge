import { getGenericErrorMessage, sanitizeError } from "./errorMessages";

/**
 * The shared axios interceptor (api/tools/instance.ts) rejects with a SANITIZED
 * plain Error that copies `.status` / `.response.{status,data}` but carries NO
 * `isAxiosError` flag. So `axios.isAxiosError(error)` is ALWAYS false at the many
 * downstream callers of `sanitizeError`/`getGenericErrorMessage`. The status-based
 * message must still be derived from the preserved `.response.status`.
 */
function sanitizedHttpError(
  status: number,
  data: Record<string, unknown> = {},
): Error {
  return Object.assign(new Error("Request failed"), {
    status,
    response: { status, data },
  });
}

describe("getGenericErrorMessage — sanitized (downstream) error shape", () => {
  it("maps a sanitized 404 to the not-found message (not the generic default)", () => {
    expect(getGenericErrorMessage(sanitizedHttpError(404))).toBe(
      "The requested resource was not found.",
    );
  });

  it("maps a sanitized 500 to the server-error message", () => {
    expect(getGenericErrorMessage(sanitizedHttpError(500))).toBe(
      "Something went wrong on our end. Please try again later.",
    );
  });

  it("maps a sanitized 409 to the conflict message", () => {
    expect(getGenericErrorMessage(sanitizedHttpError(409))).toBe(
      "This action conflicts with existing data. Please refresh and try again.",
    );
  });

  it("falls back to the default message for a non-API error (plain TypeError)", () => {
    expect(getGenericErrorMessage(new TypeError("boom"))).toBe(
      "An unexpected error occurred. Please try again.",
    );
  });

  it("sanitizeError surfaces the status-specific message for a codeless sanitized 404", () => {
    const { message, status } = sanitizeError(sanitizedHttpError(404));
    expect(status).toBe(404);
    expect(message).toBe("The requested resource was not found.");
  });

  it("maps an admin suspension via the apiErrors catalog", () => {
    const { message, code } = sanitizeError(
      sanitizedHttpError(403, { code: "business_suspended" }),
    );
    expect(code).toBe("business_suspended");
    expect(message).toContain("suspended by the server administrator");
  });
});
