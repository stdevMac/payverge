import fs from "fs";
import path from "path";

/**
 * Issue 822 — the guest menu must not collapse every blocked reason to
 * "Out of Stock"/"unavailable". Closed hours say closed, ordering-off says
 * ordering off, 86 says 86. Source-level wiring assertions in the style of
 * page.closedBanner.test.ts (the page component is too heavy to mount).
 */
describe("guest menu blocked-reason copy honesty (issue 822)", () => {
  const source = fs.readFileSync(path.resolve(__dirname, "page.tsx"), "utf8");
  const cartModalSource = fs.readFileSync(
    path.resolve(__dirname, "_components/CartModal.tsx"),
    "utf8",
  );

  it("derives an honest per-reason label via blockedReasonKind", () => {
    expect(source).toContain("blockedReasonKind");
  });

  it("bundle rows no longer hardcode Out of Stock for any blocked child", () => {
    // The old shape: {bundleBlocked && (<p ...>{t("menu.outOfStock")}</p>)}
    expect(source).not.toMatch(
      /bundleBlocked\s*&&\s*\([\s\S]{0,200}?t\("menu\.outOfStock"\)/,
    );
    // Closed/ordering-off bundles say so instead.
    expect(source).toMatch(
      /bundleBlockedReason[\s\S]{0,400}?menu\.businessClosed/,
    );
    expect(source).toMatch(
      /bundleBlockedReason[\s\S]{0,400}?menu\.orderingDisabled/,
    );
  });

  it("add-to-cart refusals name the real reason when the venue is closed or ordering is off", () => {
    // addToCart's orderability refusal and addBundleToCart's blocked-child
    // refusal both map business_closed/ordering_disabled honestly.
    const closedToasts =
      source.match(/decision[\s\S]{0,300}?menu\.businessClosed/g) || [];
    expect(closedToasts.length).toBeGreaterThan(0);
    expect(source).toMatch(
      /blockedChild[\s\S]{0,400}?menu\.businessClosed/,
    );
  });

  it("quote-blocked surfaces use the reason-mapped message, not always the generic unavailable copy", () => {
    expect(source).toContain("quoteBlockedMessage");
    // The FAB alert and GuestQuoteStatus must not hardcode the generic key
    // as the only blocked copy.
    expect(source).not.toMatch(
      /quoteBlockedLines\.length > 0\s*\n?\s*\? t\("menu\.orderErrorItemUnavailableGeneric"\)/,
    );
    expect(source).toMatch(/blockedLabel=\{quoteBlockedMessage\}/);
  });

  it("quoteBlockedMessage lets a sticky 86 outrank after-hours closed copy (GM3)", () => {
    // After hours the backend remaps every line to business_closed, so the
    // memo must also feed the catalog's sticky inventoryStatus into
    // blockedReasonKind (out_of_stock outranks business_closed) — otherwise
    // the cart line says "closed" while the tile says "Out of Stock".
    const memoMatch = source.match(
      /const quoteBlockedMessage = useMemo[\s\S]{0,1200}?\}, \[[^\]]*\]\);/,
    );
    expect(memoMatch).not.toBeNull();
    const memo = (memoMatch as RegExpMatchArray)[0];
    expect(memo).toContain("inventoryStatus");
    expect(memo).toContain("menuItemLookup");
    expect(memo).toMatch(/out_of_stock[\s\S]{0,120}?menu\.outOfStock/);
  });

  it("CartModal renders the reason-mapped blocked message", () => {
    expect(source).toMatch(/quoteBlockedMessage=\{quoteBlockedMessage\}/);
    expect(cartModalSource).toContain("quoteBlockedMessage");
    expect(cartModalSource).not.toMatch(
      /quoteBlocked\s*\n?\s*\? t\("menu\.orderErrorItemUnavailableGeneric"\)/,
    );
  });
});
