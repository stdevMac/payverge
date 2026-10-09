import { getTranslation } from "../SimpleTranslationProvider";

describe("es-AR dashboard leftovers (#915)", () => {
  it("uses voseo on the service-call retry and the 86 menu hint", () => {
    const updateFailed = getTranslation(
      "businessDashboard.dashboard.tableManager.serviceCalls.updateFailed",
      "es-AR",
    ) as string;
    const dishesHint = getTranslation(
      "businessDashboard.inventoryManager.needsAttention.dishesHint",
      "es-AR",
    ) as string;

    expect(updateFailed).toMatch(/Intentá de nuevo/);
    expect(updateFailed).not.toMatch(/Inténtalo/);
    expect(dishesHint).toMatch(/abrí el Menú/);
    expect(dishesHint).not.toMatch(/\babre el Menú\b/);
  });
});
