import fs from "fs";
import path from "path";

/**
 * L3-3 residual: sanitization review inside AI onboarding must block parent
 * modal Esc and render as a portal (not only under the modal body).
 */
describe("L3-3 AI menu sanitization Esc seam", () => {
  const root = path.join(__dirname, "..");

  it("AIMenuOnboardingModal disables keyboard dismiss while review blocks", () => {
    const src = fs.readFileSync(
      path.join(root, "components/AIMenuOnboardingModal.tsx"),
      "utf8",
    );
    expect(src).toMatch(/isKeyboardDismissDisabled=\{!allowDismiss\}/);
    expect(src).toMatch(/isDismissable=\{allowDismiss\}/);
    expect(src).toMatch(/onBlockingOverlayChange/);
  });

  it("MenuReviewEditor portals the review and notifies parent", () => {
    const src = fs.readFileSync(
      path.join(root, "AIMenuOnboarding/MenuReviewEditor.tsx"),
      "utf8",
    );
    expect(src).toMatch(/createPortal/);
    expect(src).toMatch(/onSanitizationBlockingChange/);
    expect(src).toMatch(/menu-sanitization-portal/);
  });
});
