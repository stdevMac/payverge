/** Owner sandbox must follow the active dashboard locale, not the business default. */
export function resolveAiWaiterSandboxLocale(
  ownerLocale?: string,
  businessDefaultLanguage?: string,
): string {
  const owner = ownerLocale?.trim();
  if (owner) return owner;
  return businessDefaultLanguage?.trim() || "en";
}
