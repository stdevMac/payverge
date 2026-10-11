"use client";

/**
 * M5 — styled sidebar tooltip, replacing native `title` chrome.
 *
 * Native titles are unstyleable, delay arbitrarily (~1s), and never show for
 * keyboard users. This bubble is the dashboard's ink-950 style, reveals on
 * hover AND focus-within, and links to its trigger via aria-describedby so
 * screen readers get the same hint.
 *
 * Contract: `children` must be a single element (the trigger). The tooltip id
 * is injected via cloneElement — compose it around buttons/links directly,
 * not around fragments or component wrappers that swallow DOM props.
 *
 * #744: `side="right"` (collapsed icon rail) portals the bubble to document.body
 * with `position: fixed`. The rail lives under `overflow-hidden` +
 * `overflow-y-auto` ancestors, which clip an in-tree `left-full` bubble to a
 * dark sliver. Footer / expanded uses (`side="top"`) stay in-tree.
 *
 * Focus must land on the trigger itself — native `focus` does not bubble, so
 * wrap-level onFocus is a no-op when the rail icon is tabbed. Flyout hide
 * (`aria-expanded` / `data-open`) is re-implemented for the portal path so a
 * popover and the bubble cannot stack.
 */

import React from "react";
import { createPortal } from "react-dom";

/** Matches Tailwind `ml-2` — gap between the collapsed rail and the bubble. */
const RIGHT_GAP_PX = 8;

/** Mirrors `group-has-[[aria-expanded=true]]` / `group-has-[[data-open=true]]`. */
const FLYOUT_OPEN_SELECTOR = '[aria-expanded="true"], [data-open="true"]';

interface SidebarTooltipProps {
  label: string;
  /** Secondary line ("Label — description") for expanded rows. */
  description?: string;
  /** "right" floats beside the collapsed icon rail; "top" hovers above. */
  side?: "top" | "right";
  className?: string;
  /** Extra classes for the bubble itself (e.g. breakpoint gating). */
  bubbleClassName?: string;
  children: React.ReactElement;
}

type TriggerFocusProps = {
  onFocus?: (event: React.FocusEvent<HTMLElement>) => void;
  onBlur?: (event: React.FocusEvent<HTMLElement>) => void;
};

const BUBBLE_BASE =
  "pointer-events-none whitespace-nowrap rounded-lg bg-ink-950 px-2.5 py-1 text-[10px] font-medium text-white shadow-lg shadow-ink-950/20 transition-opacity duration-150";

function bubbleCopy(label: string, description?: string) {
  return (
    <>
      {label}
      {description ? (
        <span className="text-white/70"> — {description}</span>
      ) : null}
    </>
  );
}

function composeFocus(
  theirs: TriggerFocusProps["onFocus"] | undefined,
  ours: () => void,
): (event: React.FocusEvent<HTMLElement>) => void {
  return (event) => {
    theirs?.(event);
    ours();
  };
}

function flyoutIsOpen(root: HTMLElement | null): boolean {
  return Boolean(root?.querySelector(FLYOUT_OPEN_SELECTOR));
}

export function SidebarTooltip({
  label,
  description,
  side = "top",
  className = "",
  bubbleClassName = "",
  children,
}: SidebarTooltipProps) {
  const id = React.useId();
  const wrapRef = React.useRef<HTMLSpanElement>(null);
  const [mounted, setMounted] = React.useState(false);
  const [open, setOpen] = React.useState(false);
  const [flyoutOpen, setFlyoutOpen] = React.useState(false);
  const [coords, setCoords] = React.useState({ top: 0, left: 0 });

  const portalRight = side === "right";
  const childProps = children.props as TriggerFocusProps;

  React.useEffect(() => {
    setMounted(true);
  }, []);

  const updatePosition = React.useCallback(() => {
    const el = wrapRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    setCoords({
      top: rect.top + rect.height / 2,
      left: rect.right + RIGHT_GAP_PX,
    });
  }, []);

  const refreshFlyout = React.useCallback(() => {
    setFlyoutOpen(flyoutIsOpen(wrapRef.current));
  }, []);

  const show = React.useCallback(() => {
    updatePosition();
    setOpen(true);
  }, [updatePosition]);

  const hide = React.useCallback(() => {
    setOpen(false);
  }, []);

  const trigger = React.cloneElement(children, {
    "aria-describedby": id,
    ...(portalRight
      ? {
          onFocus: composeFocus(childProps.onFocus, show),
          onBlur: composeFocus(childProps.onBlur, hide),
        }
      : {}),
  } as Partial<unknown>);

  React.useEffect(() => {
    if (!portalRight) return;
    const el = wrapRef.current;
    if (!el) return;
    refreshFlyout();
    const observer = new MutationObserver(refreshFlyout);
    observer.observe(el, {
      attributes: true,
      attributeFilter: ["aria-expanded", "data-open"],
      subtree: true,
    });
    return () => observer.disconnect();
  }, [portalRight, refreshFlyout]);

  React.useEffect(() => {
    if (!portalRight || !open) return;
    updatePosition();
    const onReposition = () => updatePosition();
    window.addEventListener("resize", onReposition);
    window.addEventListener("scroll", onReposition, true);
    return () => {
      window.removeEventListener("resize", onReposition);
      window.removeEventListener("scroll", onReposition, true);
    };
  }, [portalRight, open, updatePosition]);

  const bubbleVisible = open && !flyoutOpen;

  const inTreeBubble = (
    <span
      role="tooltip"
      id={id}
      className={[
        BUBBLE_BASE,
        "absolute z-[70] opacity-0 group-hover/tip:opacity-100 group-focus-within/tip:opacity-100 group-has-[[aria-expanded=true]]/tip:!opacity-0 group-has-[[data-open=true]]/tip:!opacity-0",
        "bottom-full left-1/2 mb-2 -translate-x-1/2",
        bubbleClassName,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {bubbleCopy(label, description)}
    </span>
  );

  const portaledBubble =
    mounted && typeof document !== "undefined"
      ? createPortal(
          <span
            role="tooltip"
            id={id}
            data-sidebar-tooltip="portal"
            className={[
              BUBBLE_BASE,
              "fixed z-[70] -translate-y-1/2",
              bubbleVisible ? "opacity-100 visible" : "opacity-0 invisible",
              bubbleClassName,
            ]
              .filter(Boolean)
              .join(" ")}
            style={{ top: coords.top, left: coords.left }}
          >
            {bubbleCopy(label, description)}
          </span>,
          document.body,
        )
      : null;

  return (
    // Hover-only positioning wrap. Keyboard reveal is on the trigger
    // (onFocus/onBlur); the rail icon remains the interactive control.
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions -- hover wrap, not a control
    <span
      ref={wrapRef}
      data-testid={portalRight ? "sidebar-tooltip-wrap" : undefined}
      className={["group/tip relative", className].filter(Boolean).join(" ")}
      onMouseEnter={portalRight ? show : undefined}
      onMouseLeave={portalRight ? hide : undefined}
    >
      {trigger}
      {portalRight ? portaledBubble : inTreeBubble}
    </span>
  );
}
