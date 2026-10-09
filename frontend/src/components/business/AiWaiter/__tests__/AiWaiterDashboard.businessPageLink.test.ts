import { readFileSync } from "fs";
import { join } from "path";

// Regression-lock: the AI-Waiter enable flow must never push the dead
// /business/<id>/design route again. It must route through the shared
// getBusinessPageEditorPath helper (which targets ?tab=business-page).
describe("AiWaiterDashboard business-page redirect (no dead /design route)", () => {
  const source = readFileSync(
    join(__dirname, "..", "AiWaiterDashboard.tsx"),
    "utf8",
  );

  it("does not push a /design path", () => {
    expect(source).not.toMatch(/\/design`/);
  });

  it("routes via getBusinessPageEditorPath", () => {
    expect(source).toContain("getBusinessPageEditorPath(business)");
  });
});
