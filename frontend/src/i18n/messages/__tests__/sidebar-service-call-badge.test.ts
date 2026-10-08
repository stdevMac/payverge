import { getTranslation } from "@/i18n/getTranslation";

describe("sidebar active service-call badge copy", () => {
  it.each([
    ["en", "Active service calls: 1"],
    ["es", "Llamadas de servicio activas: 1"],
    ["es-AR", "Llamadas de servicio activas: 1"],
  ] as const)("localizes the service-call meaning for %s", (locale, expected) => {
    expect(
      getTranslation(
        "businessDashboard.sidebar.activeServiceCalls",
        locale,
        { count: 1 },
      ),
    ).toBe(expected);
  });
});

describe("sidebar occupied-tables badge copy (#774)", () => {
  it.each([
    ["en", "5 occupied tables"],
    ["es", "5 mesas ocupadas"],
    ["es-AR", "5 mesas ocupadas"],
  ] as const)("localizes floor occupancy for %s", (locale, expected) => {
    expect(
      getTranslation(
        "businessDashboard.sidebar.occupiedTablesBadge_other",
        locale,
        { count: 5 },
      ),
    ).toBe(expected);
  });
});
