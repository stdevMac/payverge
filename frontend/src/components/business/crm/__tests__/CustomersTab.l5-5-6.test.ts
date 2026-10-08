import fs from "fs";
import path from "path";

describe("CustomersTab L5-5 / L5-6 seams", () => {
  const src = fs.readFileSync(
    path.join(__dirname, "..", "CustomersTab.tsx"),
    "utf8",
  );

  it("L5-5 resets add-customer form on open/close", () => {
    expect(src).toMatch(/resetNewCustomerForm/);
    expect(src).toMatch(/openAddModal/);
    expect(src).toMatch(/closeAddModal/);
  });

  it("L5-6 opens details modal before awaiting API", () => {
    // setDetailsModal(true) must appear before the await getCustomerDetails call
    // in handleViewDetails (open-first, fetch-second).
    const fn = src.match(
      /const handleViewDetails = async[\s\S]*?^  };/m,
    )?.[0];
    expect(fn).toBeTruthy();
    const openIdx = fn!.indexOf("setDetailsModal(true)");
    const awaitIdx = fn!.indexOf("await businessCRMAPI.getCustomerDetails");
    expect(openIdx).toBeGreaterThanOrEqual(0);
    expect(awaitIdx).toBeGreaterThan(openIdx);
  });
});
