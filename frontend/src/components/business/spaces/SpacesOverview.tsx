"use client";

import React, {
  Suspense,
  lazy,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Button,
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
  Input,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from "@nextui-org/react";
import {
  Archive,
  ArrowDown,
  ArrowUp,
  Copy,
  LayoutGrid,
  MoreHorizontal,
  Pencil,
  Plus,
  RefreshCw,
  Smartphone,
  Trash2,
} from "lucide-react";
import toast from "react-hot-toast";
import {
  computeSpacesOverviewKpis,
  type ExistingTableLike,
} from "./spacesOverviewKpis";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import { getBusinessTables } from "@/api/business";
import {
  spacesApi,
  type CreateSpaceInput,
  type Space,
  type SpaceSummary,
  type SpaceTableRef,
} from "@/api/spaces";
import { spaceCardStats } from "./spaceCardStats";
import { PremiumPanel } from "../premium";
import { EmptyState } from "@/components/ui/EmptyState";
import ConfirmationModal from "../modals/ConfirmationModal";
import { btnGhostIcon, btnPrimary, btnSecondary } from "@/components/ui/buttonStyles";
import CreateSpaceModal, {
  type CreateSpacePath,
} from "./CreateSpaceModal";
import { LayoutPreviewSvg } from "./layoutPreviewSvg";
import SpaceScanSessionModal from "./scan/SpaceScanSessionModal";
import UnassignedTablesPanel from "./UnassignedTablesPanel";

const SpaceEditorPage = lazy(() => import("./editor/SpaceEditorPage"));

export interface SpacesOverviewProps {
  businessId: number;
  /**
   * Deep-link space id (?spaceId=). When set after load, notifies parent via
   * onOpenSpace so a future editor shell can take over.
   */
  initialSpaceId?: number | null;
  /** Operator opened a space (Open / Edit / post-create). */
  onOpenSpace?: (spaceId: number, path?: CreateSpacePath) => void;
  /**
   * Live View / tables-API rows already loaded by TableManager. Same `tables`
   * entity as GET .../tables — used when spaces/summary omits unassigned.
   */
  existingTables?: ExistingTableLike[];
}

function formatRelativeTime(
  iso: string,
  locale: string,
): string {
  try {
    const then = new Date(iso).getTime();
    const now = Date.now();
    const diffSec = Math.round((then - now) / 1000);
    const rtf = new Intl.RelativeTimeFormat(locale === "es-ar" ? "es-AR" : locale, {
      numeric: "auto",
    });
    const abs = Math.abs(diffSec);
    if (abs < 60) return rtf.format(diffSec, "second");
    const min = Math.round(diffSec / 60);
    if (Math.abs(min) < 60) return rtf.format(min, "minute");
    const hr = Math.round(diffSec / 3600);
    if (Math.abs(hr) < 48) return rtf.format(hr, "hour");
    const day = Math.round(diffSec / 86400);
    return rtf.format(day, "day");
  } catch {
    return iso;
  }
}

function statusTone(status: Space["status"]): string {
  switch (status) {
    case "published":
      return "bg-emerald-50 text-emerald-800 border-emerald-200";
    case "archived":
      return "bg-warm-100 text-ink-500 border-warm-200";
    default:
      return "bg-amber-50 text-amber-900 border-amber-200";
  }
}

export default function SpacesOverview({
  businessId,
  initialSpaceId = null,
  onOpenSpace,
  existingTables: existingTablesProp,
}: SpacesOverviewProps) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`spacesTables.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [spaces, setSpaces] = useState<Space[]>([]);
  const [summary, setSummary] = useState<SpaceSummary | null>(null);
  const [unassignedTables, setUnassignedTables] = useState<SpaceTableRef[]>(
    [],
  );
  const [liveTables, setLiveTables] = useState<ExistingTableLike[]>(
    existingTablesProp ?? [],
  );
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [creating, setCreating] = useState(false);
  const [placingExisting, setPlacingExisting] = useState(false);
  const [createPathHint, setCreatePathHint] = useState<CreateSpacePath | null>(
    null,
  );
  const [editorSpaceId, setEditorSpaceId] = useState<number | null>(null);
  const [editorPath, setEditorPath] = useState<CreateSpacePath | null>(null);
  const [scanSpaceId, setScanSpaceId] = useState<number | null>(null);
  const openedInitialSpaceRef = useRef<number | null>(null);

  const {
    isOpen: isCreateOpen,
    onOpen: onCreateOpen,
    onOpenChange: onCreateOpenChange,
  } = useDisclosure();
  const {
    isOpen: isScanOpen,
    onOpen: onScanOpen,
    onClose: onScanClose,
    onOpenChange: onScanOpenChange,
  } = useDisclosure();

  const [renameTarget, setRenameTarget] = useState<Space | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const {
    isOpen: isRenameOpen,
    onOpen: onRenameOpen,
    onOpenChange: onRenameOpenChange,
    onClose: onRenameClose,
  } = useDisclosure();

  const [confirmAction, setConfirmAction] = useState<{
    kind: "delete" | "archive";
    space: Space;
  } | null>(null);
  const {
    isOpen: isConfirmOpen,
    onOpen: onConfirmOpen,
    onOpenChange: onConfirmOpenChange,
    onClose: onConfirmClose,
  } = useDisclosure();

  const load = useCallback(async () => {
    try {
      setLoading(true);
      const livePromise =
        typeof getBusinessTables === "function"
          ? getBusinessTables(businessId).catch(() => ({
              tables: [] as ExistingTableLike[],
            }))
          : Promise.resolve({ tables: [] as ExistingTableLike[] });
      const [listResult, sumResult, liveResult] = await Promise.allSettled([
        spacesApi.list(businessId),
        spacesApi.summary(businessId),
        livePromise,
      ]);

      if (listResult.status === "fulfilled") {
        setSpaces(listResult.value);
      }
      if (sumResult.status === "fulfilled") {
        setSummary(sumResult.value.summary);
        setUnassignedTables(sumResult.value.unassigned_tables ?? []);
      }
      if (liveResult.status === "fulfilled") {
        const rows = liveResult.value.tables ?? [];
        if (rows.length > 0) {
          setLiveTables(rows);
        } else if (existingTablesProp && existingTablesProp.length > 0) {
          setLiveTables(existingTablesProp);
        }
      } else if (existingTablesProp && existingTablesProp.length > 0) {
        setLiveTables(existingTablesProp);
      }

      if (
        listResult.status === "rejected" &&
        sumResult.status === "rejected"
      ) {
        toast.error(
          getSafeApiErrorMessage(
            listResult.reason,
            t("toasts.loadError"),
          ),
        );
      }
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("toasts.loadError")));
    } finally {
      setLoading(false);
    }
  }, [businessId, existingTablesProp, t]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (existingTablesProp && existingTablesProp.length > 0) {
      setLiveTables((prev) => (prev.length > 0 ? prev : existingTablesProp));
    }
  }, [existingTablesProp]);

  useEffect(() => {
    if (
      initialSpaceId &&
      openedInitialSpaceRef.current !== initialSpaceId &&
      spaces.some((s) => s.id === initialSpaceId)
    ) {
      openedInitialSpaceRef.current = initialSpaceId;
      setEditorSpaceId(initialSpaceId);
      onOpenSpace?.(initialSpaceId);
    }
  }, [initialSpaceId, onOpenSpace, spaces]);

  const openEditor = useCallback(
    (spaceId: number, path?: CreateSpacePath) => {
      setEditorSpaceId(spaceId);
      setEditorPath(path ?? null);
      onOpenSpace?.(spaceId, path);
    },
    [onOpenSpace],
  );

  const closeEditor = useCallback(() => {
    setEditorSpaceId(null);
    setEditorPath(null);
    void load();
  }, [load]);

  const kpis = useMemo(
    () =>
      computeSpacesOverviewKpis({
        spaces,
        summary,
        unassignedFromSpaces: unassignedTables,
        existingTables: liveTables,
      }),
    [liveTables, spaces, summary, unassignedTables],
  );
  const waitingTables = kpis.unassignedTables;

  const placeExistingTables = useCallback(async () => {
    if (placingExisting || waitingTables.length === 0) return;
    setPlacingExisting(true);
    try {
      const created = await spacesApi.create(businessId, {
        name: t("empty.defaultSpaceName"),
        space_type: "indoor",
        floor_level: 0,
        measurement_unit: "m",
      });
      const tableIds = waitingTables
        .map((table) => table.id)
        .filter((id) => id > 0);
      await spacesApi.assignTables(
        businessId,
        created.id,
        tableIds.length > 0 ? { table_ids: tableIds } : {},
      );
      toast.success(
        t("empty.placeExistingSuccess", { count: waitingTables.length }),
      );
      setSpaces((prev) => [...prev, created]);
      openEditor(created.id, "draw");
      void load();
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("empty.placeExistingError")));
      void load();
    } finally {
      setPlacingExisting(false);
    }
  }, [
    businessId,
    load,
    openEditor,
    placingExisting,
    t,
    waitingTables,
  ]);

  const openCreate = (path: CreateSpacePath | null = null) => {
    setCreatePathHint(path);
    onCreateOpen();
  };

  const handleCreate = async (
    input: CreateSpaceInput,
    path: CreateSpacePath,
  ) => {
    setCreating(true);
    try {
      const created = await spacesApi.create(businessId, input);
      toast.success(t("toasts.created"));
      setSpaces((prev) => [...prev, created]);
      onCreateOpenChange();
      if (path === "scan") {
        setScanSpaceId(created.id);
        onScanOpen();
      } else {
        openEditor(created.id, path);
      }
      void load();
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("toasts.createError")));
    } finally {
      setCreating(false);
    }
  };

  const startScanForSpace = (spaceId: number) => {
    setScanSpaceId(spaceId);
    onScanOpen();
  };

  const handleRename = async () => {
    if (!renameTarget || !renameValue.trim()) return;
    setBusyId(renameTarget.id);
    try {
      const updated = await spacesApi.patch(businessId, renameTarget.id, {
        name: renameValue.trim(),
      });
      setSpaces((prev) =>
        prev.map((s) => (s.id === updated.id ? updated : s)),
      );
      toast.success(t("toasts.renamed"));
      onRenameClose();
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("toasts.renameError")));
    } finally {
      setBusyId(null);
    }
  };

  const handleDuplicate = async (space: Space) => {
    setBusyId(space.id);
    try {
      const clone = await spacesApi.duplicate(businessId, space.id);
      toast.success(t("toasts.duplicated"));
      setSpaces((prev) => [...prev, clone]);
      void load();
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("toasts.duplicateError")));
    } finally {
      setBusyId(null);
    }
  };

  const handleReorder = async (space: Space, direction: "up" | "down") => {
    const sorted = [...spaces].sort(
      (a, b) => a.sort_order - b.sort_order || a.id - b.id,
    );
    const idx = sorted.findIndex((s) => s.id === space.id);
    if (idx < 0) return;
    const swapWith = direction === "up" ? idx - 1 : idx + 1;
    if (swapWith < 0 || swapWith >= sorted.length) return;
    const a = sorted[idx];
    const b = sorted[swapWith];
    const items = sorted.map((s, i) => {
      if (i === idx) return { id: s.id, sort_order: b.sort_order };
      if (i === swapWith) return { id: s.id, sort_order: a.sort_order };
      return { id: s.id, sort_order: s.sort_order };
    });
    // Ensure unique sequential orders after swap.
    const reindexed = items
      .map((e, i) => {
        const src = sorted.find((s) => s.id === e.id)!;
        const newOrder =
          e.id === a.id
            ? swapWith
            : e.id === b.id
              ? idx
              : i;
        return { id: src.id, sort_order: newOrder };
      })
      .sort((x, y) => x.sort_order - y.sort_order)
      .map((e, i) => ({ id: e.id, sort_order: i }));

    setBusyId(space.id);
    try {
      const next = await spacesApi.reorder(businessId, reindexed);
      setSpaces(next);
      toast.success(t("toasts.reordered"));
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("toasts.reorderError")));
    } finally {
      setBusyId(null);
    }
  };

  const runConfirm = async () => {
    if (!confirmAction) return;
    const { kind, space } = confirmAction;
    setBusyId(space.id);
    try {
      if (kind === "delete") {
        await spacesApi.remove(businessId, space.id);
        setSpaces((prev) => prev.filter((s) => s.id !== space.id));
        toast.success(t("toasts.deleted"));
      } else {
        await spacesApi.archive(businessId, space.id);
        setSpaces((prev) => prev.filter((s) => s.id !== space.id));
        toast.success(t("toasts.archived"));
      }
      void load();
    } catch (err: unknown) {
      const ax = err as {
        response?: { data?: { code?: string; open_bills?: number; open_reservations?: number } };
      };
      if (ax?.response?.data?.code === "has_dependencies") {
        toast.error(t("toasts.hasDependencies"));
      } else {
        toast.error(
          getSafeApiErrorMessage(
            err,
            kind === "delete" ? t("toasts.deleteError") : t("toasts.archiveError"),
          ),
        );
      }
    } finally {
      setBusyId(null);
      setConfirmAction(null);
      onConfirmClose();
    }
  };

  const sortedSpaces = useMemo(
    () =>
      [...spaces].sort(
        (a, b) => a.sort_order - b.sort_order || a.id - b.id,
      ),
    [spaces],
  );

  // Phone-scan lives on the overview cards / create flow — not the layout
  // editor toolbar. A leftover Start scan control there was a dinner-setup
  // no-op (#725).
  const scanSessionModal =
    scanSpaceId != null ? (
      <SpaceScanSessionModal
        isOpen={isScanOpen}
        onOpenChange={(open) => {
          onScanOpenChange();
          if (!open) {
            onScanClose();
            setScanSpaceId(null);
          }
        }}
        businessId={businessId}
        spaceId={scanSpaceId}
        t={t}
        onReviewReady={({ openEditor: shouldOpen }) => {
          const sid = scanSpaceId;
          onScanClose();
          setScanSpaceId(null);
          if (shouldOpen && sid != null) {
            openEditor(sid, "scan");
          }
        }}
      />
    ) : null;

  if (editorSpaceId != null) {
    return (
      <>
        <Suspense
          fallback={
            <div
              className="flex h-[50vh] items-center justify-center rounded-2xl border border-warm-200 bg-warm-50"
              role="status"
              data-testid="space-editor-suspense"
            >
              <p className="text-sm text-ink-500">{t("editor.loading")}</p>
            </div>
          }
        >
          <SpaceEditorPage
            businessId={businessId}
            spaceId={editorSpaceId}
            onClose={closeEditor}
            initialPath={editorPath}
          />
        </Suspense>
        {scanSessionModal}
      </>
    );
  }

  if (loading && spaces.length === 0) {
    return (
      <div
        className="space-y-4"
        role="status"
        aria-label={t("a11y.loadingSpaces")}
        data-testid="spaces-loading"
      >
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {[0, 1, 2, 3].map((i) => (
            <div
              key={i}
              className="h-20 animate-pulse rounded-2xl border border-warm-200 bg-warm-50"
            />
          ))}
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <div
              key={i}
              className="h-56 animate-pulse rounded-2xl border border-warm-200 bg-warm-50"
            />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5" data-testid="spaces-overview">
      {/* Aggregate strip — lead with waiting-to-place when existing tables
          aren't on a floor plan yet so hosts don't read 0/0/0 as data loss. */}
      <div
        className="grid grid-cols-2 gap-3 sm:grid-cols-4"
        role="region"
        aria-label={t("a11y.aggregateStrip")}
      >
        {(kpis.waitingToPlace
          ? [
              {
                key: "waiting",
                label: t("aggregate.waitingToPlace"),
                value: kpis.unassigned,
                title: t(
                  kpis.unassigned === 1
                    ? "aggregate.waitingToPlaceCount_one"
                    : "aggregate.waitingToPlaceCount_other",
                  { count: kpis.unassigned },
                ),
                emphasize: true,
              },
              {
                key: "seats",
                label: t("aggregate.maxSeats"),
                value: kpis.maxSeats,
                title: t("aggregate.maxSeatsFromTablesHint"),
                emphasize: true,
              },
              {
                key: "spaces",
                label: t("aggregate.totalSpaces"),
                value: kpis.spaces,
              },
              {
                key: "placed",
                label: t("aggregate.totalTables"),
                value: kpis.tablesPlaced,
              },
            ]
          : [
              {
                key: "spaces",
                label: t("aggregate.totalSpaces"),
                value: kpis.spaces,
              },
              {
                key: "placed",
                label: t("aggregate.totalTables"),
                value: kpis.tablesPlaced,
              },
              {
                key: "seats",
                label: t("aggregate.maxSeats"),
                value: kpis.maxSeats,
              },
              {
                key: "unassigned",
                label: t("aggregate.unassigned"),
                value: kpis.unassigned,
                title: t("aggregate.unassignedHint"),
                emphasize: kpis.unassigned > 0,
              },
            ]
        ).map((stat) => (
          <PremiumPanel
            key={stat.key}
            className={`px-4 py-3${stat.emphasize ? " border-brand/30 bg-brand/[0.03]" : ""}`}
            withTexture={false}
          >
            <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
              {stat.label}
            </p>
            <p
              className="mt-1 text-2xl font-semibold tabular-nums text-ink-900"
              title={stat.title}
              data-testid={`spaces-aggregate-${stat.key}`}
            >
              {stat.value}
            </p>
          </PremiumPanel>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          className="bg-brand text-white font-medium hover:bg-brand-dark"
          radius="full"
          size="sm"
          onPress={() => openCreate(null)}
          startContent={<Plus className="h-4 w-4" />}
          data-testid="spaces-create-button"
        >
          {t("overview.createSpace")}
        </Button>
        <button
          type="button"
          onClick={() => void load()}
          aria-label={t("overview.refreshAria")}
          title={t("overview.refreshAria")}
          className={btnGhostIcon}
        >
          <RefreshCw
            className={`h-4 w-4 ${loading ? "animate-spin" : ""}`}
          />
        </button>
      </div>

      {waitingTables.length > 0 && sortedSpaces.length > 0 && (
        <UnassignedTablesPanel
          businessId={businessId}
          spaces={spaces}
          unassigned={waitingTables}
          t={t}
          onAssigned={() => void load()}
          onOpenSpace={(id) => openEditor(id, "draw")}
        />
      )}

      {sortedSpaces.length === 0 ? (
        <EmptyState
          panel
          icon={LayoutGrid}
          title={
            waitingTables.length > 0
              ? t("empty.existingTitle", { count: waitingTables.length })
              : t("empty.title")
          }
          subtitle={
            waitingTables.length > 0
              ? t("empty.existingDescription", {
                  count: waitingTables.length,
                  seats: kpis.maxSeats,
                })
              : t("empty.description")
          }
          className="[&_p]:max-w-xl"
          data-testid="spaces-empty"
          action={
            <div className="flex flex-col items-center gap-3 sm:flex-row sm:flex-wrap sm:justify-center">
              {waitingTables.length > 0 ? (
                <button
                  type="button"
                  onClick={() => void placeExistingTables()}
                  disabled={placingExisting}
                  className={btnPrimary}
                  data-testid="spaces-empty-place-existing"
                >
                  {placingExisting
                    ? t("empty.placeExistingWorking")
                    : t("empty.placeExistingCta", {
                        count: waitingTables.length,
                      })}
                </button>
              ) : null}
              <button
                type="button"
                onClick={() => openCreate("scan")}
                className="inline-flex items-center gap-2 rounded-full bg-brand px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-dark"
                data-testid="spaces-empty-scan"
              >
                <Smartphone className="h-4 w-4" />
                {t("empty.scanCta")}
              </button>
              <button
                type="button"
                onClick={() => openCreate("draw")}
                className={btnSecondary}
                data-testid="spaces-empty-draw"
              >
                <Pencil className="h-4 w-4" />
                {t("empty.drawCta")}
              </button>
            </div>
          }
          hint={
            waitingTables.length > 0
              ? t("empty.existingHint")
              : [
                  t("empty.valueScan"),
                  t("empty.valueDraw"),
                  t("empty.valuePublish"),
                ].join(" · ")
          }
        />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {sortedSpaces.map((space, index) => {
            const stats = spaceCardStats(space, liveTables, sortedSpaces.length);
            const typeLabel =
              t(`spaceTypes.${space.space_type}`) || space.space_type;
            const statusLabel = t(`status.${space.status}`);

            return (
              <PremiumPanel
                key={space.id}
                className="flex flex-col overflow-hidden"
                withTexture={false}
                data-testid={`space-card-${space.id}`}
              >
                <div
                  className="relative h-28 border-b border-warm-100 bg-warm-50"
                  aria-label={t("card.previewAria", { name: space.name })}
                >
                  <LayoutPreviewSvg
                    layoutRaw={space.draft_layout_json}
                    publishedRaw={space.published_layout_json}
                    widthMm={space.width_mm}
                    heightMm={space.height_mm}
                    aria-label={t("a11y.miniPreview")}
                  />
                </div>
                <div className="flex flex-1 flex-col gap-3 p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <h3 className="truncate text-base font-semibold text-ink-900">
                        {space.name}
                      </h3>
                      <p className="mt-0.5 text-xs text-ink-500">
                        {typeLabel}
                        {" · "}
                        {t("card.floor", { level: space.floor_level })}
                      </p>
                    </div>
                    <span
                      className={`shrink-0 rounded-full border px-2 py-0.5 text-[11px] font-medium ${statusTone(space.status)}`}
                      aria-label={t("a11y.statusBadge", {
                        status: statusLabel,
                      })}
                    >
                      {statusLabel}
                    </span>
                  </div>

                  <div
                    className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-ink-600"
                    data-testid={`space-card-stats-${space.id}`}
                    data-table-count={stats.tableCount}
                    data-seat-count={stats.maxSeats}
                  >
                    <span>
                      {t("card.tablesSimple", { count: stats.tableCount })}
                    </span>
                    <span>
                      {t("card.seatsSimple", { count: stats.maxSeats })}
                    </span>
                    <span>
                      {t("card.updated", {
                        time: formatRelativeTime(space.updated_at, locale),
                      })}
                    </span>
                  </div>

                  {space.has_unpublished_changes && (
                    <p className="text-xs font-medium text-amber-800">
                      {t("card.unpublished")}
                    </p>
                  )}

                  <div className="mt-auto flex items-center gap-2 pt-1">
                    <Button
                      size="sm"
                      radius="full"
                      className="bg-brand text-white font-medium"
                      onPress={() => openEditor(space.id)}
                      data-testid={`space-open-${space.id}`}
                    >
                      {t("card.open")}
                    </Button>
                    <Button
                      size="sm"
                      radius="full"
                      variant="bordered"
                      className="border-warm-200"
                      onPress={() => openEditor(space.id, "draw")}
                      startContent={<Pencil className="h-3.5 w-3.5" />}
                      data-testid={`space-edit-${space.id}`}
                    >
                      {t("card.edit")}
                    </Button>
                    <Dropdown>
                      <DropdownTrigger>
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          aria-label={t("card.actionsAria", {
                            name: space.name,
                          })}
                          isDisabled={busyId === space.id}
                        >
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownTrigger>
                      <DropdownMenu aria-label={t("card.actionsAria", { name: space.name })}>
                        <DropdownItem
                          key="scan"
                          startContent={<Smartphone className="h-4 w-4" />}
                          onPress={() => startScanForSpace(space.id)}
                          data-testid={`space-scan-${space.id}`}
                        >
                          {t("scan.start")}
                        </DropdownItem>
                        <DropdownItem
                          key="rename"
                          startContent={<Pencil className="h-4 w-4" />}
                          onPress={() => {
                            setRenameTarget(space);
                            setRenameValue(space.name);
                            onRenameOpen();
                          }}
                        >
                          {t("card.rename")}
                        </DropdownItem>
                        <DropdownItem
                          key="duplicate"
                          startContent={<Copy className="h-4 w-4" />}
                          onPress={() => void handleDuplicate(space)}
                        >
                          {t("card.duplicate")}
                        </DropdownItem>
                        <DropdownItem
                          key="up"
                          startContent={<ArrowUp className="h-4 w-4" />}
                          isDisabled={index === 0}
                          onPress={() => void handleReorder(space, "up")}
                        >
                          {t("card.moveUp")}
                        </DropdownItem>
                        <DropdownItem
                          key="down"
                          startContent={<ArrowDown className="h-4 w-4" />}
                          isDisabled={index === sortedSpaces.length - 1}
                          onPress={() => void handleReorder(space, "down")}
                        >
                          {t("card.moveDown")}
                        </DropdownItem>
                        <DropdownItem
                          key="archive"
                          startContent={<Archive className="h-4 w-4" />}
                          onPress={() => {
                            setConfirmAction({ kind: "archive", space });
                            onConfirmOpen();
                          }}
                        >
                          {t("card.archive")}
                        </DropdownItem>
                        <DropdownItem
                          key="delete"
                          className="text-danger"
                          color="danger"
                          startContent={<Trash2 className="h-4 w-4" />}
                          onPress={() => {
                            setConfirmAction({ kind: "delete", space });
                            onConfirmOpen();
                          }}
                        >
                          {t("card.delete")}
                        </DropdownItem>
                      </DropdownMenu>
                    </Dropdown>
                  </div>
                </div>
              </PremiumPanel>
            );
          })}
        </div>
      )}

      <CreateSpaceModal
        isOpen={isCreateOpen}
        onOpenChange={onCreateOpenChange}
        onSubmit={handleCreate}
        initialPath={createPathHint}
        isSubmitting={creating}
      />

      {scanSessionModal}

      <Modal isOpen={isRenameOpen} onOpenChange={onRenameOpenChange} size="md">
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>{t("rename.title")}</ModalHeader>
              <ModalBody>
                <Input
                  label={t("rename.label")}
                  value={renameValue}
                  onValueChange={setRenameValue}
                  variant="bordered"
                  autoFocus
                  classNames={{
                    inputWrapper:
                      "border-warm-200 data-[hover=true]:border-brand/40",
                  }}
                />
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  {t("rename.cancel")}
                </Button>
                <Button
                  className="bg-brand text-white font-medium"
                  isLoading={busyId === renameTarget?.id}
                  onPress={() => void handleRename()}
                >
                  {t("rename.save")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <ConfirmationModal
        isOpen={isConfirmOpen}
        onOpenChange={onConfirmOpenChange}
        title={
          confirmAction?.kind === "delete"
            ? t("confirm.deleteTitle", {
                name: confirmAction.space.name,
              })
            : t("confirm.archiveTitle", {
                name: confirmAction?.space.name ?? "",
              })
        }
        description={
          confirmAction?.kind === "delete"
            ? t("confirm.deleteBody")
            : t("confirm.archiveBody")
        }
        cancelLabel={t("confirm.cancel")}
        confirmLabel={
          confirmAction?.kind === "delete"
            ? t("confirm.deleteConfirm")
            : t("confirm.archiveConfirm")
        }
        isDanger={confirmAction?.kind === "delete"}
        onConfirm={() => {
          void runConfirm();
        }}
      />
    </div>
  );
}
