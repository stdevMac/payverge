import { formatPreparedUnits } from "./formatPreparedUnits";

describe("formatPreparedUnits (L8-1)", () => {
  const en = (key: string) => {
    const map: Record<string, string> = {
      preparedUnitsOne: "{count} prepared unit",
      preparedUnitsOther: "{count} prepared units",
      preparedUnits: "{count} prepared units", // legacy buggy always-plural
    };
    return map[key] ?? key;
  };

  const es = (key: string) => {
    const map: Record<string, string> = {
      preparedUnitsOne: "{count} unidad a preparar",
      preparedUnitsOther: "{count} unidades a preparar",
    };
    return map[key] ?? key;
  };

  it("uses singular for count === 1 in en", () => {
    expect(formatPreparedUnits(1, en)).toBe("1 prepared unit");
  });

  it("uses plural for count !== 1 in en", () => {
    expect(formatPreparedUnits(0, en)).toBe("0 prepared units");
    expect(formatPreparedUnits(2, en)).toBe("2 prepared units");
    expect(formatPreparedUnits(5, en)).toBe("5 prepared units");
  });

  it("uses singular/plural correctly in es", () => {
    expect(formatPreparedUnits(1, es)).toBe("1 unidad a preparar");
    expect(formatPreparedUnits(2, es)).toBe("2 unidades a preparar");
  });

  it("supports a namespaced base key (display.preparedUnits)", () => {
    const t = (key: string) => {
      if (key === "display.preparedUnitsOne") return "{count} prepared unit";
      if (key === "display.preparedUnitsOther") return "{count} prepared units";
      return key;
    };
    expect(formatPreparedUnits(1, t, "display.preparedUnits")).toBe(
      "1 prepared unit",
    );
    expect(formatPreparedUnits(3, t, "display.preparedUnits")).toBe(
      "3 prepared units",
    );
  });
});
