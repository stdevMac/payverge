"use client";

import React, {
  useState,
  useEffect,
  useCallback,
  useMemo,
  useRef,
} from "react";
import { Button, Spinner, Image } from "@nextui-org/react";
import {
  ArrowLeft,
  ShoppingCart,
  Filter,
  X,
  Info,
} from "lucide-react";
import { useParams } from "next/navigation";
import Link from "next/link";
import dynamic from "next/dynamic";
import toast from "react-hot-toast";
import PersistentGuestNav from "../../../../components/navigation/PersistentGuestNav";
import CallWaiterButton from "../../../../components/guest/CallWaiterButton";
import { CurrencyPrice } from "../../../../components/common/CurrencyConverter";
import { convertAmount } from "../../../../api/currency";
import {
  formatConvertedGuestCurrency,
  normalizeGuestLocale,
} from "@/utils/guestCurrencyFormatter";
import { shouldShowAiWaiter } from "./aiGate";
import { MenuFilterToggles } from "@/components/guest/MenuFilterToggles";
import {
  GUEST_MENU_FILTERS_PANEL_ID,
  GUEST_MENU_HEADER_OFFSET_PX,
  guestMenuChromeBarPaddingClass,
  guestMenuChromeShowsTableLabel,
  guestMenuChromeTitleClass,
} from "./guestMenuChrome";
import {
  loadGuestMenuDependencies,
  resolveGuestTableSettlement,
  MENU_DEPENDENCY_TIMEOUT_MS,
  shouldShowMenuLoadingGate,
} from "./guestMenuLoad";
import MenuViewRadiogroup from "./_components/MenuViewRadiogroup";
import GuestQuoteStatus from "./_components/GuestQuoteStatus";
import { GuestMenuSearchField } from "./_components/GuestMenuSearchField";

// Lazy load heavy components

const GuestMenuViews = dynamic(
  () =>
    import("../../../../components/guest/GuestMenuViews").then((mod) => ({
      default: mod.GuestMenuViews,
    })),
  {
    loading: () => (
      <div className="flex justify-center p-8">
        <Spinner size="lg" role="status" />
      </div>
    ),
  },
);

const AiWaiter = dynamic(
  () =>
    import("../../../../components/guest/AiWaiter").then((mod) => ({
      default: mod.AiWaiter,
    })),
  { ssr: false, loading: () => null },
);
import {
  BillWithItemsResponse,
  getTableByCode,
  getOpenBillByTableCode,
  getMenuByTableCode,
  getBusinessByTableCode,
  guestBillRef,
  setBillFiscalCustomerByNumber,
} from "../../../../api/bills";
import {
  emptyFiscalIdentity,
  fiscalIdentityToPayload,
  type FiscalIdentityValue,
} from "@/components/common/FiscalIdentityFields";
import { shouldShowFiscalIdentityFields } from "@/lib/fiscalIdentityAvailability";
import {
  Business,
  MenuCategory,
  MenuItem,
  Offer,
  Bundle,
  BundleItemRef,
} from "../../../../api/business";
import {
  createGuestOrder,
  type Orderability,
  type OrderDraft,
} from "../../../../api/orders";
import { useOrderQuote } from "@/hooks/useOrderQuote";
import { useGuestBillSync } from "@/hooks/useGuestBillSync";
import type { PromoOffer } from "../../../../api/promo";
import { useGuestTranslation } from "../../../../i18n/GuestTranslationProvider";
import { readGuestLocaleCookie } from "../../../../i18n/guestLocaleResolver";
import {
  nextGuestMenuLanguage,
  resolveGuestMenuLanguage,
} from "../../../../i18n/localeRegistry";
import {
  canonicalPendingCartTableCode,
  type PendingCartIntent,
} from "@/components/guest/pendingCartIntent";
import { getRouteParam } from "@/utils/nextRouteParams";
import { formatEntityName } from "@/lib/tableLabel";
import {
  isBusinessClosedFromOrderability,
  isGuestOrderingEnabled,
} from "@/lib/guestBusinessClosed";
import {
  filterGuestSellableBundles,
  filterGuestSellableOffers,
} from "@/lib/guestPromotionAvailability";
import GuestClosedBanner from "@/components/guest/GuestClosedBanner";
import {
  blockedReasonKind,
  type BlockedReasonKind,
} from "@/components/guest/menuItemAvailability";
import CartModal from "./_components/CartModal";
import OrderSuccessModal from "./_components/OrderSuccessModal";
import {
  guestOrderErrorInfo,
  presentGuestOrderError,
  GUEST_ORDER_QUANTITY_CAP,
  GUEST_BUNDLE_QUANTITY_SOFT_MAX,
} from "@/lib/guestOrderErrors";
import {
  buildGuestOrderItems,
  buildCartSignature,
  cartItemContainsBlockedMenuItem,
} from "./_orderSubmission";
import CategoryTabs from "@/components/menu/CategoryTabs";
import type { CartItem, CartMenuItem, StoredCart } from "./_types";
import {
  GuestMenuTableScope,
  menuRouteScopeKey,
  PendingCartAppliedNotice,
  PendingCartRouteConsumer,
  type PendingCartTarget,
  undoPendingCartDelta,
} from "./_pendingCartRoute";

/** Thumbnail that hides itself when the source fails to load. */
function FailsafeImage({
  src,
  alt,
  wrapperClassName,
}: {
  src: string;
  alt: string;
  wrapperClassName: string;
}) {
  const [error, setError] = useState(false);
  if (error) return null;
  return (
    <div className={wrapperClassName}>
      <Image
        src={src}
        alt={alt}
        width={48}
        height={48}
        className="h-full w-full object-cover"
        onError={() => setError(true)}
      />
    </div>
  );
}

const slugifyCategoryName = (name: string, index: number) => {
  const base = (name || "")
    .toLowerCase()
    .normalize("NFKD")
    // Strip combining diacritical marks (U+0300–U+036F).
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
  return base || `category-${index}`;
};

// Matches the public guest table payload (buildPublicGuestTableResponse).
interface Table {
  table_code: string;
  name: string;
  capacity: number;
  is_active: boolean;
}

interface TableData {
  table: Table;
  business: Business;
  menu: {
    categories: string;
    item_orderability?: Record<string, Orderability>;
  };
  categories: MenuCategory[];
  offers?: Offer[];
  bundles?: Bundle[];
}

// Cart storage expiry
const CART_EXPIRY_MS = 60 * 60 * 1000; // 1 hour in milliseconds

// Internal component that uses the hook
function GuestMenuPageContent() {
  const { t, setBusinessId, currentLanguage } = useGuestTranslation();
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  const params = useParams();
  const tableCode = getRouteParam(params, "tableCode");

  const [tableData, setTableData] = useState<TableData | null>(null);
  /** Distinguishes network/5xx load failures from true "table not found". */
  const [tableLoadError, setTableLoadError] = useState<
    "not_found" | "network" | null
  >(null);
  const [currentBill, setCurrentBill] = useState<BillWithItemsResponse | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [cart, setCart] = useState<CartItem[]>([]);
  const [showCart, setShowCart] = useState(false);
  const [appliedPromo, setAppliedPromo] = useState<PromoOffer | null>(null);
  // Fiscal identity (AFIP/ARCA, T22) captured in the cart and persisted to the
  // bill after creation. A ref mirrors the latest valid value so the create
  // callback can read it without widening its dependency list.
  const fiscalIdentityRef = useRef<{
    value: FiscalIdentityValue;
    valid: boolean;
  }>({ value: emptyFiscalIdentity(), valid: true });

  // Cart storage key
  const CART_STORAGE_KEY = useMemo(
    () => `payverge_cart_${tableCode}`,
    [tableCode],
  );
  // The cart-restore effect re-runs on menu load and every language change (it
  // depends on menuItemLookup/t) to reconcile 86'd items — but its toasts must
  // fire at most once, not on every re-run.
  const cartRestoredToastRef = useRef(false);
  const cartRemovedToastRef = useRef(false);
  const cartRestoreStartedRef = useRef<string | null>(null);
  // Bumped on successful Place Order so a stale save/restore cannot revive the
  // pre-submit cart while the bill badge already shows the new check (#76).
  const cartEpochRef = useRef(0);
  const bundleAddLockRef = useRef(false);
  const [addedBundleId, setAddedBundleId] = useState<number | string | null>(
    null,
  );
  const [menuActionStatus, setMenuActionStatus] = useState("");
  const [cartRestoreReadyTable, setCartRestoreReadyTable] = useState("");
  const [menuLocaleReadyFor, setMenuLocaleReadyFor] = useState("");
  const translationGenerationRef = useRef(0);

  const [translatedCategories, setTranslatedCategories] = useState<
    MenuCategory[]
  >([]);

  useEffect(() => {
    translationGenerationRef.current += 1;
    cartRestoreStartedRef.current = null;
    cartRestoredToastRef.current = false;
    cartRemovedToastRef.current = false;
    setCartRestoreReadyTable("");
    setMenuLocaleReadyFor("");
    setTableData(null);
    setTranslatedCategories([]);
    setSelectedLanguage("");
    setBusinessDefaultLanguage("");
    setAvailableOffers([]);
    setAvailableBundles([]);
    setItemOrderability({});
    setCurrentBill(null);
    setCart([]);
  }, [tableCode]);

  const effectiveCategories = useMemo(
    () =>
      translatedCategories.length > 0
        ? translatedCategories
        : tableData?.categories || [],
    [translatedCategories, tableData?.categories],
  );

  const menuItemLookup = useMemo(() => {
    const byName = new Map<
      string,
      { id?: string; category: string; price: number; isAvailable: boolean }
    >();
    const byId = new Map<
      string,
      {
        name: string;
        category: string;
        price: number;
        isAvailable: boolean;
        inventoryStatus?: string;
      }
    >();
    effectiveCategories.forEach((category) => {
      category.items?.forEach((item) => {
        byName.set(item.name.toLowerCase(), {
          id: item.id,
          category: category.name,
          price: item.price || 0,
          isAvailable:
            item.is_available !== false &&
            item.inventory_status !== "out_of_stock",
        });
        if (item.id) {
          byId.set(item.id, {
            name: item.name,
            category: category.name,
            price: item.price || 0,
            isAvailable:
              item.is_available !== false &&
              item.inventory_status !== "out_of_stock",
            // Sticky 86 flag — survives the after-hours business_closed remap
            // in the orderability map, so labels can keep 86 saying 86 (#822).
            inventoryStatus: item.inventory_status,
          });
        }
      });
    });
    return { byName, byId };
  }, [effectiveCategories]);

  // Load cart from localStorage on mount
  useEffect(() => {
    if (
      !tableData ||
      canonicalPendingCartTableCode(tableData.table.table_code) !==
        canonicalPendingCartTableCode(tableCode) ||
      cartRestoreStartedRef.current === tableCode
    ) {
      return;
    }
    cartRestoreStartedRef.current = tableCode;
    try {
      const storedData = localStorage.getItem(CART_STORAGE_KEY);
      if (storedData) {
        const parsedData: StoredCart = JSON.parse(storedData);
        const now = Date.now();

        // Older entries (or a corrupted localStorage write from a third-party
        // ext) could have `items: null` — restoring that as the cart blew up
        // `cart.reduce(...)` downstream and surfaced as "Cannot read
        // properties of null (reading 'reduce')" in ShopErrorBoundary. Coerce
        // to a fresh array if the shape is wrong.
        const restoredItems = Array.isArray(parsedData.items)
          ? parsedData.items
              // G-14: drop corrupt lines (missing name, non-numeric price/qty).
              .filter(
                (item: unknown) =>
                  item &&
                  typeof (item as CartItem).name === "string" &&
                  typeof (item as CartItem).price === "number" &&
                  !isNaN((item as CartItem).price) &&
                  typeof (item as CartItem).quantity === "number" &&
                  !isNaN((item as CartItem).quantity) &&
                  (item as CartItem).quantity > 0,
              )
              // G-4: drop 86'd menu items that were available an hour ago
              // but have since been marked unavailable.
              .filter((item: CartItem) => {
                if (item.itemType !== "menu_item") return true;
                const entry =
                  (item.menuItemId
                    ? menuItemLookup.byId.get(item.menuItemId)
                    : undefined) ||
                  menuItemLookup.byName.get(item.name.toLowerCase());
                return !entry || entry.isAvailable !== false;
              })
          : [];

        // G-4/G-14: surface a toast naming items that were dropped
        // (corrupt or 86'd), so the diner isn't surprised by a
        // shorter cart than before.
        const rawItems = Array.isArray(parsedData.items)
          ? parsedData.items
          : [];
        if (rawItems.length > 0 && restoredItems.length < rawItems.length) {
          const restoredNames = new Set(
            restoredItems.map((i: { name: string }) => i.name),
          );
          const removedNames = [
            ...new Set(
              rawItems
                .filter((i: { name: string }) => !restoredNames.has(i.name))
                .map((i: { name: string }) => i.name),
            ),
          ];
          if (removedNames.length > 0 && !cartRemovedToastRef.current) {
            cartRemovedToastRef.current = true;
            toast.error(
              `${t("menu.cartItemsRemovedUnavailable")} (${removedNames.join(", ")})`,
              { duration: 6000 },
            );
          }
        }

        // Check if cart is expired (older than 1 hour)
        if (
          now - parsedData.timestamp < CART_EXPIRY_MS &&
          parsedData.tableCode === tableCode
        ) {
          setCart(restoredItems);
          // Tell the diner we restored something so a non-empty cart
          // total in the bottom bar doesn't look mysterious. We don't
          // ask for consent — they were on the same table within the
          // hour, so resuming is the right default — but we surface
          // a small toast with a "Clear cart" undo affordance.
          const itemCount = restoredItems.reduce(
            (sum, item) => sum + (item.quantity || 0),
            0,
          );
          if (itemCount > 0 && !cartRestoredToastRef.current) {
            cartRestoredToastRef.current = true;
            toast(
              (tToast) => (
                <span className="flex items-center gap-3">
                  <span>
                    {itemCount === 1
                      ? t("menu.cartRestoredOne")
                      : t("menu.cartRestoredMany", { count: itemCount })}
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      setCart([]);
                      localStorage.removeItem(CART_STORAGE_KEY);
                      toast.dismiss(tToast.id);
                    }}
                    className="font-semibold text-brand-dark hover:underline"
                  >
                    {t("common.clear")}
                  </button>
                </span>
              ),
              { duration: 5000 },
            );
          }
        } else {
          // Cart expired, clear it
          localStorage.removeItem(CART_STORAGE_KEY);
        }
      }
    } catch (error) {
      console.error("Error loading cart from localStorage:", error);
      localStorage.removeItem(CART_STORAGE_KEY);
    } finally {
      setCartRestoreReadyTable(tableCode);
    }
  }, [tableCode, CART_STORAGE_KEY, t, menuItemLookup, tableData]);

  // Save cart to localStorage whenever it changes
  useEffect(() => {
    if (cartRestoreReadyTable !== tableCode) return;
    const epochAtSchedule = cartEpochRef.current;
    const snapshot = cart;
    // Defer so a Place Order clearCart() epoch bump can cancel this write
    // before it revives the pre-submit cart (issue 76).
    const persist = () => {
      if (epochAtSchedule !== cartEpochRef.current) return;
      try {
        if (snapshot.length > 0) {
          const dataToStore: StoredCart = {
            items: snapshot,
            timestamp: Date.now(),
            tableCode: tableCode,
          };
          localStorage.setItem(CART_STORAGE_KEY, JSON.stringify(dataToStore));
        } else {
          localStorage.removeItem(CART_STORAGE_KEY);
        }
      } catch (error) {
        console.error("Error saving cart to localStorage:", error);
      }
    };
    const handle = window.setTimeout(persist, 0);
    return () => window.clearTimeout(handle);
  }, [cart, tableCode, CART_STORAGE_KEY, cartRestoreReadyTable]);

  // Currency settings
  const [businessCurrencies, setBusinessCurrencies] = useState({
    default_currency: "USD",
    display_currency: "USD",
  });
  // G-10: exchange rate from default_currency to display_currency. Cached
  // after the first successful fetch, set to 1 when currencies match. When
  // the fetch fails we format the unconverted amount in default_currency
  // (same fallback as CurrencyPrice — don't mislabel the magnitude).
  const [displayRate, setDisplayRate] = useState<number>(1);
  const [rateFailed, setRateFailed] = useState(false);
  // The language the menu data was authored in. When the saved guest
  // language matches this we can skip the translated-menu fetch entirely.
  const [businessDefaultLanguage, setBusinessDefaultLanguage] =
    useState<string>("");
  const [orderLoading, setOrderLoading] = useState(false);
  const [selectedLanguage, setSelectedLanguage] = useState<string>("");
  const [availableOffers, setAvailableOffers] = useState<Offer[]>([]);
  const [availableBundles, setAvailableBundles] = useState<Bundle[]>([]);
  const [itemOrderability, setItemOrderability] = useState<
    Record<string, Orderability>
  >({});
  const [showOrderSuccess, setShowOrderSuccess] = useState(false);
  const [orderSuccessMessage, setOrderSuccessMessage] = useState("");
  // G-3: ONE submission ref for both submit paths. The key is derived from
  // the cart signature (see buildCartSignature) and survives the
  // create-bill → add-items path switch; it is cleared ONLY on confirmed
  // success (inside submitCartAsOrder).
  const orderSubmissionRef = useRef<{ signature: string; key: string } | null>(
    null,
  );

  // Filter and search states
  const [searchQuery, setSearchQuery] = useState<string>("");
  /** RV-4: collapse brand strip on scroll to reclaim viewport. */
  const [chromeCompact, setChromeCompact] = useState(false);

  useEffect(() => {
    const onScroll = () => {
      setChromeCompact(window.scrollY > 16);
    };
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);
  const [selectedFilters, setSelectedFilters] = useState<Set<string>>(
    new Set(),
  );
  const [activeCategory, setActiveCategory] = useState<number>(0);
  const [showFilters, setShowFilters] = useState(false);

  // View mode states
  type ViewMode = "detailed" | "compact" | "grid" | "category-tabs";
  const [viewMode, setViewMode] = useState<ViewMode>("detailed");

  // Guards against stale parallel responses resolving after the effect has
  // re-run (e.g. user switches tables quickly). Full request cancellation
  // would need AbortSignal wired through the API layer.
  const loadGenerationRef = useRef(0);

  const loadTableData = useCallback(
    async (opts?: { silent?: boolean }) => {
      const generation = ++loadGenerationRef.current;
      const isStillActive = () => loadGenerationRef.current === generation;
      const silent = Boolean(opts?.silent);

      // Full-page skeleton only on the first paint. A silent refresh after Place
      // Order must not unmount cart/success chrome (issue 76).
      if (!silent) {
        setLoading(true);
        setMenuLocaleReadyFor("");
        setTableLoadError(null);
      }
      try {
        // Table/menu is essential. Bill and business must not hold the
        // "Loading menu…" gate if they hang or 404 (#420).
        const loaded = await loadGuestMenuDependencies({
          table: () => getTableByCode(tableCode, currentLanguage),
          bill: () => getOpenBillByTableCode(tableCode),
          business: () => getBusinessByTableCode(tableCode),
          timeoutMs: MENU_DEPENDENCY_TIMEOUT_MS,
        });

        if (!isStillActive()) return;

        const tableResponse = loaded.table;
        // A fulfilled settlement is not proof the table exists — a 200 with a
        // lost body resolves with no `table`. Only a classified outcome may
        // decide what the diner is told (issue 682).
        const tableOutcome = resolveGuestTableSettlement(tableResponse);
        if (tableOutcome.kind === "ready") {
          const tableValue = tableOutcome.value;
          setTableData(tableValue);
          setTableLoadError(null);
          setAvailableOffers((tableValue.offers || []).filter((o) => !o.code));
          setAvailableBundles(tableValue.bundles || []);
          setItemOrderability(tableValue.menu?.item_orderability || {});

          // Set business ID for translation provider
          if (tableValue?.business?.id) {
            setBusinessId(tableValue.business.id);
          }
          if (!silent) setLoading(false);
        } else {
          console.error(
            "Error loading table data:",
            tableResponse.status === "rejected"
              ? tableResponse.reason
              : `empty table payload for ${tableCode}`,
          );
          if (!silent) {
            setTableData(null);
            // True missing table → not_found; timeouts/5xx/offline/empty body →
            // network, so guests are never told their QR is wrong on an
            // unproven miss.
            setTableLoadError(tableOutcome.error);
            setLoading(false);
          }
        }

        const { bill: billResponse, business: businessResponse } =
          await loaded.rest;
        if (!isStillActive()) return;

        // Set business currency settings from the business response
        if (businessResponse.status === "fulfilled") {
          if (businessResponse.value.business) {
            setBusinessCurrencies({
              default_currency:
                businessResponse.value.business.default_currency || "USD", // Prices are stored in this currency (USD)
              display_currency:
                businessResponse.value.business.display_currency || "USD", // Show prices in this currency (AED)
            });
            if (businessResponse.value.business.default_language) {
              setBusinessDefaultLanguage(
                businessResponse.value.business.default_language,
              );
            }
          }
        } else {
          console.error(
            "Error loading business data:",
            businessResponse.reason,
          );
        }

        if (billResponse.status === "fulfilled") {
          // No-active-bill now resolves to { bill: null } (H3) — coerce to a
          // falsy currentBill so downstream `!!currentBill` checks stay honest.
          setCurrentBill(billResponse.value.bill ? billResponse.value : null);
        } else {
          setCurrentBill(null);
        }
      } catch (error) {
        if (!isStillActive()) return;
        console.error("Error loading data:", error);
      } finally {
        if (isStillActive() && !silent) {
          setLoading(false);
        }
      }
    },
    [tableCode, setBusinessId, currentLanguage],
  );

  // G-10: prefetch the exchange rate so filter thresholds (Under/Over)
  // convert the amount from the business's default pricing currency to
  // whatever the guest sees. Falls back to default-currency formatting
  // (same contract as CurrencyPrice — don't mislabel the magnitude).
  useEffect(() => {
    let cancelled = false;
    const { default_currency, display_currency } = businessCurrencies;
    if (default_currency === display_currency) {
      setDisplayRate(1);
      setRateFailed(false);
      return;
    }
    const loadRate = async () => {
      try {
        const result = await convertAmount(
          1,
          default_currency,
          display_currency,
        );
        if (!cancelled) {
          setDisplayRate(result.converted_amount);
          setRateFailed(false);
        }
      } catch {
        if (!cancelled) setRateFailed(true);
      }
    };
    loadRate();
    return () => {
      cancelled = true;
    };
  }, [
    businessCurrencies,
    businessCurrencies.default_currency,
    businessCurrencies.display_currency,
  ]);

  const loadTranslatedMenu = useCallback(
    async (languageCode: string) => {
      const generation = ++translationGenerationRef.current;
      const requestedTable = tableCode;
      setMenuLocaleReadyFor("");
      try {
        const menuData = await getMenuByTableCode(tableCode, languageCode);
        if (translationGenerationRef.current !== generation) return;

        // Use translated categories if available, otherwise fall back to original
        const categories = menuData.parsed_categories || menuData.categories;

        setTranslatedCategories(categories || []);
        if (menuData.offers) {
          setAvailableOffers(menuData.offers.filter((o: Offer) => !o.code));
        }
        if (menuData.bundles) {
          setAvailableBundles(menuData.bundles);
        }
        if (menuData.menu?.item_orderability) {
          setItemOrderability(menuData.menu.item_orderability);
        }
      } catch (error) {
        if (translationGenerationRef.current !== generation) return;
        if (tableData?.categories) {
          setTranslatedCategories(tableData.categories);
        }
      } finally {
        if (translationGenerationRef.current === generation) {
          setMenuLocaleReadyFor(
            `${canonicalPendingCartTableCode(requestedTable) ?? ""}:${languageCode}`,
          );
        }
      }
    },
    [tableCode, tableData?.categories],
  );

  const handleLanguageChange = useCallback(
    (languageCode: string) => {
      setSelectedLanguage(languageCode);
      loadTranslatedMenu(languageCode);
    },
    [loadTranslatedMenu],
  );

  // Initialize language and menu data when table data loads
  useEffect(() => {
    if (tableData?.business?.id && tableData?.categories) {
      const preferredLanguage = resolveGuestMenuLanguage({
        currentLanguage,
        saved:
          localStorage.getItem(`guest-language-${tableData.business.id}`) ??
          readGuestLocaleCookie(),
        businessDefault: businessDefaultLanguage,
      });

      if (preferredLanguage && !selectedLanguage) {
        setSelectedLanguage(preferredLanguage);
        // Skip the second fetch when the guest's language is the same one
        // the menu was authored in — `tableData.categories` already contains
        // that copy. Without this guard, opening the menu fires both
        // /tables/<code>/menu and /tables/<code>/menu?lang=<default>
        // back-to-back even though they return identical data.
        if (preferredLanguage !== businessDefaultLanguage) {
          loadTranslatedMenu(preferredLanguage);
        } else {
          setTranslatedCategories(tableData.categories);
          setMenuLocaleReadyFor(
            `${canonicalPendingCartTableCode(tableCode) ?? ""}:${preferredLanguage}`,
          );
        }
      }
    }
  }, [
    tableData?.business?.id,
    tableData?.categories,
    selectedLanguage,
    loadTranslatedMenu,
    businessDefaultLanguage,
    currentLanguage,
    tableCode,
  ]);

  // Re-translate the menu when the guest language changes. Follow the live
  // provider locale (URL / picker) so a later es-AR pick is actually fetched
  // even if `guestLanguageChange` fired before businessId was attached.
  useEffect(() => {
    if (!tableData?.categories) return;
    const next = nextGuestMenuLanguage(currentLanguage, selectedLanguage);
    if (!next) return;
    handleLanguageChange(next);
  }, [
    currentLanguage,
    selectedLanguage,
    tableData?.categories,
    handleLanguageChange,
  ]);

  useEffect(() => {
    const handleGuestLanguageChange = (event: CustomEvent) => {
      const { language, businessId } = event.detail;
      if (businessId != null && businessId !== tableData?.business?.id) {
        return;
      }
      handleLanguageChange(language);
    };

    window.addEventListener(
      "guestLanguageChange",
      handleGuestLanguageChange as EventListener,
    );

    return () => {
      window.removeEventListener(
        "guestLanguageChange",
        handleGuestLanguageChange as EventListener,
      );
    };
  }, [handleLanguageChange, tableData?.business?.id]);

  // Filter functions

  // Currency-aware price bucket boundaries.
  //
  // The old filter hardcoded 10 / 20 (dollar) thresholds, so on JPY / ARS / COP
  // menus — where an entrée is routinely 2,000+ — every item landed in the
  // "over-20" bucket and the price filter was useless. Instead we derive the two
  // boundaries from the menu's ACTUAL price distribution (terciles of the sorted
  // item prices), so the buckets split the menu into roughly-equal cheap / mid /
  // premium thirds regardless of currency magnitude. The two boundary amounts
  // flow straight into `formatPrice(...)` for the labels, so they read correctly
  // per currency. Prices here are the raw menu (default-currency) values — the
  // same `item.price` the filter predicates compare against.
  const priceBuckets = useMemo(() => {
    const prices: number[] = [];
    translatedCategories.forEach((category) => {
      category.items?.forEach((item) => {
        if (typeof item.price === "number" && item.price > 0) {
          prices.push(item.price);
        }
      });
    });
    if (prices.length === 0) return null;
    prices.sort((a, b) => a - b);
    const at = (fraction: number) =>
      prices[Math.min(prices.length - 1, Math.floor(prices.length * fraction))];
    let low = at(1 / 3);
    let high = at(2 / 3);
    // Degenerate distributions (few distinct prices) can collapse low === high;
    // nudge high above low so the mid bucket stays non-empty when possible.
    if (high <= low) {
      const maxPrice = prices[prices.length - 1];
      high = maxPrice > low ? maxPrice : low;
    }
    return { low, high };
  }, [translatedCategories]);

  // Classify a price into one of the three opaque bucket ids using the derived
  // boundaries. Kept as a single source of truth so `availableFilters`, the
  // menu-item filter, and the bundle filter all agree on where the cuts fall.
  const priceBucketFor = useCallback(
    (price: number | undefined | null): string | null => {
      if (!priceBuckets || typeof price !== "number" || price <= 0) return null;
      if (price < priceBuckets.low) return "under-10";
      if (price < priceBuckets.high) return "10-20";
      return "over-20";
    },
    [priceBuckets],
  );

  const availableFilters = useMemo(() => {
    const filters = new Set<string>();
    translatedCategories.forEach((category) => {
      category.items?.forEach((item) => {
        if (item.is_available) filters.add("available");
        if (!item.is_available) filters.add("unavailable");
        const bucket = priceBucketFor(item.price);
        if (bucket) filters.add(bucket);
        if (item.dietary_tags?.includes("vegetarian"))
          filters.add("vegetarian");
        if (item.dietary_tags?.includes("vegan")) filters.add("vegan");
        if (item.dietary_tags?.includes("gluten-free"))
          filters.add("gluten-free");
        if (item.dietary_tags?.includes("dairy-free"))
          filters.add("dairy-free");
        if (item.dietary_tags?.includes("nut-free")) filters.add("nut-free");
      });
    });
    return Array.from(filters);
  }, [translatedCategories, priceBucketFor]);

  const filteredCategories = useMemo(() => {
    if (!searchQuery && selectedFilters.size === 0) {
      return translatedCategories;
    }

    return translatedCategories
      .map((category) => ({
        ...category,
        items:
          category.items?.filter((item) => {
            // Search filter
            const matchesSearch =
              !searchQuery ||
              item.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
              item.description
                ?.toLowerCase()
                .includes(searchQuery.toLowerCase());

            // Availability/price filters (OR within the group)
            const availabilityFilters = Array.from(selectedFilters).filter(
              (f) => ["available", "unavailable"].includes(f),
            );
            const priceFilters = Array.from(selectedFilters).filter((f) =>
              ["under-10", "10-20", "over-20"].includes(f),
            );
            const dietaryFilters = Array.from(selectedFilters).filter((f) =>
              [
                "vegetarian",
                "vegan",
                "gluten-free",
                "dairy-free",
                "nut-free",
              ].includes(f),
            );

            const matchesAvailability =
              availabilityFilters.length === 0 ||
              availabilityFilters.some((f) => {
                switch (f) {
                  case "available":
                    return (
                      item.is_available &&
                      item.inventory_status !== "out_of_stock"
                    );
                  case "unavailable":
                    return (
                      !item.is_available ||
                      item.inventory_status === "out_of_stock"
                    );
                  default:
                    return false;
                }
              });

            const matchesPrice =
              priceFilters.length === 0 ||
              priceFilters.includes(priceBucketFor(item.price) ?? "");

            const matchesDietary =
              dietaryFilters.length === 0 ||
              dietaryFilters.some((f) => {
                switch (f) {
                  case "vegetarian":
                    return item.dietary_tags?.includes("vegetarian");
                  case "vegan":
                    return item.dietary_tags?.includes("vegan");
                  case "gluten-free":
                    return item.dietary_tags?.includes("gluten-free");
                  case "dairy-free":
                    return item.dietary_tags?.includes("dairy-free");
                  case "nut-free":
                    return item.dietary_tags?.includes("nut-free");
                  default:
                    return false;
                }
              });

            return (
              matchesSearch &&
              matchesAvailability &&
              matchesPrice &&
              matchesDietary
            );
          }) || [],
      }))
      .filter((category) => category.items && category.items.length > 0);
  }, [translatedCategories, searchQuery, selectedFilters, priceBucketFor]);

  const toggleFilter = (filter: string) => {
    setSelectedFilters((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(filter)) {
        newSet.delete(filter);
      } else {
        newSet.add(filter);
      }
      return newSet;
    });
  };

  const clearAllFilters = () => {
    setSearchQuery("");
    setSelectedFilters(new Set());
  };

  const formatPrice = (amount: number) => {
    const converted = rateFailed ? amount : amount * displayRate;
    return new Intl.NumberFormat(normalizeGuestLocale(currentLanguage), {
      style: "currency",
      currency: rateFailed
        ? businessCurrencies.default_currency
        : businessCurrencies.display_currency,
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(converted);
  };

  // Convert default→display before labeling, matching CurrencyPrice / nav.
  const fmtCurrency = (amount: number) =>
    formatConvertedGuestCurrency(
      amount,
      businessCurrencies.default_currency,
      businessCurrencies.display_currency,
      currentLanguage,
      rateFailed ? null : displayRate,
    );

  const getFilterLabel = (filter: string) => {
    switch (filter) {
      case "available":
        return t("menu.filters.available") || "Available";
      case "unavailable":
        return t("menu.filters.unavailable") || "Unavailable";
      case "under-10": {
        // Boundaries are derived from the menu's real price distribution
        // (see `priceBuckets`) and rendered through `formatPrice` so they read
        // correctly per currency — not the old hardcoded $10 / $20.
        const low = priceBuckets?.low ?? 0;
        return (
          t("menu.filters.underPrice", { price: formatPrice(low) }) ||
          `Under ${formatPrice(low)}`
        );
      }
      case "10-20": {
        const low = priceBuckets?.low ?? 0;
        const high = priceBuckets?.high ?? 0;
        return (
          t("menu.filters.priceRange", {
            min: formatPrice(low),
            max: formatPrice(high),
          }) || `${formatPrice(low)} – ${formatPrice(high)}`
        );
      }
      case "over-20": {
        const high = priceBuckets?.high ?? 0;
        return (
          t("menu.filters.overPrice", { price: formatPrice(high) }) ||
          `Over ${formatPrice(high)}`
        );
      }
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
  };

  const parseBundleItems = useCallback((bundle: Bundle): BundleItemRef[] => {
    if (!bundle.items) return [];
    if (Array.isArray(bundle.items)) return bundle.items;
    try {
      const parsed = JSON.parse(bundle.items);
      if (Array.isArray(parsed)) {
        if (typeof parsed[0] === "string") {
          return parsed.map((id: string) => ({
            menu_item_id: id,
            quantity: 1,
          }));
        }
        return parsed.map((item: any) => ({
          menu_item_id: item.menu_item_id || item.id || "",
          name: item.name,
          quantity: item.quantity || 1,
        }));
      }
    } catch {
      return [];
    }
    return [];
  }, []);

  const getBundleRegularTotal = useCallback(
    (bundle: Bundle) => {
      const refs = parseBundleItems(bundle);
      return refs.reduce((sum, ref) => {
        const resolved = menuItemLookup.byId.get(ref.menu_item_id);
        const unitPrice = resolved?.price || 0;
        return sum + unitPrice * (ref.quantity || 1);
      }, 0);
    },
    [menuItemLookup, parseBundleItems],
  );

  const promotionCatalog = useMemo(
    () => ({
      categories: effectiveCategories,
      orderability: itemOrderability,
      bundles: availableBundles,
    }),
    [effectiveCategories, itemOrderability, availableBundles],
  );

  // Hide item/bundle-targeted promotions whose target cannot be sold (issue 345).
  const sellableOffers = useMemo(
    () => filterGuestSellableOffers(availableOffers, promotionCatalog),
    [availableOffers, promotionCatalog],
  );
  const sellableBundles = useMemo(
    () => filterGuestSellableBundles(availableBundles, promotionCatalog),
    [availableBundles, promotionCatalog],
  );

  const filteredBundles = useMemo(() => {
    if (!searchQuery && selectedFilters.size === 0) return sellableBundles;
    const q = searchQuery.toLowerCase();
    return sellableBundles.filter((bundle) => {
      // Name / description match (same logic as menu items)
      const matchesSearch =
        !q ||
        bundle.name.toLowerCase().includes(q) ||
        bundle.description?.toLowerCase().includes(q) ||
        // Also match if any sub-item name matches
        (() => {
          const refs = Array.isArray(bundle.items)
            ? bundle.items
            : (() => {
                try {
                  const p = JSON.parse(bundle.items as string);
                  return Array.isArray(p) ? p : [];
                } catch {
                  return [];
                }
              })();
          return refs.some((ref: BundleItemRef | string) => {
            const name =
              typeof ref === "string"
                ? menuItemLookup.byId.get(ref)?.name
                : ref.name || menuItemLookup.byId.get(ref.menu_item_id)?.name;
            return name?.toLowerCase().includes(q);
          });
        })();
      if (!matchesSearch) return false;
      if (selectedFilters.size === 0) return true;

      // Mirror the menu-item filter: AND the availability / price / dietary
      // groups (previously the bundle strip ORed everything and dropped dietary
      // filters entirely via a `default: true`, so a vegan filter still showed
      // meat bundles).
      const filters = Array.from(selectedFilters);
      const availabilityFilters = filters.filter((f) =>
        ["available", "unavailable"].includes(f),
      );
      const priceFilters = filters.filter((f) =>
        ["under-10", "10-20", "over-20"].includes(f),
      );
      const dietaryFilters = filters.filter((f) =>
        [
          "vegetarian",
          "vegan",
          "gluten-free",
          "dairy-free",
          "nut-free",
        ].includes(f),
      );

      const matchesAvailability =
        availabilityFilters.length === 0 ||
        availabilityFilters.some((f) =>
          f === "available" ? bundle.is_active : !bundle.is_active,
        );

      const matchesPrice =
        priceFilters.length === 0 ||
        priceFilters.includes(priceBucketFor(bundle.price) ?? "");

      // A bundle satisfies a dietary filter only if EVERY resolved sub-item
      // carries that tag — a bundle is vegan only if all its components are.
      const subItems = parseBundleItems(bundle)
        .map((ref) =>
          typeof ref === "string"
            ? menuItemLookup.byId.get(ref)
            : menuItemLookup.byId.get(ref.menu_item_id),
        )
        .filter(Boolean) as { dietary_tags?: string[] }[];
      const matchesDietary =
        dietaryFilters.length === 0 ||
        dietaryFilters.some(
          (f) =>
            subItems.length > 0 &&
            subItems.every((si) => si.dietary_tags?.includes(f)),
        );

      return matchesAvailability && matchesPrice && matchesDietary;
    });
  }, [
    sellableBundles,
    searchQuery,
    selectedFilters,
    menuItemLookup,
    parseBundleItems,
    priceBucketFor,
  ]);

  const filteredOffers = useMemo(() => {
    if (!searchQuery && selectedFilters.size === 0) return sellableOffers;
    const q = searchQuery.toLowerCase();
    return sellableOffers.filter((offer) => {
      const matchesSearch =
        !q ||
        offer.name.toLowerCase().includes(q) ||
        offer.description?.toLowerCase().includes(q);
      return matchesSearch;
    });
  }, [sellableOffers, searchQuery, selectedFilters]);

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
      if (scope === "item") {
        return itemId === target || item.name === target;
      }
      if (scope === "category") {
        return categoryName === target;
      }
      return false;
    },
    [],
  );

  // Bundles are surfaced only in the curated "Active Offers + Bundles" strip
  // at the top of the menu page (see the `availableOffers.length > 0 ||
  // availableBundles.length > 0` section below). They are NOT injected as a
  // synthetic top-level category here — doing so duplicated the bundle cards
  // both at the top of the page and again inside the categories grid.
  const menuDisplayCategories = useMemo(() => {
    return filteredCategories.map((category) => ({
      ...category,
      items: (category.items || []).map((item) => ({
        ...item,
        item_type: "menu_item" as const,
        menu_item_id: item.id || item.name,
        promotion_offers: sellableOffers.filter((offer) =>
          offerMatchesMenuItem(
            offer,
            { id: item.id, name: item.name },
            category.name,
          ),
        ),
      })),
    }));
  }, [filteredCategories, sellableOffers, offerMatchesMenuItem]);

  useEffect(() => {
    if (activeCategory >= menuDisplayCategories.length) {
      setActiveCategory(0);
    }
  }, [activeCategory, menuDisplayCategories.length]);

  // Scroll detection for active category.
  //
  // The previous implementation ran a `scroll`-event handler that called
  // `querySelectorAll` + read `offsetTop` on every category heading on EVERY
  // scroll tick — a forced synchronous reflow per event that thrashed layout on
  // long menus. We replace it with an IntersectionObserver keyed off the same
  // `[id^="category-"]` heading elements: the browser reports intersection
  // asynchronously off the main scroll path, so there is no per-scroll reflow.
  //
  // `rootMargin`'s negative top inset (matching the ~200px sticky-header offset
  // the old handler added to `scrollY`) shifts the "trip line" down to just
  // below the sticky header, so the active category flips exactly when a heading
  // crosses under the header — preserving the original UX. We track the set of
  // currently-intersecting sections and pick the topmost (smallest index); when
  // nothing is intersecting we keep whichever heading was last scrolled past.
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") return;

    // RV-4: keep in sync with GUEST_MENU_HEADER_OFFSET_PX (collapsed chrome).
    const HEADER_OFFSET = GUEST_MENU_HEADER_OFFSET_PX;
    const intersecting = new Set<number>();
    let lastPassed = 0;

    const indexOf = (el: Element): number => {
      const id = el.id || "";
      const raw = id.slice("category-".length);
      const parsed = Number.parseInt(raw, 10);
      return Number.isNaN(parsed) ? 0 : parsed;
    };

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const idx = indexOf(entry.target);
          if (entry.isIntersecting) {
            intersecting.add(idx);
            lastPassed = idx;
          } else {
            intersecting.delete(idx);
          }
        }
        // Topmost section still crossing the trip line wins; if the viewport
        // sits between headings (none intersecting) keep the last one passed.
        const next =
          intersecting.size > 0 ? Math.min(...intersecting) : lastPassed;
        setActiveCategory(next);
      },
      {
        // Shrink the viewport from the top by the header offset and leave a
        // slim active band just beneath it (bottom inset keeps only the
        // heading nearest the header "active" rather than every visible one).
        rootMargin: `-${HEADER_OFFSET}px 0px -70% 0px`,
        threshold: 0,
      },
    );

    const elements = Array.from(
      document.querySelectorAll<HTMLElement>('[id^="category-"]'),
    );
    elements.forEach((el) => observer.observe(el));

    return () => observer.disconnect();
    // Re-observe whenever the rendered category set changes (search/filter
    // toggles add/remove heading nodes) so the observer never tracks stale DOM.
  }, [menuDisplayCategories]);

  // Memoize business data to prevent unnecessary re-renders
  const businessData = useMemo(
    () => tableData?.business,
    [tableData?.business],
  );

  // Hours closed from orderability; kitchen toggle is separate for banner priority.
  const isBusinessClosed = useMemo(
    () => isBusinessClosedFromOrderability(itemOrderability),
    [itemOrderability],
  );
  const kitchenOrdersOn = Boolean(
    businessData?.kitchen_enabled && businessData?.orders_enabled,
  );
  // Compose closed hours so cart / add / AI order mode stay off after hours.
  // Bill payment is never gated on this flag.
  const isOrderingEnabled = useMemo(
    () =>
      isGuestOrderingEnabled({
        kitchenEnabled: businessData?.kitchen_enabled,
        ordersEnabled: businessData?.orders_enabled,
        businessClosed: isBusinessClosed,
      }),
    [
      businessData?.kitchen_enabled,
      businessData?.orders_enabled,
      isBusinessClosed,
    ],
  );

  const showOrderingDisabledToast = useCallback(() => {
    toast.error(t("menu.orderingDisabled"));
  }, [t]);

  const addToCart = useCallback(
    (
      itemName: string,
      price: number,
      quantity: number = 1,
      specialRequests?: string,
      addOns?: Array<{ id?: string; name: string; price: number }>,
      metadata?: {
        itemType?: "menu_item" | "bundle" | "bundle_item" | "discount";
        menuItemId?: string;
        bundleId?: number;
        parentBundleId?: number;
        sourceOfferId?: number;
      },
    ) => {
      if (!isOrderingEnabled) {
        showOrderingDisabledToast();
        return false;
      }

      // G-4: refuse to add an 86'd item to the cart.
      const menuEntry =
        (metadata?.menuItemId
          ? menuItemLookup.byId.get(metadata.menuItemId)
          : undefined) || menuItemLookup.byName.get(itemName.toLowerCase());
      if (menuEntry && menuEntry.isAvailable === false) {
        toast.error(t("menu.orderErrorItemUnavailable", { name: itemName }));
        return false;
      }
      const decision = metadata?.menuItemId
        ? itemOrderability[metadata.menuItemId]
        : undefined;
      if (decision && !decision.orderable) {
        // #822: name the real reason — closed hours are not an 86.
        toast.error(
          decision.state === "business_closed"
            ? t("menu.businessClosed")
            : decision.state === "ordering_disabled"
              ? t("menu.orderingDisabled")
              : t("menu.orderErrorItemUnavailable", { name: itemName }),
        );
        return false;
      }

      // G-13: cap line quantity. Bundles use a lower soft max so phone taps
      // cannot accidentally order a table-full of Date Night (#80).
      const isBundleLine =
        metadata?.itemType === "bundle" || metadata?.bundleId != null;
      const qtyCap = isBundleLine
        ? GUEST_BUNDLE_QUANTITY_SOFT_MAX
        : GUEST_ORDER_QUANTITY_CAP;
      if (quantity > qtyCap) {
        toast.error(
          isBundleLine
            ? t("menu.bundles.softMax", { max: qtyCap })
            : t("menu.orderErrorQuantityExceeded", { max: qtyCap }),
        );
        return false;
      }

      let applied = true;
      setCart((prevCart) => {
        const resolvedItemType =
          metadata?.itemType || (metadata?.bundleId ? "bundle" : "menu_item");

        // Build a typed CartItem based on the resolved item type
        const buildCartItem = (): CartItem => {
          const base = { name: itemName, price, quantity, specialRequests };
          if (resolvedItemType === "bundle" && metadata?.bundleId != null) {
            return {
              ...base,
              itemType: "bundle",
              bundleId: metadata.bundleId,
              menuItemId: metadata?.menuItemId,
              sourceOfferId: metadata?.sourceOfferId,
            };
          }
          if (
            resolvedItemType === "bundle_item" &&
            metadata?.parentBundleId != null
          ) {
            return {
              ...base,
              itemType: "bundle_item",
              menuItemId:
                metadata?.menuItemId ??
                menuItemLookup.byName.get(itemName.toLowerCase())?.id ??
                itemName,
              parentBundleId: metadata.parentBundleId,
            };
          }
          if (
            resolvedItemType === "discount" &&
            metadata?.sourceOfferId != null
          ) {
            return {
              ...base,
              itemType: "discount",
              sourceOfferId: metadata.sourceOfferId,
            };
          }
          // Default: menu_item
          return {
            ...base,
            itemType: "menu_item",
            menuItemId:
              metadata?.menuItemId ??
              menuItemLookup.byName.get(itemName.toLowerCase())?.id ??
              itemName,
            addOns,
          };
        };

        // Create a unique identifier that includes add-ons
        const addOnsString = addOns
          ? addOns
              .map((addon) => addon.name)
              .sort()
              .join(",")
          : "";

        const isSameItem = (item: CartItem) => {
          if (item.name !== itemName) return false;
          if (item.price !== price) return false;
          if (item.itemType !== resolvedItemType) return false;
          if (item.specialRequests !== specialRequests) return false;
          const itemMenuItemId =
            "menuItemId" in item ? item.menuItemId : undefined;
          const resolvedMenuItemId =
            metadata?.menuItemId ??
            menuItemLookup.byName.get(itemName.toLowerCase())?.id;
          if (itemMenuItemId !== resolvedMenuItemId) return false;
          if (resolvedItemType === "bundle") {
            const b = item as CartItem & { itemType: "bundle" };
            if (b.bundleId !== metadata?.bundleId) return false;
            if (b.sourceOfferId !== metadata?.sourceOfferId) return false;
          }
          if (resolvedItemType === "bundle_item") {
            const bi = item as CartItem & { itemType: "bundle_item" };
            if (bi.parentBundleId !== metadata?.parentBundleId) return false;
          }
          const itemAddOns =
            "addOns" in item ? (item as CartMenuItem).addOns : undefined;
          const itemAddOnsString = itemAddOns
            ? itemAddOns
                .map((addon) => addon.name)
                .sort()
                .join(",")
            : "";
          return itemAddOnsString === addOnsString;
        };

        const existingItem = prevCart.find(isSameItem);

        if (existingItem) {
          const nextQty = existingItem.quantity + quantity;
          if (nextQty > qtyCap) {
            applied = false;
            toast.error(
              isBundleLine
                ? t("menu.bundles.softMax", { max: qtyCap })
                : t("menu.orderErrorQuantityExceeded", { max: qtyCap }),
            );
            return prevCart;
          }
          return prevCart.map((item) =>
            isSameItem(item) ? { ...item, quantity: nextQty } : item,
          );
        } else {
          return [...prevCart, buildCartItem()];
        }
      });
      return applied;
    },
    [
      isOrderingEnabled,
      itemOrderability,
      menuItemLookup,
      showOrderingDisabledToast,
      t,
    ],
  );

  const applyPendingCartTarget = useCallback(
    (target: PendingCartTarget, intent: PendingCartIntent): boolean => {
      return addToCart(
        target.itemName,
        target.price,
        intent.quantity,
        intent.notes,
        undefined,
        target.itemType === "bundle"
          ? { itemType: "bundle", bundleId: target.bundleId }
          : { itemType: "menu_item", menuItemId: target.menuItemId },
      );
    },
    [addToCart],
  );

  const undoPendingCartTarget = useCallback(
    (target: PendingCartTarget, intent: PendingCartIntent) => {
      setCart((current) => undoPendingCartDelta(current, target, intent));
    },
    [],
  );

  const showPendingCartApplied = useCallback(
    (
      target: PendingCartTarget,
      intent: PendingCartIntent,
      undo: () => void,
    ) => {
      toast(
        (tToast) => (
          <PendingCartAppliedNotice
            message={t("aiWaiter.addedToast", {
              qty: intent.quantity,
              name: target.itemName,
            })}
            undoLabel={t("bill.undoRedemption")}
            onUndo={() => {
              undo();
              toast.dismiss(tToast.id);
            }}
          />
        ),
        { duration: 5000 },
      );
    },
    [t],
  );

  const showPendingCartRejected = useCallback(
    (
      reason:
        | "storage"
        | "scope"
        | "expired"
        | "unavailable"
        | "quantity"
        | "mutation",
    ) => {
      if (reason === "quantity") {
        toast.error(
          t("menu.orderErrorQuantityExceeded", {
            max: GUEST_ORDER_QUANTITY_CAP,
          }),
        );
        return;
      }
      toast.error(
        reason === "unavailable" || reason === "expired"
          ? t("errors.menuUnavailableDescription")
          : t("aiWaiter.errConnect"),
      );
    },
    [t],
  );

  const addBundleToCart = useCallback(
    (bundle: Bundle) => {
      if (!bundle.id) return;
      // Debounce rapid taps so fat-finger "Add Bundle" cannot jump to 4× (#80).
      if (bundleAddLockRef.current) return;
      bundleAddLockRef.current = true;
      window.setTimeout(() => {
        bundleAddLockRef.current = false;
      }, 600);

      const blockedChild = parseBundleItems(bundle).find(
        (ref) => itemOrderability[ref.menu_item_id]?.orderable === false,
      );
      if (blockedChild) {
        // #822: closed hours / ordering-off are venue reasons, not an 86.
        const blockedState = itemOrderability[blockedChild.menu_item_id]?.state;
        toast.error(
          blockedState === "business_closed"
            ? t("menu.businessClosed")
            : blockedState === "ordering_disabled"
              ? t("menu.orderingDisabled")
              : t("menu.orderErrorItemUnavailable", {
                  name: blockedChild.name || bundle.name,
                }),
        );
        return;
      }

      const existingQty = cart.reduce((sum, item) => {
        if (item.itemType !== "bundle") return sum;
        const b = item as CartItem & { itemType: "bundle" };
        return b.bundleId === bundle.id ? sum + (item.quantity || 0) : sum;
      }, 0);
      if (existingQty >= GUEST_BUNDLE_QUANTITY_SOFT_MAX) {
        toast.error(
          t("menu.bundles.softMax", { max: GUEST_BUNDLE_QUANTITY_SOFT_MAX }),
        );
        return;
      }

      const applied = addToCart(bundle.name, bundle.price, 1, "", [], {
        itemType: "bundle",
        bundleId: bundle.id,
        menuItemId: `bundle:${bundle.id}`,
      });
      if (applied) {
        const confirmation = t("menu.bundles.added", {
          name: bundle.name,
          quantity: 1,
        });
        setAddedBundleId(bundle.id);
        setMenuActionStatus(confirmation);
        toast.success(confirmation);
        window.setTimeout(() => {
          setAddedBundleId((current) =>
            current === bundle.id ? null : current,
          );
        }, 2000);
      }
    },
    [addToCart, cart, itemOrderability, parseBundleItems, t],
  );

  const updateCartItemQuantity = useCallback(
    (index: number, newQuantity: number) => {
      // G-13: enforce the per-item cap in the +/- stepper too (addToCart
      // stops the initial add, this stops the UI from incrementing past it).
      setCart((prevCart) => {
        const target = prevCart[index];
        if (!target) return prevCart;
        const cap =
          target.itemType === "bundle"
            ? GUEST_BUNDLE_QUANTITY_SOFT_MAX
            : GUEST_ORDER_QUANTITY_CAP;
        if (newQuantity > cap) {
          toast.error(
            target.itemType === "bundle"
              ? t("menu.bundles.softMax", { max: cap })
              : t("menu.orderErrorQuantityExceeded", { max: cap }),
          );
          return prevCart;
        }
        if (newQuantity <= 0) {
          return prevCart.filter((_, i) => i !== index);
        }
        return prevCart.map((item, i) =>
          i === index ? { ...item, quantity: newQuantity } : item,
        );
      });
      // Cart will be automatically saved by useEffect
    },
    [t],
  );

  const clearCart = useCallback(() => {
    cartEpochRef.current += 1;
    setCart([]);
    try {
      localStorage.removeItem(CART_STORAGE_KEY);
    } catch {
      // ignore quota / private-mode failures
    }
  }, [CART_STORAGE_KEY]);

  // The browser may show the undiscounted cart subtotal while the quote is in
  // flight, but only the server is allowed to calculate promotions.
  const neutralCartSubtotal = useMemo(() => {
    return cart.reduce((sum, item) => {
      const addOnsTotal =
        item.itemType === "menu_item" && item.addOns
          ? item.addOns.reduce((sum, addon) => sum + addon.price, 0)
          : 0;
      return sum + (item.price + addOnsTotal) * item.quantity;
    }, 0);
  }, [cart]);

  const quoteDraft = useMemo<OrderDraft>(
    () => ({
      bill_id: currentBill?.bill.id,
      items: buildGuestOrderItems(
        cart,
        (lowerName) => menuItemLookup.byName.get(lowerName)?.id,
      ),
      promo_code: appliedPromo?.code || undefined,
    }),
    [appliedPromo, cart, currentBill?.bill.id, menuItemLookup],
  );
  const {
    quote: serverQuote,
    isPending: quotePending,
    error: quoteError,
    isValid: quoteValid,
    blockedLines: quoteBlockedLines,
  } = useOrderQuote({
    tableCode,
    draft: quoteDraft,
    enabled: isOrderingEnabled && cart.length > 0,
    immediate: showCart,
  });
  // #822: blocked-quote copy names the real reason. Closed hours say closed,
  // ordering-off says ordering off; only item-level blocks say unavailable.
  const quoteBlockedMessage = useMemo(() => {
    const kind: BlockedReasonKind | null = blockedReasonKind(
      quoteBlockedLines.map((line) => ({
        orderabilityState: line.orderability?.state,
        // line.key is the menu_item_id (backend quote lines key by it); the
        // catalog's sticky inventoryStatus lets an 86'd item outrank the
        // after-hours business_closed remap so cart copy matches the tile.
        inventoryStatus: menuItemLookup.byId.get(line.key)?.inventoryStatus,
      })),
    );
    if (kind === "out_of_stock") return t("menu.outOfStock");
    if (kind === "business_closed") return t("menu.businessClosed");
    if (kind === "ordering_disabled") return t("menu.orderingDisabled");
    return t("menu.orderErrorItemUnavailableGeneric");
  }, [menuItemLookup, quoteBlockedLines, t]);
  const promotionPreview = useMemo(() => {
    if (!serverQuote)
      return {
        baseSubtotal: neutralCartSubtotal,
        discountTotal: 0,
        autoDiscountTotal: 0,
        promoDiscount: 0,
        discountedSubtotal: neutralCartSubtotal,
        netSubtotal: neutralCartSubtotal,
        tax: 0,
        serviceFee: 0,
        tip: 0,
        finalTotal: neutralCartSubtotal,
        applied: [],
      };
    const discount = Number(serverQuote.discount);
    return {
      baseSubtotal: Number(serverQuote.subtotal),
      discountTotal: discount,
      autoDiscountTotal: appliedPromo ? 0 : discount,
      promoDiscount: appliedPromo ? discount : 0,
      discountedSubtotal: Number(serverQuote.net_subtotal),
      netSubtotal: Number(serverQuote.net_subtotal),
      tax: Number(serverQuote.tax),
      serviceFee: Number(serverQuote.service_fee),
      tip: Number(serverQuote.tip),
      finalTotal: Number(serverQuote.total),
      applied: [],
    };
  }, [appliedPromo, neutralCartSubtotal, serverQuote]);

  const cartItemCount = useMemo(() => {
    return cart.reduce((total, item) => total + item.quantity, 0);
  }, [cart]);

  const createGuestOrderIdempotencyKey = useCallback(
    (
      signature: string,
      ref: React.MutableRefObject<{ signature: string; key: string } | null>,
    ) => {
      if (ref.current?.signature === signature && ref.current.key) {
        return ref.current.key;
      }

      const generatedKey =
        typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
          ? crypto.randomUUID()
          : `guest-order-${Date.now()}-${Math.random().toString(16).slice(2)}`;

      ref.current = {
        signature,
        key: generatedKey,
      };
      return generatedKey;
    },
    [],
  );

  const clearGuestOrderIdempotencyKey = useCallback(
    (
      ref: React.MutableRefObject<{ signature: string; key: string } | null>,
    ) => {
      ref.current = null;
    },
    [],
  );

  // G-1/G-9: map backend error codes to truthful, translated messages.
  // item_unavailable additionally cross-checks the cart against the menu
  // lookup and offers a tap-to-remove affordance for the offending line(s).
  const handleGuestOrderError = useCallback(
    (
      error: unknown,
      context: {
        path: "create-bill" | "add-items";
        orderRequestSent: boolean;
      },
    ) => {
      const errorInfo = guestOrderErrorInfo(error);
      if (errorInfo.code === "idempotency_conflict") {
        // This key is permanently bound to another table on the server. Retire
        // it locally so a guest retry mints a fresh identity instead of looping
        // on the same safe 409 forever.
        clearGuestOrderIdempotencyKey(orderSubmissionRef);
        toast.error(t("menu.orderErrorGeneric"));
        return;
      }
      const presentation = presentGuestOrderError(errorInfo, context);

      if (presentation.offerRemoveItem) {
        const blockedItemIds = new Set(
          errorInfo.blockedItems?.map((item) => item.menuItemId) || [],
        );
        if (errorInfo.blockedItems) {
          setItemOrderability((previous) => {
            const next = { ...previous };
            for (const blocked of errorInfo.blockedItems || []) {
              next[blocked.menuItemId] = {
                state: blocked.reason,
                orderable: false,
              };
            }
            return next;
          });
        }
        const isUnavailableCartItem = (item: CartItem): boolean => {
          if (blockedItemIds.size > 0) {
            return cartItemContainsBlockedMenuItem(
              item,
              blockedItemIds,
              (bundleId) => {
                const bundle = availableBundles.find(
                  (candidate) => candidate.id === bundleId,
                );
                return bundle
                  ? parseBundleItems(bundle).map((ref) => ref.menu_item_id)
                  : [];
              },
              (lowerName) => menuItemLookup.byName.get(lowerName)?.id,
            );
          }
          if (item.itemType !== "menu_item") return false;
          const entry =
            (item.menuItemId
              ? menuItemLookup.byId.get(item.menuItemId)
              : undefined) ||
            menuItemLookup.byName.get(item.name.toLowerCase());
          return entry?.isAvailable === false;
        };
        const unavailableItems = cart.filter(isUnavailableCartItem);
        const names = unavailableItems.map((item) => item.name);
        const message =
          names.length > 0
            ? t("menu.orderErrorItemUnavailable", { name: names.join(", ") })
            : t("menu.orderErrorItemUnavailableGeneric");
        toast.error(
          (tToast) => (
            <span className="flex items-center gap-3">
              <span>{message}</span>
              {unavailableItems.length > 0 && (
                <button
                  type="button"
                  onClick={() => {
                    // Re-evaluate the live cart by identity. Capturing the
                    // indexes from toast-open time could delete a different
                    // line after the guest edited the cart.
                    setCart((prev) =>
                      prev.filter((item) => !isUnavailableCartItem(item)),
                    );
                    toast.dismiss(tToast.id);
                  }}
                  className="font-semibold underline"
                >
                  {t("menu.remove")}
                </button>
              )}
            </span>
          ),
          { duration: 8000 },
        );
        return;
      }

      toast.error(t(presentation.messageKey, presentation.params));
    },
    [
      availableBundles,
      cart,
      clearGuestOrderIdempotencyKey,
      menuItemLookup,
      parseBundleItems,
      t,
    ],
  );

  // Shared submit core for both paths. Clears the idempotency key and the
  // cart ONLY here, after createGuestOrder resolved (confirmed success).
  const submitCartAsOrder = useCallback(
    async (
      bill: BillWithItemsResponse | null,
      path: "create-bill" | "add-items",
    ) => {
      const items = buildGuestOrderItems(
        cart,
        (lowerName) => menuItemLookup.byName.get(lowerName)?.id,
      );
      const signature = buildCartSignature(tableCode, items);
      const idempotencyKey = createGuestOrderIdempotencyKey(
        signature,
        orderSubmissionRef,
      );

      const checkout = await createGuestOrder(
        tableCode,
        {
          bill_id: bill?.bill.id,
          items,
          // English chrome is intentional: leftover tickets already store this
          // shape. Kitchen remaps it at render via localizeKitchenOrderNote (#776).
          notes:
            path === "create-bill"
              ? `${formatEntityName(t("menu.tableLabel") || "Table", tableData?.table.name || "Unknown")} - Initial Order`
              : `${formatEntityName(t("menu.tableLabel") || "Table", tableData?.table.name || "Unknown")} - Additional Items`,
          promo_code: appliedPromo?.code || undefined,
        },
        { idempotencyKey },
      );

      // Confirmed success — clear cart BEFORE updating bill chrome so diners
      // never see dual totals (floating cart + BILL badge) (#76).
      clearGuestOrderIdempotencyKey(orderSubmissionRef);
      clearCart();
      setAppliedPromo(null);
      setShowCart(false);
      setCurrentBill({ bill: checkout.bill, items: [] });
      return checkout;
    },
    [
      cart,
      menuItemLookup,
      tableCode,
      tableData,
      appliedPromo,
      clearCart,
      createGuestOrderIdempotencyKey,
      clearGuestOrderIdempotencyKey,
      t,
    ],
  );

  // Persist the guest-entered fiscal identity (e.g. AFIP CUIT) to a bill,
  // best-effort. Shared by BOTH order paths: the create-bill path knows the bill
  // number only after creation, and the add-items path must apply it too — a
  // guest who enters their CUIT when a bill already exists (another phone opened
  // it, or they add a later round) would otherwise have it silently dropped and
  // get no/incorrect fiscal invoice. A failure here must never block the order.
  const persistFiscalIdentityToBill = useCallback(
    async (billToken?: string) => {
      if (
        !shouldShowFiscalIdentityFields({
          country: tableData?.business?.address?.country,
        })
      ) {
        return;
      }
      const fiscal = fiscalIdentityRef.current;
      const fiscalPayload = fiscalIdentityToPayload(fiscal.value);
      if (fiscal.valid && Object.keys(fiscalPayload).length > 0 && billToken) {
        try {
          await setBillFiscalCustomerByNumber(billToken, fiscalPayload);
        } catch {
          // Non-fatal: the operator can still set fiscal identity later.
        }
      }
    },
    [tableData?.business?.address?.country],
  );

  const handleCreateBill = useCallback(async () => {
    if (!isOrderingEnabled) {
      showOrderingDisabledToast();
      return;
    }

    if (!tableData) return;

    setOrderLoading(true);
    let orderRequestSent = false;
    try {
      orderRequestSent = true;
      const checkout = await submitCartAsOrder(null, "create-bill");

      // Atomic checkout returns the newly-created bill capability, so fiscal
      // identity can be persisted without a separate empty-bill mutation.
      await persistFiscalIdentityToBill(guestBillRef(checkout.bill));

      setOrderSuccessMessage(t("menu.orderSubmittedMessage"));
      // Open success after the cart modal starts closing so the ack isn't buried
      // (issue 76). Stay mounted — the follow-up refresh is silent.
      window.setTimeout(() => setShowOrderSuccess(true), 200);
    } catch (error) {
      handleGuestOrderError(error, {
        path: "create-bill",
        orderRequestSent,
      });
    } finally {
      setOrderLoading(false);
    }
    // Always re-sync table/bill state — including after a conflict or a
    // failed submit — so the next attempt routes through the right path.
    await loadTableData({ silent: true });
  }, [
    isOrderingEnabled,
    showOrderingDisabledToast,
    tableData,
    submitCartAsOrder,
    persistFiscalIdentityToBill,
    handleGuestOrderError,
    loadTableData,
    t,
  ]);

  const handleAddItemsToBill = useCallback(async () => {
    if (!isOrderingEnabled) {
      showOrderingDisabledToast();
      return;
    }

    if (!currentBill || cart.length === 0 || !tableData) return;

    setOrderLoading(true);
    try {
      // Apply any guest-entered fiscal identity to the existing bill too, so a
      // CUIT entered on this add-items round isn't dropped (the create-bill path
      // already does this; this is the parity fix).
      await persistFiscalIdentityToBill(guestBillRef(currentBill.bill));
      await submitCartAsOrder(currentBill, "add-items");
      setOrderSuccessMessage(t("menu.additionalItemsSubmitted"));
      window.setTimeout(() => setShowOrderSuccess(true), 200);
    } catch (error) {
      handleGuestOrderError(error, {
        path: "add-items",
        orderRequestSent: true,
      });
    } finally {
      setOrderLoading(false);
    }
    await loadTableData({ silent: true });
  }, [
    isOrderingEnabled,
    showOrderingDisabledToast,
    currentBill,
    cart.length,
    tableData,
    submitCartAsOrder,
    persistFiscalIdentityToBill,
    handleGuestOrderError,
    loadTableData,
    t,
  ]);

  const handleActiveBillChange = useCallback(
    (hasActiveBill: boolean) => {
      if (hasActiveBill) {
        void loadTableData({ silent: true });
        return;
      }
      if (currentBill) {
        setCurrentBill(null);
        clearCart();
        toast(t("bill.billClosedMessage"));
      }
    },
    [clearCart, currentBill, loadTableData, t],
  );

  useGuestBillSync({
    tableCode,
    hasActiveBill: Boolean(currentBill?.bill),
    onActiveBillChange: handleActiveBillChange,
  });

  useEffect(() => {
    loadTableData();
    return () => {
      // Bump generation so in-flight loads can't update state after unmount.
      loadGenerationRef.current += 1;
    };
  }, [tableCode, loadTableData]);

  if (shouldShowMenuLoadingGate(loading, tableData, tableCode)) {
    return (
      <div className="min-h-[100dvh] bg-warm-50">
        {/* Skeleton mirroring the real menu shell so the screen looks like
            the page-in-progress rather than a generic spinner. The few
            extra DOM nodes here matter because the bare spinner read as a
            blank screen for ~4s on slower connections. */}
        <div className="sticky top-0 z-10 border-b border-warm-200 bg-warm-50/95 backdrop-blur">
          <div className="mx-auto flex max-w-3xl items-center justify-between gap-3 px-4 py-3">
            <div className="h-10 w-10 animate-pulse rounded-full bg-warm-200/80" />
            <div className="h-8 flex-1 max-w-xs animate-pulse rounded-full bg-warm-200/80" />
            <div className="h-10 w-10 animate-pulse rounded-full bg-warm-200/80" />
          </div>
          <div className="mx-auto flex max-w-3xl gap-2 overflow-hidden px-4 pb-3">
            {Array.from({ length: 4 }).map((_, i) => (
              <div
                key={i}
                className="h-8 w-20 animate-pulse rounded-full bg-warm-200/80"
              />
            ))}
          </div>
        </div>
        <div className="mx-auto max-w-3xl space-y-3 px-4 pt-5">
          {Array.from({ length: 5 }).map((_, i) => (
            <div
              key={i}
              className="flex gap-3 rounded-2xl border border-warm-200 bg-white p-3"
            >
              <div className="h-20 w-20 shrink-0 animate-pulse rounded-xl bg-warm-100" />
              <div className="flex-1 space-y-2 pt-1">
                <div className="h-4 w-3/4 animate-pulse rounded bg-warm-100" />
                <div className="h-3 w-full animate-pulse rounded bg-warm-100" />
                <div className="h-3 w-1/2 animate-pulse rounded bg-warm-100" />
              </div>
            </div>
          ))}
        </div>
        <span className="sr-only" role="status" aria-live="polite">
          {t("menu.loadingMenu")}
        </span>
        <OrderSuccessModal
          isOpen={showOrderSuccess}
          onClose={() => setShowOrderSuccess(false)}
          message={orderSuccessMessage}
          tableCode={tableCode}
          t={t}
        />
      </div>
    );
  }

  if (!tableData) {
    const isNetwork = tableLoadError === "network";
    return (
      <div className="flex min-h-[100dvh] items-center justify-center bg-warm-50 px-6">
        <div className="max-w-md w-full text-center">
          <p className="text-label uppercase text-ink-500 mb-3">
            {isNetwork ? "…" : "404"}
          </p>
          <h1 className="font-title text-heading-lg text-ink-950 mb-4">
            {isNetwork ? t("errors.networkError") : t("errors.tableNotFound")}
          </h1>
          <p className="text-body text-ink-600 mb-8">
            {isNetwork
              ? t("errors.networkErrorDescription")
              : t("errors.tableNotFoundDescription", { tableCode })}
          </p>
          <div className="flex flex-col gap-3">
            <button
              onClick={() => window.location.reload()}
              className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
            >
              {t("errors.tableNotFoundRetry")}
            </button>
            <Link
              href="/"
              className="inline-flex items-center justify-center rounded-full border border-ink-200 bg-white px-6 py-3 text-sm font-semibold text-ink-700 transition-colors hover:bg-ink-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink-400 focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
            >
              {t("errors.tableNotFoundGoHome")}
            </Link>
            {!isNetwork && (
              <p className="text-body-sm text-ink-500 mt-2">
                {t("errors.tableNotFoundHelp")}
              </p>
            )}
          </div>
        </div>
      </div>
    );
  }

  const { business, categories } = tableData;

  // Use translated categories if available, otherwise fall back to original
  const displayCategories =
    translatedCategories.length > 0 ? translatedCategories : categories;

  // Safety check for categories data (similar to BillCreator fix)
  const safeCategories = Array.isArray(displayCategories)
    ? displayCategories
    : [];

  return (
    <div className="relative min-h-[100dvh] bg-warm-50">
      <PendingCartRouteConsumer
        tableCode={tableCode}
        ready={
          !loading &&
          canonicalPendingCartTableCode(tableData.table.table_code) ===
            canonicalPendingCartTableCode(tableCode) &&
          cartRestoreReadyTable === tableCode &&
          menuLocaleReadyFor ===
            `${canonicalPendingCartTableCode(tableCode) ?? ""}:${selectedLanguage || currentLanguage || businessDefaultLanguage || "en"}`
        }
        categories={safeCategories}
        bundles={availableBundles}
        orderability={itemOrderability}
        cart={cart}
        addToCart={applyPendingCartTarget}
        undo={undoPendingCartTarget}
        onApplied={showPendingCartApplied}
        onRejected={showPendingCartRejected}
      />
      {/* Combined sticky shell: brand bar + search + category rail.
          One backdrop-blur surface, one shadow, one border. */}
      <div
        className="sticky top-0 z-40 border-b border-warm-200 bg-warm-50/85 backdrop-blur-xl"
        data-testid="guest-menu-chrome"
        data-compact={chromeCompact ? "true" : "false"}
      >
        <div
          className={`mx-auto flex w-full max-w-2xl items-center gap-3 px-4 sm:px-6 transition-[padding] ${guestMenuChromeBarPaddingClass(chromeCompact)}`}
        >
          <Link
            href={`/t/${tableCode}`}
            className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl border border-warm-200 bg-white text-ink-700 transition-colors hover:border-ink-300 hover:text-ink-900"
            aria-label={t("navigation.table")}
          >
            <ArrowLeft className="h-5 w-5 rtl:rotate-180" strokeWidth={1.75} />
          </Link>

          <div className="min-w-0 flex-1">
            {/* RV-4: hide table label when compact to reclaim ~20–24px. */}
            {guestMenuChromeShowsTableLabel(chromeCompact) ? (
              <p
                className="truncate text-label uppercase text-ink-500"
                data-testid="guest-menu-table-label"
              >
                {formatEntityName(t("menu.tableLabel"), tableData.table.name)}
              </p>
            ) : null}
            {/* The business name is the page-level h1 — guests landing from a
                QR scan deserve a real <h1> for SEO and screen-reader
                orientation. Visual styling preserved. */}
            <h1
              data-testid="guest-menu-business-title"
              className={`font-title text-ink-950 ${guestMenuChromeTitleClass(chromeCompact)}`}
            >
              {business.name}
            </h1>
          </div>

          {business.logo && (
            <FailsafeImage
              src={business.logo}
              alt={business.name}
              wrapperClassName="hidden h-9 w-9 flex-shrink-0 overflow-hidden rounded-lg border border-warm-200 bg-white sm:block"
            />
          )}

          {isOrderingEnabled && (
            <button
              type="button"
              onClick={() => setShowCart(true)}
              className="relative flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl border border-warm-200 bg-white text-ink-700 transition-colors hover:border-ink-300 hover:text-ink-900"
              aria-label={
                cartItemCount > 0
                  ? t("accessibility.cartButton", { count: cartItemCount })
                  : t("menu.viewCart")
              }
            >
              <ShoppingCart className="h-5 w-5" strokeWidth={1.75} />
              {cartItemCount > 0 && (
                <span className="absolute -top-1.5 -end-1.5 inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-brand px-1.5 text-[10px] font-semibold tabular-nums text-white shadow-sm">
                  {cartItemCount}
                </span>
              )}
            </button>
          )}
        </div>

        <div className="mx-auto w-full max-w-2xl px-4 pb-3 sm:px-6">
          {/* RV-3: gap-3 (12px) between search and filters — SC 2.5.8 spacing;
              do not bump input height (target is already h-10 / 40px). */}
          <div className="flex items-center gap-3">
            <div className="flex-1">
              <GuestMenuSearchField
                value={searchQuery}
                onChange={setSearchQuery}
                label={
                  t("menu.search.placeholder") || "Search menu items..."
                }
              />
            </div>

            <MenuViewRadiogroup
              value={viewMode}
              onChange={setViewMode}
              label={t("menu.viewMode.label") || "Menu view"}
              optionLabels={{
                detailed: t("menu.viewMode.detailed") || "Detailed list",
                compact: t("menu.viewMode.compact") || "Compact grid",
                grid: t("menu.viewMode.grid") || "Image grid",
                "category-tabs":
                  t("menu.viewMode.categoryTabs") || "By category",
              }}
            />

            <Button
              variant={showFilters ? "solid" : "bordered"}
              size="sm"
              aria-label={t("menu.filters.title") || "Filters"}
              aria-expanded={showFilters}
              aria-controls={GUEST_MENU_FILTERS_PANEL_ID}
              title={t("menu.filters.title") || "Filters"}
              onPress={() => setShowFilters(!showFilters)}
              startContent={<Filter className="h-4 w-4" strokeWidth={1.75} />}
              className={`h-10 ${
                showFilters
                  ? "bg-brand text-white border-brand"
                  : "border-warm-200 text-ink-700 hover:border-ink-300 hover:text-ink-900"
              }`}
            >
              <span className="hidden sm:inline">
                {t("menu.filters.title") || "Filters"}
              </span>
            </Button>
            {(searchQuery || selectedFilters.size > 0) && (
              <Button
                variant="light"
                size="sm"
                onPress={clearAllFilters}
                startContent={<X className="h-4 w-4" strokeWidth={1.75} />}
                className="h-10 text-ink-500 hover:text-ink-900"
                aria-label={t("menu.filters.clear") || "Clear"}
              >
                <span className="hidden sm:inline">
                  {t("menu.filters.clear") || "Clear"}
                </span>
              </Button>
            )}
          </div>

          {showFilters && (
            <MenuFilterToggles
              id={GUEST_MENU_FILTERS_PANEL_ID}
              filters={availableFilters}
              selected={selectedFilters}
              getLabel={getFilterLabel}
              onToggle={toggleFilter}
              groupLabel={t("menu.filters.title") || "Filters"}
            />
          )}

          {/* Category rail — same sticky shell, no second sticky position.
              Buttons scroll to the matching category section AND sync the
              URL hash so guests can deep-link / share a specific course. */}
          <CategoryTabs
            categories={menuDisplayCategories.map((category, index) => ({
              slug: slugifyCategoryName(category.name, index),
              name: category.name,
            }))}
            activeIndex={activeCategory}
            getAnchorId={(_category, index) => `category-${index}`}
            onSelect={(index) => {
              if (viewMode === "category-tabs") {
                setActiveCategory(index);
              }
            }}
          />
        </div>
      </div>

      {/* Menu Content */}
      <div
        role="status"
        aria-live="polite"
        className="sr-only"
        data-testid="guest-menu-action-status"
      >
        {menuActionStatus}
      </div>
      <div className="mx-auto w-full max-w-2xl px-4 py-6 pb-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom)+6.5rem)] sm:px-6 sm:py-8">
        {/* Banner priority: kitchen off > hours closed. Persistent, non-dismissible. */}
        {!kitchenOrdersOn ? (
          <div
            role="status"
            className="mb-6 flex items-start gap-3 rounded-2xl border border-warm-200 bg-warm-50 px-4 py-3"
          >
            <Info
              className="mt-0.5 h-5 w-5 flex-shrink-0 text-brand"
              strokeWidth={1.75}
            />
            <p className="flex-1 text-sm text-ink-700">
              {t("menu.orderingDisabled")}
            </p>
          </div>
        ) : isBusinessClosed ? (
          <GuestClosedBanner className="mb-6" />
        ) : null}
        {/* Mute offers/bundles order CTAs when closed — hide merchandising strip. */}
        {!isBusinessClosed &&
          (filteredOffers.length > 0 || filteredBundles.length > 0) && (
            <section
              data-testid="menu-offers-bundles"
              className="mb-8 space-y-3"
            >
              {filteredOffers.length > 0 && (
                <div className="overflow-hidden rounded-2xl border border-amber-200 bg-amber-50/60">
                  <header className="flex items-center justify-between gap-3 px-4 pt-4 pb-2">
                    <h3 className="text-label uppercase text-amber-800">
                      {t("menu.offers.title")}
                    </h3>
                    <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold tabular-nums text-amber-800">
                      {filteredOffers.length}
                    </span>
                  </header>
                  <ul className="divide-y divide-amber-200/70">
                    {filteredOffers.map((offer) => (
                      <li
                        key={offer.id || offer.name}
                        className="flex items-start gap-3 px-4 py-3"
                      >
                        {offer.image && (
                          <FailsafeImage
                            src={offer.image}
                            alt={offer.name}
                            wrapperClassName="h-12 w-12 flex-shrink-0 overflow-hidden rounded-lg border border-amber-200"
                          />
                        )}
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-semibold text-ink-900">
                            {offer.name}
                          </p>
                          {offer.description && (
                            <p className="line-clamp-2 text-xs text-ink-600">
                              {offer.description}
                            </p>
                          )}
                        </div>
                        <span className="flex-shrink-0 rounded-full bg-white px-2.5 py-1 text-xs font-semibold tabular-nums text-amber-800 ring-1 ring-amber-200">
                          {/* Fixed-amount offers should show their currency,
                            not a bare number — previously read like
                            "-1.50" which is ambiguous next to "-15%". */}
                          {offer.discount_type === "percentage"
                            ? `-${offer.discount_value}%`
                            : `-${fmtCurrency(offer.discount_value)}`}
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {filteredBundles.length > 0 && (
                <div className="overflow-hidden rounded-2xl border border-brand/30 bg-brand/[0.04]">
                  <header className="flex items-center justify-between gap-3 px-4 pt-4 pb-2">
                    <h3 className="text-label uppercase text-brand">
                      {t("menu.bundles.title")}
                    </h3>
                    <span className="rounded-full bg-brand/10 px-2 py-0.5 text-[11px] font-semibold tabular-nums text-brand">
                      {filteredBundles.length}
                    </span>
                  </header>
                  <ul className="divide-y divide-brand/20">
                    {filteredBundles.map((bundle) => {
                      const refs = parseBundleItems(bundle);
                      const bundleBlocked = refs.some(
                        (ref) =>
                          itemOrderability[ref.menu_item_id]?.orderable ===
                          false,
                      );
                      // #822: label the block honestly — closed hours and
                      // ordering-off are venue reasons, only an 86 is stock.
                      const bundleBlockedReason: BlockedReasonKind | null =
                        bundleBlocked
                          ? blockedReasonKind(
                              refs
                                .filter(
                                  (ref) =>
                                    itemOrderability[ref.menu_item_id]
                                      ?.orderable === false,
                                )
                                .map((ref) => ({
                                  orderabilityState:
                                    itemOrderability[ref.menu_item_id]?.state,
                                  inventoryStatus: menuItemLookup.byId.get(
                                    ref.menu_item_id,
                                  )?.inventoryStatus,
                                })),
                            )
                          : null;
                      const bundleWarning = refs.some((ref) => {
                        const state = itemOrderability[ref.menu_item_id]?.state;
                        return state === "inventory_warning";
                      });
                      const regularTotal = getBundleRegularTotal(bundle);
                      const savings = Math.max(regularTotal - bundle.price, 0);
                      return (
                        <li
                          key={bundle.id || bundle.name}
                          className="px-4 py-3"
                        >
                          <div className="flex items-start gap-3">
                            {bundle.image && (
                              <FailsafeImage
                                src={bundle.image}
                                alt={bundle.name}
                                wrapperClassName="h-12 w-12 flex-shrink-0 overflow-hidden rounded-lg border border-brand/20"
                              />
                            )}
                            <div className="min-w-0 flex-1">
                              <p className="truncate text-sm font-semibold text-ink-900">
                                {bundle.name}
                              </p>
                              {bundle.description ? (
                                <p className="line-clamp-2 text-xs text-ink-600">
                                  {bundle.description}
                                </p>
                              ) : (
                                <p className="line-clamp-2 text-xs text-ink-500">
                                  {refs
                                    .map((ref) => {
                                      const itemName =
                                        ref.name ||
                                        menuItemLookup.byId.get(
                                          ref.menu_item_id,
                                        )?.name ||
                                        ref.menu_item_id;
                                      return `${ref.quantity}× ${itemName}`;
                                    })
                                    .join(" · ")}
                                </p>
                              )}
                              {bundleBlockedReason &&
                                (bundleBlockedReason === "business_closed" ||
                                bundleBlockedReason === "ordering_disabled" ? (
                                  <p className="mt-1 text-xs font-medium text-ink-600">
                                    {bundleBlockedReason === "business_closed"
                                      ? t("menu.businessClosed")
                                      : t("menu.orderingDisabled")}
                                  </p>
                                ) : (
                                  <p className="mt-1 text-xs font-semibold text-rose-700">
                                    {bundleBlockedReason === "out_of_stock"
                                      ? t("menu.outOfStock")
                                      : t("menu.unavailable")}
                                  </p>
                                ))}
                              {!bundleBlocked && bundleWarning && (
                                <p className="mt-1 text-xs font-semibold text-amber-700">
                                  {t("menu.lowStock")}
                                </p>
                              )}
                            </div>
                            <div className="text-end">
                              <p className="font-mono text-sm font-semibold tabular-nums text-brand">
                                <CurrencyPrice
                                  amount={bundle.price}
                                  fromCurrency={
                                    businessCurrencies.default_currency
                                  }
                                  displayCurrency={
                                    businessCurrencies.display_currency
                                  }
                                  locale={moneyLocale}
                                />
                              </p>
                              {savings > 0 && (
                                <p className="font-mono text-[11px] tabular-nums text-emerald-700">
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
                          {isOrderingEnabled && (
                            <div className="mt-3 flex justify-end">
                              <Button
                                size="sm"
                                color="primary"
                                variant="flat"
                                isDisabled={bundleBlocked}
                                aria-label={t("accessibility.addNamedBundle", {
                                  name: bundle.name,
                                })}
                                onPress={() => addBundleToCart(bundle)}
                              >
                                {addedBundleId === bundle.id
                                  ? t("menu.bundles.addedShort") ||
                                    t("menu.added")
                                  : t("menu.bundles.addBundle") || "Add Bundle"}
                              </Button>
                            </div>
                          )}
                        </li>
                      );
                    })}
                  </ul>
                </div>
              )}
            </section>
          )}

        <GuestMenuViews
          key={`menu-${selectedLanguage}-${menuDisplayCategories.length}-${menuDisplayCategories[0]?.items?.[0]?.name || "empty"}-${viewMode}`}
          categories={menuDisplayCategories}
          business={business}
          tableCode={tableCode}
          currentBill={currentBill}
          selectedLanguage={selectedLanguage}
          defaultCurrency={businessCurrencies.default_currency}
          displayCurrency={businessCurrencies.display_currency}
          viewMode={viewMode}
          activeCategory={activeCategory}
          onAddToCart={addToCart}
          itemOrderability={itemOrderability}
          onItemClick={(_item: MenuItem) => {
            // Handle item click - could open a modal or navigate to item details
          }}
          isOrderingEnabled={isOrderingEnabled}
          hasActiveFilters={Boolean(searchQuery) || selectedFilters.size > 0}
          onClearFilters={clearAllFilters}
        />
      </div>

      {/* Thumb-reach cart FAB — sits above bottom nav, mobile-first */}
      {isOrderingEnabled && cartItemCount > 0 && (
        <>
          {quoteError || quoteBlockedLines.length > 0 ? (
            <p
              role="alert"
              className="fixed left-1/2 z-40 max-w-[calc(100vw-2rem)] -translate-x-1/2 rounded-xl bg-rose-50 px-3 py-2 text-center text-xs font-medium text-rose-800 shadow"
              style={{
                bottom:
                  "calc(var(--guest-nav-height, 5.5rem) + var(--cookie-banner-height, 0px) + env(safe-area-inset-bottom) + 3.5rem)",
              }}
            >
              {quoteBlockedLines.length > 0
                ? quoteBlockedMessage
                : t("errors.menuUnavailableDescription")}
            </p>
          ) : null}
          <button
            type="button"
            onClick={() => setShowCart(true)}
            className="fixed left-1/2 z-40 inline-flex max-w-[calc(100vw-2rem)] -translate-x-1/2 items-center gap-3 rounded-full bg-ink-900 px-5 py-3 text-sm font-semibold text-white shadow-lg transition-transform active:translate-y-px"
            style={{
              bottom:
                "calc(var(--guest-nav-height, 5.5rem) + var(--cookie-banner-height, 0px) + env(safe-area-inset-bottom) + 0.5rem)",
            }}
          >
            <span className="inline-flex h-6 min-w-[1.5rem] items-center justify-center rounded-full bg-white/15 px-1.5 font-mono text-xs tabular-nums">
              {cartItemCount}
            </span>
            <span>{t("menu.viewCart") || "View cart"}</span>
            <span className="font-mono text-sm tabular-nums opacity-80">·</span>
            <span className="font-mono text-sm tabular-nums">
              {quotePending ? (
                <>
                  {t("common.subtotal")}{" "}
                  <CurrencyPrice
                    amount={promotionPreview.baseSubtotal}
                    fromCurrency={businessCurrencies.default_currency}
                    displayCurrency={businessCurrencies.display_currency}
                    locale={moneyLocale}
                  />
                </>
              ) : (
                <CurrencyPrice
                  amount={promotionPreview.finalTotal}
                  fromCurrency={businessCurrencies.default_currency}
                  displayCurrency={businessCurrencies.display_currency}
                  locale={moneyLocale}
                />
              )}
            </span>
            {quotePending && <Spinner size="sm" color="white" aria-hidden />}
          </button>
          <GuestQuoteStatus
            pending={quotePending}
            error={Boolean(quoteError)}
            blocked={quoteBlockedLines.length > 0}
            pendingLabel={t("menu.updatingTotal")}
            errorLabel={t("errors.menuUnavailableDescription")}
            blockedLabel={quoteBlockedMessage}
            settledLabel={t("menu.totalUpdated", {
              amount: fmtCurrency(promotionPreview.finalTotal),
            })}
          />
        </>
      )}

      {/* Raise a hand — same affordance as the table landing page */}
      <div className="pointer-events-none relative z-40 mx-auto w-full max-w-2xl px-4 pb-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom))] sm:px-6">
        <CallWaiterButton
          tableCode={tableCode}
          businessClosed={isBusinessClosed}
        />
      </div>

      {/* Persistent Navigation */}
      <PersistentGuestNav
        tableCode={tableCode}
        currentBill={currentBill}
        isOrderingEnabled={isOrderingEnabled}
        defaultCurrency={businessCurrencies.default_currency}
        displayCurrency={businessCurrencies.display_currency}
      />

      {/* Shopping Cart Modal */}
      <CartModal
        isOpen={showCart}
        onClose={() => setShowCart(false)}
        cart={cart}
        businessCurrencies={businessCurrencies}
        promotionPreview={promotionPreview}
        orderLoading={orderLoading}
        quotePending={quotePending}
        quoteError={quoteError}
        quoteValid={quoteValid}
        quoteBlocked={quoteBlockedLines.length > 0}
        quoteBlockedMessage={quoteBlockedMessage}
        onUpdateQuantity={updateCartItemQuantity}
        onSubmitOrder={currentBill ? handleAddItemsToBill : handleCreateBill}
        onClearCart={clearCart}
        t={t}
        tableCode={tableCode}
        appliedPromo={appliedPromo}
        onPromoApplied={(offer) => setAppliedPromo(offer)}
        onPromoRemoved={() => setAppliedPromo(null)}
        country={business.address?.country}
        onFiscalIdentityChange={(value, valid) => {
          fiscalIdentityRef.current = { value, valid };
        }}
      />

      {/* Order Success Modal */}
      <OrderSuccessModal
        isOpen={showOrderSuccess}
        onClose={() => setShowOrderSuccess(false)}
        message={orderSuccessMessage}
        tableCode={tableCode}
        t={t}
      />

      {/* AI Waiter - Only when the backend reports AI is available */}
      {shouldShowAiWaiter(business) && (
        <AiWaiter
          businessId={business.id}
          businessName={business.name}
          aiName={business.ai_settings?.ai_name}
          waiterMode={business.ai_waiter_mode}
          menuData={safeCategories}
          itemOrderability={itemOrderability}
          // selectedLanguage starts empty and hydrates from storage/geo; fall
          // back to the live guest locale (then English) so the AI-waiter never
          // sends an empty language before the picker resolves (audit H2).
          language={selectedLanguage || currentLanguage || "en"}
          bundles={sellableBundles}
          tableCode={tableCode}
          isOrderingEnabled={isOrderingEnabled}
          onAddToCart={(itemName, price, quantity, notes, metadata) =>
            addToCart(itemName, price, quantity, notes, undefined, {
              itemType: metadata?.itemType || "menu_item",
              menuItemId: metadata?.menuItemId,
              bundleId: metadata?.bundleId,
            })
              ? "applied"
              : "rejected"
          }
          hasActiveBill={!!currentBill}
          billItems={
            currentBill?.items.map((i) => ({
              name: i.menu_item?.name || i.name || t("bill.unknownItem"),
              price: i.price,
              quantity: i.quantity,
            })) || []
          }
          currency={businessCurrencies.default_currency}
          // Lift the AI FAB above the cart pill when both share the bottom band
          // so they can't collide in the thumb zone on notched phones (L7).
          cartVisible={Boolean(isOrderingEnabled) && cartItemCount > 0}
        />
      )}
    </div>
  );
}

// PG-21: GuestTranslationProvider is seeded by t/[tableCode]/layout.tsx so
// SSR nav labels match hydration. Do not wrap with an empty provider here —
// that would reset messages to English until the client load finishes.
export default function GuestMenuPage() {
  const params = useParams();
  const routeTableCode = getRouteParam(params, "tableCode");
  const scopeKey = menuRouteScopeKey(routeTableCode);
  return (
    <GuestMenuTableScope tableCode={scopeKey}>
      <GuestMenuPageContent />
    </GuestMenuTableScope>
  );
}
