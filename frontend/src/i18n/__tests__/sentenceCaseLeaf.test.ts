import { sentenceCaseLeaf } from "../sentenceCaseLeaf";

describe("sentenceCaseLeaf", () => {
  it("sentence-cases camelCase leaves", () => {
    expect(sentenceCaseLeaf("error.notFoundEyebrow")).toBe("Not found eyebrow");
    expect(sentenceCaseLeaf("cart.empty")).toBe("Empty");
  });

  it("treats underscores as word separators (alert-type keys)", () => {
    // Regression: the payment_refund_review fallback chip rendered
    // "Payment_refund_review".
    expect(
      sentenceCaseLeaf(
        "businessSettings.notifications.operationalTypes.payment_refund_review",
      ),
    ).toBe("Payment refund review");
    expect(sentenceCaseLeaf("service_call")).toBe("Service call");
  });

  it("treats hyphens as word separators", () => {
    expect(sentenceCaseLeaf("a.b.kebab-case-leaf")).toBe("Kebab case leaf");
  });
});
