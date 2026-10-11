/** Manual ledger income categories (create form + filters). */
export const incomeCategories = [
  "off_platform_sale",
  "catering",
  "event",
  "service",
  "adjustment",
  "other",
] as const;

/** Manual ledger expense categories (create form + filters). */
export const expenseCategories = [
  "rent",
  "utilities",
  "supplies",
  "inventory",
  "marketing",
  "software",
  "maintenance",
  "logistics",
  "tax",
  "other",
] as const;

/** Union of income + expense category keys for filter dropdowns (deduped). */
export const allEntryCategories: string[] = Array.from(
  new Set<string>([...incomeCategories, ...expenseCategories]),
);
