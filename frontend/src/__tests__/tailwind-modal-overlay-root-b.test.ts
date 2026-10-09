import fs from "fs";
import path from "path";

/**
 * Root B (L1-12 / L3-13): NextUI ModalContent mounts a fixed inset-0
 * `data-slot="wrapper"` for the full enter/exit animation. That layer has no
 * `pointer-events-none`, so it swallows clicks while the panel is at
 * opacity:0 → 1 (scaleInOut). The NextUI theme plugin cannot override modal
 * slots via `themes`, so we enforce the contract with a Tailwind `addBase`
 * plugin immediately after `nextui(...)` in tailwind.config.ts.
 *
 * Root B regression (pre-merge review): the wrapper shield is what keeps
 * `isDismissable={false}` modals non-dismissable. @nextui-org/modal 2.2.7's
 * getBackdropProps has an UNCONDITIONAL `onClick: () => state.close()` on the
 * backdrop (a sibling of the wrapper); it was only unreachable because the
 * wrapper covered it. A blanket `pointer-events: none` on every wrapper let
 * outside-panel clicks fall through to the backdrop and close non-dismissable
 * modals mid-mutation (ManagerPinModal, RenewalModal, FiscalDashboard, …).
 *
 * The rule must therefore be scoped with `:has()` to wrappers whose direct
 * dialog child is stamped `data-dismissable="true"` — getDialogProps emits
 * that attribute via `dataAttr(isDismissable)`, which is "true" when
 * dismissable and ABSENT when `isDismissable={false}`.
 */
describe("Root B — NextUI modal wrapper overlay fix", () => {
  const configSrc = fs.readFileSync(
    path.join(__dirname, "..", "..", "tailwind.config.ts"),
    "utf8",
  );

  it("scopes pointer-events-none to DISMISSABLE modal wrappers only", () => {
    // Active source must declare the scoped wrapper rule (not only a comment):
    // [data-slot="wrapper"]:has(> [aria-modal="true"][data-dismissable="true"])
    expect(configSrc).toMatch(
      /\[data-slot=["']wrapper["']\]:has\(>\s*\[aria-modal=["']true["']\]\[data-dismissable=["']true["']\]\)['"]?\s*:\s*\{[\s\S]{0,120}pointerEvents\s*:\s*["']none["']/,
    );
  });

  it("does NOT ship an unscoped wrapper pointer-events-none (would let backdrop clicks close isDismissable={false} modals)", () => {
    // A bare `'[data-slot="wrapper"]': { pointerEvents: "none" }` (no :has
    // scope) regresses the 11 non-dismissable money-mutation modals.
    expect(configSrc).not.toMatch(
      /['"]\[data-slot=["']wrapper["']\]['"]\s*:\s*\{[^}]*pointerEvents\s*:\s*["']none["']/,
    );
  });

  it("re-enables pointer-events on dismissable-wrapper children (dialog panel)", () => {
    expect(configSrc).toMatch(
      /\[data-slot=["']wrapper["']\]:has\(>\s*\[aria-modal=["']true["']\]\[data-dismissable=["']true["']\]\)\s*>\s*\*[\s\S]{0,120}pointerEvents\s*:\s*["']auto["']/,
    );
  });

  it("forces opaque wrapper for ALL modal wrappers (kills scaleInOut opacity ghosting)", () => {
    // Framer-motion sets inline opacity; !important is required. Scoped to
    // modal wrappers via the always-stamped aria-modal dialog child so it can
    // never hit NextUI Pagination (the only other data-slot="wrapper").
    expect(configSrc).toMatch(
      /\[data-slot=["']wrapper["']\]:has\(>\s*\[aria-modal=["']true["']\]\)['"]?\s*:\s*\{[\s\S]{0,120}opacity\s*:\s*["']1\s*!important["']/,
    );
  });
});
