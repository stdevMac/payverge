import React, { useMemo, useState } from 'react';
import { MenuCategory, MenuItem } from '../../../../api/business';
import { InventoryMenuItemStatus } from '../../../../api/inventory';
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { SortableItemWrapper, MenuItemCard } from './MenuItemCard';
import { Plus, Edit, Trash2, ChevronDown, MoreVertical } from "lucide-react";
import {
    Dropdown,
    DropdownItem,
    DropdownMenu,
    DropdownTrigger,
} from "@nextui-org/react";
import { SortableCategoryCard } from '../../dnd/SortableComponents';
import { interpolateEditBlockedTooltip } from '../editBlockedTooltip';
import {
    buildMenuItemIndex,
    inventoryStatusForMenuItem,
    liveMenuRefsFromIndex,
} from '../../inventory/inventoryEightySix';

// Stable, unique sortable id for a menu item. Prefer the item's own id; fall
// back to a positional id keyed by category+item index when the id is absent
// (e.g. a just-added item). Never name-derived, so duplicate names or names
// containing a colon can't collide or mis-parse.
function sortableItemId(
    categoryIndex: number,
    item: MenuItem,
    itemIndex: number,
): string {
    return item.id ? `item-${item.id}` : `pos-${categoryIndex}-${itemIndex}`;
}

// Resolve the index of a (filtered) item back in the ORIGINAL category's item
// list. Match by stable id first; when the item has no id, fall back to
// reference equality (indexOf) — filtered items are the same object refs as the
// originals (filteredMenu is built with Array.filter over the same objects), so
// indexOf targets the exact row. Name(+price) matching mis-targeted duplicates
// (two identically-named/-priced items would both resolve to the first) (R3-MB-5).
function resolveOriginalItemIndex(
    originalItems: MenuItem[] | undefined,
    item: MenuItem,
    fallbackIndex: number,
): number {
    if (!originalItems) return fallbackIndex;
    if (item.id) {
        const byId = originalItems.findIndex((o) => o.id === item.id);
        if (byId !== -1) return byId;
    }
    const byRef = originalItems.indexOf(item);
    return byRef !== -1 ? byRef : fallbackIndex;
}

interface CategoryCardProps {
    category: MenuCategory & { items: MenuItem[] };
    categoryIndex: number;
    originalCategoryIndex: number;
    tString: (key: string) => string;
    onAddItemOpen: (catIdx: number) => void;
    handleEditCategory: (catIdx: number) => void;
    handleDeleteCategory: (catIdx: number) => void;
    handleEditItem: (catIdx: number, itemIdx: number) => void;
    handleDeleteItem: (catIdx: number, itemIdx: number) => void;
    handleToggleEightySix?: (catIdx: number, itemIdx: number) => void;
    eightySixBusy?: boolean;
    viewMode: "grid" | "list";
    isDraggable: boolean;
    menu: MenuCategory[];
    inventoryStatuses?: Record<string, InventoryMenuItemStatus>;
    /** L3-7: false while viewing a translated language — mutate actions disabled. */
    canMutate?: boolean;
    /**
     * L3-41: native name of the language to switch to for edits, interpolated
     * into the `translatedEditBlocked` tooltip template.
     */
    editLanguageName?: string;
}

export function CategoryCard({
    category,
    categoryIndex,
    originalCategoryIndex,
    tString,
    onAddItemOpen,
    handleEditCategory,
    handleDeleteCategory,
    handleEditItem,
    handleDeleteItem,
    handleToggleEightySix,
    eightySixBusy = false,
    viewMode,
    isDraggable,
    menu,
    inventoryStatuses = {},
    canMutate = true,
    editLanguageName,
}: CategoryCardProps) {
    // Collapsible body (§3.7 fix 9): a large menu no longer renders every item of
    // every category at once — the operator can collapse categories they are not
    // editing. Collapsed by default off (expanded) to preserve current behavior.
    const [collapsed, setCollapsed] = useState(false);
    const liveMenuItems = useMemo(
        () => liveMenuRefsFromIndex(buildMenuItemIndex(menu)),
        [menu],
    );
    const mutateBlockedTitle = canMutate
        ? undefined
        : interpolateEditBlockedTooltip(
              tString("translatedEditBlocked"),
              editLanguageName,
          );

    const categoryContent = (
        <section className="group/cat rounded-3xl border border-warm-200/80 bg-white/70 p-4 shadow-[0_18px_46px_rgba(46,42,37,0.055)] backdrop-blur-sm sm:p-5">
            <div className="pointer-events-none -mx-4 -mt-4 mb-4 h-px bg-white/90 sm:-mx-5 sm:-mt-5" />
            <header className="mb-4 flex flex-col gap-3 border-b border-warm-200/80 pb-4 sm:flex-row sm:items-end sm:justify-between">
                <div className="min-w-0">
                    <div className="flex flex-wrap items-baseline gap-2.5">
                        <h3 className="text-lg font-semibold tracking-tight text-ink-950 sm:text-xl">
                            {category.name}
                        </h3>
                        <span className="inline-flex h-6 items-center rounded-full border border-brand/15 bg-brand/5 px-2.5 text-[11px] font-semibold text-brand-dark">
                            {category.items.length}{" "}
                            {category.items.length === 1
                                ? tString("categories.itemSingular")
                                : tString("categories.itemPlural")}
                        </span>
                    </div>
                    {category.description && (
                        <p className="mt-1 truncate text-sm text-ink-600">
                            {category.description}
                        </p>
                    )}
                </div>
                <div className="flex flex-shrink-0 items-center gap-1.5">
                    <button
                        type="button"
                        onClick={(e) => {
                            e.preventDefault();
                            setCollapsed((c) => !c);
                        }}
                        aria-expanded={!collapsed}
                        title={collapsed ? tString("categories.expand") : tString("categories.collapse")}
                        aria-label={`${collapsed ? tString("categories.expand") : tString("categories.collapse")} ${category.name}`}
                        className="inline-flex h-11 w-11 [@media(hover:hover)_and_(pointer:fine)]:h-8 [@media(hover:hover)_and_(pointer:fine)]:w-8 items-center justify-center rounded-xl text-ink-600 transition hover:bg-warm-100 hover:text-ink-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                    >
                        <ChevronDown
                            className={`w-4 h-4 transition-transform ${collapsed ? "-rotate-90" : ""}`}
                        />
                    </button>
                    <button
                        type="button"
                        onClick={(e) => {
                            e.preventDefault();
                            if (!canMutate) return;
                            onAddItemOpen(originalCategoryIndex);
                        }}
                        disabled={!canMutate}
                        title={mutateBlockedTitle}
                        className="inline-flex h-8 items-center gap-1 rounded-xl bg-brand px-3 text-xs font-semibold text-white shadow-sm shadow-brand/20 transition hover:-translate-y-px hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand active:translate-y-0 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        <Plus className="w-3.5 h-3.5" />
                        {tString("buttons.addItem")}
                    </button>
                    <button
                        type="button"
                        onClick={(e) => {
                            e.preventDefault();
                            if (!canMutate) return;
                            handleEditCategory(originalCategoryIndex);
                        }}
                        disabled={!canMutate}
                        title={mutateBlockedTitle ?? tString("buttons.edit")}
                        aria-label={`${tString("buttons.edit")} ${category.name}`}
                        className="inline-flex h-11 w-11 [@media(hover:hover)_and_(pointer:fine)]:h-8 [@media(hover:hover)_and_(pointer:fine)]:w-8 items-center justify-center rounded-xl text-ink-600 opacity-70 transition hover:bg-warm-100 hover:text-ink-950 group-hover/cat:opacity-100 focus:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        <Edit className="w-3.5 h-3.5" />
                    </button>
                    {/* Delete tucked into overflow so it is not next to Edit (#116). */}
                    <Dropdown placement="bottom-end" offset={4}>
                        <DropdownTrigger>
                            <button
                                type="button"
                                disabled={!canMutate}
                                title={mutateBlockedTitle ?? tString("buttons.moreActions")}
                                aria-label={`${tString("buttons.moreActions")} ${category.name}`}
                                data-testid="category-more-actions"
                                className="inline-flex h-11 w-11 [@media(hover:hover)_and_(pointer:fine)]:h-8 [@media(hover:hover)_and_(pointer:fine)]:w-8 items-center justify-center rounded-xl text-ink-600 opacity-70 transition hover:bg-warm-100 hover:text-ink-950 group-hover/cat:opacity-100 focus:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:cursor-not-allowed disabled:opacity-50"
                            >
                                <MoreVertical className="w-3.5 h-3.5" />
                            </button>
                        </DropdownTrigger>
                        <DropdownMenu
                            aria-label={`${tString("buttons.moreActions")} ${category.name}`}
                            onAction={(key) => {
                                if (key === "delete" && canMutate) {
                                    handleDeleteCategory(originalCategoryIndex);
                                }
                            }}
                        >
                            <DropdownItem
                                key="delete"
                                className="text-rose-600"
                                color="danger"
                                startContent={<Trash2 className="h-3.5 w-3.5" />}
                                aria-label={`${tString("buttons.delete")} ${category.name}`}
                                data-testid="category-delete-action"
                            >
                                {tString("buttons.delete")}
                            </DropdownItem>
                        </DropdownMenu>
                    </Dropdown>
                </div>
            </header>

            {/* Items — hidden when the category is collapsed (§3.7 fix 9). */}
            {!collapsed && (category.items.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-brand/20 bg-brand/5 p-6 text-center shadow-panel-highlight">
                    <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl border border-white/80 bg-white/80 shadow-sm shadow-warm-300/30">
                        <Plus className="w-5 h-5 text-brand" strokeWidth={1.75} />
                    </div>
                    <p className="mx-auto mb-4 max-w-sm text-sm text-ink-600">
                        {tString("noItems")}
                    </p>
                    <button
                        type="button"
                        onClick={(e) => {
                            e.preventDefault();
                            if (!canMutate) return;
                            onAddItemOpen(originalCategoryIndex);
                        }}
                        disabled={!canMutate}
                        title={mutateBlockedTitle}
                        className="inline-flex h-9 items-center gap-1.5 rounded-xl bg-brand px-4 text-xs font-semibold text-white shadow-sm shadow-brand/20 transition hover:-translate-y-px hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand active:translate-y-0 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        <Plus className="w-3.5 h-3.5" />
                        {tString("buttons.addItem")}
                    </button>
                </div>
            ) : isDraggable && viewMode === "list" ? (
                <div className="space-y-2">
                    <SortableContext
                        items={category.items.map((i, itemIndex) => {
                            const originalItemIndex = resolveOriginalItemIndex(
                                menu[originalCategoryIndex]?.items,
                                i,
                                itemIndex,
                            );
                            return sortableItemId(
                                originalCategoryIndex,
                                i,
                                originalItemIndex,
                            );
                        })}
                        strategy={verticalListSortingStrategy}
                    >
                        {category.items.map((item, itemIndex) => {
                            const originalItemIndex = resolveOriginalItemIndex(
                                menu[originalCategoryIndex]?.items,
                                item,
                                itemIndex,
                            );
                            return (
                                <SortableItemWrapper
                                    // Stable, unique sortable id — prefer the
                                    // item's own id; fall back to a positional
                                    // id so duplicate names or colons can't
                                    // collide or mis-parse.
                                    key={sortableItemId(
                                        originalCategoryIndex,
                                        item,
                                        originalItemIndex,
                                    )}
                                    id={sortableItemId(
                                        originalCategoryIndex,
                                        item,
                                        originalItemIndex,
                                    )}
                                    categoryIndex={originalCategoryIndex}
                                    itemIndex={originalItemIndex}
                                >
                                    <MenuItemCard
                                        item={item}
                                        originalCategoryIndex={originalCategoryIndex}
                                        originalItemIndex={originalItemIndex}
                                        tString={tString}
                                        handleEditItem={handleEditItem}
                                        handleDeleteItem={handleDeleteItem}
                                        handleToggleEightySix={handleToggleEightySix}
                                        eightySixBusy={eightySixBusy}
                                        viewMode={viewMode}
                                        inventoryStatus={inventoryStatusForMenuItem(
                                            item,
                                            inventoryStatuses,
                                            liveMenuItems,
                                        )}
                                        canMutate={canMutate}
                                        editLanguageName={editLanguageName}
                                    />
                                </SortableItemWrapper>
                            );
                        })}
                    </SortableContext>
                </div>
            ) : viewMode === "list" ? (
                <div className="space-y-2">
                    {category.items.map((item, itemIndex) => {
                        const originalItemIndex = resolveOriginalItemIndex(
                            menu[originalCategoryIndex]?.items,
                            item,
                            itemIndex,
                        );
                        return (
                            <MenuItemCard
                                key={itemIndex}
                                item={item}
                                originalCategoryIndex={originalCategoryIndex}
                                originalItemIndex={originalItemIndex}
                                tString={tString}
                                handleEditItem={handleEditItem}
                                handleDeleteItem={handleDeleteItem}
                                handleToggleEightySix={handleToggleEightySix}
                                eightySixBusy={eightySixBusy}
                                viewMode={viewMode}
                                inventoryStatus={inventoryStatusForMenuItem(
                                    item,
                                    inventoryStatuses,
                                    liveMenuItems,
                                )}
                                canMutate={canMutate}
                                editLanguageName={editLanguageName}
                            />
                        );
                    })}
                </div>
            ) : (
                <div>
                    {/* Item drag-reorder is only available in list view. Surface
                        a one-time hint (first category, when reordering is
                        otherwise possible) so the capability is discoverable. */}
                    {isDraggable && categoryIndex === 0 && (
                        <p className="mb-2 text-xs font-medium text-ink-500">
                            {tString("reorderItemsHint")}
                        </p>
                    )}
                    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
                    {category.items.map((item, itemIndex) => {
                        const originalItemIndex = resolveOriginalItemIndex(
                            menu[originalCategoryIndex]?.items,
                            item,
                            itemIndex,
                        );
                        return (
                            <MenuItemCard
                                key={itemIndex}
                                item={item}
                                originalCategoryIndex={originalCategoryIndex}
                                originalItemIndex={originalItemIndex}
                                tString={tString}
                                handleEditItem={handleEditItem}
                                handleDeleteItem={handleDeleteItem}
                                handleToggleEightySix={handleToggleEightySix}
                                eightySixBusy={eightySixBusy}
                                viewMode={viewMode}
                                inventoryStatus={inventoryStatusForMenuItem(
                                    item,
                                    inventoryStatuses,
                                    liveMenuItems,
                                )}
                                canMutate={canMutate}
                                editLanguageName={editLanguageName}
                            />
                        );
                    })}
                    </div>
                </div>
            ))}
        </section>
    );

    if (isDraggable) {
        return (
            <SortableCategoryCard
                id={
                    category.id
                        ? `cat-${category.id}`
                        : `cat-pos-${originalCategoryIndex}`
                }
                data={{ type: "category", categoryIndex: originalCategoryIndex }}
                ariaLabel={`${tString("buttons.reorderCategory")} ${category.name}`}
            >
                <div className="mb-8">{categoryContent}</div>
            </SortableCategoryCard>
        );
    }

    return <div className="mb-8">{categoryContent}</div>;
}
