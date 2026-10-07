// Audit G-04: the LI.FI SDK's built-in chain preload has historically leaked
// an unhandled rejection to the guest console when the chain-list request
// fails. Our initializeLiFi disables it (preloadChains: false) and runs a
// handled preload instead — these tests pin that contract for SDK v4's
// createClient + getChains(client, ...) API.

const mockCreateClient = jest.fn();
const mockSetChains = jest.fn();
const mockGetChains = jest.fn();

jest.mock("@lifi/sdk", () => ({
  createClient: (options: unknown) => {
    mockCreateClient(options);
    return {
      setChains: (chains: unknown) => mockSetChains(chains),
    };
  },
  getChains: (client: unknown, params: unknown) => mockGetChains(client, params),
  ChainId: {
    BAS: 8453,
    ETH: 1,
    POL: 137,
    ARB: 42161,
    OPT: 10,
    AVA: 43114,
    BSC: 56,
  },
  ChainType: { EVM: "EVM", SVM: "SVM", UTXO: "UTXO", MVM: "MVM" },
}));

jest.mock("@lifi/sdk-provider-ethereum", () => ({
  EthereumProvider: jest.fn(() => ({ type: "EVM" })),
}));

import { initializeLiFi } from "./config";

describe("initializeLiFi (G-04)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("disables the SDK's leaky built-in chain preload", () => {
    mockGetChains.mockResolvedValue([]);
    initializeLiFi();
    expect(mockCreateClient).toHaveBeenCalledWith(
      expect.objectContaining({ preloadChains: false }),
    );
  });

  it("populates chains via a handled preload on success", async () => {
    const chains = [{ id: 8453, name: "Base" }];
    mockGetChains.mockResolvedValue(chains);
    initializeLiFi();
    await expect(mockGetChains.mock.results[0]?.value).resolves.toEqual(chains);
    expect(mockSetChains).toHaveBeenCalledWith(chains);
  });

  it("swallows a failed chain preload instead of leaking an unhandled rejection", async () => {
    const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});
    mockGetChains.mockRejectedValue(new Error("Failed to fetch"));

    expect(() => initializeLiFi()).not.toThrow();
    // The loading promise must RESOLVE (handled), not reject.
    await expect(mockGetChains.mock.results[0]?.value).rejects.toThrow(
      "Failed to fetch",
    );
    // Give the .catch handler a turn to run.
    await Promise.resolve();
    expect(mockSetChains).not.toHaveBeenCalled();
    expect(warnSpy).toHaveBeenCalled();
    warnSpy.mockRestore();
  });
});
