"use client";

import React, { useState, useCallback, useMemo, memo } from "react";
import NextImage from "next/image";
import { canOptimizeImageSrc } from "@/config/imageOrigins";
import { Button, Chip, Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from "@nextui-org/react";
import { Plus, Minus, Check, ImageOff } from "lucide-react";
import { MenuCategory, MenuItem, Business, Offer } from "../../api/business";
import { BillWithItemsResponse } from "../../api/bills";
import { ImageCarousel } from "./ImageCarousel";
import { EmptyState } from "@/components/ui/EmptyState";
import { SearchX, Utensils } from "lucide-react";
import type { Orderability } from "@/api/orders";

// Memoized ImageCarousel wrapper to prevent unnecessary re-renders
const MemoizedImageCarousel = memo(({
  images,
  itemName,
  size
}: {
  images: string[];
  itemName: string;
  size: "sm" | "md" | "lg";
}) => {
  return (
    <ImageCarousel
      images={images}
      itemName={itemName}
      size={size}
    />
  );
}, (prevProps, nextProps) => {
  // Custom comparison: only re-render if images content actually changed
  if (prevProps.images.length !== nextProps.images.length) return false;
  return prevProps.images.every((img, idx) => img === nextProps.images[idx]);
});
MemoizedImageCarousel.displayName = "MemoizedImageCarousel";
import { CurrencyPrice } from "../common/CurrencyConverter";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import { normalizeGuestLocale } from "@/utils/guestCurrencyFormatter";
import { MobileMenuItem } from "./MobileMenuItem";
import {
  isInventoryEightySix,
  isVenueClosedState,
  shouldShowItemInventoryWarningBadge,
  shouldShowItemPromotionOffers,
  shouldShowItemUnavailableBadge,
} from "./menuItemAvailability";
import { isBusinessClosedFromOrderability } from "@/lib/guestBusinessClosed";
import { ALLERGENS, DIETARY_TAGS } from "../../constants/menu-tags";
import { AlertCircle } from "lucide-react";

interface GuestMenuViewsProps {
  categories: MenuCategory[];
  business: Business;
  tableCode: string;
  currentBill: BillWithItemsResponse | null;
  selectedLanguage?: string;
  defaultCurrency?: string;
  displayCurrency?: string;
  viewMode: "detailed" | "compact" | "grid" | "category-tabs";
  activeCategory: number;
  onAddToCart: (
    itemName: string,
    price: number,
    quantity?: number,
    specialRequests?: string,
    addOns?: Array<{ id?: string; name: string; price: number }>,
    metadata?: {
      itemType?: "menu_item" | "bundle";
      menuItemId?: string;
      bundleId?: number;
    },
  ) => boolean | void;
  onItemClick: (item: MenuItem) => void;
  isOrderingEnabled?: boolean;
  /** True when the parent page has an active search query or filter chip. Drives
   *  "No items found + Clear" vs "Menu coming soon" in the empty-state. */
  hasActiveFilters?: boolean;
  /** Reset the parent page's search query + filter set. Wired to the empty-state
   *  Clear action. */
  onClearFilters?: () => void;
  itemOrderability?: Record<string, Orderability>;
}

type DisplayMenuItem = MenuItem & {
  item_type?: "menu_item" | "bundle";
  bundle_id?: number;
  menu_item_id?: string;
  promotion_offers?: Offer[];
};

export const GuestMenuViews: React.FC<GuestMenuViewsProps> = ({
  categories: sourceCategories,
  business,
  tableCode,
  currentBill,
  selectedLanguage,
  defaultCurrency = "USD",
  displayCurrency = "USD",
  viewMode,
  isOrderingEnabled = true,
  activeCategory,
  onAddToCart,
  onItemClick,
  hasActiveFilters = false,
  onClearFilters,
  itemOrderability = {},
}) => {
  const { t, currentLanguage } = useGuestTranslation();
  const moneyLocale = normalizeGuestLocale(currentLanguage);

  const venueClosed = isBusinessClosedFromOrderability(itemOrderability);
  const categories = useMemo(
    () => sourceCategories.map((category) => ({
      ...category,
      items: category.items.map((item) => {
        const decision = item.id ? itemOrderability[item.id] : undefined;
        const inventoryOut = item.inventory_status === "out_of_stock";
        if (!decision && !inventoryOut) return item;
        return {
          ...item,
          is_available: decision
            ? decision.orderable && !inventoryOut
            : item.is_available !== false && !inventoryOut,
          orderability_state:
            decision?.state && decision.state !== "available"
              ? decision.state
              : inventoryOut
                ? "inventory_out"
                : decision?.state,
        };
      }),
    })),
    [itemOrderability, sourceCategories],
  );

  const showUnavailableForItem = (item: DisplayMenuItem) =>
    shouldShowItemUnavailableBadge({
      orderabilityState: item.orderability_state,
      venueClosed:
        venueClosed || isVenueClosedState(item.orderability_state),
      isAvailable: item.is_available,
      inventoryStatus: item.inventory_status,
    });

  const unavailableLabelForItem = (item: DisplayMenuItem) =>
    isInventoryEightySix({
      orderabilityState: item.orderability_state,
      inventoryStatus: item.inventory_status,
    })
      ? t("menu.outOfStock")
      : t("menu.unavailable");

  const inventoryWarningBadge = (item: DisplayMenuItem) => {
    if (
      !shouldShowItemInventoryWarningBadge({
        orderabilityState: item.orderability_state,
        venueClosed:
          venueClosed || isVenueClosedState(item.orderability_state),
      })
    ) {
      return null;
    }
    if (item.orderability_state === "inventory_warning") {
      return (
        <span className="inline-flex rounded-full bg-amber-100 px-2 py-1 text-[10px] font-semibold text-amber-800">
          {t("menu.lowStock")}
        </span>
      );
    }
    return null;
  };

  // State management from original component
  const [selectedItem, setSelectedItem] = useState<DisplayMenuItem | null>(null);
  const [addedItems, setAddedItems] = useState<Set<string>>(new Set());
  const [addedQuantities, setAddedQuantities] = useState<Record<string, number>>(
    {},
  );
  const [actionStatus, setActionStatus] = useState("");
  const [itemQuantities, setItemQuantities] = useState<Record<string, number>>(
    {},
  );
  const [animatingItems, setAnimatingItems] = useState<Set<string>>(new Set());
  const [selectedOptions, setSelectedOptions] = useState<Set<string>>(
    new Set(),
  );
  const {
    isOpen: isItemModalOpen,
    onOpen: onItemModalOpen,
    onClose: onItemModalClose,
  } = useDisclosure();
  const getItemImages = useCallback((item: DisplayMenuItem): string[] => {
    const images: string[] = [];

    // Add main image first if it exists
    if (item.image && item.image.trim() !== "") {
      images.push(item.image.trim());
    }

    // Add images from the array, filtering out duplicates
    if (item.images && Array.isArray(item.images)) {
      item.images.forEach((img) => {
        if (img && img.trim() !== "" && !images.includes(img.trim())) {
          images.push(img.trim());
        }
      });
    }

    return images;
  }, []);

  // All the functionality methods from original component
  const toggleOption = (optionId: string) => {
    setSelectedOptions((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(optionId)) {
        newSet.delete(optionId);
      } else {
        newSet.add(optionId);
      }
      return newSet;
    });
  };

  const calculateItemTotalPrice = (item: DisplayMenuItem) => {
    let totalPrice = item.price || 0;

    if (item.options) {
      item.options.forEach((option, index) => {
        const optionId = getOptionStateKey(item, index);
        if (selectedOptions.has(optionId)) {
          totalPrice += option.price_change || 0;
        }
      });
    }

    return totalPrice;
  };

  const getSelectedOptionsForItem = (item: DisplayMenuItem) => {
    if (!item.options) return [];

    return item.options.filter((option, index) => {
      const optionId = getOptionStateKey(item, index);
      return selectedOptions.has(optionId);
    });
  };

  // Clears ALL transient modal selections (chosen add-on options + per-item
  // quantity overrides). These are modal-scoped: without clearing them on every
  // open/close/add, a customization the guest made and then abandoned (closed the
  // modal without adding) would silently leak into a later card quick-add of the
  // same item — e.g. "+Bacon" or quantity 3 riding along on a plain "+" tap.
  const resetModalState = () => {
    setSelectedOptions(new Set());
    setItemQuantities({});
  };

  const handleItemClick = (item: DisplayMenuItem) => {
    setSelectedItem(item);
    resetModalState();
    onItemModalOpen();
    // PG-13 #2: parent may pass a no-op today, but the prop is part of the
    // public contract — always invoke so wiring is not dead.
    onItemClick(item);
  };

  // Closing the modal (backdrop, ESC, or the Close button) must discard the
  // in-progress customization, so it never rides along on a later quick-add.
  const handleItemModalClose = () => {
    resetModalState();
    onItemModalClose();
  };

  const isBundleItem = (item: DisplayMenuItem) => {
    if (item.item_type === "bundle") return true;
    if (item.bundle_id) return true;
    if (typeof item.id === "string" && item.id.startsWith("bundle:")) return true;
    return false;
  };

  const getBundleId = (item: DisplayMenuItem): number | undefined => {
    if (item.bundle_id) return item.bundle_id;
    if (typeof item.id === "string" && item.id.startsWith("bundle:")) {
      const parsed = Number(item.id.split(":")[1]);
      if (!Number.isNaN(parsed)) return parsed;
    }
    return undefined;
  };

  const getItemStateKey = useCallback((item: DisplayMenuItem) => {
    const itemType = isBundleItem(item) ? "bundle" : (item.item_type || "menu_item");
    const bundleId = getBundleId(item);
    const stableId =
      item.menu_item_id ||
      item.id ||
      (bundleId ? `bundle:${bundleId}` : "");
    const fallbackIdentity = [
      item.name.trim().toLowerCase(),
      (item.description || "").trim().toLowerCase(),
      item.price || 0,
    ].join("|");

    return `${itemType}:${stableId || fallbackIdentity}`;
  }, []);

  const getOptionStateKey = useCallback(
    (item: DisplayMenuItem, index: number) =>
      `${getItemStateKey(item)}:option:${index}`,
    [getItemStateKey],
  );

  const getPromotionOffers = (item: DisplayMenuItem): Offer[] => {
    const itemClosed =
      venueClosed || isVenueClosedState(item.orderability_state);
    if (!shouldShowItemPromotionOffers(itemClosed)) return [];
    if (!Array.isArray(item.promotion_offers)) return [];
    return item.promotion_offers;
  };

  const renderPromotionBadges = (item: DisplayMenuItem) => {
    const offers = getPromotionOffers(item);
    if (offers.length === 0) return null;
    return (
      <div className="flex flex-wrap gap-1 mt-1.5">
        {offers.slice(0, 2).map((offer) => (
          <Chip
            key={offer.id || `${offer.name}-${offer.target_id || "all"}`}
            size="sm"
            variant="flat"
            className="text-[10px] h-5 bg-amber-50 text-amber-700 border border-amber-200"
          >
            {offer.name}
          </Chip>
        ))}
        {offers.length > 2 && (
          <Chip size="sm" variant="flat" className="text-[10px] h-5 bg-amber-50 text-amber-700 border border-amber-200">
            +{offers.length - 2}
          </Chip>
        )}
      </div>
    );
  };

  const getItemQuantity = (itemKey: string) => {
    return itemQuantities[itemKey] || 1;
  };

  const setItemQuantity = (itemKey: string, quantity: number) => {
    setItemQuantities((prev) => ({
      ...prev,
      [itemKey]: Math.max(1, quantity),
    }));
  };


  const handleAddToCart = (item: DisplayMenuItem, quantity?: number) => {
    const itemKey = getItemStateKey(item);
    const finalQuantity = quantity || getItemQuantity(itemKey);
    const selectedItemOptions = isBundleItem(item) ? [] : getSelectedOptionsForItem(item);

    const basePrice = item.price || 0;

    const addOns = selectedItemOptions.map((option) => ({
      id: option.id,
      name: option.name,
      price: option.price_change || 0,
    }));

    const metadata = isBundleItem(item)
      ? {
        itemType: "bundle" as const,
        bundleId: getBundleId(item),
        menuItemId: typeof item.id === "string" ? item.id : undefined,
      }
      : {
        itemType: "menu_item" as const,
        menuItemId: item.menu_item_id || item.id,
      };

    const applied = onAddToCart(
      item.name,
      basePrice,
      finalQuantity,
      "",
      addOns,
      metadata,
    );
    if (applied === false) {
      return;
    }

    setAddedQuantities((prev) => ({ ...prev, [itemKey]: finalQuantity }));
    setActionStatus(
      t("menu.itemAddedNamed", {
        quantity: finalQuantity,
        name: item.name,
      }),
    );
    setAnimatingItems((prev) => new Set(Array.from(prev).concat([itemKey])));
    setAddedItems((prev) => new Set(Array.from(prev).concat([itemKey])));

    resetModalState();
    onItemModalClose();

    setTimeout(() => {
      setAnimatingItems((prev) => {
        const newSet = new Set(prev);
        newSet.delete(itemKey);
        return newSet;
      });
      setAddedItems((prev) => {
        const newSet = new Set(prev);
        newSet.delete(itemKey);
        return newSet;
      });
    }, 2000);
  };

  const isItemAdded = (itemKey: string) => {
    return addedItems.has(itemKey);
  };

  const isItemAnimating = (itemKey: string) => {
    return animatingItems.has(itemKey);
  };

  // Detailed View (Current implementation)
  const DetailedView = () => (
    <div className="space-y-16">
      {categories.map((category, categoryIndex) => (
        <div
          key={categoryIndex}
          id={`category-${categoryIndex}`}
          className="scroll-mt-32"
        >
          <div className="mb-8">
            <div className="flex items-center gap-4 mb-2">
              <h2 className="text-2xl font-medium text-ink-900 tracking-tight">
                {category.name}
              </h2>
              <div className="flex-1 h-px bg-gradient-to-r from-gray-300 to-transparent"></div>
            </div>
            {category.description && (
              <p className="text-ink-600 font-light text-sm leading-relaxed max-w-3xl">
                {category.description}
              </p>
            )}
          </div>

          {/* Mobile View (< 640px) */}
          <div className="sm:hidden space-y-3">
            {category.items?.map((item, itemIndex) => {
              const itemKey = getItemStateKey(item as DisplayMenuItem);
              return (
                <MobileMenuItem
                  key={`${itemKey}:${itemIndex}`}
                  item={item}
                  isAnimating={isItemAnimating(itemKey)}
                  isAdded={isItemAdded(itemKey)}
                  quantity={addedQuantities[itemKey] ?? getItemQuantity(itemKey)}
                  defaultCurrency={defaultCurrency}
                  displayCurrency={displayCurrency}
                  locale={moneyLocale}
                  onItemClick={handleItemClick}
                  onAddToCart={() => handleAddToCart(item)}
                  isOrderingEnabled={isOrderingEnabled}
                  venueClosed={venueClosed}
                  t={t}
                />
              );
            })}
          </div>

          {/* Desktop View (>= 640px) */}
          <div className="hidden sm:grid grid-cols-1 gap-6">
            {category.items?.map((item, itemIndex) => {
              const itemKey = getItemStateKey(item as DisplayMenuItem);
              const itemImages = getItemImages(item);
              return (
                <div
                  key={`${itemKey}:${itemIndex}`}
                  className={`group bg-white border border-warm-200 rounded-2xl p-6 hover:shadow-xl hover:border-warm-300 transition-all duration-300 hover:-translate-y-1 relative ${!item.is_available ? "opacity-60" : ""}`}
                >
                  {showUnavailableForItem(item as DisplayMenuItem) && (
                    <div className="absolute top-4 right-4 bg-rose-600 text-white text-xs px-2 py-1 rounded-full font-medium">
                      {unavailableLabelForItem(item as DisplayMenuItem)}
                    </div>
                  )}

                  {/* Success animation overlay */}
                  {isItemAnimating(itemKey) && (
                    <div className="absolute inset-0 bg-emerald-500/10 rounded-2xl flex items-center justify-center z-10">
                      <div className="bg-emerald-800 text-white px-4 py-2 rounded-full flex items-center gap-2">
                        <Check className="w-4 h-4" />
                        <span className="text-sm font-medium">
                          {t("menu.itemAdded", {
                            quantity: addedQuantities[itemKey] ?? 1,
                          })}
                        </span>
                      </div>
                    </div>
                  )}

                  <div className="flex flex-col sm:flex-row gap-4 sm:gap-6">
                    <div className="relative">
                      {itemImages.length > 0 ? (
                        <MemoizedImageCarousel
                          images={itemImages}
                          itemName={item.name}
                          size="md"
                        />
                      ) : (
                        <div className="w-full sm:w-28 h-48 sm:h-28 bg-warm-50 rounded-xl flex items-center justify-center border border-warm-200/70">
                          <ImageOff className="w-8 h-8 text-ink-500" />
                        </div>
                      )}
                      {itemImages.length > 1 && (
                        <div className="absolute bottom-2 right-2 bg-black/60 backdrop-blur-sm text-white text-xs px-1.5 py-0.5 rounded-full">
                          {getItemImages(item).length}
                        </div>
                      )}
                    </div>
                    <div className="flex-1">
                      <div className="flex justify-between items-start mb-4">
                        <div className="flex-1">
                          <div className="flex items-center gap-2 sm:gap-3 mb-2">
                            {/* PG-13 #1: named open control (title button) — parity with mobile /b open affordance */}
                            <h3 className="min-w-0 text-lg sm:text-xl font-semibold text-ink-900 tracking-wide">
                              <button
                                type="button"
                                aria-label={t("accessibility.openItem", { name: item.name })}
                                // RSP-1: see MobileMenuItem — `py-1 -my-1` lifts the
                                // tap target over the 24px WCAG 2.5.8 AA floor with
                                // zero layout displacement. Applied to all four view
                                // modes here plus the mobile card, because the
                                // surrounding card is an inert <div> in every one.
                                className="block max-w-full truncate text-left py-1 -my-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand rounded-sm"
                                onClick={() => handleItemClick(item as DisplayMenuItem)}
                              >
                                {item.name}
                              </button>
                            </h3>
                            {isBundleItem(item as DisplayMenuItem) && (
                              <Chip
                                size="sm"
                                variant="flat"
                                className="text-[10px] h-5 px-2 bg-brand/10 text-brand-dark border border-brand/20"
                              >
                                {t("menu.bundles.badge")}
                              </Chip>
                            )}
                            {inventoryWarningBadge(item as DisplayMenuItem)}
                          </div>
                          {item.description && (
                            <p dir="auto" className="text-ink-600 font-light leading-relaxed mb-3">
                              {item.description}
                            </p>
                          )}
                          {renderPromotionBadges(item as DisplayMenuItem)}

                          {/* Allergens, Dietary Tags, and Options Indicator */}
                          <div className="flex flex-wrap gap-2 mt-3">
                            {item.allergens && item.allergens.length > 0 && (
                              <div className="flex flex-wrap gap-1">
                                {item.allergens.map((allergenId, idx) => {
                                  const allergen = ALLERGENS.find(a => a.id === allergenId);
                                  const label = allergen ? t("menu." + allergen.name) : allergenId;
                                  return (
                                    <div
                                      key={idx}
                                      className="flex items-center gap-1 bg-rose-50 px-1.5 py-0.5 rounded border border-rose-200/70"
                                      title={label}
                                    >
                                      {allergen?.icon ? (
                                        <NextImage src={allergen.icon} alt="" aria-hidden="true" width={14} height={14} className="w-3.5 h-3.5" />
                                      ) : (
                                        <AlertCircle className="w-3 h-3 text-rose-700" aria-hidden="true" />
                                      )}
                                      <span className="text-[10px] font-medium text-rose-700 leading-none">{label}</span>
                                    </div>
                                  );
                                })}
                              </div>
                            )}
                            {item.dietary_tags &&
                              item.dietary_tags.length > 0 && (
                                <div className="flex flex-wrap gap-1">
                                  {item.dietary_tags
                                    .slice(0, 3)
                                    .map((tagId, tagIndex) => {
                                      const tag = DIETARY_TAGS.find(t => t.id === tagId);
                                      return (
                                        <Chip
                                          key={tagIndex}
                                          size="sm"
                                          variant="flat"
                                          color="success"
                                          className="text-[10px] h-5 px-1.5"
                                        >
                                          {tag ? t("menu." + tag.name) : tagId}
                                        </Chip>
                                      );
                                    })}
                                  {item.dietary_tags.length > 3 && (
                                    <Chip
                                      size="sm"
                                      variant="flat"
                                      color="default"
                                      className="text-[10px] h-5 px-1.5"
                                    >
                                      +{item.dietary_tags.length - 3}
                                    </Chip>
                                  )}
                                </div>
                              )}
                            {item.options && item.options.length > 0 && (
                              <Chip
                                size="sm"
                                variant="flat"
                                className="text-xs h-5 px-2 bg-brand/10 text-brand-dark border border-brand/20"
                              >
                                {t("menu.customizable")} ({item.options.length})
                              </Chip>
                            )}
                          </div>
                        </div>
                        <div className="text-right ml-6">
                          <p className="text-2xl font-semibold text-ink-900 tracking-wide">
                            <CurrencyPrice
                              amount={item.price || 0}
                              fromCurrency={defaultCurrency}
                              displayCurrency={displayCurrency}
                              locale={moneyLocale}
                            />
                          </p>
                        </div>
                      </div>

                      {/* Add to Bill Button - Only show if ordering is enabled */}
                      {isOrderingEnabled && (
                        <div className="flex justify-end">
                          <Button
                            color={isItemAdded(itemKey) ? "success" : "default"}
                            variant={
                              isItemAdded(itemKey) ? "solid" : "bordered"
                            }
                            size="lg"
                            className={`font-medium tracking-wide transition-all duration-200 ${isItemAdded(itemKey)
                              ? "bg-emerald-600 text-white hover:bg-emerald-700"
                              : "bg-brand text-white hover:bg-brand-dark hover:scale-110"
                              }`}
                            startContent={
                              isItemAdded(itemKey) ? (
                                <Check className="w-5 h-5" />
                              ) : (
                                <Plus className="w-5 h-5" />
                              )
                            }
                            aria-label={t("accessibility.addItemToCart", {
                              name: item.name,
                            })}
                            onClick={(e) => {
                              e.stopPropagation();
                              // Stepper-less "+" quick-add always adds one, not
                              // any stale per-item quantity left over from an
                              // inline stepper / modal in another view (the
                              // button label promises "1 added").
                              handleAddToCart(item, 1);
                            }}
                            isDisabled={!item.is_available}
                          >
                            <span className="sr-only">{item.name} </span>
                            {isItemAdded(itemKey)
                              ? t("menu.itemAdded", {
                                  quantity: addedQuantities[itemKey] ?? 1,
                                })
                              : t("menu.addToCart")}
                          </Button>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  );

  // Compact View
  const CompactView = () => (
    <div className="space-y-12">
      {categories.map((category, categoryIndex) => (
        <div
          key={categoryIndex}
          id={`category-${categoryIndex}`}
          className="scroll-mt-32"
        >
          <div className="mb-6">
            <h2 className="text-xl font-medium text-ink-900 tracking-tight mb-1">
              {category.name}
            </h2>
            <div className="flex-1 h-px bg-gradient-to-r from-gray-300 to-transparent"></div>
          </div>

          <div className="space-y-4">
            {category.items?.map((item, itemIndex) => {
              const itemKey = getItemStateKey(item as DisplayMenuItem);
              const itemImages = getItemImages(item);
              return (
                <div
                  key={`${itemKey}:${itemIndex}`}
                  className={`bg-white border border-warm-200 rounded-xl p-4 shadow-sm hover:-translate-y-1 transition-all duration-200 relative overflow-hidden ${isItemAnimating(itemKey) ? "ring-2 ring-emerald-500" : ""
                    }`}
                >
                  {/* Success Animation Overlay */}
                  {isItemAnimating(itemKey) && (
                    <div className="absolute inset-0 bg-emerald-500/10 flex items-center justify-center z-10">
                      <div className="bg-emerald-800 text-white px-4 py-2 rounded-full flex items-center gap-2">
                        <Check className="w-4 h-4" />
                        <span className="text-sm font-medium">{t("menu.added")}</span>
                      </div>
                    </div>
                  )}

                  <div className="flex gap-4">
                    {itemImages.length > 0 ? (
                      <div className="flex-shrink-0">
                        <MemoizedImageCarousel
                          images={itemImages}
                          itemName={item.name}
                          size="sm"
                        />
                      </div>
                    ) : (
                      <div className="flex-shrink-0">
                        <div className="w-20 h-20 bg-warm-50 rounded-xl flex items-center justify-center border border-warm-200/70">
                          <ImageOff className="w-5 h-5 text-ink-500" />
                        </div>
                      </div>
                    )}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-start justify-between">
                        <div className="flex-1">
                          <div className="flex items-center gap-2">
                            <h3 className="min-w-0 flex-1">
                              <button
                                type="button"
                                aria-label={t("accessibility.openItem", { name: item.name })}
                                className="block max-w-full truncate text-left font-light text-ink-900 tracking-wide py-1 -my-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  handleItemClick(item);
                                }}
                              >
                                {item.name}
                              </button>
                            </h3>
                            {isBundleItem(item as DisplayMenuItem) && (
                              <Chip
                                size="sm"
                                variant="flat"
                                className="text-[10px] h-5 px-2 bg-brand/10 text-brand-dark border border-brand/20"
                              >
                                {t("menu.bundles.badge")}
                              </Chip>
                            )}
                            {inventoryWarningBadge(item as DisplayMenuItem)}
                          </div>
                          {item.description && (
                            <p className="text-sm text-ink-600 line-clamp-2 mt-1 font-light">
                              {item.description}
                            </p>
                          )}
                          {renderPromotionBadges(item as DisplayMenuItem)}
                          {/* Allergens, Dietary Tags, and Options Indicator */}
                          <div className="flex flex-wrap gap-2 mt-2">
                            {item.allergens && item.allergens.length > 0 && (
                              <div className="flex flex-wrap gap-1">
                                {item.allergens.map((allergenId, idx) => {
                                  const allergen = ALLERGENS.find(a => a.id === allergenId);
                                  const label = allergen ? t("menu." + allergen.name) : allergenId;
                                  return (
                                    <div
                                      key={idx}
                                      className="flex items-center gap-1 bg-rose-50 px-1.5 py-0.5 rounded border border-rose-200/70"
                                      title={label}
                                    >
                                      {allergen?.icon ? (
                                        <NextImage src={allergen.icon} alt="" aria-hidden="true" width={12} height={12} className="w-3 h-3" />
                                      ) : (
                                        <AlertCircle className="w-2.5 h-2.5 text-rose-700" aria-hidden="true" />
                                      )}
                                      <span className="text-[10px] font-medium text-rose-700 leading-none">{label}</span>
                                    </div>
                                  );
                                })}
                              </div>
                            )}
                            {item.dietary_tags &&
                              item.dietary_tags.length > 0 && (
                                <>
                                  {item.dietary_tags
                                    .slice(0, 2)
                                    .map((tagId, index) => {
                                      const tag = DIETARY_TAGS.find(t => t.id === tagId);
                                      return (
                                        <Chip
                                          key={index}
                                          size="sm"
                                          variant="flat"
                                          color="success"
                                          className="text-[10px] h-5 px-1.5"
                                        >
                                          {tag ? t("menu." + tag.name) : tagId}
                                        </Chip>
                                      );
                                    })}
                                  {item.dietary_tags.length > 2 && (
                                    <Chip
                                      size="sm"
                                      variant="flat"
                                      color="default"
                                      className="text-[10px] h-5 px-1.5"
                                    >
                                      +{item.dietary_tags.length - 2}
                                    </Chip>
                                  )}
                                </>
                              )}
                            {item.options && item.options.length > 0 && (
                              <Chip
                                size="sm"
                                variant="flat"
                                className="text-xs bg-brand/10 text-brand-dark border border-brand/20"
                              >
                                {t("menu.customizable")} ({item.options.length})
                              </Chip>
                            )}
                          </div>
                        </div>
                        <div className="text-right ml-4">
                          <div className="text-lg font-semibold text-ink-900">
                            <CurrencyPrice
                              amount={item.price}
                              fromCurrency={defaultCurrency}
                              displayCurrency={displayCurrency}
                              locale={moneyLocale}
                            />
                          </div>
                          {showUnavailableForItem(item as DisplayMenuItem) && (
                            <Chip
                              size="sm"
                              color="danger"
                              variant="flat"
                              className="mt-1"
                            >
                              {unavailableLabelForItem(item as DisplayMenuItem)}
                            </Chip>
                          )}
                          {/* Quick Add Button - Only show if ordering is enabled */}
                          {isOrderingEnabled && (
                            <Button
                              size="sm"
                              aria-label={t("accessibility.addItemToOrder", {
                                name: item.name,
                              })}
                              className="mt-2 bg-brand text-white hover:bg-brand-dark hover:scale-110 transition-transform"
                              isIconOnly
                              onClick={(e) => {
                                e.stopPropagation();
                                // Stepper-less "+" quick-add always adds one, not
                              // any stale per-item quantity left over from an
                              // inline stepper / modal in another view (the
                              // button label promises "1 added").
                              handleAddToCart(item, 1);
                              }}
                              isDisabled={!item.is_available}
                            >
                              <Plus className="w-4 h-4" />
                            </Button>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  );

  // Grid View
  const GridView = () => (
    <div className="space-y-12">
      {categories.map((category, categoryIndex) => (
        <div
          key={categoryIndex}
          id={`category-${categoryIndex}`}
          className="scroll-mt-32"
        >
          <div className="mb-6">
            <h2 className="text-xl font-medium text-ink-900 tracking-tight mb-1">
              {category.name}
            </h2>
            {category.description && (
              <p className="text-ink-500 text-sm leading-relaxed">
                {category.description}
              </p>
            )}
          </div>

          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3 sm:gap-4">
            {category.items?.map((item, itemIndex) => {
              const itemKey = getItemStateKey(item as DisplayMenuItem);
              const coverSrc = getItemImages(item)[0];
              return (
                <div
                key={`${itemKey}:${itemIndex}`}
                className={`group bg-white border border-warm-200 rounded-xl p-4 hover:shadow-xl hover:border-warm-300 hover:-translate-y-1 transition-all duration-300 relative overflow-hidden ${!item.is_available ? "opacity-60" : ""
                  } ${isItemAnimating(itemKey) ? "ring-2 ring-emerald-500" : ""}`}
              >
                {/* Success Animation Overlay */}
                {isItemAnimating(itemKey) && (
                  <div className="absolute inset-0 bg-emerald-500/10 flex items-center justify-center z-10">
                    <div className="bg-emerald-600 text-white px-3 py-1 rounded-full flex items-center gap-2 motion-safe:animate-pulse">
                      <Check className="w-3 h-3" />
                      <span className="text-xs font-medium">{t("menu.added")}</span>
                    </div>
                  </div>
                )}

                {showUnavailableForItem(item as DisplayMenuItem) && (
                  <div className="absolute top-2 right-2 bg-rose-600 text-white text-xs px-1.5 py-0.5 rounded-full font-medium">
                    {unavailableLabelForItem(item as DisplayMenuItem)}
                  </div>
                )}
                {item.is_available && inventoryWarningBadge(item as DisplayMenuItem)}

                <div className="relative aspect-square rounded-lg overflow-hidden bg-warm-100 mb-3">
                  {coverSrc ? (
                    <NextImage
                      src={coverSrc}
                      alt={item.name}
                      fill
                      sizes="(min-width: 1024px) 25vw, (min-width: 640px) 33vw, 50vw"
                      unoptimized={!canOptimizeImageSrc(coverSrc)}
                      className="w-full h-full object-cover"
                    />
                  ) : (
                    <div className="w-full h-full bg-warm-100 flex items-center justify-center">
                      <ImageOff className="w-8 h-8 text-ink-400" />
                    </div>
                  )}
                </div>

                <div className="space-y-2">
                  <div>
                    <h3>
                      <button
                        type="button"
                        aria-label={t("accessibility.openItem", { name: item.name })}
                        className="block text-left text-sm font-light text-ink-900 line-clamp-2 tracking-wide py-1 -my-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                        onClick={(event) => {
                          event.stopPropagation();
                          handleItemClick(item);
                        }}
                      >
                        {item.name}
                      </button>
                    </h3>
                    {isBundleItem(item as DisplayMenuItem) && (
                      <Chip
                        size="sm"
                        variant="flat"
                        className="text-[10px] h-5 px-2 bg-brand/10 text-brand-dark border border-brand/20 mt-1"
                      >
                        {t("menu.bundles.badge")}
                      </Chip>
                    )}
                    {renderPromotionBadges(item as DisplayMenuItem)}
                  </div>
                  <p className="text-sm font-semibold text-ink-900">
                    <CurrencyPrice
                      amount={item.price || 0}
                      fromCurrency={defaultCurrency}
                      displayCurrency={displayCurrency}
                      locale={moneyLocale}
                    />
                  </p>

                  {/* Dietary Tags, Allergens, and Options Indicator */}
                  <div className="flex flex-wrap gap-1">
                    {item.allergens && item.allergens.length > 0 && (
                      <div className="flex flex-wrap gap-1">
                        {item.allergens.slice(0, 3).map((allergenId, idx) => {
                          const allergen = ALLERGENS.find(a => a.id === allergenId);
                          const label = allergen ? t("menu." + allergen.name) : allergenId;
                          return (
                            <div
                              key={idx}
                              className="flex items-center gap-1 bg-rose-50 px-1.5 py-0.5 rounded border border-rose-200/70"
                              title={label}
                            >
                              {allergen?.icon ? (
                                <NextImage src={allergen.icon} alt="" aria-hidden="true" width={12} height={12} className="w-3 h-3" />
                              ) : (
                                <AlertCircle className="w-2.5 h-2.5 text-rose-700" aria-hidden="true" />
                              )}
                              <span className="text-[10px] font-medium text-rose-700 leading-none">{label}</span>
                            </div>
                          );
                        })}
                        {item.allergens.length > 3 && (
                          <div className="flex items-center bg-rose-50 px-1.5 py-0.5 rounded border border-rose-200/70">
                            <span className="text-[10px] font-medium text-rose-700 leading-none">
                              {t("menu.moreAllergens", { count: item.allergens.length - 3 })}
                            </span>
                          </div>
                        )}
                      </div>
                    )}
                    {item.dietary_tags && item.dietary_tags.length > 0 && (
                      <>
                        {item.dietary_tags
                          .slice(0, 2)
                          .map((tagId, index) => {
                            const tag = DIETARY_TAGS.find(t => t.id === tagId);
                            return (
                              <Chip
                                key={index}
                                size="sm"
                                variant="flat"
                                color="success"
                                className="text-[10px] h-5 px-1.5"
                              >
                                {tag ? t("menu." + tag.name) : tagId}
                              </Chip>
                            );
                          })}
                      </>
                    )}
                    {item.options && item.options.length > 0 && (
                      <Chip
                        size="sm"
                        variant="flat"
                        className="text-xs bg-brand/10 text-brand-dark border border-brand/20"
                      >
                        {t("menu.customizable")} ({item.options.length})
                      </Chip>
                    )}
                  </div>

                  {isOrderingEnabled && (
                    <Button
                      className="w-full bg-brand text-white hover:bg-brand-dark hover:scale-105 transition-transform"
                      size="sm"
                      startContent={<Plus className="w-3 h-3" />}
                      aria-label={t("accessibility.addItemToCart", {
                        name: item.name,
                      })}
                      onClick={(e) => {
                        e.stopPropagation();
                        // Stepper-less "+" quick-add always adds one.
                        handleAddToCart(item, 1);
                      }}
                      isDisabled={!item.is_available}
                    >
                      <span className="sr-only">{item.name} </span>
                      {t("menu.add")}
                    </Button>
                  )}
                </div>
              </div>
            )})}
          </div>
        </div>
      ))}
    </div>
  );

  // Category Tabs View (shows only active category)
  const CategoryTabsView = () => {
    const category = categories[activeCategory];
    if (!category) return null;

    return (
      <div className="space-y-8">
        <div className="text-center">
          <h2 className="text-3xl font-title text-ink-900 tracking-wide mb-3">
            {category.name}
          </h2>
          {category.description && (
            <p className="text-ink-600 font-light leading-relaxed max-w-2xl mx-auto">
              {category.description}
            </p>
          )}
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {category.items?.map((item, index) => {
            const itemKey = getItemStateKey(item as DisplayMenuItem);
            const coverSrc = getItemImages(item)[0];
            return (
            <div
              key={`${itemKey}:${index}`}
              className={`bg-white border border-warm-200/70 rounded-xl p-4 flex gap-4 hover:shadow-lg transition-all duration-300 ${!item.is_available ? "opacity-60" : ""
                } ${isItemAnimating(itemKey) ? "ring-2 ring-emerald-500" : ""}`}
            >
              <div className="relative w-24 h-24 flex-shrink-0 bg-warm-100 rounded-lg overflow-hidden">
                {coverSrc ? (
                  <NextImage
                    src={coverSrc}
                    alt={item.name}
                    fill
                    sizes="6rem"
                    unoptimized={!canOptimizeImageSrc(coverSrc)}
                    className="w-full h-full object-cover transition-transform duration-700 group-hover:scale-110"
                  />
                ) : (
                  <div className="w-full h-full flex items-center justify-center text-ink-500">
                    <ImageOff className="w-8 h-8" />
                  </div>
                )}
              </div>
              <div className="flex-1 flex flex-col justify-between">
                <div>
                  <div className="flex justify-between items-start gap-2">
                    <div>
                      <h3>
                        <button
                          type="button"
                          aria-label={t("accessibility.openItem", { name: item.name })}
                          className="block text-left font-medium text-ink-900 line-clamp-2 py-1 -my-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                          onClick={(event) => {
                            event.stopPropagation();
                            handleItemClick(item);
                          }}
                        >
                          {item.name}
                        </button>
                      </h3>
                      {isBundleItem(item as DisplayMenuItem) && (
                        <Chip
                          size="sm"
                          variant="flat"
                          className="text-[10px] h-5 px-2 bg-brand/10 text-brand-dark border border-brand/20 mt-1"
                        >
                          {t("menu.bundles.badge")}
                        </Chip>
                      )}
                      {showUnavailableForItem(item as DisplayMenuItem) && (
                        <Chip
                          size="sm"
                          color="danger"
                          variant="flat"
                          className="mt-1"
                        >
                          {unavailableLabelForItem(item as DisplayMenuItem)}
                        </Chip>
                      )}
                      {inventoryWarningBadge(item as DisplayMenuItem)}
                      {renderPromotionBadges(item as DisplayMenuItem)}
                    </div>
                    <div className="font-semibold text-ink-900 whitespace-nowrap">
                      <CurrencyPrice
                        amount={item.price || 0}
                        fromCurrency={defaultCurrency}
                        displayCurrency={displayCurrency}
                        locale={moneyLocale}
                      />
                    </div>
                  </div>
                  {item.description && (
                    <p className="text-ink-500 text-sm mt-1 line-clamp-2">
                      {item.description}
                    </p>
                  )}
                </div>

                <div className="flex items-center justify-between mt-3">
                  <div className="flex gap-1">
                    {item.dietary_tags?.slice(0, 2).map((tagId, idx) => {
                      const tag = DIETARY_TAGS.find(t => t.id === tagId);
                      return tag ? (
                        <span key={idx} className="rounded bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-800">
                          {t("menu." + tag.name)}
                        </span>
                      ) : null;
                    })}
                  </div>

                  {isOrderingEnabled && (
                    <Button
                      size="sm"
                      className="bg-brand text-white hover:bg-brand-dark min-w-0 h-8 px-3"
                      startContent={<Plus className="w-3 h-3" />}
                      aria-label={t("accessibility.addItemToCart", {
                        name: item.name,
                      })}
                      onClick={(e) => {
                        e.stopPropagation();
                        // Stepper-less "+" quick-add always adds one.
                        handleAddToCart(item, 1);
                      }}
                      isDisabled={!item.is_available}
                    >
                      <span className="sr-only">{item.name} </span>
                      {t("menu.add")}
                    </Button>
                  )}
                </div>
              </div>
            </div>
          )})}
        </div>
      </div>
    );
  };

  // Render the appropriate view based on viewMode

  // Total items across every (already-filtered) category. When this is zero the
  // four view bodies would render a blank scroll area, so surface an intentional
  // empty-state instead. When filters are active it means "your search matched
  // nothing" (offer a Clear action); otherwise the menu itself is unbuilt.
  const totalItemCount = categories.reduce(
    (sum, category) => sum + (category.items?.length ?? 0),
    0,
  );

  const renderView = () => {
    if (totalItemCount === 0) {
      if (hasActiveFilters) {
        return (
          <EmptyState
            icon={SearchX}
            title={t("menu.search.noResultsTitle")}
            subtitle={t("menu.search.noResultsDescription")}
            actionLabel={onClearFilters ? t("menu.filters.clear") : undefined}
            onAction={onClearFilters}
          />
        );
      }
      return (
        <EmptyState
          icon={Utensils}
          title={t("menu.menuComingSoon")}
          subtitle={t("menu.menuComingSoonDescription")}
        />
      );
    }

    // Invoke as plain functions, NOT as <Component/> elements. These views are
    // declared inside this component body, so their function identity changes
    // every render; rendered as JSX elements React treats each as a brand-new
    // component type and unmounts/remounts the ENTIRE menu subtree on every
    // state change (search keystroke, quantity tick, language switch), which is
    // what makes long (300+ item) menus janky and reflickers every image.
    // Calling them inlines their returned tree so React reconciles in place.
    // (Safe because none of them use hooks.)
    switch (viewMode) {
      case "compact":
        return CompactView();
      case "grid":
        return GridView();
      case "category-tabs":
        return CategoryTabsView();
      case "detailed":
      default:
        return DetailedView();
    }
  };

  const selectedItemKey = selectedItem ? getItemStateKey(selectedItem) : null;

  return (
    <div>
      <div
        role="status"
        aria-live="polite"
        aria-atomic="true"
        className="sr-only"
        data-testid="guest-menu-item-status"
      >
        {actionStatus}
      </div>
      {renderView()}

      {/* Item Details Modal */}
      <Modal
        isOpen={isItemModalOpen}
        onClose={handleItemModalClose}
        size="full"
        scrollBehavior="inside"
        classNames={{
          base: "sm:max-w-3xl",
          wrapper: "items-end sm:items-center",
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <div className="flex items-center justify-between gap-3 w-full">
                  <div className="flex min-w-0 items-center gap-2">
                    <h2 dir="auto" className="text-xl font-semibold text-ink-900 tracking-wide">
                      {selectedItem?.name}
                    </h2>
                    {selectedItem && isBundleItem(selectedItem) && (
                      <Chip
                        size="sm"
                        variant="flat"
                        className="text-[10px] h-5 px-2 bg-brand/10 text-brand-dark border border-brand/20"
                      >
                        {t("menu.bundles.badge")}
                      </Chip>
                    )}
                    {selectedItem &&
                      showUnavailableForItem(selectedItem) && (
                      <Chip size="sm" color="danger" variant="flat">
                        {unavailableLabelForItem(selectedItem)}
                      </Chip>
                    )}
                    {selectedItem &&
                      selectedItem.is_available &&
                      inventoryWarningBadge(selectedItem)}
                  </div>
                  {/* PG-13 #3: price in modal header (parity with /b) */}
                  {selectedItem ? (
                    <span className="shrink-0 text-lg font-semibold text-ink-900 tabular-nums">
                      <CurrencyPrice
                        amount={selectedItem.price || 0}
                        fromCurrency={defaultCurrency}
                        displayCurrency={displayCurrency}
                        locale={moneyLocale}
                      />
                    </span>
                  ) : null}
                </div>
                {selectedItem && renderPromotionBadges(selectedItem)}
                <p className="text-sm text-ink-500 font-light">
                  {t("menu.priceShownIn", { currency: displayCurrency })}
                </p>
              </ModalHeader>
              <ModalBody>
                {selectedItem && (
                  <div className="space-y-6">
                    {/* Item Images */}
                    {getItemImages(selectedItem).length > 0 && (
                      <div className="w-full">
                        <MemoizedImageCarousel
                          images={getItemImages(selectedItem)}
                          itemName={selectedItem.name}
                          size="lg"
                        />
                      </div>
                    )}

                    {/* Description */}
                    {selectedItem.description && (
                      <div>
                        <h3 className="text-lg font-medium text-ink-900 mb-2">
                          {t("menu.description")}
                        </h3>
                        <p dir="auto" className="text-ink-600 font-light leading-relaxed">
                          {selectedItem.description}
                        </p>
                      </div>
                    )}

                    {/* Options & Add-ons */}
                    {selectedItem.options &&
                      selectedItem.options.length > 0 && (
                        <div>
                          <h3 className="text-lg font-medium text-ink-900 mb-4">
                            {t("menu.optionsAndAddons")}
                          </h3>
                          <div className="space-y-3">
                            {selectedItem.options.map((option, index) => {
                              const optionId = getOptionStateKey(
                                selectedItem,
                                index,
                              );
                              const isSelected = selectedOptions.has(optionId);
                              return (
                                <button
                                  key={index}
                                  type="button"
                                  aria-pressed={
                                    isOrderingEnabled ? isSelected : undefined
                                  }
                                  disabled={!isOrderingEnabled}
                                  className={`p-4 rounded-xl border-2 transition-all duration-200 w-full text-left ${isOrderingEnabled
                                    ? `cursor-pointer ${isSelected ? "border-ink-900 bg-warm-50" : "border-warm-200 hover:border-warm-300"}`
                                    : "border-warm-200 bg-warm-50"
                                    }`}
                                  onClick={() =>
                                    isOrderingEnabled && toggleOption(optionId)
                                  }
                                >
                                  <div className="flex justify-between items-center">
                                    <div className="flex-1">
                                      <div className="flex items-center gap-2">
                                        <h4 className="font-medium text-ink-900">
                                          {option.name}
                                        </h4>
                                        {option.price_change &&
                                          option.price_change !== 0 && (
                                            <span
                                              className={`text-sm ${option.price_change > 0
                                                ? // A surcharge (costs more) is neutral, not green —
                                                  // green reads as "savings". Only a genuine price
                                                  // reduction gets the positive/green treatment.
                                                  "text-ink-700"
                                                : "text-emerald-700"
                                                }`}
                                            >
                                              {option.price_change > 0
                                                ? "+"
                                                : ""}
                                              <CurrencyPrice
                                                amount={option.price_change}
                                                fromCurrency={defaultCurrency}
                                                displayCurrency={
                                                  displayCurrency
                                                }
                                                locale={moneyLocale}
                                              />
                                            </span>
                                          )}
                                      </div>
                                    </div>
                                    {isOrderingEnabled && (
                                      <div
                                        className={`w-5 h-5 rounded-full border-2 flex items-center justify-center ${isSelected
                                          ? "border-ink-900 bg-ink-900"
                                          : "border-warm-300"
                                          }`}
                                      >
                                        {isSelected && (
                                          <Check className="w-3 h-3 text-white" />
                                        )}
                                      </div>
                                    )}
                                  </div>
                                </button>
                              );
                            })}
                          </div>
                        </div>
                      )}

                    {/* Allergen Information */}
                    {selectedItem.allergens &&
                      selectedItem.allergens.length > 0 && (
                        <div>
                          <h3 className="text-lg font-medium text-ink-900 mb-3">
                            {t("menu.allergenInformation")}
                          </h3>
                          <div className="bg-yellow-50 border border-yellow-200 rounded-xl p-4">
                            <p className="text-sm text-yellow-800 mb-2">
                              {t("menu.allergenWarning")}
                            </p>
                            <div className="flex flex-wrap gap-2">
                              {selectedItem.allergens.map((allergenId, index) => {
                                const allergen = ALLERGENS.find(a => a.id === allergenId);
                                // PG-13 #5: announce translated label, never the raw i18n key
                                const allergenLabel = allergen
                                  ? t("menu." + allergen.name)
                                  : allergenId;
                                return (
                                  <Chip
                                    key={index}
                                    size="sm"
                                    variant="solid"
                                    className="bg-red-600 text-white"
                                    startContent={allergen?.icon ? <NextImage src={allergen.icon} alt={allergenLabel} width={14} height={14} className="invert" /> : null}
                                  >
                                    {allergenLabel}
                                  </Chip>
                                );
                              })}
                            </div>
                          </div>
                        </div>
                      )}

                    {/* Dietary Information */}
                    {selectedItem.dietary_tags &&
                      selectedItem.dietary_tags.length > 0 && (
                        <div>
                          <h3 className="text-lg font-medium text-ink-900 mb-3">
                            {t("menu.dietaryInformation")}
                          </h3>
                          <div className="flex flex-wrap gap-2">
                            {selectedItem.dietary_tags.map((tagId, index) => {
                              const tag = DIETARY_TAGS.find(t => t.id === tagId);
                              return (
                                <Chip
                                  key={index}
                                  size="sm"
                                  variant="flat"
                                  color="success"
                                  className="text-xs"
                                >
                                  {tag ? t("menu." + tag.name) : tagId}
                                </Chip>
                              );
                            })}
                          </div>
                        </div>
                      )}
                  </div>
                )}
              </ModalBody>
              <ModalFooter>
                <div className="flex flex-col w-full gap-4">
                  {/* Quantity Selector - Only show if ordering is enabled */}
                  {isOrderingEnabled && (
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium text-ink-700">
                        {t("menu.quantityQ")}
                      </span>
                      <div className="flex items-center gap-3">
                        <Button
                          isIconOnly
                          size="sm"
                          variant="bordered"
                          aria-label={t(
                            "accessibility.decreaseItemQuantity",
                            { name: selectedItem?.name ?? "" },
                          )}
                          onPress={() =>
                            selectedItemKey &&
                            setItemQuantity(
                              selectedItemKey,
                              getItemQuantity(selectedItemKey) - 1,
                            )
                          }
                          isDisabled={
                            !selectedItemKey ||
                            getItemQuantity(selectedItemKey) <= 1
                          }
                        >
                          <Minus className="w-4 h-4" />
                        </Button>
                        <span className="text-lg font-medium min-w-[2rem] text-center">
                          {selectedItemKey ? getItemQuantity(selectedItemKey) : 1}
                        </span>
                        <Button
                          isIconOnly
                          size="sm"
                          variant="bordered"
                          aria-label={t(
                            "accessibility.increaseItemQuantity",
                            { name: selectedItem?.name ?? "" },
                          )}
                          onPress={() =>
                            selectedItemKey &&
                            setItemQuantity(
                              selectedItemKey,
                              getItemQuantity(selectedItemKey) + 1,
                            )
                          }
                        >
                          <Plus className="w-4 h-4" />
                        </Button>
                      </div>
                    </div>
                  )}

                  {/* Total Price - Only show if ordering is enabled */}
                  {isOrderingEnabled && (
                    <div className="flex items-center justify-between py-2 border-t border-warm-200">
                      <span className="text-lg font-medium text-ink-900">
                        {t("menu.totalLabel")}
                      </span>
                      <span className="text-xl font-semibold text-ink-900">
                        {selectedItem && (
                          <CurrencyPrice
                            amount={
                              calculateItemTotalPrice(selectedItem) *
                              getItemQuantity(selectedItemKey || "")
                            }
                            fromCurrency={defaultCurrency}
                            displayCurrency={displayCurrency}
                            locale={moneyLocale}
                          />
                        )}
                      </span>
                    </div>
                  )}

                  {/* Action Buttons */}
                  <div className="flex gap-3">
                    <Button
                      variant="light"
                      onPress={handleItemModalClose}
                      className="flex-1"
                    >
                      {t("menu.close")}
                    </Button>
                    {selectedItem && isOrderingEnabled && (
                      <Button
                        size="lg"
                        className="flex-1 font-medium bg-brand text-white hover:bg-brand-dark"
                        startContent={<Plus className="w-5 h-5" />}
                        aria-label={t("accessibility.addItemToCart", {
                          name: selectedItem.name,
                        })}
                        onPress={() => {
                            handleAddToCart(selectedItem);
                          }}
                        isDisabled={!selectedItem.is_available}
                      >
                        {getItemQuantity(selectedItemKey || "") === 1 ? (
                            <span className="truncate">
                              <span className="sr-only">{selectedItem.name} </span>
                              {t("menu.add")}
                            </span>
                          ) : (
                            <span className="truncate">
                              <span className="sr-only">{selectedItem.name} </span>
                              {t("menu.add")} (
                              {getItemQuantity(selectedItemKey || "")})
                            </span>
                          )}
                        </Button>
                    )}
                  </div>
                </div>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
};
