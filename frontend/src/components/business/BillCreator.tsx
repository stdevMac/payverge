import React, { useState, useEffect, useCallback, useMemo } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Select,
  SelectItem,
  Card,
  CardBody,
  CardHeader,
  Divider,
  Chip,
  Input,
  Textarea,
  Spinner,
  useDisclosure,
} from "@nextui-org/react";
import {
  Plus,
  Minus,
  X,
  Search,
  ChefHat,
  MapPin,
  AlertCircle,
  CheckCircle2,
} from "lucide-react";
import toast from "react-hot-toast";
import {
  createBill,
  BillItem,
  CreateBillRequest,
  getActiveBillConflictID,
} from "../../api/bills";
import {
  businessApi,
  getTablesWithStatus,
  Table,
  getMenu,
  Bundle,
  BundleItemRef,
} from "../../api/business";
import { MenuCategory, MenuItem, MenuItemOption } from "../../api/business";
import { parseMenuCategories } from "../../utils/businessDataParsers";
import { ItemCustomizer } from "./ItemCustomizer";
import ConfirmationModal from "./modals/ConfirmationModal";
import {
  createOrder,
  CreateOrderRequest,
  type Orderability,
  type OrderDraft,
} from "../../api/orders";
import { useOrderQuote } from "@/hooks/useOrderQuote";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { asDollars } from "../../types/money";
import { getAvailableCounters, Counter } from "../../api/counters";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { formatEntityName } from "@/lib/tableLabel";
import { randomUUID } from "@/lib/randomUUID";
import PageHeader from "./shared/PageHeader";

interface BillCreatorProps {
  isOpen: boolean;
  onClose: () => void;
  businessId: number;
  onBillCreated: () => void;
  /** When set, the table with this ID is pre-selected as soon as table data loads. */
  initialTableId?: number;
  // Business default currency. Required so the price chips, totals,
  // and bundle savings render in the operator's actual currency
  // instead of a hardcoded "$".
  currency?: string;
  /** Wave 4: open the created bill's details modal (BillManager.handleViewBill). */
  onViewBill?: (billId: number) => void;
  /** Wave 4: open the created bill's details modal with the record-payment modal pre-opened. */
  onRecordPayment?: (billId: number) => void;
}

interface SelectedItem extends BillItem {
  menuItem?: MenuItem;
  bundle?: Bundle;
  selectedOptions?: MenuItemOption[];
  specialRequests?: string;
  item_type?: "menu_item" | "bundle" | "bundle_item" | "discount";
  bundle_id?: number;
  parent_bundle_id?: number;
  source_offer_id?: number;
}

export const BillCreator: React.FC<BillCreatorProps> = ({
  isOpen,
  onClose,
  businessId,
  onBillCreated,
  initialTableId,
  currency = "USD",
  onViewBill,
  onRecordPayment,
}) => {
  const { locale: currentLocale } = useSimpleLocale();

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `billCreator.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Local formatter so we route every price chip / total through the
  // shared Intl helper without sprinkling the import everywhere.
  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, currency, undefined, currentLocale);

  const [selectedTable, setSelectedTable] = useState<Table | null>(null);
  const [selectedCounter, setSelectedCounter] = useState<Counter | null>(null);
  const [tables, setTables] = useState<Table[]>([]);
  const [counters, setCounters] = useState<Counter[]>([]);
  const [createdBill, setCreatedBill] = useState<{
    id: number;
    label: string;
  } | null>(null);
  // Occupancy is derived from the status board (which nests active bills per
  // table) rather than hydrating every active bill in the business just to know
  // which tables are taken. Only the occupied table id set is needed here.
  const [occupiedTableIds, setOccupiedTableIds] = useState<Set<number>>(
    () => new Set(),
  );
  const [menu, setMenu] = useState<{ categories: MenuCategory[] }>({
    categories: [],
  });
  const [bundles, setBundles] = useState<Bundle[]>([]);
  const [itemOrderability, setItemOrderability] = useState<
    Record<string, Orderability>
  >({});
  const [items, setItems] = useState<SelectedItem[]>([]);
  // Transient per-row text while the operator is retyping a quantity. An entry
  // here (including "") overrides the rendered value so clearing the field to
  // type a new number does not immediately re-clamp or delete the row. Cleared
  // on blur once the value is committed.
  const [pendingQuantities, setPendingQuantities] = useState<
    Record<string, string>
  >({});
  const [isCreating, setIsCreating] = useState(false);
  const [loadingData, setLoadingData] = useState(false);
  // Distinguishes a transient load failure (show error panel + Retry) from a
  // genuinely-empty menu (keep the empty state). Without this a network blip
  // rendered "No menu items available" and misled staff into thinking the menu
  // was empty.
  const [loadError, setLoadError] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedCategory, setSelectedCategory] = useState<string>("all");
  const [notes, setNotes] = useState("");
  const [showItemCustomizer, setShowItemCustomizer] = useState(false);
  const [itemToCustomize, setItemToCustomize] = useState<MenuItem | null>(null);
  // Guards against wiping a built-up order on a stray backdrop click / Escape.
  const {
    isOpen: isDiscardOpen,
    onOpen: onDiscardOpen,
    onOpenChange: onDiscardOpenChange,
  } = useDisclosure();

  const parseBundleItems = (bundle: Bundle): BundleItemRef[] => {
    if (!bundle.items) return [];
    if (Array.isArray(bundle.items)) return bundle.items;
    try {
      const parsed = JSON.parse(bundle.items);
      if (!Array.isArray(parsed)) return [];
      if (typeof parsed[0] === "string") {
        return parsed.map((id: string) => ({ menu_item_id: id, quantity: 1 }));
      }
      return parsed.map((item: any) => ({
        menu_item_id: item.menu_item_id || item.id || "",
        name: item.name,
        quantity: item.quantity || 1,
      }));
    } catch {
      return [];
    }
  };

  const loadData = useCallback(async () => {
    setLoadingData(true);
    setLoadError(false);
    try {
      const [tablesResponse, menuResponse, bundlesResponse] =
        await Promise.all([
          // The status board returns tables WITH their occupancy in one bounded
          // query (no per-bill hydration, no 1,000-row fetch-all).
          getTablesWithStatus(businessId),
          getMenu(businessId),
          businessApi.getBundles(businessId),
        ]);
      const tableRows = tablesResponse.tables || [];
      setTables(tableRows.map((t) => t.table));
      setOccupiedTableIds(
        new Set(
          tableRows
            .filter(
              (t) =>
                t.status === "occupied" ||
                (t.active_bills?.length ?? 0) > 0,
            )
            .map((t) => t.table.id),
        ),
      );
      setBundles(bundlesResponse || []);
      setItemOrderability(menuResponse.item_orderability || {});

      // Try to load counters, but don't fail if they're not available
      try {
        const countersResponse = await getAvailableCounters(businessId);
        setCounters(countersResponse.counters || []);
      } catch (error) {
        setCounters([]);
      }

      setMenu({ categories: parseMenuCategories(menuResponse) });
    } catch (error) {
      console.error("Error loading data:", error);
      setMenu({ categories: [] }); // Ensure safe fallback
      setOccupiedTableIds(new Set()); // Ensure safe fallback
      setBundles([]);
      setItemOrderability({});
      // Flag the failure so we render the retry panel instead of the empty
      // state, which would falsely tell staff the menu has no items.
      setLoadError(true);
    } finally {
      setLoadingData(false);
    }
  }, [businessId]);

  // Load tables and menu when modal opens
  useEffect(() => {
    if (isOpen && businessId) {
      loadData().catch((err) => console.error("loadData failed:", err));
    }
  }, [isOpen, businessId, loadData]);

  // Pre-select the table requested via deep-link (initialTableId) once tables
  // load. The table <Select> only renders tables WITHOUT an active bill, so
  // selecting an occupied table here would leave the field blank (a key with no
  // SelectItem). Instead we only auto-select when the deep-linked table is
  // actually available; if it already has an open bill we surface a warning so
  // staff don't silently start a second bill on an occupied table.
  const deepLinkHandledRef = React.useRef<number | undefined>(undefined);
  // B-6: one key per logical kitchen-order submission. Kept across a failed
  // attempt so the retry replays instead of duplicating; cleared on success
  // and on form reset.
  const orderRequestIdRef = React.useRef<string | null>(null);
  useEffect(() => {
    if (!isOpen) {
      deepLinkHandledRef.current = undefined;
      return;
    }
    if (initialTableId === undefined || tables.length === 0) return;
    // Only act once per opened deep-link to avoid re-toasting on bill refresh.
    if (deepLinkHandledRef.current === initialTableId) return;

    const match = tables.find((t) => t.id === initialTableId);
    if (!match) return;

    const hasOpenBill = occupiedTableIds.has(match.id);

    deepLinkHandledRef.current = initialTableId;
    if (hasOpenBill || !match.is_active) {
      // Cannot render this table in the Select; warn instead of leaving a blank
      // selection that still passes handleCreateBill's truthiness check.
      toast.error(tString("messages.tableOccupied"));
      return;
    }
    setSelectedTable(match);
    setSelectedCounter(null);
  }, [isOpen, initialTableId, tables, occupiedTableIds, tString]);

  const menuLookup = useMemo(() => {
    const byId = new Map<
      string,
      { name: string; category: string; price: number }
    >();
    const byName = new Map<
      string,
      { id?: string; category: string; price: number }
    >();
    menu.categories.forEach((category) => {
      category.items.forEach((item) => {
        if (item.id) {
          byId.set(item.id, {
            name: item.name,
            category: category.name,
            price: item.price || 0,
          });
        }
        byName.set(item.name.toLowerCase(), {
          id: item.id,
          category: category.name,
          price: item.price || 0,
        });
      });
    });
    return { byId, byName };
  }, [menu.categories]);

  const handleItemClick = (menuItem: MenuItem) => {
    // If item has options/add-ons, open customizer
    if (menuItem.options && menuItem.options.length > 0) {
      setItemToCustomize(menuItem);
      setShowItemCustomizer(true);
      return;
    }

    // Otherwise, add directly with quantity 1
    handleItemAdded(menuItem, 1);
  };

  const handleItemAdded = (
    menuItem: MenuItem,
    quantity: number,
    selectedOptions?: MenuItemOption[],
    specialRequests?: string,
  ) => {
    // Calculate price including add-ons
    const addOnPrice = selectedOptions
      ? selectedOptions.reduce(
          (sum, option) => sum + (option.price_change || 0),
          0,
        )
      : 0;
    const totalPrice = menuItem.price + addOnPrice;

    const newItem: SelectedItem = {
      id: `${Date.now()}-${Math.random()}`,
      menu_item_id: menuItem.id || menuItem.name,
      name: menuItem.name,
      price: asDollars(totalPrice),
      quantity: quantity,
      options: selectedOptions
        ? selectedOptions.map((option) => ({
            name: option.name,
            price: asDollars(option.price_change || 0),
          }))
        : [],
      subtotal: asDollars(totalPrice * quantity),
      menuItem,
      selectedOptions,
      specialRequests,
      item_type: "menu_item",
    };
    setItems([...items, newItem]);
  };

  const handleCustomizedItemAdd = (
    item: MenuItem,
    quantity: number,
    selectedOptions: MenuItemOption[],
    specialRequests: string,
  ) => {
    handleItemAdded(item, quantity, selectedOptions, specialRequests);
  };

  const handleBundleAdded = (bundle: Bundle) => {
    if (!bundle.id) return;
    const newItem: SelectedItem = {
      id: `${Date.now()}-${Math.random()}`,
      menu_item_id: `bundle:${bundle.id}`,
      name: bundle.name,
      price: asDollars(bundle.price),
      quantity: 1,
      options: [],
      subtotal: asDollars(bundle.price),
      bundle,
      item_type: "bundle",
      bundle_id: bundle.id,
    };
    setItems((prev) => [...prev, newItem]);
  };

  // Clamp to a positive integer. Quantity can never legitimately drop to 0 via
  // the +/- controls or the number input — only the explicit X/remove button
  // deletes a cart line (and its customizations). Clearing the field to retype
  // must NOT wipe the row, so we floor the committed value at 1.
  const updateItemQuantity = (itemId: string, quantity: number) => {
    const next = Number.isFinite(quantity)
      ? Math.max(1, Math.floor(quantity))
      : 1;
    setItems((items: SelectedItem[]) =>
      items.map((item: SelectedItem) =>
        item.id === itemId
          ? { ...item, quantity: next, subtotal: asDollars(item.price * next) }
          : item,
      ),
    );
  };

  const removeItem = (itemId: string) => {
    setItems((items: SelectedItem[]) =>
      items.filter((item: SelectedItem) => item.id !== itemId),
    );
    // Drop any in-flight transient edit for the removed row.
    setPendingQuantities((prev) => {
      if (!(itemId in prev)) return prev;
      const next = { ...prev };
      delete next[itemId];
      return next;
    });
  };

  // Commit the transient quantity text (on blur / Enter). Parse and clamp to a
  // positive integer, then clear the pending override so the row snaps back to
  // its committed quantity.
  const commitItemQuantity = (itemId: string) => {
    if (!(itemId in pendingQuantities)) return;
    const parsed = parseInt(pendingQuantities[itemId], 10);
    updateItemQuantity(itemId, Number.isNaN(parsed) ? 1 : parsed);
    setPendingQuantities((prev) => {
      if (!(itemId in prev)) return prev;
      const next = { ...prev };
      delete next[itemId];
      return next;
    });
  };

  // While the authoritative quote is loading, show only the neutral cart
  // subtotal. Discounts are never inferred in the browser.
  const neutralCartSubtotal = useMemo(
    () => items.reduce((sum, item) => sum + item.subtotal, 0),
    [items],
  );

  const quoteDraft = useMemo<OrderDraft>(
    () => ({
      items: items.map((item) => ({
        menu_item_name: item.name,
        menu_item_id: item.menu_item_id,
        quantity: item.quantity,
        price: item.price,
        item_type: item.item_type || "menu_item",
        bundle_id: item.bundle_id,
        parent_bundle_id: item.parent_bundle_id,
        source_offer_id: item.source_offer_id,
        options: item.selectedOptions,
        special_requests: item.specialRequests,
      })),
    }),
    [items],
  );
  const {
    quote: serverQuote,
    isPending: quotePending,
    error: quoteError,
    isValid: quoteValid,
    blockedLines: quoteBlockedLines = [],
  } = useOrderQuote({
    businessId,
    draft: quoteDraft,
    enabled: isOpen && items.length > 0,
  });
  const promotionPreview = useMemo(
    () =>
      serverQuote
        ? {
            baseSubtotal: Number(serverQuote.subtotal),
            discountTotal: Number(serverQuote.discount),
            discountedSubtotal: Number(serverQuote.net_subtotal),
            netSubtotal: Number(serverQuote.net_subtotal),
            tax: Number(serverQuote.tax),
            serviceFee: Number(serverQuote.service_fee),
            tip: Number(serverQuote.tip),
            finalTotal: Number(serverQuote.total),
          }
        : {
            baseSubtotal: neutralCartSubtotal,
            discountTotal: 0,
            discountedSubtotal: neutralCartSubtotal,
            netSubtotal: neutralCartSubtotal,
            tax: 0,
            serviceFee: 0,
            tip: 0,
            finalTotal: neutralCartSubtotal,
          },
    [neutralCartSubtotal, serverQuote],
  );

  const handleCreateBill = async () => {
    if (
      (!selectedTable && !selectedCounter) ||
      items.length === 0 ||
      quotePending ||
      quoteError ||
      !quoteValid ||
      !serverQuote
    )
      return;

    setIsCreating(true);
    let billCreated = false;
    try {
      // First, create the bill
      const billData: CreateBillRequest = {
        table_id: selectedTable?.id,
        counter_id: selectedCounter?.id,
        notes: notes.trim(),
        items: items.map((item: SelectedItem) => ({
          id: item.id,
          menu_item_id: item.menu_item_id,
          name: item.name,
          price: item.price,
          quantity: item.quantity,
          options: item.options,
          item_type: item.item_type || "menu_item",
          bundle_id: item.bundle_id,
          parent_bundle_id: item.parent_bundle_id,
          source_offer_id: item.source_offer_id,
          subtotal: item.subtotal,
        })),
      };

      const created = await createBill(businessId, billData);
      // Past this point the bill row exists. If the kitchen order fails we must
      // NOT report a generic "bill create failed" (which makes staff retry and
      // create a duplicate bill) — instead refresh the list and tell them the
      // bill saved but the kitchen order didn't.
      billCreated = true;

      // Then, create the kitchen order
      // Table/counter names already embed the type word ("Table 1", "Counter
      // 1"), so we use the name as-is — prefixing here produced kitchen notes
      // like "Order for Counter Counter 1".
      const orderLocation = (selectedTable ?? selectedCounter)?.name ?? "";
      const orderForLocation = tString("kitchenNote.orderFor").replace(
        "{location}",
        orderLocation,
      );
      const orderNotes = notes.trim()
        ? `${orderForLocation}\n\n${tString("kitchenNote.notesLabel")} ${notes.trim()}`
        : orderForLocation;
      const orderData: CreateOrderRequest = {
        bill_id: created.bill.id,
        notes: orderNotes,
        items: items.map((item: SelectedItem) => ({
          menu_item_name: item.name,
          menu_item_id: item.menu_item_id,
          quantity: item.quantity,
          price: item.price,
          item_type: item.item_type || "menu_item",
          bundle_id: item.bundle_id,
          parent_bundle_id: item.parent_bundle_id,
          source_offer_id: item.source_offer_id,
          options: (item.selectedOptions || []).map((option) => ({
            ...option,
            id:
              option.id ||
              `option-${option.name.toLowerCase().replace(/\s+/g, "-")}`,
            is_required: option.is_required || false,
          })), // Include selected add-ons/options with proper IDs
          special_requests: item.specialRequests || "",
        })),
      };

      if (!orderRequestIdRef.current) {
        orderRequestIdRef.current = randomUUID();
      }
      await createOrder(businessId, orderData, {
        idempotencyKey: orderRequestIdRef.current,
      });
      orderRequestIdRef.current = null;

      toast.success(tString("messages.billCreated"));
      onBillCreated();
      const label = selectedLocationLabel;
      clearFormFields();
      setCreatedBill({ id: created.bill.id, label });
    } catch (error) {
      if (billCreated) {
        console.error("Error creating bill and order:", error);
        // The bill persisted but the kitchen order didn't. Refresh the list so
        // the operator sees the new (orphan) bill, keep the modal open, and
        // surface a targeted message rather than the misleading create-failed.
        toast.error(tString("messages.orderCreateFailed"));
        onBillCreated();
      } else {
        const activeBillID = getActiveBillConflictID(error);
        if (activeBillID !== null) {
          toast.error(tString("messages.locationOccupied"));
          onBillCreated();
          resetForm();
          onViewBill?.(activeBillID);
        } else {
          console.error("Error creating bill and order:", error);
          toast.error(tString("messages.billCreateFailed"));
        }
      }
    } finally {
      setIsCreating(false);
    }
  };

  const clearFormFields = () => {
    orderRequestIdRef.current = null;
    setSelectedTable(null);
    setSelectedCounter(null);
    setItems([]);
    setPendingQuantities({});
    setSearchQuery("");
    setSelectedCategory("all");
    setNotes("");
  };
  const resetForm = () => {
    clearFormFields();
    setOccupiedTableIds(new Set());
    setCounters([]);
    setCreatedBill(null);
    onClose();
  };

  // Intercept any close attempt (backdrop click, Escape, Cancel button). If the
  // operator has already built up an order, confirm before discarding so a stray
  // click can't wipe items and their customizations. With an empty cart, close
  // straight through. Success step is always safe to close.
  const handleAttemptClose = () => {
    if (createdBill) {
      resetForm();
      return;
    }
    if (items.length > 0) {
      onDiscardOpen();
      return;
    }
    resetForm();
  };

  // Filter menu items based on search query
  const filteredMenu = React.useMemo(() => {
    const query = searchQuery.toLowerCase().trim();
    return menu.categories
      .filter(
        (category) =>
          selectedCategory === "all" || category.name === selectedCategory,
      )
      .map((category) => ({
        ...category,
        items: category.items.filter(
          (item) =>
            !query ||
            item.name.toLowerCase().includes(query) ||
            item.description.toLowerCase().includes(query),
        ),
      }))
      .filter((category) => category.items.length > 0);
  }, [menu.categories, searchQuery, selectedCategory]);

  const filteredBundles = useMemo(() => {
    if (selectedCategory !== "all" && selectedCategory !== "__bundles__") {
      return [];
    }
    if (!searchQuery.trim()) {
      return bundles.filter((bundle) => bundle.is_active);
    }
    const query = searchQuery.toLowerCase().trim();
    return bundles.filter(
      (bundle) =>
        bundle.is_active &&
        (bundle.name.toLowerCase().includes(query) ||
          bundle.description?.toLowerCase().includes(query)),
    );
  }, [bundles, searchQuery, selectedCategory]);

  const getBundleInventorySignals = useCallback(
    (bundle: Bundle) => {
      const refs = parseBundleItems(bundle);
      const childDecisions = refs
        .map((ref) => itemOrderability[ref.menu_item_id])
        .filter((decision): decision is Orderability => !!decision);

      return {
        refs,
        blocksSale: childDecisions.some((decision) => !decision.orderable),
        hasWarning: childDecisions.some(
          (decision) => decision.state === "inventory_warning",
        ),
      };
    },
    [itemOrderability],
  );

  const totalSelectedQuantity = items.reduce(
    (sum, item) => sum + item.quantity,
    0,
  );
  const selectedLocationLabel = selectedTable
    ? formatEntityName(tString("form.tableLabel"), selectedTable.name)
    : selectedCounter
      ? formatEntityName(tString("form.counterLabel"), selectedCounter.name)
      : "";

  // Filter out inactive tables and any table already occupied by an active
  // bill (occupancy derived from the status board, not a fetch-all of bills).
  const availableTables = tables.filter(
    (table) => table.is_active && !occupiedTableIds.has(table.id),
  );

  // L2-6: surface the primary create block reason next to the action (S-5).
  const createBlockReason = (() => {
    if (!selectedTable && !selectedCounter) {
      return tString("validation.selectLocation");
    }
    if (items.length === 0) {
      return tString("validation.addItems");
    }
    if (quotePending) {
      return tString("validation.quotePending");
    }
    if (quoteError) {
      return tString("validation.quoteError");
    }
    if (!quoteValid || !serverQuote) {
      return tString("validation.quoteInvalid");
    }
    return null;
  })();

  return (
    <>
    <Modal
      isOpen={isOpen}
      onClose={handleAttemptClose}
      size="5xl"
      scrollBehavior="inside"
      aria-labelledby="bill-creator-title"
      classNames={{
        base: "max-h-[90vh] bg-white text-ink-950",
        body: "p-0",
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-3 border-b border-warm-200 bg-white pb-4">
          {/* Shared PageHeader gives this modal the same serif title size /
              tracking / color as sibling dashboard tabs, removing the visible
              title-size jump. The Modal chrome (5xl, scroll-inside, two-column
              body) can't adopt the full DashboardTabShell without breaking the
              dialog, so we adopt only the header primitive here. */}
          <div id="bill-creator-title">
          <PageHeader
            title={tString("title")}
            subtitle={tString("header.subtitle")}
            className="w-full"
            actions={
              <div className="flex flex-wrap items-center gap-2">
                <Chip
                  variant="flat"
                  color={selectedLocationLabel ? "success" : "warning"}
                  classNames={{
                    base: selectedLocationLabel
                      ? "!bg-emerald-50 border border-emerald-200"
                      : "!bg-amber-50 border border-amber-200",
                    content: selectedLocationLabel
                      ? "!text-emerald-900"
                      : "!text-amber-950",
                  }}
                >
                  {selectedLocationLabel || tString("header.locationRequired")}
                </Chip>
                <Chip
                  variant="flat"
                  color="primary"
                  classNames={{
                    base: "!bg-brand/10 border border-brand/30",
                    content: "!text-ink-950",
                  }}
                >
                  {totalSelectedQuantity} {tString("header.itemsSelected")}
                </Chip>
                <Chip
                  variant="flat"
                  classNames={{
                    base: "!bg-white border border-warm-300",
                    content: "!text-ink-950",
                  }}
                >
                  {formatCurrency(promotionPreview.finalTotal)}
                </Chip>
                {quotePending ? (
                  <Spinner
                    size="sm"
                    aria-label={tString("messages.loadingQuote")}
                  />
                ) : null}
                {quoteError ? (
                  <Chip color="danger" variant="flat" role="alert">
                    {tString("messages.quoteFailed")}
                  </Chip>
                ) : null}
                {quoteBlockedLines.length > 0 ? (
                  <Chip color="danger" variant="flat" role="alert">
                    {tString("messages.quoteBlocked")}
                  </Chip>
                ) : null}
              </div>
            }
          />
          </div>
        </ModalHeader>
        <ModalBody>
          {createdBill ? (
            <div className="p-6" data-testid="bill-creator-success">
              <div className="flex flex-col items-center gap-4 rounded-2xl border border-emerald-200 bg-emerald-50 p-8 text-center">
                <CheckCircle2
                  className="h-10 w-10 text-emerald-600"
                  aria-hidden="true"
                />
                <div>
                  <p className="text-lg font-semibold text-ink-950">
                    {tString("success.title").replace(
                      "{location}",
                      createdBill.label,
                    )}
                  </p>
                  <p className="mt-1 text-sm text-ink-600">
                    {tString("success.subtitle")}
                  </p>
                </div>
                <div className="flex flex-wrap items-center justify-center gap-2">
                  {onViewBill ? (
                    <Button
                      color="primary"
                      onPress={() => {
                        const id = createdBill.id;
                        resetForm();
                        onViewBill(id);
                      }}
                    >
                      {tString("success.viewBill")}
                    </Button>
                  ) : null}
                  {onRecordPayment ? (
                    <Button
                      variant="flat"
                      className="bg-brand/10 font-medium text-ink-950"
                      onPress={() => {
                        const id = createdBill.id;
                        resetForm();
                        onRecordPayment(id);
                      }}
                    >
                      {tString("success.recordPayment")}
                    </Button>
                  ) : null}
                  <Button variant="light" onPress={resetForm}>
                    {tString("success.done")}
                  </Button>
                </div>
              </div>
            </div>
          ) : loadingData ? (
            <div className="flex justify-center items-center py-8">
              <Spinner size="lg" />
            </div>
          ) : loadError ? (
            // Transient load failure: show a retry panel instead of the empty
            // state so staff don't mistake a network blip for an empty menu.
            <div className="p-4">
              <div
                data-testid="bill-creator-load-error"
                className="rounded-xl border border-red-200 bg-red-50 p-6 flex flex-col items-center gap-3 text-center"
              >
                <AlertCircle className="w-8 h-8 text-red-400" />
                <p className="text-sm text-red-700">
                  {tString("messages.loadError")}
                </p>
                <Button
                  size="sm"
                  color="danger"
                  variant="flat"
                  onPress={() => {
                    loadData().catch((err) =>
                      console.error("loadData failed:", err),
                    );
                  }}
                >
                  {tString("buttons.retry")}
                </Button>
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_360px]">
              {/* Menu Selection */}
              <div className="min-w-0">
                <Card className="border border-warm-200/90 bg-white/90 shadow-sm shadow-warm-900/5">
                  <CardHeader className="pb-2">
                    <div className="w-full space-y-3">
                      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                        <div>
                          <h3 className="font-title text-lg font-semibold tracking-0 text-ink-950">
                            {tString("menu.title")}
                          </h3>
                          <p className="text-sm text-ink-600">
                            {tString("menu.description")}
                          </p>
                        </div>
                        <Input
                          placeholder={tString("menu.searchPlaceholder")}
                          value={searchQuery}
                          onValueChange={setSearchQuery}
                          startContent={
                            <Search className="h-4 w-4 text-ink-700" />
                          }
                          variant="bordered"
                          size="sm"
                          className="w-full lg:max-w-xs"
                          isClearable
                          onClear={() => setSearchQuery("")}
                        />
                      </div>

                      <div className="flex flex-wrap gap-2">
                        <Button
                          size="sm"
                          color={
                            selectedCategory === "all" ? "primary" : "default"
                          }
                          variant={
                            selectedCategory === "all" ? "solid" : "flat"
                          }
                          onPress={() => setSelectedCategory("all")}
                        >
                          {tString("menu.allCategories")}
                        </Button>
                        {bundles.some((bundle) => bundle.is_active) && (
                          <Button
                            size="sm"
                            color={
                              selectedCategory === "__bundles__"
                                ? "primary"
                                : "default"
                            }
                            variant={
                              selectedCategory === "__bundles__"
                                ? "solid"
                                : "flat"
                            }
                            onPress={() => setSelectedCategory("__bundles__")}
                          >
                            {tString("menu.bundlesTitle")}
                          </Button>
                        )}
                        {menu.categories.map((category) => (
                          <Button
                            key={category.name}
                            size="sm"
                            color={
                              selectedCategory === category.name
                                ? "primary"
                                : "default"
                            }
                            variant={
                              selectedCategory === category.name
                                ? "solid"
                                : "flat"
                            }
                            onPress={() => setSelectedCategory(category.name)}
                          >
                            {category.name}
                          </Button>
                        ))}
                      </div>
                    </div>
                  </CardHeader>
                  <CardBody className="max-h-[34rem] overflow-y-auto pt-2">
                    {filteredMenu.length === 0 &&
                    filteredBundles.length === 0 ? (
                      <div className="text-center py-8">
                        <Search className="mx-auto mb-4 h-12 w-12 text-ink-400" />
                        <h4 className="mb-2 text-lg font-semibold text-ink-700">
                          {searchQuery
                            ? tString("menu.noItemsFound")
                            : tString("menu.noMenuItems")}
                        </h4>
                        <p className="text-ink-700">
                          {searchQuery
                            ? tString("menu.noItemsMatch").replace(
                                "{query}",
                                searchQuery,
                              )
                            : tString("menu.noItemsAvailable")}
                        </p>
                        {searchQuery && (
                          <Button
                            variant="light"
                            color="primary"
                            size="sm"
                            onPress={() => setSearchQuery("")}
                            className="mt-3"
                          >
                            {tString("menu.clearSearch")}
                          </Button>
                        )}
                      </div>
                    ) : (
                      <>
                        {filteredBundles.length > 0 && (
                          <div className="mb-4">
                            <h4 className="mb-2 font-title text-sm font-semibold tracking-0 text-ink-950">
                              {tString("menu.bundlesTitle")}
                            </h4>
                            <div className="grid grid-cols-1 gap-2">
                              {filteredBundles.map((bundle) => {
                                const {
                                  refs,
                                  blocksSale: bundleBlocked,
                                  hasWarning: bundleHasWarning,
                                } = getBundleInventorySignals(bundle);
                                const regularTotal = refs.reduce((sum, ref) => {
                                  const resolved = menuLookup.byId.get(
                                    ref.menu_item_id,
                                  );
                                  return (
                                    sum +
                                    (resolved?.price || 0) * (ref.quantity || 1)
                                  );
                                }, 0);
                                const savings = Math.max(
                                  regularTotal - bundle.price,
                                  0,
                                );
                                return (
                                  <Card
                                    key={bundle.id || bundle.name}
                                    isPressable={!bundleBlocked}
                                    onPress={() =>
                                      bundleBlocked
                                        ? undefined
                                        : handleBundleAdded(bundle)
                                    }
                                    className={
                                      bundleBlocked
                                        ? "border border-brand/20 bg-brand/5 opacity-60 shadow-none"
                                        : "border border-brand/20 bg-white/80 shadow-sm transition hover:bg-brand/5"
                                    }
                                  >
                                    <CardBody className="p-3">
                                      <div className="flex justify-between items-start">
                                        <div className="flex-1">
                                          <div className="flex items-center gap-2">
                                            <p className="font-semibold text-ink-950">
                                              {bundle.name}
                                            </p>
                                            <Chip
                                              size="sm"
                                              color="primary"
                                              variant="flat"
                                              classNames={{
                                                base:
                                                  "!bg-brand/10 border border-brand/30",
                                                content:
                                                  "!text-ink-950",
                                              }}
                                            >
                                              {tString("menu.bundleBadge")}
                                            </Chip>
                                            {bundleBlocked && (
                                              <Chip
                                                size="sm"
                                                color="danger"
                                                variant="flat"
                                              >
                                                {tString(
                                                  "inventoryWarnings.outBadge",
                                                )}
                                              </Chip>
                                            )}
                                            {!bundleBlocked &&
                                              bundleHasWarning && (
                                                <Chip
                                                  size="sm"
                                                  color="warning"
                                                  variant="flat"
                                                  classNames={{
                                                    base:
                                                      "!bg-amber-50 border border-amber-200",
                                                    content:
                                                      "!text-amber-950",
                                                  }}
                                                >
                                                  {tString(
                                                    "inventoryWarnings.lowBadge",
                                                  )}
                                                </Chip>
                                              )}
                                          </div>
                                          {bundle.description && (
                                            <p className="text-small text-ink-600">
                                              {bundle.description}
                                            </p>
                                          )}
                                          {bundleBlocked && (
                                            <p className="mt-1 text-xs font-semibold text-rose-700">
                                              {tString(
                                                "inventoryWarnings.outDescription",
                                              )}
                                            </p>
                                          )}
                                          {!bundleBlocked &&
                                            bundleHasWarning && (
                                              <p className="mt-1 text-xs font-semibold text-amber-900">
                                                {tString(
                                                  "inventoryWarnings.lowDescription",
                                                )}
                                              </p>
                                            )}
                                          {refs.length > 0 && (
                                            <p className="mt-1 text-xs text-ink-700">
                                              {refs
                                                .map(
                                                  (ref) =>
                                                    `${ref.quantity}x ${
                                                      ref.name ||
                                                      menuLookup.byId.get(
                                                        ref.menu_item_id,
                                                      )?.name ||
                                                      ref.menu_item_id
                                                    }`,
                                                )
                                                .join(", ")}
                                            </p>
                                          )}
                                        </div>
                                        <div className="text-right">
                                          <Chip
                                            color="primary"
                                            variant="flat"
                                            classNames={{
                                              base:
                                                "!bg-brand/10 border border-brand/30",
                                              content: "!text-ink-950",
                                            }}
                                          >
                                            {formatCurrency(bundle.price || 0)}
                                          </Chip>
                                          {savings > 0 && (
                                            <p className="mt-1 text-xs font-semibold text-emerald-700">
                                              {tString("menu.bundleSave")}{" "}
                                              {formatCurrency(savings)}
                                            </p>
                                          )}
                                        </div>
                                      </div>
                                    </CardBody>
                                  </Card>
                                );
                              })}
                            </div>
                          </div>
                        )}

                        {filteredMenu.map((category, categoryIndex) => (
                          <div key={categoryIndex} className="mb-4">
                            <h4 className="mb-2 font-title text-sm font-semibold tracking-0 text-ink-950">
                              {category.name}
                              {searchQuery && (
                                <Chip
                                  size="sm"
                                  variant="flat"
                                  color="primary"
                                  className="ml-2"
                                  classNames={{
                                    base:
                                      "!bg-brand/10 border border-brand/30",
                                    content: "!text-ink-950",
                                  }}
                                >
                                  {category.items.length}{" "}
                                  {category.items.length !== 1
                                    ? tString("menu.items")
                                    : tString("menu.item")}
                                </Chip>
                              )}
                            </h4>
                            <div className="grid grid-cols-1 gap-2">
                              {category.items.map((item, itemIndex) => {
                                const decision = item.id
                                  ? itemOrderability[item.id]
                                  : undefined;
                                const canSelectItem = decision
                                  ? decision.orderable
                                  : item.is_available !== false;
                                const inventoryBlocked =
                                  decision?.state === "inventory_out";
                                const inventoryWarning =
                                  decision?.state === "inventory_warning";
                                return (
                                  <Card
                                    key={itemIndex}
                                    isPressable={canSelectItem}
                                    onPress={() =>
                                      canSelectItem
                                        ? handleItemClick(item)
                                        : undefined
                                    }
                                    className={
                                      canSelectItem
                                        ? "border border-warm-200/90 bg-white/80 shadow-sm transition hover:bg-brand/5"
                                        : "border border-warm-200/90 bg-white opacity-75 shadow-none"
                                    }
                                  >
                                    <CardBody className="p-3">
                                      <div className="flex justify-between items-center">
                                        <div className="flex-1">
                                          <div className="flex items-center gap-2">
                                            <p className="font-semibold text-ink-950">
                                              {item.name}
                                            </p>
                                            {item.options &&
                                              item.options.length > 0 && (
                                                <Chip
                                                  size="sm"
                                                  color="secondary"
                                                  variant="dot"
                                                  classNames={{
                                                    base:
                                                      "!border-ink-300 !text-ink-950",
                                                    content: "!text-ink-950",
                                                  }}
                                                >
                                                  {tString("menu.addOns")}
                                                </Chip>
                                              )}
                                            {inventoryBlocked && (
                                              <Chip
                                                size="sm"
                                                color="danger"
                                                variant="flat"
                                              >
                                                {tString(
                                                  "inventoryWarnings.outBadge",
                                                )}
                                              </Chip>
                                            )}
                                            {inventoryWarning && (
                                              <Chip
                                                size="sm"
                                                color="warning"
                                                variant="flat"
                                                classNames={{
                                                  base:
                                                    "!bg-amber-50 border border-amber-200",
                                                  content: "!text-amber-950",
                                                }}
                                              >
                                                {tString(
                                                  "inventoryWarnings.lowBadge",
                                                )}
                                              </Chip>
                                            )}
                                          </div>
                                          {item.description && (
                                            <p className="text-small text-ink-600">
                                              {item.description}
                                            </p>
                                          )}
                                          {inventoryBlocked && (
                                            <p className="mt-1 text-xs font-semibold text-rose-700">
                                              {tString(
                                                "inventoryWarnings.outDescription",
                                              )}
                                            </p>
                                          )}
                                          {inventoryWarning && (
                                            <p className="mt-1 text-xs font-semibold text-amber-900">
                                              {tString(
                                                "inventoryWarnings.lowDescription",
                                              )}
                                            </p>
                                          )}
                                          {decision?.state ===
                                            "manual_disabled" && (
                                            <p className="mt-1 text-xs font-semibold text-rose-700">
                                              {tString("menu.unavailable")}
                                            </p>
                                          )}
                                        </div>
                                        <div className="flex items-center gap-2">
                                          <Chip
                                            color="primary"
                                            variant="flat"
                                            classNames={{
                                              base:
                                                "!bg-brand/10 border border-brand/30",
                                              content:
                                                "!text-ink-950",
                                            }}
                                          >
                                            {formatCurrency(item.price || 0)}
                                          </Chip>
                                          <span
                                            className="flex h-8 w-8 items-center justify-center rounded-full bg-brand/10 text-ink-950"
                                            aria-hidden="true"
                                          >
                                            <Plus className="h-4 w-4" />
                                          </span>
                                        </div>
                                      </div>
                                    </CardBody>
                                  </Card>
                                );
                              })}
                            </div>
                          </div>
                        ))}
                      </>
                    )}
                  </CardBody>
                </Card>
              </div>

              {/* Bill Summary */}
              <div className="xl:sticky xl:top-0 xl:self-start">
                <Card className="border border-warm-200/90 bg-white/95 shadow-card">
                  <CardHeader>
                    <div className="w-full space-y-3">
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <h3 className="font-title text-lg font-semibold tracking-0 text-ink-950">
                            {tString("billSummary.title")}
                          </h3>
                          <p className="text-sm text-ink-600">
                            {tString("billSummary.description")}
                          </p>
                        </div>
                        <Chip
                          color={items.length > 0 ? "primary" : "default"}
                          variant="flat"
                          classNames={{
                            base:
                              items.length > 0
                                ? "!bg-brand/10 border border-brand/30"
                                : "!bg-white border border-warm-300",
                            content: "!text-ink-950",
                          }}
                        >
                          {items.length} {tString("billSummary.items")}
                        </Chip>
                      </div>

                      <div
                        className={`rounded-2xl border px-3 py-3 ${
                          selectedLocationLabel
                            ? "border-emerald-200 bg-emerald-50"
                            : "border-amber-200 bg-amber-50"
                        }`}
                      >
                        <div className="flex items-start gap-3">
                          <div
                            className={`rounded-full p-2 ${
                              selectedLocationLabel
                                ? "bg-emerald-100"
                                : "bg-amber-100"
                            }`}
                          >
                            <MapPin
                              className={`h-4 w-4 ${
                                selectedLocationLabel
                                  ? "text-emerald-700"
                                  : "text-amber-900"
                              }`}
                            />
                          </div>
                          <div>
                            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-ink-600">
                              {tString("billSummary.location")}
                            </p>
                            <p className="text-sm font-semibold text-ink-950">
                              {selectedLocationLabel ||
                                tString("billSummary.locationPlaceholder")}
                            </p>
                            <p className="text-xs text-ink-600">
                              {selectedLocationLabel
                                ? tString("billSummary.locationReady")
                                : tString("billSummary.locationHint")}
                            </p>
                          </div>
                        </div>
                      </div>
                    </div>
                  </CardHeader>
                  <CardBody>
                    {/* Table/Counter Selection */}
                    <div className="mb-4 space-y-4">
                      {/* Table Selection */}
                      <div>
                        <Select
                          label={tString("form.selectTable")}
                          classNames={{
                            base: "!opacity-100",
                            label: "!text-ink-700",
                            trigger:
                              "!bg-white border border-warm-200 data-[disabled=true]:!bg-white data-[disabled=true]:!opacity-100",
                            value:
                              "!text-ink-700 group-data-[has-value=true]:!text-ink-950",
                          }}
                          placeholder={
                            availableTables.length > 0
                              ? tString("form.chooseTable")
                              : tString("form.noTablesAvailable")
                          }
                          selectedKeys={
                            selectedTable ? [selectedTable.id.toString()] : []
                          }
                          onSelectionChange={(keys) => {
                            const key = Array.from(keys)[0] as string;
                            const table = availableTables.find(
                              (t) => t.id.toString() === key,
                            );
                            setSelectedTable(table || null);
                            if (table) setSelectedCounter(null); // Clear counter selection
                          }}
                          isDisabled={
                            availableTables.length === 0 ||
                            selectedCounter !== null
                          }
                        >
                          {availableTables.map((table) => (
                            <SelectItem
                              key={table.id.toString()}
                              value={table.id.toString()}
                            >
                              {table.name}
                            </SelectItem>
                          ))}
                        </Select>
                        {availableTables.length === 0 &&
                          counters.length === 0 && (
                            <p className="mt-2 text-small font-medium text-amber-800">
                              {tString("form.allTablesHaveBills")}
                            </p>
                          )}
                        {availableTables.length === 0 &&
                          counters.length > 0 &&
                          !selectedCounter && (
                            <p className="mt-2 text-small text-ink-700">
                              {tString("form.allTablesOccupied")}
                            </p>
                          )}
                      </div>

                      {/* Counter Selection */}
                      <div>
                        <div className="flex items-center justify-center mb-2">
                          <div className="flex-1 border-t border-warm-200"></div>
                          <span className="px-3 text-small text-ink-700">
                            {tString("form.or")}
                          </span>
                          <div className="flex-1 border-t border-warm-200"></div>
                        </div>

                        {counters.length > 0 ? (
                          <div>
                            <Select
                              label={tString("form.selectCounter")}
                              placeholder={tString("form.chooseCounter")}
                              classNames={{
                                label: "!text-ink-700",
                                trigger:
                                  "!bg-white border border-warm-200 data-[disabled=true]:!bg-white data-[disabled=true]:!opacity-100",
                                value:
                                  "!text-ink-700 group-data-[has-value=true]:!text-ink-950",
                              }}
                              selectedKeys={
                                selectedCounter
                                  ? [selectedCounter.id.toString()]
                                  : []
                              }
                              onSelectionChange={(keys) => {
                                const key = Array.from(keys)[0] as string;
                                const counter = counters.find(
                                  (c) => c.id.toString() === key,
                                );
                                setSelectedCounter(counter || null);
                                if (counter) setSelectedTable(null); // Clear table selection
                              }}
                              isDisabled={selectedTable !== null}
                            >
                              {counters.map((counter) => (
                                <SelectItem
                                  key={counter.id.toString()}
                                  value={counter.id.toString()}
                                >
                                  {counter.name}
                                </SelectItem>
                              ))}
                            </Select>
                            <p className="mt-1 text-small text-ink-700">
                              {tString("form.perfectForTakeaway")}
                            </p>
                          </div>
                        ) : (
                          <div className="text-center py-4">
                            <p className="mb-2 text-small font-medium text-ink-800">
                              {tString("form.noCountersAvailable")}
                            </p>
                            <p className="text-tiny text-ink-700">
                              {tString("form.enableCountersMessage")}
                            </p>
                          </div>
                        )}
                      </div>

                      {/* Order Notes */}
                      <div>
                        <Textarea
                          label={tString("form.orderNotes")}
                          placeholder={tString("form.orderNotesPlaceholder")}
                          value={notes}
                          onValueChange={setNotes}
                          variant="bordered"
                          minRows={2}
                          maxRows={4}
                          description={tString("form.orderNotesDescription")}
                        />
                      </div>
                    </div>

                    <Divider className="mb-4" />

                    {/* Selected Items */}
                    <div className="mb-4">
                      <h4 className="font-medium mb-2">
                        {tString("billSummary.selectedItems")} ({items.length})
                      </h4>
                      <div className="max-h-48 overflow-y-auto space-y-2">
                        {items.length === 0 ? (
                          <div className="rounded-2xl border border-dashed border-warm-300 bg-white px-4 py-8 text-center">
                            <ChefHat className="mx-auto mb-2 h-8 w-8 text-ink-700" />
                            <p className="text-sm font-semibold text-ink-700">
                              {tString("billSummary.noItemsSelected")}
                            </p>
                            <p className="text-sm text-ink-600">
                              {tString("billSummary.noItemsDescription")}
                            </p>
                          </div>
                        ) : (
                          items.map((item: SelectedItem) => (
                            <Card
                              key={item.id}
                              className="border border-warm-200/90 bg-white p-2 shadow-none"
                            >
                              <div className="flex justify-between items-start">
                                <div className="flex-1">
                                  <div className="flex items-center gap-2">
                                    <p className="text-sm font-semibold text-ink-950">
                                      {item.name}
                                    </p>
                                    {item.item_type === "bundle" && (
                                      <Chip
                                        size="sm"
                                        variant="flat"
                                        color="primary"
                                        classNames={{
                                          base:
                                            "!bg-brand/10 border border-brand/30",
                                          content:
                                            "!text-ink-950",
                                        }}
                                      >
                                        {tString("menu.bundleBadge")}
                                      </Chip>
                                    )}
                                  </div>
                                  <p className="text-xs text-ink-600">
                                    {formatCurrency(item.price || 0)}{" "}
                                    {tString("billSummary.each")}
                                  </p>
                                  {item.selectedOptions &&
                                    item.selectedOptions.length > 0 && (
                                      <div className="mt-1 text-xs text-ink-700">
                                        {tString("billSummary.addOns")}:{" "}
                                        {item.selectedOptions
                                          .map((opt) => opt.name)
                                          .join(", ")}
                                      </div>
                                    )}
                                  {item.specialRequests && (
                                    <div className="mt-1 text-xs text-ink-700">
                                      {tString("billSummary.note")}:{" "}
                                      {item.specialRequests}
                                    </div>
                                  )}
                                </div>
                                <div className="flex items-center gap-1">
                                  <Button
                                    isIconOnly
                                    size="sm"
                                    variant="light"
                                    aria-label={tString(
                                      "cart.decreaseQuantityAria",
                                    ).replace("{name}", item.name)}
                                    onPress={() =>
                                      updateItemQuantity(
                                        item.id,
                                        item.quantity - 1,
                                      )
                                    }
                                  >
                                    <Minus className="w-3 h-3" />
                                  </Button>
                                  <Input
                                    size="sm"
                                    className="w-12"
                                    aria-label={tString(
                                      "cart.quantityAria",
                                    ).replace("{name}", item.name)}
                                    inputMode="numeric"
                                    value={
                                      item.id in pendingQuantities
                                        ? pendingQuantities[item.id]
                                        : item.quantity.toString()
                                    }
                                    onChange={(e) => {
                                      // Store the raw text transiently so an empty
                                      // field (mid-retype) keeps the row instead of
                                      // deleting it. Committed on blur/Enter.
                                      const raw = e.target.value;
                                      if (/^\d*$/.test(raw)) {
                                        setPendingQuantities((prev) => ({
                                          ...prev,
                                          [item.id]: raw,
                                        }));
                                      }
                                    }}
                                    onBlur={() => commitItemQuantity(item.id)}
                                    onKeyDown={(e) => {
                                      if (e.key === "Enter") {
                                        commitItemQuantity(item.id);
                                      }
                                    }}
                                  />
                                  <Button
                                    isIconOnly
                                    size="sm"
                                    variant="light"
                                    aria-label={tString(
                                      "cart.increaseQuantityAria",
                                    ).replace("{name}", item.name)}
                                    onPress={() =>
                                      updateItemQuantity(
                                        item.id,
                                        item.quantity + 1,
                                      )
                                    }
                                  >
                                    <Plus className="w-3 h-3" />
                                  </Button>
                                  <Button
                                    isIconOnly
                                    size="sm"
                                    color="danger"
                                    variant="light"
                                    aria-label={tString(
                                      "cart.removeItemAria",
                                    ).replace("{name}", item.name)}
                                    onPress={() => removeItem(item.id)}
                                  >
                                    <X className="w-3 h-3" />
                                  </Button>
                                </div>
                              </div>
                              <div className="flex justify-between items-center mt-1">
                                <span className="text-xs text-ink-600">
                                  {item.quantity} ×{" "}
                                  {formatCurrency(item.price || 0)}
                                </span>
                                <span className="font-medium text-sm">
                                  {formatCurrency(item.subtotal || 0)}
                                </span>
                              </div>
                            </Card>
                          ))
                        )}
                      </div>
                    </div>

                    <Divider className="mb-4" />

                    {/* Totals — L2-7: min-w-0 labels + tabular-nums values so
                        long labels never collide with amounts. */}
                    <div className="space-y-2">
                      <div className="flex justify-between gap-3">
                        <span className="min-w-0">
                          {tString("billSummary.subtotal")}
                        </span>
                        <span className="whitespace-nowrap tabular-nums">
                          {formatCurrency(promotionPreview.baseSubtotal || 0)}
                        </span>
                      </div>
                      {promotionPreview.discountTotal > 0 && (
                        <div className="flex justify-between gap-3 font-medium text-emerald-700">
                          <span className="min-w-0">
                            {tString("billSummary.autoDiscounts")}
                          </span>
                          <span className="whitespace-nowrap tabular-nums">
                            -{" "}
                            {formatCurrency(
                              promotionPreview.discountTotal || 0,
                            )}
                          </span>
                        </div>
                      )}
                      <div className="flex justify-between gap-3 font-semibold">
                        <span className="min-w-0">
                          {tString("billSummary.totalBeforeTaxService")}
                        </span>
                        <span className="whitespace-nowrap tabular-nums">
                          {formatCurrency(promotionPreview.discountedSubtotal)}
                        </span>
                      </div>
                      <div className="flex justify-between gap-3">
                        <span className="min-w-0">
                          {tString("billSummary.tax")}
                        </span>
                        <span className="whitespace-nowrap tabular-nums">
                          {formatCurrency(promotionPreview.tax)}
                        </span>
                      </div>
                      <div className="flex justify-between gap-3">
                        <span className="min-w-0">
                          {tString("billSummary.serviceFee")}
                        </span>
                        <span className="whitespace-nowrap tabular-nums">
                          {formatCurrency(promotionPreview.serviceFee)}
                        </span>
                      </div>
                      {promotionPreview.tip > 0 && (
                        <div className="flex justify-between gap-3">
                          <span className="min-w-0">
                            {tString("billSummary.tip")}
                          </span>
                          <span className="whitespace-nowrap tabular-nums">
                            {formatCurrency(promotionPreview.tip)}
                          </span>
                        </div>
                      )}
                      <div className="flex justify-between gap-3 border-t border-warm-200 pt-2 font-semibold">
                        <span className="min-w-0">
                          {tString("billSummary.finalTotal")}
                        </span>
                        <span className="whitespace-nowrap tabular-nums">
                          {formatCurrency(promotionPreview.finalTotal)}
                        </span>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              </div>
            </div>
          )}
        </ModalBody>
        <ModalFooter>
          {createdBill ? null : (
            <>
              <Button variant="light" onPress={handleAttemptClose}>
                {tString("buttons.cancel")}
              </Button>
              <div className="flex flex-col items-end gap-1">
                <Button
                  color="primary"
                  onPress={handleCreateBill}
                  isLoading={isCreating}
                  isDisabled={Boolean(createBlockReason) || isCreating}
                  aria-describedby={
                    createBlockReason ? "bill-creator-block-reason" : undefined
                  }
                >
                  {`${tString("buttons.createBill")} ${formatCurrency(
                    promotionPreview.finalTotal,
                  )}`}
                </Button>
                {createBlockReason ? (
                  <p
                    id="bill-creator-block-reason"
                    className="max-w-xs text-right text-xs text-rose-700"
                    role="status"
                  >
                    {createBlockReason}
                  </p>
                ) : null}
              </div>
            </>
          )}
        </ModalFooter>
      </ModalContent>
    </Modal>

    {/* L2-5: nested modals must be siblings of the parent Modal, not children. */}
    {showItemCustomizer && itemToCustomize && (
      <ItemCustomizer
        isOpen={showItemCustomizer}
        onClose={() => {
          setShowItemCustomizer(false);
          setItemToCustomize(null);
        }}
        item={itemToCustomize}
        onAddToCart={handleCustomizedItemAdd}
        currency={currency}
      />
    )}

    <ConfirmationModal
      isOpen={isDiscardOpen}
      onOpenChange={onDiscardOpenChange}
      title={tString("discardOrderTitle")}
      description={tString("discardOrderDescription")}
      cancelLabel={tString("keepEditing")}
      confirmLabel={tString("discard")}
      isDanger
      onConfirm={resetForm}
    />
    </>
  );
};
