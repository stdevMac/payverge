import { deepMerge } from "../deepMerge";

describe("deepMerge (locale override layering)", () => {
  test("overlays override values over the base, recursively", () => {
    const base = {
      common: { save: "Guardar", cancel: "Cancelar" },
      dashboard: { greeting: "Tú tienes pedidos" },
    };
    const override = {
      dashboard: { greeting: "Vos tenés pedidos" },
    };
    expect(deepMerge(base, override)).toEqual({
      common: { save: "Guardar", cancel: "Cancelar" },
      dashboard: { greeting: "Vos tenés pedidos" },
    });
  });

  test("keeps base keys the override does not mention (DRY overrides)", () => {
    const base = { a: { x: "1", y: "2" } };
    const override = { a: { y: "two" } };
    expect(deepMerge(base, override)).toEqual({ a: { x: "1", y: "two" } });
  });

  test("an empty override yields a value equal to the base", () => {
    const base = { a: { x: "1" }, b: "2" };
    expect(deepMerge(base, {})).toEqual(base);
  });

  test("does not mutate the base object", () => {
    const base = { a: { x: "1" } };
    const snapshot = JSON.parse(JSON.stringify(base));
    deepMerge(base, { a: { x: "9" } });
    expect(base).toEqual(snapshot);
  });

  test("override replaces arrays wholesale rather than merging by index", () => {
    const base = { tabs: ["a", "b", "c"] };
    const override = { tabs: ["x", "y"] };
    expect(deepMerge(base, override)).toEqual({ tabs: ["x", "y"] });
  });

  test("override scalar replaces a base scalar at the same path", () => {
    const base = { greeting: "Hola tú" };
    const override = { greeting: "Hola vos" };
    expect(deepMerge(base, override)).toEqual({ greeting: "Hola vos" });
  });
});
