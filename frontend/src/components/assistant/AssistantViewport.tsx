"use client";

import type { ReactNode } from "react";
import { useEffect, useRef } from "react";
import {
  type AssistantScrollController,
  type ContentChange,
  useAssistantScroll,
} from "./useAssistantScroll";
import {
  assistantEventProperties,
  type AssistantAnalyticsContext,
  trackAssistantEvent,
} from "./assistantAnalytics";

export type AssistantViewportProps = {
  children: ReactNode;
  conversationLabel: string;
  jumpToLatestLabel: string;
  completionAnnouncement?: string;
  contentChange?: ContentChange;
  contentChangeOrigin?: "restored" | "live";
  controller?: AssistantScrollController;
  className?: string;
  analytics?: AssistantAnalyticsContext;
};

export function AssistantViewport({
  children,
  conversationLabel,
  jumpToLatestLabel,
  completionAnnouncement = "",
  contentChange,
  contentChangeOrigin,
  controller,
  className = "",
  analytics,
}: AssistantViewportProps) {
  const internalController = useAssistantScroll();
  const scrollController = controller ?? internalController;
  const { setScroller, onContentChange, jumpToLatest, showJumpToLatest } =
    scrollController;
  const inferredRestoredTurnIdRef = useRef<string | undefined>(
    !contentChangeOrigin && contentChange?.responseComplete
      ? contentChange.turnId
      : undefined,
  );
  const onContentChangeRef = useRef(onContentChange);
  onContentChangeRef.current = onContentChange;
  const jumpToLatestRef = useRef(jumpToLatest);
  jumpToLatestRef.current = jumpToLatest;
  const canBottomLateRestoredHydrationRef = useRef(!contentChange);
  const isRestoredChange = Boolean(
    contentChangeOrigin === "restored" ||
    (!contentChangeOrigin &&
      contentChange?.responseComplete &&
      contentChange.turnId === inferredRestoredTurnIdRef.current),
  );
  const isStreaming = Boolean(
    contentChange && !contentChange.responseComplete && !isRestoredChange,
  );
  const shouldAnnounceCompletion = Boolean(
    contentChange?.responseComplete && !isRestoredChange,
  );
  const hasCompletionAnnouncement = completionAnnouncement.trim().length > 0;
  const previousShowJumpRef = useRef(showJumpToLatest);
  const userScrolledAwayTransition =
    scrollController.userScrolledAwayTransition ?? 0;
  const previousUserScrolledAwayTransitionRef = useRef(
    userScrolledAwayTransition,
  );

  useEffect(() => {
    const wasVisible = previousShowJumpRef.current;
    previousShowJumpRef.current = showJumpToLatest;
    if (!analytics || wasVisible || !showJumpToLatest) return;
    trackAssistantEvent(
      "assistant_jump_control_shown",
      assistantEventProperties(analytics),
    );
  }, [analytics, showJumpToLatest]);

  useEffect(() => {
    const previous = previousUserScrolledAwayTransitionRef.current;
    previousUserScrolledAwayTransitionRef.current = userScrolledAwayTransition;
    if (!analytics || userScrolledAwayTransition <= previous) return;
    trackAssistantEvent(
      "assistant_user_scrolled_away",
      assistantEventProperties(analytics),
    );
  }, [analytics, userScrolledAwayTransition]);

  useEffect(() => {
    if (!contentChange) {
      canBottomLateRestoredHydrationRef.current = true;
      return;
    }
    if (isRestoredChange) {
      if (canBottomLateRestoredHydrationRef.current) {
        canBottomLateRestoredHydrationRef.current = false;
        jumpToLatestRef.current();
      }
      return;
    }
    canBottomLateRestoredHydrationRef.current = false;
    onContentChangeRef.current(contentChange);
  }, [contentChange, isRestoredChange]);

  return (
    <div className={`relative min-h-0 ${className}`}>
      <div
        ref={setScroller}
        role="log"
        aria-label={conversationLabel}
        aria-live="polite"
        aria-relevant="additions text"
        aria-busy={isStreaming}
        className="h-full min-h-0 overflow-y-auto overscroll-contain"
      >
        <div data-assistant-scroll-content>{children}</div>
      </div>

      {hasCompletionAnnouncement ? (
        <div
          role="status"
          aria-live="polite"
          aria-atomic="true"
          className="sr-only"
        >
          {shouldAnnounceCompletion ? completionAnnouncement : ""}
        </div>
      ) : null}

      {showJumpToLatest ? (
        <div className="pointer-events-none absolute inset-x-0 bottom-3 z-10 flex justify-center px-3">
          <button
            type="button"
            className="pointer-events-auto rounded-full bg-brand-600 px-4 py-2 text-label font-semibold text-white shadow-md transition-colors hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
            onClick={() => {
              if (analytics) {
                trackAssistantEvent(
                  "assistant_jump_control_clicked",
                  assistantEventProperties(analytics),
                );
              }
              jumpToLatest();
            }}
            aria-label={jumpToLatestLabel}
          >
            {jumpToLatestLabel}
          </button>
        </div>
      ) : null}
    </div>
  );
}
