/**
 * Pure validation for the registration "business" step, extracted so the
 * country requirement is unit-testable. Country is REQUIRED: the payload
 * derives default_currency / timezone from it via getDefaultsForCountry, and
 * an empty country silently seeds the business with USD/UTC (Wave 4 fix).
 */
export interface BusinessStepData {
  name: string;
  owner_name?: string;
  email?: string;
  address?: { country?: string };
}

export function isValidEmail(email: string): boolean {
  // Mirrors the page's original inline regex so email behavior is unchanged.
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}

export function isBusinessStepComplete(data: BusinessStepData): boolean {
  return (
    data.name.trim().length > 0 &&
    (data.owner_name || "").trim().length > 0 &&
    (data.email || "").trim().length > 0 &&
    isValidEmail(data.email || "") &&
    (data.address?.country || "").trim().length > 0
  );
}
