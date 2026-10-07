import {
  isOpaqueBillNumber,
  operatorBillDisplayNumber,
  operatorBillSupportId,
  operatorTenderLabel,
  stripDemoBillPrefix,
} from "./operatorBillNumber";

describe("operatorBillNumber", () => {
  it("strips DEMO- prefixes so operator surfaces never show them", () => {
    expect(stripDemoBillPrefix("DEMO-B2-0d60c280-49f")).toBe("B2-0d60c280-49f");
    expect(stripDemoBillPrefix("#DEMO-1001")).toBe("1001");
    expect(stripDemoBillPrefix("demo-abc")).toBe("abc");
  });

  it("prefers non-opaque bill_number and falls back to sequential id", () => {
    expect(operatorBillDisplayNumber({ id: 42, bill_number: "1001" })).toBe(
      "1001",
    );
    expect(operatorBillDisplayNumber({ id: 42, bill_number: "" })).toBe("42");
    expect(operatorBillDisplayNumber({ id: 7, bill_number: null })).toBe("7");
    expect(
      operatorBillDisplayNumber({ id: 9, bill_number: "DEMO-SEQ-12" }),
    ).toBe("SEQ-12");
  });

  // L2-4 / L2-28: opaque UUID fragments must not be the only customer-facing
  // invoice number when a sequential id exists.
  it("uses sequential id for display when bill_number is opaque", () => {
    expect(
      operatorBillDisplayNumber({ id: 42, bill_number: "B2-0d60c280-49f" }),
    ).toBe("42");
    expect(
      operatorBillDisplayNumber({ id: 7, bill_number: "DEMO-B1-deadbeefcafe" }),
    ).toBe("7");
  });

  it("exposes a support id for copy-to-clipboard without DEMO-", () => {
    expect(
      operatorBillSupportId({ id: 1, bill_number: "DEMO-B1-deadbeefcafe" }),
    ).toBe("B1-deadbeefcafe");
    expect(operatorBillSupportId({ id: 99, bill_number: "" })).toBe("99");
  });

  it("flags opaque UUID-fragment bill numbers", () => {
    expect(isOpaqueBillNumber("B2-0d60c280-49f")).toBe(true);
    expect(isOpaqueBillNumber("1001")).toBe(false);
    expect(isOpaqueBillNumber("B-42")).toBe(false);
  });

  // NEW-6: legacy PENDING-* placeholders must not render as #PENDING-2.
  it("treats PENDING- prefixes as opaque and falls back to sequential id", () => {
    expect(isOpaqueBillNumber("PENDING-2")).toBe(true);
    expect(isOpaqueBillNumber("pending-99")).toBe(true);
    expect(
      operatorBillDisplayNumber({ id: 376, bill_number: "PENDING-2" }),
    ).toBe("376");
  });

  // R2-10: tender rows (unassigned cash) identify themselves by bill number,
  // by the person who paid, or only by an internal id. Routing them through
  // operatorBillDisplayNumber dropped both the decorative "#" and the
  // participant-name rung, so a row with a bill_id but no bill_number showed a
  // bare internal number.
  describe("operatorTenderLabel (R2-10)", () => {
    it("prefers a readable bill number and keeps the decorative #", () => {
      expect(
        operatorTenderLabel({
          id: 10,
          bill_number: "B-10",
          participant_name: "Cash",
        }),
      ).toBe("#B-10");
      expect(
        operatorTenderLabel({ id: 9, bill_number: "DEMO-1001" }),
      ).toBe("#1001");
    });

    it("falls back to the participant name before any internal id", () => {
      expect(
        operatorTenderLabel({
          id: 10,
          bill_number: "",
          participant_name: "Ana",
        }),
      ).toBe("Ana");
      expect(
        operatorTenderLabel({
          id: 10,
          bill_number: null,
          participant_name: "   Ana   ",
        }),
      ).toBe("Ana");
    });

    it("never shows an opaque UUID fragment when a name or id exists", () => {
      expect(
        operatorTenderLabel({
          id: 10,
          bill_number: "B2-0d60c280-49f",
          participant_name: "Ana",
        }),
      ).toBe("Ana");
      expect(
        operatorTenderLabel({
          id: 10,
          bill_number: "B2-0d60c280-49f",
          participant_name: "",
        }),
      ).toBe("#10");
    });

    it("falls back to #id, then to the em dash", () => {
      expect(
        operatorTenderLabel({ id: 10, bill_number: "", participant_name: "" }),
      ).toBe("#10");
      expect(
        operatorTenderLabel({ id: 0, bill_number: "", participant_name: "" }),
      ).toBe("—");
      expect(operatorTenderLabel({})).toBe("—");
    });
  });
});
