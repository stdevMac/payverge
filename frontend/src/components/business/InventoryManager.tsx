"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";
import { useSubTab } from "@/lib/subTabs";

import React, { useCallback, useEffect, useId, useMemo, useState } from "react";
import { Autocomplete, AutocompleteItem, AutocompleteSection, Button, Chip, Input, Pagination, Select, SelectItem, Table, TableBody, TableCell, TableColumn, TableHeader, TableRow } from "@nextui-org/react";
import { NamedSwitch } from "@/components/ui/NamedSwitch";
import { NamedSelect } from "@/components/ui/NamedSelect";
import {
  AlertTriangle,
  Ban,
  Boxes,
  ChevronDown,
  ChevronUp,
  PackagePlus,
  Pencil,
  RefreshCw,
  SearchX,
  SlidersHorizontal,
  Trash2,
  Truck,
  X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import toast from "react-hot-toast";

import {
  getBusiness,
  getMenu,
  MenuCategory,
  updateMenuItem,
} from "@/api/business";
import { formatCurrency } from "@/api/currency";
import { InventoryItem, InventoryItemHealth, InventoryMenuItemStatus, InventoryRecipe, InventorySettings, inventoryApi } from "@/api/inventory";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { formatCount, formatUnit } from "@/utils/formatUnit";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { intlLocaleFor } from "@/utils/intlLocale";
import { parseMenuCategories } from "@/utils/businessDataParsers";
import DashboardLockedTabView from "./DashboardLockedTabView";
import SaveBar from "./SaveBar";
import { InventorySkeleton } from "./InventorySkeleton";
import InventoryToggle from "./InventoryToggle";
import InventoryItemModal from "./inventory/InventoryItemModal";
import {
  itemStatus,
  selectItemValue,
  selectTotalStockValue,
} from "./inventory/inventorySelectors";
import {
  InventoryFilterState,
  useInventoryFilter,
} from "./inventory/useInventoryFilter";
import { buildInventoryCsv, downloadCsv } from "./inventory/inventoryCsv";
import InventoryToolbar from "./inventory/InventoryToolbar";
import QuickAdjustDrawer from "./inventory/QuickAdjustDrawer";
import ItemDetailDrawer from "./inventory/ItemDetailDrawer";
import { buildReorderRows } from "./inventory/reorderList";
import ReorderListModal from "./inventory/ReorderListModal";
import {
  buildMenuItemIndex,
  canOfferEightySix,
  dishesUsingInventoryItem,
  liveMenuRefsFromIndex,
  mappedSellableDishes,
} from "./inventory/inventoryEightySix";
import { useInventoryMovements } from "./inventory/useInventoryMovements";
import ConfirmationModal from "./modals/ConfirmationModal";
import SegmentedTabs from "./shared/SegmentedTabs";
import DashboardTabShell from "./shared/DashboardTabShell";
import { btnGhostIcon, btnPrimaryNextUI, touchIconBtn } from "@/components/ui/buttonStyles";
import { DashboardTabTransition, PremiumPanel } from "./premium";

interface InventoryManagerProps {
  businessId: number;
  /** Wave 4: open Menu Builder focused on a low-stock menu item. */
  onNavigateToTab?: (tab: string) => void;
}

interface RecipeDraftRow {
  inventoryItemId: string;
  quantityRequired: string;
}

interface FlatMenuItem {
  id: string;
  name: string;
  category: string;
}

function parseMenuItems(menuResponse: {
  // categories is optional to match the operator GET /menu wire contract, which
  // ships parsed_categories only (§3.7 slim); parseMenuCategories tolerates absence.
  categories?: MenuCategory[] | string | null;
  parsed_categories?: MenuCategory[];
}): FlatMenuItem[] {
  const categories = parseMenuCategories(menuResponse);

  return categories.flatMap((category) =>
    (category.items || [])
      .filter((item) => !!item.id)
      .map((item) => ({
        id: item.id as string,
        name: item.name,
        category: category.name,
      })),
  );
}

// Movement-type filter options for the Activity tab, in the order the operator
// scans them. Values are the backend enum keys the server filters on.
const MOVEMENT_TYPE_KEYS = [
  "purchase",
  "manual_adjustment",
  "restock",
  "waste",
  "correction",
  "order_consumption",
  "order_restoration",
] as const;

// S-4: locale-aware quantity (shared with formatUnit) so movement deltas
// match Items/Reorder under es ("-0,35" not "-0.35").

function statusTone(
  status: string,
): "primary" | "warning" | "danger" | "default" {
  switch (status) {
    case "low_stock":
      return "warning";
    case "out_of_stock":
    case "manual_unavailable":
      return "danger";
    case "ok":
    case "healthy":
      return "primary";
    default:
      return "default";
  }
}

type InventoryEmptyPanelProps = {
  actionLabel: string;
  body: string;
  hint?: string;
  icon: LucideIcon;
  onAction: () => void;
  title: string;
  variant: "firstRun" | "filtered";
};

const inventoryMetricLabelClass =
  "text-sm font-medium leading-5 text-ink-600";
const inventoryMetricValueClass =
  "mt-1 text-2xl font-semibold tabular-nums text-ink-950";
const inventorySectionShellClass =
  "rounded-2xl border border-warm-200/80 bg-white/90 p-5 shadow-sm shadow-warm-900/5";
const inventorySectionTitleClass =
  "text-lg font-semibold text-ink-950";
const inventorySectionCopyClass = "text-sm leading-6 text-ink-600";
const inventoryTableHeaderClass =
  "bg-warm-50 text-xs font-semibold uppercase tracking-[0.12em] text-ink-600";
const inventoryEmptyIconWrapClass =
  "mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl border border-brand/10 bg-brand/5 text-brand";
const inventoryActivityCardClass =
  "rounded-2xl border border-warm-200/80 bg-white/85 px-4 py-3 shadow-sm shadow-warm-900/5";
const inventorySettingCardClass =
  "flex items-center justify-between gap-4 rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3";

function InventoryEmptyPanel({
  actionLabel,
  body,
  hint,
  icon: Icon,
  onAction,
  title,
  variant,
}: InventoryEmptyPanelProps) {
  const isFirstRun = variant === "firstRun";

  return (
    <div className="overflow-hidden rounded-2xl border border-brand/10 bg-brand/5 p-4 sm:p-5">
      <div>
        <div>
          <div className="mb-5 flex h-12 w-12 items-center justify-center rounded-2xl border border-white/80 bg-white shadow-sm">
            <Icon className="h-5 w-5 text-brand" strokeWidth={1.7} />
          </div>
          <h3 className="font-title text-2xl text-ink-950">{title}</h3>
          <p className="mt-3 max-w-xl text-sm leading-6 text-ink-600">
            {body}
          </p>
          {hint ? (
            <p className="mt-2 max-w-xl text-xs leading-5 text-ink-500">
              {hint}
            </p>
          ) : null}
          <Button
            className="mt-6"
            color={isFirstRun ? "primary" : "default"}
            variant={isFirstRun ? "solid" : "flat"}
            startContent={
              isFirstRun ? (
                <PackagePlus className="h-4 w-4" />
              ) : (
                <SlidersHorizontal className="h-4 w-4" />
              )
            }
            onPress={onAction}
          >
            {actionLabel}
          </Button>
        </div>

      </div>
    </div>
  );
}

export default function InventoryManager({
  businessId,
  onNavigateToTab,
}: InventoryManagerProps) {
  const { locale } = useSimpleLocale();
  const {
    hasAccess,
    isSuspended,
    loading: accessLoading,
  } = useBusinessAccess(businessId);

  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [savingSettings, setSavingSettings] = useState(false);
  const [savingRecipe, setSavingRecipe] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Pending inventory-item deletion — a destructive, irreversible action
  // (drops the item from view and permanently removes its recipe links), so it
  // must be confirmed before firing the DELETE.
  const [pendingDelete, setPendingDelete] = useState<{
    id: number;
    name: string;
  } | null>(null);
  // Set when Save would clear the LAST row of an existing recipe (which deletes
  // the recipe entirely, silently untracking the dish). Confirmed before firing.
  const [pendingRecipeClear, setPendingRecipeClear] = useState(false);
  const INVENTORY_SUBS = ["items", "recipes", "activity", "settings"] as const;
  const { sub, unknownSub, setSub } = useSubTab(
    "inventory",
    INVENTORY_SUBS,
    "items",
  );
  const activeTab = sub ?? "items";
  const setActiveTab = (key: string) => {
    if ((INVENTORY_SUBS as readonly string[]).includes(key)) {
      setSub(key as (typeof INVENTORY_SUBS)[number]);
    }
  };
  useEffect(() => {
    if (unknownSub) setSub("items");
  }, [unknownSub, setSub]);

  const [settings, setSettings] = useState<InventorySettings | null>(null);
  const [items, setItems] = useState<InventoryItem[]>([]);
  const [recipes, setRecipes] = useState<InventoryRecipe[]>([]);
  const [menuItems, setMenuItems] = useState<FlatMenuItem[]>([]);
  const [menuCategories, setMenuCategories] = useState<MenuCategory[]>([]);
  const [menuVersion, setMenuVersion] = useState<number | undefined>(undefined);
  const [menuStatuses, setMenuStatuses] = useState<
    Record<string, InventoryMenuItemStatus>
  >({});
  const [lowStockDetails, setLowStockDetails] = useState<InventoryItemHealth[]>(
    [],
  );
  const [outOfStockDetails, setOutOfStockDetails] = useState<
    InventoryItemHealth[]
  >([]);
  // Server-computed stat-card figures — the single source of truth for the
  // low/out-of-stock counts and total stock value, so the cards don't recompute
  // from the client-held item list (audit §3.5 LOW, fix 9).
  const [summaryStats, setSummaryStats] = useState<{
    lowStockItems: number;
    outOfStockItems: number;
    totalStockValue: number;
  } | null>(null);
  const [reorderModalOpen, setReorderModalOpen] = useState(false);

  // Item modal state — the form itself lives inside <InventoryItemModal>
  // so nothing leaks between opens. `initialEditItem` is the row being
  // edited (null = create mode).
  const [itemModalOpen, setItemModalOpen] = useState(false);
  const [initialEditItem, setInitialEditItem] = useState<InventoryItem | null>(
    null,
  );
  const [selectedRecipeMenuItemId, setSelectedRecipeMenuItemId] = useState("");
  const [selectedRecipeMenuItemName, setSelectedRecipeMenuItemName] =
    useState("");
  const [recipeDraft, setRecipeDraft] = useState<RecipeDraftRow[]>([
    { inventoryItemId: "", quantityRequired: "1" },
  ]);
  // Set when the operator tries to switch the recipe menu-item Select while the
  // current recipe has unsaved ingredient edits. Holds the pending target id
  // (a bare "" is truthy-blocked, so we wrap it) until the discard is confirmed.
  const [pendingRecipeSwitch, setPendingRecipeSwitch] = useState<{
    id: string;
  } | null>(null);
  const [itemsPage, setItemsPage] = useState(1);
  const ITEMS_PER_PAGE = 25;

  // Facelift composition state: business currency, the toolbar filter, the
  // collapsible needs-attention strip, and the two right-side drawers.
  const [currencyCode, setCurrencyCode] = useState("USD");
  // Business IANA timezone drives movement-timestamp rendering so operators see
  // wall-clock in the restaurant's zone, never the viewing device's (R17).
  // null falls back to UTC in formatBusinessDateTime.
  const [businessTimezone, setBusinessTimezone] = useState<string | null>(null);
  // True when the business (and thus its currency) couldn't be resolved, so the
  // stock-value figures fall back to USD. Surfaced as a non-blocking note rather
  // than silently mislabelling money in the wrong currency (INV-L6).
  const [currencyUnavailable, setCurrencyUnavailable] = useState(false);
  const [filterState, setFilterState] = useState<InventoryFilterState>({
    search: "",
    category: "",
    status: "all",
    sort: "attention",
  });
  const [pendingEightySix, setPendingEightySix] = useState<{
    item: InventoryItem;
    dishes: InventoryMenuItemStatus[];
  } | null>(null);
  const [eightySixBusy, setEightySixBusy] = useState(false);
  const [needsAttentionOpen, setNeedsAttentionOpen] = useState(false);
  const [detailItem, setDetailItem] = useState<InventoryItem | null>(null);
  const [adjustItem, setAdjustItem] = useState<InventoryItem | null>(null);
  const [adjustOpen, setAdjustOpen] = useState(false);
  const patchFilter = useCallback(
    (patch: Partial<InventoryFilterState>) =>
      setFilterState((c) => ({ ...c, ...patch })),
    [],
  );

  const [activityItemFilter, setActivityItemFilter] = useState<string>("");
  const [activityTypeFilter, setActivityTypeFilter] = useState<string>("");
  const settingsA11yId = useId();

  // The Activity tab reads a server-paginated, server-filtered ledger — not a
  // browser-side slice of 25 global rows (audit §3.5 HIGH/C1). Only fetches
  // while the Activity tab is the active surface.
  const activityLedger = useInventoryMovements({
    businessId,
    itemFilter: activityItemFilter,
    typeFilter: activityTypeFilter,
    enabled: activeTab === "activity",
  });

  const t = useCallback(
    (key: string): string => {
      const result = getTranslation(
        `businessDashboard.inventoryManager.${key}`,
        locale,
      );
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const tWith = useCallback(
    (key: string, replacements: Record<string, string | number>): string => {
      let value = t(key);
      Object.entries(replacements).forEach(([name, replacement]) => {
        value = value.replace(
          new RegExp(`\\{${name}\\}`, "g"),
          String(replacement),
        );
      });
      return value;
    },
    [t],
  );

  // Shared SaveBar copy lives under businessSettings.saveBar.* so idle / dirty
  // / autosave affordances read identically across settings tabs.
  const saveBarString = useCallback(
    (key: string): string => {
      const result = getTranslation(`businessSettings.saveBar.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // Resolve to a canonical BCP-47 tag so es-AR operators get the Argentine
  // prefixed-currency convention ("US$ 212,10"), not es-ES's suffix ("212,10 US$")
  // that the old binary es-ES/en-US ternary forced on every Spanish locale (H5).
  const localeTag = intlLocaleFor(locale ?? "");

  const statusLabel = useCallback(
    (status: string) => {
      switch (status) {
        case "low_stock":
          return t("status.lowStock");
        case "out_of_stock":
          return t("status.outOfStock");
        case "manual_unavailable":
          return t("status.manualUnavailable");
        case "untracked":
          return t("status.untracked");
        case "healthy":
          return t("status.healthy");
        default:
          return t("status.ok");
      }
    },
    [t],
  );

  // Inventory item badges use the SAME three words as the toolbar filter chips
  // (Healthy / Low / Out) so a brand-new operator who clicks the "Healthy"
  // filter sees items badged with the matching word — no "Healthy" pill ⇄ "OK"
  // badge mismatch. Kept separate from the shared `statusLabel`, which also
  // labels menu-item statuses (manual_unavailable / untracked) in the Recipes
  // tab and must not change. Both still derive from the one `itemStatus`.
  const inventoryStatusLabel = useCallback(
    (status: string) => {
      switch (status) {
        case "out_of_stock":
          return t("toolbar.statusOut");
        case "low_stock":
          return t("toolbar.statusLow");
        default:
          return t("toolbar.statusHealthy");
      }
    },
    [t],
  );

  // Movement reasons written by QuickAdjustDrawer are STABLE enum keys
  // (spoilage, prep_waste, …, physical_count) rather than translated labels,
  // so the ledger data is locale-independent. Translate the known keys back at
  // render time; anything else (free-text notes, legacy rows) is shown as-is.
  // See R3-AC.
  const reasonLabel = useCallback(
    (reason: string): string => {
      switch (reason) {
        case "spoilage":
        case "prep_waste":
        case "server_error":
        case "quality_reject":
        case "other":
          return t(`quickAdjust.reasons.${reason}`);
        case "physical_count":
          return t("quickAdjust.reasons.physicalCount");
        default:
          return reason;
      }
    },
    [t],
  );

  const movementTypeLabel = useCallback(
    (movementType: string) => {
      switch (movementType) {
        case "purchase":
          return t("movements.types.purchase");
        case "manual_adjustment":
          return t("movements.types.manualAdjustment");
        case "waste":
          return t("movements.types.waste");
        case "restock":
          return t("movements.types.restock");
        case "correction":
          return t("movements.types.correction");
        case "order_consumption":
          return t("movements.types.orderConsumption");
        case "order_restoration":
          return t("movements.types.orderRestoration");
        default:
          return movementType.replace(/_/g, " ");
      }
    },
    [t],
  );

  const loadInventory = useCallback(
    async (isRefresh = false) => {
      if (isRefresh) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      setError(null);

      try {
        const [
          settingsData,
          itemsData,
          recipesData,
          summaryData,
          menuData,
          businessData,
        ] = await Promise.all([
          inventoryApi.getSettings(businessId),
          inventoryApi.listItems(businessId),
          inventoryApi.listRecipes(businessId),
          inventoryApi.getSummary(businessId),
          getMenu(businessId).catch(() => ({
            categories: [],
            parsed_categories: [],
            version: undefined as number | undefined,
          })),
          // Business currency drives the stock-value formatting. Non-fatal —
          // a failed read just keeps the USD default.
          getBusiness(businessId).catch(() => null),
        ]);

        setSettings(settingsData);
        setItems(itemsData);
        setRecipes(recipesData);
        setMenuItems(parseMenuItems(menuData));
        setMenuCategories(parseMenuCategories(menuData));
        setMenuVersion(
          typeof menuData.version === "number" ? menuData.version : undefined,
        );
        if (businessData?.default_currency) {
          setCurrencyCode(businessData.default_currency);
          setCurrencyUnavailable(false);
        } else {
          // Business load failed or carried no currency — values stay in USD.
          setCurrencyUnavailable(true);
        }
        // Movement timestamps render in the business's zone (R17); null/absent
        // safely falls back to UTC, never the viewing device's timezone.
        setBusinessTimezone(businessData?.timezone ?? null);
        setMenuStatuses(
          Object.fromEntries(
            (summaryData.menu_item_statuses || []).map((status) => [
              status.menu_item_id,
              status,
            ]),
          ),
        );
        setLowStockDetails(summaryData.low_stock_details || []);
        setOutOfStockDetails(summaryData.out_of_stock_details || []);
        setSummaryStats({
          lowStockItems: summaryData.low_stock_items,
          outOfStockItems: summaryData.out_of_stock_items,
          totalStockValue: summaryData.total_stock_value,
        });
      } catch (err) {
        console.error("Failed to load inventory:", err);
        setError(getSafeApiErrorMessage(err, t("errors.loadData")));
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [businessId, t],
  );

  useEffect(() => {
    if (!accessLoading && hasAccess && !isSuspended) {
      void loadInventory();
    }
  }, [loadInventory, accessLoading, hasAccess, isSuspended]);

  // Refresh ONLY the derived summary (low/out-of-stock details, menu statuses,
  // the stat cards) after a stock-affecting change, instead of re-running all
  // mount fetches (audit §3.5 HIGH/C1). Items/recipes/menu are patched in place
  // by the caller. Non-fatal — a summary refresh failure leaves the patched
  // item state intact.
  const refreshSummary = useCallback(async () => {
    try {
      const summaryData = await inventoryApi.getSummary(businessId);
      setSettings(summaryData.settings);
      setMenuStatuses(
        Object.fromEntries(
          (summaryData.menu_item_statuses || []).map((status) => [
            status.menu_item_id,
            status,
          ]),
        ),
      );
      setLowStockDetails(summaryData.low_stock_details || []);
      setOutOfStockDetails(summaryData.out_of_stock_details || []);
      setSummaryStats({
        lowStockItems: summaryData.low_stock_items,
        outOfStockItems: summaryData.out_of_stock_items,
        totalStockValue: summaryData.total_stock_value,
      });
    } catch (err) {
      console.error("Failed to refresh inventory summary:", err);
    }
  }, [businessId]);

  const recipeGroups = useMemo(() => {
    const grouped = new Map<string, InventoryRecipe[]>();
    recipes.forEach((recipe) => {
      const key = recipe.menu_item_id;
      const current = grouped.get(key) || [];
      current.push(recipe);
      grouped.set(key, current);
    });
    return grouped;
  }, [recipes]);

  // Menu items grouped by category for the recipe menu-item Autocomplete —
  // typeahead + sectioned so a 1,000-item menu is scannable, not an 8k-option
  // native <select> DOM (audit §3.5 MED).
  const groupedMenuItems = useMemo(() => {
    const byCategory = new Map<string, FlatMenuItem[]>();
    menuItems.forEach((item) => {
      const key = item.category || t("recipes.uncategorized");
      const list = byCategory.get(key) || [];
      list.push(item);
      byCategory.set(key, list);
    });
    return Array.from(byCategory.entries()).map(([category, groupItems]) => ({
      category,
      items: groupItems,
    }));
  }, [menuItems, t]);

  // Inventory items grouped by category for the ingredient Autocomplete.
  const groupedInventoryItems = useMemo(() => {
    const byCategory = new Map<string, InventoryItem[]>();
    items.forEach((item) => {
      const key = item.category || t("recipes.uncategorized");
      const list = byCategory.get(key) || [];
      list.push(item);
      byCategory.set(key, list);
    });
    return Array.from(byCategory.entries()).map(([category, groupItems]) => ({
      category,
      items: groupItems,
    }));
  }, [items, t]);

  useEffect(() => {
    if (!selectedRecipeMenuItemId) {
      setRecipeDraft([{ inventoryItemId: "", quantityRequired: "1" }]);
      setSelectedRecipeMenuItemName("");
      return;
    }

    const selectedMenuItem = menuItems.find(
      (item) => item.id === selectedRecipeMenuItemId,
    );
    setSelectedRecipeMenuItemName(selectedMenuItem?.name || "");

    const existing = recipeGroups.get(selectedRecipeMenuItemId) || [];
    if (existing.length === 0) {
      setRecipeDraft([{ inventoryItemId: "", quantityRequired: "1" }]);
      return;
    }

    setRecipeDraft(
      existing.map((recipe) => ({
        inventoryItemId: String(recipe.inventory_item_id),
        quantityRequired: String(recipe.quantity_required),
      })),
    );
  }, [menuItems, recipeGroups, selectedRecipeMenuItemId]);

  // The saved-recipe baseline for the currently selected menu item, derived the
  // SAME way the restore effect above builds recipeDraft. Comparing the live
  // draft against this tells us whether there are unsaved ingredient edits.
  const recipeBaseline = useMemo<RecipeDraftRow[]>(() => {
    if (!selectedRecipeMenuItemId) {
      return [{ inventoryItemId: "", quantityRequired: "1" }];
    }
    const existing = recipeGroups.get(selectedRecipeMenuItemId) || [];
    if (existing.length === 0) {
      return [{ inventoryItemId: "", quantityRequired: "1" }];
    }
    return existing.map((recipe) => ({
      inventoryItemId: String(recipe.inventory_item_id),
      quantityRequired: String(recipe.quantity_required),
    }));
  }, [recipeGroups, selectedRecipeMenuItemId]);

  // True when the live draft diverges from the saved recipe — i.e. switching
  // menu items now would silently discard unsaved ingredient rows.
  const recipeDirty = useMemo(() => {
    if (!selectedRecipeMenuItemId) {
      return false;
    }
    if (recipeDraft.length !== recipeBaseline.length) {
      return true;
    }
    return recipeDraft.some(
      (row, i) =>
        row.inventoryItemId !== recipeBaseline[i].inventoryItemId ||
        row.quantityRequired !== recipeBaseline[i].quantityRequired,
    );
  }, [recipeBaseline, recipeDraft, selectedRecipeMenuItemId]);

  // Guards the recipe menu-item Select: if the current recipe has unsaved
  // edits, stash the pending target and open the discard-confirm modal instead
  // of switching immediately (which would silently drop the edits). Otherwise
  // switch straight away.
  const requestRecipeMenuItemChange = useCallback(
    (nextId: string) => {
      if (recipeDirty && nextId !== selectedRecipeMenuItemId) {
        setPendingRecipeSwitch({ id: nextId });
        return;
      }
      setSelectedRecipeMenuItemId(nextId);
    },
    [recipeDirty, selectedRecipeMenuItemId],
  );

  const reorderRows = useMemo(
    () => buildReorderRows(lowStockDetails, outOfStockDetails),
    [lowStockDetails, outOfStockDetails],
  );

  const lowStockItems = useMemo(
    () => items.filter((item) => itemStatus(item) === "low_stock"),
    [items],
  );

  const outOfStockItems = useMemo(
    () => items.filter((item) => itemStatus(item) === "out_of_stock"),
    [items],
  );
  const hasStockAlerts = outOfStockItems.length > 0 || lowStockItems.length > 0;

  const totalStockValue = useMemo(() => selectTotalStockValue(items), [items]);
  const filtered = useInventoryFilter(items, filterState);

  // Stat-card figures prefer the server summary (single source of truth), and
  // fall back to the client computation only until the summary has loaded so the
  // cards aren't blank on first paint (audit §3.5 LOW, fix 9).
  const lowStockCount = summaryStats?.lowStockItems ?? lowStockItems.length;
  const outOfStockCount =
    summaryStats?.outOfStockItems ?? outOfStockItems.length;
  const stockValueDisplay = summaryStats?.totalStockValue ?? totalStockValue;

  // The detail drawer stays mounted; only derive its props when an item is
  // actually selected so frequent loadInventory re-renders don't recompute
  // (and discard) these while detailItem is null. Its "Recent movements" section
  // now fetches per-item server-side (load-more) inside the drawer.
  const detailUsedBy = useMemo(
    () =>
      detailItem
        ? Object.values(menuStatuses).filter((s) =>
            recipes.some(
              (r) =>
                r.menu_item_id === s.menu_item_id &&
                r.inventory_item_id === detailItem.id,
            ),
          )
        : [],
    [detailItem, menuStatuses, recipes],
  );
  const fmtCurrency = useCallback(
    (amount: number) =>
      formatCurrency(amount, currencyCode, undefined, localeTag),
    [currencyCode, localeTag],
  );

  const itemsTotalPages = Math.max(
    1,
    Math.ceil(filtered.visibleItems.length / ITEMS_PER_PAGE),
  );
  useEffect(() => {
    if (itemsPage > itemsTotalPages) {
      setItemsPage(itemsTotalPages);
    }
  }, [itemsPage, itemsTotalPages]);
  const pagedItems = useMemo(
    () =>
      filtered.visibleItems.slice(
        (itemsPage - 1) * ITEMS_PER_PAGE,
        itemsPage * ITEMS_PER_PAGE,
      ),
    [filtered.visibleItems, itemsPage],
  );

  const handleExportCsv = useCallback(() => {
    const csv = buildInventoryCsv(filtered.visibleItems, {
      statusLabel: (s) => inventoryStatusLabel(s),
      locale,
    });
    downloadCsv("inventory.csv", csv);
  }, [filtered.visibleItems, inventoryStatusLabel, locale]);

  const openAdjust = useCallback((item: InventoryItem) => {
    setAdjustItem(item);
    setAdjustOpen(true);
    setDetailItem(null);
  }, []);

  const menuItemIndex = useMemo(
    () => buildMenuItemIndex(menuCategories),
    [menuCategories],
  );
  const liveMenuRefs = useMemo(
    () => liveMenuRefsFromIndex(menuItemIndex),
    [menuItemIndex],
  );

  /** Dishes that consume this inventory SKU (via recipe mapping). */
  const dishesUsingItem = useCallback(
    (item: InventoryItem) =>
      dishesUsingInventoryItem(
        item,
        recipes,
        Object.values(menuStatuses),
        liveMenuRefs,
      ),
    [liveMenuRefs, menuStatuses, recipes],
  );

  const recipeCoverage = useMemo(() => {
    const mapped = Object.values(menuStatuses).filter((s) => s.has_recipe).length;
    const total = Math.max(menuItems.length, Object.keys(menuStatuses).length);
    return { mapped, total };
  }, [menuItems.length, menuStatuses]);

  const requestEightySix = useCallback(
    (item: InventoryItem) => {
      const dishes = mappedSellableDishes(
        item,
        recipes,
        Object.values(menuStatuses),
        liveMenuRefs,
      ).filter((dish) => Boolean(menuItemIndex[dish.menu_item_id]));
      if (!canOfferEightySix(item, dishes)) {
        if (item.current_quantity <= 0 && dishesUsingItem(item).length > 0) {
          setError(t("items.eightySixMissingMenu"));
        }
        return;
      }
      setPendingEightySix({ item, dishes });
    },
    [dishesUsingItem, liveMenuRefs, menuItemIndex, menuStatuses, recipes, t],
  );

  const confirmEightySix = useCallback(async () => {
    if (!pendingEightySix) return;
    setEightySixBusy(true);
    let version = menuVersion;
    let done = 0;
    let failed = 0;
    for (const dish of pendingEightySix.dishes) {
      const ref = menuItemIndex[dish.menu_item_id];
      if (!ref) {
        failed += 1;
        continue;
      }
      try {
        const result = await updateMenuItem(
          businessId,
          ref.categoryIndex,
          ref.itemIndex,
          {
            ...ref.item,
            description: ref.item.description || "",
            is_available: false,
          },
          version,
          ref.categoryId || undefined,
          ref.itemId,
        );
        if (result.requires_confirmation) {
          failed += 1;
          continue;
        }
        if (typeof result.version === "number") {
          version = result.version;
          setMenuVersion(result.version);
        }
        setMenuCategories((cats) =>
          cats.map((category, categoryIndex) =>
            categoryIndex !== ref.categoryIndex
              ? category
              : {
                  ...category,
                  items: (category.items || []).map((menuItem, itemIndex) =>
                    itemIndex !== ref.itemIndex
                      ? menuItem
                      : { ...(result.item ?? menuItem), is_available: false },
                  ),
                },
          ),
        );
        done += 1;
      } catch (err) {
        failed += 1;
        console.error("Failed to 86 menu item from inventory:", err);
      }
    }
    await refreshSummary();
    setEightySixBusy(false);
    setPendingEightySix(null);
    if (done > 0 && failed === 0) {
      toast.success(tWith("items.eightySixSuccess", { count: done }));
      return;
    }
    if (done > 0) {
      toast.error(
        tWith("items.eightySixPartial", {
          done,
          total: done + failed,
          failed,
        }),
      );
      return;
    }
    toast.error(t("items.eightySixError"));
  }, [
    businessId,
    menuItemIndex,
    menuVersion,
    pendingEightySix,
    refreshSummary,
    t,
    tWith,
  ]);

  const openItemModalForCreate = useCallback(() => {
    setInitialEditItem(null);
    setItemModalOpen(true);
  }, []);

  const openItemModalForEdit = useCallback((item: InventoryItem) => {
    setInitialEditItem(item);
    setItemModalOpen(true);
  }, []);

  const closeItemModal = useCallback(() => {
    setItemModalOpen(false);
    // Clearing the initial snapshot is deferred so the modal's close
    // animation doesn't briefly flash a blank edit body.
    setTimeout(() => setInitialEditItem(null), 150);
  }, []);

  const handleItemSaved = useCallback(
    async ({
      item,
      mode,
    }: {
      item: InventoryItem;
      mode: "create" | "edit";
    }) => {
      // L5-41: successful modal save must clear a prior failed-save banner.
      setError(null);
      // Patch the saved item into local state; refresh only the derived summary
      // (stock health may have changed). No full mount-fetch cascade.
      setItems((current) =>
        mode === "edit"
          ? current.map((it) => (it.id === item.id ? item : it))
          : [...current, item],
      );
      await refreshSummary();
    },
    [refreshSummary],
  );

  const handleSaveSettings = async (
    nextSettings: Partial<InventorySettings>,
  ) => {
    // L5-41: clear stale banner at the start of every mutating attempt.
    setError(null);
    try {
      setSavingSettings(true);
      const updated = await inventoryApi.updateSettings(
        businessId,
        nextSettings,
      );
      setSettings(updated);
      // A sync-mode / auto-deduct change can flip menu-item availability, so
      // refresh the derived summary — but not the whole inventory.
      await refreshSummary();
    } catch (err) {
      console.error("Failed to save inventory settings:", err);
      setError(getSafeApiErrorMessage(err, t("errors.saveSettings")));
    } finally {
      setSavingSettings(false);
    }
  };

  const handleInventoryStatusChange = useCallback(
    async (enabled: boolean) => {
      setSettings((current) =>
        current ? { ...current, inventory_enabled: enabled } : current,
      );
      await refreshSummary();
    },
    [refreshSummary],
  );

  const handleDeleteItem = async (itemId: number) => {
    setError(null);
    try {
      await inventoryApi.deleteItem(businessId, itemId);
      // If the row being edited is the one we just deleted, dismiss the
      // modal so we don't post an update against a missing record.
      if (initialEditItem?.id === itemId) {
        closeItemModal();
      }
      // Drop the row locally and refresh the derived summary (its recipe links
      // are gone, which can change menu-item availability).
      setItems((current) => current.filter((it) => it.id !== itemId));
      await refreshSummary();
    } catch (err) {
      console.error("Failed to delete inventory item:", err);
      setError(getSafeApiErrorMessage(err, t("errors.deleteItem")));
    }
  };

  const handleSaveRecipe = async () => {
    setError(null);
    if (!selectedRecipeMenuItemId) {
      setError(t("errors.selectMenuItem"));
      return;
    }

    const hasIncompleteRow = recipeDraft.some((row) => {
      const hasInventoryItem = row.inventoryItemId.trim() !== "";
      const hasQuantity = row.quantityRequired.trim() !== "";

      if (!hasInventoryItem && !hasQuantity) {
        return false;
      }

      const quantity = Number(row.quantityRequired);
      return (
        !hasInventoryItem ||
        !hasQuantity ||
        !Number.isFinite(quantity) ||
        quantity <= 0
      );
    });

    if (hasIncompleteRow) {
      setError(t("errors.completeRecipeRows"));
      return;
    }

    const entries = recipeDraft
      .map((row) => ({
        inventory_item_id: Number(row.inventoryItemId),
        quantity_required: Number(row.quantityRequired),
      }))
      .filter((row) => row.inventory_item_id > 0 && row.quantity_required > 0);

    // Saving with no valid rows sends an empty recipe, which the backend treats
    // as "delete this menu item's recipe". If the item currently HAS a recipe,
    // that silently untracks the dish (auto-deduction stops), so confirm first.
    const hasExistingRecipe =
      (recipeGroups.get(selectedRecipeMenuItemId)?.length ?? 0) > 0;
    if (entries.length === 0 && hasExistingRecipe) {
      setPendingRecipeClear(true);
      return;
    }

    await persistRecipe(entries);
  };

  const persistRecipe = async (
    entries: { inventory_item_id: number; quantity_required: number }[],
  ) => {
    setError(null);
    try {
      setSavingRecipe(true);
      await inventoryApi.replaceRecipe(
        businessId,
        selectedRecipeMenuItemId,
        selectedRecipeMenuItemName,
        entries,
      );
      // Refetch only the recipe list (the replace endpoint returns no body) and
      // refresh the derived summary (recipe changes flip menu availability).
      // Items/menu/settings are untouched, so they are not re-fetched.
      const [recipesData] = await Promise.all([
        inventoryApi.listRecipes(businessId),
        refreshSummary(),
      ]);
      setRecipes(recipesData);
    } catch (err) {
      console.error("Failed to save recipe:", err);
      setError(getSafeApiErrorMessage(err, t("errors.saveRecipe")));
    } finally {
      setSavingRecipe(false);
    }
  };

  return (
    <DashboardTabShell
      locked={
        !accessLoading && (!hasAccess || isSuspended) ? (
          <DashboardLockedTabView
            title={t("header.title")}
            subtitle={t("header.subtitle")}
            businessId={businessId}
          />
        ) : null
      }
      loading={loading || accessLoading ? <InventorySkeleton /> : null}
      header={{
        title: t("header.title"),
        subtitle: t("header.subtitle"),
        /* Stat cards inside the content are display-only. Status filtering
           lives on the toolbar chips so the two cannot disagree. */
        status: {
          label: settings?.inventory_enabled
            ? t("shell.activeMode")
            : t("shell.pausedMode"),
          tone: settings?.inventory_enabled ? "positive" : "neutral",
        },
        actions: (
          <button
            type="button"
            onClick={() => void loadInventory(true)}
            disabled={refreshing}
            aria-label={t("header.refresh")}
            title={t("header.refresh")}
            className={`${btnGhostIcon} disabled:opacity-50`}
          >
            <RefreshCw
              className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`}
            />
          </button>
        ),
      }}
    >

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          <span>{error}</span>
          <button
            type="button"
            onClick={() => setError(null)}
            aria-label={t("header.dismissError")}
            className="shrink-0 rounded-lg p-0.5 text-rose-600 transition-colors hover:bg-rose-100 hover:text-rose-800"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {!settings?.inventory_enabled ? (
        <InventoryToggle
          businessId={businessId}
          enabled={settings?.inventory_enabled ?? false}
          isLocked={!hasAccess || isSuspended}
          variant="card"
          onError={(message) => setError(message)}
          onStatusChange={handleInventoryStatusChange}
        />
      ) : (
        <>
          {/* Tabbed Content — the page-level header already shows the
              title + subtitle, so the card opens directly with the
              stats grid (was duplicated as an inner h2 below the
              identical page heading until 2026-05-13). */}
          <PremiumPanel className="overflow-hidden" withTexture={false}>
            <div className="p-6 space-y-6">
              {currencyUnavailable && (
                <div
                  className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800"
                  role="status"
                >
                  {t("valuation.currencyUnavailable")}
                </div>
              )}
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                <PremiumPanel
                  tone="accent"
                  className="px-4 py-3"
                  withTexture={false}
                >
                  <p className="text-sm text-emerald-700">
                    {t("valuation.stockValue")}
                  </p>
                  <p className="mt-1 text-2xl font-semibold tabular-nums text-emerald-900">
                    {fmtCurrency(stockValueDisplay)}
                  </p>
                </PremiumPanel>
                <PremiumPanel
                  className="w-full px-4 py-3 text-left"
                  withTexture={false}
                  data-testid="inventory-kpi-tracked"
                >
                  <p className={inventoryMetricLabelClass}>
                    {t("stats.trackedItems")}
                  </p>
                  <p className={inventoryMetricValueClass}>{items.length}</p>
                </PremiumPanel>
                <PremiumPanel
                  tone={lowStockCount > 0 ? "urgent" : "default"}
                  className="w-full px-4 py-3 text-left"
                  withTexture={false}
                  data-testid="inventory-kpi-low"
                >
                  <p
                    className={
                      lowStockCount > 0
                        ? "text-sm text-amber-700"
                        : inventoryMetricLabelClass
                    }
                  >
                    {t("stats.lowStockItems")}
                  </p>
                  <p
                    className={
                      lowStockCount > 0
                        ? "mt-1 text-2xl font-semibold tabular-nums text-amber-800"
                        : inventoryMetricValueClass
                    }
                  >
                    {lowStockCount}
                  </p>
                </PremiumPanel>
                <PremiumPanel
                  tone={outOfStockCount > 0 ? "danger" : "default"}
                  className="w-full px-4 py-3 text-left"
                  withTexture={false}
                  data-testid="inventory-kpi-out"
                >
                  <p
                    className={
                      outOfStockCount > 0
                        ? "text-sm font-medium leading-5 text-rose-700"
                        : inventoryMetricLabelClass
                    }
                  >
                    {t("stats.outOfStockItems")}
                  </p>
                  <p
                    className={
                      outOfStockCount > 0
                        ? "mt-1 text-2xl font-semibold tabular-nums text-rose-800"
                        : inventoryMetricValueClass
                    }
                  >
                    {outOfStockCount}
                  </p>
                </PremiumPanel>
              </div>

              <SegmentedTabs
                tabs={[
                  { key: "items", label: t("tabs.items"), icon: Boxes },
                  { key: "recipes", label: t("tabs.recipes"), icon: PackagePlus },
                  { key: "activity", label: t("tabs.activity"), icon: RefreshCw },
                  { key: "settings", label: t("tabs.settings"), icon: SlidersHorizontal },
                ]}
                activeKey={activeTab}
                onChange={(key) => {
                  // Clear any stale failure banner so a prior tab's error
                  // doesn't linger over content the operator just switched to.
                  setError(null);
                  setActiveTab(key);
                }}
                ariaLabel={t("header.title")}
              />

              {/* Items Tab */}
              <DashboardTabTransition tabKey={activeTab}>
              {activeTab === "items" && (
                <div className="space-y-6">
                  {hasStockAlerts && (
                    <div
                      className={
                        outOfStockItems.length > 0
                          ? "rounded-xl border border-rose-200 bg-rose-50"
                          : "rounded-xl border border-amber-200 bg-amber-50"
                      }
                      data-testid="inventory-needs-attention"
                    >
                      <button
                        type="button"
                        className="flex w-full items-center justify-between px-4 py-3 text-left"
                        onClick={() => setNeedsAttentionOpen((o) => !o)}
                      >
                        <span
                          className={`flex items-center gap-2 text-sm font-medium ${
                            outOfStockItems.length > 0
                              ? "text-rose-900"
                              : "text-amber-900"
                          }`}
                        >
                          <AlertTriangle className="h-4 w-4" />
                          {tWith("needsAttention.summary", {
                            out: outOfStockItems.length,
                            low: lowStockItems.length,
                          })}
                        </span>
                        {needsAttentionOpen ? (
                          <ChevronUp
                            className={`h-4 w-4 ${
                              outOfStockItems.length > 0
                                ? "text-rose-700"
                                : "text-amber-700"
                            }`}
                          />
                        ) : (
                          <ChevronDown
                            className={`h-4 w-4 ${
                              outOfStockItems.length > 0
                                ? "text-rose-700"
                                : "text-amber-700"
                            }`}
                          />
                        )}
                      </button>
                      {needsAttentionOpen && (
                        <div className="space-y-3 px-4 pb-3">
                          <p className="text-xs text-ink-600">
                            {t("needsAttention.dishesHint")}
                          </p>
                          {Object.values(menuStatuses)
                            .filter(
                              (status) =>
                                status.status === "low_stock" ||
                                status.status === "out_of_stock",
                            )
                            .map((status) => (
                              <div
                                key={status.menu_item_id}
                                className={`flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-white/70 px-3 py-2 ${
                                  status.status === "out_of_stock"
                                    ? "border-rose-200/80"
                                    : "border-amber-200/80"
                                }`}
                              >
                                <span className="text-sm font-medium text-ink-950">
                                  {status.menu_item_name}
                                  {status.blocks_sale ? (
                                    <span className="ml-2 text-xs font-semibold uppercase tracking-wide text-rose-600">
                                      {t("needsAttention.blocked")}
                                    </span>
                                  ) : null}
                                </span>
                                {onNavigateToTab ? (
                                  <button
                                    type="button"
                                    onClick={() =>
                                      onNavigateToTab(
                                        `menu?menuSearch=${encodeURIComponent(status.menu_item_name)}`,
                                      )
                                    }
                                    className="rounded text-xs font-medium text-brand underline decoration-brand/40 underline-offset-2 hover:text-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                                  >
                                    {t("needsAttention.viewInMenu")}
                                  </button>
                                ) : null}
                              </div>
                            ))}
                        </div>
                      )}
                    </div>
                  )}

                  <div className={inventorySectionShellClass}>
                    <div className="mb-4 flex min-w-0 flex-wrap items-center justify-between gap-3">
                      <div className="min-w-0 max-w-full">
                        <h3 className={inventorySectionTitleClass}>
                          {t("items.title")}
                        </h3>
                        <p className={inventorySectionCopyClass}>
                          {t("items.subtitle")}
                        </p>
                      </div>
                      <div
                        data-testid="inventory-items-header-actions"
                        className="flex min-w-0 max-w-full flex-wrap items-center justify-end gap-2 sm:gap-3"
                      >
                        <Chip variant="flat" color="primary">
                          {tWith("items.count", { count: items.length })}
                        </Chip>
                        <Button
                          radius="full"
                          variant="bordered"
                          className="border-brand/30 text-brand-dark"
                          onPress={() => setReorderModalOpen(true)}
                          data-testid="inventory-reorder-list-button"
                          endContent={
                            reorderRows.length > 0 ? (
                              <Chip size="sm" variant="flat" color="warning">
                                {reorderRows.length}
                              </Chip>
                            ) : undefined
                          }
                        >
                          {t("reorder.button")}
                        </Button>
                        <Button
                          radius="full"
                          className={btnPrimaryNextUI}
                          startContent={<PackagePlus className="h-4 w-4" />}
                          onPress={openItemModalForCreate}
                          data-testid="inventory-add-item-button"
                        >
                          {t("itemForm.addTitle")}
                        </Button>
                      </div>
                    </div>

                    {items.length > 0 && (
                      <div className="mb-4">
                        <InventoryToolbar
                          state={filterState}
                          onChange={patchFilter}
                          categories={filtered.categories}
                          counts={filtered.counts}
                          shown={filtered.visibleItems.length}
                          total={items.length}
                          onExport={handleExportCsv}
                          t={t}
                          tWith={tWith}
                        />
                      </div>
                    )}

                    {items.length === 0 ? (
                      <InventoryEmptyPanel
                        icon={Boxes}
                        title={t("emptyStates.firstRunTitle")}
                        body={t("emptyStates.firstRunBody")}
                        hint={t("emptyStates.firstRunParHint")}
                        actionLabel={t("emptyStates.addFirstItem")}
                        onAction={openItemModalForCreate}
                        variant="firstRun"
                      />
                    ) : filtered.visibleItems.length === 0 ? (
                      <InventoryEmptyPanel
                        icon={SearchX}
                        title={t("emptyStates.filteredEmptyTitle")}
                        body={t("emptyStates.filteredEmptyBody")}
                        actionLabel={t("emptyStates.clearFilters")}
                        onAction={() =>
                          setFilterState({
                            search: "",
                            category: "",
                            status: "all",
                            sort: "attention",
                          })
                        }
                        variant="filtered"
                      />
                    ) : (
                      <div
                        data-testid="inventory-items-table-scroller"
                        role="region"
                        aria-label={t("items.title")}
                        tabIndex={0}
                        className="min-w-0 max-w-full overflow-x-auto rounded-2xl border border-warm-200/80 bg-white shadow-sm shadow-warm-900/5"
                      >
                        <Table
                          aria-label={t("items.title")}
                          removeWrapper
                          bottomContent={
                            itemsTotalPages > 1 ? (
                              <div className="flex justify-center py-2">
                                <Pagination
                                  total={itemsTotalPages}
                                  page={itemsPage}
                                  onChange={setItemsPage}
                                  showControls
                                />
                              </div>
                            ) : null
                          }
                        >
                          <TableHeader>
                            <TableColumn className={inventoryTableHeaderClass}>
                              {t("items.tableColumns.name")}
                            </TableColumn>
                            <TableColumn className={`${inventoryTableHeaderClass} hidden md:table-cell`}>
                              {t("items.tableColumns.category")}
                            </TableColumn>
                            <TableColumn className={`${inventoryTableHeaderClass} text-right`}>
                              {t("items.tableColumns.quantity")}
                            </TableColumn>
                            <TableColumn className={`${inventoryTableHeaderClass} hidden text-right md:table-cell`}>
                              <span title={t("toolbar.reorderAtTooltip")}>
                                {t("items.tableColumns.threshold")}
                              </span>
                            </TableColumn>
                            <TableColumn className={`${inventoryTableHeaderClass} hidden lg:table-cell`}>
                              {t("items.tableColumns.sku")}
                            </TableColumn>
                            <TableColumn className={`${inventoryTableHeaderClass} hidden text-right lg:table-cell`}>
                              {t("valuation.valueColumn")}
                            </TableColumn>
                            <TableColumn className={inventoryTableHeaderClass}>
                              {t("items.tableColumns.status")}
                            </TableColumn>
                            <TableColumn className={`${inventoryTableHeaderClass} text-right`}>
                              {t("items.tableColumns.actions")}
                            </TableColumn>
                          </TableHeader>
                          <TableBody>
                            {pagedItems.map((item) => {
                              const statusKey = itemStatus(item);
                              const usedBy = dishesUsingItem(item);
                              const blockedDishes = usedBy.filter(
                                (d) =>
                                  d.status === "out_of_stock" || d.blocks_sale,
                              );
                              const sellableMapped = mappedSellableDishes(
                                item,
                                recipes,
                                Object.values(menuStatuses),
                                liveMenuRefs,
                              );
                              const showEightySix = canOfferEightySix(
                                item,
                                sellableMapped,
                              );
                              const needsReceive =
                                statusKey === "out_of_stock" ||
                                statusKey === "low_stock";
                              return (
                                <TableRow key={item.id}>
                                  <TableCell>
                                    <button
                                      type="button"
                                      className="text-left font-medium text-ink-950 transition hover:text-brand"
                                      onClick={() => setDetailItem(item)}
                                    >
                                      {item.name}
                                    </button>
                                    {blockedDishes.length > 0 ? (
                                      <p
                                        className="mt-0.5 text-xs text-rose-700"
                                        data-testid={`inventory-blocks-${item.id}`}
                                      >
                                        {tWith("items.blocksDishes", {
                                          count: blockedDishes.length,
                                          names: blockedDishes
                                            .map((d) => d.menu_item_name)
                                            .slice(0, 2)
                                            .join(", "),
                                        })}
                                      </p>
                                    ) : null}
                                  </TableCell>
                                  <TableCell className="hidden md:table-cell">
                                    {item.category ? (
                                      <Chip size="sm" variant="flat">
                                        {item.category}
                                      </Chip>
                                    ) : (
                                      <span className="text-sm text-ink-400">
                                        —
                                      </span>
                                    )}
                                  </TableCell>
                                  <TableCell
                                    className={`text-right tabular-nums text-sm ${
                                      item.current_quantity < 0
                                        ? "font-medium text-rose-600"
                                        : "text-ink-950"
                                    }`}
                                  >
                                    {item.current_quantity < 0 ? (
                                      // Oversold: keep the true negative visible
                                      // but tag it so a bare "-0.35 kg" doesn't
                                      // read as a display glitch.
                                      <span className="inline-flex items-center justify-end gap-1.5">
                                        {formatUnit(
                                          item.current_quantity,
                                          item.unit,
                                          locale,
                                        )}
                                        <span
                                          title={t("valuation.oversoldTooltip")}
                                          className="rounded-full bg-rose-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-rose-600"
                                        >
                                          {t("valuation.oversold")}
                                        </span>
                                      </span>
                                    ) : (
                                      formatUnit(
                                        item.current_quantity,
                                        item.unit,
                                        locale,
                                      )
                                    )}
                                  </TableCell>
                                  <TableCell className="hidden text-right tabular-nums text-sm text-ink-600 md:table-cell">
                                    {item.reorder_threshold > 0
                                      ? formatUnit(
                                          item.reorder_threshold,
                                          item.unit,
                                          locale,
                                        )
                                      : "—"}
                                  </TableCell>
                                  <TableCell className="hidden text-sm text-ink-600 lg:table-cell">
                                    {item.sku || "—"}
                                  </TableCell>
                                  <TableCell className="hidden text-right tabular-nums text-sm text-ink-950 lg:table-cell">
                                    {fmtCurrency(selectItemValue(item))}
                                  </TableCell>
                                  <TableCell>
                                    <Chip
                                      size="sm"
                                      color={statusTone(statusKey)}
                                      variant="flat"
                                    >
                                      {inventoryStatusLabel(statusKey)}
                                    </Chip>
                                  </TableCell>
                                  <TableCell>
                                    <div className="flex items-center justify-end gap-1">
                                      {needsReceive ? (
                                        <Button
                                          size="sm"
                                          variant="flat"
                                          className="bg-brand/10 font-semibold text-brand-dark"
                                          startContent={
                                            <Truck className="h-3.5 w-3.5" />
                                          }
                                          aria-label={t("items.receiveStock")}
                                          onPress={() => openAdjust(item)}
                                          data-testid={`inventory-receive-${item.id}`}
                                        >
                                          {t("items.receiveStock")}
                                        </Button>
                                      ) : null}
                                      {showEightySix ? (
                                        <Button
                                          size="sm"
                                          variant="flat"
                                          className="bg-rose-50 font-semibold text-rose-800"
                                          startContent={
                                            <Ban className="h-3.5 w-3.5" />
                                          }
                                          aria-label={t("items.eightySixDishes")}
                                          onPress={() => requestEightySix(item)}
                                          data-testid={`inventory-86-${item.id}`}
                                        >
                                          {tWith("items.eightySixDishesCount", {
                                            count: sellableMapped.length,
                                          })}
                                        </Button>
                                      ) : null}
                                      {!needsReceive ? (
                                        <Button
                                          size="sm"
                                          variant="light"
                                          isIconOnly
                                          aria-label={t("detail.adjust")}
                                          onPress={() => openAdjust(item)}
                                          className={touchIconBtn}
                                        >
                                          <SlidersHorizontal className="h-4 w-4" />
                                        </Button>
                                      ) : null}
                                      <Button
                                        size="sm"
                                        variant="light"
                                        isIconOnly
                                        aria-label={t("items.edit")}
                                        onPress={() =>
                                          openItemModalForEdit(item)
                                        }
                                        className={touchIconBtn}
                                      >
                                        <Pencil className="h-4 w-4" />
                                      </Button>
                                      <Button
                                        size="sm"
                                        color="danger"
                                        variant="light"
                                        isIconOnly
                                        aria-label={t("items.delete")}
                                        onPress={() =>
                                          setPendingDelete({
                                            id: item.id,
                                            name: item.name,
                                          })
                                        }
                                        className={touchIconBtn}
                                      >
                                        <Trash2 className="h-4 w-4" />
                                      </Button>
                                    </div>
                                  </TableCell>
                                </TableRow>
                              );
                            })}
                          </TableBody>
                        </Table>
                      </div>
                    )}
                  </div>
                </div>
              )}

              {/* Recipes Tab */}
              {activeTab === "recipes" && (
                <div className="space-y-4">
                  <div className={inventorySectionShellClass}>
                    <div className="mb-1 flex items-center justify-between gap-2">
                      <h3 className={inventorySectionTitleClass}>
                        {t("recipes.title")}
                      </h3>
                      <Chip size="sm" variant="flat" data-testid="recipes-mapped-count">
                        {tWith("recipes.mappedCount", {
                          mapped: recipeCoverage.mapped,
                          total: recipeCoverage.total,
                        })}
                      </Chip>
                    </div>
                    <p className={`${inventorySectionCopyClass} mb-4`}>
                      {t("recipes.subtitle")}
                    </p>
                    {items.length > 0 &&
                    recipeCoverage.total > 0 &&
                    recipeCoverage.mapped < recipeCoverage.total ? (
                      <div
                        className="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900"
                        data-testid="recipes-coverage-warning"
                      >
                        {tWith(
                          recipeCoverage.mapped === 0
                            ? "recipes.coverageWarning"
                            : "recipes.incompleteCoverage",
                          {
                            ingredients: items.length,
                            mapped: recipeCoverage.mapped,
                            total: recipeCoverage.total,
                            dishes: menuItems.length,
                          },
                        )}
                      </div>
                    ) : null}
                    <div>
                      <Autocomplete
                        label={t("recipes.menuItemLabel")}
                        labelPlacement="outside"
                        placeholder={t("recipes.menuItemPlaceholder")}
                        aria-label={t("recipes.menuItemLabel")}
                        selectedKey={selectedRecipeMenuItemId || null}
                        onSelectionChange={(key) =>
                          requestRecipeMenuItemChange(key ? String(key) : "")
                        }
                        variant="bordered"
                        radius="lg"
                        menuTrigger="input"
                        isClearable
                      >
                        {groupedMenuItems.map((group) => (
                          <AutocompleteSection
                            key={group.category}
                            title={group.category}
                          >
                            {group.items.map((item) => (
                              <AutocompleteItem
                                key={item.id}
                                textValue={item.name}
                              >
                                {item.name}
                              </AutocompleteItem>
                            ))}
                          </AutocompleteSection>
                        ))}
                      </Autocomplete>
                    </div>

                    {selectedRecipeMenuItemId ? (
                      <div className="mt-6 space-y-3">
                        {recipeDraft.map((row, index) => (
                          <div
                            key={`${selectedRecipeMenuItemId}-${index}`}
                            className="grid grid-cols-1 items-end gap-3 md:grid-cols-[1fr_140px_auto]"
                          >
                            <div>
                              <Autocomplete
                                label={t("recipes.inventoryItemLabel")}
                                labelPlacement="outside"
                                placeholder={t(
                                  "recipes.inventoryItemPlaceholder",
                                )}
                                aria-label={t("recipes.inventoryItemLabel")}
                                selectedKey={row.inventoryItemId || null}
                                onSelectionChange={(key) =>
                                  setRecipeDraft((current) =>
                                    current.map((draft, i) =>
                                      i === index
                                        ? {
                                            ...draft,
                                            inventoryItemId: key
                                              ? String(key)
                                              : "",
                                          }
                                        : draft,
                                    ),
                                  )
                                }
                                variant="bordered"
                                radius="lg"
                                menuTrigger="input"
                                isClearable
                              >
                                {groupedInventoryItems.map((group) => (
                                  <AutocompleteSection
                                    key={group.category}
                                    title={group.category}
                                  >
                                    {group.items.map((it) => (
                                      <AutocompleteItem
                                        key={String(it.id)}
                                        textValue={`${it.name} · ${it.unit}`}
                                      >
                                        {it.name}
                                        <span className="text-ink-400">
                                          {" "}
                                          · {it.unit}
                                        </span>
                                      </AutocompleteItem>
                                    ))}
                                  </AutocompleteSection>
                                ))}
                              </Autocomplete>
                            </div>
                            <Input
                              type="number"
                              label={t("recipes.quantityPerSale")}
                              value={row.quantityRequired}
                              onValueChange={(v) =>
                                setRecipeDraft((current) =>
                                  current.map((draft, i) =>
                                    i === index
                                      ? { ...draft, quantityRequired: v }
                                      : draft,
                                  ),
                                )
                              }
                            />
                            <Button
                              variant="light"
                              color="danger"
                              isDisabled={recipeDraft.length === 1}
                              onPress={() =>
                                setRecipeDraft((current) =>
                                  current.filter((_, i) => i !== index),
                                )
                              }
                            >
                              {t("recipes.removeIngredient")}
                            </Button>
                          </div>
                        ))}
                        <div className="flex gap-3">
                          <Button
                            variant="flat"
                            startContent={<Boxes className="h-4 w-4" />}
                            onPress={() =>
                              setRecipeDraft((current) => [
                                ...current,
                                { inventoryItemId: "", quantityRequired: "1" },
                              ])
                            }
                          >
                            {t("recipes.addIngredient")}
                          </Button>
                          <Button
                            radius="full"
                            className={btnPrimaryNextUI}
                            onPress={() => void handleSaveRecipe()}
                            isLoading={savingRecipe}
                          >
                            {t("recipes.saveRecipe")}
                          </Button>
                        </div>
                        {menuStatuses[selectedRecipeMenuItemId] && (
                          <div className="rounded-2xl border border-warm-200 bg-warm-50/70 px-4 py-3">
                            <div className="mb-2 flex flex-wrap items-center gap-2">
                              <Chip
                                size="sm"
                                color={statusTone(
                                  menuStatuses[selectedRecipeMenuItemId].status,
                                )}
                                variant="flat"
                              >
                                {statusLabel(
                                  menuStatuses[selectedRecipeMenuItemId].status,
                                )}
                              </Chip>
                              {menuStatuses[selectedRecipeMenuItemId]
                                .max_possible_servings >= 0 && (
                                <Chip size="sm" variant="flat">
                                  {tWith("recipes.maxServings", {
                                    count:
                                      menuStatuses[selectedRecipeMenuItemId]
                                        .max_possible_servings,
                                  })}
                                </Chip>
                              )}
                            </div>
                            {menuStatuses[selectedRecipeMenuItemId]
                              .affected_inventory.length > 0 && (
                              <p className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
                                {tWith("recipes.blockingIngredients", {
                                  items:
                                    menuStatuses[
                                      selectedRecipeMenuItemId
                                    ].affected_inventory.join(", "),
                                })}
                              </p>
                            )}
                            {menuStatuses[selectedRecipeMenuItemId]
                              .warning_inventory.length > 0 && (
                              <p className="text-sm text-amber-700">
                                {tWith("recipes.warningIngredients", {
                                  items:
                                    menuStatuses[
                                      selectedRecipeMenuItemId
                                    ].warning_inventory.join(", "),
                                })}
                              </p>
                            )}
                          </div>
                        )}
                      </div>
                    ) : (
                      <div className="mt-6 space-y-3" data-testid="recipes-empty-guide">
                        <div className="rounded-2xl border border-warm-200 bg-warm-50/70 px-4 py-4">
                          <h4 className="text-sm font-semibold text-ink-950">
                            {t("recipes.emptyGuideTitle")}
                          </h4>
                          <p className="mt-1 text-sm text-ink-600">
                            {t("recipes.emptyGuideBody")}
                          </p>
                        </div>
                        {menuItems.length === 0 ? (
                          <p className="text-sm text-ink-500">
                            {t("recipes.emptyNoMenu")}
                          </p>
                        ) : null}
                      </div>
                    )}
                    {menuItems.length > 0 ? (
                      <ul
                        className="mt-6 divide-y divide-warm-200 rounded-2xl border border-warm-200 bg-white"
                        data-testid="recipes-dish-list"
                      >
                        {menuItems.map((item) => {
                          const status = menuStatuses[item.id];
                          const mapped = Boolean(status?.has_recipe);
                          const selected = selectedRecipeMenuItemId === item.id;
                          return (
                            <li key={item.id}>
                              <button
                                type="button"
                                className={`flex w-full items-center justify-between gap-3 px-4 py-3 text-left transition hover:bg-warm-50 ${
                                  selected ? "bg-brand/5" : ""
                                }`}
                                onClick={() =>
                                  requestRecipeMenuItemChange(item.id)
                                }
                                data-testid={`recipe-pick-${item.id}`}
                              >
                                <span className="min-w-0">
                                  <span className="block truncate text-sm font-medium text-ink-950">
                                    {item.name}
                                  </span>
                                  <span className="text-xs text-ink-500">
                                    {item.category}
                                  </span>
                                </span>
                                <Chip
                                  size="sm"
                                  variant="flat"
                                  color={mapped ? "success" : "warning"}
                                >
                                  {mapped
                                    ? t("recipes.statusMapped")
                                    : t("recipes.statusUnmapped")}
                                </Chip>
                              </button>
                            </li>
                          );
                        })}
                      </ul>
                    ) : null}
                  </div>
                </div>
              )}

              {/* Activity Tab */}
              {activeTab === "activity" && (
                <div className="space-y-3">
                  <div className={inventorySectionShellClass}>
                    <h3 className={`${inventorySectionTitleClass} mb-1`}>
                      {t("movements.title")}
                    </h3>
                    <p className={`${inventorySectionCopyClass} mb-4`}>
                      {t("movements.subtitle")}
                    </p>

                    <div className="mb-4 flex flex-col gap-3 md:flex-row">
                      <Autocomplete
                        aria-label={t("activityFilter.allItems")}
                        placeholder={t("activityFilter.allItems")}
                        selectedKey={activityItemFilter || null}
                        onSelectionChange={(key) =>
                          setActivityItemFilter(key ? String(key) : "")
                        }
                        variant="bordered"
                        radius="lg"
                        size="sm"
                        menuTrigger="input"
                        className="md:max-w-[240px]"
                        isClearable
                      >
                        {items.map((it) => (
                          <AutocompleteItem key={String(it.id)} textValue={it.name}>
                            {it.name}
                          </AutocompleteItem>
                        ))}
                      </Autocomplete>
                      <Select
                        aria-label={t("activityFilter.allTypes")}
                        placeholder={t("activityFilter.allTypes")}
                        selectedKeys={
                          activityTypeFilter ? [activityTypeFilter] : []
                        }
                        onSelectionChange={(keys) => {
                          const next = Array.from(keys)[0];
                          setActivityTypeFilter(next ? String(next) : "");
                        }}
                        variant="bordered"
                        radius="lg"
                        size="sm"
                        className="md:max-w-[240px]"
                      >
                        {MOVEMENT_TYPE_KEYS.map((mt) => (
                          <SelectItem key={mt} textValue={movementTypeLabel(mt)}>
                            {movementTypeLabel(mt)}
                          </SelectItem>
                        ))}
                      </Select>
                    </div>

                    {activityLedger.error ? (
                      <InventoryEmptyPanel
                        icon={AlertTriangle}
                        title={t("movements.loadError")}
                        body={t("errors.loadData")}
                        actionLabel={t("movements.retry")}
                        onAction={() => activityLedger.reload()}
                        variant="filtered"
                      />
                    ) : activityLedger.loading &&
                      activityLedger.movements.length === 0 ? (
                      <div className="space-y-3" aria-hidden>
                        {[0, 1, 2, 3].map((i) => (
                          <div
                            key={i}
                            className="h-16 animate-pulse rounded-2xl border border-warm-200/60 bg-warm-100/60"
                          />
                        ))}
                      </div>
                    ) : activityLedger.total === 0 &&
                      !activityItemFilter &&
                      !activityTypeFilter ? (
                      <div className="py-12 text-center">
                        <div className={inventoryEmptyIconWrapClass}>
                          <Boxes className="h-8 w-8" />
                        </div>
                        <h3 className={`${inventorySectionTitleClass} mb-2`}>
                          {t("movements.title")}
                        </h3>
                        <p className={inventorySectionCopyClass}>
                          {t("movements.empty")}
                        </p>
                      </div>
                    ) : activityLedger.movements.length === 0 ? (
                      // Server returned no rows for the active item/type filters.
                      <InventoryEmptyPanel
                        icon={SearchX}
                        title={t("movements.filteredEmptyTitle")}
                        body={t("movements.filteredEmptyBody")}
                        actionLabel={t("emptyStates.clearFilters")}
                        onAction={() => {
                          setActivityItemFilter("");
                          setActivityTypeFilter("");
                        }}
                        variant="filtered"
                      />
                    ) : (
                      <div className="space-y-3">
                        {activityLedger.movements.map((movement) => (
                          <div
                            key={movement.id}
                            className={inventoryActivityCardClass}
                          >
                            <div className="flex items-start justify-between gap-3">
                              <div>
                                <div className="flex flex-wrap items-center gap-2">
                                  <span className="font-medium text-ink-950">
                                    {movement.inventory_item?.name ||
                                      tWith("movements.itemFallback", {
                                        id: movement.inventory_item_id,
                                      })}
                                  </span>
                                  <Chip size="sm" variant="flat">
                                    {movementTypeLabel(movement.movement_type)}
                                  </Chip>
                                  <Chip
                                    size="sm"
                                    color={
                                      movement.quantity_delta < 0
                                        ? "danger"
                                        : "success"
                                    }
                                    variant="flat"
                                  >
                                    {movement.quantity_delta > 0 ? "+" : ""}
                                    {formatCount(
                                      movement.quantity_delta,
                                      locale,
                                    )}
                                  </Chip>
                                </div>
                                <p className="mt-1 text-sm text-ink-600">
                                  {movement.reason
                                    ? reasonLabel(movement.reason)
                                    : t("movements.noReason")}
                                </p>
                                {movement.inventory_item?.cost_per_unit ? (
                                  <p className="mt-1 text-xs text-ink-500">
                                    {tWith("activityFilter.valueImpact", {
                                      amount: fmtCurrency(
                                        Math.abs(movement.quantity_delta) *
                                          movement.inventory_item.cost_per_unit,
                                      ),
                                    })}
                                  </p>
                                ) : null}
                                {(movement.menu_item_name ||
                                  movement.reference_order_id) && (
                                  <p className="mt-1 text-xs text-ink-500">
                                    {movement.menu_item_name
                                      ? tWith("movements.menuItem", {
                                          name: movement.menu_item_name,
                                        })
                                      : ""}
                                    {movement.menu_item_name &&
                                    movement.reference_order_id
                                      ? " · "
                                      : ""}
                                    {movement.reference_order_id
                                      ? tWith("movements.orderReference", {
                                          id: movement.reference_order_id,
                                        })
                                      : ""}
                                  </p>
                                )}
                              </div>
                              <div className="text-right text-xs leading-5 text-ink-500">
                                <div>
                                  {formatBusinessDateTime(
                                    movement.created_at,
                                    locale,
                                    businessTimezone,
                                    DATE_TIME_SHORT,
                                  )}
                                </div>
                                <div>
                                  {tWith("movements.quantityFlow", {
                                    before: formatCount(
                                      movement.quantity_before,
                                      locale,
                                    ),
                                    after: formatCount(
                                      movement.quantity_after,
                                      locale,
                                    ),
                                  })}
                                </div>
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}

                    {activityLedger.total > 0 &&
                      activityLedger.movements.length > 0 && (
                        <div className="mt-4 flex flex-col items-center gap-2 border-t border-warm-200/60 pt-4 sm:flex-row sm:justify-between">
                          <span className="text-xs font-medium text-ink-500">
                            {tWith("movements.showingCount", {
                              shown: activityLedger.movements.length,
                              total: activityLedger.total,
                            })}
                          </span>
                          {activityLedger.totalPages > 1 && (
                            <Pagination
                              size="sm"
                              showControls
                              total={activityLedger.totalPages}
                              page={activityLedger.page}
                              onChange={activityLedger.setPage}
                              classNames={{ cursor: "bg-brand text-white" }}
                            />
                          )}
                        </div>
                      )}
                  </div>
                </div>
              )}

              {/* Settings Tab */}
              {activeTab === "settings" && (
                <div className="space-y-4">
                  <div className={`${inventorySectionShellClass} flex items-center justify-between gap-4`}>
                    <div>
                      <h3 className="text-base font-semibold text-ink-950">
                        {t("header.title")}
                      </h3>
                      <p className="mt-0.5 text-sm leading-5 text-ink-600">
                        {t("header.subtitle")}
                      </p>
                    </div>
                    <InventoryToggle
                      businessId={businessId}
                      enabled={settings?.inventory_enabled ?? false}
                      isLocked={!hasAccess || isSuspended}
                      variant="button"
                      onError={(message) => setError(message)}
                      onStatusChange={handleInventoryStatusChange}
                    />
                  </div>
                  <div className={inventorySectionShellClass}>
                    <h3 className={`${inventorySectionTitleClass} mb-1`}>
                      {t("settings.title")}
                    </h3>
                    <p className={`${inventorySectionCopyClass} mb-4`}>
                      {t("settings.subtitle")}
                    </p>
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                      <div className={inventorySettingCardClass}>
                        <div>
                          <p
                            id={`${settingsA11yId}-auto-deduct-label`}
                            className="font-medium text-ink-950"
                          >
                            {t("settings.autoDeduct.title")}
                          </p>
                          <p
                            id={`${settingsA11yId}-auto-deduct-desc`}
                            className="text-sm leading-5 text-ink-600"
                          >
                            {t("settings.autoDeduct.description")}
                          </p>
                        </div>
                        <NamedSwitch
                          name={t("settings.autoDeduct.title")}
                          labelId={`${settingsA11yId}-auto-deduct-label`}
                          descriptionId={`${settingsA11yId}-auto-deduct-desc`}
                          isSelected={
                            settings?.auto_deduct_on_order_approval ?? true
                          }
                          onValueChange={(v) =>
                            void handleSaveSettings({
                              auto_deduct_on_order_approval: v,
                            })
                          }
                        />
                      </div>
                      <div className={inventorySettingCardClass}>
                        <div>
                          <p
                            id={`${settingsA11yId}-low-stock-label`}
                            className="font-medium text-ink-950"
                          >
                            {t("settings.lowStockWarnings.title")}
                          </p>
                          <p
                            id={`${settingsA11yId}-low-stock-desc`}
                            className="text-sm leading-5 text-ink-600"
                          >
                            {t("settings.lowStockWarnings.description")}
                          </p>
                        </div>
                        <NamedSwitch
                          name={t("settings.lowStockWarnings.title")}
                          labelId={`${settingsA11yId}-low-stock-label`}
                          descriptionId={`${settingsA11yId}-low-stock-desc`}
                          isSelected={
                            settings?.low_stock_warnings_enabled ?? true
                          }
                          onValueChange={(v) =>
                            void handleSaveSettings({
                              low_stock_warnings_enabled: v,
                            })
                          }
                        />
                      </div>
                      <div className="rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3">
                        <NamedSelect
                          name={t("settings.availabilitySyncMode")}
                          valueLabel={
                            settings?.availability_sync_mode === "manual"
                              ? t("settings.availabilityModes.manual")
                              : settings?.availability_sync_mode === "hard_block"
                                ? t("settings.availabilityModes.hardBlock")
                                : t("settings.availabilityModes.warn")
                          }
                          label={t("settings.availabilitySyncMode")}
                          labelPlacement="outside"
                          selectedKeys={[
                            settings?.availability_sync_mode ?? "warn",
                          ]}
                          onSelectionChange={(keys) => {
                            const next = Array.from(keys)[0];
                            if (next) {
                              void handleSaveSettings({
                                availability_sync_mode:
                                  next as InventorySettings["availability_sync_mode"],
                              });
                            }
                          }}
                          variant="bordered"
                          radius="lg"
                        >
                          <SelectItem
                            key="warn"
                            textValue={t("settings.availabilityModes.warn")}
                          >
                            {t("settings.availabilityModes.warn")}
                          </SelectItem>
                          <SelectItem
                            key="manual"
                            textValue={t("settings.availabilityModes.manual")}
                          >
                            {t("settings.availabilityModes.manual")}
                          </SelectItem>
                          <SelectItem
                            key="hard_block"
                            textValue={t(
                              "settings.availabilityModes.hardBlock",
                            )}
                          >
                            {t("settings.availabilityModes.hardBlock")}
                          </SelectItem>
                        </NamedSelect>
                      </div>
                    </div>
                  </div>
                  {/* M19: these switches persist on toggle. Auto-mode SaveBar
                      is the only surface that may claim "saved automatically". */}
                  {/* Auto-save surface: switches persist on toggle. dirty is
                      never the saving spinner (Finding 65) — leave false so
                      the passive autosave chip is the only signal. */}
                  <SaveBar
                    mode="auto"
                    isSaving={savingSettings}
                    dirty={false}
                    onSave={() => {}}
                    labels={{
                      save: saveBarString("save"),
                      saving: saveBarString("saving"),
                      unsaved: saveBarString("unsaved"),
                      auto: saveBarString("auto"),
                      clean: saveBarString("clean"),
                    }}
                  />
                </div>
              )}
              </DashboardTabTransition>
            </div>
          </PremiumPanel>
        </>
      )}

      <InventoryItemModal
        isOpen={itemModalOpen}
        onClose={closeItemModal}
        mode={initialEditItem ? "edit" : "create"}
        initial={initialEditItem}
        businessId={businessId}
        categories={filtered.categories}
        onSaved={handleItemSaved}
        onError={(message) => setError(message)}
        t={t}
      />

      <QuickAdjustDrawer
        isOpen={adjustOpen}
        item={adjustItem}
        businessId={businessId}
        onClose={() => setAdjustOpen(false)}
        onSaved={async ({ item }) => {
          // L5-41: clear any page-level error left by a prior failed mutation.
          setError(null);
          // Patch the adjusted item's new on-hand quantity in place, refresh the
          // derived summary (stock health may have changed), and reload the
          // Activity ledger so the new movement appears — no full mount cascade.
          setItems((current) =>
            current.map((it) => (it.id === item.id ? item : it)),
          );
          await refreshSummary();
          activityLedger.reload();
        }}
        t={t}
      />

      <ItemDetailDrawer
        isOpen={detailItem !== null}
        item={detailItem}
        businessId={businessId}
        usedBy={detailUsedBy}
        formatValue={fmtCurrency}
        formatQty={(qty, unit) => formatUnit(qty, unit, locale)}
        statusLabel={inventoryStatusLabel}
        statusTone={statusTone}
        movementTypeLabel={movementTypeLabel}
        onClose={() => setDetailItem(null)}
        onAdjust={openAdjust}
        onEdit={(item) => {
          setDetailItem(null);
          openItemModalForEdit(item);
        }}
        onDelete={(item) => {
          setDetailItem(null);
          setPendingDelete({ id: item.id, name: item.name });
        }}
        t={t}
      />

      <ConfirmationModal
        isOpen={pendingDelete !== null}
        onOpenChange={() => setPendingDelete(null)}
        isDanger
        title={t("items.confirmDeleteTitle")}
        description={t("items.confirmDeleteBody").replace(
          "{name}",
          pendingDelete?.name ?? "",
        )}
        confirmLabel={t("items.delete")}
        onConfirm={() => {
          if (pendingDelete) {
            void handleDeleteItem(pendingDelete.id);
          }
          setPendingDelete(null);
        }}
      />

      <ConfirmationModal
        isOpen={pendingRecipeClear}
        onOpenChange={() => setPendingRecipeClear(false)}
        isDanger
        title={t("recipes.confirmClearTitle")}
        description={t("recipes.confirmClearBody")}
        confirmLabel={t("recipes.confirmClearConfirm")}
        cancelLabel={t("recipes.confirmClearCancel")}
        onConfirm={() => {
          setPendingRecipeClear(false);
          void persistRecipe([]);
        }}
      />

      <ConfirmationModal
        isOpen={pendingRecipeSwitch !== null}
        onOpenChange={() => setPendingRecipeSwitch(null)}
        isDanger
        title={t("recipes.confirmSwitchTitle")}
        description={t("recipes.confirmSwitchBody")}
        confirmLabel={t("recipes.confirmSwitchConfirm")}
        cancelLabel={t("recipes.confirmSwitchCancel")}
        onConfirm={() => {
          if (pendingRecipeSwitch) {
            setSelectedRecipeMenuItemId(pendingRecipeSwitch.id);
          }
          setPendingRecipeSwitch(null);
        }}
      />

      <ConfirmationModal
        isOpen={pendingEightySix !== null}
        onOpenChange={() => (eightySixBusy ? undefined : setPendingEightySix(null))}
        isDanger
        isLoading={eightySixBusy}
        title={t("items.eightySixConfirmTitle")}
        description={tWith("items.eightySixConfirmBody", {
          name: pendingEightySix?.item.name ?? "",
          dishes:
            pendingEightySix?.dishes.map((d) => d.menu_item_name).join(", ") ??
            "",
        })}
        confirmLabel={t("items.eightySixConfirm")}
        onConfirm={() => confirmEightySix()}
      />

      <ReorderListModal
        isOpen={reorderModalOpen}
        onClose={() => setReorderModalOpen(false)}
        rows={reorderRows}
        t={t}
      />
    </DashboardTabShell>
  );
}
