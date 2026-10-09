import fs from "fs";
import path from "path";

describe("GuestBill progressive disclosure", () => {
  const source = fs.readFileSync(
    path.resolve(__dirname, "GuestBill.tsx"),
    "utf8",
  );

  it("expands kitchen by default, keeps split collapsed, and surfaces kitchen before pay", () => {
    expect(source).toContain('data-testid="guest-bill-split-disclosure"');
    expect(source).toContain('data-testid="guest-bill-kitchen-disclosure"');
    expect(source).toMatch(
      /const \[splitExpanded, setSplitExpanded\] = useState\(false\)/,
    );
    expect(source).toMatch(
      /const \[kitchenExpanded, setKitchenExpanded\] = useState\(true\)/,
    );

    // Kitchen status before PaymentSection so "kitchen got it" is not buried.
    const kitchenIdx = source.indexOf(
      'data-testid="guest-bill-kitchen-disclosure"',
    );
    const payIdx = source.indexOf("<PaymentSection");
    const splitIdx = source.indexOf('data-testid="guest-bill-split-disclosure"');
    expect(kitchenIdx).toBeGreaterThan(-1);
    expect(payIdx).toBeGreaterThan(kitchenIdx);
    expect(splitIdx).toBeGreaterThan(payIdx);
  });

  it("still mounts cancel affordance for pending orders when kitchen expanded", () => {
    expect(source).toContain("handleGuestCancel");
    expect(source).toContain('t("orders.cancel")');
    expect(source).toContain("OrderStatusTimeline");
  });
});
