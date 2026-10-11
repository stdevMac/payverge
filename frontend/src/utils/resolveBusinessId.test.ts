import { resolveNumericBusinessId } from "./resolveBusinessId";
import { getBusiness } from "@/api/business";

jest.mock("@/api/business", () => ({ getBusiness: jest.fn() }));

describe("resolveNumericBusinessId", () => {
  beforeEach(() => jest.resetAllMocks());

  it("returns the number directly for numeric params without any API call", async () => {
    await expect(resolveNumericBusinessId("42")).resolves.toBe(42);
    expect(getBusiness).not.toHaveBeenCalled();
  });

  it("resolves a slug via getBusiness", async () => {
    (getBusiness as jest.Mock).mockResolvedValue({ id: 7, name: "Demo" });
    await expect(resolveNumericBusinessId("demo-admin-1-ai-pro")).resolves.toBe(7);
    expect(getBusiness).toHaveBeenCalledWith("demo-admin-1-ai-pro");
  });

  it("returns null (never NaN) when the slug cannot be resolved", async () => {
    (getBusiness as jest.Mock).mockRejectedValue(new Error("HTTP 404"));
    await expect(resolveNumericBusinessId("nope")).resolves.toBeNull();
  });
});
