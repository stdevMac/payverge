/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../BusinessSettings.tsx"),
  "utf-8",
);

describe("BusinessSettings — no stale business-page snapshot in saves (P2-23)", () => {
  test("the mount-time preservedSettings snapshot is gone", () => {
    expect(SOURCE).not.toContain("preservedSettings");
  });

  test("the save payload no longer carries business-page fields", () => {
    // Stream 9: persistProfile builds section-aware payloads via a ternary.
    const persist = SOURCE.match(
      /const updateData: UpdateBusinessRequest =\s*section === "payments"[\s\S]{0,1600}?if \(section === "payments"/,
    );
    expect(persist).not.toBeNull();
    expect(persist![0]).not.toContain("business_page_enabled");
    expect(persist![0]).not.toContain("welcome_message");
    expect(persist![0]).not.toContain("banner_images");
    expect(persist![0]).not.toContain("show_gallery");
  });
});
