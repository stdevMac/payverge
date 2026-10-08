import React from "react";
import { Chip, Select, SelectItem } from "@nextui-org/react";
import { X } from "lucide-react";
import Toolbar, { ViewToggle } from "../../shared/Toolbar";
import { btnGhostIcon } from "@/components/ui/buttonStyles";

type Filter = "all" | "available" | "unavailable";
type ViewMode = "grid" | "list";

interface MenuPageHeaderProps {
    tString: (key: string) => string;

    // Search / filter / view
    searchQuery: string;
    setSearchQuery: (q: string) => void;
    searchFilter: Filter;
    setSearchFilter: (f: Filter) => void;
    viewMode: ViewMode;
    setViewMode: (m: ViewMode) => void;
    searchResultsCount: { totalItems: number; totalCategories: number };
}

/**
 * The Menu Builder toolbar row: shared `Toolbar` (leading-icon search) with a
 * right slot carrying the live result-count, a clear button, the availability
 * filter Select, and the grid/list `ViewToggle`. The page title/stats and the
 * language + add actions now live in the DashboardTabShell header (index.tsx);
 * this component is search/filter/view only. Kept at the same module path with
 * the same default-shaped named export so the jest mocks in
 * `__tests__/MenuBuilder.*.test.tsx` keep resolving `MenuPageHeader`.
 */
export function MenuPageHeader({
    tString,
    searchQuery,
    setSearchQuery,
    searchFilter,
    setSearchFilter,
    viewMode,
    setViewMode,
    searchResultsCount,
}: MenuPageHeaderProps) {
    const filterActive = searchFilter !== "all";
    const searchActive = searchQuery.trim().length > 0;
    const showResultCount = filterActive || searchActive;

    const formatResultCount = (n: number) => {
        if (n === 1) return tString("header.searchResultCountSingular");
        return tString("header.searchResultCountPlural").replace(
            "{count}",
            String(n),
        );
    };

    return (
        <div className="space-y-2">
            <Toolbar
                search={{
                    value: searchQuery,
                    onChange: setSearchQuery,
                    placeholder: tString("search.placeholder"),
                }}
            >
                {showResultCount && (
                    <span className="whitespace-nowrap text-xs text-ink-500">
                        {formatResultCount(searchResultsCount.totalItems)}
                    </span>
                )}
                {searchActive && (
                    <button
                        type="button"
                        onClick={() => setSearchQuery("")}
                        className={btnGhostIcon}
                        aria-label={tString("buttons.clear")}
                    >
                        <X className="h-4 w-4" />
                    </button>
                )}
                <Select
                    aria-label={
                        tString("search.filterByAvailability") ||
                        "Filter by availability"
                    }
                    selectedKeys={[searchFilter]}
                    onSelectionChange={(keys) => {
                        // Esc / clear yields an empty selection — never write
                        // `undefined` into the filter (that blanks the menu).
                        const next = Array.from(keys)[0] as Filter | undefined;
                        if (next === "all" || next === "available" || next === "unavailable") {
                            setSearchFilter(next);
                        } else {
                            setSearchFilter("all");
                        }
                    }}
                    size="sm"
                    variant="bordered"
                    className="w-40 sm:w-44"
                    classNames={{
                        trigger:
                            "h-10 min-h-10 bg-white border-warm-200 data-[hover=true]:border-brand/40",
                    }}
                >
                    <SelectItem key="all" value="all">
                        {tString("search.filters.all")}
                    </SelectItem>
                    <SelectItem key="available" value="available">
                        {tString("search.filters.available")}
                    </SelectItem>
                    <SelectItem key="unavailable" value="unavailable">
                        {tString("search.filters.unavailable")}
                    </SelectItem>
                </Select>

                <ViewToggle
                    value={viewMode}
                    onChange={setViewMode}
                    labels={{
                        grid: tString("viewMode.grid"),
                        list: tString("viewMode.list"),
                    }}
                />
            </Toolbar>

            {/* Active filter chip strip — only when a non-default filter is set */}
            {filterActive && (
                <div className="flex items-center gap-2">
                    <Chip
                        size="sm"
                        variant="flat"
                        onClose={() => setSearchFilter("all")}
                        className="h-7 rounded-full bg-brand/10 text-xs text-brand-dark"
                        classNames={{
                            content: "px-1",
                            closeButton: "text-brand-dark",
                        }}
                    >
                        {tString("header.filterChipPrefix")}{" "}
                        <span className="font-medium">
                            {tString(`search.filters.${searchFilter}`)}
                        </span>
                    </Chip>
                </div>
            )}
        </div>
    );
}
