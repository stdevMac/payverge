import React from 'react';
import { MenuCategory } from '../../../../api/business';
import { InventoryMenuItemStatus } from '../../../../api/inventory';
import { DndContext, closestCenter } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CategoryCard } from './CategoryCard';
import { EmptySearchResults, EmptyMenu, EmptyMenuLoadFailed } from './EmptyStates';
import { emptyMenuSurface } from '../hooks/lastGoodOperatorMenu';
import { useMenuDragDrop } from '../hooks/useMenuDragDrop';

interface MenuListProps {
    menu: MenuCategory[];
    setMenu: React.Dispatch<React.SetStateAction<MenuCategory[]>>;
    filteredMenu: MenuCategory[];
    businessId: number;
    tString: (key: string) => string;
    searchQuery: string;
    searchFilter: "all" | "available" | "unavailable";
    onAddCategoryOpen: () => void;
    onAddItemOpen: (catIdx: number) => void;
    handleEditCategory: (catIdx: number) => void;
    handleDeleteCategory: (catIdx: number) => void;
    handleEditItem: (catIdx: number, itemIdx: number) => void;
    handleDeleteItem: (catIdx: number, itemIdx: number) => void;
    handleToggleEightySix?: (catIdx: number, itemIdx: number) => void;
    eightySixBusy?: boolean;
    viewMode: "grid" | "list";
    clearSearch: () => void;
    inventoryStatuses?: Record<string, InventoryMenuItemStatus>;
    onOpenAIFeature?: (feature: "pdf" | "wizard") => void;
    // Optimistic-lock version + reload plumbing for the CAS-guarded reorder save.
    menuVersion: number;
    setMenuVersion: (version: number) => void;
    loadMenu: (language?: string) => Promise<void>;
    loadFailed?: boolean;
    currentViewLanguage?: string;
    // False while viewing a non-default language: structural mutations
    // (drag-reorder) are disabled so translated text can't be written back into
    // the canonical menu (R3-MB-2).
    canMutate?: boolean;
    /**
     * L3-41: native name of the default (editable) language. Threaded down so
     * the blocked-edit tooltips can interpolate `{language}`.
     */
    editLanguageName?: string;
}

export function MenuList({
    menu,
    setMenu,
    filteredMenu,
    businessId,
    tString,
    searchQuery,
    searchFilter,
    onAddCategoryOpen,
    onAddItemOpen,
    handleEditCategory,
    handleDeleteCategory,
    handleEditItem,
    handleDeleteItem,
    handleToggleEightySix,
    eightySixBusy = false,
    viewMode,
    clearSearch,
    inventoryStatuses = {},
    onOpenAIFeature,
    menuVersion,
    setMenuVersion,
    loadMenu,
    loadFailed = false,
    currentViewLanguage,
    canMutate = true,
    editLanguageName,
}: MenuListProps) {
    const { sensors, handleDragEnd } = useMenuDragDrop({
        businessId,
        menu,
        setMenu,
        menuVersion,
        setMenuVersion,
        loadMenu,
        currentViewLanguage,
        reorderErrorText: tString("reorderError"),
    });

    // L3-11: screen-reader announcements for keyboard drag (list view only —
    // grid cards are non-sortable). Fall back to English if keys are missing.
    const accessibility = React.useMemo(() => {
        const ann = (key: string, fallback: string) => {
            const v = tString(key);
            return v === key ? fallback : v;
        };
        return {
            announcements: {
                onDragStart({ active }: { active: { id: string | number } }) {
                    return ann("dnd.lifted", "Picked up {id}").replace(
                        "{id}",
                        String(active.id),
                    );
                },
                onDragOver({
                    active,
                    over,
                }: {
                    active: { id: string | number };
                    over: { id: string | number } | null;
                }) {
                    if (!over) {
                        return ann(
                            "dnd.noDropTarget",
                            "{id} is no longer over a droppable area",
                        ).replace("{id}", String(active.id));
                    }
                    return ann("dnd.movedOver", "{id} was moved over {over}")
                        .replace("{id}", String(active.id))
                        .replace("{over}", String(over.id));
                },
                onDragEnd({
                    active,
                    over,
                }: {
                    active: { id: string | number };
                    over: { id: string | number } | null;
                }) {
                    if (!over) {
                        return ann("dnd.dropped", "{id} was dropped").replace(
                            "{id}",
                            String(active.id),
                        );
                    }
                    return ann("dnd.droppedOn", "{id} was dropped on {over}")
                        .replace("{id}", String(active.id))
                        .replace("{over}", String(over.id));
                },
                onDragCancel({ active }: { active: { id: string | number } }) {
                    return ann(
                        "dnd.cancelled",
                        "Dragging was cancelled. {id} was dropped.",
                    ).replace("{id}", String(active.id));
                },
            },
        };
    }, [tString]);

    const surface = emptyMenuSurface({
        categoryCount: Array.isArray(menu) ? menu.length : 0,
        loadFailed,
    });
    if (surface === "retry") {
        return (
            <EmptyMenuLoadFailed
                tString={tString}
                onRetry={() => {
                    void loadMenu(currentViewLanguage);
                }}
            />
        );
    }
    if (surface === "empty") {
        return (
            <EmptyMenu
                tString={tString}
                onAddCategoryOpen={onAddCategoryOpen}
                onOpenAIFeature={onOpenAIFeature}
            />
        );
    }

    if (filteredMenu.length === 0) {
        return <EmptySearchResults tString={tString} clearSearch={clearSearch} />;
    }

    return (
        <div className="space-y-6">
            <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                // No onDragCancel handler: local order only commits in
                // onDragEnd when `over` is set, so cancel needs no cleanup.
                // (The accessibility.announcements.onDragCancel below is a
                // separate, load-bearing screen-reader string.)
                onDragEnd={handleDragEnd}
                accessibility={accessibility}
            >
                <SortableContext
                    items={menu.map((c, idx) =>
                        c.id ? `cat-${c.id}` : `cat-pos-${idx}`,
                    )}
                    strategy={verticalListSortingStrategy}
                >
                    {filteredMenu.map((category, categoryIndex) => {
                        // Resolve the original category index. Match by stable id
                        // first; else fall back to reference equality (filtered
                        // categories are the same object refs as the originals),
                        // so two identically-named categories can't both resolve
                        // to the first (R3-MB-5).
                        const projectedSourceIndex = (category as MenuCategory & {
                            sourceCategoryIndex?: number;
                        }).sourceCategoryIndex;
                        const originalCategoryIndex =
                            typeof projectedSourceIndex === "number"
                                ? projectedSourceIndex
                                : category.id
                                  ? menu.findIndex((c) => c.id === category.id)
                                  : menu.indexOf(category) !== -1
                                    ? menu.indexOf(category)
                                    : menu.findIndex(
                                          (c) => c.name === category.name,
                                      );
                        // Reorder disabled while searching/filtering (indices
                        // don't map) OR while viewing a non-default language
                        // (canMutate=false) so translated text can't be dragged
                        // back into the canonical menu (R3-MB-2).
                        const isDraggable =
                            !searchQuery && searchFilter === "all" && canMutate;

                        return (
                            <CategoryCard
                                key={
                                    category.id
                                        ? `cat-${category.id}`
                                        : `cat-pos-${originalCategoryIndex}`
                                }
                                category={category} // Pass filtered category items
                                categoryIndex={categoryIndex}
                                originalCategoryIndex={originalCategoryIndex}
                                tString={tString}
                                onAddItemOpen={onAddItemOpen}
                                handleEditCategory={handleEditCategory}
                                handleDeleteCategory={handleDeleteCategory}
                                handleEditItem={handleEditItem}
                                handleDeleteItem={handleDeleteItem}
                                handleToggleEightySix={handleToggleEightySix}
                                eightySixBusy={eightySixBusy}
                                viewMode={viewMode}
                                isDraggable={isDraggable}
                                menu={menu}
                                inventoryStatuses={inventoryStatuses}
                                canMutate={canMutate}
                                editLanguageName={editLanguageName}
                            />
                        )
                    })}
                </SortableContext>
            </DndContext>
        </div>
    );
}
