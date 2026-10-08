import React from "react";
import { Chip } from "@nextui-org/react";

export type FilterStatus =
  | "all"
  | "pending"
  | "confirmed"
  | "preparing"
  | "ready"
  | "assigned"
  | "picked_up"
  | "in_transit"
  | "nearby"
  | "delivered"
  | "cancelled"
  | "failed";

interface StatusFilterProps {
  active: FilterStatus;
  onChange: (status: FilterStatus) => void;
  tString: (key: string, vars?: Record<string, string>) => string;
}

const FILTER_OPTIONS: FilterStatus[] = [
  "all",
  "pending",
  "preparing",
  "ready",
  "in_transit",
  "delivered",
  "cancelled",
  "failed",
];

const STATUS_TO_LABEL_KEY: Record<FilterStatus, string> = {
  all: "dispatch.filters.all",
  pending: "dispatch.filters.pending",
  confirmed: "dispatch.filters.pending",
  preparing: "dispatch.filters.preparing",
  ready: "dispatch.filters.ready",
  assigned: "dispatch.filters.ready",
  picked_up: "dispatch.filters.in_transit",
  in_transit: "dispatch.filters.in_transit",
  nearby: "dispatch.filters.in_transit",
  delivered: "dispatch.filters.delivered",
  cancelled: "dispatch.filters.cancelled",
  failed: "dispatch.filters.failed",
};

export function StatusFilter({ active, onChange, tString }: StatusFilterProps) {
  return (
    <div
      className="flex flex-wrap gap-2"
      role="tablist"
      aria-label={tString("dispatch.filters.label")}
    >
      {FILTER_OPTIONS.map((status) => {
        const isActive = active === status;
        const label = tString(STATUS_TO_LABEL_KEY[status]);
        return (
          <button
            key={status}
            role="tab"
            onClick={() => onChange(status)}
            className="focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-1 focus-visible:ring-primary rounded-full"
            aria-selected={isActive}
            aria-label={tString("dispatch.filters.filterByAria", { status: label })}
          >
            <Chip
              variant={isActive ? "solid" : "flat"}
              color={isActive ? "primary" : "default"}
              classNames={{
                base: "cursor-pointer transition-colors",
              }}
            >
              {label}
            </Chip>
          </button>
        );
      })}
    </div>
  );
}
