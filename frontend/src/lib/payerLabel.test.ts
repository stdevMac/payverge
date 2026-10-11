import { isWalletAddress, payerMethodKey } from "./payerLabel";

// Canonical EVM address: 0x + exactly 40 hex chars.
const WALLET_40 = "0x000000000000000000000000000000000000dE01";

describe("isWalletAddress", () => {
  it("accepts a canonical 0x + 40-hex wallet address", () => {
    expect(WALLET_40.length).toBe(42);
    expect(isWalletAddress(WALLET_40)).toBe(true);
  });

  it("accepts an over-long all-hex 0x string (intentional 40+ laxity)", () => {
    // 41 hex chars — not canonical, but all-hex and long enough that it can
    // only be a wallet-ish value, never a sentinel.
    expect(isWalletAddress(`${WALLET_40}f`)).toBe(true);
  });

  it("rejects sentinels and non-wallet strings", () => {
    expect(isWalletAddress("0xguest")).toBe(false);
    expect(isWalletAddress("crypto_guest")).toBe(false);
    expect(isWalletAddress("staff_manual")).toBe(false);
    expect(isWalletAddress("plugin")).toBe(false);
    expect(isWalletAddress("")).toBe(false);
    expect(isWalletAddress(null)).toBe(false);
    expect(isWalletAddress(undefined)).toBe(false);
  });

  it("rejects a 0x string shorter than 40 hex chars", () => {
    expect(isWalletAddress("0x1234567890abcdef")).toBe(false);
  });
});

describe("payerMethodKey", () => {
  // Regression (audit L5 follow-up): staff-manual payments carry a synthetic
  // `manual_*` tx hash from the backend (newManualPaymentTxHash) — a cash or
  // card payment entered by staff must NOT be labeled crypto.
  it("is manual for staff_manual payer with a synthetic manual tx hash", () => {
    expect(
      payerMethodKey({
        payer_address: "staff_manual",
        tx_hash: "manual_1_abc",
      }),
    ).toBe("manual");
  });

  it("is manual for the staff_manual sentinel even without a tx hash", () => {
    expect(payerMethodKey({ payer_address: "staff_manual" })).toBe("manual");
  });

  it("is manual for a manual_* tx hash regardless of payer value", () => {
    expect(
      payerMethodKey({ payer_address: "0xguest", tx_hash: "manual_9_def" }),
    ).toBe("manual");
  });

  // Regression (audit L5 round 2): plugin settlements (Stripe/PayPal/
  // MercadoPago) write payer "plugin" with synthetic tx "plugin_<paymentID>"
  // — they must NOT be labeled crypto. Realistic row shape: real plugin rows
  // always carry the synthetic hash.
  it("is plugin for plugin payer with a synthetic plugin tx hash", () => {
    expect(
      payerMethodKey({ payer_address: "plugin", tx_hash: "plugin_123" }),
    ).toBe("plugin");
  });

  it("is plugin for a plugin_* tx hash regardless of payer value", () => {
    expect(
      payerMethodKey({ payer_address: "0xguest", tx_hash: "plugin_abc" }),
    ).toBe("plugin");
  });

  // Split-share settlements stamp a synthetic `split_<sha…>` hash whose real
  // tender (cash/card/venmo/plugin) the history endpoint can't see — never
  // crypto.
  it("is guest for a split_* synthetic tx hash", () => {
    expect(
      payerMethodKey({
        payer_address: "split_guest",
        tx_hash: "split_0123abc",
      }),
    ).toBe("guest");
  });

  it("is crypto when a real (non-synthetic) tx hash is present", () => {
    expect(payerMethodKey({ payer_address: "0xguest", tx_hash: "0xabc" })).toBe(
      "crypto",
    );
  });

  it("is crypto for a real wallet address with no tx hash", () => {
    expect(payerMethodKey({ payer_address: WALLET_40 })).toBe("crypto");
  });

  it("is crypto for crypto/cross-chain guest sentinels (exact match)", () => {
    expect(payerMethodKey({ payer_address: "crypto_guest" })).toBe("crypto");
    expect(payerMethodKey({ payer_address: "cross_chain_guest" })).toBe(
      "crypto",
    );
  });

  it("is guest for non-wallet, non-manual sentinels and empty payers", () => {
    expect(payerMethodKey({ payer_address: "0xguest" })).toBe("guest");
    expect(payerMethodKey({ payer_address: "webhook" })).toBe("guest");
    expect(payerMethodKey({ payer_address: "" })).toBe("guest");
    expect(payerMethodKey({})).toBe("guest");
  });
});
