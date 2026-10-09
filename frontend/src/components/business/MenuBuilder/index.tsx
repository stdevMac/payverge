"use client";

import React, { useState, useMemo, useEffect, useCallback, useRef } from "react";
import { useSearchParams } from "next/navigation";
import { useMenuData } from "./hooks/useMenuData";
import { useMenuMutations } from "./hooks/useMenuMutations";
import { countMenuSearchResults } from "./menuSearchStats";
import { MenuPageHeader } from "./components/MenuPageHeader";
import { LanguagesPopover } from "./components/LanguagesPopover";
import { AddSourceMenu } from "./components/AddSourceMenu";
import { ActiveLanguagePills } from "./components/ActiveLanguagePills";
import { AIMenuOnboardingModal } from "./components/AIMenuOnboardingModal";
import MenuSanitizationReview from "./AIMenuOnboarding/MenuSanitizationReview";
import { MenuList } from "./components/MenuList";
import { MenuLoadingSkeleton } from "./MenuLoadingSkeleton";
import { PrintMenuButton } from "./print/PrintMenuButton";
import { PrintMenuStudio } from "./print/PrintMenuStudio";
import OffersManager from "../OffersManager";
import BundlesManager from "../BundlesManager";
import MenuEngineeringMatrix from "./MenuEngineeringMatrix";
import {
    shouldProbeWiderPeriod,
    widerPeriodWithSales as pickWiderPeriod,
} from "./menuEngineeringFallback";
import { isUnavailableForFilter, menuItemManualAvailable } from "./menuAvailabilityFilter";
import {
    menuEngineeringApi,
    type MenuEngineeringReport,
} from "@/api/menuEngineering";
import type { Dollars } from "@/types/money";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import { useDisclosure } from "@nextui-org/react";
import { BarChart3, Package, Tag, Utensils } from "lucide-react";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useSessionTabState } from "@/hooks/useSessionTabState";
import DashboardTabShell from "../shared/DashboardTabShell";
import { DashboardTabTransition, PremiumPanel } from "../premium";

// Modals
import AddCategoryModal from "../modals/AddCategoryModal";
import EditCategoryModal from "../modals/EditCategoryModal";
import AddItemModal from "../modals/AddItemModal";
import EditItemModal from "../modals/EditItemModal";
import ConfirmationModal from "../modals/ConfirmationModal";
import { MenuItemOption } from '../../../api/business';
import { InventoryMenuItemStatus, inventoryApi } from "@/api/inventory";
import { asDollars } from "@/types/money";
import { parseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { interpolateTranslatedViewNotice } from "./translatedViewNotice";
import { interpolateEditBlockedTooltip } from "./editBlockedTooltip";
import toast from "react-hot-toast";

interface MenuBuilderProps {
    businessId: number;
}

export default function MenuBuilder({ businessId }: MenuBuilderProps) {
    const {
        menu,
        setMenu,
        itemOrderability = {},
        menuVersion,
        setMenuVersion,
        isLoading: isMenuLoading,
        error,
        setError,
        loadFailed,
        business,
        defaultCurrency,
        loadMenu,
        tString,
        // Language
        supportedLanguages,
        businessLanguages,
        selectedLanguages,
        setSelectedLanguages,
        defaultLanguage,
        setDefaultLanguage,
        isLanguageLoading,
        currentViewLanguage,
        setCurrentViewLanguage,
        languagesLoaded,
        handleTranslateMenu,
        handleSyncAllTranslations,
        cancelSyncAllTranslations,
        handleLanguageUpdate,
        isTranslating,
        syncProgress,
        syncError,
    } = useMenuData(businessId);

    const {
        selectedCategoryIndex,
        setSelectedCategoryIndex,
        selectedItemIndex: _selectedItemIndex,
        setSelectedItemIndex,
        editingCategory: _editingCategory,
        setEditingCategory,
        editingItem,
        setEditingItem,
        handleAddCategory,
        handleUpdateCategory,
        handleDeleteCategory,
        handleAddItem,
        handleUpdateItem,
        handleToggleEightySix,
        handleDeleteItem,
        handleGenerateBreakdown,
        handleGeneratePhoto,
        handleEnhancePhoto,
        isGeneratingBreakdown,
        isGeneratingPhoto,
        isEnhancingPhoto,
        isSavingItem,
        dailyLimitReached,
        sanitizationReview,
        isConfirmingSanitization,
        confirmPendingSanitization,
        cancelPendingSanitization,
    } = useMenuMutations({ businessId, menu, menuVersion, setMenuVersion, setMenu, loadMenu, setError, defaultCurrency, tString, currentViewLanguage, defaultLanguage });

    // The only lock is the server administrator's lifecycle (suspended /
    // closed). Language and print controls stay read-only until the access
    // check confirms the business is operational.
    const {
        hasAccess,
        loading: accessLoading,
        isError: accessError,
    } = useBusinessAccess(businessId);
    const entitlementConfirmed = hasAccess && !accessError;

    // Surface menu add/update/delete + version-conflict errors. The mutation
    // hook sets `error` on every failure, but it was never rendered — so a
    // failed save looked like nothing happened (a silent failure on a core
    // operator path). Toast it and clear so the next failure re-fires.
    useEffect(() => {
        if (!error) return;
        toast.error(error);
        setError(null);
    }, [error, setError]);

    // Whether the operator is viewing the canonical (default-language) menu.
    // When viewing a translated language, `menu` holds TRANSLATED text — editing
    // or reordering it would write the translation back into the canonical menu
    // (R3-MB-2). So structural + text mutations are gated to the default view; a
    // toast points the operator to switch back. currentViewLanguage is "" until
    // languages load — treat that as the default (editable) view.
    const isDefaultLanguageView =
        !currentViewLanguage || currentViewLanguage === defaultLanguage;
    const defaultLanguageNativeName = useMemo(() => {
        const match = supportedLanguages.find((l) => l.code === defaultLanguage);
        return match?.native_name || defaultLanguage || "";
    }, [supportedLanguages, defaultLanguage]);
    // L3-6: banner names the *viewed* locale (not the default) and the language
    // operators must switch to in order to edit.
    const viewedLanguageNativeName = useMemo(() => {
        const code = currentViewLanguage || defaultLanguage;
        const match = supportedLanguages.find((l) => l.code === code);
        return match?.native_name || code || "";
    }, [supportedLanguages, currentViewLanguage, defaultLanguage]);
    // Returns true (blocking) and toasts when a mutation is attempted in a
    // non-default view. Used to gate add/edit/delete for categories and items.
    const guardTranslatedEdit = useCallback((): boolean => {
        if (isDefaultLanguageView) return false;
        // L3-41: one shared interpolation path with the card tooltips.
        toast(
            interpolateEditBlockedTooltip(
                tString("translatedEditBlocked"),
                defaultLanguageNativeName,
            ),
        );
        return true;
    }, [isDefaultLanguageView, tString, defaultLanguageNativeName]);

    // States
    const menuSearchParams = useSearchParams();
    const [selectedTab, setSelectedTab] = useState("menu");
    const [isPrintStudioOpen, setIsPrintStudioOpen] = useState(false);
    // M6: grid/list view + availability filter survive rail switches for the
    // browser-tab session, then reset with it.
    const [viewMode, setViewMode] = useSessionTabState<"grid" | "list">(
        businessId ? `payverge_menu_viewmode:${businessId}` : null,
        "grid",
        (v) => v === "grid" || v === "list",
    );
    const [searchQuery, setSearchQuery] = useState(
        () => menuSearchParams?.get("menuSearch") ?? "",
    );
    // Debounced copy of searchQuery drives the filter memo (§3.7 fix 9) so a
    // large menu doesn't re-filter + re-render every category on every keystroke.
    // The input itself stays instant (controlled by searchQuery).
    const [debouncedSearchQuery, setDebouncedSearchQuery] = useState(searchQuery);
    useEffect(() => {
        const id = setTimeout(() => setDebouncedSearchQuery(searchQuery), 200);
        return () => clearTimeout(id);
    }, [searchQuery]);
    const [searchFilter, setSearchFilter] = useSessionTabState<"all" | "available" | "unavailable">(
        businessId ? `payverge_menu_searchfilter:${businessId}` : null,
        "all",
        (v) => v === "all" || v === "available" || v === "unavailable",
    );
    const [inventoryStatuses, setInventoryStatuses] = useState<Record<string, InventoryMenuItemStatus>>({});

    // --- Menu engineering matrix ------------------------------------------
    // The report is fetched lazily the first time the "engineering" tab is
    // opened (and again on period change), so non-finance roles that get a
    // `financial:read` 403 just fall through to the matrix's empty state via
    // the `.catch(() => null)` below.
    type MePeriod = "day" | "week" | "month";
    const [mePeriod, setMePeriod] = useState<MePeriod>("week");
    const [meReport, setMeReport] = useState<MenuEngineeringReport | null>(null);
    const [meLoading, setMeLoading] = useState(false);
    const meLoadedRef = useRef(false);
    // Stale-response guard: rapid period toggles (day→week→month) can let an
    // earlier request resolve last and overwrite the newer report, so the
    // highlighted period tab and the matrix would disagree. Only the latest
    // request — matching both the request id and the still-selected period —
    // is allowed to commit. Mirrors the sibling Food-Cost card.
    const meRequestIdRef = useRef(0);
    const mePeriodRef = useRef<MePeriod>(mePeriod);
    // #834: the panel opens on `week`, and a venue whose last recognized
    // payment predates Monday lands on an empty matrix while its Accounting tab
    // shows five figures. When the selected window reports no sales we probe
    // the widest preset once (month ⊇ week ⊇ day, so one probe settles it) and
    // let the empty state name the period that actually has the history.
    const [meWiderPeriod, setMeWiderPeriod] = useState<MePeriod | null>(null);

    const loadMenuEngineering = useCallback(async (period: MePeriod) => {
        const requestId = (meRequestIdRef.current += 1);
        setMeLoading(true);
        const r = await menuEngineeringApi
            .getMenuEngineering(String(businessId), period)
            .catch(() => null);
        if (meRequestIdRef.current !== requestId || mePeriodRef.current !== period) return;
        setMeReport(r);
        setMeLoading(false);
        if (!shouldProbeWiderPeriod(r, period)) {
            setMeWiderPeriod(null);
            return;
        }
        const probe = await menuEngineeringApi
            .getMenuEngineering(String(businessId), "month")
            .catch(() => null);
        // The probe outlives its own load, so re-check the guard: a period
        // switch mid-flight must not paint an offer for the abandoned window.
        if (meRequestIdRef.current !== requestId || mePeriodRef.current !== period) return;
        setMeWiderPeriod(pickWiderPeriod(probe));
    }, [businessId]);

    const handleMePeriodChange = useCallback((p: MePeriod) => {
        if (mePeriodRef.current === p) return; // ignore reselecting the active period
        mePeriodRef.current = p;
        setMePeriod(p);
        void loadMenuEngineering(p);
    }, [loadMenuEngineering]);

    // Business switch: drop the prior business's report (don't flash stale data)
    // and re-arm the lazy loader so the new business refetches on tab open.
    useEffect(() => {
        meLoadedRef.current = false;
        setMeReport(null);
        setMeWiderPeriod(null);
    }, [businessId]);

    // Locale-aware money: intlLocaleFor maps the operator's app locale (es-AR,
    // es, en) to an Intl locale so es-AR sees $1.234,50 rather than the browser
    // default. Mirrors formatCurrency in the sibling AccountingDashboard.
    const { locale } = useSimpleLocale();
    const meFormatMoney = useCallback(
        (value: Dollars, currency: string) =>
            new Intl.NumberFormat(intlLocaleFor(locale), { style: "currency", currency }).format(value as number),
        [locale],
    );

    /** Seed money fields with fixed 2dp so Price matches list formatting (#184). */
    const seedMoneyField = useCallback(
        (value: unknown): string => {
            if (value == null || value === "") return "";
            const n = Number(value);
            if (!Number.isFinite(n)) return "";
            const fixed = n.toFixed(2);
            return locale === "es" || locale === "es-AR"
                ? fixed.replace(".", ",")
                : fixed;
        },
        [locale],
    );

    useEffect(() => {
        if (selectedTab === "engineering" && !meLoadedRef.current) {
            meLoadedRef.current = true;
            void loadMenuEngineering(mePeriod);
        }
    }, [selectedTab, mePeriod, loadMenuEngineering]);

    // Matrix labels, mapped one-to-one from the operator-tier translation tree
    // under dashboard.menuBuilder.menuEngineering. Shape must match
    // MenuEngineeringMatrixProps["labels"] exactly.
    const meLabels = useMemo(
        () => ({
            subtitle: tString("menuEngineering.subtitle"),
            periods: {
                day: tString("menuEngineering.periods.day"),
                week: tString("menuEngineering.periods.week"),
                month: tString("menuEngineering.periods.month"),
            },
            axes: {
                margin: tString("menuEngineering.axes.margin"),
                popularity: tString("menuEngineering.axes.popularity"),
            },
            quadrants: {
                star: {
                    label: tString("menuEngineering.quadrants.star.label"),
                    action: tString("menuEngineering.quadrants.star.action"),
                },
                plowhorse: {
                    label: tString("menuEngineering.quadrants.plowhorse.label"),
                    action: tString("menuEngineering.quadrants.plowhorse.action"),
                },
                puzzle: {
                    label: tString("menuEngineering.quadrants.puzzle.label"),
                    action: tString("menuEngineering.quadrants.puzzle.action"),
                },
                dog: {
                    label: tString("menuEngineering.quadrants.dog.label"),
                    action: tString("menuEngineering.quadrants.dog.action"),
                },
            },
            cards: {
                revenueShare: tString("menuEngineering.cards.revenueShare"),
                dishes: tString("menuEngineering.cards.dishes"),
            },
            table: {
                title: tString("menuEngineering.table.title"),
                item: tString("menuEngineering.table.item"),
                quadrant: tString("menuEngineering.table.quadrant"),
                foodCostPct: tString("menuEngineering.table.foodCostPct"),
                qty: tString("menuEngineering.table.qty"),
                price: tString("menuEngineering.table.price"),
                margin: tString("menuEngineering.table.margin"),
                suggestion: tString("menuEngineering.table.suggestion"),
            },
            states: {
                empty: tString("menuEngineering.states.empty"),
                emptyMissingCost: tString("menuEngineering.states.emptyMissingCost"),
                emptyNoSales: tString("menuEngineering.states.emptyNoSales"),
                emptyNoSalesWider: tString("menuEngineering.states.emptyNoSalesWider"),
                showWiderPeriod: tString("menuEngineering.states.showWiderPeriod"),
                sparse: tString("menuEngineering.states.sparse"),
                needsCost: tString("menuEngineering.states.needsCost"),
                loading: tString("menuEngineering.states.loading"),
            },
            suggest: tString("menuEngineering.suggest"),
        }),
        [tString],
    );

    // M6: viewMode persistence moved to sessionStorage via useSessionTabState
    // (payverge_menu_viewmode:<businessId>) — per-shift, not cross-session.
    // The legacy localStorage mirror ("menuBuilder.viewMode") is retired so it
    // can't resurrect yesterday's view over the session value.
    const persistViewMode = useCallback((next: "grid" | "list") => {
        setViewMode(next);
    }, [setViewMode]);

    // AI onboarding modal
    const [aiOnboardingOpen, setAiOnboardingOpen] = useState(false);
    const [aiInitialTab, setAiInitialTab] = useState<"pdf" | "wizard">("wizard");

    // Modal Disclosures
    const {
        isOpen: isAddCategoryOpen,
        onOpen: onAddCategoryOpen,
        onOpenChange: onAddCategoryOpenChange,
    } = useDisclosure();
    const {
        isOpen: isEditCategoryOpen,
        onOpen: onEditCategoryOpen,
        onOpenChange: onEditCategoryOpenChange,
    } = useDisclosure();
    const {
        isOpen: isAddItemOpen,
        onOpen: onAddItemOpen,
        onOpenChange: onAddItemOpenChange,
    } = useDisclosure();
    const {
        isOpen: isEditItemOpen,
        onOpen: onEditItemOpen,
        onClose: onEditItemClose,
        onOpenChange: onEditItemOpenChange,
    } = useDisclosure();

    // Form states (kept locally as they interact with modals deeply)
    const [categoryName, setCategoryName] = useState("");
    const [categoryDescription, setCategoryDescription] = useState("");

    const [itemName, setItemName] = useState("");
    const [itemDescription, setItemDescription] = useState("");
    const [itemPrice, setItemPrice] = useState("");
    // Plate-level food cost (COGS) — optional; food-cost calculator uses it when
    // the dish has no inventory recipe yet (Task 35).
    const [itemCogs, setItemCogs] = useState("");
    const [itemImages, setItemImages] = useState<string[]>([]);
    const [itemAvailable, setItemAvailable] = useState(true);
    const [itemOptions, setItemOptions] = useState<MenuItemOption[]>([]);
    const [itemAllergens, setItemAllergens] = useState<string[]>([]);
    const [itemDietaryTags, setItemDietaryTags] = useState<string[]>([]);
    const [itemSortOrder, setItemSortOrder] = useState(0);

    // Confirmation Modal State
    const {
        isOpen: isConfirmOpen,
        onOpen: onConfirmOpen,
        onOpenChange: onConfirmOpenChange,
    } = useDisclosure();
    const [confirmAction, setConfirmAction] = useState<() => Promise<void> | void>(() => { });
    const [confirmTitle, setConfirmTitle] = useState("");
    const [confirmDescription, setConfirmDescription] = useState("");
    const [newAllergen, setNewAllergen] = useState("");
    const [newDietaryTag, setNewDietaryTag] = useState("");
    const [newOptionName, setNewOptionName] = useState("");
    const [newOptionPrice, setNewOptionPrice] = useState("");

    // Helpers
    const resetCategoryForm = () => {
        setCategoryName("");
        setCategoryDescription("");
        setEditingCategory(null);
    };

    const resetItemForm = () => {
        setItemName("");
        setItemDescription("");
        setItemPrice("");
        setItemCogs("");
        setItemImages([]);
        setItemAvailable(true);
        setItemOptions([]);
        setItemAllergens([]);
        setItemDietaryTags([]);
        setItemSortOrder(0);
        setEditingItem(null);
        setNewAllergen("");
        setNewDietaryTag("");
        setNewOptionName("");
        setNewOptionPrice("");
    };

    const populateItemForm = (item: any) => {
        setItemName(item.name || "");
        setItemDescription(item.description || "");
        // #184: seed with fixed 2dp ("12.00") so the edit field matches list $12.00.
        setItemPrice(
            item.price != null && item.price !== "" && Number.isFinite(Number(item.price))
                ? seedMoneyField(item.price)
                : "",
        );
        setItemCogs(
            item.cogs != null && item.cogs !== "" && Number(item.cogs) > 0
                ? seedMoneyField(item.cogs)
                : "",
        );
        // Items can have a legacy singular `image` and/or a plural `images` array.
        // The modal renders MultipleImageUpload from itemImages, so seed it with
        // whatever the item actually has — promoting the singular image if the
        // array is empty so existing images stay visible after opening the modal.
        const existingImages: string[] =
            Array.isArray(item.images) && item.images.length > 0
                ? item.images
                : item.image
                    ? [item.image]
                    : [];
        setItemImages(existingImages);
        // #727: is_available is the EFFECTIVE flag (inventory can turn it off).
        // Seeding the toggle from it would let a plain Save on an out-of-stock
        // dish persist a manual 86 that outlives the restock.
        setItemAvailable(menuItemManualAvailable(item));
        setItemOptions(item.options || []);
        setItemAllergens(item.allergens || []);
        setItemDietaryTags(item.dietary_tags || []);
        setItemSortOrder(item.sort_order || 0);
    };

    const populateCategoryForm = (category: any) => {
        setCategoryName(category.name);
        setCategoryDescription(category.description);
    };

    // Handlers
    const onAddCategoryClick = () => {
        if (guardTranslatedEdit()) return;
        resetCategoryForm();
        onAddCategoryOpen();
    };

    const onEditCategoryClick = (categoryIndex: number) => {
        if (guardTranslatedEdit()) return;
        const category = menu[categoryIndex];
        if (!category) return;
        setEditingCategory(category);
        setSelectedCategoryIndex(categoryIndex);
        populateCategoryForm(category);
        onEditCategoryOpen();
    };

    const onAddCategorySubmit = async () => {
        const success = await handleAddCategory(categoryName, categoryDescription);
        if (success) {
            resetCategoryForm();
            onAddCategoryOpenChange();
        }
    };

    const onUpdateCategorySubmit = async () => {
        const success = await handleUpdateCategory(categoryName, categoryDescription);
        if (success) {
            resetCategoryForm();
            onEditCategoryOpenChange();
        }
    };

    const onAddItemClick = (categoryIndex: number) => {
        if (guardTranslatedEdit()) return;
        setSelectedCategoryIndex(categoryIndex);
        resetItemForm();
        onAddItemOpen();
    };

    const onEditItemClick = (categoryIndex: number, itemIndex: number) => {
        if (guardTranslatedEdit()) return;
        const item = menu[categoryIndex]?.items?.[itemIndex];
        if (!item) return;
        setEditingItem(item);
        setSelectedCategoryIndex(categoryIndex);
        setSelectedItemIndex(itemIndex);
        populateItemForm(item);
        onEditItemOpen();
    };

    const onAddItemSubmit = async () => {
        // Keep the singular `image` in sync with the first entry of `images`
        // so consumers that only read one or the other still see the upload.
        // Derive solely from itemImages[0] (no `|| itemImage` fallback): when the
        // operator removes all photos, itemImages is empty and the singular image
        // must clear too — otherwise a stale legacy `itemImage` resurrects the
        // just-removed photo (R3-MB-6).
        const primaryImage = itemImages[0] || "";
        const parsedCogs = parseLocaleDecimal(itemCogs);
        const newItem = {
            name: itemName,
            description: itemDescription,
            // parseLocaleDecimal (not parseFloat) so a comma-decimal operator
            // ("5,50" in es/es-AR) saves $5.50 — parseFloat("5,50") === 5 would
            // truncate the cents. Matches what the modal validates against.
            price: asDollars(parseLocaleDecimal(itemPrice)),
            cogs:
                itemCogs.trim() !== "" && Number.isFinite(parsedCogs) && parsedCogs > 0
                    ? asDollars(parsedCogs)
                    : asDollars(0),
            image: primaryImage,
            images: itemImages,
            is_available: itemAvailable,
            options: itemOptions,
            allergens: itemAllergens,
            dietary_tags: itemDietaryTags,
            sort_order: itemSortOrder,
            currency: defaultCurrency,
        };
        const success = await handleAddItem(newItem);
        if (success) {
            resetItemForm();
            onAddItemOpenChange();
        }
    };

    const onUpdateItemSubmit = async () => {
        // Solely from itemImages[0] — see onAddItemSubmit (R3-MB-6). Removing all
        // photos must clear the singular `image`, not fall back to the legacy one.
        const primaryImage = itemImages[0] || "";
        const parsedCogs = parseLocaleDecimal(itemCogs);
        const updatedItem = {
            ...editingItem!,
            name: itemName,
            description: itemDescription,
            // parseLocaleDecimal (not parseFloat) so a comma-decimal operator
            // ("5,50" in es/es-AR) saves $5.50 — parseFloat("5,50") === 5 would
            // truncate the cents. Matches what the modal validates against.
            price: asDollars(parseLocaleDecimal(itemPrice)),
            cogs:
                itemCogs.trim() !== "" && Number.isFinite(parsedCogs) && parsedCogs > 0
                    ? asDollars(parsedCogs)
                    : asDollars(0),
            image: primaryImage,
            images: itemImages,
            is_available: itemAvailable,
            options: itemOptions,
            allergens: itemAllergens,
            dietary_tags: itemDietaryTags,
            sort_order: itemSortOrder,
            currency: defaultCurrency,
        };
        const success = await handleUpdateItem(updatedItem);
        if (success) {
            resetItemForm();
            onEditItemOpenChange();
        }
    };

    const onConfirmManualSanitization = async () => {
        const kind = await confirmPendingSanitization();
        if (!kind) return;
        if (kind === "add-category") {
            resetCategoryForm();
            onAddCategoryOpenChange();
        } else if (kind === "update-category") {
            resetCategoryForm();
            onEditCategoryOpenChange();
        } else if (kind === "add-item") {
            resetItemForm();
            onAddItemOpenChange();
        } else {
            resetItemForm();
            onEditItemOpenChange();
        }
    };

    const openAIFeature = (feature: "pdf" | "wizard") => {
        setAiInitialTab(feature);
        setAiOnboardingOpen(true);
    };

    // The menu endpoint's item_orderability map is the authoritative display
    // state. Keep the canonical menu untouched so the edit form still controls
    // the manual availability flag, and project orderability only into the
    // rendered/filterable copy.
    const orderableMenu = useMemo(
        () => menu.map((category, sourceCategoryIndex) => ({
            ...category,
            // Category ids are absent in legacy menus. Projection clones would
            // otherwise destroy reference-based index resolution, so carry the
            // canonical index explicitly through subsequent filter clones.
            sourceCategoryIndex,
            items: category.items.map((item) => {
                const decision = item.id ? itemOrderability[item.id] : undefined;
                if (!decision) return item;
                // Canonical manual 86 wins over a stale item_orderability map
                // (one-tap 86 patches local state before the next menu fetch).
                if (!menuItemManualAvailable(item)) {
                    return {
                        ...item,
                        is_available: false,
                        manual_available: false,
                        orderability_state: "manual_disabled" as const,
                    };
                }
                // Inventory/orderability is stamped separately so a sellable
                // dish (or one on a live ticket) is not painted 86'd.
                return {
                    ...item,
                    orderability_state: decision.state,
                };
            }),
        })),
        [itemOrderability, menu],
    );

    // Filter Logic (uses the debounced query so keystrokes don't thrash the memo)
    const filteredMenu = useMemo(() => {
        if (!debouncedSearchQuery && searchFilter === "all") return orderableMenu;
        const query = debouncedSearchQuery.toLowerCase();

        return orderableMenu.map(category => {
            const filteredItems = category.items.filter(item => {
                const matchesQuery = item.name.toLowerCase().includes(query) ||
                    (item.description && item.description.toLowerCase().includes(query));
                const unavailable = isUnavailableForFilter(item);
                const matchesFilter = searchFilter === "all" ||
                    (searchFilter === "available" && !unavailable) ||
                    (searchFilter === "unavailable" && unavailable);
                return matchesQuery && matchesFilter;
            });

            return {
                ...category,
                items: filteredItems,
            };
        }).filter(category =>
            category.items.length > 0 ||
            (query.length > 0 && category.name.toLowerCase().includes(query)),
        );
    }, [orderableMenu, debouncedSearchQuery, searchFilter]);

    const loadInventoryStatuses = useCallback(async () => {
        try {
            const summary = await inventoryApi.getSummary(businessId);
            setInventoryStatuses(
                Object.fromEntries(
                    (summary.menu_item_statuses || []).map((status) => [status.menu_item_id, status]),
                ),
            );
        } catch (error) {
            console.error("Failed to load inventory statuses:", error);
            setInventoryStatuses({});
        }
    }, [businessId]);

    useEffect(() => {
        loadInventoryStatuses().catch((err) => console.error("loadInventoryStatuses failed:", err));
    }, [loadInventoryStatuses]);

    // Include empty categories so a newly created category bumps the header
    // category stat immediately (filteredMenu already drops non-matching empties
    // during search) [L3-10].
    const searchResultsCount = useMemo(
        () => countMenuSearchResults(filteredMenu),
        [filteredMenu],
    );

    const hasLanguageChanges = useMemo(() => {
        const currentCodes = businessLanguages.map((bl) => bl.language_code).sort();
        const selectedCodes = selectedLanguages.slice().sort();
        const currentDefault = businessLanguages.find(
            (bl) => bl.is_default,
        )?.language_code;

        return (
            JSON.stringify(currentCodes) !== JSON.stringify(selectedCodes) ||
            currentDefault !== defaultLanguage
        );
    }, [businessLanguages, selectedLanguages, defaultLanguage]);

    // Item Option Handlers
    const addOption = () => {
        // The modal now gates the Add button on a finite, non-negative parse and
        // surfaces inline feedback. This stays as a defensive backstop (locale-
        // tolerant: comma-decimal locales type "5,50"); parseLocaleDecimal mirrors
        // what the modal validated against, so a slipped-through "-5" still no-ops.
        if (!newOptionName.trim() || !newOptionPrice.trim()) return;
        const price = parseLocaleDecimal(newOptionPrice);
        if (!Number.isFinite(price) || price < 0) return;
        const option: MenuItemOption = {
            id: `option-${Date.now()}-${Math.random().toString(36).substring(2, 11)}`,
            name: newOptionName.trim(),
            price_change: asDollars(price),
            is_required: false,
        };
        setItemOptions([...itemOptions, option]);
        setNewOptionName("");
        setNewOptionPrice("");
    };
    const removeOption = (index: number) => setItemOptions(itemOptions.filter((_, i) => i !== index));

    // Allergen Handlers
    const toggleAllergen = (allergen: string) => {
        if (itemAllergens.includes(allergen)) setItemAllergens(itemAllergens.filter(a => a !== allergen));
        else setItemAllergens([...itemAllergens, allergen]);
    };
    const removeAllergen = (allergen: string) => setItemAllergens(itemAllergens.filter(a => a !== allergen));

    // Dietary Tag Handlers
    const toggleDietaryTag = (tag: string) => {
        if (itemDietaryTags.includes(tag)) setItemDietaryTags(itemDietaryTags.filter(t => t !== tag));
        else setItemDietaryTags([...itemDietaryTags, tag]);
    };
    const removeDietaryTag = (tag: string) => setItemDietaryTags(itemDietaryTags.filter(t => t !== tag));

    return (
        <DashboardTabShell
            header={{
                title: tString("title"),
                subtitle: tString("subtitle"),
                // Only surface counts once a search/filter has narrowed the
                // menu — matches the prior header's `totalItems > 0` gate.
                stats:
                    searchResultsCount.totalItems > 0
                        ? [
                              {
                                  label: tString(
                                      searchResultsCount.totalItems === 1
                                          ? "header.item"
                                          : "header.items",
                                  ),
                                  value: searchResultsCount.totalItems,
                              },
                              {
                                  label: tString(
                                      searchResultsCount.totalCategories === 1
                                          ? "header.category"
                                          : "header.categories",
                                  ),
                                  value: searchResultsCount.totalCategories,
                              },
                              {
                                  label: tString(
                                      (businessLanguages.length ||
                                          selectedLanguages.length ||
                                          1) === 1
                                          ? "header.language"
                                          : "header.languages",
                                  ),
                                  value:
                                      businessLanguages.length ||
                                      selectedLanguages.length ||
                                      1,
                              },
                          ]
                        : undefined,
                actions: (
                    <>
                        {/* Language chrome only applies to the editable Menu
                            catalogue — hide on Offers/Bundles/Engineering (#186). */}
                        {selectedTab === "menu" ? (
                            <LanguagesPopover
                                tString={tString}
                                isLocked={!entitlementConfirmed}
                                supportedLanguages={supportedLanguages}
                                businessLanguages={businessLanguages}
                                selectedLanguages={selectedLanguages}
                                setSelectedLanguages={setSelectedLanguages}
                                defaultLanguage={defaultLanguage}
                                setDefaultLanguage={setDefaultLanguage}
                                isLanguageLoading={isLanguageLoading}
                                hasLanguageChanges={hasLanguageChanges}
                                handleLanguageUpdate={handleLanguageUpdate}
                            />
                        ) : null}
                        {selectedTab === "menu" ? (
                            <PrintMenuButton
                                tString={tString}
                                isDisabled={
                                    !entitlementConfirmed ||
                                    !menu.some(
                                        (c) => (c.items?.length ?? 0) > 0,
                                    )
                                }
                                onPress={() => setIsPrintStudioOpen(true)}
                            />
                        ) : null}
                        {selectedTab === "menu" ? (
                            <AddSourceMenu
                                tString={tString}
                                onAddCategory={onAddCategoryClick}
                                onAIWizard={() => openAIFeature("wizard")}
                                onPDFImport={() => openAIFeature("pdf")}
                            />
                        ) : null}
                    </>
                ),
            }}
            // One loading affordance: the category-shaped skeleton covers the
            // tier check AND the menu/languages fetch, replacing both prior
            // early returns (the tier check used to show a bare text loader).
            loading={
                (accessLoading || !languagesLoaded || isMenuLoading) &&
                menu.length === 0 &&
                !loadFailed ? (
                    <MenuLoadingSkeleton />
                ) : null
            }
            tabs={{
                items: [
                    { key: "menu", label: tString("tabs.menu"), icon: Utensils },
                    { key: "offers", label: tString("tabs.offers"), icon: Tag },
                    { key: "bundles", label: tString("tabs.bundles"), icon: Package },
                    { key: "engineering", label: tString("tabs.engineering"), icon: BarChart3 },
                ],
                activeKey: selectedTab,
                onChange: setSelectedTab,
                ariaLabel: tString("tabs.ariaLabel"),
            }}
        >
            {/* Menu-item search + availability filter + grid/list toggle only
                apply to the Menu tab. Rendering it on Offers/Bundles/Engineering
                showed controls that do nothing there (F14). */}
            {selectedTab === "menu" && (
                <MenuPageHeader
                    tString={tString}
                    searchQuery={searchQuery}
                    setSearchQuery={setSearchQuery}
                    searchFilter={searchFilter}
                    setSearchFilter={setSearchFilter}
                    viewMode={viewMode}
                    setViewMode={persistViewMode}
                    searchResultsCount={searchResultsCount}
                />
            )}

            {selectedTab === "menu" ? (
                <ActiveLanguagePills
                    tString={tString}
                    supportedLanguages={supportedLanguages}
                    businessLanguages={businessLanguages}
                    currentViewLanguage={currentViewLanguage}
                    setCurrentViewLanguage={setCurrentViewLanguage}
                    loadMenu={loadMenu}
                    handleTranslateMenu={handleTranslateMenu}
                    handleSyncAllTranslations={handleSyncAllTranslations}
                    cancelSyncAllTranslations={cancelSyncAllTranslations}
                    isTranslating={isTranslating}
                    syncProgress={syncProgress}
                    syncError={syncError}
                    defaultLanguage={defaultLanguage}
                />
            ) : null}

            <DashboardTabTransition tabKey={selectedTab}>
                {selectedTab === "menu" && (
                    <PremiumPanel
                        className="p-4 sm:p-6"
                        withTexture={false}
                        data-testid="menu-tab-panel-menu"
                    >
                        {!isDefaultLanguageView && (
                            <div className="mb-4 rounded-2xl border border-amber-200 bg-amber-50/80 px-4 py-2.5 text-sm text-amber-800">
                                {interpolateTranslatedViewNotice(
                                    tString("translatedViewNotice"),
                                    viewedLanguageNativeName,
                                    defaultLanguageNativeName,
                                )}
                            </div>
                        )}
                        {loadFailed && menu.length > 0 ? (
                            <div
                                data-testid="menu-stale-catalog"
                                className="mb-4 flex items-center justify-between gap-3 rounded-2xl border border-amber-200 bg-amber-50/80 px-4 py-2.5 text-sm text-amber-800"
                            >
                                <span>{tString("loadFailedStale")}</span>
                                <button
                                    type="button"
                                    onClick={() => {
                                        void loadMenu(currentViewLanguage);
                                    }}
                                    className="whitespace-nowrap font-medium underline hover:text-amber-900"
                                >
                                    {tString("retryLoad")}
                                </button>
                            </div>
                        ) : null}
                        <MenuList
                            menu={menu}
                            setMenu={setMenu}
                            filteredMenu={filteredMenu}
                            businessId={businessId}
                            tString={tString}
                            searchQuery={searchQuery}
                            searchFilter={searchFilter}
                            onAddCategoryOpen={onAddCategoryClick}
                            onAddItemOpen={onAddItemClick}
                            handleEditCategory={onEditCategoryClick}
                            handleDeleteCategory={(index) => {
                                if (guardTranslatedEdit()) return;
                                setConfirmTitle(tString("categories.deleteCategory"));
                                setConfirmDescription(tString("categories.confirmDelete"));
                                setConfirmAction(() => () => handleDeleteCategory(index));
                                onConfirmOpen();
                            }}
                            handleEditItem={onEditItemClick}
                            handleDeleteItem={(catIndex, itemIndex) => {
                                if (guardTranslatedEdit()) return;
                                setConfirmTitle(tString("items.deleteItem"));
                                setConfirmDescription(tString("items.confirmDelete") || "Are you sure you want to delete this item?");
                                setConfirmAction(() => () => handleDeleteItem(catIndex, itemIndex));
                                onConfirmOpen();
                            }}
                            handleToggleEightySix={(catIndex, itemIndex) => {
                                if (guardTranslatedEdit()) return;
                                void handleToggleEightySix(catIndex, itemIndex);
                            }}
                            eightySixBusy={isSavingItem}
                            viewMode={viewMode}
                            clearSearch={() => { setSearchQuery(""); setSearchFilter("all"); }}
                            inventoryStatuses={inventoryStatuses}
                            onOpenAIFeature={openAIFeature}
                            menuVersion={menuVersion}
                            setMenuVersion={setMenuVersion}
                            loadMenu={loadMenu}
                            loadFailed={loadFailed}
                            currentViewLanguage={currentViewLanguage}
                            canMutate={isDefaultLanguageView}
                            editLanguageName={defaultLanguageNativeName}
                        />
                    </PremiumPanel>
                )}
                {selectedTab === "offers" && (
                    <PremiumPanel
                        className="p-4 sm:p-6"
                        withTexture={false}
                        data-testid="menu-tab-panel-offers"
                    >
                        <OffersManager businessId={businessId} />
                    </PremiumPanel>
                )}
                {selectedTab === "bundles" && (
                    <PremiumPanel
                        className="p-4 sm:p-6"
                        withTexture={false}
                        data-testid="menu-tab-panel-bundles"
                    >
                        <BundlesManager businessId={businessId} />
                    </PremiumPanel>
                )}
                {/* Engineering is suggestion-only (#131): no one-click reprice
                    or staged ProposalCard from this surface. */}
                {selectedTab === "engineering" && (
                    <div data-testid="menu-tab-panel-engineering">
                        <MenuEngineeringMatrix
                            report={meReport}
                            loading={meLoading}
                            currency={defaultCurrency}
                            period={mePeriod}
                            onPeriodChange={handleMePeriodChange}
                            widerPeriodWithSales={meWiderPeriod}
                            formatMoney={meFormatMoney}
                            labels={meLabels}
                        />
                    </div>
                )}
            </DashboardTabTransition>

            <AIMenuOnboardingModal
                tString={tString}
                isOpen={aiOnboardingOpen}
                onClose={() => setAiOnboardingOpen(false)}
                businessId={businessId}
                onImportComplete={() => {
                    loadMenu();
                    setSelectedTab("menu");
                    setAiOnboardingOpen(false);
                }}
                initialTab={aiInitialTab}
            />

            {sanitizationReview ? (
                // L3-3: keep the review above the form; suspend the AddItem modal
                // so Esc/focus do not close the form underneath.
                <div className="fixed inset-0 z-[200] flex items-center justify-center bg-ink-950/60 p-4 backdrop-blur-sm">
                    <div className="w-full max-w-3xl">
                        <MenuSanitizationReview
                            report={sanitizationReview.report}
                            translate={tString}
                            onConfirm={() => { void onConfirmManualSanitization(); }}
                            onBack={cancelPendingSanitization}
                            isConfirming={isConfirmingSanitization}
                        />
                    </div>
                </div>
            ) : null}

            {business && (
                <PrintMenuStudio
                    isOpen={isPrintStudioOpen}
                    onClose={() => setIsPrintStudioOpen(false)}
                    business={business}
                    categories={menu}
                    defaultCurrency={defaultCurrency}
                    languages={businessLanguages}
                    supportedLanguages={supportedLanguages}
                    currentViewLanguage={currentViewLanguage}
                    defaultLanguage={defaultLanguage}
                    businessId={businessId}
                    tString={tString}
                />
            )}

            <AddCategoryModal
                isOpen={isAddCategoryOpen}
                onOpenChange={onAddCategoryOpenChange}
                categoryName={categoryName}
                setCategoryName={setCategoryName}
                categoryDescription={categoryDescription}
                setCategoryDescription={setCategoryDescription}
                onAddCategory={onAddCategorySubmit}
                onResetForm={resetCategoryForm}
                tString={tString}
            />

            <EditCategoryModal
                isOpen={isEditCategoryOpen}
                onOpenChange={onEditCategoryOpenChange}
                categoryName={categoryName}
                setCategoryName={setCategoryName}
                categoryDescription={categoryDescription}
                setCategoryDescription={setCategoryDescription}
                onUpdateCategory={onUpdateCategorySubmit}
                onResetForm={resetCategoryForm}
                tString={tString}
            />

            <AddItemModal
                // L3-3: suspend (close) the form modal while sanitization review
                // is up so Esc cannot dismiss the form under the alertdialog.
                isOpen={isAddItemOpen && !sanitizationReview}
                onOpenChange={onAddItemOpenChange}
                selectedCategoryIndex={selectedCategoryIndex}
                menu={menu}
                businessId={businessId}
                defaultCurrency={defaultCurrency}
                itemName={itemName}
                setItemName={setItemName}
                itemDescription={itemDescription}
                setItemDescription={setItemDescription}
                itemPrice={itemPrice}
                setItemPrice={setItemPrice}
                itemCogs={itemCogs}
                setItemCogs={setItemCogs}
                itemImages={itemImages}
                setItemImages={setItemImages}
                itemAvailable={itemAvailable}
                setItemAvailable={setItemAvailable}
                itemSortOrder={itemSortOrder}
                setItemSortOrder={setItemSortOrder}
                itemOptions={itemOptions}
                newOptionName={newOptionName}
                setNewOptionName={setNewOptionName}
                newOptionPrice={newOptionPrice}
                setNewOptionPrice={setNewOptionPrice}
                onAddOption={addOption}
                onRemoveOption={removeOption}
                itemAllergens={itemAllergens}
                newAllergen={newAllergen}
                setNewAllergen={setNewAllergen}
                onAddAllergen={toggleAllergen}
                onRemoveAllergen={removeAllergen}
                itemDietaryTags={itemDietaryTags}
                newDietaryTag={newDietaryTag}
                setNewDietaryTag={setNewDietaryTag}
                onAddDietaryTag={toggleDietaryTag}
                onRemoveDietaryTag={removeDietaryTag}
                onAddItem={onAddItemSubmit}
                onResetForm={resetItemForm}
                tString={tString}
                onGenerateBreakdown={handleGenerateBreakdown}
                onGeneratePhoto={handleGeneratePhoto}
                onEnhancePhoto={handleEnhancePhoto}
                isGeneratingBreakdown={isGeneratingBreakdown}
                isGeneratingPhoto={isGeneratingPhoto}
                isEnhancing={isEnhancingPhoto}
                isSaving={isSavingItem}
                dailyLimitReached={dailyLimitReached}
            />

            <EditItemModal
                isOpen={isEditItemOpen}
                onOpenChange={onEditItemClose}
                selectedCategoryIndex={selectedCategoryIndex}
                menu={menu}
                businessId={businessId}
                defaultCurrency={defaultCurrency}
                itemName={itemName}
                setItemName={setItemName}
                itemDescription={itemDescription}
                setItemDescription={setItemDescription}
                itemPrice={itemPrice}
                setItemPrice={setItemPrice}
                itemCogs={itemCogs}
                setItemCogs={setItemCogs}
                itemImages={itemImages}
                setItemImages={setItemImages}
                itemAvailable={itemAvailable}
                setItemAvailable={setItemAvailable}
                itemSortOrder={itemSortOrder}
                setItemSortOrder={setItemSortOrder}
                itemOptions={itemOptions}
                newOptionName={newOptionName}
                setNewOptionName={setNewOptionName}
                newOptionPrice={newOptionPrice}
                setNewOptionPrice={setNewOptionPrice}
                onAddOption={addOption}
                onRemoveOption={removeOption}
                itemAllergens={itemAllergens}
                newAllergen={newAllergen}
                setNewAllergen={setNewAllergen}
                onAddAllergen={toggleAllergen}
                onRemoveAllergen={removeAllergen}
                itemDietaryTags={itemDietaryTags}
                newDietaryTag={newDietaryTag}
                setNewDietaryTag={setNewDietaryTag}
                onAddDietaryTag={toggleDietaryTag}
                onRemoveDietaryTag={removeDietaryTag}
                onUpdateItem={onUpdateItemSubmit}
                onResetForm={resetItemForm}
                tString={tString}
                onGenerateBreakdown={handleGenerateBreakdown}
                onGeneratePhoto={handleGeneratePhoto}
                onEnhancePhoto={handleEnhancePhoto}
                isGeneratingBreakdown={isGeneratingBreakdown}
                isGeneratingPhoto={isGeneratingPhoto}
                isEnhancing={isEnhancingPhoto}
                isSaving={isSavingItem}
                dailyLimitReached={dailyLimitReached}
            />

            <ConfirmationModal
                isOpen={isConfirmOpen}
                onOpenChange={onConfirmOpenChange}
                title={confirmTitle}
                description={confirmDescription}
                onConfirm={confirmAction}
                isDanger
                confirmLabel={tString("buttons.delete") || "Delete"}
                cancelLabel={tString("buttons.cancel") || "Cancel"}
            />
        </DashboardTabShell>
    );
}
