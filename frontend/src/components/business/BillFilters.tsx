import React, { useState } from "react";
import {
  Input,
  Select,
  SelectItem,
  Button,
  DatePicker,
} from "@nextui-org/react";
import { parseDate } from "@internationalized/date";
import { Search, RotateCcw } from "lucide-react";

interface BillFiltersProps {
  searchQuery: string;
  onSearchChange: (value: string) => void;
  dateFrom: string;
  onDateFromChange: (value: string) => void;
  dateTo: string;
  onDateToChange: (value: string) => void;
  statusFilter?: "all" | "paid" | "closed" | "voided";
  onStatusFilterChange?: (value: "all" | "paid" | "closed" | "voided") => void;
  onReset: () => void;
  tString: (key: string) => string;
  showStatusFilter?: boolean;
  /** Hide idle date pickers below `sm` so Active Bills stay on the first screen. */
  collapseIdleDates?: boolean;
}

export const BillFilters: React.FC<BillFiltersProps> = ({
  searchQuery,
  onSearchChange,
  dateFrom,
  onDateFromChange,
  dateTo,
  onDateToChange,
  statusFilter,
  onStatusFilterChange,
  onReset,
  tString,
  showStatusFilter = false,
  collapseIdleDates = false,
}) => {
  const datesAreIdle = !dateFrom && !dateTo;
  const [datesExpanded, setDatesExpanded] = useState(false);
  const hideIdleDatesOnMobile =
    collapseIdleDates && datesAreIdle && !datesExpanded;
  return (
    <div className="mb-6 rounded-2xl border border-warm-200/80 bg-warm-50/80 p-4 shadow-[0_18px_42px_rgba(46,42,37,0.05)]">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-center">
        {/* Search Input */}
        <div className="min-w-0 flex-1">
          <Input
            value={searchQuery}
            onValueChange={onSearchChange}
            placeholder={
              showStatusFilter
                ? tString("search.billHistoryPlaceholder")
                : tString("search.activeBillsPlaceholder")
            }
            startContent={<Search className="h-4 w-4 text-ink-400" />}
            size="sm"
            className="w-full"
            classNames={{
              input: "text-sm",
              inputWrapper:
                "border border-warm-200/90 bg-white shadow-sm hover:border-brand/35 data-[focus=true]:border-brand data-[focus=true]:ring-2 data-[focus=true]:ring-brand/15",
            }}
          />
        </div>

        {/* Status Filter (for history tab) */}
        {showStatusFilter && statusFilter && onStatusFilterChange && (
          <div
            className="w-full shrink-0 lg:w-52"
            data-testid="bill-history-status-filter"
          >
            <Select
              selectedKeys={[statusFilter]}
              onChange={(e) =>
                onStatusFilterChange((e.target.value as any) || "all")
              }
              size="sm"
              aria-label={tString("filters.filterByStatusAria")}
              // Single collection array (not static SelectItem siblings) so
              // NextUI's typed items never trip TS2322 when the option set grows.
              items={[
                { key: "all", label: tString("allStatuses") },
                { key: "paid", label: tString("paid") },
                { key: "closed", label: tString("closed") },
                // Voided bills are real money reversals — keep them reachable
                // from the history filter (they were previously invisible in
                // every list). StatusChip already renders them in danger tone.
                { key: "voided", label: tString("voided") },
              ]}
              classNames={{
                trigger:
                  "border border-warm-200/90 bg-white shadow-sm hover:border-brand/35 data-[open=true]:border-brand min-w-[11rem]",
                value: "text-sm truncate",
              }}
            >
              {(item) => <SelectItem key={item.key}>{item.label}</SelectItem>}
            </Select>
          </div>
        )}

        {hideIdleDatesOnMobile ? (
          <button
            type="button"
            className="rounded-full px-3 py-1.5 text-sm font-medium text-ink-600 sm:hidden"
            onClick={() => setDatesExpanded(true)}
            data-testid="bill-filters-dates-toggle"
          >
            {tString("filters.fromDate")} {tString("filters.to")}{" "}
            {tString("filters.toDate")}
          </button>
        ) : null}

        {/* Date Range */}
        <div
          data-testid="bill-filters-dates"
          className={
            hideIdleDatesOnMobile
              ? "hidden flex-col gap-3 sm:flex sm:flex-row sm:items-center"
              : "flex flex-col gap-3 sm:flex-row sm:items-center"
          }
        >
          <DatePicker
            aria-label={tString("filters.fromDateAria")}
            label={tString("filters.fromDate")}
            value={dateFrom ? parseDate(dateFrom) : null}
            maxValue={dateTo ? parseDate(dateTo) : undefined}
            onChange={(value) => onDateFromChange(value?.toString() ?? "")}
            granularity="day"
            size="sm"
            className="w-full sm:w-44"
          />
          <span className="px-2 text-sm font-medium text-ink-500">
            {tString("filters.to")}
          </span>
          <DatePicker
            aria-label={tString("filters.toDateAria")}
            label={tString("filters.toDate")}
            value={dateTo ? parseDate(dateTo) : null}
            minValue={dateFrom ? parseDate(dateFrom) : undefined}
            onChange={(value) => onDateToChange(value?.toString() ?? "")}
            granularity="day"
            size="sm"
            className="w-full sm:w-44"
          />
        </div>

        {/* Reset Button */}
        <Button
          variant="light"
          size="sm"
          onPress={onReset}
          radius="full"
          startContent={<RotateCcw className="h-4 w-4" />}
          className="font-medium text-ink-600 hover:bg-white hover:text-ink-900"
        >
          {tString("reset")}
        </Button>
      </div>
    </div>
  );
};
