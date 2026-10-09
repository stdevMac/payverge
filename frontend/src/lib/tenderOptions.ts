import {
  PAYMENT_METHOD_OPTIONS,
  PaymentMethod,
  type PaymentMethodOption,
} from "@/types/alternativePayments";

const LATAM_COUNTRIES = new Set(["AR", "MX", "CL", "UY", "PE", "CO", "BR"]);

function normalizeOperatorLocale(locale?: string | null): string {
  return (locale || "").trim().toLowerCase().replace(/_/g, "-");
}

/**
 * Operator in-person tenders for Registrar pago.
 * es / es-AR / LATAM venues hide US-only Venmo and treat card as Mercado Pago (#649).
 */
export function isLatamOperatorVenue(
  locale?: string | null,
  country?: string | null,
): boolean {
  const loc = normalizeOperatorLocale(locale);
  if (loc === "es" || loc.startsWith("es-")) return true;
  const cc = (country || "").trim().toUpperCase();
  return LATAM_COUNTRIES.has(cc);
}

export function recordPaymentTenderOptions(
  locale?: string | null,
  country?: string | null,
): PaymentMethodOption[] {
  const latam = isLatamOperatorVenue(locale, country);
  return PAYMENT_METHOD_OPTIONS.filter((opt) => {
    if (opt.value === PaymentMethod.CRYPTO) return false;
    if (opt.value === PaymentMethod.VENMO && latam) return false;
    return true;
  });
}

export function recordPaymentMethodI18nSuffix(
  method: PaymentMethod,
  locale?: string | null,
  country?: string | null,
): string {
  if (
    method === PaymentMethod.CARD &&
    isLatamOperatorVenue(locale, country)
  ) {
    return "mercadopago";
  }
  switch (method) {
    case PaymentMethod.CASH:
      return "cash";
    case PaymentMethod.CARD:
      return "card";
    case PaymentMethod.VENMO:
      return "venmo";
    case PaymentMethod.OTHER:
      return "other";
    default:
      return "unknown";
  }
}
