/**
 * The dashboard button system. Six roles, nothing else — every operator-tab
 * button uses one of these recipes so the whole dashboard reads as one hand.
 *
 * Plain <button>/<a>: use the constant directly as className.
 * NextUI <Button> (needed for isLoading/isDisabled/onPress): pair the
 * `*NextUI` variant with radius="full" — NextUI owns sizing/padding, the
 * class owns color and weight.
 */

/** Primary CTA — one per view, brand-filled pill. */
export const btnPrimary =
  "inline-flex min-h-11 items-center gap-2 rounded-full bg-brand px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:cursor-not-allowed disabled:opacity-50 [@media(hover:hover)_and_(pointer:fine)]:min-h-0";

/** Primary CTA inside cards/rows (Plugin “Enable”, gallery buttons). */
export const btnPrimaryCompact =
  "inline-flex min-h-11 items-center gap-1.5 rounded-full bg-brand px-3.5 py-1.5 text-xs font-medium text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:cursor-not-allowed disabled:opacity-50 [@media(hover:hover)_and_(pointer:fine)]:min-h-0";

/** Secondary — bordered pill for the non-primary header/inline actions. */
export const btnSecondary =
  "inline-flex min-h-11 items-center gap-2 rounded-full border border-warm-300 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-warm-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand [@media(hover:hover)_and_(pointer:fine)]:min-h-0";

/** Ghost icon button — refresh, gear, view toggles. Icon h-4 w-4 inside. */
export const btnGhostIcon =
  "inline-flex h-11 w-11 min-w-11 items-center justify-center rounded-full text-ink-400 transition-colors hover:bg-warm-100 hover:text-ink-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand [@media(hover:hover)_and_(pointer:fine)]:h-9 [@media(hover:hover)_and_(pointer:fine)]:w-9 [@media(hover:hover)_and_(pointer:fine)]:min-w-9";

/**
 * Touch-target grow for NextUI `isIconOnly size="sm"` row/toolbar actions.
 * NextUI sizes those at 32px, below the 44px touch minimum. This forces a 44px
 * hit area on coarse pointers (phones/tablets) and shrinks back to the compact
 * 32px on hover-capable devices so the desktop density is preserved.
 */
export const touchIconBtn =
  "h-11 w-11 min-w-11 [@media(hover:hover)_and_(pointer:fine)]:h-8 [@media(hover:hover)_and_(pointer:fine)]:w-8 [@media(hover:hover)_and_(pointer:fine)]:min-w-8";

/** Active/selected state of btnGhostIcon (view toggles) — keep in sync with the idle recipe above. */
export const btnGhostIconActive =
  "inline-flex h-11 w-11 min-w-11 items-center justify-center rounded-full bg-brand/10 text-brand transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand [@media(hover:hover)_and_(pointer:fine)]:h-9 [@media(hover:hover)_and_(pointer:fine)]:w-9 [@media(hover:hover)_and_(pointer:fine)]:min-w-9";

/** Tonal inline actions (approve/deny rows). */
export const btnTonalSuccess =
  "inline-flex min-h-11 items-center gap-1.5 rounded-full bg-emerald-50 px-3 py-1.5 text-xs font-medium text-emerald-800 transition-colors hover:bg-emerald-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50 disabled:cursor-not-allowed disabled:opacity-50 [@media(hover:hover)_and_(pointer:fine)]:min-h-0";
export const btnTonalDanger =
  "inline-flex min-h-11 items-center gap-1.5 rounded-full bg-rose-50 px-3 py-1.5 text-xs font-medium text-rose-800 transition-colors hover:bg-rose-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-400/50 disabled:cursor-not-allowed disabled:opacity-50 [@media(hover:hover)_and_(pointer:fine)]:min-h-0";

/** Quiet destructive sibling — pairs with btnTonalSuccess in approve/deny rows
 * so the approving path reads as the primary action. */
export const btnQuietDanger =
  "inline-flex min-h-11 items-center gap-1.5 rounded-full border border-warm-200 bg-white px-3 py-1.5 text-xs font-medium text-ink-500 transition-colors hover:border-rose-200 hover:bg-rose-50 hover:text-rose-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-400/50 disabled:cursor-not-allowed disabled:opacity-50 [@media(hover:hover)_and_(pointer:fine)]:min-h-0";

/** Destructive text link. */
export const btnDangerLink =
  "rounded-md text-sm font-medium text-rose-600 transition-colors hover:text-rose-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-400/50";

/** NextUI pairings — always combine with radius="full". */
export const btnPrimaryNextUI =
  "bg-brand font-medium text-white hover:bg-brand-dark";
/** Combine with variant="bordered" radius="full". */
export const btnSecondaryNextUI =
  "border-warm-300 bg-white font-medium text-ink-700 hover:bg-warm-100";
