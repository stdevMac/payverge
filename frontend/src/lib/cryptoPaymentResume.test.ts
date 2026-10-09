/** @jest-environment jsdom */
import {
  readCryptoInflight,
  writeCryptoInflight,
  clearCryptoInflight,
  CRYPTO_INFLIGHT_KEY,
  type CryptoInflight,
} from "./cryptoPaymentResume";

const REC: CryptoInflight = {
  billToken: "B-77",
  txHash: "0xhash",
  quoteToken: "tok.x",
  confirmed: false,
};

describe("cryptoPaymentResume", () => {
  beforeEach(() => sessionStorage.clear());

  it("round-trips a record for the matching bill", () => {
    writeCryptoInflight(REC);
    // write stamps a savedAt for TTL bookkeeping; the rest round-trips verbatim.
    expect(readCryptoInflight("B-77")).toEqual(
      expect.objectContaining(REC),
    );
  });

  it("returns null when the stored bill does not match the requested bill", () => {
    writeCryptoInflight(REC);
    expect(readCryptoInflight("B-99")).toBeNull();
  });

  it("returns null and does not throw on corrupt JSON", () => {
    sessionStorage.setItem(CRYPTO_INFLIGHT_KEY, "{not json");
    expect(readCryptoInflight("B-77")).toBeNull();
  });

  it("returns null when a required field is missing or wrong-typed", () => {
    sessionStorage.setItem(
      CRYPTO_INFLIGHT_KEY,
      JSON.stringify({ billToken: "B-77", txHash: 123, quoteToken: "t", confirmed: false }),
    );
    expect(readCryptoInflight("B-77")).toBeNull();
  });

  it("clear removes the key", () => {
    writeCryptoInflight(REC);
    clearCryptoInflight();
    expect(sessionStorage.getItem(CRYPTO_INFLIGHT_KEY)).toBeNull();
  });

  it("round-trips an optional cross-chain lifiRouteId", () => {
    const xc: CryptoInflight = { ...REC, lifiRouteId: "route-1" };
    writeCryptoInflight(xc);
    const read = readCryptoInflight("B-77");
    expect(read).toEqual(expect.objectContaining(xc));
    expect(read?.lifiRouteId).toBe("route-1");
  });

  it("accepts a record without lifiRouteId (direct-USDC flow)", () => {
    writeCryptoInflight(REC);
    expect(readCryptoInflight("B-77")?.lifiRouteId).toBeUndefined();
  });

  it("treats a record older than the 30-minute quote TTL as absent and clears it", () => {
    const stale = {
      ...REC,
      savedAt: Date.now() - (31 * 60 * 1000), // 31 minutes ago
    };
    sessionStorage.setItem(CRYPTO_INFLIGHT_KEY, JSON.stringify(stale));
    expect(readCryptoInflight("B-77")).toBeNull();
    // It also evicts the expired record so a stale resume cannot be retried.
    expect(sessionStorage.getItem(CRYPTO_INFLIGHT_KEY)).toBeNull();
  });

  it("returns a fresh record that is within the TTL", () => {
    const fresh = { ...REC, savedAt: Date.now() - (5 * 60 * 1000) }; // 5 min ago
    sessionStorage.setItem(CRYPTO_INFLIGHT_KEY, JSON.stringify(fresh));
    const read = readCryptoInflight("B-77");
    expect(read?.billToken).toBe("B-77");
    expect(read?.txHash).toBe("0xhash");
  });

  it("stamps savedAt on write so a TTL check is possible", () => {
    const before = Date.now();
    writeCryptoInflight(REC);
    const stored = JSON.parse(
      sessionStorage.getItem(CRYPTO_INFLIGHT_KEY) as string,
    );
    expect(typeof stored.savedAt).toBe("number");
    expect(stored.savedAt).toBeGreaterThanOrEqual(before);
  });

  it("evicts a stored record without savedAt (cannot prove freshness)", () => {
    sessionStorage.setItem(CRYPTO_INFLIGHT_KEY, JSON.stringify(REC));
    expect(readCryptoInflight("B-77")).toBeNull();
    expect(sessionStorage.getItem(CRYPTO_INFLIGHT_KEY)).toBeNull();
  });

  it("rejects a record keyed by billNumber instead of billToken", () => {
    sessionStorage.setItem(
      CRYPTO_INFLIGHT_KEY,
      JSON.stringify({
        billNumber: "B-77",
        txHash: "0xhash",
        quoteToken: "tok.x",
        confirmed: true,
        savedAt: Date.now(),
      }),
    );
    expect(readCryptoInflight("B-77")).toBeNull();
  });

  it("round-trips the cross-chain source identity (token symbol + chain name + id)", () => {
    // The backend's idempotency match compares SourceToken/SourceChain EXACTLY,
    // so a remount-resume must replay the persisted source identity verbatim.
    const xc: CryptoInflight = {
      ...REC,
      lifiRouteId: "route-1",
      sourceToken: "WETH",
      sourceChain: "Polygon",
      sourceChainId: 137,
    };
    writeCryptoInflight(xc);
    const read = readCryptoInflight("B-77");
    expect(read).toEqual(expect.objectContaining(xc));
    expect(read?.sourceToken).toBe("WETH");
    expect(read?.sourceChain).toBe("Polygon");
    expect(read?.sourceChainId).toBe(137);
  });
});
