"use client";
import NextImage from "next/image";

import React from "react";
import { Button, Chip, Badge } from "@nextui-org/react";
import { Plus, Check } from "lucide-react";
import { MenuItem, Offer } from "../../api/business";
import { ImageCarousel } from "./ImageCarousel";
import { CurrencyPrice } from "../common/CurrencyConverter";
import { ALLERGENS, DIETARY_TAGS } from "../../constants/menu-tags";
import { AlertCircle } from "lucide-react";
import {
  isInventoryEightySix,
  isMenuItemOrderable,
  isVenueClosedState,
  shouldShowItemInventoryWarningBadge,
  shouldShowItemPromotionOffers,
  shouldShowItemUnavailableBadge,
} from "./menuItemAvailability";

interface MobileMenuItemProps {
  item: MenuItem & {
    item_type?: "menu_item" | "bundle";
    bundle_id?: number;
    promotion_offers?: Offer[];
    orderability_state?: string;
  };
  isAnimating: boolean;
  isAdded: boolean;
  quantity: number;
  defaultCurrency: string;
  displayCurrency: string;
  /** Guest BCP-47 locale for money formatting (PG-8). */
  locale: string;
  onItemClick: (item: MenuItem) => void;
  onAddToCart?: () => void;
  isOrderingEnabled?: boolean;
  /** Parent Closed Mode flag; also derived from item.orderability_state. */
  venueClosed?: boolean;
  t: (key: string, params?: Record<string, string | number>) => string;
}

export const MobileMenuItem: React.FC<MobileMenuItemProps> = ({
  item,
  isAnimating,
  isAdded,
  quantity,
  defaultCurrency,
  displayCurrency,
  locale,
  onItemClick,
  onAddToCart,
  isOrderingEnabled = true,
  venueClosed: venueClosedProp,
  t,
}) => {
  const getItemImages = (item: MenuItem): string[] => {
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
  };

  const isBundleItem =
    item.item_type === "bundle" ||
    !!item.bundle_id ||
    (typeof item.id === "string" && item.id.startsWith("bundle:"));
  const venueClosed =
    Boolean(venueClosedProp) || isVenueClosedState(item.orderability_state);
  const showUnavailableBadge = shouldShowItemUnavailableBadge({
    orderabilityState: item.orderability_state,
    venueClosed,
    isAvailable: item.is_available,
    inventoryStatus: item.inventory_status,
  });
  const showInventoryWarning = shouldShowItemInventoryWarningBadge({
    orderabilityState: item.orderability_state,
    venueClosed,
  });
  const itemOrderable = isMenuItemOrderable(item);
  const promotionOffers =
    shouldShowItemPromotionOffers(venueClosed) &&
    Array.isArray(item.promotion_offers)
      ? item.promotion_offers
      : [];

  return (
    <div
      className={`relative bg-white rounded-2xl overflow-hidden transition-all duration-200 shadow-sm hover:shadow-md ${!itemOrderable ? "opacity-60" : ""
        } ${isAnimating ? "ring-2 ring-emerald-500 shadow-lg" : ""}`}
    >
      {/* Success Animation */}
      {isAnimating && (
        <div className="absolute inset-0 bg-emerald-500/10 flex items-center justify-center z-20">
          <div className="bg-emerald-600 text-white px-3 py-1.5 rounded-full flex items-center gap-2 motion-safe:animate-pulse">
            <Check className="w-4 h-4" />
            <span className="text-sm font-medium">
              {t("menu.itemAdded", { quantity })}
            </span>
          </div>
        </div>
      )}

      {/* Unavailable Badge — suppressed in Closed Mode (banner owns the message) */}
      {showUnavailableBadge && (
        <div className="absolute top-2 right-2 bg-rose-600 text-white text-xs px-2 py-1 rounded-full font-medium z-10">
          {isInventoryEightySix({
            orderabilityState: item.orderability_state,
            inventoryStatus: item.inventory_status,
          })
            ? t("menu.outOfStock")
            : t("menu.unavailable")}
        </div>
      )}
      {showInventoryWarning &&
        item.orderability_state === "inventory_warning" && (
        <div className="absolute top-2 right-2 z-10 rounded-full bg-amber-100 px-2 py-1 text-xs font-medium text-amber-800">
          {t("menu.lowStock")}
        </div>
      )}

      {/* Horizontal Layout for Mobile */}
      <div className="flex gap-4 p-4">
        {/* Compact Image */}
        <div className="relative flex-shrink-0">
          <div className="w-24 h-24 rounded-xl overflow-hidden bg-warm-100 shadow-sm">
            <ImageCarousel
              key={`carousel-${item.id || item.name}`}
              images={getItemImages(item)}
              itemName={item.name}
              size="sm"
            />
          </div>
          {getItemImages(item).length > 1 && (
            <div className="absolute bottom-1.5 right-1.5 bg-black/70 text-white text-xs px-1.5 py-0.5 rounded-md font-medium">
              {getItemImages(item).length}
            </div>
          )}
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0 flex flex-col">
          {/* Title and Price Row */}
          <div className="flex items-start justify-between gap-2 mb-1.5">
            <div className="flex-1 min-w-0">
              <h3>
                <button
                  type="button"
                  aria-label={t("accessibility.openItem", { name: item.name })}
                  // RSP-1: `py-1 -my-1` expands the tap target from 20px to 28px
                  // (WCAG 2.5.8 AA floor is 24) while the negative margin cancels
                  // the padding, so the rendered layout is byte-identical. This is
                  // the primary ordering interaction on a phone and the card around
                  // it is an inert <div>, so this button is the only way in.
                  className="block text-left text-base font-semibold text-ink-900 line-clamp-2 leading-tight py-1 -my-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                  onClick={(event) => {
                    event.stopPropagation();
                    onItemClick(item);
                  }}
                >
                  {item.name}
                </button>
              </h3>
              <div className="flex flex-wrap gap-1 mt-1">
                {isBundleItem && (
                  <Chip
                    size="sm"
                    variant="flat"
                    className="text-[10px] h-5 px-2 bg-brand/10 text-brand-dark border border-brand/20"
                  >
                    {t("menu.bundles.badge")}
                  </Chip>
                )}
                {promotionOffers.slice(0, 2).map((offer) => (
                  <Chip
                    key={offer.id || `${offer.name}-${offer.target_id || "all"}`}
                    size="sm"
                    variant="flat"
                    className="text-[10px] h-5 px-2 bg-amber-50 text-amber-700 border border-amber-200"
                  >
                    {offer.name}
                  </Chip>
                ))}
                {promotionOffers.length > 2 && (
                  <Chip
                    size="sm"
                    variant="flat"
                    className="text-[10px] h-5 px-2 bg-amber-50 text-amber-700 border border-amber-200"
                  >
                    +{promotionOffers.length - 2}
                  </Chip>
                )}
              </div>
            </div>
            <div className="flex-shrink-0 text-base font-bold text-ink-900">
              <CurrencyPrice
                amount={item.price || 0}
                fromCurrency={defaultCurrency}
                displayCurrency={displayCurrency}
                locale={locale}
              />
            </div>
          </div>

          {/* Description */}
          {item.description && (
            <p className="text-sm text-ink-600 line-clamp-2 mb-3 leading-relaxed">
              {item.description}
            </p>
          )}

          {/* Tags and Add Button Row */}
          <div className="flex items-center justify-between gap-3 mt-auto">
            {/* Dietary Tags, Allergens, and Options Indicator */}
            <div className="flex flex-wrap gap-1.5 flex-1">
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
                          className="text-[10px] h-6 px-1.5 bg-emerald-50 text-emerald-800 font-medium"
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
                  className="text-xs h-6 px-2 bg-brand/10 text-brand-dark border border-brand/20 font-medium"
                >
                  {t("menu.customizable")} ({item.options.length})
                </Chip>
              )}
            </div>

            {/* Add Button - Only show if ordering is enabled */}
            {itemOrderable && onAddToCart && isOrderingEnabled && (
              <Button
                size="md"
                color={isAdded ? "success" : "default"}
                variant={isAdded ? "flat" : "solid"}
                className={`min-w-[80px] h-9 font-semibold shadow-sm ${isAdded
                  ? "bg-emerald-50 text-emerald-800 border border-emerald-200"
                  : "bg-brand text-white hover:bg-brand-dark"
                  }`}
                startContent={
                  isAdded ? (
                    <Badge
                      content={quantity}
                      size="sm"
                      color="success"
                      className="font-bold"
                    >
                      <Check className="w-4 h-4" />
                    </Badge>
                  ) : (
                    <Plus className="w-4 h-4" />
                  )
                }
                aria-label={t("accessibility.addItemToCart", {
                  name: item.name,
                })}
                onClick={(event) => {
                  event.stopPropagation();
                  if (onAddToCart) {
                    onAddToCart();
                  }
                }}
              >
                <span className="text-sm">
                  <span className="sr-only">{item.name} </span>
                  {isAdded ? t("menu.added") : t("menu.add")}
                </span>
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
