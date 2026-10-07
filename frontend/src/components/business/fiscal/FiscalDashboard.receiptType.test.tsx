import { TAX_CONDITIONS_BY_COUNTRY } from "./taxConditions";

describe("FiscalDashboard AR tax conditions", () => {
  it("sends monotributo (backend wire value), not monotributista", () => {
    expect(TAX_CONDITIONS_BY_COUNTRY.AR).toContain("monotributo");
    expect(TAX_CONDITIONS_BY_COUNTRY.AR).not.toContain("monotributista");
  });
});
