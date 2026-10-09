import { runDirectorRename } from "../directorRename";
import enApiErrors from "@/i18n/locales/en/apiErrors.json";

/**
 * L4-14: drives the SHIPPED runDirectorRename path (used by
 * DirectorConsoleDashboard.handleRenameThread).
 */
const backendErr = (status: number, data: { error?: string; code?: string }) =>
  Object.assign(new Error("request failed"), {
    status,
    response: { status, data },
  });

describe("runDirectorRename (L4-14 shipped path)", () => {
  it("clears a prior banner, then surfaces backend detail on failure", async () => {
    const setError = jest.fn();
    const rename = jest
      .fn()
      .mockRejectedValue(
        backendErr(400, {
          error: "title too long",
          code: "VALIDATION_INVALID_INPUT",
        }),
      );

    // Simulate a stale banner still showing from the previous attempt.
    setError.mockClear();
    await runDirectorRename({
      rename,
      locale: "en",
      fallback: "Couldn't rename this thread. Please try again.",
      setError,
    });

    // First call must clear the banner before the request.
    expect(setError.mock.calls[0]).toEqual([null]);
    // Final call surfaces catalog/backend detail — not only the fallback.
    const last = setError.mock.calls[setError.mock.calls.length - 1][0];
    expect(last).toBe(enApiErrors.VALIDATION_INVALID_INPUT);
    expect(last).not.toMatch(/Couldn't rename/);
  });

  it("clears the banner on success after a prior failure state", async () => {
    const setError = jest.fn();
    await runDirectorRename({
      rename: jest.fn().mockResolvedValue(undefined),
      locale: "en",
      fallback: "Couldn't rename this thread. Please try again.",
      setError,
    });
    // clear-before + clear-after-success
    expect(setError.mock.calls).toEqual([[null], [null]]);
  });

  it("uses fallback only when the body is unusable", async () => {
    const setError = jest.fn();
    await runDirectorRename({
      rename: jest.fn().mockRejectedValue(new TypeError("boom")),
      locale: "en",
      fallback: "Couldn't rename this thread. Please try again.",
      setError,
    });
    expect(setError.mock.calls[0]).toEqual([null]);
    expect(setError.mock.calls[1][0]).toBe(
      "Couldn't rename this thread. Please try again.",
    );
  });
});
