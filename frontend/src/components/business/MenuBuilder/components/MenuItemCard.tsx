import React, { useState, useCallback } from 'react';
import { MenuItem } from '../../../../api/business';
import { InventoryMenuItemStatus } from '../../../../api/inventory';
import { ALLERGENS, DIETARY_TAGS } from '../../../../constants/menu-tags';
import { formatCurrency } from '../../../../api/currency';
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import {
    Dropdown,
    DropdownItem,
    DropdownMenu,
    DropdownTrigger,
    Image,
    Popover,
    PopoverContent,
    PopoverTrigger,
    Tooltip,
} from "@nextui-org/react";
import { Ban, Edit, MoreVertical, RotateCcw, Trash2, GripVertical, Utensils } from "lucide-react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { interpolateEditBlockedTooltip } from "../editBlockedTooltip";
import {
    isManualEightySix as itemIsManualEightySix,
    isMenuItemInventoryOut,
    menuItemManualAvailable,
} from "../menuAvailabilityFilter";

interface MenuItemCardProps {
    item: MenuItem;
    originalCategoryIndex: number;
    originalItemIndex: number;
    tString: (key: string) => string;
    handleEditItem: (catIdx: number, itemIdx: number) => void;
    handleDeleteItem: (catIdx: number, itemIdx: number) => void;
    /** One-tap manual 86 / restore without opening Edit Item (#102). */
    handleToggleEightySix?: (catIdx: number, itemIdx: number) => void;
    eightySixBusy?: boolean;
    viewMode: "grid" | "list";
    inventoryStatus?: InventoryMenuItemStatus;
    // dnd-kit listeners/attributes applied to the grip handle ONLY, so the
    // grip starts the drag and the rest of the row opens edit-on-click.
    dragHandleProps?: React.HTMLAttributes<HTMLElement>;
    /** L3-7: false while viewing a translated language — edit/delete disabled. */
    canMutate?: boolean;
    /**
     * L3-41: native name of the language the operator must switch to in order
     * to edit. Interpolated into the `translatedEditBlocked` template.
     */
    editLanguageName?: string;
}

// Internal Sortable Wrap
function SortableItemWrapper({
    id,
    children,
    disabled,
    categoryIndex,
    itemIndex,
}: {
    id: string;
    children: React.ReactNode;
    disabled?: boolean;
    categoryIndex: number;
    itemIndex: number;
}) {
    const {
        attributes,
        listeners,
        setNodeRef,
        transform,
        transition,
        isDragging,
    } = useSortable({
        id,
        data: { type: "item", categoryIndex, itemIndex },
        disabled,
    });

    const style = {
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
        zIndex: isDragging ? 10 : 1,
    };

    // Listeners/attributes go to the child's grip handle (dragHandleProps), NOT
    // the whole row — so the grip drags and the row body opens edit-on-click.
    const dragHandleProps = { ...attributes, ...listeners };

    return (
        <div ref={setNodeRef} style={style}>
            {React.isValidElement(children)
                ? React.cloneElement(
                      children as React.ReactElement<{
                          dragHandleProps?: React.HTMLAttributes<HTMLElement>;
                      }>,
                      { dragHandleProps },
                  )
                : children}
        </div>
    );
}

interface ChipMeta {
    key: string;
    label: string;
    icon?: string;
    emoji?: string;
    tone: "warning" | "success";
}

function buildChipMeta(item: MenuItem, tString: (key: string) => string): ChipMeta[] {
    const chips: ChipMeta[] = [];

    for (const id of item.allergens || []) {
        const meta = ALLERGENS.find((a) => a.id === id);
        if (meta) {
            chips.push({
                key: `a-${id}`,
                label: tString(`items.${meta.name}`),
                icon: meta.icon,
                tone: "warning",
            });
        }
    }

    for (const id of item.dietary_tags || []) {
        const meta = DIETARY_TAGS.find((t) => t.id === id);
        if (meta) {
            chips.push({
                key: `d-${id}`,
                label: tString(`items.${meta.name}`),
                emoji: meta.emoji,
                tone: "success",
            });
        }
    }

    return chips;
}

// Canonical inventory/availability pill — single visual treatment used in both
// grid (image overlay) and list (inline before title). Replaces the older
// status-dot path so an item's state is always communicated the same way.
function StatusPill({
    showStatusBadge,
    isManualEightySix,
    inventoryWarning,
    inventoryOut,
    inventoryStatus,
    tString,
}: {
    showStatusBadge: boolean;
    isManualEightySix: boolean;
    inventoryWarning: boolean;
    inventoryOut: boolean;
    inventoryStatus?: InventoryMenuItemStatus;
    tString: (key: string) => string;
}) {
    if (showStatusBadge) {
        return (
            <span className="rounded-full border border-rose-200 bg-white/95 px-2 py-0.5 text-[10px] font-semibold text-rose-700 shadow-sm shadow-rose-900/10 backdrop-blur whitespace-nowrap">
                {tString(isManualEightySix ? "eightySix.badge" : "status.unavailable")}
            </span>
        );
    }
    if (inventoryWarning) {
        // item_orderability / inventory_status win over a remapped summary
        // row. blocks_sale alone used to paint "Unavailable · inventory" on
        // Harvest Bowl while steak stayed on Marcar 86 (#727).
        const inventoryBlocked =
            inventoryOut || inventoryStatus?.blocks_sale === true;
        const badgeKey =
            !inventoryOut && inventoryStatus?.status === "low_stock"
                ? "inventoryStatus.lowBadge"
                : inventoryBlocked
                  ? "inventoryStatus.outBadge"
                  : "inventoryStatus.warningBadge";
        return (
            <span
                className={`rounded-full border bg-white/95 px-2 py-0.5 text-[10px] font-semibold shadow-sm backdrop-blur whitespace-nowrap ${
                    inventoryBlocked
                        ? "border-rose-200 text-rose-700 shadow-rose-900/10"
                        : "border-amber-200 text-amber-800 shadow-amber-900/10"
                }`}
            >
                {tString(badgeKey)}
            </span>
        );
    }
    return null;
}

function ChipBadge({ chip }: { chip: ChipMeta }) {
    const toneClass =
        chip.tone === "warning"
            ? "bg-amber-50/90 text-amber-800 border-amber-200"
            : "bg-emerald-50/90 text-emerald-800 border-emerald-200";
    return (
        <Tooltip content={chip.label} placement="top" delay={300}>
            <span
                className={`inline-flex h-5 items-center gap-1 rounded-lg border px-1.5 text-[10px] font-medium shadow-panel-highlight ${toneClass}`}
            >
                {chip.icon ? (
                    <Image
                        src={chip.icon}
                        alt={chip.label}
                        width={10}
                        height={10}
                        className="w-2.5 h-2.5"
                    />
                ) : chip.emoji ? (
                    <span aria-hidden>{chip.emoji}</span>
                ) : null}
                <span className="leading-none">{chip.label}</span>
            </span>
        </Tooltip>
    );
}

/** +N overflow that reveals hidden allergen/dietary labels on click/tap (#185). */
function HiddenChipsOverflow({
    hidden,
    tString,
}: {
    hidden: ChipMeta[];
    tString: (key: string) => string;
}) {
    const list = hidden.map((c) => c.label).join(", ");
    const moreLabel = (tString("items.moreAllergens") || "Show {count} more")
        .replace("{count}", String(hidden.length));
    const hiddenLabel = (tString("items.hiddenAllergens") || "Hidden: {list}")
        .replace("{list}", list);
    return (
        <Popover placement="top" offset={6}>
            <PopoverTrigger>
                <button
                    type="button"
                    className="inline-flex h-5 items-center rounded-lg border border-warm-200 bg-warm-50 px-1.5 text-[10px] font-medium text-ink-600 transition hover:bg-warm-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                    aria-label={moreLabel}
                    title={hiddenLabel}
                    data-testid="menu-item-hidden-chips"
                    onClick={(e) => e.stopPropagation()}
                    onPointerDown={(e) => e.stopPropagation()}
                >
                    +{hidden.length}
                </button>
            </PopoverTrigger>
            <PopoverContent className="max-w-xs px-3 py-2">
                <p className="text-xs font-medium text-ink-800">{list}</p>
            </PopoverContent>
        </Popover>
    );
}

const actionBtnClass =
    "inline-flex h-11 w-11 [@media(hover:hover)_and_(pointer:fine)]:h-8 [@media(hover:hover)_and_(pointer:fine)]:w-8 items-center justify-center rounded-xl focus-visible:outline-none focus-visible:ring-2 disabled:cursor-not-allowed disabled:opacity-50";

const eightySixBtnClass =
    `${actionBtnClass} w-auto min-w-11 gap-1 px-2 [@media(hover:hover)_and_(pointer:fine)]:w-auto [@media(hover:hover)_and_(pointer:fine)]:px-1.5`;

export function MenuItemCard({
    item,
    originalCategoryIndex,
    originalItemIndex,
    tString,
    handleEditItem,
    handleDeleteItem,
    handleToggleEightySix,
    eightySixBusy = false,
    viewMode,
    inventoryStatus,
    dragHandleProps,
    canMutate = true,
    editLanguageName,
}: MenuItemCardProps) {
    // Thread the operator locale so es/es-AR businesses see comma-grouped money
    // ("US$ 36,08") instead of the en-US fallback ("$36.08").
    const { locale } = useSimpleLocale();
    const intlLocale = intlLocaleFor(locale);
    const chips = buildChipMeta(item, tString);
    const visibleChips = chips.slice(0, 3);
    const hiddenChipCount = chips.length - visibleChips.length;
    // 86'd badge is the manual flag only. Inventory empty uses inventory
    // badges so sellability and 86 share one rule.
    const isManualEightySix = itemIsManualEightySix(item);
    // #727: item.is_available is the EFFECTIVE flag now, so an inventory-blocked
    // dish reads false there. The badge gates below mean "not hand-pulled", so
    // they have to read the operator's own switch or the inventory badge would
    // suppress itself on exactly the dishes it exists for.
    const manualAvailable = menuItemManualAvailable(item);
    const showStatusBadge = isManualEightySix;
    const hasOrderability = Boolean(item.orderability_state);
    const orderabilityOut = isMenuItemInventoryOut(item);
    // isMenuItemInventoryOut already ignores leftover inventory_status when
    // orderability_state is present. Summary rows are fallback-only (#727).
    const inventoryOut = hasOrderability
        ? orderabilityOut
        : orderabilityOut ||
          (inventoryStatus?.status === "out_of_stock" &&
              inventoryStatus.blocks_sale === true);
    const inventoryWarning =
        !isManualEightySix &&
        (inventoryOut ||
            item.orderability_state === "inventory_warning" ||
            (!hasOrderability &&
                (inventoryStatus?.status === "out_of_stock" ||
                    inventoryStatus?.status === "low_stock") &&
                !!inventoryStatus?.shows_warning));
    const mutateBlockedTitle = canMutate
        ? undefined
        : interpolateEditBlockedTooltip(
              tString("translatedEditBlocked"),
              editLanguageName,
          );

    const handleEdit = (e: React.SyntheticEvent) => {
        e.stopPropagation();
        e.preventDefault();
        if (!canMutate) return;
        handleEditItem(originalCategoryIndex, originalItemIndex);
    };
    const handleDelete = () => {
        if (!canMutate) return;
        handleDeleteItem(originalCategoryIndex, originalItemIndex);
    };
    const handleEightySix = (e: React.SyntheticEvent) => {
        e.stopPropagation();
        e.preventDefault();
        if (!canMutate || eightySixBusy || !handleToggleEightySix) return;
        handleToggleEightySix(originalCategoryIndex, originalItemIndex);
    };
    // #727: inventory already 86s this dish, but the operator's own switch is
    // still on. "Mark 86" read as "this is sellable, tap to pull it" — right
    // next to the card's own "Unavailable · inventory" pill — and "86 anyway"
    // still never said WHY the dish was off. The control now names the block
    // ("No stock") and the only thing tapping it can still add (keeping the
    // dish off past the restock).
    const inventoryBlocksSale = inventoryOut && !isManualEightySix;
    const eightySixLabel = isManualEightySix
        ? tString("eightySix.restore")
        : inventoryBlocksSale
          ? tString("eightySix.markInventoryOut")
          : tString("eightySix.mark");
    const eightySixAria = (
        isManualEightySix
            ? tString("eightySix.restoreAria")
            : inventoryBlocksSale
              ? tString("eightySix.markInventoryOutAria")
              : tString("eightySix.markAria")
    ).replace("{name}", item.name);
    // Tooltip carries the "why" on the inventory-blocked variant; elsewhere the
    // label is already the whole story.
    const eightySixTitle = inventoryBlocksSale ? eightySixAria : eightySixLabel;
    const EightySixIcon = isManualEightySix ? RotateCcw : Ban;

    const primaryImage = (item.images && item.images.length > 0 ? item.images[0] : item.image) || "";
    const [imageFailed, setImageFailed] = useState(false);
    const handleImageError = useCallback(() => setImageFailed(true), []);
    const showPlaceholder = !primaryImage || imageFailed;

    if (viewMode === 'grid') {
        const hasOverlayBadges = showStatusBadge || (inventoryWarning && manualAvailable);
        return (
            <div
                className="group relative flex flex-col overflow-hidden rounded-2xl border border-warm-200/80 bg-white/90 shadow-[0_14px_38px_rgba(46,42,37,0.06)] transition-all duration-200 hover:-translate-y-0.5 hover:border-brand/30 hover:shadow-[0_20px_46px_rgba(26,107,106,0.12)]"
            >
                <div className="pointer-events-none absolute inset-x-0 top-0 z-[1] h-px bg-white/90" />
                {/* Always render an image area so cards in mixed (with/without
                    image) rows stay the same height. Empty state shows a soft
                    placeholder rather than the dominant gray slab from v1. */}
                <div className="relative aspect-[5/4] overflow-hidden bg-warm-100">
                    {!showPlaceholder ? (
                        <Image
                            src={primaryImage}
                            alt={item.name}
                            removeWrapper
                            className="h-full w-full object-cover transition duration-500 group-hover:scale-[1.035]"
                            onError={handleImageError}
                        />
                    ) : (
                        <div className="flex h-full w-full flex-col items-center justify-center gap-2 bg-[radial-gradient(circle_at_30%_18%,rgba(255,255,255,0.86),transparent_34%),linear-gradient(135deg,rgba(26,107,106,0.08),rgba(242,240,234,0.92))]">
                            <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-white/80 bg-white/60 shadow-sm shadow-warm-300/30">
                                <Utensils className="h-5 w-5 text-ink-400" />
                            </div>
                            <span className="text-[10px] font-medium text-ink-500 opacity-0 transition-opacity group-hover:opacity-100">
                                {tString("items.noImageHint") || "Click to add photo"}
                            </span>
                        </div>
                    )}
                    {hasOverlayBadges && (
                        <div className="absolute top-2 left-2 flex flex-wrap gap-1 pointer-events-none">
                            <StatusPill
                                showStatusBadge={showStatusBadge}
                                isManualEightySix={isManualEightySix}
                                inventoryWarning={!!inventoryWarning && manualAvailable}
                                inventoryOut={inventoryOut}
                                inventoryStatus={inventoryStatus}
                                tString={tString}
                            />
                        </div>
                    )}
                    {/* Persistently visible row actions (Task 35). Delete lives
                        in an overflow menu so it is not next to Edit (#116). */}
                    <div className="absolute top-2 right-2 z-20 flex gap-1">
                        {handleToggleEightySix ? (
                            <button
                                type="button"
                                onClick={handleEightySix}
                                onPointerDown={(e) => e.stopPropagation()}
                                disabled={!canMutate || eightySixBusy}
                                className={`${eightySixBtnClass} border border-white/70 bg-white/95 text-amber-800 shadow-sm backdrop-blur transition hover:-translate-y-px hover:bg-amber-50 focus-visible:ring-amber-300`}
                                title={mutateBlockedTitle ?? eightySixTitle}
                                aria-label={eightySixAria}
                            >
                                <EightySixIcon className="w-3.5 h-3.5" aria-hidden />
                                <span className="text-[10px] font-semibold leading-none">
                                    {eightySixLabel}
                                </span>
                            </button>
                        ) : null}
                        <button
                            type="button"
                            onClick={handleEdit}
                            disabled={!canMutate}
                            className={`${actionBtnClass} border border-white/70 bg-white/95 text-ink-600 shadow-sm backdrop-blur transition hover:-translate-y-px hover:bg-white hover:text-ink-950 focus-visible:ring-brand`}
                            title={mutateBlockedTitle ?? tString("buttons.edit")}
                            aria-label={`${tString("buttons.edit")} ${item.name}`}
                        >
                            <Edit className="w-3.5 h-3.5" />
                        </button>
                        <Dropdown placement="bottom-end" offset={4}>
                            <DropdownTrigger>
                                <button
                                    type="button"
                                    disabled={!canMutate}
                                    className={`${actionBtnClass} border border-white/70 bg-white/95 text-ink-600 shadow-sm backdrop-blur transition hover:-translate-y-px hover:bg-white hover:text-ink-950 focus-visible:ring-brand`}
                                    title={mutateBlockedTitle ?? tString("buttons.moreActions")}
                                    aria-label={`${tString("buttons.moreActions")} ${item.name}`}
                                    data-testid="menu-item-more-actions"
                                >
                                    <MoreVertical className="w-3.5 h-3.5" />
                                </button>
                            </DropdownTrigger>
                            <DropdownMenu
                                aria-label={`${tString("buttons.moreActions")} ${item.name}`}
                                onAction={(key) => {
                                    if (key === "delete") handleDelete();
                                }}
                            >
                                <DropdownItem
                                    key="delete"
                                    className="text-rose-600"
                                    color="danger"
                                    startContent={<Trash2 className="h-3.5 w-3.5" />}
                                    aria-label={`${tString("buttons.delete")} ${item.name}`}
                                    data-testid="menu-item-delete-action"
                                >
                                    {tString("buttons.delete")}
                                </DropdownItem>
                            </DropdownMenu>
                        </Dropdown>
                    </div>
                </div>

                {/* Body */}
                <div className="space-y-2 p-3.5">
                    <div className="flex items-baseline justify-between gap-2">
                        <h4 className="flex-1 truncate text-sm font-semibold text-ink-950">
                            {item.name}
                        </h4>
                        <span className="whitespace-nowrap text-sm font-semibold tabular-nums text-ink-950">
                            {formatCurrency(item.price || 0, item.currency || "USD", undefined, intlLocale)}
                        </span>
                    </div>
                    {chips.length > 0 && (
                        <div className="flex flex-wrap gap-1">
                            {visibleChips.map((chip) => (
                                <ChipBadge key={chip.key} chip={chip} />
                            ))}
                            {hiddenChipCount > 0 && (
                                <HiddenChipsOverflow
                                    hidden={chips.slice(3)}
                                    tString={tString}
                                />
                            )}
                        </div>
                    )}
                </div>
            </div>
        );
    }

    // List view — single row
    return (
        <div
            className="group flex items-center gap-3 rounded-2xl border border-warm-200/80 bg-white/90 p-2.5 shadow-[0_10px_26px_rgba(46,42,37,0.045)] transition-all duration-200 hover:-translate-y-0.5 hover:border-brand/30 hover:shadow-[0_16px_34px_rgba(26,107,106,0.10)]"
        >
            <div
                className="flex-shrink-0 cursor-grab rounded-xl p-1 text-ink-400 transition hover:bg-warm-100 hover:text-ink-700"
                // The grip is the drag handle: spread dnd-kit listeners here so a
                // drag starts from the grip. Stop click (but NOT pointerdown)
                // from bubbling so tapping the grip doesn't open edit.
                // L3-11: merge onKeyDown — a literal onKeyDown AFTER the spread
                // overwrote KeyboardSensor's activator. Call dnd-kit first.
                {...dragHandleProps}
                onClick={(e) => e.stopPropagation()}
                onKeyDown={(e) => {
                    const dndKeyDown = dragHandleProps?.onKeyDown;
                    if (typeof dndKeyDown === "function") {
                        dndKeyDown(e);
                    }
                }}
                role="button"
                aria-label={`${tString("buttons.reorder")} ${item.name}`}
            >
                <GripVertical className="w-4 h-4" />
            </div>

            <div className="h-12 w-12 flex-shrink-0 overflow-hidden rounded-xl border border-warm-200/70 bg-warm-100">
                {!showPlaceholder ? (
                    <Image
                        src={primaryImage}
                        alt={item.name}
                        removeWrapper
                        width={48}
                        height={48}
                        className="h-full w-full object-cover transition duration-300 group-hover:scale-[1.04]"
                        onError={handleImageError}
                    />
                ) : (
                    <div className="flex h-full w-full items-center justify-center bg-[radial-gradient(circle_at_30%_18%,rgba(255,255,255,0.86),transparent_38%),rgba(242,240,234,0.92)]">
                        <Utensils className="h-5 w-5 text-ink-400" />
                    </div>
                )}
            </div>

            <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-baseline gap-2">
                    {(showStatusBadge || inventoryWarning) && (
                        <StatusPill
                            showStatusBadge={showStatusBadge}
                            isManualEightySix={isManualEightySix}
                            inventoryWarning={!!inventoryWarning && manualAvailable}
                            inventoryOut={inventoryOut}
                            inventoryStatus={inventoryStatus}
                            tString={tString}
                        />
                    )}
                    <h4 className="truncate text-sm font-semibold text-ink-950">
                        {item.name}
                    </h4>
                    {chips.length > 0 && (
                        <div className="hidden md:flex items-center gap-1 ml-1">
                            {visibleChips.map((chip) => (
                                <ChipBadge key={chip.key} chip={chip} />
                            ))}
                            {hiddenChipCount > 0 && (
                                <HiddenChipsOverflow
                                    hidden={chips.slice(3)}
                                    tString={tString}
                                />
                            )}
                        </div>
                    )}
                </div>
                {item.description && (
                    <p className="mt-0.5 truncate text-xs text-ink-600">
                        {item.description}
                    </p>
                )}
            </div>

            <span className="whitespace-nowrap text-sm font-semibold tabular-nums text-ink-950">
                {formatCurrency(item.price || 0, item.currency || "USD", undefined, intlLocale)}
            </span>

            {/* Persistently visible list actions. Delete in overflow (#116). */}
            <div className="flex items-center gap-1">
                {handleToggleEightySix ? (
                    <button
                        type="button"
                        onClick={handleEightySix}
                        onPointerDown={(e) => e.stopPropagation()}
                        disabled={!canMutate || eightySixBusy}
                        className={`${eightySixBtnClass} text-amber-800 transition hover:bg-amber-50 focus-visible:ring-amber-300`}
                        title={mutateBlockedTitle ?? eightySixTitle}
                        aria-label={eightySixAria}
                    >
                        <EightySixIcon className="w-3.5 h-3.5" aria-hidden />
                        <span className="text-[10px] font-semibold leading-none">
                            {eightySixLabel}
                        </span>
                    </button>
                ) : null}
                <button
                    type="button"
                    onClick={handleEdit}
                    onPointerDown={(e) => e.stopPropagation()}
                    disabled={!canMutate}
                    className={`${actionBtnClass} text-ink-600 transition hover:bg-warm-100 hover:text-ink-950 focus-visible:ring-brand`}
                    title={mutateBlockedTitle ?? tString("buttons.edit")}
                    aria-label={`${tString("buttons.edit")} ${item.name}`}
                >
                    <Edit className="w-3.5 h-3.5" />
                </button>
                <Dropdown placement="bottom-end" offset={4}>
                    <DropdownTrigger>
                        <button
                            type="button"
                            onPointerDown={(e) => e.stopPropagation()}
                            disabled={!canMutate}
                            className={`${actionBtnClass} text-ink-600 transition hover:bg-warm-100 hover:text-ink-950 focus-visible:ring-brand`}
                            title={mutateBlockedTitle ?? tString("buttons.moreActions")}
                            aria-label={`${tString("buttons.moreActions")} ${item.name}`}
                            data-testid="menu-item-more-actions"
                        >
                            <MoreVertical className="w-3.5 h-3.5" />
                        </button>
                    </DropdownTrigger>
                    <DropdownMenu
                        aria-label={`${tString("buttons.moreActions")} ${item.name}`}
                        onAction={(key) => {
                            if (key === "delete") handleDelete();
                        }}
                    >
                        <DropdownItem
                            key="delete"
                            className="text-rose-600"
                            color="danger"
                            startContent={<Trash2 className="h-3.5 w-3.5" />}
                            aria-label={`${tString("buttons.delete")} ${item.name}`}
                            data-testid="menu-item-delete-action"
                        >
                            {tString("buttons.delete")}
                        </DropdownItem>
                    </DropdownMenu>
                </Dropdown>
            </div>
        </div>
    );
}

// Export the Sortable Wrap
export { SortableItemWrapper };
