"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Input, Select, SelectItem } from "@nextui-org/react";
import { BookOpen, Download, Plus } from "lucide-react";
import type { Locale } from "@/i18n/localeRegistry";
import {
  accountingApi,
  type ManualLedgerEntry,
} from "@/api/accounting";
import {
  useEntries,
  useVoidEntry,
} from "@/hooks/accounting/useAccountingQueries";
import {
  formatDay,
  formatMoney,
} from "@/components/business/accounting/format";
import { PremiumPanel } from "../premium";
import DataTable, { type DataTableColumn } from "../shared/DataTable";
import RowActionsMenu from "../shared/RowActionsMenu";
import ConfirmationModal from "../modals/ConfirmationModal";
import StatusBadge from "./StatusBadge";
import EntryDetailDrawer from "./EntryDetailDrawer";
import EntryFormDrawer from "./EntryFormDrawer";
import {
  accountingPanelClass,
  accountingPanelHeaderClass,
  accountingPanelTitleClass,
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
  formatCategoryLabel,
} from "./accountingShared";
import { allEntryCategories } from "./categories";
import RecurringTemplatesPanel from "./RecurringTemplatesPanel";
import ManageCategoriesPanel from "./ManageCategoriesPanel";
import { Paperclip } from "lucide-react";

const PAGE_SIZE = 20;
const SEARCH_DEBOUNCE_MS = 250;

type EntryTypeFilter = "all" | "income" | "expense";
type EntryStatusFilter = "all" | "active" | "voided";

export type EntriesTabProps = {
  businessId: string;
  start: string;
  end: string;
  locale: Locale;
  currency: string;
  /** When false, hide New entry / Void (write) controls. */
  canWrite?: boolean;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith?: (
    key: string,
    replacements: Record<string, string | number>,
  ) => string;
};

export default function EntriesTab({
  businessId,
  start,
  end,
  locale,
  currency,
  canWrite = true,
  t,
  tWith,
}: EntriesTabProps) {
  const [typeFilter, setTypeFilter] = useState<EntryTypeFilter>("all");
  const [categoryFilter, setCategoryFilter] = useState<string>("all");
  const [statusFilter, setStatusFilter] = useState<EntryStatusFilter>("all");
  const [searchInput, setSearchInput] = useState("");
  const [debouncedQ, setDebouncedQ] = useState("");
  const [page, setPage] = useState(1);

  const [selectedEntry, setSelectedEntry] = useState<ManualLedgerEntry | null>(
    null,
  );
  const [createOpen, setCreateOpen] = useState(false);
  const [duplicateFrom, setDuplicateFrom] =
    useState<ManualLedgerEntry | null>(null);
  const [confirmVoid, setConfirmVoid] = useState<{
    entryId: number;
    description: string;
  } | null>(null);
  // Defaults + custom keys for the category filter (mirrors EntryFormDrawer).
  const [categoryFilterItems, setCategoryFilterItems] = useState<
    { key: string; label: string }[]
  >(() =>
    allEntryCategories.map((cat) => ({
      key: cat,
      label: formatCategoryLabel(cat, t),
    })),
  );

  useEffect(() => {
    let cancelled = false;
    void accountingApi
      .listAccountingCategories(businessId)
      .then((res) => {
        if (cancelled) return;
        const defaults = (res.defaults || []).map((d) => ({
          key: d.key,
          label: d.label || formatCategoryLabel(d.key, t),
        }));
        const customs = (res.custom || [])
          .filter((c) => c.active !== false)
          .map((c) => ({
            key: c.key,
            label: c.label || c.key,
          }));
        const byKey = new Map<string, string>();
        for (const cat of allEntryCategories) {
          byKey.set(cat, formatCategoryLabel(cat, t));
        }
        for (const d of defaults) byKey.set(d.key, d.label);
        for (const c of customs) byKey.set(c.key, c.label);
        setCategoryFilterItems(
          Array.from(byKey.entries()).map(([key, label]) => ({ key, label })),
        );
      })
      .catch(() => {
        /* keep hardcoded defaults */
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, t]);

  // Debounce search into `q` (250ms).
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQ(searchInput.trim());
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Reset page when filters / period / search change.
  useEffect(() => {
    setPage(1);
  }, [typeFilter, categoryFilter, statusFilter, debouncedQ, start, end]);

  const listParams = useMemo(
    () => ({
      start,
      end,
      page,
      page_size: PAGE_SIZE,
      ...(typeFilter !== "all" ? { type: typeFilter as "income" | "expense" } : {}),
      ...(statusFilter !== "all" ? { status: statusFilter } : { status: "all" as const }),
      ...(categoryFilter !== "all" ? { category: categoryFilter } : {}),
      ...(debouncedQ ? { q: debouncedQ } : {}),
    }),
    [
      start,
      end,
      page,
      typeFilter,
      statusFilter,
      categoryFilter,
      debouncedQ,
    ],
  );

  // businessId may be a numeric id or a slug (e.g. "biz-1"); pass through as-is.
  const { data, isLoading, isFetching } = useEntries(businessId, listParams);
  const voidMutation = useVoidEntry(businessId);

  const entries = data?.entries ?? [];
  const total = data?.total ?? 0;

  const exportUrl = useMemo(
    () =>
      accountingApi.entriesExportUrl(businessId, {
        start,
        end,
        // The server localizes the streamed CSV headers and otherwise falls
        // back to Accept-Language — send the locale the dashboard is actually
        // rendering in so the file matches the screen, not the browser.
        lang: locale,
        ...(typeFilter !== "all"
          ? { type: typeFilter as "income" | "expense" }
          : {}),
        ...(statusFilter !== "all" ? { status: statusFilter } : {}),
        ...(categoryFilter !== "all" ? { category: categoryFilter } : {}),
        ...(debouncedQ ? { q: debouncedQ } : {}),
      }),
    [
      businessId,
      start,
      end,
      locale,
      typeFilter,
      statusFilter,
      categoryFilter,
      debouncedQ,
    ],
  );

  const interpolate = useCallback(
    (key: string, replacements: Record<string, string | number>) => {
      if (tWith) return tWith(key, replacements);
      let value = t(key);
      Object.entries(replacements).forEach(([name, replacement]) => {
        value = value.replace(
          new RegExp(`\\{${name}\\}`, "g"),
          String(replacement),
        );
      });
      return value;
    },
    [t, tWith],
  );

  const openCreate = useCallback(() => {
    setDuplicateFrom(null);
    setCreateOpen(true);
  }, []);

  const openDuplicate = useCallback((entry: ManualLedgerEntry) => {
    setDuplicateFrom(entry);
    setCreateOpen(true);
  }, []);

  const openDetail = useCallback((entry: ManualLedgerEntry) => {
    setSelectedEntry(entry);
  }, []);

  const handleRowAction = useCallback(
    (key: string, entry: ManualLedgerEntry) => {
      if (key === "view") {
        openDetail(entry);
        return;
      }
      if (key === "duplicate") {
        openDuplicate(entry);
        return;
      }
      if (key === "void" && !entry.voided_at && canWrite) {
        setConfirmVoid({
          entryId: entry.id,
          description: entry.description,
        });
      }
    },
    [canWrite, openDetail, openDuplicate],
  );

  const columns: DataTableColumn<ManualLedgerEntry>[] = useMemo(
    () => [
      {
        key: "date",
        header: t("entries.table.date"),
        render: (e) => formatDay(e.occurred_at, locale),
      },
      {
        key: "description",
        header: t("entries.table.description"),
        render: (e) => (
          <span
            className={`inline-flex items-center gap-1.5 ${
              e.voided_at ? "line-through text-warm-400" : ""
            }`}
          >
            {e.reference?.startsWith("recurring:") ? (
              <span className="rounded bg-brand/10 px-1 py-0.5 text-[10px] font-bold uppercase text-brand-800">
                {t("entries.autoBadge")}
              </span>
            ) : null}
            {(e.attachment_count ?? 0) > 0 ? (
              <span
                title={t("attachments.title")}
                className="inline-flex"
                data-testid="entry-attachment-icon"
              >
                <Paperclip
                  className="h-3 w-3 shrink-0 text-ink-400"
                  aria-hidden
                />
              </span>
            ) : null}
            {e.description}
          </span>
        ),
      },
      {
        key: "category",
        header: t("entries.table.category"),
        hideBelow: "md",
        render: (e) => formatCategoryLabel(e.category, t),
      },
      {
        key: "type",
        header: t("entries.table.type"),
        hideBelow: "sm",
        render: (e) => (
          <StatusBadge
            tone={e.entry_type === "income" ? "success" : "neutral"}
            label={
              e.entry_type === "income"
                ? t("entries.income")
                : t("entries.expense")
            }
            size="sm"
          />
        ),
      },
      {
        key: "amount",
        header: t("entries.table.amount"),
        align: "right",
        render: (e) => (
          <span
            className={
              e.entry_type === "expense" ? "text-warm-700" : "text-brand-700"
            }
          >
            {formatMoney(e.amount, e.currency || currency, locale)}
          </span>
        ),
      },
      {
        key: "status",
        header: t("entries.table.status"),
        render: (e) => (
          <StatusBadge
            tone={e.voided_at ? "neutral" : "success"}
            label={
              e.voided_at
                ? t("entries.status.voided")
                : t("entries.status.active")
            }
            size="sm"
          />
        ),
      },
    ],
    [t, locale, currency],
  );

  const selectClassNames = {
    trigger:
      "h-8 min-h-8 bg-white border-warm-200 data-[hover=true]:border-brand/40",
    value: "text-sm",
  };

  return (
    <>
      <RecurringTemplatesPanel
        businessId={businessId}
        currency={currency}
        t={t}
      />
      <ManageCategoriesPanel businessId={businessId} t={t} />
      <PremiumPanel className={accountingPanelClass} withTexture={false}>
        <div className={accountingPanelHeaderClass}>
          <h3 className={accountingPanelTitleClass}>{t("entries.title")}</h3>
          <div className="flex flex-wrap items-center gap-2">
            <a
              href={exportUrl}
              className={accountingSecondaryButtonClass}
              download
            >
              <Download className="h-4 w-4" aria-hidden />
              {t("entries.exportCsv")}
            </a>
            {canWrite ? (
              <button
                type="button"
                className={accountingPrimaryButtonClass}
                onClick={openCreate}
              >
                <Plus className="h-4 w-4" aria-hidden />
                {t("entries.newEntry")}
              </button>
            ) : null}
          </div>
        </div>

        <div className="flex flex-wrap items-end gap-2 border-b border-warm-200/80 px-4 py-3 sm:px-5">
          <Select
            aria-label={t("entries.filters.type")}
            label={t("entries.filters.type")}
            labelPlacement="outside"
            size="sm"
            variant="bordered"
            selectedKeys={[typeFilter]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0] as EntryTypeFilter | undefined;
              if (next) setTypeFilter(next);
            }}
            className="w-36"
            classNames={selectClassNames}
          >
            <SelectItem key="all" textValue={t("entries.filters.statusAll")}>
              {t("entries.filters.statusAll")}
            </SelectItem>
            <SelectItem key="income" textValue={t("entries.income")}>
              {t("entries.income")}
            </SelectItem>
            <SelectItem key="expense" textValue={t("entries.expense")}>
              {t("entries.expense")}
            </SelectItem>
          </Select>

          <Select
            aria-label={t("entries.filters.category")}
            label={t("entries.filters.category")}
            labelPlacement="outside"
            size="sm"
            variant="bordered"
            selectedKeys={[categoryFilter]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0] as string | undefined;
              if (next) setCategoryFilter(next);
            }}
            className="w-44"
            classNames={selectClassNames}
            data-testid="entries-category-filter"
            items={[
              { key: "all", label: t("entries.filters.statusAll") },
              ...categoryFilterItems,
            ]}
          >
            {(item) => (
              <SelectItem key={item.key} textValue={item.label}>
                {item.label}
              </SelectItem>
            )}
          </Select>

          <Select
            aria-label={t("entries.filters.status")}
            label={t("entries.filters.status")}
            labelPlacement="outside"
            size="sm"
            variant="bordered"
            selectedKeys={[statusFilter]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0] as EntryStatusFilter | undefined;
              if (next) setStatusFilter(next);
            }}
            className="w-36"
            classNames={selectClassNames}
            data-testid="entries-status-filter"
          >
            <SelectItem key="all" textValue={t("entries.filters.statusAll")}>
              {t("entries.filters.statusAll")}
            </SelectItem>
            <SelectItem
              key="active"
              textValue={t("entries.filters.statusActive")}
            >
              {t("entries.filters.statusActive")}
            </SelectItem>
            <SelectItem
              key="voided"
              textValue={t("entries.filters.statusVoided")}
            >
              {t("entries.filters.statusVoided")}
            </SelectItem>
          </Select>

          <Input
            aria-label={t("entries.filters.searchPlaceholder")}
            placeholder={t("entries.filters.searchPlaceholder")}
            size="sm"
            variant="bordered"
            value={searchInput}
            onValueChange={setSearchInput}
            className="w-full min-w-[10rem] flex-1 sm:max-w-xs"
            classNames={{
              inputWrapper:
                "h-8 min-h-8 bg-white border-warm-200 data-[hover=true]:border-brand/40",
            }}
          />
        </div>

        <div className="px-2 py-3 sm:px-4">
          <DataTable<ManualLedgerEntry>
            aria-label={t("entries.title")}
            columns={columns}
            rows={entries}
            rowKey={(row) => row.id}
            loading={isLoading || (isFetching && entries.length === 0)}
            emptyState={{
              icon: BookOpen,
              title: t("entries.empty.title"),
              subtitle: t("entries.empty.subtitle"),
            }}
            pagination={{
              page,
              pageSize: PAGE_SIZE,
              total,
              onPageChange: setPage,
            }}
            onRowClick={openDetail}
            rowActions={(row) => (
              <RowActionsMenu
                aria-label={`entry-actions-${row.id}`}
                items={[
                  { key: "view", label: t("entries.menu.view") },
                  { key: "duplicate", label: t("entries.menu.duplicate") },
                  {
                    key: "void",
                    label: t("entries.menu.void"),
                    tone: "danger",
                    disabled: Boolean(row.voided_at) || !canWrite,
                  },
                ]}
                onAction={(key) => handleRowAction(key, row)}
              />
            )}
          />
        </div>
      </PremiumPanel>

      <EntryDetailDrawer
        open={!!selectedEntry}
        entry={selectedEntry}
        businessId={businessId}
        locale={locale}
        currency={currency}
        canWrite={canWrite}
        onClose={() => setSelectedEntry(null)}
        onDuplicate={(entry) => {
          setSelectedEntry(null);
          openDuplicate(entry);
        }}
        t={t}
        tWith={interpolate}
      />

      <EntryFormDrawer
        open={createOpen}
        onClose={() => {
          setCreateOpen(false);
          setDuplicateFrom(null);
        }}
        businessId={businessId}
        currency={currency}
        initial={duplicateFrom}
        t={t}
      />

      {/* Table row-menu void confirm (detail drawer owns its own confirm). */}
      <ConfirmationModal
        isOpen={!!confirmVoid}
        onOpenChange={() => setConfirmVoid(null)}
        title={t("entries.confirmVoidTitle")}
        description={interpolate("entries.confirmVoidBody", {
          description: confirmVoid?.description ?? "",
        })}
        confirmLabel={t("entries.void")}
        onConfirm={() => {
          if (!confirmVoid) return;
          const { entryId } = confirmVoid;
          setConfirmVoid(null);
          voidMutation.mutate(entryId);
        }}
        isDanger
      />
    </>
  );
}
