"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const NEAR_BOTTOM_THRESHOLD_PX = 96;
const RESPONSE_START_OFFSET_PX = 12;

export type ContentChange = {
  turnId: string;
  responseStart: HTMLElement | null;
  responseComplete: boolean;
};

export type AssistantScrollController = {
  setScroller(node: HTMLDivElement | null): void;
  onContentChange(change: ContentChange): void;
  jumpToLatest(): void;
  showJumpToLatest: boolean;
  userScrolledAwayTransition?: number;
};

function distanceFromBottom(scroller: HTMLElement): number {
  return scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
}

function isInteractiveKeyboardTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;

  const editable = target.closest("[contenteditable]");
  if (editable) {
    return editable.getAttribute("contenteditable")?.toLowerCase() !== "false";
  }

  return Boolean(
    target.closest(
      'button, a[href], input, textarea, select, summary, [role="button"], [role="link"], [role="textbox"], [role="combobox"], [role="listbox"], [role="menu"], [role="menuitem"], [role="option"], [role="radio"], [role="slider"], [role="spinbutton"], [role="tab"], [role="tree"], [role="treeitem"]',
    ),
  );
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

function moveToBottom(scroller: HTMLDivElement): void {
  const top = scroller.scrollHeight;
  const behavior: ScrollBehavior = prefersReducedMotion() ? "auto" : "smooth";

  if (typeof scroller.scrollTo === "function") {
    scroller.scrollTo({ top, behavior });
    return;
  }

  scroller.scrollTop = top;
}

export function useAssistantScroll(): AssistantScrollController {
  const scrollerRef = useRef<HTMLDivElement | null>(null);
  const followingRef = useRef(true);
  const userIntentRef = useRef(false);
  const longAnswerRef = useRef(false);
  const activeTurnIdRef = useRef<string | null>(null);
  const responseStartRef = useRef<HTMLElement | null>(null);
  const manualFollowTurnIdRef = useRef<string | null>(null);
  const resizeObserverRef = useRef<ResizeObserver | null>(null);
  const programmaticScrollRef = useRef(false);
  const explicitUserScrollRef = useRef(false);
  const programmaticScrollTargetRef = useRef<{
    requested: number;
    maximum: number;
  } | null>(null);
  const [scrollerNode, setScrollerNode] = useState<HTMLDivElement | null>(null);
  const [showJumpToLatest, setShowJumpToLatest] = useState(false);
  const [userScrolledAwayTransition, setUserScrolledAwayTransition] =
    useState(0);

  const setFollowing = useCallback((following: boolean) => {
    followingRef.current = following;
    setShowJumpToLatest(!following);
  }, []);

  const scrollToBottom = useCallback((scroller: HTMLDivElement) => {
    programmaticScrollRef.current = true;
    programmaticScrollTargetRef.current = {
      requested: scroller.scrollHeight,
      maximum: Math.max(0, scroller.scrollHeight - scroller.clientHeight),
    };
    moveToBottom(scroller);
  }, []);

  const alignLongResponse = useCallback(
    (scroller: HTMLDivElement, responseStart: HTMLElement): boolean => {
      const responseRect = responseStart.getBoundingClientRect();
      if (responseRect.height <= scroller.clientHeight) return false;

      const scrollerRect = scroller.getBoundingClientRect();
      longAnswerRef.current = true;
      userIntentRef.current = false;
      manualFollowTurnIdRef.current = null;
      programmaticScrollRef.current = true;
      scroller.scrollTop +=
        responseRect.top - scrollerRect.top - RESPONSE_START_OFFSET_PX;
      programmaticScrollTargetRef.current = {
        requested: scroller.scrollTop,
        maximum: scroller.scrollTop,
      };
      setFollowing(false);
      return true;
    },
    [setFollowing],
  );

  const setScroller = useCallback(
    (node: HTMLDivElement | null) => {
      scrollerRef.current = node;
      setScrollerNode(node);

      if (!node) return;

      followingRef.current = true;
      userIntentRef.current = false;
      longAnswerRef.current = false;
      activeTurnIdRef.current = null;
      responseStartRef.current = null;
      manualFollowTurnIdRef.current = null;
      setShowJumpToLatest(false);
      scrollToBottom(node);
    },
    [scrollToBottom],
  );

  const onContentChange = useCallback(
    (change: ContentChange) => {
      const scroller = scrollerRef.current;
      if (!scroller) return;

      const isNewTurn = activeTurnIdRef.current !== change.turnId;
      if (isNewTurn) {
        activeTurnIdRef.current = change.turnId;
        longAnswerRef.current = false;
        manualFollowTurnIdRef.current = null;
      }

      const previousResponseStart = responseStartRef.current;
      if (previousResponseStart !== change.responseStart) {
        if (previousResponseStart) {
          resizeObserverRef.current?.unobserve(previousResponseStart);
        }
        responseStartRef.current = change.responseStart;
        if (change.responseStart) {
          resizeObserverRef.current?.observe(change.responseStart);
        }
      }

      if (!change.responseStart) {
        if (isNewTurn) {
          userIntentRef.current = false;
          setFollowing(true);
          scrollToBottom(scroller);
          return;
        }

        if (followingRef.current && !userIntentRef.current) {
          scrollToBottom(scroller);
        } else {
          setShowJumpToLatest(true);
        }
        return;
      }

      if (!followingRef.current || userIntentRef.current) {
        setShowJumpToLatest(true);
        return;
      }

      if (longAnswerRef.current) {
        setFollowing(false);
        return;
      }

      if (manualFollowTurnIdRef.current === change.turnId) {
        scrollToBottom(scroller);
        setFollowing(true);
        return;
      }

      if (alignLongResponse(scroller, change.responseStart)) {
        return;
      }

      scrollToBottom(scroller);
      setFollowing(true);
    },
    [alignLongResponse, scrollToBottom, setFollowing],
  );

  const jumpToLatest = useCallback(() => {
    const scroller = scrollerRef.current;
    if (!scroller) return;

    longAnswerRef.current = false;
    userIntentRef.current = false;
    manualFollowTurnIdRef.current = activeTurnIdRef.current;
    setFollowing(true);
    scrollToBottom(scroller);
  }, [scrollToBottom, setFollowing]);

  useEffect(() => {
    if (!scrollerNode) return;

    const cancelProgrammaticScroll = () => {
      programmaticScrollRef.current = false;
      programmaticScrollTargetRef.current = null;
    };

    const markUserScrollIntent = () => {
      explicitUserScrollRef.current = true;
      cancelProgrammaticScroll();
    };

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || isInteractiveKeyboardTarget(event.target)) {
        return;
      }

      if (
        [
          "ArrowUp",
          "ArrowDown",
          "PageUp",
          "PageDown",
          "Home",
          "End",
          " ",
        ].includes(event.key)
      ) {
        markUserScrollIntent();
      }
    };

    const handlePointerDown = (event: PointerEvent) => {
      if (event.target === scrollerNode) markUserScrollIntent();
    };

    const handleScroll = () => {
      const programmaticTarget = programmaticScrollTargetRef.current;
      if (programmaticScrollRef.current) {
        if (
          programmaticTarget &&
          (Math.abs(scrollerNode.scrollTop - programmaticTarget.requested) <=
            1 ||
            Math.abs(scrollerNode.scrollTop - programmaticTarget.maximum) <= 1)
        ) {
          programmaticScrollRef.current = false;
          programmaticScrollTargetRef.current = null;
        }
        return;
      }

      const nearBottom =
        distanceFromBottom(scrollerNode) <= NEAR_BOTTOM_THRESHOLD_PX;
      const wasFollowing = followingRef.current;
      if (nearBottom) {
        longAnswerRef.current = false;
        manualFollowTurnIdRef.current = activeTurnIdRef.current;
      } else {
        manualFollowTurnIdRef.current = null;
      }
      userIntentRef.current = !nearBottom;
      setFollowing(nearBottom);
      if (explicitUserScrollRef.current && wasFollowing && !nearBottom) {
        setUserScrolledAwayTransition((current) => current + 1);
      }
      explicitUserScrollRef.current = false;
    };

    scrollerNode.addEventListener("scroll", handleScroll, { passive: true });
    scrollerNode.addEventListener("wheel", markUserScrollIntent, {
      passive: true,
    });
    scrollerNode.addEventListener("touchmove", markUserScrollIntent, {
      passive: true,
    });
    scrollerNode.addEventListener("keydown", handleKeyDown);
    scrollerNode.addEventListener("pointerdown", handlePointerDown);

    let observer: ResizeObserver | null = null;
    if (typeof ResizeObserver !== "undefined") {
      observer = new ResizeObserver(() => {
        if (followingRef.current && !longAnswerRef.current) {
          const responseStart = responseStartRef.current;
          if (
            responseStart &&
            manualFollowTurnIdRef.current !== activeTurnIdRef.current &&
            alignLongResponse(scrollerNode, responseStart)
          ) {
            return;
          }
          scrollToBottom(scrollerNode);
        } else {
          setShowJumpToLatest(true);
        }
      });
      resizeObserverRef.current = observer;
      observer.observe(scrollerNode);
      Array.from(scrollerNode.children).forEach((child) =>
        observer?.observe(child),
      );
      if (responseStartRef.current) {
        observer.observe(responseStartRef.current);
      }
    }

    return () => {
      scrollerNode.removeEventListener("scroll", handleScroll);
      scrollerNode.removeEventListener("wheel", markUserScrollIntent);
      scrollerNode.removeEventListener("touchmove", markUserScrollIntent);
      scrollerNode.removeEventListener("keydown", handleKeyDown);
      scrollerNode.removeEventListener("pointerdown", handlePointerDown);
      observer?.disconnect();
      if (resizeObserverRef.current === observer) {
        resizeObserverRef.current = null;
      }
    };
  }, [alignLongResponse, scrollerNode, scrollToBottom, setFollowing]);

  return {
    setScroller,
    onContentChange,
    jumpToLatest,
    showJumpToLatest,
    userScrolledAwayTransition,
  };
}
