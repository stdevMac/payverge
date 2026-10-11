import { pctClasses } from "@/components/business/accounting/FoodCostTable";

describe("pctClasses food-cost thresholds", () => {
  it("returns neutral for no sales (0)", () => {
    expect(pctClasses(0)).toContain("ink");
    expect(pctClasses(-0.1)).toContain("ink"); // pathological, guarded by pct <= 0
  });
  it("is green below 30%", () => {
    expect(pctClasses(0.29)).toContain("emerald");
  });
  it("is amber from 30% through 35%", () => {
    expect(pctClasses(0.3)).toContain("amber");
    expect(pctClasses(0.35)).toContain("amber");
  });
  it("is red above 35%", () => {
    expect(pctClasses(0.36)).toContain("rose");
  });
});
