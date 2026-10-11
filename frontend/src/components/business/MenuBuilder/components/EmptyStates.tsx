import React from 'react';
import { BookOpen, RefreshCcw, Search, X } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { btnPrimary } from "@/components/ui/buttonStyles";
import MenuFrontDoor from "../../onboarding/MenuFrontDoor";

interface EmptyMenuProps {
    tString: (key: string) => string;
    onAddCategoryOpen: () => void;
    onOpenAIFeature?: (feature: "pdf" | "wizard") => void;
}

export function EmptyMenu({
    tString,
    onAddCategoryOpen,
    onOpenAIFeature,
}: EmptyMenuProps) {
    return (
        <EmptyState
            panel
            icon={BookOpen}
            title={tString("noCategories")}
            subtitle={tString("categories.createFirstDescription")}
            action={
                <MenuFrontDoor
                    tString={tString}
                    onAddCategoryOpen={onAddCategoryOpen}
                    onOpenAIFeature={onOpenAIFeature}
                />
            }
        />
    );
}

interface EmptyMenuLoadFailedProps {
    tString: (key: string) => string;
    onRetry: () => void;
}

export function EmptyMenuLoadFailed({
    tString,
    onRetry,
}: EmptyMenuLoadFailedProps) {
    return (
        <EmptyState
            panel
            icon={RefreshCcw}
            title={tString("loadFailedTitle")}
            subtitle={tString("loadFailedHint")}
            data-testid="menu-load-failed"
            action={
                <button type="button" onClick={onRetry} className={btnPrimary}>
                    <RefreshCcw className="h-4 w-4" aria-hidden="true" />
                    {tString("retryLoad")}
                </button>
            }
        />
    );
}

interface EmptySearchResultsProps {
    tString: (key: string) => string;
    clearSearch: () => void;
}

export function EmptySearchResults({ tString, clearSearch }: EmptySearchResultsProps) {
    return (
        <EmptyState
            panel
            icon={Search}
            title={tString("search.noResults")}
            subtitle={tString("search.noResultsDescription")}
            action={
                <button type="button" onClick={clearSearch} className={btnPrimary}>
                    <X className="h-4 w-4" aria-hidden="true" />
                    {tString("buttons.clear")}
                </button>
            }
        />
    );
}
