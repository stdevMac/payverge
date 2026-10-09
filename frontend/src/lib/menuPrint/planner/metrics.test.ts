import { PT_TO_MM, estimateTextBox } from "./metrics";

describe("estimateTextBox", () => {
  it("uses the approved physical text estimate", () => {
    expect(PT_TO_MM).toBe(25.4 / 72);
    expect(
      estimateTextBox({
        text: "Seasonal ingredients prepared daily",
        widthMm: 55,
        fontPt: 10,
        floorPt: 9.5,
        lineHeight: 1.35,
      }),
    ).toEqual({ lines: 2, heightMm: 9.53 });
  });

  it("allows estimates when no family floor applies", () => {
    expect(
      estimateTextBox({
        text: "Charred steak with potatoes and chimichurri",
        widthMm: 55,
        fontPt: 10,
        lineHeight: 1.35,
      }),
    ).toEqual({ lines: 2, heightMm: 9.53 });
  });

  it("rejects type below the family floor with the approved message", () => {
    expect(() =>
      estimateTextBox({
        text: "Tiny",
        widthMm: 55,
        fontPt: 8,
        floorPt: 9.5,
        lineHeight: 1.35,
      }),
    ).toThrow("below family type floor");
  });

  it.each([
    ["", 1],
    ["旬の素材をていねいに仕立てます", 2],
    ["averylongunbreakableingredienttoken", 4],
  ])(
    "wraps blank, CJK, and long-token text deterministically",
    (text, lines) => {
      const input = {
        text,
        widthMm: 20,
        fontPt: 10,
        floorPt: 9.5,
        lineHeight: 1.2,
      };
      expect(estimateTextBox(input).lines).toBe(lines);
      expect(estimateTextBox(input)).toEqual(estimateTextBox({ ...input }));
    },
  );

  it("chunks arbitrary unbroken code-point runs across every required line", () => {
    expect(
      estimateTextBox({
        text: "界".repeat(1000),
        widthMm: 20,
        fontPt: 10,
        floorPt: 9.5,
        lineHeight: 1.2,
        glyphWidthFactor: 1,
      }).lines,
    ).toBe(200);
  });

  it.each([
    ["widthMm", { widthMm: 0 }, "widthMm must be finite and positive"],
    ["fontPt", { fontPt: Number.NaN }, "fontPt must be finite and positive"],
    [
      "lineHeight",
      { lineHeight: -1 },
      "lineHeight must be finite and positive",
    ],
    ["floorPt", { floorPt: 0 }, "floorPt must be finite and positive"],
    [
      "glyphWidthFactor",
      { glyphWidthFactor: Number.POSITIVE_INFINITY },
      "glyphWidthFactor must be finite and positive",
    ],
  ])("validates %s", (_field, override, message) => {
    expect(() =>
      estimateTextBox({
        text: "Dish",
        widthMm: 55,
        fontPt: 10,
        floorPt: 9.5,
        lineHeight: 1.2,
        ...override,
      }),
    ).toThrow(message);
  });
});
