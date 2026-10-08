import {
  parseCustomerTags,
  serializeCustomerTagsInput,
} from "@/utils/customerTags";

describe("customer tag helpers", () => {
  it("parses stored JSON array tags", () => {
    expect(parseCustomerTags('["vip","birthday"," patio "]')).toEqual([
      "vip",
      "birthday",
      "patio",
    ]);
  });

  it("parses legacy comma-separated tags", () => {
    expect(parseCustomerTags("vip, birthday, patio")).toEqual([
      "vip",
      "birthday",
      "patio",
    ]);
  });

  it("serializes tag input as the backend JSON-array string", () => {
    expect(serializeCustomerTagsInput("vip, birthday, , patio")).toBe(
      '["vip","birthday","patio"]',
    );
  });
});
