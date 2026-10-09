"use client";

import React from "react";
import { Clock3, Pin } from "lucide-react";
import ThreadActionsMenu from "./ThreadActionsMenu";

export type ThreadRowProps = {
  title: string;
  timestampLabel: string;
  pinned: boolean;
  selected: boolean;
  onSelect: () => void;
  onRename: (next: string) => void;
  onPinToggle: () => void;
  onArchive: () => void;
  onDelete: () => void;
  onExport: () => void;
  pinnedLabel: string;
};

/**
 * Active thread row: select target + always-visible ThreadActionsMenu.
 * Not a single outer <button> — avoids invalid nested buttons with the menu trigger.
 */
export default function ThreadRow(props: ThreadRowProps) {
  const {
    title,
    timestampLabel,
    pinned,
    selected,
    onSelect,
    onRename,
    onPinToggle,
    onArchive,
    onDelete,
    onExport,
    pinnedLabel,
  } = props;

  return (
    <div
      data-testid="dc-thread-row"
      className={`group flex w-full items-start gap-1 rounded-lg py-1.5 pl-2 pr-1 transition-colors ${
        selected
          ? "bg-brand/5 text-ink-900 ring-1 ring-brand/20"
          : "text-ink-800 hover:bg-warm-100/60"
      }`}
    >
      <button
        type="button"
        onClick={onSelect}
        className="min-w-0 flex-1 rounded-md px-1 py-0.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      >
        <div className="flex items-start gap-1.5">
          {pinned ? (
            <Pin
              className="mt-0.5 h-3 w-3 shrink-0 fill-current text-brand"
              aria-label={pinnedLabel}
            />
          ) : null}
          <div className="text-body-sm font-medium line-clamp-2 break-words">
            {title}
          </div>
        </div>
        <div
          className={`mt-1 flex items-center gap-1 text-xs ${
            selected ? "text-brand-dark" : "text-ink-500"
          }`}
        >
          <Clock3 className="h-3 w-3 shrink-0" />
          <span className="truncate">{timestampLabel}</span>
        </div>
      </button>

      {/* Layout-only wrapper: selection is the sibling <button>, not this row.
          Do not attach click handlers here (jsx-a11y/no-static-element-interactions). */}
      <div className="shrink-0 pt-0.5">
        <ThreadActionsMenu
          title={title}
          pinned={pinned}
          onRename={onRename}
          onPinToggle={onPinToggle}
          onArchive={onArchive}
          onDelete={onDelete}
          onExport={onExport}
        />
      </div>
    </div>
  );
}
