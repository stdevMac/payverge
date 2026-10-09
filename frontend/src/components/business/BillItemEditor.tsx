import React, { useEffect, useState } from "react";
import {
  Button,
  Chip,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Spinner,
  Textarea,
} from "@nextui-org/react";
import {
  AlertCircle,
  ChefHat,
  Minus,
  Plus,
  Receipt,
  Search,
  Trash2,
} from "lucide-react";
import toast from "react-hot-toast";
import {
  getMenu,
  MenuCategory,
  MenuItem,
  MenuItemOption,
} from "../../api/business";
import { parseMenuCategories } from "../../utils/businessDataParsers";
import { addBillItem, adjustBillItem, BillItem } from "../../api/bills";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { ItemCustomizer } from "./ItemCustomizer";
import { asDollars } from "../../types/money";
// IMP-14: voids on bill items go through the manager-PIN prompt when the
// backend gates the call. The hook is a no-op (single attempt) when the
// caller is an owner or unenrolled staff.
import { useWithManagerPin } from "./managerPin";

interface BillItemEditorProps {
  billId: number;
  businessId: number;
  currentItems: BillItem[];
  onItemsChanged: () => void;
  tString: (key: string) => string;
  // Business default currency for the menu prices + per-item subtotals
  // rendered while editing a bill. Was hardcoded "$" prior to this.
  currency?: string;
  // Operator UI locale so money grouping follows it (Audit A-04). Defaults to
  // en-US grouping when omitted.
  locale?: string;
}

type MenuEntry = MenuItem & { categoryName: string };

const getLookupKey = (menuItemId?: string, name?: string) =>
  (menuItemId || name || "").trim().toLowerCase();

export const BillItemEditor = React.memo(function BillItemEditor({
  billId,
  businessId,
  currentItems,
  onItemsChanged,
  tString,
  currency = "USD",
  locale = "en-US",
}: BillItemEditorProps) {
  const [menuCategories, setMenuCategories] = useState<MenuCategory[]>([]);
  const [menuLoading, setMenuLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedCategory, setSelectedCategory] = useState<string>("all");
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [itemPendingVoid, setItemPendingVoid] = useState<BillItem | null>(null);
  const [voidReason, setVoidReason] = useState("");
  const [itemToCustomize, setItemToCustomize] = useState<MenuItem | null>(null);
  const { withManagerPin } = useWithManagerPin();

  useEffect(() => {
    const loadMenu = async () => {
      setMenuLoading(true);
      try {
        const menu = await getMenu(businessId);
        setMenuCategories(parseMenuCategories(menu));
      } catch (error) {
        console.error("Error loading menu:", error);
      } finally {
        setMenuLoading(false);
      }
    };

    loadMenu().catch((error) => console.error("loadMenu failed:", error));
  }, [businessId]);

  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, currency, undefined, locale);

  const editableCurrentItems = currentItems
    .filter((item) => {
      const type = item.item_type || item.itemType || "menu_item";
      return type !== "bundle_item" && type !== "discount";
    })
    .sort((left, right) => {
      if ((right.subtotal || 0) !== (left.subtotal || 0)) {
        return (right.subtotal || 0) - (left.subtotal || 0);
      }
      return left.name.localeCompare(right.name);
    });

  const autoDiscounts = currentItems.filter(
    (item) => (item.item_type || item.itemType || "menu_item") === "discount",
  );

  const totalCurrentQuantity = editableCurrentItems.reduce(
    (sum, item) => sum + (item.quantity || 0),
    0,
  );
  const editableSubtotal = editableCurrentItems.reduce(
    (sum, item) => sum + (item.subtotal || 0),
    0,
  );
  const autoDiscountTotal = autoDiscounts.reduce(
    (sum, item) => sum + (item.subtotal || 0),
    0,
  );
  const liveSubtotal = editableSubtotal + autoDiscountTotal;

  const billPresence = new Map<string, { lines: number; quantity: number }>();
  editableCurrentItems.forEach((item) => {
    const key = getLookupKey(item.menu_item_id, item.name);
    const existing = billPresence.get(key) || { lines: 0, quantity: 0 };
    existing.lines += 1;
    existing.quantity += item.quantity || 0;
    billPresence.set(key, existing);
  });

  const allMenuItems: MenuEntry[] = menuCategories.flatMap((category) =>
    (category.items || []).map((item) => ({
      ...item,
      categoryName: category.name,
    })),
  );

  const filteredMenuItems = allMenuItems
    .filter((item) => {
      const query = searchQuery.trim().toLowerCase();
      const matchesQuery =
        !query ||
        item.name.toLowerCase().includes(query) ||
        item.description?.toLowerCase().includes(query);
      const matchesCategory =
        selectedCategory === "all" || item.categoryName === selectedCategory;

      return matchesQuery && matchesCategory;
    })
    .sort((left, right) => {
      const leftPresence =
        billPresence.get(getLookupKey(left.id, left.name))?.quantity || 0;
      const rightPresence =
        billPresence.get(getLookupKey(right.id, right.name))?.quantity || 0;

      if (Number(right.is_available) !== Number(left.is_available)) {
        return Number(right.is_available) - Number(left.is_available);
      }

      if (rightPresence !== leftPresence) {
        return rightPresence - leftPresence;
      }

      return left.name.localeCompare(right.name);
    });

  const handleAddItem = async (
    item: MenuItem,
    quantity = 1,
    selectedOptions: MenuItemOption[] = [],
  ) => {
    const itemKey = item.id || item.name;
    const addOnPrice = selectedOptions.reduce(
      (sum, option) => sum + (option.price_change || 0),
      0,
    );
    const unitPrice = item.price + addOnPrice;

    setActionLoading(`add-${itemKey}`);
    try {
      await addBillItem(billId, {
        menu_item_id: item.id || item.name,
        name: item.name,
        price: asDollars(unitPrice),
        quantity,
        options: selectedOptions.map((option) => ({
          name: option.name,
          price: asDollars(option.price_change || 0),
        })),
      });

      toast.success(
        quantity > 1
          ? tString("editItems.itemAddedWithQuantity")
              .replace("{item}", item.name)
              .replace("{quantity}", String(quantity))
          : tString("editItems.itemAdded").replace("{item}", item.name),
      );
      onItemsChanged();
    } catch (error) {
      console.error("Error adding item:", error);
      toast.error(tString("editItems.itemAddFailed"));
    } finally {
      setActionLoading(null);
      setItemToCustomize(null);
    }
  };

  const handleQuantityChange = async (item: BillItem, nextQuantity: number) => {
    setActionLoading(`qty-${item.id}`);
    try {
      await adjustBillItem(billId, item.id, { quantity: nextQuantity });
      toast.success(
        tString("editItems.quantityUpdated")
          .replace("{item}", item.name)
          .replace("{from}", String(item.quantity))
          .replace("{to}", String(nextQuantity)),
      );
      onItemsChanged();
    } catch (error) {
      console.error("Error adjusting item quantity:", error);
      toast.error(tString("editItems.quantityUpdateFailed"));
    } finally {
      setActionLoading(null);
    }
  };

  const isActionLoading = (key: string) => actionLoading === key;

  const handleVoidPrompt = (item: BillItem) => {
    setItemPendingVoid(item);
    setVoidReason("");
  };

  const closeVoidModal = () => {
    setItemPendingVoid(null);
    setVoidReason("");
  };

  // Returns true only when the void actually succeeded. Callers must NOT close
  // the surrounding editor on a false result, so the operator keeps their typed
  // reason and can retry after a transient failure.
  const handleVoidItem = async (): Promise<boolean> => {
    if (!itemPendingVoid) return false;

    setActionLoading(`void-${itemPendingVoid.id}`);
    try {
      // IMP-14: the backend may demand a manager PIN. withManagerPin
      // transparently opens the keypad on a `pin_required` / `pin_invalid`
      // 403, retries with `X-Manager-Pin`, and only then resolves.
      await withManagerPin(
        (pin) =>
          adjustBillItem(
            billId,
            itemPendingVoid.id,
            { void: true, reason: voidReason.trim() },
            { pin },
          ),
        {
          title: tString("editItems.voidPinTitle"),
          description: tString("editItems.voidPinDescription").replace(
            "{name}",
            itemPendingVoid.name,
          ),
        },
      );
      toast.success(
        tString("editItems.itemVoided").replace(
          "{item}",
          itemPendingVoid.name,
        ),
      );
      onItemsChanged();
      closeVoidModal();
      return true;
    } catch (error) {
      // Operator-cancelled PIN entry should fail silently — the modal
      // already disappeared and forcing a toast here would be noisy.
      const message =
        error instanceof Error ? error.message : "";
      if (message === "PIN entry cancelled") {
        return false;
      }
      console.error("Error voiding bill item:", error);
      toast.error(tString("editItems.itemVoidFailed"));
      return false;
    } finally {
      setActionLoading(null);
    }
  };

  const categoryButtonClass = (isActive: boolean) =>
    isActive
      ? "border border-ink-900 bg-ink-900 text-white"
      : "border border-warm-200 bg-white text-ink-700";

  return (
    <>
      <div className="space-y-5">
        <div className="grid gap-3 grid-cols-2 xl:grid-cols-4">
          <div className="rounded-2xl border border-warm-200 bg-white px-4 py-3 shadow-sm">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-ink-500">
              {tString("editItems.summary.lines")}
            </p>
            <p className="mt-2 text-2xl font-semibold text-ink-900">
              {editableCurrentItems.length}
            </p>
          </div>
          <div className="rounded-2xl border border-warm-200 bg-white px-4 py-3 shadow-sm">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-ink-500">
              {tString("editItems.summary.units")}
            </p>
            <p className="mt-2 text-2xl font-semibold text-ink-900">
              {totalCurrentQuantity}
            </p>
          </div>
          <div className="rounded-2xl border border-warm-200 bg-white px-4 py-3 shadow-sm">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-ink-500">
              {tString("editItems.summary.subtotal")}
            </p>
            <p className="mt-2 text-2xl font-semibold text-ink-900 whitespace-nowrap">
              {formatCurrency(liveSubtotal)}
            </p>
          </div>
          <div className="rounded-2xl border border-warm-200 bg-white px-4 py-3 shadow-sm">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-ink-500">
              {tString("editItems.summary.discounts")}
            </p>
            <p className="mt-2 text-2xl font-semibold text-emerald-700">
              {autoDiscounts.length}
            </p>
          </div>
        </div>

        <div className="grid gap-5 lg:grid-cols-[minmax(0,0.92fr)_minmax(0,1.08fr)]">
          <section className="rounded-[28px] border border-warm-200 bg-white shadow-sm">
            <div className="border-b border-warm-200 px-5 py-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <Receipt className="h-4 w-4 text-ink-500" />
                    <p className="text-sm font-semibold text-ink-900">
                      {tString("editItems.currentItemsTitle")}
                    </p>
                  </div>
                  <p className="mt-1 text-sm text-ink-500">
                    {tString("editItems.currentItemsSubtitle")}
                  </p>
                </div>
                <Chip size="sm" variant="flat">
                  {editableCurrentItems.length}
                </Chip>
              </div>
            </div>

            <div className="space-y-4 p-4">
              <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3">
                <div className="flex items-start gap-3">
                  <div className="mt-0.5 rounded-full bg-amber-100 p-2">
                    <AlertCircle className="h-4 w-4 text-amber-700" />
                  </div>
                  <div>
                    <p className="text-sm font-semibold text-amber-900">
                      {tString("editItems.manualChangesTitle")}
                    </p>
                    <p className="mt-1 text-sm text-amber-800">
                      {tString("editItems.manualChangesDescription")}
                    </p>
                  </div>
                </div>
              </div>

              <div className="space-y-3">
                {editableCurrentItems.length > 0 ? (
                  editableCurrentItems.map((item) => (
                    <div
                      key={item.id}
                      className="rounded-2xl border border-warm-200 bg-warm-50/70 p-4"
                    >
                      <div className="space-y-3">
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0 flex-1">
                            <p className="text-sm font-semibold text-ink-900">
                              {item.name}
                            </p>
                            <p className="mt-1 text-xs text-ink-500">
                              {item.quantity} × {formatCurrency(item.price || 0)}
                            </p>
                          </div>
                          <p className="shrink-0 text-sm font-semibold text-ink-900">
                            {formatCurrency(item.subtotal || 0)}
                          </p>
                        </div>

                        {item.options?.length ? (
                          <div className="flex flex-wrap gap-2">
                            {item.options.map((option, index) => (
                              <Chip
                                key={`${item.id}-option-${index}`}
                                size="sm"
                                variant="flat"
                                className="bg-white"
                              >
                                {option.name}
                                {option.price
                                  ? ` (${formatCurrency(option.price)})`
                                  : ""}
                              </Chip>
                            ))}
                          </div>
                        ) : null}

                        <div className="flex items-center justify-between gap-2">
                          <div className="flex items-center gap-2">
                            <Button
                              isIconOnly
                              size="sm"
                              variant="flat"
                              aria-label={tString("editItems.decreaseQuantityAria")}
                              isDisabled={item.quantity <= 1}
                              isLoading={isActionLoading(`qty-${item.id}`)}
                              onPress={() =>
                                void handleQuantityChange(item, item.quantity - 1)
                              }
                            >
                              <Minus className="h-4 w-4" />
                            </Button>
                            <div
                              className="min-w-[3rem] rounded-xl border border-warm-200 bg-white px-3 py-1 text-center text-sm font-semibold text-ink-900"
                              aria-live="polite"
                              aria-label={tString("editItems.quantityAria").replace(
                                "{quantity}",
                                String(item.quantity),
                              )}
                            >
                              {item.quantity}
                            </div>
                            <Button
                              isIconOnly
                              size="sm"
                              variant="flat"
                              aria-label={tString("editItems.increaseQuantityAria")}
                              isLoading={isActionLoading(`qty-${item.id}`)}
                              onPress={() =>
                                void handleQuantityChange(item, item.quantity + 1)
                              }
                            >
                              <Plus className="h-4 w-4" />
                            </Button>
                          </div>

                          <Button
                            size="sm"
                            color="danger"
                            variant="light"
                            isLoading={isActionLoading(`void-${item.id}`)}
                            onPress={() => handleVoidPrompt(item)}
                            startContent={
                              !isActionLoading(`void-${item.id}`) ? (
                                <Trash2 className="h-4 w-4" />
                              ) : undefined
                            }
                          >
                            {tString("editItems.voidItem")}
                          </Button>
                        </div>
                      </div>
                    </div>
                  ))
                ) : (
                  <div className="rounded-2xl border border-dashed border-warm-300 bg-warm-50 px-4 py-10 text-center">
                    <ChefHat className="mx-auto mb-3 h-9 w-9 text-ink-500" />
                    <p className="text-sm font-medium text-ink-700">
                      {tString("editItems.noCurrentItems")}
                    </p>
                    <p className="mt-1 text-sm text-ink-500">
                      {tString("editItems.noCurrentItemsDescription")}
                    </p>
                  </div>
                )}
              </div>

              {autoDiscounts.length > 0 ? (
                <div
                  className="rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-4"
                  data-testid="bill-auto-discounts"
                >
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-sm font-semibold text-emerald-900">
                      {tString("editItems.autoDiscounts")}
                    </p>
                    <Chip size="sm" color="success" variant="flat">
                      {formatCurrency(autoDiscountTotal)}
                    </Chip>
                  </div>
                  <p className="mt-2 text-xs text-emerald-800/80">
                    {tString("editItems.autoDiscountsHint")}
                  </p>
                  <div className="mt-3 space-y-2">
                    {autoDiscounts.map((discount) => {
                      const orderId = discount.order_id ?? discount.orderId;
                      return (
                        <div
                          key={discount.id}
                          className="flex flex-wrap items-center justify-between gap-3 text-sm text-emerald-900"
                          data-testid="bill-auto-discount-line"
                        >
                          <div className="min-w-0 flex-1">
                            <span className="block truncate font-medium">
                              {discount.name}
                            </span>
                            {orderId ? (
                              <span className="block text-xs text-emerald-800/80">
                                {tString("discountOrderAttribution").replace(
                                  "{orderId}",
                                  String(orderId),
                                )}
                              </span>
                            ) : null}
                          </div>
                          <div className="flex shrink-0 items-center gap-2">
                            <span className="font-medium tabular-nums">
                              {formatCurrency(discount.subtotal || 0)}
                            </span>
                            <Button
                              size="sm"
                              color="danger"
                              variant="light"
                              isLoading={isActionLoading(`void-${discount.id}`)}
                              onPress={() => handleVoidPrompt(discount)}
                              startContent={
                                !isActionLoading(`void-${discount.id}`) ? (
                                  <Trash2 className="h-4 w-4" />
                                ) : undefined
                              }
                              aria-label={`${tString("editItems.voidItem")}: ${discount.name}`}
                            >
                              {tString("editItems.voidItem")}
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : null}
            </div>
          </section>

          <section className="rounded-[28px] border border-warm-200 bg-white shadow-sm">
            <div className="space-y-4 border-b border-warm-200 px-5 py-4">
              <div className="flex flex-col gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <Plus className="h-4 w-4 text-ink-500" />
                    <p className="text-sm font-semibold text-ink-900">
                      {tString("editItems.addFromMenu")}
                    </p>
                  </div>
                  <p className="mt-1 text-sm text-ink-500">
                    {tString("editItems.menuSubtitle")}
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Chip size="sm" variant="flat">
                    {tString("editItems.menuResultsCount").replace(
                      "{count}",
                      String(filteredMenuItems.length),
                    )}
                  </Chip>
                  <Chip size="sm" variant="flat" color="success">
                    {tString("editItems.availableNow").replace(
                      "{count}",
                      String(filteredMenuItems.filter((item) => item.is_available).length),
                    )}
                  </Chip>
                </div>
              </div>

              <Input
                placeholder={tString("editItems.searchPlaceholder")}
                value={searchQuery}
                onValueChange={setSearchQuery}
                startContent={<Search className="h-4 w-4 text-ink-400" />}
                size="sm"
                classNames={{ inputWrapper: "bg-warm-50" }}
                isClearable
                onClear={() => setSearchQuery("")}
              />

              <div className="rounded-2xl border border-warm-200 bg-warm-50 px-4 py-3">
                <p className="text-sm text-ink-600">
                  {tString("editItems.browseHint")}
                </p>
              </div>

              <div className="overflow-x-auto pb-1">
                <div className="flex min-w-max gap-2">
                  <Button
                    size="sm"
                    variant="light"
                    className={categoryButtonClass(selectedCategory === "all")}
                    onPress={() => setSelectedCategory("all")}
                  >
                    {`${tString("editItems.allCategories")} · ${allMenuItems.length}`}
                  </Button>
                  {menuCategories.map((category) => (
                    <Button
                      key={category.name}
                      size="sm"
                      variant="light"
                      className={categoryButtonClass(
                        selectedCategory === category.name,
                      )}
                      onPress={() => setSelectedCategory(category.name)}
                    >
                      {`${category.name} · ${category.items.length}`}
                    </Button>
                  ))}
                </div>
              </div>
            </div>

            {menuLoading ? (
              <div className="flex justify-center py-10">
                <Spinner size="sm" />
              </div>
            ) : filteredMenuItems.length === 0 ? (
              <div className="px-4 py-10 text-center">
                <ChefHat className="mx-auto mb-3 h-8 w-8 text-ink-500" />
                <p className="text-sm font-medium text-ink-700">
                  {tString("editItems.noMenuItems")}
                </p>
                <p className="mt-1 text-sm text-ink-500">
                  {tString("editItems.menuEmptyDescription")}
                </p>
              </div>
            ) : (
              <div className="grid max-h-[34rem] gap-3 overflow-y-auto p-4 lg:grid-cols-2">
                {filteredMenuItems.map((item) => {
                  const itemKey = item.id || item.name;
                  const actionKey = `add-${itemKey}`;
                  const quantityOnBill =
                    billPresence.get(getLookupKey(item.id, item.name))?.quantity || 0;
                  const hasOptions = Boolean(item.options?.length);

                  return (
                    <div
                      key={itemKey}
                      className={`rounded-2xl border p-4 ${
                        item.is_available
                          ? "border-warm-200 bg-white"
                          : "border-warm-100 bg-warm-50 opacity-70"
                      }`}
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <p className="text-sm font-semibold text-ink-900">
                              {item.name}
                            </p>
                            <Chip size="sm" variant="flat">
                              {item.categoryName}
                            </Chip>
                            {quantityOnBill > 0 ? (
                              <Chip size="sm" color="primary" variant="flat">
                                {tString("editItems.alreadyOnBill").replace(
                                  "{count}",
                                  String(quantityOnBill),
                                )}
                              </Chip>
                            ) : null}
                            {hasOptions ? (
                              <Chip size="sm" color="secondary" variant="flat">
                                {tString("editItems.hasOptions").replace(
                                  "{count}",
                                  String(item.options?.length || 0),
                                )}
                              </Chip>
                            ) : null}
                            {!item.is_available ? (
                              <Chip size="sm" color="danger" variant="flat">
                                {tString("editItems.unavailable")}
                              </Chip>
                            ) : null}
                          </div>

                          <p className="mt-2 line-clamp-2 text-sm text-ink-500">
                            {item.description || tString("editItems.noDescription")}
                          </p>
                        </div>

                        <div className="shrink-0 text-right">
                          <p className="text-base font-semibold text-ink-900">
                            {formatCurrency(item.price)}
                          </p>
                        </div>
                      </div>

                      <div className="mt-4 flex flex-wrap gap-2">
                        <Button
                          size="sm"
                          color="primary"
                          variant="flat"
                          isLoading={actionLoading === actionKey}
                          isDisabled={!item.is_available}
                          onPress={() =>
                            hasOptions
                              ? setItemToCustomize(item)
                              : handleAddItem(item)
                          }
                          startContent={
                            !hasOptions && actionLoading !== actionKey ? (
                              <Plus className="h-4 w-4" />
                            ) : undefined
                          }
                        >
                          {hasOptions
                            ? tString("editItems.customizeItem")
                            : tString("editItems.quickAdd")}
                        </Button>

                        {!hasOptions ? (
                          <Button
                            size="sm"
                            variant="light"
                            isDisabled={!item.is_available}
                            onPress={() => setItemToCustomize(item)}
                          >
                            {tString("editItems.chooseQuantity")}
                          </Button>
                        ) : null}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </section>
        </div>
      </div>

      <Modal isOpen={itemPendingVoid !== null} onOpenChange={closeVoidModal}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <p className="text-lg font-semibold text-ink-900">
                  {tString("editItems.voidItemTitle")}
                </p>
                <p className="text-sm font-normal text-ink-500">
                  {tString("editItems.voidItemDescription").replace(
                    "{item}",
                    itemPendingVoid?.name || "",
                  )}
                </p>
              </ModalHeader>

              <ModalBody>
                <Textarea
                  label={tString("editItems.voidReasonLabel")}
                  placeholder={tString("editItems.voidReasonPlaceholder")}
                  value={voidReason}
                  onValueChange={setVoidReason}
                  minRows={3}
                  maxLength={240}
                />
                <p className="text-xs text-ink-500">
                  {tString("editItems.voidReasonHint")}
                </p>
              </ModalBody>

              <ModalFooter>
                <Button
                  variant="light"
                  onPress={() => {
                    closeVoidModal();
                    onClose();
                  }}
                >
                  {tString("editItems.voidItemCancel")}
                </Button>
                <Button
                  color="danger"
                  isDisabled={!voidReason.trim()}
                  isLoading={isActionLoading(
                    itemPendingVoid ? `void-${itemPendingVoid.id}` : "",
                  )}
                  onPress={async () => {
                    // Only close the editor when the void succeeded; on failure
                    // keep the reason modal open so the typed reason isn't lost.
                    const ok = await handleVoidItem();
                    if (ok) onClose();
                  }}
                >
                  {tString("editItems.voidItemConfirm")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {itemToCustomize ? (
        <ItemCustomizer
          isOpen={Boolean(itemToCustomize)}
          onClose={() => setItemToCustomize(null)}
          item={itemToCustomize}
          onAddToCart={(item, quantity, selectedOptions) =>
            void handleAddItem(item, quantity, selectedOptions)
          }
          allowSpecialRequests={false}
          submitLabel={tString("editItems.addToBill")}
          currency={currency}
        />
      ) : null}
    </>
  );
});
