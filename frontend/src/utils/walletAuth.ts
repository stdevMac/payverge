export function buildWalletAuthMessage(
  walletAddress: string,
  challenge: string,
  currentChainId: number,
  statement: string,
): string {
  const domain = typeof window !== "undefined" ? window.location.host : "localhost";
  const origin = typeof window !== "undefined" ? window.location.origin : "http://localhost:3000";
  const issuedAt = new Date().toISOString();

  return `${domain} wants you to sign in with your Ethereum account:\n${walletAddress}\n\n${statement}\n\nURI: ${origin}\nVersion: 1\nChain ID: #${currentChainId}\nNonce: ${challenge}\nIssued At: ${issuedAt}`;
}
