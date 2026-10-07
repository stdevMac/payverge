import fs from "fs";
import path from "path";

describe("guest menu submit flow (G-1/G-2/G-3)", () => {
  const source = fs.readFileSync(path.resolve(__dirname, "page.tsx"), "utf8");

  it("uses a single cart-signature idempotency ref shared by both submit paths", () => {
    expect(source).toContain("const orderSubmissionRef = useRef");
    expect(source).not.toContain("createBillSubmissionRef");
    expect(source).not.toContain("addItemsSubmissionRef");
    expect(source).toContain("buildCartSignature(tableCode, items)");
  });

  it("clears the shared idempotency key only after success or a terminal key conflict", () => {
    const clears =
      source.match(/clearGuestOrderIdempotencyKey\(orderSubmissionRef\)/g) ||
      [];
    expect(clears).toHaveLength(2);
    expect(source).toMatch(
      /await createGuestOrder\([\s\S]{0,2500}?\{ idempotencyKey \},?\s*\);[\s\S]{0,800}?clearGuestOrderIdempotencyKey\(orderSubmissionRef\)/,
    );
  });

  it("creates the initial bill and order with one atomic checkout mutation", () => {
    expect(source).toContain('submitCartAsOrder(null, "create-bill")');
    expect(source).not.toContain("createBillByTableCode(");
  });

  it("maps backend error codes instead of claiming the order was placed (G-1)", () => {
    expect(source).toContain("handleGuestOrderError(");
    expect(source).not.toContain('toast.error(t("menu.orderApprovalWarning"))');
    expect(source).not.toContain(
      'toast.error(t("menu.additionalApprovalWarning"))',
    );
    expect(source).not.toContain("menu.orderRetryAfterBillCreated");
  });

  it("removes only authoritative blocked IDs, including bundle children", () => {
    expect(source).toContain("errorInfo.blockedItems");
    expect(source).toContain("cartItemContainsBlockedMenuItem(");
    expect(source).toContain("parseBundleItems(bundle).map");
    expect(source).toContain(
      "prev.filter((item) => !isUnavailableCartItem(item))",
    );
    expect(source).not.toContain("unavailableIndexes");
  });

  it("refreshes table state silently after Place Order so cart/success stay mounted (issue 76)", () => {
    expect(source).toContain("loadTableData({ silent: true })");
    expect(source).toMatch(
      /async \(opts\?: \{ silent\?: boolean \}\) => \{/,
    );
    expect(source).toMatch(
      /Full-page skeleton only on the first paint[\s\S]*issue 76/,
    );
    const silentRefreshes = source.match(/loadTableData\(\{ silent: true \}\)/g) || [];
    expect(silentRefreshes.length).toBeGreaterThanOrEqual(3);
  });

  it("defers cart persistence so a Place Order epoch bump can cancel stale writes (issue 76)", () => {
    expect(source).toContain("const epochAtSchedule = cartEpochRef.current");
    expect(source).toContain("const snapshot = cart");
    expect(source).toContain("window.setTimeout(persist, 0)");
    expect(source).toContain("window.clearTimeout(handle)");
  });

  it("keeps OrderSuccessModal mounted on the loading skeleton (issue 76)", () => {
    const loadingReturn = source.slice(
      source.indexOf("if (shouldShowMenuLoadingGate("),
      source.indexOf("if (!tableData) {"),
    );
    expect(loadingReturn).toContain("OrderSuccessModal");
    expect(loadingReturn).toContain("isOpen={showOrderSuccess}");
  });

  it("retires a cross-table idempotency key so the next retry can proceed", () => {
    expect(source).toContain('errorInfo.code === "idempotency_conflict"');
    expect(source).toMatch(
      /errorInfo\.code === "idempotency_conflict"[\s\S]{0,240}clearGuestOrderIdempotencyKey\(orderSubmissionRef\)/,
    );
  });
});
