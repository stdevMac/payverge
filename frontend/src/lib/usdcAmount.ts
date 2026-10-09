// USDC has 6 decimals. Guest crypto quotes carry the EXACT on-chain amount in
// micro-USDC, including a sub-cent offset (1..9999 micro-USDC) that binds the
// transfer to one quote. Rounding it to cents for display would show the guest
// an amount that does not match what their wallet signs, so it is rendered
// with all six decimals using integer math (no float drift).
const USDC_DECIMALS = 6;

const MICROUNITS_PER_USDC = 1_000_000;

export function formatUsdcMicrounits(microunits: number): string {
  if (!Number.isSafeInteger(microunits)) {
    throw new RangeError(
      `USDC micro-units must be a safe integer: ${microunits}`,
    );
  }
  const sign = microunits < 0 ? "-" : "";
  const abs = Math.abs(microunits);
  const whole = Math.floor(abs / MICROUNITS_PER_USDC);
  const fraction = String(abs % MICROUNITS_PER_USDC).padStart(
    USDC_DECIMALS,
    "0",
  );
  return `${sign}${whole}.${fraction}`;
}
