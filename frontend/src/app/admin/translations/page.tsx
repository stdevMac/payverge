"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Spinner,
  Chip,
  Select,
  SelectItem,
} from "@nextui-org/react";
import { RefreshCw, Languages } from "lucide-react";
import {
  getMissingTranslations,
  updateMissingTranslationStatus,
  type MissingTranslationRow,
  type MissingTranslationStatus,
} from "@/api/adminMissingTranslations";
import { AdminPageFrame } from "@/components/admin/primitives";
import { localeRegistry } from "@/i18n/localeRegistry";

const PAGE_SIZE = 100;
/** Open `en` reports older than this are treated as a default-locale bug. */
const EN_STALE_MS = 48 * 60 * 60 * 1000;
const STATUSES: Array<{
  key: MissingTranslationStatus;
  label: string;
}> = [
  { key: "open", label: "Open" },
  { key: "resolved", label: "Resolved" },
  { key: "ignored", label: "Ignored" },
];
const absoluteDateFormatter = new Intl.DateTimeFormat("en", {
  dateStyle: "medium",
  timeStyle: "short",
});
const relativeTimeFormatter = new Intl.RelativeTimeFormat("en", {
  numeric: "auto",
});
const LOCALES = Object.keys(localeRegistry).sort((a, b) => {
  if (a === "en") return -1;
  if (b === "en") return 1;
  if (a === "es") return -1;
  if (b === "es") return 1;
  if (a === "es-AR") return -1;
  if (b === "es-AR") return 1;
  return a.localeCompare(b);
});

function formatLastSeen(value: string): { relative: string; absolute: string } {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) {
    return { relative: "at an unknown time", absolute: "Unknown date" };
  }

  const elapsedSeconds = Math.max(
    0,
    Math.floor((Date.now() - timestamp) / 1000),
  );
  if (elapsedSeconds < 60) {
    return {
      relative: "just now",
      absolute: absoluteDateFormatter.format(timestamp),
    };
  }

  const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ["year", 365 * 24 * 60 * 60],
    ["month", 30 * 24 * 60 * 60],
    ["day", 24 * 60 * 60],
    ["hour", 60 * 60],
    ["minute", 60],
  ];
  const [unit, secondsPerUnit] =
    units.find(([, seconds]) => elapsedSeconds >= seconds) ?? units.at(-1)!;
  const amount = Math.floor(elapsedSeconds / secondsPerUnit);

  return {
    relative: relativeTimeFormatter.format(-amount, unit),
    absolute: absoluteDateFormatter.format(timestamp),
  };
}

function lifecycleActionLabel(
  action: "Resolve" | "Ignore" | "Reopen",
  row: MissingTranslationRow,
): string {
  return `${action} ${row.key_path} for ${row.locale} on ${row.page || "unknown page"} (${row.fallback_used} fallback, report ${row.id})`;
}

export default function AdminTranslationsPage() {
  const [rows, setRows] = useState<MissingTranslationRow[]>([]);
  const [total, setTotal] = useState(0);
  const [locale, setLocale] = useState("");
  const [status, setStatus] = useState<MissingTranslationStatus>("open");
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [updatingStatuses, setUpdatingStatuses] = useState<
    Record<number, MissingTranslationStatus>
  >({});
  const [mutationErrors, setMutationErrors] = useState<Record<number, string>>(
    {},
  );
  const filtersRef = useRef({ page, locale, status });
  const loadRequestIdRef = useRef(0);
  filtersRef.current = { page, locale, status };

  const load = useCallback(async (background = false) => {
    const requestId = loadRequestIdRef.current + 1;
    loadRequestIdRef.current = requestId;
    const filters = filtersRef.current;
    setLoading(!background);
    setLoadError(null);
    try {
      const data = await getMissingTranslations({
        limit: PAGE_SIZE,
        offset: filters.page * PAGE_SIZE,
        locale: filters.locale || undefined,
        status: filters.status,
      });
      if (requestId !== loadRequestIdRef.current) return;
      const lastValidPage = Math.max(0, Math.ceil(data.total / PAGE_SIZE) - 1);
      if (filters.page > lastValidPage) {
        filtersRef.current = { ...filters, page: lastValidPage };
        setPage(lastValidPage);
        return;
      }
      setRows(data.rows);
      setTotal(data.total);
    } catch {
      if (requestId !== loadRequestIdRef.current) return;
      setRows([]);
      setTotal(0);
      setLoadError("Failed to load missing translation reports.");
    } finally {
      if (requestId === loadRequestIdRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, locale, page, status]);

  const changeStatus = useCallback(
    async (rowId: number, nextStatus: MissingTranslationStatus) => {
      setUpdatingStatuses((current) => ({
        ...current,
        [rowId]: nextStatus,
      }));
      setMutationErrors((current) => {
        const next = { ...current };
        delete next[rowId];
        return next;
      });
      try {
        await updateMissingTranslationStatus(rowId, nextStatus);
        await load(true);
      } catch {
        setMutationErrors((current) => ({
          ...current,
          [rowId]: "Failed to update report status. Try again.",
        }));
      } finally {
        setUpdatingStatuses((current) => {
          const next = { ...current };
          delete next[rowId];
          return next;
        });
      }
    },
    [load],
  );

  const totalPages = useMemo(
    () => Math.max(1, Math.ceil(total / PAGE_SIZE)),
    [total],
  );
  const reportLabel = total === 1 ? "report" : "reports";

  // Task 35: a missing string in the default locale (`en`) open >48h is a bug,
  // not a translation backlog item. Surface a persistent banner so admins
  // can't miss it.
  const staleEnOpen = useMemo(() => {
    const now = Date.now();
    return rows.filter((row) => {
      if (row.status !== "open") return false;
      if (row.locale !== "en") return false;
      const firstSeen = Date.parse(row.first_seen_at);
      if (!Number.isFinite(firstSeen)) return false;
      return now - firstSeen >= EN_STALE_MS;
    });
  }, [rows]);

  return (
    <AdminPageFrame
      description={`Missing i18n fallback reports from guest/operator telemetry — ${total} ${reportLabel}`}
      actions={
        <Button
          variant="bordered"
          onPress={() => void load()}
          isLoading={loading}
          startContent={<RefreshCw size={16} />}
        >
          Refresh
        </Button>
      }
    >
      {staleEnOpen.length > 0 ? (
        <div
          role="alert"
          className="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-950"
        >
          <p className="font-semibold">
            {staleEnOpen.length === 1
              ? "1 English missing-translation report has been open for more than 48 hours"
              : `${staleEnOpen.length} English missing-translation reports have been open for more than 48 hours`}
          </p>
          <p className="mt-1 text-amber-900/90">
            A missing key in the default locale is a product bug — resolve or
            ignore these before they spread to every other language.
          </p>
          <ul className="mt-2 list-disc space-y-0.5 pl-5 font-mono text-xs text-amber-950/90">
            {staleEnOpen.slice(0, 5).map((row) => (
              <li key={row.id}>
                {row.key_path}
                {row.page ? ` · ${row.page}` : ""}
              </li>
            ))}
            {staleEnOpen.length > 5 ? (
              <li>+{staleEnOpen.length - 5} more on this page</li>
            ) : null}
          </ul>
        </div>
      ) : null}
      <Card>
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <div className="flex items-center gap-2">
            <Languages size={18} className="text-brand" />
            <p className="font-semibold">Missing translations</p>
          </div>
          <div className="grid w-full gap-3 sm:ml-auto sm:max-w-xl sm:grid-cols-2">
            <Select
              label="Status filter"
              selectedKeys={[status]}
              onChange={(e) => {
                setStatus(e.target.value as MissingTranslationStatus);
                setPage(0);
                setMutationErrors({});
              }}
              size="sm"
            >
              {STATUSES.map((option) => (
                <SelectItem key={option.key} value={option.key}>
                  {option.label}
                </SelectItem>
              ))}
            </Select>
            <Select
              label="Locale filter"
              selectedKeys={[locale || "all"]}
              onChange={(e) => {
                setLocale(e.target.value === "all" ? "" : e.target.value);
                setPage(0);
                setMutationErrors({});
              }}
              size="sm"
            >
              {["all", ...LOCALES].map((loc) => (
                <SelectItem key={loc} value={loc}>
                  {loc === "all" ? "All locales" : loc}
                </SelectItem>
              ))}
            </Select>
          </div>
        </CardHeader>
        <CardBody>
          {loadError ? (
            <p className="text-danger text-center py-8 text-sm">{loadError}</p>
          ) : loading ? (
            <div className="flex justify-center py-12">
              <Spinner />
            </div>
          ) : rows.length === 0 ? (
            <p className="text-default-500 text-center py-8">
              No missing keys reported.
            </p>
          ) : (
            <div className="space-y-2">
              {rows.map((row) => {
                const lastSeen = formatLastSeen(row.last_seen_at);
                const updatingStatus = updatingStatuses[row.id];
                const isUpdating = Boolean(updatingStatus);
                const mutationError = mutationErrors[row.id];
                const isStaleEn =
                  row.status === "open" &&
                  row.locale === "en" &&
                  Number.isFinite(Date.parse(row.first_seen_at)) &&
                  Date.now() - Date.parse(row.first_seen_at) >= EN_STALE_MS;
                return (
                  <article
                    key={row.id}
                    aria-label={`Missing translation ${row.key_path}`}
                    className={`rounded-xl border bg-white p-4 shadow-sm shadow-warm-900/5 ${
                      isStaleEn
                        ? "border-amber-300 ring-1 ring-amber-200"
                        : "border-warm-200"
                    }`}
                  >
                    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                      <div className="min-w-0">
                        <p className="truncate font-mono text-sm font-medium text-ink-950">
                          {row.key_path}
                        </p>
                        <p className="mt-1 text-xs text-ink-500">
                          {row.locale} · {row.fallback_used}
                          {row.page ? ` · ${row.page}` : ""}
                          {isStaleEn ? " · open >48h" : ""}
                        </p>
                        <time
                          dateTime={row.last_seen_at}
                          className="mt-1 block text-xs text-ink-500"
                          title={lastSeen.absolute}
                        >
                          Last seen {lastSeen.relative} · {lastSeen.absolute}
                        </time>
                      </div>
                      <div className="flex flex-wrap items-center gap-2 sm:justify-end">
                        <Chip size="sm" variant="flat">
                          {row.hit_count} hits
                        </Chip>
                        {row.status === "open" ? (
                          <>
                            <Button
                              aria-label={lifecycleActionLabel("Resolve", row)}
                              size="sm"
                              color="primary"
                              variant="flat"
                              isLoading={updatingStatus === "resolved"}
                              isDisabled={isUpdating}
                              onPress={() =>
                                void changeStatus(row.id, "resolved")
                              }
                            >
                              Resolve
                            </Button>
                            <Button
                              aria-label={lifecycleActionLabel("Ignore", row)}
                              size="sm"
                              variant="bordered"
                              isLoading={updatingStatus === "ignored"}
                              isDisabled={isUpdating}
                              onPress={() =>
                                void changeStatus(row.id, "ignored")
                              }
                            >
                              Ignore
                            </Button>
                          </>
                        ) : (
                          <Button
                            aria-label={lifecycleActionLabel("Reopen", row)}
                            size="sm"
                            color="primary"
                            variant="flat"
                            isLoading={updatingStatus === "open"}
                            isDisabled={isUpdating}
                            onPress={() => void changeStatus(row.id, "open")}
                          >
                            Reopen
                          </Button>
                        )}
                      </div>
                    </div>
                    {mutationError ? (
                      <p
                        role="alert"
                        className="mt-3 rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-700"
                      >
                        {mutationError}
                      </p>
                    ) : null}
                  </article>
                );
              })}
              <div className="flex flex-col gap-2 pt-3 sm:flex-row sm:items-center sm:justify-between">
                <p className="text-xs text-default-500">
                  Page {page + 1} of {totalPages}
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="bordered"
                    isDisabled={page === 0 || loading}
                    onPress={() =>
                      setPage((current) => Math.max(0, current - 1))
                    }
                  >
                    Previous
                  </Button>
                  <Button
                    size="sm"
                    variant="bordered"
                    isDisabled={page + 1 >= totalPages || loading}
                    onPress={() =>
                      setPage((current) =>
                        Math.min(totalPages - 1, current + 1),
                      )
                    }
                  >
                    Next
                  </Button>
                </div>
              </div>
            </div>
          )}
        </CardBody>
      </Card>
    </AdminPageFrame>
  );
}
