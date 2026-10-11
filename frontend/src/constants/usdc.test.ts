import { USDC_ADDRESSES } from "./usdc";

const EVM_ADDRESS = /^0x[0-9a-fA-F]{40}$/;

describe("USDC_ADDRESSES", () => {
  it("maps Base and Base Sepolia to the known USDC contracts", () => {
    expect(USDC_ADDRESSES[8453]).toBe(
      "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
    );
    expect(USDC_ADDRESSES[84532]).toBe(
      "0x82d491aB292C06Aa7148234b910cdea5FE788223",
    );
  });

  it("stores only 0x-prefixed 40-hex addresses", () => {
    const addresses = Object.values(USDC_ADDRESSES);
    expect(addresses.length).toBeGreaterThan(0);
    for (const address of addresses) {
      expect(address).toMatch(EVM_ADDRESS);
    }
  });
});
