// Payer-label helpers for the operator payment views.
//
// The backend payment-history row exposes a `payer_address` that is a grab-bag:
// sometimes a real on-chain wallet hex (0x + 40 hex chars), sometimes a
// machine sentinel written by the various payment paths (`crypto_guest`,
// `cross_chain_guest`, `staff_manual`, `plugin`, `split_guest`, `0xguest`, …).
// Showing that raw value as the primary payer label is meaningless to
// operators, so we derive a stable, translatable *method key* and only surface
// the hex as a demoted secondary line when it is genuinely a wallet address.

/** Signals available on a payment row for deriving a human payer label. */
export interface PayerSignals {
  payer_address?: string | null;
  /**
   * Transaction hash. Present for settled crypto payments, but the backend
   * ALSO stamps synthetic hashes on non-crypto settlements:
   * `manual_<billID>_<uuid>` (staff manual, newManualPaymentTxHash),
   * `plugin_<paymentID>` (Stripe/PayPal/MercadoPago plugin settlements), and
   * `split_<sha…>` (split-share tenders, splitSyntheticTxHash). A non-empty
   * hash alone does NOT mean on-chain.
   */
  tx_hash?: string | null;
}

// A real EVM wallet address is 0x followed by 40 hex chars. We accept 40+ (all
// hex) so an occasional non-canonical-length hex still reads as a wallet, while
// sentinels like `0xguest` (non-hex) and short test stubs are excluded.
const WALLET_HEX = /^0x[0-9a-fA-F]{40,}$/;

// Synthetic (not-on-chain) tx-hash prefixes stamped by backend settlement
// paths. Their presence must never be read as crypto evidence. `split_`
// hashes cover cash/card/venmo/plugin split tenders whose real method the
// history endpoint can't see.
const SYNTHETIC_TX_PREFIXES = ["manual_", "plugin_", "split_", "demo_tx_"];

// `staff_manual` is the only payer sentinel the staff-manual settlement path
// writes to payments today (MarkBillAsPaidWithPayment). The endpoint can't
// see the underlying cash/card instrument, so it maps to an honest "Manual"
// bucket rather than a guessed one.
const MANUAL_SENTINELS = new Set(["staff_manual"]);

// Crypto guest sentinels written by the guest crypto / cross-chain payment
// paths (backend/internal/handlers/payments.go). Exact matches only.
const CRYPTO_SENTINELS = new Set(["crypto_guest", "cross_chain_guest"]);

/**
 * True when the payer_address is a genuine on-chain wallet hex we can safely
 * show (and link) as a secondary line. Sentinels like `0xguest` are excluded.
 */
export function isWalletAddress(address?: string | null): boolean {
  return !!address && WALLET_HEX.test(address.trim());
}

/**
 * Method key describing how a payment was made, derived from the row's
 * available signals. Callers map this key to a translated label via i18n
 * (namespace `…paymentHistory.payerMethods.<key>`).
 *
 * - `manual` — staff-entered settlement (`staff_manual` payer or a synthetic
 *   `manual_*` tx hash).
 * - `plugin` — plugin (Stripe/PayPal/MercadoPago) settlement: `plugin` payer
 *   or a `plugin_*` tx hash, mirroring the backend's
 *   NormalizeRecognizedPaymentMethod classification.
 * - `crypto` — settled on-chain: a real (non-synthetic) tx hash, a real
 *   wallet address, or a crypto/cross-chain guest sentinel.
 * - `guest`  — everything else (split/other sentinels, or a missing
 *   payer_address).
 */
export function payerMethodKey(
  signals: PayerSignals,
): "crypto" | "manual" | "plugin" | "guest" {
  const addr = (signals.payer_address ?? "").trim().toLowerCase();
  const tx = (signals.tx_hash ?? "").trim().toLowerCase();

  if (MANUAL_SENTINELS.has(addr) || tx.startsWith("manual_")) return "manual";
  if (addr === "plugin" || tx.startsWith("plugin_")) return "plugin";

  const txIsSynthetic = SYNTHETIC_TX_PREFIXES.some((p) => tx.startsWith(p));
  if (tx && !txIsSynthetic) return "crypto";
  if (isWalletAddress(signals.payer_address)) return "crypto";
  if (CRYPTO_SENTINELS.has(addr)) return "crypto";
  return "guest";
}
