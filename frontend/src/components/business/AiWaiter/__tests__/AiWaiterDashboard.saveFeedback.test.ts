import {
  saveSettingsErrorMessage,
  saveSettingsSuccessMessage,
} from "../saveSettingsFeedback";
import enApiErrors from "@/i18n/locales/en/apiErrors.json";

/**
 * L4-4 helper unit coverage (secondary).
 * D1 DOM + toast wiring lives in AiWaiterDashboard.error-paths.test.tsx —
 * pure helpers alone are not sufficient for pass-on-revert.
 */
const backendErr = (status: number, data: { error?: string; code?: string }) =>
  Object.assign(new Error("request failed"), {
    status,
    response: { status, data },
  });

describe("saveSettingsFeedback (L4-4 shipped path)", () => {
  it("returns the success copy for a completed save", () => {
    expect(saveSettingsSuccessMessage("Configuration saved.")).toBe(
      "Configuration saved.",
    );
  });

  it("surfaces a backend-shaped 400 via surfaceBackendError, not bare try-again", () => {
    const err = backendErr(400, {
      code: "VALIDATION_INVALID_INPUT",
      error: "ai_name is required",
    });
    expect(saveSettingsErrorMessage(err, "en", "Couldn't save settings.")).toBe(
      enApiErrors.VALIDATION_INVALID_INPUT,
    );
  });

  it("falls back to the saveFailed copy for non-API throws", () => {
    expect(
      saveSettingsErrorMessage(
        new TypeError("boom"),
        "en",
        "Couldn't save settings. Please try again.",
      ),
    ).toBe("Couldn't save settings. Please try again.");
  });
});
