import fs from "fs";
import path from "path";

const GUEST_DIR = path.join(__dirname, "..", "guest-messages");

const REQUIRED = [
  "businessPage.info.reservationForm.invalidEmail",
  "businessPage.info.reservationForm.invalidPhone",
  "businessPage.info.reservationForm.fillAllFields",
  "businessPage.info.reservationForm.summaryStatusChooseSlot",
  "businessPage.info.reservationForm.waitlistDoNotArrive",
  "businessPage.info.reservationForm.waitlistWaitForContact",
  "businessPage.info.reservationForm.loadingAvailability",
  "businessPage.info.reservationForm.modal.importantWaitlistDesc",
  "reservationConfirmation.action.alreadyCancelledBody",
  "reservationConfirmation.action.notFoundBody",
  "reservationConfirmation.action.windowClosedBody",
  "reservationConfirmation.action.windowNeverOpenBody",
  "reservationConfirmation.onlineCancelNeverOpen",
  "reservationConfirmation.onlineCancelWindowClosed",
  "reservationConfirmation.action.notAllowedBody",
  "reservationConfirmation.action.noLongerCancellableBody",
  "reservationConfirmation.notFoundFallback",
] as const;

function getNested(obj: unknown, dotted: string): unknown {
  return dotted.split(".").reduce<unknown>((acc, key) => {
    if (acc && typeof acc === "object" && key in (acc as object)) {
      return (acc as Record<string, unknown>)[key];
    }
    return undefined;
  }, obj);
}

describe("reservation validation + cancel keys (Batch B)", () => {
  const locales = fs
    .readdirSync(GUEST_DIR)
    .filter((f) => f.endsWith(".json") && !f.startsWith("."));

  it("ships 21 guest locale files", () => {
    expect(locales.length).toBe(21);
  });

  it.each(locales)("%s has required reservation keys", (file) => {
    const data = JSON.parse(
      fs.readFileSync(path.join(GUEST_DIR, file), "utf8"),
    );
    for (const key of REQUIRED) {
      const value = getNested(data, key);
      expect(typeof value).toBe("string");
      expect((value as string).trim().length).toBeGreaterThan(0);
    }
  });
});
