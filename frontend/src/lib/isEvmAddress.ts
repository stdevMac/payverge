/**
 * Lightweight EVM-address format guard for money-config surfaces.
 *
 * Mirrors the backend's authoritative check (isValidEthereumAddress in
 * business_handlers.go: `^0x[0-9a-fA-F]{40}$`) so a client-side guard rejects
 * the exact same shapes the server rejects, giving the operator an immediate
 * inline error instead of a generic save failure round-trip. This deliberately
 * does NOT enforce an EIP-55 checksum — neither does the backend, and pulling
 * viem's checksum path into the bundle (and into jsdom) is not worth it for a
 * format guard whose authority is the server.
 *
 * @param value - raw user input; leading/trailing whitespace is ignored
 * @returns true when `value` is a well-formed `0x` + 40 hex-character address
 */
const EVM_ADDRESS_REGEX = /^0x[0-9a-fA-F]{40}$/;

export function isEvmAddress(value: string | null | undefined): boolean {
  if (!value) return false;
  return EVM_ADDRESS_REGEX.test(value.trim());
}

// Inclusive ceiling of the reserved low-address range used by seeds and
// fixtures. Matches backend maxPlaceholderSettlement (includes 0x…dE01).
const MAX_PLACEHOLDER_SETTLEMENT = BigInt("0xde01");

/**
 * Guest-crypto settlement guard. Format-valid zero and low placeholder
 * addresses (including the demo seed wallet 0x…dE01) must not surface a
 * USDC tender — funds sent there are burned.
 */
export function isUsableGuestSettlementAddress(
  value: string | null | undefined,
): boolean {
  if (!isEvmAddress(value)) return false;
  const n = BigInt(`0x${value!.trim().slice(2)}`);
  return n > MAX_PLACEHOLDER_SETTLEMENT;
}
