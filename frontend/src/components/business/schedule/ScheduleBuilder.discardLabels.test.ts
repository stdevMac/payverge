import fs from "fs";
import path from "path";

import enSchedule from "@/i18n/messages/en/dashboardSchedule.json";
import esSchedule from "@/i18n/messages/es/dashboardSchedule.json";

/**
 * Session G added `editor.discardConfirm*` to dashboardSchedule, but the keys
 * live in a file Session K owns (ScheduleBuilder), so nothing ever passed them
 * down. ShiftEditorModal fell back to English literals in every locale.
 *
 * Same shape as the DetailDrawer fix: the labels are required, so a missing
 * translation is a compile error rather than silent English.
 */
describe("ScheduleBuilder wires the discard-confirm copy [G/K seam]", () => {
  const builder = fs.readFileSync(
    path.join(__dirname, "ScheduleBuilder.tsx"),
    "utf8",
  );
  const modal = fs.readFileSync(
    path.join(__dirname, "ShiftEditorModal.tsx"),
    "utf8",
  );

  const DISCARD_KEYS = [
    "discardConfirmTitle",
    "discardConfirmDescription",
    "discardConfirm",
    "discardKeep",
  ] as const;

  it.each(DISCARD_KEYS)("passes editor.%s down from ScheduleBuilder", (key) => {
    expect(builder).toMatch(
      new RegExp(`${key}:\\s*t\\("editor\\.${key}"\\)`),
    );
  });

  it.each(DISCARD_KEYS)("declares %s as required on the label type", (key) => {
    // `key?: string` means a caller can silently omit it.
    expect(modal).not.toMatch(new RegExp(`\\n\\s*${key}\\?:\\s*string;`));
    expect(modal).toMatch(new RegExp(`\\n\\s*${key}:\\s*string;`));
  });

  it("ships no English discard fallbacks in the modal", () => {
    // Scan code only — prose in a comment explaining the rule is not a violation.
    const code = modal
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .replace(/^\s*\/\/.*$/gm, "");
    expect(code).not.toMatch(/Discard changes\?/);
    expect(code).not.toMatch(/You have unsaved shift edits/);
    expect(code).not.toMatch(/labels\.discard\w*\s*\?\?/);
  });

  // Same defect class as the discard labels: optional label + English `||`
  // fallback, so a missing key renders English instead of failing loudly.
  it.each(["breakMinutesInvalid", "endEqualsStart"])(
    "declares %s as required too",
    (key) => {
      expect(modal).not.toMatch(new RegExp(`\\n\\s*${key}\\?:\\s*string;`));
      expect(modal).toMatch(new RegExp(`\\n\\s*${key}:\\s*string;`));
    },
  );

  it("ships no English `||` label fallbacks anywhere in the modal", () => {
    const code = modal
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .replace(/^\s*\/\/.*$/gm, "");
    expect(code).not.toMatch(/labels\.\w+\s*\|\|\s*\n?\s*"/);
  });

  it("ships no dead `|| \"English\"` fallback on translated editor labels", () => {
    // getTranslation never returns falsy — it returns the sentence-cased key
    // leaf — so `t(x) || "literal"` is unreachable code that hides a missing key.
    expect(builder).not.toMatch(/t\("editor\.\w+"\)\s*\|\|/);
  });

  it.each(DISCARD_KEYS)("has real copy for editor.%s in en and es", (key) => {
    const editorOf = (bundle: unknown) =>
      (bundle as { editor?: Record<string, string> }).editor ?? {};
    const en = editorOf(enSchedule)[key];
    const es = editorOf(esSchedule)[key];
    expect(typeof en).toBe("string");
    expect(typeof es).toBe("string");
    expect(en).not.toBe("");
    expect(es).not.toBe("");
    // A real translation, not an English copy pasted into the es bundle.
    expect(es).not.toBe(en);
  });
});
