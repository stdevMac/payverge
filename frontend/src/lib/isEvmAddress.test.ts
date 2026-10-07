import { isEvmAddress, isUsableGuestSettlementAddress } from "./isEvmAddress";

describe("isEvmAddress", () => {
  it("accepts a well-formed lowercase 0x + 40-hex address", () => {
    expect(isEvmAddress("0x1234567890123456789012345678901234567890")).toBe(
      true,
    );
  });

  it("accepts a mixed-case (checksum-shaped) address without enforcing EIP-55", () => {
    expect(isEvmAddress("0xAbC1230000000000000000000000000000000DEF")).toBe(
      true,
    );
  });

  it("trims surrounding whitespace before validating", () => {
    expect(
      isEvmAddress("  0x1234567890123456789012345678901234567890  "),
    ).toBe(true);
  });

  it("rejects a non-hex / freeform string", () => {
    expect(isEvmAddress("not-an-address")).toBe(false);
  });

  it("rejects a value missing the 0x prefix", () => {
    expect(isEvmAddress("1234567890123456789012345678901234567890")).toBe(
      false,
    );
  });

  it("rejects an address that is too short", () => {
    expect(isEvmAddress("0x1234")).toBe(false);
  });

  it("rejects an address that is too long", () => {
    expect(
      isEvmAddress("0x12345678901234567890123456789012345678901"),
    ).toBe(false);
  });

  it("rejects a value with non-hex characters in the body", () => {
    expect(isEvmAddress("0x123456789012345678901234567890123456789g")).toBe(
      false,
    );
  });

  it("rejects empty and nullish input", () => {
    expect(isEvmAddress("")).toBe(false);
    expect(isEvmAddress(null)).toBe(false);
    expect(isEvmAddress(undefined)).toBe(false);
  });
});

describe("isUsableGuestSettlementAddress", () => {
  it("accepts a non-placeholder funded-looking address", () => {
    expect(
      isUsableGuestSettlementAddress(
        "0x1234567890123456789012345678901234567890",
      ),
    ).toBe(true);
  });

  it("rejects the zero address and the demo seed placeholder", () => {
    expect(
      isUsableGuestSettlementAddress(
        "0x0000000000000000000000000000000000000000",
      ),
    ).toBe(false);
    expect(
      isUsableGuestSettlementAddress(
        "0x0000000000000000000000000000000000000001",
      ),
    ).toBe(false);
    expect(
      isUsableGuestSettlementAddress(
        "0x000000000000000000000000000000000000dE01",
      ),
    ).toBe(false);
  });

  it("rejects malformed input", () => {
    expect(isUsableGuestSettlementAddress("not-an-address")).toBe(false);
    expect(isUsableGuestSettlementAddress("")).toBe(false);
  });
});
