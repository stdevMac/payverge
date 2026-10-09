"use client";

/**
 * Task 2.3 — TableManager rebuilt as a list + drawer layout.
 *
 * The previous implementation rendered tables as a wall of QR cards with a
 * Grid/List toggle, an absolute-positioned name badge, and an inline error
 * boundary around `TableGridView`. All of that has been retired in favour of:
 *
 *   - a single tabular list (TableRow per table) with search + status filter
 *   - a right-side drawer (TableDetailDrawer) holding Overview / QR / Settings
 *   - the existing Create Table modal (TableModals) still wired to onCreateOpen
 *
 * Edit/delete/QR customization no longer live on the page chrome — they move
 * into the drawer's Settings/QR tabs. `TableGridView` and its inline error
 * boundary fallback (TableGridErrorFallback) are deleted as part of this task.
 */

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useDisclosure, Select, SelectItem, Button } from "@nextui-org/react";
import {
  LayoutGrid,
  List,
  Map as MapIcon,
  Plus,
  Printer,
  QrCode,
  Radio,
  RefreshCw,
  Search,
  X,
} from "lucide-react";
import toast from "react-hot-toast";
import { usePolling } from "@/hooks/usePolling";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import {
  buildTableViewModel,
  countTableStatuses,
  type StatusFilter as ViewModelStatusFilter,
} from "./tableViewModel";
import {
  businessApi,
  getBusiness,
  Table,
  TableWithStatus,
  UpdateTableRequest,
} from "@/api/business";
import { getSetupStatus } from "@/api/onboarding";
import { spacesApi, parseLayoutDocument } from "@/api/spaces";
import TableModals from "./TableModals";
import QRCustomizationModal from "./QRCustomizationModal";
import ConfirmationModal from "./modals/ConfirmationModal";
import TableRow, {
  TableRowData,
  TableStatus,
} from "./tables/TableRow";
import { seatedSourcesFromStatus } from "./tables/seatedAge";
import TableDetailModal from "./tables/TableDetailModal";
import ServiceCallsQueue from "./tables/ServiceCallsQueue";
import TablesLiveMap from "./tables/TablesLiveMap";
import { matchesTableSearch } from "./tables/tableSearchFilter";
import { useQrSheetPrint } from "./tables/useQrSheetPrint";
import { TablesSkeleton } from "./TablesSkeleton";
import { PremiumPanel } from "./premium";
import DashboardTabShell from "./shared/DashboardTabShell";
import ActivationPanel from "./shared/ActivationPanel";
import Toolbar from "./shared/Toolbar";
import { EmptyState } from "@/components/ui/EmptyState";
import { btnGhostIcon, btnSecondary } from "@/components/ui/buttonStyles";
import SpacesOverview from "./spaces/SpacesOverview";
import type { CreateSpacePath } from "./spaces/CreateSpaceModal";

/** Sub-view under the Tables tab (not a top-level sidebar key). */
type TablesView = "live" | "spaces";

function parseTablesView(raw: string | null): TablesView {
  return raw === "spaces" ? "spaces" : "live";
}

interface TableManagerProps {
  businessId: number;
  businessName?: string;
}

// "inactive" is a filter-only pseudo-status (a soft-deleted table). Rows carry
// is_active separately. "all" / "Todos los estados" includes inactive (L3-27);
// the dedicated inactive segment still isolates them for reactivate flows.
type StatusFilter = TableStatus | "all" | "inactive";

function toRowData(tws: TableWithStatus): TableRowData {
  const table = tws.table;
  const firstBill =
    tws.active_bills && tws.active_bills.length > 0
      ? tws.active_bills[0]
      : null;
  const nextReservation =
    tws.reservations && tws.reservations.length > 0
      ? tws.reservations[0]
      : null;
  const seated = seatedSourcesFromStatus(tws);
  return {
    id: table.id,
    name: table.name,
    table_code: table.table_code,
    is_active: table.is_active,
    // Use the real status from the /tables/status endpoint; fall back to
    // "available" defensively if the field is missing.
    status: tws.status || "available",
    capacity: table.capacity ?? 0,
    active_bill: firstBill
      ? {
          id: firstBill.id,
          total: firstBill.total_amount,
          physical_item_quantity:
            tws.active_bill_physical_item_quantity ?? 0,
          created_at: seated.created_at,
          updated_at: seated.last_seen,
        }
      : null,
    server_name: tws.active_bill_server_name?.trim() || null,
    next_reservation: nextReservation
      ? {
          id: nextReservation.id,
          customer_name: nextReservation.customer_name,
          party_size: nextReservation.party_size,
          reservation_time: nextReservation.reservation_time,
          status: nextReservation.status,
        }
      : null,
    last_seen: seated.last_seen,
    // Per-row QR overrides flow through so the row-level Download action
    // honors per-table customization without needing the drawer.
    qr_foreground_color: table.qr_foreground_color,
    qr_background_color: table.qr_background_color,
  };
}

function TableEmptyPanel({
  mode,
  tString,
  onAction,
}: {
  mode: "firstRun" | "filtered";
  tString: (key: string, params?: Record<string, string | number>) => string;
  onAction: () => void;
}) {
  if (mode === "firstRun") {
    return (
      <ActivationPanel
        icon={QrCode}
        title={tString("emptyState.title")}
        description={tString("emptyState.description")}
        features={[
          { title: tString("emptyState.stepCreate") },
          { title: tString("emptyState.stepQr") },
          { title: tString("emptyState.stepSeat") },
        ]}
        action={
          <button
            onClick={onAction}
            className="inline-flex items-center gap-2 rounded-full bg-brand px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-dark"
          >
            {tString("emptyState.createButton")}
          </button>
        }
      />
    );
  }

  return (
    <EmptyState
      panel
      compact
      icon={Search}
      title={tString("search.noResults")}
      subtitle={tString("search.noResultsDescription")}
      action={
        <button onClick={onAction} className={btnSecondary}>
          <X className="h-4 w-4" />
          {tString("buttons.clear")}
        </button>
      }
    />
  );
}

export default function TableManager({
  businessId,
  businessName: businessNameProp = "",
}: TableManagerProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const [currentLocale, setCurrentLocale] = useState(locale);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const fullKey = `businessDashboard.dashboard.tableManager.${key}`;
      const result = getTranslation(fullKey, currentLocale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const tSpaces = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`spacesTables.${key}`, currentLocale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Deep-link: ?tablesView=spaces|live and optional ?spaceId=
  const urlTablesViewRaw = searchParams?.get("tablesView") ?? "";
  const [tablesView, setTablesView] = useState<TablesView>(() =>
    parseTablesView(urlTablesViewRaw || null),
  );
  // Adopt URL during render (BillManager pattern) so a local tab click is never
  // stomped by a stale searchParams effect echo — first click must win.
  const [lastUrlTablesView, setLastUrlTablesView] = useState(urlTablesViewRaw);
  if (urlTablesViewRaw !== lastUrlTablesView) {
    setLastUrlTablesView(urlTablesViewRaw);
    setTablesView(parseTablesView(urlTablesViewRaw || null));
  }
  const spaceIdParam = searchParams?.get("spaceId");
  const initialSpaceId = spaceIdParam
    ? Number.parseInt(spaceIdParam, 10) || null
    : null;

  const setTablesViewAndUrl = useCallback(
    (view: TablesView) => {
      setTablesView(view);
      if (view === "spaces") {
        // Search is not visible on Espacios y mesas — drop it so the header
        // cannot inherit a Live View filter the host cannot see (#658).
        setSearchQuery("");
        setStatusFilter("all");
      }
      if (typeof window === "undefined") return;
      const params = new URLSearchParams(searchParams?.toString() ?? "");
      if (view === "live") {
        params.delete("tablesView");
        params.delete("spaceId");
      } else {
        params.set("tablesView", "spaces");
        params.delete("tableSearch");
      }
      const qs = params.toString();
      const href = qs ? `${pathname}?${qs}` : pathname || "";
      router.replace(href, { scroll: false });
    },
    [pathname, router, searchParams],
  );

  const handleOpenSpace = useCallback(
    (spaceId: number, _path?: CreateSpacePath) => {
      // Deep-link so refresh / share keeps the Spaces view + open editor id.
      // SpacesOverview lazy-loads SpaceEditorPage when this fires / on initialSpaceId.
      if (typeof window === "undefined") return;
      const params = new URLSearchParams(searchParams?.toString() ?? "");
      params.set("tablesView", "spaces");
      params.set("spaceId", String(spaceId));
      const qs = params.toString();
      const href = qs ? `${pathname}?${qs}` : pathname || "";
      router.replace(href, { scroll: false });
    },
    [pathname, router, searchParams],
  );

  // Raw tables (for the create/edit/delete callbacks that need the full
  // shape) and the derived list-row projection (what TableRow consumes).
  const [tables, setTables] = useState<TableWithStatus[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Business's configured currency, used by TableRow to format active-bill
  // totals. Defaults to USD until getBusiness() resolves so the column
  // renders something sensible during the initial load.
  const [businessCurrency, setBusinessCurrency] = useState<string>("USD");
  const [businessTimezone, setBusinessTimezone] = useState<string | null>(null);
  // Business-level QR defaults (logo, colors, font) used by the drawer's
  // QR preview when the per-table override isn't set. Surfacing these
  // means the operator sees their actual branded QR — not a blank
  // black-on-white placeholder — without having to customize every row.
  const [businessName, setBusinessNameState] = useState<string>("");
  const [qrDefaults, setQrDefaults] = useState<
    | {
        qr_logo_url?: string;
        qr_foreground_color?: string;
        qr_background_color?: string;
        qr_logo_size?: number;
        qr_show_business_name?: boolean;
        qr_show_table_name?: boolean;
        qr_text_font?: string;
      }
    | undefined
  >(undefined);

  // P3: activation signal for the "no orders yet" banner. null = unknown
  // (loading or fetch failed) → banner hidden; advisory only.
  const [hasFirstPaidBill, setHasFirstPaidBill] = useState<boolean | null>(null);
  useEffect(() => {
    let cancelled = false;
    getSetupStatus(businessId)
      .then((s) => {
        if (!cancelled) setHasFirstPaidBill(s.has_first_paid_bill);
      })
      .catch(() => {
        /* advisory banner only — stay hidden on error */
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  // P2-16: one print-ready sheet for every active table.
  const resolvedBusinessName = businessName || businessNameProp;
  const { printAll, printing: printingQr, activeTableCount } = useQrSheetPrint({
    businessName: resolvedBusinessName,
    tables: tables.map((tws) => tws.table),
    defaults: qrDefaults,
    strings: {
      scanCaption: tString("printAll.sheetScanCaption"),
      documentTitle: tString("printAll.sheetTitle", {
        businessName: resolvedBusinessName,
      }),
      poweredBy: tString("qrCustomization.poweredBy"),
    },
    onError: () => setError(tString("printAll.error")),
  });

  // Celebration "Print your QR sheet" hand-off (OnboardingHub sets this
  // one-shot flag, then navigates here). Consume it once after tables load.
  const autoPrintFiredRef = useRef(false);
  useEffect(() => {
    if (autoPrintFiredRef.current || isLoading || activeTableCount === 0) return;
    if (typeof window === "undefined") return;
    const key = `payverge_print_qr_on_open_${businessId}`;
    if (window.sessionStorage.getItem(key) === "1") {
      window.sessionStorage.removeItem(key);
      autoPrintFiredRef.current = true;
      void printAll();
    }
  }, [isLoading, activeTableCount, businessId, printAll]);

  const firstActiveTableCode = useMemo(() => {
    const first = tables.find((tws) => tws.table.is_active && tws.table.table_code);
    return first ? first.table.table_code : null;
  }, [tables]);

  // List filters — tableSearch deep-link (AI Waiter) seeds the box once.
  // Spaces has no search control, so a leftover Live View query must not
  // initialize the filter (#658).
  const [searchQuery, setSearchQuery] = useState(() =>
    parseTablesView(urlTablesViewRaw || null) === "spaces"
      ? ""
      : (searchParams?.get("tableSearch") ?? ""),
  );
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");

  // Live View list|map mode. Map is only offered when a published space exists.
  type LiveMode = "list" | "map";
  const [liveMode, setLiveMode] = useState<LiveMode>("list");
  const [hasPublishedSpace, setHasPublishedSpace] = useState(false);

  useEffect(() => {
    let cancelled = false;
    spacesApi
      .list(businessId)
      .then((list) => {
        if (cancelled) return;
        const any = list.some(
          (s) =>
            s.status === "published" &&
            parseLayoutDocument(s.published_layout_json) != null,
        );
        setHasPublishedSpace(any);
        if (!any) setLiveMode("list");
      })
      .catch(() => {
        if (!cancelled) {
          setHasPublishedSpace(false);
          setLiveMode("list");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, tablesView]);

  // Drawer state — keyed by table id so it survives re-renders of the
  // underlying list without holding a stale row object. Seed from ?tableId=
  // so service-call "Open table" deep-links land on the right row.
  const [selectedId, setSelectedId] = useState<number | null>(() => {
    const raw = searchParams?.get("tableId");
    if (!raw) return null;
    const n = Number.parseInt(raw, 10);
    return Number.isFinite(n) && n > 0 ? n : null;
  });

  useEffect(() => {
    const raw = searchParams?.get("tableId");
    if (!raw) return;
    const n = Number.parseInt(raw, 10);
    if (Number.isFinite(n) && n > 0) {
      setSelectedId(n);
    }
  }, [searchParams]);

  // Create modal state (Edit modal is intentionally retired; Settings tab
  // inside the drawer now owns rename / activate / delete).
  const [tableName, setTableName] = useState("");
  const [tableCapacity, setTableCapacity] = useState(4);
  const [creatingTable, setCreatingTable] = useState(false);
  const {
    isOpen: isCreateOpen,
    onOpen: onCreateOpen,
    onOpenChange: onCreateOpenChange,
  } = useDisclosure();

  // QR customization modal — opened from the drawer's QR tab. Selecting
  // a table in the list sets `selectedId`; when the drawer's QR tab
  // fires `onCustomize`, we look the row's full Table up by id and open
  // this modal. Regression caught 2026-05-13: the modal was mounted but
  // the open callback was never destructured, so QR customization was
  // unreachable through the UI.
  const [customizingTable, setCustomizingTable] = useState<Table | null>(null);
  const [savingCustomization, setSavingCustomization] = useState(false);
  const {
    isOpen: isCustomizeOpen,
    onOpen: onCustomizeOpen,
    onClose: onCustomizeClose,
  } = useDisclosure();

  const handleRequestCustomize = useCallback(
    (tableId: number) => {
      const full = tables.find((t) => t.table.id === tableId);
      if (!full) return;
      setCustomizingTable(full.table);
      onCustomizeOpen();
    },
    [tables, onCustomizeOpen],
  );

  // Confirmation modal for delete from the drawer's Settings tab.
  const [tableToDelete, setTableToDelete] = useState<number | null>(null);
  const {
    isOpen: isDeleteOpen,
    onOpen: onDeleteOpen,
    onOpenChange: onDeleteOpenChange,
    onClose: onDeleteClose,
  } = useDisclosure();

  // Confirmation for the QR "Apply to All" fan-out — it rewrites every table's
  // branding, so the operator confirms the affected count first (R3-OK LOW).
  const [pendingApplyAll, setPendingApplyAll] = useState<{
    qr_logo_url: string;
    qr_foreground_color: string;
    qr_background_color: string;
    qr_logo_size: number;
    qr_show_business_name: boolean;
    qr_show_table_name: boolean;
    qr_text_font: string;
  } | null>(null);
  const {
    isOpen: isApplyAllOpen,
    onOpen: onApplyAllOpen,
    onOpenChange: onApplyAllOpenChange,
    onClose: onApplyAllClose,
  } = useDisclosure();

  const loadTables = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      // Always fetch inactive tables too so the operator can reactivate a
      // deactivated table; the board hides them from the default view but
      // exposes them under the "Inactive" filter.
      const response = await businessApi.getTablesWithStatus(businessId, true);
      setTables(response.tables || []);
    } catch (err) {
      setError(
        getSafeApiErrorMessage(err, tString("error.loadTables")),
      );
    } finally {
      setIsLoading(false);
    }
  }, [businessId, tString]);

  useEffect(() => {
    void loadTables();
  }, [loadTables]);

  // Modest background refresh so occupied/available status stays fresh
  // without the operator manually hitting refresh. 25s is a low-risk cadence
  // for a status board that also has a manual refresh button (R3-OK LOW).
  usePolling({
    callback: loadTables,
    interval: 25_000,
    enabled: true,
    immediate: false,
    pauseWhenHidden: true,
  });

  // Resolve the business's currency + QR defaults in parallel with the
  // tables query. Best-effort: if this fails, the column stays on USD
  // and the QR preview renders with hard library defaults.
  useEffect(() => {
    let cancelled = false;
    getBusiness(businessId)
      .then((b) => {
        if (cancelled || !b) return;
        if (b.default_currency) {
          setBusinessCurrency(b.default_currency);
        }
        if (b.name) {
          setBusinessNameState(b.name);
        }
        setBusinessTimezone(b.timezone ?? null);
        setQrDefaults({
          qr_logo_url: b.default_qr_logo_url,
          qr_foreground_color: b.default_qr_foreground_color,
          qr_background_color: b.default_qr_background_color,
          qr_logo_size: b.default_qr_logo_size,
          qr_show_business_name: b.default_qr_show_business_name,
          qr_show_table_name: b.default_qr_show_table_name,
          qr_text_font: b.default_qr_text_font,
        });
      })
      .catch(() => {
        /* keep defaults */
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  const rows = useMemo(() => tables.map(toRowData), [tables]);

  // L3-27: chips + list share one filter/count view-model, and the chips now
  // describe the search-matched set instead of the whole board.
  const tableView = useMemo(
    () =>
      buildTableViewModel(rows, statusFilter as ViewModelStatusFilter, (t) =>
        matchesTableSearch(t, searchQuery),
      ),
    [rows, statusFilter, searchQuery],
  );
  const filteredRows = tableView.rows;
  const unfilteredTableCounts = useMemo(
    () => countTableStatuses(rows),
    [rows],
  );
  const isSpacesView = tablesView === "spaces";
  // Spaces has no search box — header totals must stay unfiltered so a Live
  // View search cannot leak into Espacios y mesas.
  const tableCounts = isSpacesView ? unfilteredTableCounts : tableView.counts;

  // Availability of the Inactive filter option keys off the board, not the
  // search: a search that matches no inactive table must not remove an option
  // that may currently be the selected one.
  const hasInactiveTables = useMemo(
    () => rows.some((row) => !row.is_active),
    [rows],
  );

  const selectedRow = useMemo(
    () => rows.find((r) => r.id === selectedId) || null,
    [rows, selectedId],
  );


  const selectedTable = useMemo(
    () => {
      const found = tables.find((t) => t.table.id === selectedId);
      return found ? found.table : null;
    },
    [tables, selectedId],
  );

  const clearSearch = () => {
    setSearchQuery("");
    setStatusFilter("all");
  };

  const handleCreateTable = async () => {
    if (!tableName.trim() || creatingTable) return;
    setCreatingTable(true);
    try {
      const newTable = await businessApi.createTableWithQR(businessId, {
        name: tableName.trim(),
        capacity: tableCapacity,
      });
      // Wrap the bare Table in a TableWithStatus shell — a freshly
      // created table has no bills or reservations.
      const wrapped: TableWithStatus = {
        table: newTable,
        status: "available",
        active_bills: [],
        active_bills_count: 0,
        active_bill_physical_item_quantity: 0,
        reservations: [],
        reservations_count: 0,
      };
      setTables([...tables, wrapped]);
      setTableName("");
      setTableCapacity(4);
      onCreateOpenChange();
    } catch (err) {
      setError(
        getSafeApiErrorMessage(err, tString("error.createTable")),
      );
    } finally {
      setCreatingTable(false);
    }
  };

  // Modal → inline rename. Rethrows on failure so the detail modal can toast,
  // resync its name draft to server truth, and clear its saving indicator
  // (R3-OK table edit feedback) — a page banner behind the open modal was
  // invisible to the operator.
  const handleRename = async (newName: string) => {
    if (!selectedTable || !newName.trim()) return;
    try {
      const updated = await businessApi.updateTableDetails(selectedTable.id, {
        name: newName.trim(),
      });
      setTables(tables.map((t) => (t.table.id === updated.id ? { ...t, table: updated } : t)));
    } catch (err) {
      toast.error(
        getSafeApiErrorMessage(err, tString("error.updateTable")),
      );
      throw err;
    }
  };

  // Modal → seats stepper. Mirrors handleRename so capacity edits flow
  // through the same axios path and refresh the row from the response.
  const handleCapacityChange = async (capacity: number) => {
    if (!selectedTable || !Number.isFinite(capacity) || capacity < 1) return;
    if (capacity === selectedTable.capacity) return;
    try {
      const updated = await businessApi.updateTableDetails(selectedTable.id, {
        capacity,
      });
      setTables(tables.map((t) => (t.table.id === updated.id ? { ...t, table: updated } : t)));
    } catch (err) {
      toast.error(
        getSafeApiErrorMessage(err, tString("error.updateTable")),
      );
      throw err;
    }
  };

  // Drawer → Settings tab → Activate/Deactivate toggle
  const handleToggleActive = async () => {
    if (!selectedTable) return;
    try {
      const updated = await businessApi.updateTableDetails(selectedTable.id, {
        is_active: !selectedTable.is_active,
      });
      setTables(tables.map((t) => (t.table.id === updated.id ? { ...t, table: updated } : t)));
    } catch (err) {
      toast.error(
        getSafeApiErrorMessage(err, tString("error.updateTable")),
      );
      throw err;
    }
  };

  // Drawer → Settings tab → Delete
  const handleDeleteRequest = () => {
    if (!selectedTable) return;
    setTableToDelete(selectedTable.id);
    onDeleteOpen();
  };

  const confirmDeleteTable = async () => {
    if (tableToDelete == null) return;
    try {
      await businessApi.deleteTable(tableToDelete);
      setTables(tables.filter((t) => t.table.id !== tableToDelete));
      // Close the drawer if it was pointing at the deleted row.
      if (selectedId === tableToDelete) setSelectedId(null);
    } catch (err) {
      setError(
        getSafeApiErrorMessage(err, tString("error.deleteTable")),
      );
    } finally {
      setTableToDelete(null);
      onDeleteClose();
    }
  };

  // QR customization save — retained from the previous implementation so
  // any existing entry point (e.g. drawer's QR tab callback wiring) can
  // still write through to the API. Currently un-invoked from chrome.
  type QRCustomization = {
    qr_logo_url: string;
    qr_foreground_color: string;
    qr_background_color: string;
    qr_logo_size: number;
    qr_show_business_name: boolean;
    qr_show_table_name: boolean;
    qr_text_font: string;
  };

  // Fan-out the branding to every table + the business defaults. Extracted so
  // the "Apply to All" path can run it only after the operator confirms the
  // affected count (R3-OK LOW).
  const runApplyToAll = async (customization: QRCustomization) => {
    setError(null);
    setSavingCustomization(true);
    try {
      // One transactional request updates every table + the business defaults
      // (was a per-table PUT fan-out — 201 concurrent, non-atomic requests at
      // 200 tables, where a partial apply left the floor inconsistent).
      await businessApi.applyQrBrandingToAllTables(businessId, customization);
      // Refresh so the grid reflects the applied branding.
      await loadTables();
      setCustomizingTable(null);
      onCustomizeClose();
    } catch (err) {
      console.error("Failed to save QR customization:", err);
      setError(
        getSafeApiErrorMessage(err, tString("qrCustomizeSaveError")),
      );
    } finally {
      setSavingCustomization(false);
    }
  };

  const handleSaveCustomization = async (
    customization: QRCustomization,
    applyToAll: boolean = false,
  ) => {
    if (!customizingTable) return;
    // Apply-to-all rewrites every table's branding — confirm the count first.
    if (applyToAll) {
      setPendingApplyAll(customization);
      onApplyAllOpen();
      return;
    }
    setError(null);
    setSavingCustomization(true);
    try {
      const updateData: UpdateTableRequest = {
        name: customizingTable.name,
        is_active: customizingTable.is_active,
        ...customization,
      };
      await businessApi.updateBusinessTable(
        businessId,
        customizingTable.id,
        updateData,
      );
      await loadTables();
      setCustomizingTable(null);
      onCustomizeClose();
    } catch (err) {
      console.error("Failed to save QR customization:", err);
      setError(
        getSafeApiErrorMessage(err, tString("qrCustomizeSaveError")),
      );
    } finally {
      setSavingCustomization(false);
    }
  };

  const confirmApplyToAll = async () => {
    if (!pendingApplyAll) return;
    const customization = pendingApplyAll;
    setPendingApplyAll(null);
    onApplyAllClose();
    await runApplyToAll(customization);
  };

  return (
    <DashboardTabShell
      loading={
        !isSpacesView && isLoading && tables.length === 0 ? (
          <TablesSkeleton />
        ) : null
      }
      header={{
        // Keep a stable H1 across Live / Spaces so sub-tabs feel like one page
        // (#188). Spaces copy stays in the subtitle only.
        title: tString("title"),
        subtitle: isSpacesView
          ? tSpaces("overview.subtitle")
          : tString("subtitle"),
        stats:
          tableCounts.total > 0
            ? [
                { label: tString("hero.total"), value: tableCounts.total },
                {
                  label: tString("hero.available"),
                  value: tableCounts.available,
                },
                {
                  label: tString("hero.occupied"),
                  value: tableCounts.occupied,
                },
                {
                  label: tString("hero.reserved"),
                  value: tableCounts.reserved,
                  title: tString("hero.reservedTooltip"),
                },
              ]
            : [],
        actions: (
          <>
            <Button
              variant="bordered"
              radius="full"
              className="border-warm-200 bg-white text-ink-700 font-medium hover:border-brand/40"
              onPress={() => void printAll()}
              isDisabled={activeTableCount === 0}
              isLoading={printingQr}
              startContent={<Printer className="h-4 w-4" />}
            >
              {tString("printAll.button")}
            </Button>
            <Button
              className="bg-brand text-white font-medium hover:bg-brand-dark"
              radius="full"
              onPress={onCreateOpen}
              startContent={<Plus className="h-4 w-4" />}
            >
              {tString("createTable")}
            </Button>
          </>
        ),
      }}
      tabs={{
        items: [
          {
            key: "live",
            label: tSpaces("nav.liveView"),
            icon: Radio,
          },
          {
            key: "spaces",
            label: tSpaces("nav.spacesTables"),
            icon: LayoutGrid,
          },
        ],
        activeKey: tablesView,
        onChange: (key) => setTablesViewAndUrl(key as TablesView),
        ariaLabel: tSpaces("nav.ariaLabel"),
      }}
    >
      {isSpacesView ? (
        <SpacesOverview
          businessId={businessId}
          initialSpaceId={initialSpaceId}
          onOpenSpace={handleOpenSpace}
          existingTables={tables
            .filter((row) => row.table.is_active)
            .map((row) => row.table)}
        />
      ) : (
        <>
      {hasPublishedSpace && (
        <div
          className="mb-3 inline-flex rounded-full border border-warm-200 bg-white p-0.5 shadow-sm"
          role="tablist"
          aria-label={tSpaces("liveMap.modeAria")}
          data-testid="live-mode-toggle"
        >
          {(
            [
              { key: "list" as const, label: tSpaces("liveMap.modeList"), Icon: List },
              { key: "map" as const, label: tSpaces("liveMap.modeMap"), Icon: MapIcon },
            ] as const
          ).map(({ key, label, Icon }) => {
            const active = liveMode === key;
            return (
              <button
                key={key}
                type="button"
                role="tab"
                aria-selected={active}
                data-testid={`live-mode-${key}`}
                onClick={() => setLiveMode(key)}
                className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium transition-colors ${
                  active
                    ? "bg-brand text-white shadow-sm"
                    : "text-ink-600 hover:bg-warm-50 hover:text-ink-900"
                }`}
              >
                <Icon className="h-3.5 w-3.5" aria-hidden />
                {label}
              </button>
            );
          })}
        </div>
      )}

      {liveMode === "map" && hasPublishedSpace ? (
        <TablesLiveMap
          businessId={businessId}
          tables={tables}
          statusFilter={
            statusFilter === "available" ||
            statusFilter === "occupied" ||
            statusFilter === "reserved"
              ? statusFilter
              : "all"
          }
          onStatusFilterChange={(next) => setStatusFilter(next)}
          onSelectTable={(id) => setSelectedId(id)}
          selectedTableId={selectedId}
          t={tSpaces}
          statusLabel={(s) => tString(`tableStatus.${s}`)}
        />
      ) : (
      <Toolbar
        search={{
          value: searchQuery,
          onChange: setSearchQuery,
          placeholder: tString("search.placeholder"),
        }}
      >
        <Select
          aria-label={tString("statusFilter.all")}
          selectedKeys={[statusFilter]}
          onSelectionChange={(keys) => {
            const next = Array.from(keys)[0] as StatusFilter | undefined;
            if (next) setStatusFilter(next);
          }}
          size="sm"
          variant="bordered"
          className="w-44"
          classNames={{
            trigger:
              "h-10 min-h-10 bg-white border-warm-200 data-[hover=true]:border-brand/40",
          }}
          // Single collection array (never static SelectItem siblings mixed
          // with a mapped set) so the option set stays TS2322-safe. The
          // Inactive segment only appears once there are inactive tables to
          // reach, so the control stays clean for the common case.
          items={[
            { key: "all", label: tString("statusFilter.all") },
            { key: "available", label: tString("tableStatus.available") },
            { key: "occupied", label: tString("tableStatus.occupied") },
            { key: "reserved", label: tString("tableStatus.reserved") },
            ...(hasInactiveTables
              ? [{ key: "inactive", label: tString("tableStatus.inactive") }]
              : []),
          ]}
        >
          {(item) => <SelectItem key={item.key}>{item.label}</SelectItem>}
        </Select>
        {(searchQuery || statusFilter !== "all") && (
          <button
            type="button"
            onClick={clearSearch}
            className={btnGhostIcon}
            aria-label={tString("buttons.clear")}
            title={tString("buttons.clear")}
          >
            <X className="h-4 w-4" />
          </button>
        )}
        <button
          type="button"
          onClick={() => void loadTables()}
          aria-label={tString("buttons.refreshAria")}
          title={tString("buttons.refreshAria")}
          className={btnGhostIcon}
        >
          <RefreshCw className={`h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
        </button>
      </Toolbar>
      )}

      {error && (
        <div className="bg-rose-50 border border-rose-200 rounded-lg p-3 mb-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="w-5 h-5 bg-rose-100 rounded flex items-center justify-center">
                <X className="w-3 h-3 text-rose-600" />
              </div>
              <p className="text-rose-700 text-sm">{error}</p>
            </div>
            <button
              onClick={() => setError(null)}
              className="text-rose-600 hover:text-rose-800 text-sm"
            >
              ×
            </button>
          </div>
        </div>
      )}

      {/* PV-LIVE-20260720-009: open service calls (table + reason + ack/handle) */}
      <ServiceCallsQueue businessId={businessId} />

      {hasFirstPaidBill === false && rows.length > 0 && (
        <div className="mb-4 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-xl border border-warm-200 bg-warm-50/60 px-4 py-3">
          <p className="text-sm text-ink-700">
            <span className="font-medium text-ink-900">
              {tString("noOrdersBanner.title")}
            </span>{" "}
            — {tString("noOrdersBanner.body")}
          </p>
          <div className="flex items-center gap-4">
            <button
              type="button"
              onClick={() => void printAll()}
              className="text-sm font-medium text-brand hover:text-brand-dark"
            >
              {tString("noOrdersBanner.printCta")} →
            </button>
            {firstActiveTableCode && (
              <a
                href={`/t/${firstActiveTableCode}`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-sm font-medium text-brand hover:text-brand-dark"
              >
                {tString("noOrdersBanner.previewCta")} →
              </a>
            )}
          </div>
        </div>
      )}

      {liveMode !== "map" || !hasPublishedSpace ? (
        tables.length === 0 ? (
          <TableEmptyPanel mode="firstRun" tString={tString} onAction={onCreateOpen} />
        ) : filteredRows.length === 0 ? (
          <TableEmptyPanel mode="filtered" tString={tString} onAction={clearSearch} />
        ) : (
          <PremiumPanel className="overflow-x-auto" withTexture={false}>
            {/* Auto layout + min-width: table-fixed crushed NAME / CURRENT BILL
                into one-letter columns at 1280 + sidebar. */}
            <table
              className="w-full min-w-[72rem] text-sm"
              data-testid="tables-live-table"
            >
              <thead className="bg-warm-50 text-ink-500 text-xs uppercase tracking-wider">
                <tr>
                  <th className="px-3 py-2 text-left w-[9rem] whitespace-nowrap">{tString("columns.code")}</th>
                  <th className="px-3 py-2 text-left min-w-[8rem] whitespace-nowrap">{tString("columns.name")}</th>
                  <th className="px-3 py-2 text-left w-16 whitespace-nowrap">{tString("columns.seats")}</th>
                  <th className="px-3 py-2 text-left w-36 whitespace-nowrap">{tString("columns.status")}</th>
                  <th className="px-3 py-2 text-left w-20 whitespace-nowrap">{tString("columns.covers")}</th>
                  <th className="px-3 py-2 text-left w-28 whitespace-nowrap">{tString("columns.server")}</th>
                  <th className="px-3 py-2 text-left w-28 whitespace-nowrap">{tString("columns.seated")}</th>
                  <th className="px-3 py-2 text-left min-w-[9rem] whitespace-nowrap">{tString("columns.currentBill")}</th>
                  <th className="px-3 py-2 text-left w-28 whitespace-nowrap">{tString("columns.lastSeen")}</th>
                  <th className="px-3 py-2 text-right w-40 whitespace-nowrap">
                    <span className="sr-only">{tString("columns.actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-warm-100">
                {filteredRows.map((t) => (
                  <TableRow
                    key={t.id}
                    table={t}
                    currency={businessCurrency}
                    businessTimezone={businessTimezone}
                    businessId={businessId}
                    hostTargets={rows.map((row) => ({
                      id: row.id,
                      name: row.name,
                      status: row.status,
                      is_active: row.is_active,
                      has_open_bill: !!row.active_bill,
                    }))}
                    onHostActionComplete={loadTables}
                    qrFallback={{
                      foregroundColor: qrDefaults?.qr_foreground_color,
                      backgroundColor: qrDefaults?.qr_background_color,
                    }}
                    onSelect={(row) => setSelectedId(row.id)}
                  />
                ))}
              </tbody>
            </table>
          </PremiumPanel>
        )
      ) : null}

      {selectedRow && (
        <TableDetailModal
          table={selectedRow}
          open
          onClose={() => setSelectedId(null)}
          businessId={businessId}
          businessName={businessName || businessNameProp}
          qrDefaults={qrDefaults}
          currency={businessCurrency}
          onDelete={handleDeleteRequest}
          onToggleActive={handleToggleActive}
          onRename={handleRename}
          onCapacityChange={handleCapacityChange}
          onCustomizeQR={handleRequestCustomize}
        />
      )}

      {/* Create-only modal. Editing moved into the drawer's Settings/Detail
          tabs, so TableModals no longer carries edit props. */}
      <TableModals
        isCreateOpen={isCreateOpen}
        onCreateOpenChange={onCreateOpenChange}
        tableName={tableName}
        setTableName={setTableName}
        tableCapacity={tableCapacity}
        setTableCapacity={setTableCapacity}
        handleCreateTable={handleCreateTable}
        isCreating={creatingTable}
      />

      {customizingTable && (
        <QRCustomizationModal
          isOpen={isCustomizeOpen}
          onClose={onCustomizeClose}
          table={customizingTable}
          businessId={businessId}
          businessName={businessName || businessNameProp}
          onSave={handleSaveCustomization}
          isSaving={savingCustomization}
        />
      )}
      <ConfirmationModal
        isOpen={isDeleteOpen}
        onOpenChange={onDeleteOpenChange}
        title={tString("buttons.delete")}
        description={tString("confirmDelete")}
        cancelLabel={tString("modals.create.cancel")}
        confirmLabel={tString("buttons.delete")}
        isDanger
        onConfirm={() => {
          void confirmDeleteTable();
        }}
      />
      <ConfirmationModal
        isOpen={isApplyAllOpen}
        onOpenChange={onApplyAllOpenChange}
        title={tString("qrCustomization.applyToAll")}
        description={tString("qrApplyAllConfirm", { count: rows.length })}
        cancelLabel={tString("modals.create.cancel")}
        confirmLabel={tString("qrCustomization.applyToAll")}
        onConfirm={() => {
          void confirmApplyToAll();
        }}
      />
        </>
      )}
    </DashboardTabShell>
  );
}
