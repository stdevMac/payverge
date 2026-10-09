import {
    KeyboardSensor,
    PointerSensor,
    useSensor,
    useSensors,
    DragEndEvent,
} from "@dnd-kit/core";
import {
    sortableKeyboardCoordinates,
    arrayMove,
} from "@dnd-kit/sortable";
import toast from "react-hot-toast";
import { businessApi, MenuCategory } from '../../../../api/business';

interface UseMenuDragDropProps {
    businessId: number;
    menu: MenuCategory[];
    setMenu: React.Dispatch<React.SetStateAction<MenuCategory[]>>;
    // Optimistic-lock version + setter so the reorder write is CAS-guarded and
    // the bumped version is kept in sync (a discarded version would make the
    // next edit hit a false 409).
    menuVersion: number;
    setMenuVersion: (version: number) => void;
    // Reload the current view after a reorder failure (409/other) so the UI
    // snaps back to the server truth instead of showing the failed local move.
    loadMenu: (language?: string) => Promise<void>;
    // The language currently being viewed — used to reload in the right locale.
    currentViewLanguage?: string;
    // Localized error string for a failed reorder save.
    reorderErrorText?: string;
}

export function useMenuDragDrop({
    businessId,
    menu,
    setMenu,
    menuVersion,
    setMenuVersion,
    loadMenu,
    currentViewLanguage,
    reorderErrorText,
}: UseMenuDragDropProps) {
    // Persist a single reorder move via the granular /menu/reorder endpoint under
    // optimistic CAS (§3.7 fix 4) — the server applies just this move to the
    // stored tree, so a drag no longer uploads the entire menu document. On
    // success keep the bumped version; on failure toast + reload so the view
    // returns to server truth. categoryId is required for scope "item".
    const persistReorder = (
        scope: "category" | "item",
        from: number,
        to: number,
        categoryId?: string,
    ) => {
        businessApi
            .reorderMenu(businessId, {
                scope,
                category_id: categoryId,
                from,
                to,
                version: menuVersion,
            })
            .then((res) => {
                if (typeof res.version === "number") setMenuVersion(res.version);
            })
            .catch((err) => {
                console.error("Failed to save menu order:", err);
                if (reorderErrorText) toast.error(reorderErrorText);
                loadMenu(currentViewLanguage).catch((e) =>
                    console.error("reload after failed reorder failed:", e),
                );
            });
    };

    const sensors = useSensors(
        useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
        useSensor(KeyboardSensor, {
            coordinateGetter: sortableKeyboardCoordinates,
        })
    );

    const handleDragEnd = (event: DragEndEvent) => {
        const { active, over } = event;

        if (!over) return;

        // Resolve positions from the dnd-kit data payload (category/item index)
        // rather than parsing the sortable id string. Earlier the ids were
        // name-derived composites ("categoryName:itemName"), which broke for
        // duplicate names (the wrong row moved or the move no-op'd) and for any
        // name containing a colon (mis-parsed → reorder silently dropped).
        const activeData = active.data.current as
            | { type?: string; categoryIndex?: number; itemIndex?: number }
            | undefined;
        const overData = over.data.current as
            | { type?: string; categoryIndex?: number; itemIndex?: number }
            | undefined;

        // Handle Category sorting
        if (activeData?.type === "category" && overData?.type === "category") {
            const oldIndex = activeData.categoryIndex;
            const newIndex = overData.categoryIndex;
            if (
                typeof oldIndex === "number" &&
                typeof newIndex === "number" &&
                oldIndex !== newIndex
            ) {
                setMenu((items) => arrayMove(items, oldIndex, newIndex));
                persistReorder("category", oldIndex, newIndex);
            }
            return;
        }

        // Handle Item sorting
        if (activeData?.type === "item" && overData?.type === "item") {
            const sourceCategoryIndex = activeData.categoryIndex;
            const targetCategoryIndex = overData.categoryIndex;
            const sourceItemIndex = activeData.itemIndex;
            const targetItemIndex = overData.itemIndex;

            // Only allow reordering within the same category for now.
            if (
                typeof sourceCategoryIndex === "number" &&
                typeof targetCategoryIndex === "number" &&
                typeof sourceItemIndex === "number" &&
                typeof targetItemIndex === "number" &&
                sourceCategoryIndex === targetCategoryIndex &&
                sourceItemIndex !== targetItemIndex
            ) {
                const categoryId = menu[sourceCategoryIndex]?.id;
                setMenu((prevMenu) => {
                    const newMenu = [...prevMenu];
                    const category = { ...newMenu[sourceCategoryIndex] };
                    category.items = arrayMove(
                        category.items,
                        sourceItemIndex,
                        targetItemIndex,
                    );
                    newMenu[sourceCategoryIndex] = category;
                    return newMenu;
                });
                persistReorder(
                    "item",
                    sourceItemIndex,
                    targetItemIndex,
                    categoryId,
                );
            }
        }
    };

    return {
        sensors,
        handleDragEnd,
    };
}
