import { isGuestChromePath } from "./guestChromePath";

describe("isGuestChromePath", () => {
  it("matches table, storefront, and scan surfaces including locale prefixes", () => {
    expect(isGuestChromePath("/t/FV214XU12D")).toBe(true);
    expect(isGuestChromePath("/t/FV214XU12D/menu")).toBe(true);
    expect(isGuestChromePath("/es-ar/t/FV214XU12D")).toBe(true);
    expect(isGuestChromePath("/es/t/ABC/bill")).toBe(true);
    expect(isGuestChromePath("/b/parrilla-quebracho-azul")).toBe(true);
    expect(isGuestChromePath("/es-ar/b/parrilla-quebracho-azul")).toBe(true);
    expect(isGuestChromePath("/scan")).toBe(true);
    expect(isGuestChromePath("/es/scan")).toBe(true);
  });

  it("matches the instance root (venue page or directory)", () => {
    expect(isGuestChromePath("/")).toBe(true);
    expect(isGuestChromePath("/es")).toBe(true);
    expect(isGuestChromePath("/es-ar")).toBe(true);
  });

  it("does not match operator routes", () => {
    expect(isGuestChromePath("/pricing")).toBe(false);
    expect(isGuestChromePath("/es/pricing")).toBe(false);
    expect(isGuestChromePath("/dashboard")).toBe(false);
    expect(isGuestChromePath("/business/1/dashboard")).toBe(false);
  });
});
