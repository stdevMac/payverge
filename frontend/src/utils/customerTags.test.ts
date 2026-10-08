import { parseCustomerTags, serializeCustomerTagsInput } from "./customerTags";

describe("customerTags", () => {
  describe("serializeCustomerTagsInput", () => {
    it("dedupes tags case-insensitively and keeps first-seen casing (L5-7)", () => {
      expect(serializeCustomerTagsInput("VIP, vip, Vip, regular")).toBe(
        JSON.stringify(["VIP", "regular"]),
      );
    });

    it("trims whitespace around tags before dedupe", () => {
      expect(serializeCustomerTagsInput("  a , A , b ")).toBe(
        JSON.stringify(["a", "b"]),
      );
    });
  });

  describe("parseCustomerTags", () => {
    it("dedupes JSON-array tags case-insensitively (L5-7)", () => {
      expect(parseCustomerTags(JSON.stringify(["VIP", "vip", "Regular"]))).toEqual(
        ["VIP", "Regular"],
      );
    });

    it("dedupes comma-separated legacy tags case-insensitively", () => {
      expect(parseCustomerTags("vip, VIP, regular")).toEqual(["vip", "regular"]);
    });
  });
});
