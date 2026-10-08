"use client";

import React, { useCallback } from "react";
import type { LucideIcon } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonLine } from "@/components/ui/skeletons";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import type { Locale } from "@/i18n/localeRegistry";
import { tableBody, tableBodyCell, tableRoot } from "./tableStyles";

export interface DataTableColumn<T> {
  key: string;
  header: string;
  align?: "left" | "right";
  hideBelow?: "sm" | "md" | "lg";
  render: (row: T) => React.ReactNode;
}

export interface DataTablePagination {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
}

export interface DataTableProps<T> {
  columns: DataTableColumn<T>[];
  rows: T[];
  rowKey: (row: T) => string | number;
  loading?: boolean;
  emptyState: { icon: LucideIcon; title: string; subtitle?: string };
  pagination?: DataTablePagination;
  onRowClick?: (row: T) => void;
  rowActions?: (row: T) => React.ReactNode;
  rowActionsLabel?: string;
  "aria-label": string;
}

const HIDE_BELOW_CLASS: Record<NonNullable<DataTableColumn<unknown>["hideBelow"]>, string> = {
  sm: "hidden sm:table-cell",
  md: "hidden md:table-cell",
  lg: "hidden lg:table-cell",
};

const headerCellClass =
  "px-3 py-2 text-[11px] font-semibold uppercase tracking-wide text-warm-500";

function columnAlignClass(align?: "left" | "right"): string {
  return align === "right" ? "text-right tabular-nums" : "";
}

function columnVisibilityClass(
  hideBelow?: DataTableColumn<unknown>["hideBelow"],
): string {
  return hideBelow ? HIDE_BELOW_CLASS[hideBelow] : "";
}

function joinClasses(...parts: Array<string | false | undefined | null>): string {
  return parts.filter(Boolean).join(" ");
}

export default function DataTable<T>({
  columns,
  rows,
  rowKey,
  loading = false,
  emptyState,
  pagination,
  onRowClick,
  rowActions,
  rowActionsLabel,
  "aria-label": ariaLabel,
}: DataTableProps<T>) {
  const { locale } = useSimpleLocale();
  const tPagination = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const fullKey = `common.pagination.${key}`;
      const result = getTranslation(fullKey, locale as Locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  if (!loading && rows.length === 0) {
    return (
      <EmptyState
        panel
        compact
        icon={emptyState.icon}
        title={emptyState.title}
        subtitle={emptyState.subtitle ?? ""}
      />
    );
  }

  const pageSize = pagination?.pageSize ?? 5;
  const colCount = columns.length + (rowActions ? 1 : 0);
  const showPagination =
    Boolean(pagination) && pagination!.total > pagination!.pageSize;

  const rangeStart =
    pagination && pagination.total > 0
      ? (pagination.page - 1) * pagination.pageSize + 1
      : 0;
  const rangeEnd = pagination
    ? Math.min(pagination.page * pagination.pageSize, pagination.total)
    : 0;
  const isFirstPage = !pagination || pagination.page <= 1;
  const isLastPage =
    !pagination || rangeEnd >= pagination.total || pagination.total === 0;

  const handleRowKeyDown = (
    event: React.KeyboardEvent<HTMLTableRowElement>,
    row: T,
  ) => {
    if (!onRowClick) return;
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onRowClick(row);
    }
  };

  return (
    <div>
      <div className="overflow-x-auto">
        <table className={tableRoot} aria-label={ariaLabel}>
          <caption className="sr-only">{ariaLabel}</caption>
          <thead>
            <tr>
              {columns.map((col) => (
                <th
                  key={col.key}
                  scope="col"
                  className={joinClasses(
                    headerCellClass,
                    columnAlignClass(col.align),
                    columnVisibilityClass(col.hideBelow),
                  )}
                >
                  {col.header}
                </th>
              ))}
              {rowActions ? (
                <th scope="col" className={joinClasses(headerCellClass, "w-12")}>
                  <span className="sr-only">{rowActionsLabel ?? ariaLabel}</span>
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody className={tableBody}>
            {loading
              ? Array.from({ length: pageSize }, (_, rowIndex) => (
                  <tr key={`skeleton-${rowIndex}`}>
                    {Array.from({ length: colCount }, (_, colIndex) => (
                      <td
                        key={`skeleton-${rowIndex}-${colIndex}`}
                        className={tableBodyCell}
                      >
                        <SkeletonLine
                          width={colIndex === 0 ? "70%" : "50%"}
                          height="0.875rem"
                        />
                      </td>
                    ))}
                  </tr>
                ))
              : rows.map((row) => (
                  <tr
                    key={rowKey(row)}
                    className={joinClasses(
                      "hover:bg-warm-50/60",
                      onRowClick && "cursor-pointer",
                    )}
                    onClick={onRowClick ? () => onRowClick(row) : undefined}
                    onKeyDown={
                      onRowClick ? (event) => handleRowKeyDown(event, row) : undefined
                    }
                    tabIndex={onRowClick ? 0 : undefined}
                  >
                    {columns.map((col) => (
                      <td
                        key={col.key}
                        className={joinClasses(
                          tableBodyCell,
                          "text-ink-800",
                          columnAlignClass(col.align),
                          columnVisibilityClass(col.hideBelow),
                        )}
                      >
                        {col.render(row)}
                      </td>
                    ))}
                    {rowActions ? (
                      <td
                        className={joinClasses(tableBodyCell, "text-ink-800")}
                        onClick={(event) => event.stopPropagation()}
                      >
                        {rowActions(row)}
                      </td>
                    ) : null}
                  </tr>
                ))}
          </tbody>
        </table>
      </div>

      {showPagination && pagination ? (
        <div className="mt-3 flex items-center justify-between gap-3 px-1">
          <p className="text-xs tabular-nums text-warm-600">
            {tPagination("range", {
              start: rangeStart,
              end: rangeEnd,
              total: pagination.total,
            })}
          </p>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className="rounded-full border border-warm-200 bg-white px-3 py-1.5 text-xs font-medium text-ink-600 transition-colors hover:bg-warm-50 disabled:cursor-not-allowed disabled:opacity-40"
              disabled={isFirstPage}
              onClick={() => pagination.onPageChange(pagination.page - 1)}
              aria-label={tPagination("previousPageAria")}
            >
              {tPagination("previous")}
            </button>
            <button
              type="button"
              className="rounded-full border border-warm-200 bg-white px-3 py-1.5 text-xs font-medium text-ink-600 transition-colors hover:bg-warm-50 disabled:cursor-not-allowed disabled:opacity-40"
              disabled={isLastPage}
              onClick={() => pagination.onPageChange(pagination.page + 1)}
              aria-label={tPagination("nextPageAria")}
            >
              {tPagination("next")}
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
