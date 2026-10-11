import { staffHomeHref } from "./staffHomeHref";

describe("staffHomeHref (#879)", () => {
  it("keeps English on the unprefixed staff home", () => {
    expect(staffHomeHref("en")).toBe("/staff/home");
  });

  it("prefixes es and es-AR so localized login can land on a real route", () => {
    expect(staffHomeHref("es")).toBe("/es/staff/home");
    expect(staffHomeHref("es-AR")).toBe("/es-ar/staff/home");
  });
});
