import { formatUnit } from "./formatUnit";

describe("formatUnit", () => {
  describe("English", () => {
    it("formats symbol units without plural", () => {
      expect(formatUnit(0, "g", "en")).toBe("0 g");
      expect(formatUnit(1, "g", "en")).toBe("1 g");
      expect(formatUnit(3, "g", "en")).toBe("3 g");
      expect(formatUnit(0, "kg", "en")).toBe("0 kg");
      expect(formatUnit(0, "ml", "en")).toBe("0 ml");
    });

    it("pluralizes word-form units", () => {
      expect(formatUnit(0, "unit", "en")).toBe("0 units");
      expect(formatUnit(1, "unit", "en")).toBe("1 unit");
      expect(formatUnit(3, "unit", "en")).toBe("3 units");
      expect(formatUnit(0, "liter", "en")).toBe("0 liters");
      expect(formatUnit(1, "liter", "en")).toBe("1 liter");
      expect(formatUnit(3, "liter", "en")).toBe("3 liters");
      expect(formatUnit(0, "box", "en")).toBe("0 boxes");
      expect(formatUnit(1, "box", "en")).toBe("1 box");
      expect(formatUnit(2, "box", "en")).toBe("2 boxes");
    });

    it("passes through unknown units verbatim", () => {
      expect(formatUnit(2, "shipment", "en")).toBe("2 shipment");
    });
  });

  describe("Spanish", () => {
    it("formats symbol units without plural", () => {
      expect(formatUnit(3, "g", "es")).toBe("3 g");
      expect(formatUnit(0, "kg", "es")).toBe("0 kg");
    });

    it("pluralizes word-form units", () => {
      expect(formatUnit(0, "unit", "es")).toBe("0 unidades");
      expect(formatUnit(1, "unit", "es")).toBe("1 unidad");
      expect(formatUnit(3, "unit", "es")).toBe("3 unidades");
      expect(formatUnit(0, "liter", "es")).toBe("0 litros");
      expect(formatUnit(1, "liter", "es")).toBe("1 litro");
      expect(formatUnit(0, "box", "es")).toBe("0 cajas");
      expect(formatUnit(1, "box", "es")).toBe("1 caja");
    });
  });

  describe("decimal formatting", () => {
    it("renders integers without trailing zeros", () => {
      expect(formatUnit(3, "unit", "en")).toBe("3 units");
      expect(formatUnit(0, "g", "en")).toBe("0 g");
    });

    it("renders non-integers with two decimal places (en-US style)", () => {
      expect(formatUnit(1.5, "unit", "en")).toBe("1.50 units");
      expect(formatUnit(2.345, "g", "en")).toBe("2.35 g");
      expect(formatUnit(0.5, "kg", "en")).toBe("0.50 kg");
    });

    // S-4 operator: Spanish inventory must not show en-US "-0.35 kg".
    it("renders non-integers with locale decimal separator (es)", () => {
      expect(formatUnit(1.5, "liter", "es")).toBe("1,50 litros");
      expect(formatUnit(-0.35, "kg", "es")).toBe("-0,35 kg");
      expect(formatUnit(2.345, "g", "es")).toBe("2,35 g");
    });

    it("treats 1 (singular) and 1.0 (technically singular but typically a decimal) consistently", () => {
      // count === 1 stays singular; an exact 1 prints without trailing zeros.
      expect(formatUnit(1, "unit", "en")).toBe("1 unit");
      expect(formatUnit(1, "liter", "es")).toBe("1 litro");
      // 1.0 is === 1 in JS, so it follows the same path.
      expect(formatUnit(1.0, "unit", "en")).toBe("1 unit");
    });
  });
});
