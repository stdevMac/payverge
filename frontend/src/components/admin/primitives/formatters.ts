export function formatAdminDate(
  dateStr: string | null | undefined,
  withTime = false,
): string {
  if (!dateStr) return "N/A";
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return dateStr;
  return date.toLocaleString(
    "en-US",
    withTime
      ? {
          year: "numeric",
          month: "short",
          day: "numeric",
          hour: "2-digit",
          minute: "2-digit",
        }
      : { year: "numeric", month: "short", day: "numeric" },
  );
}

/** The admin lifecycle a business row can be in. */
export type AdminLifecycleStatus = "active" | "suspended" | "closed";

const ADMIN_LIFECYCLE_LABELS: Record<AdminLifecycleStatus, string> = {
  active: "Active",
  suspended: "Suspended",
  closed: "Closed",
};

export function formatAdminLifecycleStatus(
  status: string | null | undefined,
): string {
  const normalized = status?.trim().toLowerCase() ?? "";
  return (
    ADMIN_LIFECYCLE_LABELS[normalized as AdminLifecycleStatus] ??
    (status || "N/A")
  );
}

/** Chip colors keyed by the admin lifecycle status. */
export const ADMIN_LIFECYCLE_STATUS_COLORS: Record<
  string,
  "success" | "warning" | "danger" | "default"
> = {
  active: "success",
  suspended: "warning",
  closed: "danger",
};

/** Status filter options; "" is the all sentinel. */
export const ADMIN_LIFECYCLE_STATUS_FILTER_OPTIONS: {
  key: "" | AdminLifecycleStatus;
  label: string;
}[] = [
  { key: "", label: "All statuses" },
  { key: "active", label: "Active" },
  { key: "suspended", label: "Suspended" },
  { key: "closed", label: "Closed" },
];
