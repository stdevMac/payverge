/**
 * Calculates and formats the progress percentage based on current and total values
 * @param currentValue - The current value (e.g., amount funded)
 * @param totalValue - The total target value (e.g., total funding goal)
 * @returns Formatted progress percentage as number
 */
export const calculateProgress = (
  currentValue: number,
  totalValue: number
): number => {
  if (totalValue === 0) return 0;
  return ((currentValue - totalValue) / totalValue) * 100;
};

export const formatPercentage = (progress: number): string => {
  return progress > 0 && progress < 0.01 ? "<0.01" : progress.toFixed(2);
};
