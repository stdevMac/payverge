// Z-index scale for the business-page surface.
// Each layer is 10 units apart. Stacking-context boundaries:
//   - Dropdowns / toggles: 20
//   - Floating pills (language, etc.): 30
//   - Floating action buttons: 40
//   - Cart drawer / side panel: 50
//   - Modals: 60
//   - Toasts: 70
// Anything above 70 should be rare; if you need it, extend this scale rather
// than reaching for arbitrary four-digit z-index literals.

export const Z_DROPDOWN = 20;
export const Z_FLOATING_PILL = 30;
export const Z_FAB = 40;
export const Z_CART_PANEL = 50;
export const Z_MODAL = 60;
export const Z_TOAST = 70;

// Helper for inline-style consumption when Tailwind utility doesn't suffice.
export const zStyle = (z: number): { zIndex: number } => ({ zIndex: z });
