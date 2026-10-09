"use client";

import React from "react";

export type UseStickToBottomOptions = {
  /** Distance from bottom (px) that still counts as "near bottom". Default 100. */
  threshold?: number;
};

export type UseStickToBottomResult = {
  /** Callback ref for the scrollable message list container. */
  scrollRef: (el: HTMLDivElement | null) => void;
  stickToBottom: boolean;
  showJumpToLatest: boolean;
  /** Call when messages/pending/tools/proposals change. force=true on operator send. */
  onContentChange: (opts?: { force?: boolean }) => void;
  jumpToLatest: () => void;
};

function isNearBottom(el: HTMLElement, threshold: number): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= threshold;
}

function scrollToBottom(el: HTMLElement): void {
  el.scrollTop = el.scrollHeight;
}

/**
 * Smart stick-to-bottom for the Director message list.
 * Scrolls the list container only (never window.scrollIntoView).
 */
export function useStickToBottom(
  options: UseStickToBottomOptions = {},
): UseStickToBottomResult {
  const threshold = options.threshold ?? 100;
  const scrollerRef = React.useRef<HTMLDivElement | null>(null);
  const stickRef = React.useRef(true);
  const [scroller, setScroller] = React.useState<HTMLDivElement | null>(null);
  const [stickToBottom, setStickToBottom] = React.useState(true);
  const [showJumpToLatest, setShowJumpToLatest] = React.useState(false);

  const scrollRef = React.useCallback((el: HTMLDivElement | null) => {
    scrollerRef.current = el;
    setScroller(el);
  }, []);

  const setStick = React.useCallback((next: boolean) => {
    stickRef.current = next;
    setStickToBottom(next);
    if (next) setShowJumpToLatest(false);
  }, []);

  const jumpToLatest = React.useCallback(() => {
    const el = scrollerRef.current;
    if (el) scrollToBottom(el);
    setStick(true);
  }, [setStick]);

  const onContentChange = React.useCallback(
    (opts: { force?: boolean } = {}) => {
      const el = scrollerRef.current;
      if (!el) return;
      if (opts.force || stickRef.current) {
        scrollToBottom(el);
        setStick(true);
        return;
      }
      setShowJumpToLatest(true);
    },
    [setStick],
  );

  React.useEffect(() => {
    const el = scroller;
    if (!el) return;

    const onScroll = () => {
      setStick(isNearBottom(el, threshold));
    };

    el.addEventListener("scroll", onScroll, { passive: true });

    let ro: ResizeObserver | null = null;
    if (typeof ResizeObserver !== "undefined") {
      ro = new ResizeObserver(() => {
        if (stickRef.current) {
          scrollToBottom(el);
        }
      });
      const target = el.firstElementChild ?? el;
      ro.observe(target);
    }

    return () => {
      el.removeEventListener("scroll", onScroll);
      ro?.disconnect();
    };
  }, [scroller, threshold, setStick]);

  return {
    scrollRef,
    stickToBottom,
    showJumpToLatest,
    onContentChange,
    jumpToLatest,
  };
}
