"use client";

import React, {
  useState,
  useEffect,
  useCallback,
  useMemo,
  useRef,
} from "react";
import NextImage from "next/image";
import {
  Image as NextUIImage,
  Chip,
  Button,
  Input,
  useDisclosure,
  Badge,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@nextui-org/react";
import { Bundle, BundleItemRef, MenuCategory, Offer } from "@/api/business";
import type { Orderability } from "@/api/orders";
import { CurrencyPrice } from "@/components/common/CurrencyConverter";
import {
  formatConvertedGuestCurrency,
  normalizeGuestLocale,
} from "@/utils/guestCurrencyFormatter";
import { useGuestConversionRate } from "@/hooks/useGuestConversionRate";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { ALLERGENS, DIETARY_TAGS } from "@/constants/menu-tags";
import {
  Search,
  AlertCircle,
  Plus,
  Minus,
  Utensils,
  X,
  ChefHat,
  MapPin,
  Truck,
  Clock,
  PencilLine,
  ShoppingCart,
  Trash2,
} from "lucide-react";
import type { FulfillmentContext } from "@/hooks/useFulfillmentContext";
import GuestDeliveryCheckout, {
  CartItem,
} from "@/components/delivery/GuestDeliveryCheckout";
import {
  getRadiusClass,
  getShadowClass,
  getMenuLayoutClass,
  getMenuCardClass,
  getMenuMediaClass,
  getSectionPadding,
} from "./designClasses";
import ImageLoadingSkeleton, {
  isImageAlreadyLoaded,
} from "@/components/shared/ImageLoadingSkeleton";
import {
  hexWithAlpha,
  resolveStorefrontFontClass,
} from "@/lib/storefront/theme";
import StorefrontMenuFilters, {
  bundleMatchesDietaryFilters,
  collectDietaryFiltersFromItems,
  itemMatchesDietaryFilters,
} from "./StorefrontMenuFilters";
import { ensureContrastWithWhiteText } from "./themedColor";
import { Z_DROPDOWN, Z_FAB, zStyle } from "./designLayers";
import MenuSkeleton from "./MenuSkeleton";
import SectionHeader from "./SectionHeader";
import MenuItemMedia from "./MenuItemMedia";
import {
  shouldShowItemInventoryWarningBadge,
  shouldShowItemUnavailableBadge,
} from "@/components/guest/menuItemAvailability";
import {
  computeGuestFulfillmentMinimumDelta,
  filterGuestMerchBundles,
  filterGuestMerchOffers,
  filterGuestSellableBundles,
  filterGuestSellableOffers,
  resolveGuestBundleChildName,
  shouldHideGuestPromoMerchForSearch,
} from "@/lib/guestPromotionAvailability";

interface PublicMenuDisplayProps {
  customUrl: string;
  businessId?: number;
  businessName?: string;
  categories: MenuCategory[];
  offers?: Offer[];
  bundles?: Bundle[];
  itemOrderability?: Record<string, Orderability>;
  menuSnapshotAuthoritative?: boolean;
  loading: boolean;
  fulfillmentContext?: FulfillmentContext | null;
  onEditFulfillment?: () => void;
  onClearFulfillment?: () => void;
  /** True only when Payverge-native delivery is enabled for this business.
   *  Partner links (UberEats etc.) do NOT count — the on-page cart can only
   *  check out through native delivery. Gates all add-to-cart affordances. */
  deliveryAvailable?: boolean;
  /** Whether the business is currently open. Defaults to true so existing
   *  callers without this prop are unaffected. When false, an inline
   *  "ordering paused" hint is shown and add-to-cart buttons are suppressed. */
  isOpen?: boolean;
  designSettings: {
    primary_color: string;
    secondary_color: string;
    corner_radius?: string;
    shadow_intensity?: string;
    font_family?: string;
    show_images?: boolean;
    show_descriptions?: boolean;
    menu_layout?: string;
    section_density?: string;
  };
  businessCurrencies: {
    default_currency: string;
    display_currency: string;
  };
  /** Business tax rate as a percent (e.g. 8 = 8%). Threaded to checkout
   *  for the price breakdown estimate. Defaults to 0. */
  taxRate?: number;
  /** Business service fee rate as a percent (e.g. 5 = 5%). Defaults to 0. */
  serviceFeeRate?: number;
}

const EMPTY_ITEM_ORDERABILITY: Record<string, Orderability> = {};

export const MenuImage = ({
  src,
  alt,
  className,
}: {
  src?: string;
  alt: string;
  className?: string;
}) => {
  const [error, setError] = useState(false);
  // Srcs whose bitmap has painted, keyed by src so carousel navigation back
  // to an already-loaded image never re-shows the skeleton.
  const [loadedSrcs, setLoadedSrcs] = useState<Record<string, boolean>>({});

  const markLoaded = useCallback((loadedSrc: string) => {
    setLoadedSrcs((prev) =>
      prev[loadedSrc] ? prev : { ...prev, [loadedSrc]: true },
    );
  }, []);

  if (!src || error) {
    return (
      <div
        className={`bg-gray-100 flex items-center justify-center ${className}`}
      >
        <Utensils className="w-1/3 h-1/3 text-gray-400" />
      </div>
    );
  }

  return (
    <div className="relative w-full h-full">
      <NextUIImage
        src={src}
        alt={alt}
        className={className}
        ref={(node: HTMLImageElement | null) => {
          // Cached images can be complete before onLoad is wired — mark them
          // immediately so there is no skeleton flash.
          if (isImageAlreadyLoaded(node)) markLoaded(src);
        }}
        onLoad={() => markLoaded(src)}
        onError={() => setError(true)}
        classNames={{
          wrapper: "w-full h-full",
          img: "w-full h-full object-cover",
        }}
      />
      {/* z-10 matches NextUI's img z-10; the skeleton renders later in the
          tree, so the DOM-order tiebreak keeps it on top without leaving the
          designLayers scale. */}
      {!loadedSrcs[src] && <ImageLoadingSkeleton className="z-10 bg-warm-100" />}
    </div>
  );
};

interface MenuCarouselProps {
  images: string[];
  alt: string;
  className?: string;
}

export const MenuCarousel = ({ images, alt, className }: MenuCarouselProps) => {
  const { t } = useGuestTranslation();
  const [currentIndex, setCurrentIndex] = useState(0);
  const [touchStart, setTouchStart] = useState(0);
  const [touchEnd, setTouchEnd] = useState(0);

  // Reset index when images change
  useEffect(() => {
    setCurrentIndex(0);
  }, [images]);

  const nextImage = (e?: React.MouseEvent) => {
    e?.preventDefault();
    e?.stopPropagation();
    if (images.length > 1) {
      setCurrentIndex((prev) => (prev + 1) % images.length);
    }
  };

  const prevImage = (e?: React.MouseEvent) => {
    e?.preventDefault();
    e?.stopPropagation();
    if (images.length > 1) {
      setCurrentIndex((prev) => (prev - 1 + images.length) % images.length);
    }
  };

  // Touch handlers for swipe
  const handleTouchStart = (e: React.TouchEvent) => {
    setTouchStart(e.targetTouches[0].clientX);
  };

  const handleTouchMove = (e: React.TouchEvent) => {
    setTouchEnd(e.targetTouches[0].clientX);
  };

  const handleTouchEnd = (e: React.TouchEvent) => {
    e.stopPropagation();
    if (!touchStart || !touchEnd) return;
    const distance = touchStart - touchEnd;
    const isLeftSwipe = distance > 50;
    const isRightSwipe = distance < -50;

    if (isLeftSwipe) {
      nextImage();
    }
    if (isRightSwipe) {
      prevImage();
    }

    setTouchEnd(0);
    setTouchStart(0);
  };

  if (!images || images.length === 0) {
    return (
      <div
        className={`relative w-full overflow-hidden bg-gray-100 ${className}`}
      >
        <MenuImage
          src={undefined}
          alt={alt}
          className="w-full h-full object-cover"
        />
      </div>
    );
  }

  return (
    <div
      className={`relative w-full overflow-hidden group ${className}`}
      onTouchStart={handleTouchStart}
      onTouchMove={handleTouchMove}
      onTouchEnd={handleTouchEnd}
    >
      <MenuImage
        src={images[currentIndex]}
        alt={`${alt} ${images.length > 1 ? `- ${currentIndex + 1}` : ""}`}
        className="w-full h-full object-cover transition-transform duration-500"
      />

      {images.length > 1 && (
        <>
          {/* Controls - Stop propagation to prevent opening modal if in list/modal overlay */}
          <button
            onClick={(e) => {
              e.stopPropagation();
              prevImage(e);
            }}
            className="absolute left-2 top-1/2 -translate-y-1/2 w-8 h-8 rounded-full bg-white/80 backdrop-blur-sm shadow-md flex items-center justify-center transition-all hover:bg-white hover:scale-110 active:scale-95"
            style={zStyle(Z_DROPDOWN)}
            aria-label={t("publicMenu.previousImage") || "Previous image"}
          >
            <svg
              className="w-5 h-5 text-gray-800"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M15 19l-7-7 7-7"
              />
            </svg>
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              nextImage(e);
            }}
            className="absolute right-2 top-1/2 -translate-y-1/2 w-8 h-8 rounded-full bg-white/80 backdrop-blur-sm shadow-md flex items-center justify-center transition-all hover:bg-white hover:scale-110 active:scale-95"
            style={zStyle(Z_DROPDOWN)}
            aria-label={t("publicMenu.nextImage") || "Next image"}
          >
            <svg
              className="w-5 h-5 text-gray-800"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9 5l7 7-7 7"
              />
            </svg>
          </button>

          {/* Indicators */}
          <div
            className="absolute bottom-3 left-1/2 -translate-x-1/2 flex gap-1.5 p-1 rounded-full bg-black/10 backdrop-blur-[2px]"
            style={zStyle(Z_DROPDOWN)}
          >
            {images.map((_, idx) => (
              <button
                key={idx}
                onClick={(e) => {
                  e.stopPropagation();
                  setCurrentIndex(idx);
                }}
                className={`w-1.5 h-1.5 rounded-full transition-all duration-300 ${
                  currentIndex === idx
                    ? "bg-white w-3 scale-110 shadow-sm"
                    : "bg-white/50 hover:bg-white/80"
                }`}
                aria-label={
                  (t("publicMenu.goToImage", { index: idx + 1 }) as string) ||
                  `Go to image ${idx + 1}`
                }
              />
            ))}
          </div>
        </>
      )}
    </div>
  );
};

/** Offer/bundle thumbnail that hides when the image fails. */
function OfferBundleImage({ src, alt }: { src: string; alt: string }) {
  const [error, setError] = useState(false);
  if (error) return null;
  return (
    <NextUIImage
      src={src}
      alt={alt}
      className="w-14 h-14 rounded-md object-cover shrink-0"
      onError={() => setError(true)}
    />
  );
}

function CategoryTab({
  id,
  panelId,
  label,
  isActive,
  primaryColor,
  onClick,
  onKeyDown,
  tabRef,
}: {
  id: string;
  panelId: string;
  label: string;
  isActive: boolean;
  primaryColor: string;
  onClick: () => void;
  onKeyDown: (event: React.KeyboardEvent<HTMLButtonElement>) => void;
  tabRef: (element: HTMLButtonElement | null) => void;
}) {
  return (
    <button
      id={id}
      ref={tabRef}
      type="button"
      role="tab"
      aria-selected={isActive}
      aria-controls={panelId}
      tabIndex={isActive ? 0 : -1}
      onClick={onClick}
      onKeyDown={onKeyDown}
      className={`relative flex-shrink-0 px-4 py-3 text-sm font-semibold tracking-wide whitespace-nowrap transition-colors ${
        isActive ? "text-gray-900" : "text-gray-500 hover:text-gray-900"
      }`}
    >
      {label}
      <span
        aria-hidden
        className={`absolute left-3 right-3 bottom-0 h-0.5 rounded-full transition-opacity ${
          isActive ? "opacity-100" : "opacity-0"
        }`}
        style={{ backgroundColor: primaryColor }}
      />
    </button>
  );
}

export default function PublicMenuDisplay({
  customUrl,
  businessId,
  businessName = "",
  categories,
  offers = [],
  bundles = [],
  itemOrderability = EMPTY_ITEM_ORDERABILITY,
  menuSnapshotAuthoritative = true,
  loading,
  fulfillmentContext,
  onEditFulfillment,
  onClearFulfillment,
  isOpen = true,
  deliveryAvailable = false,
  designSettings,
  businessCurrencies,
  taxRate = 0,
  serviceFeeRate = 0,
}: PublicMenuDisplayProps) {
  const { t, currentLanguage } = useGuestTranslation();
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  const { rate: displayRate } = useGuestConversionRate(
    businessCurrencies.default_currency,
    businessCurrencies.display_currency,
  );
  const fmtCurrency = (amount: number, _code?: string) =>
    formatConvertedGuestCurrency(
      amount,
      businessCurrencies.default_currency,
      businessCurrencies.display_currency,
      currentLanguage,
      displayRate,
    );
  const [searchQuery, setSearchQuery] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [activeCategory, setActiveCategory] = useState<number>(0);
  // Dietary filter toggles (QR table menu parity, storefront plan 3.7).
  const [selectedDietaryFilters, setSelectedDietaryFilters] = useState<
    Set<string>
  >(new Set());
  const categoryTabsScope = `public-menu-${String(businessId ?? customUrl).replace(/[^a-zA-Z0-9_-]/g, "-")}`;
  const categoryPanelId = `${categoryTabsScope}-panel`;
  const activeCategoryTabId = `${categoryTabsScope}-tab-${activeCategory}`;
  const categoryTabRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();
  // Delivery can intentionally use different hours from the dining room.
  // A live quote is authoritative for whether delivery ordering is open.
  const orderingOpen =
    isOpen ||
    (fulfillmentContext != null &&
      ["quote_available", "below_minimum"].includes(
        fulfillmentContext.quote.reason_code,
      ));

  // ── Cart state ──────────────────────────────────────────────────────────────
  // Persist the cart per business for the session, mirroring the delivery
  // context in useFulfillmentContext — a refresh must not wipe a half-built
  // delivery order while its address context survives.
  const cartStorageKey = businessId
    ? `payverge_public_cart_${businessId}`
    : null;
  const [cart, setCart] = useState<CartItem[]>(() => {
    if (!cartStorageKey || typeof window === "undefined") return [];
    try {
      const parsed = JSON.parse(
        window.sessionStorage.getItem(cartStorageKey) || "[]",
      );
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  });

  useEffect(() => {
    if (!cartStorageKey || typeof window === "undefined") return;
    try {
      if (cart.length === 0) {
        window.sessionStorage.removeItem(cartStorageKey);
      } else {
        window.sessionStorage.setItem(cartStorageKey, JSON.stringify(cart));
      }
    } catch {
      // Storage full/blocked — cart still works in-memory.
    }
  }, [cart, cartStorageKey]);

  const [isCartOpen, setIsCartOpen] = useState(false);
  const [isCheckoutOpen, setIsCheckoutOpen] = useState(false);

  const cartItemCount = cart.reduce((s, i) => s + i.quantity, 0);
  const cartSubtotalValue = cart.reduce(
    (sum, item) =>
      sum +
      (item.unit_price + item.options.reduce((s, o) => s + o.price_change, 0)) *
        item.quantity,
    0,
  );

  // Effective delivery fee against the LIVE cart subtotal (DLV-GUEST-3): the
  // stored quote was taken at address time (subtotal=0), so its delivery_fee
  // is always the base fee. Mirror GuestDeliveryCheckout's effectiveBaseFee —
  // once the cart meets free_delivery_minimum, the fee shown here is 0 so the
  // cart footer and the checkout modal can never contradict each other.
  const effectiveDeliveryFee = fulfillmentContext
    ? fulfillmentContext.quote.free_delivery_minimum > 0 &&
      cartSubtotalValue >= fulfillmentContext.quote.free_delivery_minimum
      ? 0
      : fulfillmentContext.quote.delivery_fee
    : 0;

  const normalizedCartSubtotal = Math.round(cartSubtotalValue * 100) / 100;
  const cartTaxAmount =
    taxRate > 0 ? Math.round(normalizedCartSubtotal * taxRate) / 100 : 0;
  const cartServiceFeeAmount =
    serviceFeeRate > 0
      ? Math.round(normalizedCartSubtotal * serviceFeeRate) / 100
      : 0;
  const cartGrandTotal =
    normalizedCartSubtotal +
    cartTaxAmount +
    cartServiceFeeAmount +
    effectiveDeliveryFee;

  const addToCart = useCallback(
    (item: any, selectedOpts: Set<string>, specialRequests?: string) => {
      if (item.is_available === false) return;
      const optionList = (item.options || [])
        .map((opt: any, idx: number) => {
          const optId = `${item.name}-option-${idx}`;
          return selectedOpts.has(optId) ? opt : null;
        })
        .filter(Boolean) as Array<{ name: string; price_change: number }>;

      const key = [
        item.menu_item_id || item.id || item.name,
        ...optionList.map((o) => o.name),
      ].join("|");

      setCart((prev) => {
        const existing = prev.findIndex((c) => c.key === key);
        if (existing >= 0) {
          return prev.map((c, i) =>
            i === existing ? { ...c, quantity: c.quantity + 1 } : c,
          );
        }
        const cartItem: CartItem = {
          key,
          menu_item_name: item.name,
          menu_item_id: item.menu_item_id || item.id,
          quantity: 1,
          unit_price: item.price || 0,
          item_type: item.item_type,
          bundle_id: item.bundle_id,
          options: optionList,
          special_requests: specialRequests,
        };
        return [...prev, cartItem];
      });
    },
    [],
  );

  const removeFromCart = useCallback((key: string) => {
    setCart((prev) => prev.filter((c) => c.key !== key));
  }, []);

  const updateCartQuantity = useCallback((key: string, delta: number) => {
    setCart((prev) =>
      prev
        .map((c) =>
          c.key === key ? { ...c, quantity: c.quantity + delta } : c,
        )
        .filter((c) => c.quantity > 0),
    );
  }, []);

  const clearCart = useCallback(() => setCart([]), []);
  // ── End cart state ──────────────────────────────────────────────────────────

  useEffect(() => {
    debounceRef.current = setTimeout(
      () => setDebouncedSearch(searchQuery),
      300,
    );
    return () => clearTimeout(debounceRef.current);
  }, [searchQuery]);

  const menuItemById = useMemo(() => {
    const lookup = new Map<
      string,
      { name: string; price: number; orderable: boolean }
    >();
    categories.forEach((category) => {
      category.items?.forEach((item) => {
        if (item.id) {
          const decision = itemOrderability[item.id];
          lookup.set(item.id, {
            name: item.name,
            price: item.price || 0,
            orderable: decision ? decision.orderable : item.is_available,
          });
        }
      });
    });
    return lookup;
  }, [categories, itemOrderability]);

  const parseBundleItems = useCallback((bundle: Bundle): BundleItemRef[] => {
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
  }, []);

  const getBundleRegularTotal = useCallback(
    (bundle: Bundle) => {
      const refs = parseBundleItems(bundle);
      return refs.reduce((sum, ref) => {
        const resolved = menuItemById.get(ref.menu_item_id);
        return sum + (resolved?.price || 0) * (ref.quantity || 1);
      }, 0);
    },
    [menuItemById, parseBundleItems],
  );

  const bundleChildName = useCallback(
    (ref: BundleItemRef) => resolveGuestBundleChildName(ref, menuItemById),
    [menuItemById],
  );

  const promotionCatalog = useMemo(
    () => ({
      categories,
      orderability: itemOrderability,
      bundles,
    }),
    [bundles, categories, itemOrderability],
  );

  const sellableOffers = useMemo(
    () => filterGuestSellableOffers(offers, promotionCatalog),
    [offers, promotionCatalog],
  );

  const sellableBundles = useMemo(
    () => filterGuestSellableBundles(bundles, promotionCatalog),
    [bundles, promotionCatalog],
  );

  const isBundleOrderable = useCallback(
    (bundle: Bundle) => {
      if (!bundle.is_active) return false;
      const refs = parseBundleItems(bundle);
      return (
        refs.length > 0 &&
        refs.every(
          (ref) => menuItemById.get(ref.menu_item_id)?.orderable === true,
        )
      );
    },
    [menuItemById, parseBundleItems],
  );

  // Revalidate persisted lines after the current menu projection arrives. A
  // refresh must not resurrect an item that inventory has since made
  // unorderable; bundle parents are valid only while every child is orderable.
  useEffect(() => {
    if (loading || !menuSnapshotAuthoritative) return;
    setCart((previous) => {
      const next = previous.filter((line) => {
        if (line.item_type === "bundle" || line.bundle_id) {
          const bundleId =
            line.bundle_id ?? Number(line.menu_item_id?.replace("bundle:", ""));
          const bundle = bundles.find((candidate) => candidate.id === bundleId);
          return bundle ? isBundleOrderable(bundle) : false;
        }

        return line.menu_item_id
          ? menuItemById.get(line.menu_item_id)?.orderable === true
          : false;
      });
      return next.length === previous.length ? previous : next;
    });
  }, [
    bundles,
    isBundleOrderable,
    loading,
    menuItemById,
    menuSnapshotAuthoritative,
  ]);

  const offerMatchesMenuItem = useCallback(
    (
      offer: Offer,
      item: { id?: string; name: string },
      categoryName: string,
    ) => {
      if (!offer.is_active) return false;
      const scope = offer.applicable_to || "all";
      const target = offer.target_id || "";
      const itemId = item.id || item.name;
      if (scope === "all") return true;
      if (scope === "item") return itemId === target || item.name === target;
      if (scope === "category") return categoryName === target;
      return false;
    },
    [],
  );

  const offerMatchesBundle = useCallback((offer: Offer, bundle: Bundle) => {
    if (!offer.is_active) return false;
    const scope = offer.applicable_to || "all";
    const target = offer.target_id || "";
    if (scope === "all") return true;
    if (scope === "bundle") return !!bundle.id && String(bundle.id) === target;
    return false;
  }, []);

  // After-hours merchandising: when no ordering path is open (dining closed and
  // no live delivery quote), do not attach promo chips to cards (table Closed Mode parity).
  const showPromoMerch = orderingOpen;

  const categoriesWithPromotions = useMemo(() => {
    const regularCategories = categories.map((category) => ({
      ...category,
      items: (category.items || []).map((item) => {
        const decision = item.id ? itemOrderability[item.id] : undefined;
        const promotion_offers = showPromoMerch
          ? sellableOffers.filter((offer) =>
              offerMatchesMenuItem(
                offer,
                { id: item.id, name: item.name },
                category.name,
              ),
            )
          : [];
        const inventoryOut = item.inventory_status === "out_of_stock";
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
                : decision?.state ?? item.orderability_state,
          item_type: "menu_item" as const,
          menu_item_id: item.id || item.name,
          promotion_offers,
          search_haystack: promotion_offers
            .map((offer) => `${offer.name} ${offer.description || ""}`)
            .join(" "),
        };
      }),
    }));

    const bundleItems = sellableBundles
      .filter((bundle) => bundle.is_active)
      .map((bundle) => {
        const refs = parseBundleItems(bundle);
        const summary = refs
          .map((ref) => `${ref.quantity}× ${bundleChildName(ref)}`)
          .join(" • ");
        const promotion_offers = showPromoMerch
          ? sellableOffers.filter((offer) => offerMatchesBundle(offer, bundle))
          : [];
        return {
          id: `bundle:${bundle.id}`,
          name: bundle.name,
          description: bundle.description || summary,
          price: bundle.price,
          currency: bundle.currency,
          image: bundle.image,
          images: bundle.image ? [bundle.image] : [],
          is_available: isBundleOrderable(bundle),
          options: [],
          allergens: [],
          dietary_tags: [],
          item_type: "bundle" as const,
          bundle_id: bundle.id,
          menu_item_id: `bundle:${bundle.id}`,
          promotion_offers,
          search_haystack: [
            summary,
            ...promotion_offers.map(
              (offer) => `${offer.name} ${offer.description || ""}`,
            ),
          ].join(" "),
        };
      });

    if (bundleItems.length === 0) {
      return regularCategories;
    }

    return [
      {
        name: t("menu.bundles.categoryName"),
        description: t("menu.bundles.categoryDescription"),
        items: bundleItems,
      },
      ...regularCategories,
    ];
  }, [
    bundleChildName,
    categories,
    sellableBundles,
    sellableOffers,
    itemOrderability,
    parseBundleItems,
    isBundleOrderable,
    offerMatchesMenuItem,
    offerMatchesBundle,
    showPromoMerch,
    t,
  ]);

  // Dietary filtering (QR parity). Bundle pseudo-items carry empty
  // dietary_tags, so available filters derive only from real menu items; a
  // bundle matches a selected filter when EVERY resolved child carries it.
  const menuItemDietaryTagsById = useMemo(() => {
    const lookup = new Map<string, string[]>();
    categories.forEach((category) => {
      category.items?.forEach((item) => {
        if (item.id) lookup.set(item.id, item.dietary_tags || []);
      });
    });
    return lookup;
  }, [categories]);

  const availableDietaryFilters = useMemo(
    () =>
      collectDietaryFiltersFromItems(
        categoriesWithPromotions.flatMap((category) =>
          (category.items || []).map((item) => ({
            dietary_tags: item.dietary_tags,
          })),
        ),
      ),
    [categoriesWithPromotions],
  );

  const toggleDietaryFilter = useCallback((filter: string) => {
    setSelectedDietaryFilters((prev) => {
      const next = new Set(prev);
      if (next.has(filter)) {
        next.delete(filter);
      } else {
        next.add(filter);
      }
      return next;
    });
  }, []);

  const getDietaryFilterLabel = useCallback(
    (filter: string): string => {
      switch (filter) {
        case "vegetarian":
          return t("menu.filters.vegetarian") || "Vegetarian";
        case "vegan":
          return t("menu.filters.vegan") || "Vegan";
        case "gluten-free":
          return t("menu.filters.glutenFree") || "Gluten-Free";
        case "dairy-free":
          return t("menu.filters.dairyFree") || "Dairy-Free";
        case "nut-free":
          return t("menu.filters.nutFree") || "Nut-Free";
        default:
          return filter;
      }
    },
    [t],
  );

  const radiusClass = getRadiusClass(designSettings.corner_radius || "medium");
  const shadowClass = getShadowClass(designSettings.shadow_intensity);
  const menuLayout = designSettings.menu_layout || "grid";
  const menuContainerClass = getMenuLayoutClass(menuLayout);
  const menuCardClass = getMenuCardClass(menuLayout);
  const menuMediaClass = getMenuMediaClass(menuLayout);
  // Primary color guarded for white text (the editor enforces AA, but
  // design_settings can also arrive via direct API writes).
  const themedSolid = ensureContrastWithWhiteText(designSettings.primary_color);

  const [selectedItem, setSelectedItem] = useState<any>(null);
  const [selectedOptions, setSelectedOptions] = useState<Set<string>>(
    new Set(),
  );
  const {
    isOpen: isItemModalOpen,
    onOpen: onItemModalOpen,
    onClose: onItemModalClose,
  } = useDisclosure();

  // Filter items based on debounced search + selected dietary filters
  const filteredCategories = useMemo(
    () =>
      categoriesWithPromotions.map((category) => ({
        ...category,
        items: category.items?.filter((item) => {
          const q = debouncedSearch.toLowerCase();
          const matchesSearch =
            item.name.toLowerCase().includes(q) ||
            item.description?.toLowerCase().includes(q) ||
            (item as { search_haystack?: string }).search_haystack
              ?.toLowerCase()
              .includes(q);
          if (!matchesSearch) return false;
          if (selectedDietaryFilters.size === 0) return true;
          if (item.item_type === "bundle") {
            const bundle = bundles.find(
              (candidate) => candidate.id === item.bundle_id,
            );
            if (!bundle) return false;
            const childTags = parseBundleItems(bundle)
              .map((ref) => menuItemDietaryTagsById.get(ref.menu_item_id))
              .filter((tags): tags is string[] => Array.isArray(tags));
            return bundleMatchesDietaryFilters(
              childTags,
              selectedDietaryFilters,
            );
          }
          return itemMatchesDietaryFilters(
            item.dietary_tags,
            selectedDietaryFilters,
          );
        }),
      })),
    [
      bundles,
      categoriesWithPromotions,
      debouncedSearch,
      menuItemDietaryTagsById,
      parseBundleItems,
      selectedDietaryFilters,
    ],
  );

  const filteredItemCount = useMemo(
    () =>
      filteredCategories.reduce(
        (count, category) => count + (category.items?.length ?? 0),
        0,
      ),
    [filteredCategories],
  );

  const searchActive = debouncedSearch.trim().length > 0;
  const displayedOffers = useMemo(
    () => filterGuestMerchOffers(sellableOffers, debouncedSearch),
    [debouncedSearch, sellableOffers],
  );
  const displayedBundles = useMemo(
    () =>
      filterGuestMerchBundles(sellableBundles, debouncedSearch, menuItemById),
    [debouncedSearch, menuItemById, sellableBundles],
  );
  const hidePromoForSearch = shouldHideGuestPromoMerchForSearch(
    searchActive,
    filteredItemCount,
  );
  const showPromoStrip =
    showPromoMerch &&
    !hidePromoForSearch &&
    (displayedOffers.length > 0 || displayedBundles.length > 0);

  const handleCategoryTabKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
      const count = filteredCategories.length + 1;
      if (count <= 0) return;
      let next = index;
      if (event.key === "ArrowRight" || event.key === "ArrowDown") {
        event.preventDefault();
        next = (index + 1) % count;
      } else if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
        event.preventDefault();
        next = (index - 1 + count) % count;
      } else if (event.key === "Home") {
        event.preventDefault();
        next = 0;
      } else if (event.key === "End") {
        event.preventDefault();
        next = count - 1;
      } else {
        return;
      }
      setActiveCategory(next);
      categoryTabRefs.current[next]?.focus();
    },
    [filteredCategories.length],
  );

  useEffect(() => {
    if (activeCategory > filteredCategories.length) {
      setActiveCategory(0);
    }
  }, [activeCategory, filteredCategories.length]);

  // Get all items for display
  const allItems = filteredCategories.flatMap((cat) => cat.items || []);

  // Handle item click to open modal
  const handleItemClick = (item: any) => {
    setSelectedItem(item);
    setSelectedOptions(new Set());
    onItemModalOpen();
  };

  // Toggle option selection
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

  // Calculate total price with selected options
  const calculateItemTotalPrice = (item: any) => {
    let totalPrice = item.price || 0;

    if (item.options) {
      item.options.forEach((option: any, index: number) => {
        const optionId = `${item.name}-option-${index}`;
        if (selectedOptions.has(optionId)) {
          totalPrice += option.price_change || 0;
        }
      });
    }

    return totalPrice;
  };

  if (loading) {
    return (
      <MenuSkeleton
        layout={menuLayout === "list" ? "list" : "grid"}
        density={designSettings.section_density}
      />
    );
  }

  if (categoriesWithPromotions.length === 0) {
    return (
      <section className="py-16 bg-gray-50">
        <div className="max-w-6xl mx-auto px-6">
          <div className="text-center py-20">
            <ChefHat className="w-16 h-16 mx-auto mb-4 text-gray-400" />
            <h3 className="text-2xl font-semibold text-gray-700 mb-2">
              {t("menu.menuComingSoon")}
            </h3>
            <p className="text-gray-500">
              {t("menu.menuComingSoonDescription")}
            </p>
          </div>
        </div>
      </section>
    );
  }

  const fontClass = resolveStorefrontFontClass(designSettings.font_family);
  const fulfillmentDelta = fulfillmentContext
    ? computeGuestFulfillmentMinimumDelta(
        cartSubtotalValue,
        fulfillmentContext.quote.minimum_order_amount,
      )
    : 0;

  return (
    <div className={`relative z-10 ${fontClass}`}>
      <section
        className={getSectionPadding(designSettings.section_density)}
      >
        <div className="max-w-6xl mx-auto px-6">
          {fulfillmentContext ? (
            <div className="mb-8 sticky top-24" style={zStyle(Z_DROPDOWN)}>
              <div
                className={`${radiusClass} border border-gray-200 bg-white px-5 py-4 shadow-sm`}
              >
                <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                  <div className="flex items-baseline gap-3 min-w-0">
                    <Chip color="success" variant="flat" className="shrink-0">
                      {t("businessPage.fulfillmentStrip.contextChip")}
                    </Chip>
                    <p className="text-sm text-gray-600 truncate">
                      {t("businessPage.fulfillmentStrip.contextHelp")}
                    </p>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    <Button
                      size="sm"
                      variant="light"
                      startContent={<PencilLine className="w-4 h-4" />}
                      onPress={onEditFulfillment}
                    >
                      {t("businessPage.fulfillmentStrip.edit")}
                    </Button>
                    <Button
                      size="sm"
                      variant="light"
                      color="danger"
                      onPress={onClearFulfillment}
                    >
                      {t("businessPage.fulfillmentStrip.clear")}
                    </Button>
                  </div>
                </div>

                <dl className="mt-3 pt-3 border-t border-gray-100 grid grid-cols-2 md:grid-cols-4 gap-y-2 gap-x-4 text-sm">
                  <div className="min-w-0">
                    <dt className="text-[11px] uppercase tracking-[0.2em] text-gray-500">
                      <MapPin className="inline w-3 h-3 mr-1 -mt-0.5" />
                      {t("businessPage.delivery.deliveryAddress")}
                    </dt>
                    <dd className="text-gray-900 font-medium truncate">
                      {fulfillmentContext.delivery_address.formatted_address}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-[11px] uppercase tracking-[0.2em] text-gray-500">
                      <Truck className="inline w-3 h-3 mr-1 -mt-0.5" />
                      {t("businessPage.fulfillmentStrip.deliveryFee")}
                    </dt>
                    <dd className="text-gray-900 font-medium">
                      {fmtCurrency(
                        effectiveDeliveryFee,
                        businessCurrencies.default_currency,
                      )}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-[11px] uppercase tracking-[0.2em] text-gray-500">
                      {t("businessPage.fulfillmentStrip.deliveryMinimum")}
                    </dt>
                    <dd className="text-gray-900 font-medium">
                      {fmtCurrency(
                        fulfillmentContext.quote.minimum_order_amount,
                        businessCurrencies.default_currency,
                      )}
                    </dd>
                    {fulfillmentDelta > 0 ? (
                      <p
                        className="text-[11px] text-amber-700 mt-0.5"
                        data-testid="fulfillment-minimum-progress"
                      >
                        {t("businessPage.fulfillmentStrip.addMore", {
                          amount: fmtCurrency(
                            fulfillmentDelta,
                            businessCurrencies.default_currency,
                          ),
                        })}
                      </p>
                    ) : (
                      <p
                        className="text-[11px] text-emerald-700 mt-0.5"
                        data-testid="fulfillment-minimum-progress"
                      >
                        {t("businessPage.fulfillmentStrip.minimumMet")}
                      </p>
                    )}
                  </div>
                  <div>
                    <dt className="text-[11px] uppercase tracking-[0.2em] text-gray-500">
                      <Clock className="inline w-3 h-3 mr-1 -mt-0.5" />
                      {t("businessPage.fulfillmentStrip.estimateLabel")}
                    </dt>
                    <dd className="text-gray-900 font-medium">
                      ~{fulfillmentContext.quote.estimated_total_minutes}{" "}
                      {t("businessPage.fulfillmentStrip.minutes")}
                    </dd>
                  </div>
                </dl>
              </div>
            </div>
          ) : null}

          {/* Header */}
          <SectionHeader
            badge={t("businessPage.menuTab")}
            title={t("businessPage.exploreOurOfferings")}
            subtitle={t("businessPage.discoverDishes")}
            designSettings={{
              primary_color: designSettings.primary_color,
              corner_radius: designSettings.corner_radius,
            }}
            centered
            as="h2"
          />

          {/* Search Bar */}
          <div className="mb-8 max-w-2xl mx-auto">
            <Input
              placeholder={t("menu.search.placeholder")}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              startContent={<Search className="w-5 h-5 text-gray-400" />}
              classNames={{
                inputWrapper: `border border-gray-200 hover:border-gray-300 h-12 ${radiusClass} bg-white`,
              }}
            />
          </div>

          {/* Closed-ordering hint — only when the business is currently closed */}
          {!orderingOpen && (
            <div
              data-testid="menu-closed-hint"
              role="status"
              className="mb-8 max-w-2xl mx-auto flex items-start gap-3 rounded-xl border border-amber-300/60 bg-amber-50/80 px-4 py-3 text-sm text-amber-900"
            >
              <Clock className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" />
              <span>
                {t("businessPage.menuClosedHint") ||
                  "Online ordering is paused while we’re closed. You can still browse the menu — please check back during opening hours."}
              </span>
            </div>
          )}

          {/* Mute offers/bundles merchandising strip when ordering is closed
              (dining closed and no live delivery path). Browse menu stays. */}
          {showPromoStrip && (
            <div
              data-testid="storefront-offers-bundles"
              className="mb-8 space-y-4"
            >
              {displayedOffers.length > 0 && (
                <section
                  className={`${radiusClass} border shadow-card p-5`}
                  style={{
                    backgroundColor: hexWithAlpha(
                      designSettings.primary_color,
                      "14",
                    ),
                    borderColor: hexWithAlpha(
                      designSettings.primary_color,
                      "33",
                    ),
                  }}
                  aria-labelledby="menu-offers-heading"
                >
                  <header className="flex items-center justify-between mb-3 gap-3">
                    <h3
                      id="menu-offers-heading"
                      className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-700"
                    >
                      {t("menu.offers.title")}
                    </h3>
                    <span
                      className="inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-white"
                      style={{ backgroundColor: themedSolid }}
                    >
                      {t("menu.offers.badge")}
                    </span>
                  </header>
                  <ul className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {displayedOffers.map((offer) => (
                      <li
                        key={offer.id || offer.name}
                        className="rounded-lg bg-white/70 p-3"
                      >
                        <div className="flex items-start gap-3">
                          {offer.image && (
                            <OfferBundleImage
                              src={offer.image}
                              alt={offer.name}
                            />
                          )}
                          <div className="flex-1 min-w-0">
                            <div className="flex items-baseline justify-between gap-2">
                              <p
                                dir="auto"
                                className="font-semibold text-gray-900 truncate"
                              >
                                {offer.name}
                              </p>
                              <p
                                className="text-sm font-semibold whitespace-nowrap"
                                style={{ color: designSettings.primary_color }}
                              >
                                {offer.discount_type === "percentage" ? (
                                  `−${offer.discount_value}%`
                                ) : (
                                  <>
                                    −{" "}
                                    <CurrencyPrice
                                      amount={offer.discount_value}
                                      fromCurrency={
                                        businessCurrencies.default_currency
                                      }
                                      displayCurrency={
                                        businessCurrencies.display_currency ||
                                        businessCurrencies.default_currency
                                      }
                                      locale={moneyLocale}
                                    />
                                  </>
                                )}
                              </p>
                            </div>
                            {offer.description && (
                              <p
                                dir="auto"
                                className="text-xs text-gray-500 mt-0.5 line-clamp-2"
                              >
                                {offer.description}
                              </p>
                            )}
                          </div>
                        </div>
                      </li>
                    ))}
                  </ul>
                </section>
              )}

              {displayedBundles.length > 0 && (
                <section
                  className={`${radiusClass} border shadow-card p-5`}
                  style={{
                    backgroundColor: hexWithAlpha(
                      designSettings.primary_color,
                      "14",
                    ),
                    borderColor: hexWithAlpha(
                      designSettings.primary_color,
                      "33",
                    ),
                  }}
                  aria-labelledby="menu-bundles-heading"
                >
                  <header className="flex items-center justify-between mb-3 gap-3">
                    <h3
                      id="menu-bundles-heading"
                      className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-700"
                    >
                      {t("menu.bundles.title")}
                    </h3>
                    <span
                      className="inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-white"
                      style={{ backgroundColor: themedSolid }}
                    >
                      {t("menu.bundles.badge")}
                    </span>
                  </header>
                  <ul className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {displayedBundles.map((bundle) => {
                      const regularTotal = getBundleRegularTotal(bundle);
                      const savings = Math.max(regularTotal - bundle.price, 0);
                      const refs = parseBundleItems(bundle);
                      return (
                        <li
                          key={bundle.id || bundle.name}
                          className="rounded-lg bg-white/70 p-3"
                        >
                          <div className="flex items-start gap-3">
                            {bundle.image && (
                              <OfferBundleImage
                                src={bundle.image}
                                alt={bundle.name}
                              />
                            )}
                            <div className="flex-1 min-w-0">
                              <div className="flex items-baseline justify-between gap-2">
                                <p
                                  dir="auto"
                                  className="font-semibold text-gray-900 truncate"
                                >
                                  {bundle.name}
                                </p>
                                <div className="text-right whitespace-nowrap">
                                  <CurrencyPrice
                                    amount={bundle.price}
                                    fromCurrency={
                                      businessCurrencies.default_currency
                                    }
                                    displayCurrency={
                                      businessCurrencies.display_currency
                                    }
                                    className="font-semibold"
                                    locale={moneyLocale}
                                  />
                                  {savings > 0 && (
                                    <p className="text-[11px] text-emerald-700 mt-0.5">
                                      {t("menu.bundles.saveLabel")}{" "}
                                      <CurrencyPrice
                                        amount={savings}
                                        fromCurrency={
                                          businessCurrencies.default_currency
                                        }
                                        displayCurrency={
                                          businessCurrencies.display_currency
                                        }
                                        locale={moneyLocale}
                                      />
                                    </p>
                                  )}
                                </div>
                              </div>
                              {bundle.description && (
                                <p
                                  dir="auto"
                                  className="text-xs text-gray-500 mt-0.5 line-clamp-2"
                                >
                                  {bundle.description}
                                </p>
                              )}
                              {refs.length > 0 && (
                                <p className="text-[11px] text-gray-500 mt-1 truncate">
                                  {refs
                                    .map(
                                      (ref) =>
                                        `${ref.quantity}× ${bundleChildName(ref)}`,
                                    )
                                    .join(" · ")}
                                </p>
                              )}
                            </div>
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                </section>
              )}
            </div>
          )}

          {/* Dietary filter toggles (QR table menu parity) — rendered only
              when at least one menu item carries filterable flags. */}
          <StorefrontMenuFilters
            filters={availableDietaryFilters}
            selected={selectedDietaryFilters}
            primaryColor={designSettings.primary_color}
            getLabel={getDietaryFilterLabel}
            onToggle={toggleDietaryFilter}
            groupLabel={t("menu.filters.title") || "Filters"}
          />

          {/* Category Tabs */}
          {filteredCategories.length > 1 && (
            <div
              role="tablist"
              aria-label={t("menu.categoryNav")}
              className="flex gap-1 mb-8 overflow-x-auto overflow-y-hidden pb-1 border-b border-gray-200 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
            >
              <CategoryTab
                id={`${categoryTabsScope}-tab-0`}
                panelId={categoryPanelId}
                label={t("menu.allItems")}
                isActive={activeCategory === 0}
                primaryColor={designSettings.primary_color}
                onClick={() => setActiveCategory(0)}
                tabRef={(el) => {
                  categoryTabRefs.current[0] = el;
                }}
                onKeyDown={(event) => handleCategoryTabKeyDown(event, 0)}
              />
              {filteredCategories.map((category, index) => (
                <CategoryTab
                  key={index}
                  id={`${categoryTabsScope}-tab-${index + 1}`}
                  panelId={categoryPanelId}
                  label={category.name}
                  isActive={activeCategory === index + 1}
                  primaryColor={designSettings.primary_color}
                  onClick={() => setActiveCategory(index + 1)}
                  tabRef={(el) => {
                    categoryTabRefs.current[index + 1] = el;
                  }}
                  onKeyDown={(event) =>
                    handleCategoryTabKeyDown(event, index + 1)
                  }
                />
              ))}
            </div>
          )}

          {/* Menu Items — grid or list per design_settings.menu_layout */}
          <div
            id={filteredCategories.length > 1 ? categoryPanelId : undefined}
            role={filteredCategories.length > 1 ? "tabpanel" : undefined}
            aria-labelledby={
              filteredCategories.length > 1 ? activeCategoryTabId : undefined
            }
            className={menuContainerClass}
          >
            {(activeCategory === 0
              ? allItems
              : filteredCategories[activeCategory - 1]?.items || []
            ).map((item: any) => {
              // Closed Mode (no ordering path): suppress Unavailable storm — closed
              // hint owns the message (table CM1 parity).
              const showUnavailableBadge = shouldShowItemUnavailableBadge({
                orderabilityState: item.orderability_state,
                venueClosed: !orderingOpen,
                isAvailable: item.is_available,
                inventoryStatus: item.inventory_status,
              });
              const showInventoryWarn = shouldShowItemInventoryWarningBadge({
                orderabilityState: item.orderability_state,
                venueClosed: !orderingOpen,
              });
              return (
                <article
                  key={item.id}
                  className={`group bg-white border border-gray-200 ${radiusClass} ${shadowClass} ${menuCardClass} hover:border-gray-300 hover:-translate-y-0.5 hover:shadow-lg transition-all duration-200 motion-reduce:transform-none motion-reduce:transition-none overflow-hidden`}
                >
                  <div className="p-0">
                    {designSettings.show_images !== false && (
                      <div className={`relative ${menuMediaClass}`}>
                        <MenuItemMedia
                          images={item.images || []}
                          alt={item.name}
                          noMediaLabel={t("menu.noImage")}
                        />
                        {showUnavailableBadge && (
                          <div className="absolute inset-0 bg-black/50 flex items-center justify-center z-10">
                            <Chip
                              className="bg-red-700 text-white"
                              variant="solid"
                            >
                              {t("menu.unavailable")}
                            </Chip>
                          </div>
                        )}
                      </div>
                    )}

                    <button
                      type="button"
                      aria-label={t("accessibility.openItem", {
                        name: item.name,
                      })}
                      onClick={() => handleItemClick(item)}
                      className="block w-full p-6 text-left cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand"
                    >
                      {designSettings.show_images === false &&
                        showUnavailableBadge && (
                          <Chip
                            size="sm"
                            variant="flat"
                            className="mb-2 bg-red-50 text-red-800"
                          >
                            {t("menu.unavailable")}
                          </Chip>
                        )}
                      {showInventoryWarn &&
                        item.orderability_state === "inventory_warning" && (
                          <Chip
                            size="sm"
                            variant="flat"
                            className="mb-2 border border-amber-300/60 bg-amber-50/80 text-amber-800"
                          >
                            {t("menu.lowStock")}
                          </Chip>
                        )}
                      <div className="flex justify-between items-start mb-2">
                        <h3
                          dir="auto"
                          className="font-title text-xl text-gray-900 leading-tight"
                        >
                          {item.name}
                        </h3>
                        <div className="text-right">
                          <div style={{ color: designSettings.primary_color }}>
                            <CurrencyPrice
                              amount={item.price}
                              fromCurrency={businessCurrencies.default_currency}
                              displayCurrency={
                                businessCurrencies.display_currency
                              }
                              className="text-lg font-bold"
                              locale={moneyLocale}
                            />
                          </div>
                        </div>
                      </div>

                      {designSettings.show_descriptions !== false &&
                        item.description && (
                          <p
                            dir="auto"
                            className="text-gray-600 text-sm mb-3 line-clamp-2"
                          >
                            {item.description}
                          </p>
                        )}

                      <div className="flex flex-wrap gap-1 mb-2">
                        {item.item_type === "bundle" && (
                          <Chip
                            size="sm"
                            variant="flat"
                            className="text-[10px] h-5 px-2 bg-brand/10 text-brand-dark border border-brand/20"
                          >
                            {t("menu.bundles.badge")}
                          </Chip>
                        )}
                        {Array.isArray(item.promotion_offers) &&
                          item.promotion_offers
                            .slice(0, 2)
                            .map((offer: Offer) => (
                              <Chip
                                key={
                                  offer.id ||
                                  `${offer.name}-${offer.target_id || "all"}`
                                }
                                size="sm"
                                variant="flat"
                                className="text-[10px] h-5 px-2 bg-amber-50 text-amber-700 border border-amber-200"
                              >
                                {offer.name}
                              </Chip>
                            ))}
                      </div>

                      {/* Dietary Tags, Allergens, and Options Indicator */}
                      <div className="flex flex-wrap gap-2 mt-3">
                        {item.allergens && item.allergens.length > 0 && (
                          <div className="flex flex-wrap gap-1">
                            {item.allergens.map(
                              (allergenId: string, idx: number) => {
                                const allergen = ALLERGENS.find(
                                  (a) => a.id === allergenId,
                                );
                                return (
                                  <div
                                    key={idx}
                                    className="inline-flex items-center justify-center w-6 h-6 bg-gray-50 border border-gray-200 rounded-full"
                                    title={
                                      allergen
                                        ? t("menu." + allergen.name)
                                        : allergenId
                                    }
                                  >
                                    {allergen?.icon ? (
                                      <NextImage
                                        src={allergen.icon}
                                        alt={
                                          allergen
                                            ? t("menu." + allergen.name)
                                            : allergenId
                                        }
                                        width={14}
                                        height={14}
                                        className="w-3.5 h-3.5 opacity-70"
                                      />
                                    ) : (
                                      <AlertCircle className="w-3 h-3 text-amber-600" />
                                    )}
                                  </div>
                                );
                              },
                            )}
                          </div>
                        )}
                        {item.dietary_tags && item.dietary_tags.length > 0 && (
                          <div className="flex flex-wrap gap-1">
                            {item.dietary_tags
                              .slice(0, 3)
                              .map((tagId: string, idx: number) => {
                                const tag = DIETARY_TAGS.find(
                                  (t) => t.id === tagId,
                                );
                                return (
                                  <Chip
                                    key={idx}
                                    size="sm"
                                    variant="flat"
                                    style={{
                                      backgroundColor: `${designSettings.primary_color}15`,
                                      color: designSettings.primary_color,
                                    }}
                                  >
                                    {tag ? t("menu." + tag.name) : tagId}
                                  </Chip>
                                );
                              })}
                          </div>
                        )}
                        {item.options && item.options.length > 0 && (
                          <Chip
                            size="sm"
                            variant="flat"
                            className="bg-brand/10 text-brand-dark border border-brand/20"
                            startContent={<Plus className="w-3 h-3" />}
                          >
                            {t("menu.customizable")}
                          </Chip>
                        )}
                      </div>
                    </button>
                  </div>
                </article>
              );
            })}
          </div>

          {/* No Results */}
          {searchActive && filteredItemCount === 0 && (
            <div className="text-center py-20">
              <Utensils className="w-16 h-16 mx-auto mb-4 text-gray-400" />
              <h3 className="text-2xl font-semibold text-gray-700 mb-2">
                {t("menu.search.noResultsTitle")}
              </h3>
              <p className="text-gray-500">
                {t("menu.search.noResultsDescription")}
              </p>
              <Button
                className="mt-4"
                variant="flat"
                data-testid="menu-search-clear"
                onPress={() => setSearchQuery("")}
              >
                {t("menu.filters.clear")}
              </Button>
            </div>
          )}
        </div>

        {/* Item Details Modal */}
        <Modal
          isOpen={isItemModalOpen}
          onClose={onItemModalClose}
          hideCloseButton
          scrollBehavior="inside"
          size="2xl"
          classNames={{
            wrapper: "items-end md:items-center",
            base: "m-0 max-h-[85vh] w-full rounded-b-none rounded-t-3xl bg-white text-gray-950 md:m-4 md:w-[90%] md:rounded-3xl",
            backdrop: "bg-black/60 backdrop-blur-md",
          }}
        >
          <ModalContent className="bg-white text-gray-950">
            {(closeDialog) => (
              <>
                {/* Drag Handle - Mobile Only */}
                <div className="md:hidden flex justify-center pt-4 pb-2">
                  <div className="w-12 h-1.5 bg-gray-300 rounded-full" />
                </div>

                {/* Header */}
                <ModalHeader className="flex items-center justify-between px-6 py-4 border-b border-gray-100 bg-white">
                  <h2
                    dir="auto"
                    className="text-xl md:text-2xl font-bold text-gray-900"
                  >
                    {selectedItem?.name}
                  </h2>
                  <button
                    onClick={closeDialog}
                    className="w-8 h-8 rounded-full bg-white border border-gray-300 text-gray-900 flex items-center justify-center hover:bg-gray-100 transition-colors"
                    aria-label={t("menu.dismissItemDetails")}
                  >
                    <X className="w-4 h-4 text-gray-900" />
                  </button>
                </ModalHeader>

                {/* Scrollable Content */}
                <ModalBody className="px-6 py-4" tabIndex={0}>
                  {selectedItem && (
                    <div className="space-y-6">
                      {/* Item Image with Carousel */}
                      <MenuCarousel
                        key={selectedItem.id}
                        images={selectedItem.images || []}
                        alt={selectedItem.name}
                        className="aspect-square rounded-xl bg-gray-100"
                      />

                      {/* Price */}
                      <div className="flex items-center justify-between">
                        <span className="text-lg font-semibold text-gray-700">
                          {t("menu.price")}:
                        </span>
                        <div style={{ color: designSettings.primary_color }}>
                          <CurrencyPrice
                            amount={calculateItemTotalPrice(selectedItem)}
                            fromCurrency={businessCurrencies.default_currency}
                            displayCurrency={
                              businessCurrencies.display_currency
                            }
                            className="text-2xl font-bold"
                            locale={moneyLocale}
                          />
                        </div>
                      </div>

                      {/* Description */}
                      {selectedItem.description && (
                        <div>
                          <h3 className="text-lg font-semibold text-gray-900 mb-2">
                            {t("menu.description")}
                          </h3>
                          <p
                            dir="auto"
                            className="text-gray-600 leading-relaxed"
                          >
                            {selectedItem.description}
                          </p>
                        </div>
                      )}

                      {/* Options & Add-ons */}
                      {selectedItem.options &&
                        selectedItem.options.length > 0 && (
                          <div>
                            <h3 className="text-lg font-semibold text-gray-900 mb-3 flex items-center gap-2">
                              <Plus className="w-5 h-5" />
                              {t("menu.optionsAndAddons")}
                            </h3>
                            <div className="space-y-2">
                              {selectedItem.options.map(
                                (option: any, index: number) => {
                                  const optionId = `${selectedItem.name}-option-${index}`;
                                  const isSelected =
                                    selectedOptions.has(optionId);

                                  return (
                                    <button
                                      key={index}
                                      type="button"
                                      onClick={() => toggleOption(optionId)}
                                      className={`p-4 rounded-lg border cursor-pointer transition-colors w-full text-left ${
                                        isSelected
                                          ? "bg-gray-50"
                                          : "border-gray-200 hover:border-gray-300"
                                      }`}
                                      style={
                                        isSelected
                                          ? {
                                              borderColor:
                                                designSettings.primary_color,
                                              backgroundColor: `${designSettings.primary_color}10`,
                                            }
                                          : undefined
                                      }
                                    >
                                      <div className="flex items-center justify-between">
                                        <div className="flex items-center gap-3">
                                          <div
                                            className={`w-5 h-5 rounded border flex items-center justify-center ${isSelected ? "" : "border-gray-300"}`}
                                            style={
                                              isSelected
                                                ? {
                                                    backgroundColor:
                                                      designSettings.primary_color,
                                                    borderColor:
                                                      designSettings.primary_color,
                                                  }
                                                : undefined
                                            }
                                          >
                                            {isSelected && (
                                              <svg
                                                className="w-3 h-3 text-white"
                                                fill="none"
                                                strokeLinecap="round"
                                                strokeLinejoin="round"
                                                strokeWidth="2"
                                                viewBox="0 0 24 24"
                                                stroke="currentColor"
                                              >
                                                <path d="M5 13l4 4L19 7"></path>
                                              </svg>
                                            )}
                                          </div>
                                          <span className="font-medium text-gray-900">
                                            {option.name}
                                          </span>
                                        </div>
                                        {option.price_change !== 0 && (
                                          <span className="text-sm font-semibold text-gray-700">
                                            +
                                            <CurrencyPrice
                                              amount={option.price_change}
                                              fromCurrency={
                                                businessCurrencies.default_currency
                                              }
                                              displayCurrency={
                                                businessCurrencies.display_currency
                                              }
                                              locale={moneyLocale}
                                            />
                                          </span>
                                        )}
                                      </div>
                                    </button>
                                  );
                                },
                              )}
                            </div>
                          </div>
                        )}

                      {/* Allergens */}
                      {selectedItem.allergens &&
                        selectedItem.allergens.length > 0 && (
                          <div>
                            <h3 className="text-lg font-semibold text-gray-900 mb-3 flex items-center gap-2">
                              <AlertCircle className="w-5 h-5 text-amber-600" />
                              {t("menu.allergenInformation")}
                            </h3>
                            <div className="rounded-lg bg-amber-50/50 p-4 border border-amber-200">
                              <p className="text-sm text-amber-800 mb-3 leading-snug">
                                {t("menu.allergenWarning")}
                              </p>
                              <ul className="flex flex-wrap gap-2">
                                {selectedItem.allergens.map(
                                  (allergenId: string, idx: number) => {
                                    const allergen = ALLERGENS.find(
                                      (a) => a.id === allergenId,
                                    );
                                    const label = allergen
                                      ? t("menu." + allergen.name)
                                      : allergenId;
                                    return (
                                      <li
                                        key={idx}
                                        className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-white border border-amber-200 text-xs font-medium text-amber-900"
                                      >
                                        {allergen?.icon ? (
                                          <NextImage
                                            src={allergen.icon}
                                            alt={label}
                                            width={14}
                                            height={14}
                                            className="w-3.5 h-3.5 opacity-70"
                                          />
                                        ) : (
                                          <AlertCircle className="w-3 h-3 text-amber-600" />
                                        )}
                                        <span>{label}</span>
                                      </li>
                                    );
                                  },
                                )}
                              </ul>
                            </div>
                          </div>
                        )}

                      {/* Dietary Tags */}
                      {selectedItem.dietary_tags &&
                        selectedItem.dietary_tags.length > 0 && (
                          <div>
                            <h3 className="text-lg font-semibold text-gray-900 mb-3">
                              {t("menu.dietaryInformation")}
                            </h3>
                            <div className="flex flex-wrap gap-2">
                              {selectedItem.dietary_tags.map(
                                (tagId: string, idx: number) => {
                                  const tag = DIETARY_TAGS.find(
                                    (t) => t.id === tagId,
                                  );
                                  return (
                                    <Chip
                                      key={idx}
                                      size="md"
                                      variant="flat"
                                      style={{
                                        backgroundColor: `${designSettings.primary_color}20`,
                                        color: designSettings.primary_color,
                                      }}
                                    >
                                      {tag ? t("menu." + tag.name) : tagId}
                                    </Chip>
                                  );
                                },
                              )}
                            </div>
                          </div>
                        )}

                      {/* Availability Status */}
                      {!selectedItem.is_available && (
                        <div className="rounded-lg border border-ink-950 bg-ink-950 p-4">
                          <p className="font-medium text-white">
                            {t("menu.itemUnavailable")}
                          </p>
                        </div>
                      )}
                      {selectedItem.orderability_state ===
                        "inventory_warning" && (
                        <div className="rounded-lg border border-amber-300/60 bg-amber-50/80 p-4">
                          <p className="font-medium text-amber-800">
                            {t("menu.lowStock")}
                          </p>
                        </div>
                      )}
                    </div>
                  )}
                </ModalBody>

                {/* Footer */}
                <ModalFooter className="block px-6 py-4 border-t border-gray-100 bg-white space-y-2">
                  {!orderingOpen ? (
                    <Button
                      color="default"
                      variant="flat"
                      isDisabled
                      className="w-full cursor-not-allowed"
                    >
                      {t("businessPage.menuClosedCta") ||
                        "Ordering unavailable while closed"}
                    </Button>
                  ) : selectedItem?.is_available !== false &&
                    fulfillmentContext ? (
                    <Button
                      color="primary"
                      className="w-full"
                      data-testid="add-to-cart-btn"
                      onPress={() => {
                        addToCart(selectedItem, selectedOptions);
                        closeDialog();
                      }}
                    >
                      {t("menu.addToCart")} ·{" "}
                      {fmtCurrency(
                        calculateItemTotalPrice(selectedItem),
                        businessCurrencies.default_currency,
                      )}
                    </Button>
                  ) : selectedItem?.is_available !== false &&
                    deliveryAvailable ? (
                    <Button
                      color="primary"
                      variant="flat"
                      className="w-full"
                      data-testid="start-delivery-order-btn"
                      onPress={() => {
                        closeDialog();
                        onEditFulfillment?.();
                      }}
                    >
                      {t("publicMenu.startDeliveryOrder")}
                    </Button>
                  ) : null}
                  <Button
                    color="default"
                    variant="light"
                    onPress={closeDialog}
                    className="w-full text-gray-900"
                  >
                    {t("menu.close")}
                  </Button>
                </ModalFooter>
              </>
            )}
          </ModalContent>
        </Modal>

        {/* ── Cart panel ────────────────────────────────────────────────────────── */}
        {cartItemCount > 0 && (
          <>
            {/* Floating cart button */}
            <div
              className={`fixed right-4 md:right-8 bottom-[calc(var(--cookie-banner-height,0px)+6rem)] md:bottom-[calc(var(--cookie-banner-height,0px)+2rem)] ${
                isCartOpen ? "invisible pointer-events-none" : ""
              }`}
              style={zStyle(Z_FAB)}
            >
              <Badge content={cartItemCount} color="danger" size="lg">
                <Button
                  color="primary"
                  className="shadow-xl"
                  startContent={<ShoppingCart className="w-5 h-5" />}
                  onPress={() => setIsCartOpen(true)}
                  data-testid="open-cart-btn"
                >
                  {t("menu.viewCart")} ·{" "}
                  {fmtCurrency(
                    cartSubtotalValue,
                    businessCurrencies.default_currency,
                  )}
                </Button>
              </Badge>
            </div>

            {/* Cart side panel */}
            <Modal
              isOpen={isCartOpen}
              onClose={() => setIsCartOpen(false)}
              aria-label={t("menu.cart")}
              hideCloseButton
              scrollBehavior="inside"
              size="sm"
              classNames={{
                wrapper: "justify-end",
                base: "!m-0 h-full max-h-full w-full max-w-sm rounded-none",
                backdrop: "bg-black/40 backdrop-blur-sm",
              }}
            >
              <ModalContent data-testid="cart-panel">
                {(closeCart) => (
                  <>
                    {/* Cart header */}
                    <ModalHeader className="flex items-center justify-between px-6 py-4 border-b border-gray-100">
                      <div className="flex items-center gap-2">
                        <ShoppingCart className="w-5 h-5 text-gray-700" />
                        <span className="font-semibold text-gray-900">
                          {t("menu.cart")} ({cartItemCount})
                        </span>
                      </div>
                      <button
                        onClick={closeCart}
                        className="w-8 h-8 rounded-full bg-gray-100 flex items-center justify-center hover:bg-gray-200 transition-colors"
                        aria-label={t("menu.close")}
                      >
                        <X className="w-4 h-4 text-gray-600" />
                      </button>
                    </ModalHeader>

                    {/* Cart items */}
                    <ModalBody className="px-6 py-4 space-y-3">
                      {cart.map((item) => {
                        const linePrice =
                          item.unit_price +
                          item.options.reduce((s, o) => s + o.price_change, 0);
                        return (
                          <div
                            key={item.key}
                            className="flex items-start gap-3 rounded-xl border border-gray-100 bg-gray-50 p-3"
                          >
                            <div className="flex-1 min-w-0">
                              <p className="font-medium text-gray-900 text-sm truncate">
                                {item.menu_item_name}
                              </p>
                              {item.options.length > 0 && (
                                <p className="text-xs text-gray-500 mt-0.5">
                                  {item.options.map((o) => o.name).join(", ")}
                                </p>
                              )}
                              <p className="text-sm font-semibold text-gray-700 mt-1">
                                {fmtCurrency(
                                  linePrice * item.quantity,
                                  businessCurrencies.default_currency,
                                )}
                              </p>
                            </div>
                            <div className="flex items-center gap-1 shrink-0">
                              <button
                                onClick={() => updateCartQuantity(item.key, -1)}
                                className="w-7 h-7 rounded-full border border-gray-200 flex items-center justify-center hover:bg-gray-100 transition-colors"
                                aria-label={
                                  t("publicMenu.decreaseQuantity") ||
                                  "Decrease quantity"
                                }
                              >
                                <Minus className="w-3 h-3 text-gray-600" />
                              </button>
                              <span className="w-6 text-center text-sm font-medium text-gray-900">
                                {item.quantity}
                              </span>
                              <button
                                onClick={() => updateCartQuantity(item.key, 1)}
                                className="w-7 h-7 rounded-full border border-gray-200 flex items-center justify-center hover:bg-gray-100 transition-colors"
                                aria-label={
                                  t("publicMenu.increaseQuantity") ||
                                  "Increase quantity"
                                }
                              >
                                <Plus className="w-3 h-3 text-gray-600" />
                              </button>
                              <button
                                onClick={() => removeFromCart(item.key)}
                                className="w-7 h-7 ml-1 rounded-full flex items-center justify-center hover:bg-red-50 transition-colors"
                                aria-label={
                                  t("publicMenu.removeItem") || "Remove item"
                                }
                              >
                                <Trash2 className="w-3 h-3 text-red-500" />
                              </button>
                            </div>
                          </div>
                        );
                      })}
                    </ModalBody>

                    {/* Cart footer */}
                    <ModalFooter className="block px-6 py-4 border-t border-gray-100 bg-gray-50 space-y-3">
                      <div className="flex justify-between text-sm text-gray-700">
                        <span>
                          {t("businessPage.checkout.subtotal") ||
                            t("menu.cartTotal")}
                        </span>
                        <span>
                          {fmtCurrency(
                            cartSubtotalValue,
                            businessCurrencies.default_currency,
                          )}
                        </span>
                      </div>
                      {fulfillmentContext ? (
                        <>
                          {cartTaxAmount > 0 && (
                            <div className="flex justify-between text-sm text-gray-700">
                              <span>
                                {t("businessPage.checkout.tax") || "Tax"}
                              </span>
                              <span>
                                {fmtCurrency(
                                  cartTaxAmount,
                                  businessCurrencies.default_currency,
                                )}
                              </span>
                            </div>
                          )}
                          {cartServiceFeeAmount > 0 && (
                            <div className="flex justify-between text-sm text-gray-700">
                              <span>
                                {t("businessPage.checkout.serviceFee") ||
                                  "Service fee"}
                              </span>
                              <span>
                                {fmtCurrency(
                                  cartServiceFeeAmount,
                                  businessCurrencies.default_currency,
                                )}
                              </span>
                            </div>
                          )}
                          <div className="flex justify-between text-sm text-gray-700">
                            <span>
                              {t("publicMenu.deliveryFee") || "Delivery fee"}
                            </span>
                            <span>
                              {fmtCurrency(
                                effectiveDeliveryFee,
                                businessCurrencies.default_currency,
                              )}
                            </span>
                          </div>
                          <div className="flex justify-between text-sm text-ink-900">
                            <span className="font-semibold">
                              {t("menu.cartTotal")}
                            </span>
                            <span className="font-semibold">
                              {fmtCurrency(
                                cartGrandTotal,
                                businessCurrencies.default_currency,
                              )}
                            </span>
                          </div>
                          {cartSubtotalValue <
                            fulfillmentContext.quote.minimum_order_amount && (
                            <p className="text-xs text-warning-700">
                              {(t("publicMenu.addMoreForDeliveryMinimum", {
                                amount: fmtCurrency(
                                  fulfillmentContext.quote
                                    .minimum_order_amount - cartSubtotalValue,
                                  businessCurrencies.default_currency,
                                ),
                              }) as string) ||
                                `Add ${fmtCurrency(
                                  fulfillmentContext.quote
                                    .minimum_order_amount - cartSubtotalValue,
                                  businessCurrencies.default_currency,
                                )} more to meet the delivery minimum.`}
                            </p>
                          )}
                          <Button
                            color="primary"
                            className="w-full"
                            isDisabled={
                              cartSubtotalValue <
                              fulfillmentContext.quote.minimum_order_amount
                            }
                            onPress={() => {
                              setIsCartOpen(false);
                              setIsCheckoutOpen(true);
                            }}
                            data-testid="checkout-btn"
                          >
                            {t("menu.reviewOrder")} ·{" "}
                            {fmtCurrency(
                              cartGrandTotal,
                              businessCurrencies.default_currency,
                            )}
                          </Button>
                        </>
                      ) : (
                        <div className="space-y-2">
                          <p className="text-xs text-gray-500 text-center">
                            {t("businessPage.checkout.addToDeliveryMinNote")}
                          </p>
                          {deliveryAvailable && (
                            <Button
                              color="primary"
                              variant="flat"
                              className="w-full"
                              data-testid="cart-set-address-btn"
                              onPress={() => {
                                setIsCartOpen(false);
                                onEditFulfillment?.();
                              }}
                            >
                              {t("publicMenu.setDeliveryAddress")}
                            </Button>
                          )}
                        </div>
                      )}
                      <Button
                        variant="light"
                        color="danger"
                        className="w-full"
                        size="sm"
                        onPress={clearCart}
                      >
                        {t("menu.clearCart")}
                      </Button>
                    </ModalFooter>
                  </>
                )}
              </ModalContent>
            </Modal>
          </>
        )}
      </section>

      {/* ── Guest delivery checkout modal ────────────────────────────────────────── */}
      {isCheckoutOpen && fulfillmentContext && businessId ? (
        <GuestDeliveryCheckout
          isOpen={isCheckoutOpen}
          onClose={() => setIsCheckoutOpen(false)}
          businessId={businessId}
          businessName={businessName}
          fulfillmentContext={fulfillmentContext}
          cart={cart}
          onSuccess={clearCart}
          currency={businessCurrencies.default_currency}
          taxRate={taxRate}
          serviceFeeRate={serviceFeeRate}
        />
      ) : null}
    </div>
  );
}
