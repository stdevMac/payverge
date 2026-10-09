export function orderStatusTranslationKey(status?: string | null): string {
  const normalized = status?.trim();
  return normalized ? `orderStatuses.${normalized}` : "orderStatuses.unknown";
}
