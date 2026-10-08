import { getMarketingCapabilities } from "./marketingCapabilities";

describe("getMarketingCapabilities", () => {
  it("gives owners every marketing capability including approve", () => {
    expect(getMarketingCapabilities(undefined, true)).toEqual({
      canView: true,
      canEdit: true,
      canGenerate: true,
      canUpload: true,
      canApprove: true,
      isOwner: true,
    });
  });

  it("keeps marketing:read view-only", () => {
    expect(getMarketingCapabilities(["marketing:read"], false)).toEqual({
      canView: true,
      canEdit: false,
      canGenerate: false,
      canUpload: false,
      canApprove: false,
      isOwner: false,
    });
  });

  it("uses marketing:write for creative mutations but not approve", () => {
    const capabilities = getMarketingCapabilities(
      ["marketing:read", "marketing:write"],
      false,
    );
    expect(capabilities).toEqual({
      canView: true,
      canEdit: true,
      canGenerate: true,
      canUpload: true,
      canApprove: false,
      isOwner: false,
    });
  });

  it.each([undefined, null, [], ["unknown:role"]])(
    "safely denies read access when effective permissions are absent or unknown (%p)",
    (permissions) => {
      expect(getMarketingCapabilities(permissions, false).canView).toBe(false);
    },
  );

  it("owner flag wins even when permission list is empty", () => {
    const caps = getMarketingCapabilities([], true);
    expect(caps.canApprove).toBe(true);
    expect(caps.canEdit).toBe(true);
    expect(caps.isOwner).toBe(true);
  });

  it("staff never receive canApprove even with every non-owner permission", () => {
    const caps = getMarketingCapabilities(
      [
        "marketing:read",
        "marketing:write",
        "menu:write",
      ],
      false,
    );
    expect(caps.canApprove).toBe(false);
    expect(caps.isOwner).toBe(false);
    expect(caps.canEdit).toBe(true);
  });
});
