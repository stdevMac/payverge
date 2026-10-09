// USDC addresses on different chains.
// Numeric chain ids (not @lifi/sdk ChainId) so guest routes can read this
// map without downloading the bridging SDK.
export const USDC_ADDRESSES: Record<number, string> = {
  // Mainnets
  1: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // Ethereum
  137: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359", // Polygon (native USDC)
  42161: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", // Arbitrum
  10: "0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85", // Optimism
  8453: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", // Base
  43114: "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E", // Avalanche
  56: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", // BSC
  // Testnets
  84532: "0x82d491aB292C06Aa7148234b910cdea5FE788223", // Base Sepolia (from contracts.baseSepolia.txt)
  11155111: "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238", // Sepolia
};
