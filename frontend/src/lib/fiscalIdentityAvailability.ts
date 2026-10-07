import { normalizeFiscalCountry } from "@/components/business/fiscal/receiptTypes";

/** Countries whose fiscal-identity capture form is implemented (CUIT/IVA). */
const FISCAL_IDENTITY_COUNTRIES = new Set(["AR"]);

export function shouldShowFiscalIdentityFields({
  country,
  fiscalCountry,
}: {
  country?: string | null;
  fiscalCountry?: string | null;
} = {}): boolean {
  // Already-loaded fiscal settings win, including empty/null (fail closed).
  const resolved =
    fiscalCountry !== undefined
      ? normalizeFiscalCountry(fiscalCountry)
      : normalizeFiscalCountry(country);
  return FISCAL_IDENTITY_COUNTRIES.has(resolved);
}
