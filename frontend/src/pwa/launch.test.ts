import { resolvePwaLaunchPath } from "./launch";

describe("resolvePwaLaunchPath", () => {
  it("uses the current authenticated staff business without loading owner businesses", async () => {
    const loadBusinesses = jest.fn();
    const onResolutionError = jest.fn();

    await expect(
      resolvePwaLaunchPath({
        identity: { key: "staff:7:42", roleType: "staff" },
        rememberedPath: "/business/old/dashboard",
        staffData: { business_id: 42, business_slug: "casa" },
        loadBusinesses,
        onResolutionError,
      }),
    ).resolves.toBe("/business/casa/dashboard");
    expect(loadBusinesses).not.toHaveBeenCalled();
    expect(onResolutionError).not.toHaveBeenCalled();
  });

  it("falls back to the current staff business id when no safe slug is available", async () => {
    const loadBusinesses = jest.fn();
    const onResolutionError = jest.fn();

    await expect(
      resolvePwaLaunchPath({
        identity: { key: "staff:7:42", roleType: "staff" },
        rememberedPath: "/business/old/dashboard",
        staffData: { business_id: 42, business_slug: "../other" },
        loadBusinesses,
        onResolutionError,
      }),
    ).resolves.toBe("/business/42/dashboard");
    expect(loadBusinesses).not.toHaveBeenCalled();
    expect(onResolutionError).toHaveBeenCalledTimes(1);
  });

  it("signals an invalid current staff business before falling back", async () => {
    const onResolutionError = jest.fn();

    await expect(
      resolvePwaLaunchPath({
        identity: { key: "staff:7:42", roleType: "staff" },
        rememberedPath: null,
        staffData: null,
        loadBusinesses: jest.fn(),
        onResolutionError,
      }),
    ).resolves.toBe("/dashboard");
    expect(onResolutionError).toHaveBeenCalledTimes(1);
  });

  it("opens an owner path only when it exactly matches a current business", async () => {
    const loadBusinesses = jest
      .fn()
      .mockResolvedValue([{ id: 42, business_id: "casa" }, { id: 9 }]);
    const onResolutionError = jest.fn();

    await expect(
      resolvePwaLaunchPath({
        identity: { key: "user:19", roleType: "owner" },
        rememberedPath: "/business/casa/dashboard",
        staffData: null,
        loadBusinesses,
        onResolutionError,
      }),
    ).resolves.toBe("/business/casa/dashboard");
    expect(loadBusinesses).toHaveBeenCalledTimes(1);
    expect(onResolutionError).not.toHaveBeenCalled();
  });

  it.each([
    ["missing", null],
    ["revoked", "/business/revoked/dashboard"],
    ["malformed", "/business/casa/dashboard?tab=bills"],
    ["external", "https://evil.test/business/casa/dashboard"],
    ["protocol-relative", "//evil.test/business/casa/dashboard"],
  ])(
    "falls back for a %s owner destination",
    async (_caseName, rememberedPath) => {
      const loadBusinesses = jest
        .fn()
        .mockResolvedValue([{ id: 42, business_id: "casa" }]);
      const onResolutionError = jest.fn();

      await expect(
        resolvePwaLaunchPath({
          identity: { key: "user:19", roleType: "owner" },
          rememberedPath,
          staffData: null,
          loadBusinesses,
          onResolutionError,
        }),
      ).resolves.toBe("/dashboard");

      if (rememberedPath === "/business/revoked/dashboard") {
        expect(loadBusinesses).toHaveBeenCalledTimes(1);
      } else {
        expect(loadBusinesses).not.toHaveBeenCalled();
      }
      if (rememberedPath === null) {
        expect(onResolutionError).not.toHaveBeenCalled();
      } else {
        expect(onResolutionError).toHaveBeenCalledTimes(1);
      }
    },
  );

  it("falls back when owner business validation fails", async () => {
    const onResolutionError = jest.fn();
    await expect(
      resolvePwaLaunchPath({
        identity: { key: "user:19", roleType: "owner" },
        rememberedPath: "/business/casa/dashboard",
        staffData: null,
        loadBusinesses: async () => {
          throw new Error("network unavailable");
        },
        onResolutionError,
      }),
    ).resolves.toBe("/dashboard");
    expect(onResolutionError).toHaveBeenCalledTimes(1);
  });

  it("keeps fallback resolution non-gating when the error callback throws", async () => {
    await expect(
      resolvePwaLaunchPath({
        identity: { key: "user:19", roleType: "owner" },
        rememberedPath: "/business/revoked/dashboard",
        staffData: null,
        loadBusinesses: async () => [],
        onResolutionError: () => {
          throw new Error("analytics unavailable");
        },
      }),
    ).resolves.toBe("/dashboard");
  });
});
