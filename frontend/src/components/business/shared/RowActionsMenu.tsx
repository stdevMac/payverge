"use client";

import React, {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { Button } from "@nextui-org/react";
import { MoreVertical } from "lucide-react";

type RowActionTone = "default" | "danger";

export interface RowActionItem {
  key: string;
  label: string;
  icon?: React.ReactNode;
  tone?: RowActionTone;
  disabled?: boolean;
}

export interface RowActionsMenuProps {
  /** Required accessible name for the ellipsis trigger. */
  "aria-label": string;
  /** Optional test id — keep separate from the human-readable aria-label. */
  "data-testid"?: string;
  items: RowActionItem[];
  onAction: (key: string) => void;
  className?: string;
  /** Align the menu toward the trigger's end — use on right-edge table cells. */
  placement?: "bottom" | "bottom-end" | "bottom-start";
}

const MENU_WIDTH_PX = 256;

/**
 * Compact ellipsis menu for table/list rows. Portals a fixed menu to
 * document.body so PremiumPanel `overflow-hidden` and DataTable
 * `overflow-x-auto` cannot clip it (issue 258). NextUI Dropdown's default
 * content slot is only z-10 and shouldBlockScroll can dismiss the menu on the
 * same click that opened it.
 */
export default function RowActionsMenu({
  "aria-label": ariaLabel,
  "data-testid": dataTestId,
  items,
  onAction,
  className = "",
  placement = "bottom-end",
}: RowActionsMenuProps) {
  const triggerWrapRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [coords, setCoords] = useState({ top: 0, left: 0 });
  const menuId = useId();

  const stopRowClick = (event: React.SyntheticEvent) => {
    event.stopPropagation();
  };

  const onWrapKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Escape" && open) {
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      return;
    }
    event.stopPropagation();
  };

  const updatePosition = useCallback(() => {
    const trigger = triggerWrapRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    let left = rect.left;
    if (placement === "bottom-end") {
      left = rect.right - MENU_WIDTH_PX;
    } else if (placement === "bottom") {
      left = rect.left + rect.width / 2 - MENU_WIDTH_PX / 2;
    }
    const maxLeft = window.innerWidth - MENU_WIDTH_PX - 8;
    setCoords({
      top: rect.bottom + 4,
      left: Math.min(Math.max(8, left), Math.max(8, maxLeft)),
    });
  }, [placement]);

  useLayoutEffect(() => {
    if (!open) return;
    updatePosition();
  }, [open, updatePosition]);

  useEffect(() => {
    if (!open) return;

    const onPointerDown = (event: MouseEvent) => {
      const target = event.target as Node | null;
      if (!target) return;
      if (
        menuRef.current?.contains(target) ||
        triggerWrapRef.current?.contains(target)
      ) {
        return;
      }
      setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    const onReposition = () => updatePosition();

    document.addEventListener("mousedown", onPointerDown);
    // Capture so a row-level stopPropagation (or a focused trigger) cannot
    // swallow Escape before the menu closes.
    document.addEventListener("keydown", onKeyDown, true);
    window.addEventListener("resize", onReposition);
    window.addEventListener("scroll", onReposition, true);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown, true);
      window.removeEventListener("resize", onReposition);
      window.removeEventListener("scroll", onReposition, true);
    };
  }, [open, updatePosition]);

  const menu =
    open && typeof document !== "undefined"
      ? createPortal(
          <div
            ref={menuRef}
            id={menuId}
            role="menu"
            aria-label={ariaLabel}
            data-testid="row-actions-menu"
            style={{ top: coords.top, left: coords.left, minWidth: MENU_WIDTH_PX }}
            className="fixed z-[100] rounded-xl border border-warm-200 bg-white py-1 shadow-lg shadow-warm-900/10"
          >
            {items.map((item) => (
              <button
                key={item.key}
                type="button"
                role="menuitem"
                disabled={item.disabled}
                className={
                  item.tone === "danger"
                    ? "flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-rose-600 hover:bg-rose-50 hover:text-rose-700 disabled:cursor-not-allowed disabled:opacity-40"
                    : "flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-ink-700 hover:bg-warm-50 disabled:cursor-not-allowed disabled:opacity-40"
                }
                onClick={(event) => {
                  event.stopPropagation();
                  if (item.disabled) return;
                  setOpen(false);
                  onAction(item.key);
                }}
              >
                {item.icon}
                <span>{item.label}</span>
              </button>
            ))}
          </div>,
          document.body,
        )
      : null;

  return (
    // role="presentation": propagation guard only — not an interactive control.
    <div
      className={className}
      onClick={stopRowClick}
      onKeyDown={onWrapKeyDown}
      role="presentation"
    >
      <div ref={triggerWrapRef}>
        <Button
          isIconOnly
          size="sm"
          variant="light"
          aria-label={ariaLabel}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-controls={open ? menuId : undefined}
          data-testid={dataTestId}
          className="min-w-8 text-ink-500 hover:bg-warm-100 hover:text-ink-700"
          onPress={() => setOpen((value) => !value)}
        >
          <MoreVertical className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      {menu}
    </div>
  );
}
