import { appendOptionalSuffix } from "./appendOptionalSuffix";

describe("appendOptionalSuffix", () => {
  it("appends the localized optional marker once", () => {
    expect(appendOptionalSuffix("Solicitudes especiales", "opcional")).toBe(
      "Solicitudes especiales (opcional)",
    );
    expect(appendOptionalSuffix("Special Requests", "optional")).toBe(
      "Special Requests (optional)",
    );
  });

  it("does not double-suffix when the catalog already includes optional", () => {
    expect(
      appendOptionalSuffix("Solicitudes especiales (opcional)", "opcional"),
    ).toBe("Solicitudes especiales (opcional)");
    expect(appendOptionalSuffix("Special Requests (Optional)", "optional")).toBe(
      "Special Requests (Optional)",
    );
    expect(appendOptionalSuffix("特殊要求（可选）", "可选")).toBe("特殊要求（可选）");
  });
});
