/**
 * Format a number as a string with thousands separators
 * @param value - The number to format
 * @returns Formatted string with thousands separators
 */
export const formatNumber = (value: number): string => {
  return value.toLocaleString();
};

/**
 * Format a number as a currency string
 * @param value - The number to format
 * @param currency - The currency code (default: 'USD')
 * @param minimumFractionDigits - Minimum number of fraction digits (default: 0)
 * @param maximumFractionDigits - Maximum number of fraction digits (default: 0)
 * @returns Formatted currency string
 */
export const formatCurrency = (
  value: number,
  currency: string = 'USD',
  minimumFractionDigits: number = 0,
  maximumFractionDigits: number = 0
): string => {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(value);
};
