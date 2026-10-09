import { useRef, useState } from 'react';
import toast from 'react-hot-toast';
import { businessApi, MenuCategory, MenuItem, MenuSanitizeReport } from '../../../../api/business';
import { menuItemManualAvailable } from '../menuAvailabilityFilter';

export type ManualMenuMutationKind =
    | "add-category"
    | "update-category"
    | "add-item"
    | "update-item";

export interface ManualMenuSanitizationReview {
    kind: ManualMenuMutationKind;
    report: MenuSanitizeReport;
}

export interface DailyLimitInfo {
    dailyLimit: number;
    resetsInSeconds: number;
}

/** Reads the fair-use rejection off an axios error, or null for anything else. */
function readDailyLimit(err: unknown): DailyLimitInfo | null {
    const data = (err as { response?: { data?: Record<string, unknown> } })?.response?.data;
    if (data?.code !== "image_daily_limit_reached") return null;
    return {
        dailyLimit: Number(data.daily_limit ?? 0),
        resetsInSeconds: Number(data.resets_in_seconds ?? 0),
    };
}

interface UseMenuMutationsProps {
    businessId: number;
    menu: MenuCategory[];
    menuVersion: number;
    setMenuVersion: (version: number) => void;
    // Patch menu state in place after a mutation (§3.7 fix 3) so an add/update/
    // delete no longer forces a full-menu re-download.
    setMenu: React.Dispatch<React.SetStateAction<MenuCategory[]>>;
    loadMenu: (language?: string) => Promise<void>;
    setError: (error: string | null) => void;
    defaultCurrency: string;
    tString: (key: string) => string;
    // The language currently being viewed. Mutations reload in this locale so a
    // successful edit doesn't snap the view back to the default while the
    // language pill stays active (R3-MB-7).
    currentViewLanguage?: string;
    defaultLanguage?: string;
}

export function useMenuMutations({
    businessId,
    menu,
    menuVersion,
    setMenuVersion,
    setMenu,
    loadMenu,
    setError,
    defaultCurrency,
    tString,
    currentViewLanguage,
    defaultLanguage,
}: UseMenuMutationsProps) {
    // Reload the menu in whatever locale is currently being viewed. Falls back
    // to the default language, then to undefined (server default).
    const reloadInView = () =>
        loadMenu(currentViewLanguage || defaultLanguage || undefined);

    // True when the source-language menu is on screen. The mutation echo carries
    // the operator's source text; patching it in place is correct only in that
    // view. When a translated language is active, we still reload so the freshly
    // written machine translation is rendered (source echo would show untranslated
    // text). Default: patch (no language configured means default view).
    const viewingSourceLanguage = () =>
        !currentViewLanguage ||
        !defaultLanguage ||
        currentViewLanguage === defaultLanguage;
    // Selection states (for editing)
    const [selectedCategoryIndex, setSelectedCategoryIndex] = useState<number | null>(null);
    const [selectedItemIndex, setSelectedItemIndex] = useState<number | null>(null);
    const [editingCategory, setEditingCategory] = useState<MenuCategory | null>(null);
    const [editingItem, setEditingItem] = useState<MenuItem | null>(null);
    const [isGeneratingBreakdown, setIsGeneratingBreakdown] = useState(false);
    const [isGeneratingPhoto, setIsGeneratingPhoto] = useState(false);
    const [isEnhancingPhoto, setIsEnhancingPhoto] = useState(false);
    const [dailyLimitReached, setDailyLimitReached] = useState<DailyLimitInfo | null>(null);
    const [sanitizationReview, setSanitizationReview] = useState<ManualMenuSanitizationReview | null>(null);
    const [isConfirmingSanitization, setIsConfirmingSanitization] = useState(false);
    const pendingSanitizationRef = useRef<{
        kind: ManualMenuMutationKind;
        retry: () => Promise<boolean>;
    } | null>(null);
    // In-flight guard for the item add/update save. Without it a double-click
    // fires two CAS writes; the loser 409s and shows a scary conflict toast even
    // though the operator only meant to save once (R3-MB double-submit).
    const [isSavingItem, setIsSavingItem] = useState(false);

    const handleVersionConflict = () => {
        setError(tString("versionConflict"));
        reloadInView().catch((err) => console.error("loadMenu failed:", err));
    };

    const stageSanitizationReview = (
        kind: ManualMenuMutationKind,
        report: MenuSanitizeReport | undefined,
        retry: () => Promise<boolean>,
    ) => {
        if (!report) {
            setError(tString("sanitization.invalidResponse"));
            return;
        }
        pendingSanitizationRef.current = { kind, retry };
        setSanitizationReview({ kind, report });
    };

    const cancelPendingSanitization = () => {
        pendingSanitizationRef.current = null;
        setSanitizationReview(null);
    };

    const confirmPendingSanitization = async (): Promise<ManualMenuMutationKind | null> => {
        const pending = pendingSanitizationRef.current;
        if (!pending || isConfirmingSanitization) return null;
        setIsConfirmingSanitization(true);
        try {
            const saved = await pending.retry();
            if (!saved) return null;
            pendingSanitizationRef.current = null;
            setSanitizationReview(null);
            return pending.kind;
        } finally {
            setIsConfirmingSanitization(false);
        }
    };

    const handleAddCategory = async (name: string, description: string, confirmSanitization = false) => {
        try {
            const newCategory: MenuCategory = {
                name,
                description,
                items: [],
            };
            const result = await businessApi.addMenuCategory(
                businessId,
                newCategory,
                menuVersion,
                confirmSanitization,
            );
            if (result.requires_confirmation) {
                stageSanitizationReview(
                    "add-category",
                    result.sanitization,
                    () => handleAddCategory(name, description, true),
                );
                return false;
            }
            if (result.version) setMenuVersion(result.version);
            // Patch in place from the echoed category (fix 3). Reload only when a
            // translated language is on screen so the translation is fetched.
            if (result.category && viewingSourceLanguage()) {
                setMenu((prev) => [...prev, result.category as MenuCategory]);
            } else {
                await reloadInView();
            }
            return true;
        } catch (error: unknown) {
            const err = error as { response?: { status?: number } };
            if (err.response?.status === 409) {
                handleVersionConflict();
                return false;
            }
            console.error("Failed to add category:", error);
            setError(tString("categories.addError"));
            return false;
        }
    };

    const handleUpdateCategory = async (name: string, description: string, confirmSanitization = false) => {
        if (selectedCategoryIndex === null || !editingCategory) return false;
        try {
            const category = menu[selectedCategoryIndex];
            const updatedCategory: MenuCategory = {
                ...editingCategory,
                name,
                description,
                items: category.items,
            };
            const result = await businessApi.updateMenuCategory(
                businessId, selectedCategoryIndex, updatedCategory,
                menuVersion, category.id, confirmSanitization,
            );
            if (result.requires_confirmation) {
                stageSanitizationReview(
                    "update-category",
                    result.sanitization,
                    () => handleUpdateCategory(name, description, true),
                );
                return false;
            }
            if (result.version) setMenuVersion(result.version);
            if (result.category && viewingSourceLanguage()) {
                const idx = selectedCategoryIndex;
                setMenu((prev) => prev.map((c, i) =>
                    i === idx
                        // Keep existing items (echo carries the posted items, which
                        // already match) but adopt the server-confirmed name/desc/id.
                        ? { ...(result.category as MenuCategory), items: c.items }
                        : c,
                ));
            } else {
                await reloadInView();
            }
            return true;
        } catch (error: unknown) {
            const err = error as { response?: { status?: number } };
            if (err.response?.status === 409) {
                handleVersionConflict();
                return false;
            }
            console.error("Failed to update category:", error);
            setError(tString("categories.updateError"));
            return false;
        }
    };

    const handleDeleteCategory = async (categoryIndex: number) => {
        try {
            const category = menu[categoryIndex];
            const result = await businessApi.deleteMenuCategory(
                businessId, categoryIndex,
                menuVersion, category?.id,
            );
            if (result.version) setMenuVersion(result.version);
            // A delete is locale-independent — patch in place unconditionally.
            setMenu((prev) => prev.filter((_, i) => i !== categoryIndex));
        } catch (error: unknown) {
            const err = error as { response?: { status?: number } };
            if (err.response?.status === 409) {
                handleVersionConflict();
                return;
            }
            console.error("Failed to delete category:", error);
            setError(tString("categories.deleteError"));
        }
    };

    const handleAddItem = async (itemData: MenuItem, confirmSanitization = false) => {
        if (selectedCategoryIndex === null) return false;
        if (isSavingItem) return false; // in-flight guard against double-submit
        setIsSavingItem(true);
        try {
            const category = menu[selectedCategoryIndex];
            const newItem = { ...itemData, currency: defaultCurrency };
            const result = await businessApi.addMenuItem(
                businessId, selectedCategoryIndex, newItem,
                menuVersion, category?.id, confirmSanitization,
            );
            if (result.requires_confirmation) {
                stageSanitizationReview(
                    "add-item",
                    result.sanitization,
                    () => handleAddItem(itemData, true),
                );
                return false;
            }
            if (result.version) setMenuVersion(result.version);
            if (result.item && viewingSourceLanguage()) {
                const idx = selectedCategoryIndex;
                setMenu((prev) => prev.map((c, i) =>
                    i === idx
                        ? { ...c, items: [...(c.items ?? []), result.item as MenuItem] }
                        : c,
                ));
            } else {
                await reloadInView();
            }
            return true;
        } catch (error: unknown) {
            const err = error as { response?: { status?: number } };
            if (err.response?.status === 409) {
                handleVersionConflict();
                return false;
            }
            console.error("Failed to add item:", error);
            setError(tString("items.addError"));
            return false;
        } finally {
            setIsSavingItem(false);
        }
    };

    const handleUpdateItem = async (itemData: MenuItem, confirmSanitization = false) => {
        if (selectedCategoryIndex === null || selectedItemIndex === null || !editingItem) return false;
        if (isSavingItem) return false; // in-flight guard against double-submit
        setIsSavingItem(true);
        try {
            const category = menu[selectedCategoryIndex];
            const item = category?.items?.[selectedItemIndex];
            const updatedItem = { ...editingItem, ...itemData, currency: defaultCurrency };
            const result = await businessApi.updateMenuItem(
                businessId, selectedCategoryIndex, selectedItemIndex, updatedItem,
                menuVersion, category?.id, item?.id, confirmSanitization,
            );
            if (result.requires_confirmation) {
                stageSanitizationReview(
                    "update-item",
                    result.sanitization,
                    () => handleUpdateItem(itemData, true),
                );
                return false;
            }
            if (result.version) setMenuVersion(result.version);
            if (result.item && viewingSourceLanguage()) {
                const cIdx = selectedCategoryIndex;
                const iIdx = selectedItemIndex;
                setMenu((prev) => prev.map((c, i) =>
                    i === cIdx
                        ? {
                            ...c,
                            items: (c.items ?? []).map((it, j) =>
                                j === iIdx ? (result.item as MenuItem) : it,
                            ),
                        }
                        : c,
                ));
            } else {
                await reloadInView();
            }
            return true;
        } catch (error: unknown) {
            const err = error as { response?: { status?: number } };
            if (err.response?.status === 409) {
                handleVersionConflict();
                return false;
            }
            console.error("Failed to update item:", error);
            setError(tString("items.updateError"));
            return false;
        } finally {
            setIsSavingItem(false);
        }
    };

    // First-class 86 / un-86 without opening the full Edit Item modal (issue 102).
    // Uses the canonical menu row (not the orderability projection) so restoring
    // after a manual 86 does not fight inventory_out badges.
    const patchItemAvailability = (
        categoryIndex: number,
        itemIndex: number,
        nextAvailable: boolean,
        echoed?: MenuItem,
    ) => {
        setMenu((prev) =>
            prev.map((c, i) =>
                i !== categoryIndex
                    ? c
                    : {
                        ...c,
                        items: (c.items ?? []).map((it, j) =>
                            j !== itemIndex
                                ? it
                                : {
                                    ...(echoed ?? it),
                                    is_available: nextAvailable,
                                    manual_available: nextAvailable,
                                },
                        ),
                    },
            ),
        );
    };

    const handleToggleEightySix = async (categoryIndex: number, itemIndex: number) => {
        const category = menu[categoryIndex];
        const item = category?.items?.[itemIndex];
        if (!item || isSavingItem) return false;
        // #727: is_available is now the effective flag — inventory can pull it
        // low while the operator switch is still on. Toggle the STORED switch,
        // or one tap on an out-of-stock dish would "restore" a dish that was
        // never manually 86'd and change nothing.
        const nextAvailable = !menuItemManualAvailable(item);
        setIsSavingItem(true);
        // Flip the card immediately so Saturday-rush 86 does not wait on reload.
        patchItemAvailability(categoryIndex, itemIndex, nextAvailable);
        try {
            const result = await businessApi.updateMenuItem(
                businessId,
                categoryIndex,
                itemIndex,
                {
                    ...item,
                    is_available: nextAvailable,
                    manual_available: nextAvailable,
                    currency: item.currency || defaultCurrency,
                },
                menuVersion,
                category?.id,
                item.id,
            );
            if (result.requires_confirmation) {
                patchItemAvailability(categoryIndex, itemIndex, !nextAvailable);
                // Availability-only flips should never need sanitization review.
                setError(tString("eightySix.error").replace("{message}", tString("sanitization.invalidResponse")));
                return false;
            }
            if (result.version) setMenuVersion(result.version);
            if (result.item) {
                patchItemAvailability(categoryIndex, itemIndex, nextAvailable, result.item as MenuItem);
            }
            // Reload so item_orderability matches the new manual flag (and any
            // concurrent inventory projection) instead of trusting a stale map.
            await reloadInView();
            const toastKey = nextAvailable ? "eightySix.restoreSuccess" : "eightySix.markSuccess";
            toast.success(tString(toastKey).replace("{name}", item.name));
            return true;
        } catch (error: unknown) {
            patchItemAvailability(categoryIndex, itemIndex, !nextAvailable);
            const err = error as { response?: { status?: number }; message?: string };
            if (err.response?.status === 409) {
                handleVersionConflict();
                return false;
            }
            setError(
                tString("eightySix.error").replace(
                    "{message}",
                    err.message || tString("items.updateError"),
                ),
            );
            return false;
        } finally {
            setIsSavingItem(false);
        }
    };

    const handleDeleteItem = async (categoryIndex: number, itemIndex: number) => {
        try {
            const category = menu[categoryIndex];
            const item = category?.items?.[itemIndex];
            const result = await businessApi.deleteMenuItem(
                businessId, categoryIndex, itemIndex,
                menuVersion, category?.id, item?.id,
            );
            if (result.version) setMenuVersion(result.version);
            // A delete is locale-independent — patch in place unconditionally.
            setMenu((prev) => prev.map((c, i) =>
                i === categoryIndex
                    ? { ...c, items: (c.items ?? []).filter((_, j) => j !== itemIndex) }
                    : c,
            ));
        } catch (error: unknown) {
            const err = error as { response?: { status?: number } };
            if (err.response?.status === 409) {
                handleVersionConflict();
                return;
            }
            console.error("Failed to delete item:", error);
            setError(tString("items.deleteError"));
        }
    };

    // Ingredient breakdown — exploded-view diagram from name + description.
    // dietaryTags (the item's current vegetarian/vegan/... selection) is
    // forwarded so the backend can add hard no-meat/no-animal-product
    // constraints and never hallucinate animal protein onto the dish (#588).
    // Labels and leader lines are stamped server-side from this same text.
    const handleGenerateBreakdown = async (
        name: string,
        description: string,
        dietaryTags: string[] = [],
    ): Promise<string | null> => {
        setIsGeneratingBreakdown(true);
        try {
            const data = await businessApi.generateMenuImage(businessId, {
                name,
                description,
                ingredients: description,
                dietary_tags: dietaryTags,
            });
            setDailyLimitReached(null);
            return data.url ?? null;
        } catch (e) {
            console.error(e);
            const limit = readDailyLimit(e);
            setDailyLimitReached(limit);
            if (!limit) {
                setError(tString("ai.imageGeneration.error"));
            }
            return null;
        } finally {
            setIsGeneratingBreakdown(false);
        }
    };

    // Generate a photo — realistic food photo from name + description (text-to-image).
    // dietaryTags travel to the backend so the photo prompt carries hard dietary
    // constraints and never hallucinates non-compliant ingredients (#601).
    const handleGeneratePhoto = async (
        name: string,
        description: string,
        dietaryTags: string[] = [],
    ): Promise<string | null> => {
        setIsGeneratingPhoto(true);
        try {
            const result = await businessApi.regenerateMenuItemImage(
                businessId,
                name,
                description,
                "Create an enhanced, photorealistic food image with premium lighting, sharper details, natural textures, and a clean restaurant-style composition.",
                dietaryTags,
            );
            setDailyLimitReached(null);
            return result.url ?? null;
        } catch (e) {
            console.error(e);
            const limit = readDailyLimit(e);
            setDailyLimitReached(limit);
            if (!limit) {
                setError(tString("ai.imageGeneration.error"));
            }
            return null;
        } finally {
            setIsGeneratingPhoto(false);
        }
    };

    // Enhance my photo — improve an existing uploaded photo (image-to-image).
    // dietaryTags: same dietary-safety threading as handleGeneratePhoto (#601).
    const handleEnhancePhoto = async (
        name: string,
        description: string,
        imageUrl: string,
        dietaryTags: string[] = [],
    ): Promise<string | null> => {
        setIsEnhancingPhoto(true);
        try {
            const result = await businessApi.enhanceMenuItemImage(businessId, imageUrl, name, description, dietaryTags);
            setDailyLimitReached(null);
            return result.url ?? null;
        } catch (e) {
            console.error(e);
            const limit = readDailyLimit(e);
            setDailyLimitReached(limit);
            if (!limit) {
                setError(tString("ai.imageEnhancement.error"));
            }
            return null;
        } finally {
            setIsEnhancingPhoto(false);
        }
    };


    return {
        selectedCategoryIndex,
        setSelectedCategoryIndex,
        selectedItemIndex,
        setSelectedItemIndex,
        editingCategory,
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
    };
}
