import fs from "fs";
import path from "path";

/**
 * Source contract: Edit must open the address editor with the saved
 * fulfillment context instead of only changing the hash.
 */
describe("ConvertingBusinessLandingPage delivery edit wiring", () => {
  const landing = fs.readFileSync(
    path.join(__dirname, "../ConvertingBusinessLandingPage.tsx"),
    "utf8",
  );
  const tab = fs.readFileSync(
    path.join(__dirname, "../BusinessDeliveryTab.tsx"),
    "utf8",
  );

  it("opens the editor from Edit and forwards the saved address context", () => {
    expect(landing).toContain("handleEditFulfillment");
    expect(landing).toContain("setAddressEditorToken");
    expect(landing).toContain("onEditFulfillment={handleEditFulfillment}");
    expect(landing).toContain("fulfillmentContext={fulfillmentContext}");
    expect(landing).toContain("openAddressEditorToken={addressEditorToken}");
    expect(tab).toContain("fulfillmentContext={fulfillmentContext}");
    expect(tab).toContain("openAddressEditorToken={openAddressEditorToken}");
  });
});
