/** @jest-environment node */
/**
 * Contract: every SaveBar consumer must pass a real form-dirty flag — never a
 * saving/loading spinner flag into the `dirty` slot (Finding 65).
 *
 * The shared primitive disables Save when `dirty === false` (Finding 44); a
 * mis-wired `dirty={isSaving}` would either keep Save permanently disabled or
 * only light the "unsaved" chip while a request is in flight.
 */
import fs from "node:fs";
import path from "node:path";

const businessDir = path.resolve(__dirname, "..");

function read(rel: string): string {
  return fs.readFileSync(path.join(businessDir, rel), "utf-8");
}

describe("SaveBar dirty wiring (Findings 44, 65)", () => {
  it("SaveBar disables the button when clean: isDisabled={!dirty || isSaving}", () => {
    const source = read("SaveBar.tsx");
    expect(source).toMatch(/isDisabled=\{!\s*dirty\s*\|\|\s*isSaving\s*\}/);
  });

  it("button-mode idle uses labels.clean, never labels.auto (#380)", () => {
    const source = read("SaveBar.tsx");
    expect(source).toMatch(/labels\.clean/);
    // The autosave string is reserved for mode === "auto".
    const autoUses = [...source.matchAll(/labels\.auto/g)];
    expect(autoUses).toHaveLength(1);
  });

  it("shared saveBar.clean copy does not claim autosave (#380)", () => {
    const messagesDir = path.resolve(__dirname, "../../../i18n/messages");
    const en = JSON.parse(
      fs.readFileSync(path.join(messagesDir, "en/businessSettings.json"), "utf-8"),
    );
    const es = JSON.parse(
      fs.readFileSync(path.join(messagesDir, "es/businessSettings.json"), "utf-8"),
    );
    expect(en.saveBar.clean).toMatch(/no unsaved changes/i);
    expect(en.saveBar.clean).not.toMatch(/automatic/i);
    expect(es.saveBar.clean).not.toMatch(/automátic/i);
  });

  it("InventoryManager does not pass savingSettings into the dirty slot", () => {
    const source = read("InventoryManager.tsx");
    expect(source).not.toMatch(/dirty=\{\s*savingSettings\s*\}/);
    // Auto-save surface: dirty must be a real form flag (false for pure autosave).
    expect(source).toMatch(/dirty=\{\s*false\s*\}/);
  });

  it("CounterManager passes real dirty (useDirtyForm) and guards navigation", () => {
    const source = read("CounterManager.tsx");
    expect(source).toMatch(/useDirtyForm/);
    expect(source).toMatch(/useUnsavedChangesGuard/);
    expect(source).toMatch(/dirty=\{\s*dirty\s*\}/);
  });

  it("DeliverySettings uses useDirtyForm + useUnsavedChangesGuard", () => {
    const source = read("DeliverySettings.tsx");
    expect(source).toMatch(/useDirtyForm/);
    expect(source).toMatch(/useUnsavedChangesGuard/);
    expect(source).toMatch(/dirty=\{\s*(hasChanges|dirty)\s*\}/);
  });

  it("BusinessSettings passes saveBarDirty derived from real dirty state", () => {
    const source = read("BusinessSettings.tsx");
    expect(source).toMatch(/useDirtyForm/);
    expect(source).toMatch(/dirty=\{\s*saveBarDirty\s*\}/);
    expect(source).toMatch(/useUnsavedChangesGuard/);
    // Notifications auto-save; localization uses its own flag (#224 / #268).
    expect(source).toMatch(/activeTab === "notifications"\s*\?\s*false/);
    expect(source).toMatch(/activeTab === "localization"\s*\?\s*localizationDirty/);
  });

  it("BusinessPageEditor passes isDirty gated on successful load", () => {
    const source = read("BusinessPageEditor.tsx");
    expect(source).toContain("dirty={isDirty && !loadFailed}");
    expect(source).toMatch(/useUnsavedChangesGuard/);
  });
});
