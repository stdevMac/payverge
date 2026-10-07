"use client";

import React from "react";
import { Button, Spinner } from "@nextui-org/react";
import { Plus, RotateCcw, History } from "lucide-react";
import { useRouter } from "next/navigation";

import { Business } from "@/api/business";
import {
  DirectorAction,
  DirectorAppliedAction,
  DirectorProposedAction,
  DirectorThread,
  DirectorThreadMessage,
  archiveDirectorThread,
  askDirectorStreamURL,
  deleteDirectorThread,
  exportDirectorThreadURL,
  getDirectorThreadMessages,
  listDirectorAppliedActions,
  listDirectorThreads,
  patchDirectorThread,
  pinDirectorThread,
  restoreDirectorThread,
  submitDirectorFeedback,
  undoDirectorAction,
  directorActionErrorInfo,
} from "@/api/directorConsole";
import { readAndClearDirectorHandoff } from "@/api/opsAssistant";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import {
  type DirectorToolCallState,
  type ToolCallEvent,
  useDirectorStream,
} from "@/hooks/useDirectorStream";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import type { Locale } from "@/i18n/config";
import { runDirectorRename } from "./directorRename";

import { useClickTracking } from "@/hooks/useAnalytics";
import DashboardLockedTabView from "../DashboardLockedTabView";
import { AiProviderNotice } from "../AiProviderNotice";
import AssistantMessage from "./AssistantMessage";
import Composer from "./Composer";
import BriefingStrip from "./BriefingStrip";
import ProposalCard from "./ProposalCard";
import SageMark from "./SageMark";
import ChatEmptyState from "./EmptyState";
import { DirectorSkeleton } from "./DirectorSkeleton";
import {
  filterProposalsForThread,
  mapUndoErrorKey,
  UNDO_WINDOW_HOURS,
} from "./proposalActions";
import {
  isTrailingAssistant,
  messagesAfterRegenerateStart,
} from "./regenerateTurn";
import ThreadActionsMenu from "./ThreadActionsMenu";
import ThreadRow from "./ThreadRow";
import ToolTrace from "./ToolTrace";
import { uniqueDirectorThreads } from "./uniqueDirectorThreads";
import UserBubble from "./UserBubble";
import { useStickToBottom } from "./useStickToBottom";

// Tailwind's `md` breakpoint (768px). Threads collapse to a `<select>` below this width.
const MD_BREAKPOINT_PX = 768;

/**
 * The console's page box. Shared by the tier-loading state and the loaded
 * console so neither can drift and reintroduce the width/height snap — the
 * pair is pinned by `TierLoadingGeometry.test.tsx`. Keep it a literal string:
 * Tailwind's scanner does not resolve computed class names.
 */
const CONSOLE_GEOMETRY =
  "mx-auto flex h-[calc(100dvh-8rem)] max-w-7xl flex-col p-4 sm:p-6";

// IMP-26: drop seconds + go relative when recent. The audit flagged
// "5/18/2026, 10:02:35 PM" as too dense for a navigation list — every
// premium-fintech timestamp (Stripe, Linear, Vercel) collapses to
// "3 m ago" within a 7-day window and falls back to date-only beyond.
//
// L8 fix: relative-time strings are now translated (operator en/es) and the
// absolute fallback honors the active locale instead of `undefined`. `t`
// interpolates {count}; `localeTag` is the BCP-47 tag for toLocaleDateString.
function formatThreadTimestamp(
  iso: string,
  t: (key: string, params?: Record<string, string | number>) => string,
  localeTag: string,
): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const diffMs = Date.now() - d.getTime();
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diffMs < minute) return t("relativeTime.justNow");
  if (diffMs < hour) {
    const m = Math.floor(diffMs / minute);
    return t("relativeTime.minutesAgo", { count: m });
  }
  if (diffMs < day) {
    const h = Math.floor(diffMs / hour);
    return t("relativeTime.hoursAgo", { count: h });
  }
  if (diffMs < 7 * day) {
    const days = Math.floor(diffMs / day);
    return days === 1
      ? t("relativeTime.yesterday")
      : t("relativeTime.daysAgo", { count: days });
  }
  // Older than a week: show absolute date without seconds, in the active locale.
  return d.toLocaleDateString(localeTag, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function useIsNarrowViewport(): boolean {
  const [isNarrow, setIsNarrow] = React.useState(false);
  React.useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const mq = window.matchMedia(`(min-width: ${MD_BREAKPOINT_PX}px)`);
    const update = () => setIsNarrow(!mq.matches);
    update();
    mq.addEventListener("change", update);
    return () => mq.removeEventListener("change", update);
  }, []);
  return isNarrow;
}

interface DirectorConsoleDashboardProps {
  business: Business;
  // L14: the dashboard page's own tab handler (handleSetActiveTab). When
  // present, briefing chips route through it — same as BusinessOverview — so
  // they honor the tab whitelist + analytics, add no browser-history entry, and
  // never rewrite a slug URL into numeric-id form. Absent (e.g. standalone
  // render / tests), we fall back to router.push.
  onNavigateToTab?: (tab: string) => void;
}

type PendingItem = {
  nonce: string;
  question: string;
  /** L4-15: hide duplicate user bubble when regenerating. */
  regenerate?: boolean;
};

// Design-system note (M30): DirectorConsole is a DELIBERATE exemption from the
// shared DashboardTabShell/PageHeader tab contract. It is a full-height chat
// surface (composer pinned to the bottom, scrolling message thread) whose layout
// is incompatible with the shell's title-header + scroll-body idiom. Do not
// migrate it onto DashboardTabShell.
export default function DirectorConsoleDashboard({
  business,
  onNavigateToTab,
}: DirectorConsoleDashboardProps) {
  const router = useRouter();
  const { locale } = useSimpleLocale();
  const trackClick = useClickTracking();

  // The Pre-Shift deck's cards deep-link operators into the relevant dashboard
  // tab. Prefer the page's tab handler (whitelist + analytics + no history
  // entry, keeps the slug URL) when it was threaded in; otherwise fall back to
  // a router.push — tabs are URL-driven (the dashboard page reads `?tab=` from
  // useSearchParams and re-selects on every query change), so pushing the same
  // dashboard route with a new `?tab=` still switches tabs.
  const openTab = React.useCallback(
    (tab: string) => {
      if (onNavigateToTab) {
        onNavigateToTab(tab);
        return;
      }
      router.push(`/business/${business.id}/dashboard?tab=${tab}`);
    },
    [router, business.id, onNavigateToTab],
  );

  // Pass raw locale (not localeMessageBundles collapse) so es-AR voseo applies.
  const t = React.useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const value = getTranslation(`directorConsole.${key}`, locale, params);
      return Array.isArray(value) ? value[0] || key : (value as string);
    },
    [locale],
  );

  const quickPrompts = React.useMemo(() => {
    const value = getTranslation(`directorConsole.quickPrompts`, locale);
    return Array.isArray(value) ? value : [];
  }, [locale]);

  const [threads, setThreads] = React.useState<DirectorThread[]>([]);
  const [selectedThreadId, setSelectedThreadId] = React.useState<number | null>(
    null,
  );
  // Archived threads (soft-deleted) load lazily behind a disclosure; restore
  // returns them to the active list. Kept separate so the active sidebar stays
  // uncluttered and the archived read is only paid when the operator opens it.
  const [showArchived, setShowArchived] = React.useState(false);
  const [archivedThreads, setArchivedThreads] = React.useState<
    DirectorThread[]
  >([]);
  const [loadingArchived, setLoadingArchived] = React.useState(false);
  // Applied-changes history: server-backed so inspect/undo survive a refresh
  // (previously stranded in ephemeral React state). Loads on demand.
  const [appliedActions, setAppliedActions] = React.useState<
    DirectorAppliedAction[]
  >([]);
  const [showApplied, setShowApplied] = React.useState(false);
  const [loadingApplied, setLoadingApplied] = React.useState(false);
  const [undoingId, setUndoingId] = React.useState<string | null>(null);
  const [messages, setMessages] = React.useState<DirectorThreadMessage[]>([]);
  const [messageInput, setMessageInput] = React.useState("");
  React.useEffect(() => {
    const handoff = readAndClearDirectorHandoff(business.id);
    if (handoff) setMessageInput(handoff);
  }, [business.id]);
  const [loadingThreads, setLoadingThreads] = React.useState(true);
  const [loadingMessages, setLoadingMessages] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const [pending, setPending] = React.useState<PendingItem | null>(null);
  const pendingRef = React.useRef(pending);
  pendingRef.current = pending;
  const translationRef = React.useRef(t);
  translationRef.current = t;
  // Staged write proposals from the latest streamed answer (Phase 5). Live
  // only until the next ask / thread switch — proposals expire server-side
  // and a regenerated answer supersedes them in the backend too.
  const [proposals, setProposals] = React.useState<DirectorProposedAction[]>(
    [],
  );
  // Proposals the operator has already APPLIED carry their only Undo button in
  // the card. Blowing away the whole `proposals` array on the next ask / thread
  // switch removed those cards from the DOM and stranded the undo. We track the
  // applied ids so those cards stay pinned; only idle/confirm proposals are
  // dropped when the context moves on. (R3-AI-11)
  const [appliedProposalIds, setAppliedProposalIds] = React.useState<
    Set<string>
  >(() => new Set());
  const appliedProposalIdsRef = React.useRef(appliedProposalIds);
  appliedProposalIdsRef.current = appliedProposalIds;
  // L4-19: origin thread for each applied proposal so cards don't leak across chats.
  const [appliedThreadById, setAppliedThreadById] = React.useState<
    Record<string, number>
  >({});
  const appliedThreadByIdRef = React.useRef(appliedThreadById);
  appliedThreadByIdRef.current = appliedThreadById;
  const allowAutoThreadSelectionRef = React.useRef(true);
  const composerRef = React.useRef<HTMLTextAreaElement | null>(null);
  const messagesEndRef = React.useRef<HTMLDivElement | null>(null);

  const {
    toolCalls,
    streaming,
    complete,
    error: streamError,
    aborted,
    start: startStream,
    abort: abortStream,
    reset: resetStream,
  } = useDirectorStream();

  // Flatten the hook's internal `DirectorToolCallState[]` into the
  // `ToolCallEvent[]` view-model that `<ToolTrace>` expects. We derive a
  // stable id from `name + index` so the pills keep their place when the
  // matching `tool.call.completed` event flips `pending` → false.
  const toolCallEvents: ToolCallEvent[] = React.useMemo(
    () =>
      toolCalls.map((call: DirectorToolCallState, idx: number) => ({
        id: `${call.name}-${idx}`,
        name: call.name,
        human_label: call.human_label || call.name,
        args: call.args ?? {},
        state: call.pending
          ? "running"
          : call.success === false
            ? "error"
            : "done",
        summary: call.summary,
        duration_ms: call.duration_ms,
        error: call.error,
      })),
    [toolCalls],
  );

  const {
    hasAccess,
    aiConfigured,
    loading: accessLoading,
  } = useBusinessAccess(business.id);
  const directorRequestsEnabled =
    !accessLoading && hasAccess && aiConfigured;

  const isNarrow = useIsNarrowViewport();

  const selectedThread = React.useMemo(
    () => threads.find((thread) => thread.id === selectedThreadId) || null,
    [threads, selectedThreadId],
  );

  // Slash palette items: default quick prompts + the titles of the 5
  // most-recent threads, deduped and capped at 9 rows so the popover
  // never spills past `max-h-72`.
  const paletteItems = React.useMemo(() => {
    const fromThreads = threads
      .slice(0, 5)
      .map((thread) => thread.title)
      .filter((title): title is string => Boolean(title));
    const merged = [...quickPrompts, ...fromThreads];
    const dedup = Array.from(new Set(merged));
    return dedup.slice(0, 9);
  }, [quickPrompts, threads]);

  const loadThreads = React.useCallback(async () => {
    try {
      setLoadingThreads(true);
      const response = await listDirectorThreads(business.id);
      const nextThreads = uniqueDirectorThreads(response.threads || []);
      setThreads(nextThreads);
      setSelectedThreadId((current) => {
        if (
          current ||
          !allowAutoThreadSelectionRef.current ||
          nextThreads.length === 0
        ) {
          return current;
        }
        return nextThreads[0].id;
      });
    } catch (err) {
      console.error("Failed to load director threads:", err);
      setError(t("errors.loadThreads"));
    } finally {
      setLoadingThreads(false);
    }
  }, [business.id, t]);

  const loadMessages = React.useCallback(
    async (threadId: number) => {
      try {
        setLoadingMessages(true);
        const response = await getDirectorThreadMessages(business.id, threadId);
        setMessages(response.messages || []);
      } catch (err) {
        console.error("Failed to load director messages:", err);
        setError(t("errors.loadMessages"));
      } finally {
        setLoadingMessages(false);
      }
    },
    [business.id, t],
  );

  const loadArchivedThreads = React.useCallback(async () => {
    try {
      setLoadingArchived(true);
      const response = await listDirectorThreads(business.id, {
        archived: true,
      });
      setArchivedThreads(uniqueDirectorThreads(response.threads || []));
    } catch (err) {
      console.error("Failed to load archived threads:", err);
      setError(t("errors.loadThreads"));
    } finally {
      setLoadingArchived(false);
    }
  }, [business.id, t]);

  const loadAppliedActions = React.useCallback(async () => {
    try {
      setLoadingApplied(true);
      const response = await listDirectorAppliedActions(business.id);
      setAppliedActions(response.actions || []);
    } catch (err) {
      console.error("Failed to load applied actions:", err);
      setError(t("errors.loadApplied"));
    } finally {
      setLoadingApplied(false);
    }
  }, [business.id, t]);

  React.useEffect(() => {
    if (!directorRequestsEnabled) return;
    loadThreads().catch((err) => console.error("loadThreads failed:", err));
  }, [directorRequestsEnabled, loadThreads]);

  React.useEffect(() => {
    if (!directorRequestsEnabled || !selectedThreadId) {
      setMessages([]);
      return;
    }
    loadMessages(selectedThreadId).catch((err) =>
      console.error("loadMessages failed:", err),
    );
  }, [directorRequestsEnabled, selectedThreadId, loadMessages]);

  React.useEffect(() => {
    trackClick(
      "director-console-open",
      "director_console",
      `business-${business.id}`,
    );
  }, [business.id, trackClick]);

  React.useEffect(() => {
    if (!directorRequestsEnabled || !showArchived) return;
    loadArchivedThreads().catch((err) =>
      console.error("loadArchivedThreads failed:", err),
    );
  }, [directorRequestsEnabled, showArchived, loadArchivedThreads]);

  React.useEffect(() => {
    if (!directorRequestsEnabled || !showApplied) return;
    loadAppliedActions().catch((err) =>
      console.error("loadAppliedActions failed:", err),
    );
  }, [directorRequestsEnabled, showApplied, loadAppliedActions]);

  // SSE lifecycle: response.complete → swap the optimistic placeholder for the
  // real assistant message by re-fetching the thread's messages locally. We
  // skip the global `loadThreads()` call so the threads list doesn't blink;
  // the sidebar refreshes naturally on the next mount or manual reload.
  React.useEffect(() => {
    if (!directorRequestsEnabled || !complete) return;
    const { thread_id, message_id, model, latency_ms } = complete;
    if (typeof latency_ms === "number") {
      trackClick(
        "director-console-response",
        "director_console",
        `${model ?? "unknown"}:${latency_ms}`,
      );
    }
    allowAutoThreadSelectionRef.current = true;
    setSelectedThreadId(thread_id);
    loadMessages(thread_id).catch((err) =>
      console.error("loadMessages after stream complete failed:", err),
    );
    // New-thread binding (fix): the first ask on a fresh chat creates a thread
    // the sidebar never learned about — the actions menu and the row stayed
    // absent until a remount. Insert the thread into local state on
    // stream-complete (optimistic title from the question if the backend didn't
    // send one) so the sidebar and header actions render immediately, then
    // reconcile the real title/order in the background.
    if (typeof thread_id === "number") {
      setThreads((prev) => {
        if (prev.some((th) => th.id === thread_id)) return prev;
        const nowIso = new Date().toISOString();
        const optimistic: DirectorThread = {
          id: thread_id,
          title:
            pendingRef.current?.question?.slice(0, 60) ||
            translationRef.current("chat.newConversation"),
          created_at: nowIso,
          updated_at: nowIso,
          pinned: false,
        };
        return uniqueDirectorThreads([optimistic, ...prev]);
      });
      loadThreads().catch((err) =>
        console.error("loadThreads after stream complete failed:", err),
      );
    }
    // Keep any already-APPLIED cards from a prior turn pinned (their Undo button
    // is the only way to reverse the change), then append the fresh proposals
    // for this answer. (R3-AI-11)
    const fresh = complete.proposed_actions ?? [];
    setProposals((prev) => {
      const freshIds = new Set(fresh.map((p) => p.id));
      const pinnedApplied = prev.filter(
        (p) => appliedProposalIdsRef.current.has(p.id) && !freshIds.has(p.id),
      );
      return [...pinnedApplied, ...fresh];
    });
    setPending(null);
    composerRef.current?.focus();
    // message_id intentionally unused for now — Phase 3 punts persisted-message
    // ToolTrace fetching to a follow-up.
    void message_id;
  }, [
    directorRequestsEnabled,
    complete,
    loadMessages,
    loadThreads,
    trackClick,
  ]);

  React.useEffect(() => {
    if (!aborted) return;
    // Restore the composer text so the operator can edit and retry without
    // having to retype the question.
    const interrupted = pendingRef.current;
    if (interrupted) {
      setMessageInput(interrupted.question);
    }
    setPending(null);
    composerRef.current?.focus();
  }, [aborted]);

  React.useEffect(() => {
    if (!streamError) return;
    // The hook's `streamError.message` carries developer-facing strings
    // ("stream HTTP 502", "fetch failed", "stream read failed"). Keep those in
    // the console for debugging, but always show the operator a friendly,
    // localized message instead of a raw technical code.
    console.error("Director stream error:", streamError);
    setError(t("errors.askFailed"));
    const failed = pendingRef.current;
    if (failed) {
      setMessageInput(failed.question);
    }
    setPending(null);
  }, [streamError, t]);

  // Smart stick-to-bottom: only follow content when the operator is near the
  // bottom (or on their own send). Scrolling up preserves position and surfaces
  // a "New reply" jump control instead of yanking the viewport.
  const {
    scrollRef: messageListScrollRef,
    showJumpToLatest,
    onContentChange,
    jumpToLatest,
  } = useStickToBottom({ threshold: 100 });

  const prevPendingRef = React.useRef<PendingItem | null>(null);
  React.useEffect(() => {
    const force = pending != null && prevPendingRef.current == null;
    prevPendingRef.current = pending;
    onContentChange({ force });
  }, [messages, pending, toolCallEvents.length, loadingMessages, proposals, onContentChange]);

  const handleSendMessage = async (
    providedMessage?: string,
    opts?: { regenerate?: boolean },
  ) => {
    if (!directorRequestsEnabled) return;
    const message = (providedMessage ?? messageInput).trim();
    if (!message || streaming) return;

    setError(null);
    // A new ask supersedes any staged proposals (the backend dismisses them
    // on regen as well); drop the UNAPPLIED cards so the operator never applies a
    // proposal whose context has moved on — but KEEP applied cards so their Undo
    // button survives the next question. (R3-AI-11)
    setProposals((prev) => prev.filter((p) => appliedProposalIds.has(p.id)));
    // `startStream` resets the hook's internal state on entry, so we don't
    // need an explicit `resetStream()` call here. We keep `resetStream` in
    // scope for future use (e.g. dismissing a stale error banner).
    void resetStream;
    // Nonce is retained as a forward-compatible seam for SSE delta
    // reconciliation; abort handling now lives inside `useDirectorStream`.
    const nonce = `nonce-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    setPending({
      nonce,
      question: message,
      regenerate: opts?.regenerate,
    });
    setMessageInput("");
    trackClick(
      "director-console-ask",
      "director_console",
      `business-${business.id}`,
    );

    await startStream({
      url: askDirectorStreamURL(business.id),
      body: {
        message,
        thread_id: selectedThreadId ?? undefined,
        locale,
        active_tab: "director-console",
        regenerate: opts?.regenerate === true,
      },
    });
  };

  const handleFeedback = async (messageId: number, vote: "up" | "down") => {
    if (!directorRequestsEnabled) return;
    try {
      trackClick(
        `director-console-feedback-${vote}`,
        "director_console",
        `${messageId}`,
      );
      await submitDirectorFeedback(business.id, messageId, vote);
      if (selectedThreadId) {
        await loadMessages(selectedThreadId);
      }
    } catch (err) {
      console.error("Failed to submit feedback:", err);
      setError(t("errors.feedbackFailed"));
    }
  };

  const handleActionClick = (action: DirectorAction) => {
    trackClick(
      "director-console-action-click",
      "director_console",
      action.deep_link,
    );
    router.push(action.deep_link);
  };

  const handleFollowUpClick = (followUp: string) => {
    setMessageInput(followUp);
    composerRef.current?.focus();
  };

  const handleCopy = React.useCallback((text: string) => {
    navigator.clipboard?.writeText(text);
  }, []);

  const handleEdit = React.useCallback((text: string) => {
    setMessageInput(text);
    composerRef.current?.focus();
  }, []);

  const handleRegenerate = async (assistantMessageId: number) => {
    if (!directorRequestsEnabled) return;
    // L4-15: the server replaces the TRAILING assistant turn, so regenerate is
    // only valid for the last message. Refuse anything else (defense in depth —
    // the affordance is also hidden on non-trailing turns).
    if (!isTrailingAssistant(messages, assistantMessageId)) return;
    const idx = messages.findIndex((m) => m.id === assistantMessageId);
    if (idx <= 0) return;
    const prior = messages[idx - 1];
    if (prior.role !== "user") return;
    // L4-15: drop the trailing assistant in-place and re-ask without a
    // duplicate user bubble (server also honors regenerate when present).
    setMessages((prev) =>
      messagesAfterRegenerateStart(prev, assistantMessageId),
    );
    await handleSendMessage(prior.content, { regenerate: true });
  };

  const handleStartNewChat = () => {
    allowAutoThreadSelectionRef.current = false;
    setSelectedThreadId(null);
    setMessages([]);
    setMessageInput("");
    setError(null);
    // A brand-new chat is a full reset — clear proposals AND the applied-id
    // tracking so pinned cards don't leak into an unrelated conversation.
    setProposals([]);
    setAppliedProposalIds(new Set());
    setAppliedThreadById({});
    trackClick(
      "director-console-new-chat",
      "director_console",
      `business-${business.id}`,
    );
  };

  // Manual thread switches drop staged proposals: they belong to the answer
  // they arrived with, not to whichever transcript is now on screen. (This is
  // NOT done in the selectedThreadId effect — the stream-complete path also
  // changes selectedThreadId, and that path must keep its fresh proposals.)
  const handleSelectThread = (threadId: number) => {
    allowAutoThreadSelectionRef.current = true;
    setSelectedThreadId(threadId);
    // L4-19: applied cards only follow their origin thread (not every chat).
    setProposals((prev) =>
      filterProposalsForThread(prev, {
        threadId,
        appliedThreadById: appliedThreadByIdRef.current,
        appliedIds: appliedProposalIds,
      }).filter((p) => appliedProposalIds.has(p.id)),
    );
    trackClick(
      "director-console-thread-select",
      "director_console",
      `${threadId}`,
    );
  };

  const handleDismissProposal = React.useCallback((proposalId: string) => {
    setProposals((prev) => prev.filter((p) => p.id !== proposalId));
    setAppliedProposalIds((prev) => {
      if (!prev.has(proposalId)) return prev;
      const next = new Set(prev);
      next.delete(proposalId);
      return next;
    });
  }, []);

  const handleRenameThread = React.useCallback(
    async (threadId: number, next: string) => {
      // L4-14: clear-before-retry + surfaceBackendError (see runDirectorRename).
      await runDirectorRename({
        rename: async () => {
          await patchDirectorThread(business.id, threadId, { title: next });
          await loadThreads();
        },
        locale: locale as Locale,
        fallback: t("errors.renameFailed"),
        setError,
      });
    },
    [business.id, loadThreads, t, locale],
  );

  const handlePinToggle = React.useCallback(
    async (threadId: number, nextPinned: boolean) => {
      try {
        await pinDirectorThread(business.id, threadId, nextPinned);
        await loadThreads();
      } catch (err) {
        console.error("Failed to toggle pin:", err);
        setError(t("errors.pinFailed"));
      }
    },
    [business.id, loadThreads, t],
  );

  // Soft-archive (reversible). Hides the thread from the active sidebar; the
  // Archived section restores it. This replaces the previous mislabeled
  // permanent DELETE.
  const handleArchiveThread = React.useCallback(
    async (threadId: number) => {
      try {
        await archiveDirectorThread(business.id, threadId);
        setSelectedThreadId((cur) => (cur === threadId ? null : cur));
        await loadThreads();
        if (showArchived) await loadArchivedThreads();
      } catch (err) {
        console.error("Failed to archive thread:", err);
        setError(t("errors.archiveFailed"));
      }
    },
    [business.id, loadThreads, loadArchivedThreads, showArchived, t],
  );

  const handleRestoreThread = React.useCallback(
    async (threadId: number) => {
      try {
        await restoreDirectorThread(business.id, threadId);
        await Promise.all([loadThreads(), loadArchivedThreads()]);
      } catch (err) {
        console.error("Failed to restore thread:", err);
        setError(t("errors.restoreFailed"));
      }
    },
    [business.id, loadThreads, loadArchivedThreads, t],
  );

  // Permanent, irreversible erase — always confirmed inside ThreadActionsMenu.
  const handleDeleteThread = React.useCallback(
    async (threadId: number) => {
      try {
        await deleteDirectorThread(business.id, threadId);
        setSelectedThreadId((cur) => (cur === threadId ? null : cur));
        await loadThreads();
      } catch (err) {
        console.error("Failed to delete thread:", err);
        setError(t("errors.deleteFailed"));
      }
    },
    [business.id, loadThreads, t],
  );

  const handleUndoApplied = React.useCallback(
    async (proposalId: string) => {
      setUndoingId(proposalId);
      try {
        await undoDirectorAction(business.id, proposalId);
        await loadAppliedActions();
      } catch (err) {
        console.error("Failed to undo applied action:", err);
        // L4-19: same mapping as ProposalCard — accurate 24h window copy.
        const info = directorActionErrorInfo(err);
        const key = mapUndoErrorKey(info);
        setError(t(key, { hours: UNDO_WINDOW_HOURS }));
      } finally {
        setUndoingId(null);
      }
    },
    [business.id, loadAppliedActions, t],
  );

  // Loading reserves the loaded geometry so the tab does not snap width AND
  // height when the tier resolves. `DirectorSkeleton` is deliberately
  // width-agnostic — it is also rendered inside the loaded layout, where
  // filling its column is correct — so the wrapper is the console's job.
  // (This console cannot use DashboardTabShell: it is a fixed-height two-pane
  // chat layout, which the shell's scrolling `space-y-5` container can't
  // express. Same S-9 intent, reached the only way available here.)
  if (accessLoading) {
    return (
      <div className={CONSOLE_GEOMETRY}>
        <DirectorSkeleton />
      </div>
    );
  }

  // Admin lock is a full bypass — deliberately NOT wrapped in console geometry.
  if (!hasAccess) {
    return (
      <DashboardLockedTabView
        title={t("title")}
        subtitle={t("subtitle")}
        businessId={business.id}
      />
    );
  }

  return (
    <div className={`${CONSOLE_GEOMETRY} gap-4`}>
      <AiProviderNotice businessId={business.id} />
      <BriefingStrip business={business} onOpenTab={openTab} />

      {error ? (
        <div className="mb-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          {error}
        </div>
      ) : null}

      <div className="grid min-h-0 flex-1 grid-cols-1 grid-rows-[auto_minmax(0,1fr)] items-stretch gap-4 overflow-hidden xl:grid-cols-[minmax(16rem,18rem)_minmax(0,1fr)] xl:grid-rows-[minmax(0,1fr)]">
        {isNarrow ? (
          <div className="flex items-center gap-2">
            <label htmlFor="dc-thread-select" className="sr-only">
              {t("threadsAria") || "Threads"}
            </label>
            <select
              id="dc-thread-select"
              role="combobox"
              aria-label={t("threadsAria") || "Threads"}
              className="flex-1 rounded-lg border border-warm-200 bg-white px-3 py-2 text-body-sm text-ink-800"
              value={selectedThreadId ?? ""}
              onChange={(e) => {
                const value = e.target.value;
                if (!value) {
                  handleStartNewChat();
                  return;
                }
                handleSelectThread(Number(value));
              }}
            >
              <option value="">{t("chat.newConversation")}</option>
              {threads.map((thread) => (
                <option key={thread.id} value={thread.id}>
                  {thread.title}
                </option>
              ))}
            </select>
            <Button
              size="sm"
              variant="flat"
              startContent={<Plus className="w-3 h-3" />}
              onPress={handleStartNewChat}
            >
              {t("threads.new")}
            </Button>
          </div>
        ) : (
          <aside
            data-testid="dc-thread-sidebar"
            className="flex min-h-0 w-full flex-col overflow-hidden xl:h-full xl:pr-2"
          >
            <div className="mb-3 flex shrink-0 items-center justify-between px-1">
              <h2 className="text-label font-semibold text-ink-500 tracking-wide uppercase">
                {t("threads.title")}
              </h2>
              <div className="flex items-center gap-2">
                {loadingThreads ? <Spinner size="sm" /> : null}
                <Button
                  size="sm"
                  variant="light"
                  startContent={<Plus className="w-3 h-3" />}
                  onPress={handleStartNewChat}
                >
                  {t("threads.new")}
                </Button>
              </div>
            </div>

            <div
              data-testid="dc-thread-scroller"
              className="min-h-0 max-h-48 flex-1 space-y-1.5 overflow-y-auto pr-1 xl:max-h-none"
            >
              {threads.length === 0 && !loadingThreads ? (
                <div className="flex flex-col items-center gap-2 py-6 text-center">
                  <SageMark size="md" variant="soft" />
                  <p className="text-body-sm font-medium text-ink-800">
                    {t("threads.emptyTitle")}
                  </p>
                  <p className="text-body-sm text-ink-500">
                    {t("threads.empty")}
                  </p>
                </div>
              ) : null}

              {threads.map((thread) => {
                const isSelected = selectedThreadId === thread.id;
                return (
                  <ThreadRow
                    key={thread.id}
                    title={thread.title}
                    timestampLabel={formatThreadTimestamp(
                      thread.updated_at,
                      t,
                      locale,
                    )}
                    pinned={Boolean(thread.pinned)}
                    selected={isSelected}
                    pinnedLabel={t("threads.pinnedLabel")}
                    onSelect={() => handleSelectThread(thread.id)}
                    onRename={(next) => handleRenameThread(thread.id, next)}
                    onPinToggle={() =>
                      handlePinToggle(thread.id, !thread.pinned)
                    }
                    onArchive={() => handleArchiveThread(thread.id)}
                    onDelete={() => handleDeleteThread(thread.id)}
                    onExport={() => {
                      window.open(
                        exportDirectorThreadURL(business.id, thread.id),
                        "_blank",
                      );
                    }}
                  />
                );
              })}
            </div>

            {/* Applied / archived sit outside the thread scroller so they
                never compete with chat geometry or clip under short viewports.
                Toggle labels stay overflow-visible + nowrap so "APPLIED CHANGES"
                / "SHOW ARCHIVED" cannot be sliced by the chat column. */}
            <div
              className="mt-2 shrink-0 space-y-1 overflow-visible border-t border-warm-100 pt-2"
              data-testid="dc-applied-panel"
            >
              <button
                type="button"
                data-testid="dc-applied-toggle"
                onClick={() => setShowApplied((v) => !v)}
                className="flex w-full items-center gap-1.5 whitespace-nowrap px-1 py-1 text-label font-semibold uppercase tracking-wide text-ink-500 hover:text-ink-800"
              >
                <History className="h-3 w-3 shrink-0" />
                {t("threads.appliedTitle")}
              </button>
              {showApplied ? (
                <div className="mt-1 max-h-32 space-y-1 overflow-y-auto">
                  {loadingApplied ? (
                    <div className="px-1 py-1">
                      <Spinner size="sm" />
                    </div>
                  ) : appliedActions.length === 0 ? (
                    <p className="px-1 py-1 text-xs text-ink-500">
                      {t("applied.empty")}
                    </p>
                  ) : (
                    appliedActions.map((action) => (
                      <div
                        key={action.proposal_id}
                        className="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 hover:bg-warm-100/60"
                      >
                        <div className="min-w-0">
                          <div className="truncate text-xs font-medium text-ink-800">
                            {action.title}
                          </div>
                          <div className="text-[10px] text-ink-500">
                            {formatThreadTimestamp(
                              action.applied_at,
                              t,
                              locale,
                            )}
                          </div>
                        </div>
                        {action.undone_at ? (
                          <span className="shrink-0 text-[10px] text-ink-400">
                            {t("applied.undone")}
                          </span>
                        ) : action.can_undo ? (
                          <Button
                            size="sm"
                            variant="light"
                            className="h-6 min-w-0 px-2 text-xs"
                            isLoading={undoingId === action.proposal_id}
                            onPress={() =>
                              handleUndoApplied(action.proposal_id)
                            }
                          >
                            {t("applied.undo")}
                          </Button>
                        ) : (
                          <span className="shrink-0 text-[10px] text-ink-400">
                            {t("applied.expired")}
                          </span>
                        )}
                      </div>
                    ))
                  )}
                </div>
              ) : null}

              <button
                type="button"
                data-testid="dc-archived-toggle"
                onClick={() => setShowArchived((v) => !v)}
                className="flex w-full items-center gap-1.5 whitespace-nowrap px-1 py-1 text-label font-semibold uppercase tracking-wide text-ink-500 hover:text-ink-800"
              >
                {showArchived
                  ? t("threads.hideArchived")
                  : t("threads.showArchived")}
              </button>
              {showArchived ? (
                <div className="mt-1 max-h-32 space-y-1 overflow-y-auto">
                  {loadingArchived ? (
                    <div className="px-1 py-1">
                      <Spinner size="sm" />
                    </div>
                  ) : archivedThreads.length === 0 ? (
                    <p className="px-1 py-1 text-xs text-ink-500">
                      {t("threads.archivedEmpty")}
                    </p>
                  ) : (
                    archivedThreads.map((thread) => (
                      <div
                        key={thread.id}
                        className="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 hover:bg-warm-100/60"
                      >
                        <div className="min-w-0 truncate text-xs text-ink-600">
                          {thread.title}
                        </div>
                        <Button
                          size="sm"
                          variant="light"
                          className="h-6 min-w-0 px-2 text-xs"
                          startContent={<RotateCcw className="h-3 w-3" />}
                          onPress={() => handleRestoreThread(thread.id)}
                        >
                          {t("threads.restore")}
                        </Button>
                      </div>
                    ))
                  )}
                </div>
              ) : null}
            </div>
          </aside>
        )}

        <div
          data-testid="dc-chat-column"
          className="flex h-full min-h-0 w-full flex-col xl:border-l xl:border-warm-100 xl:pl-6"
        >
          <div className="mb-4 flex items-center justify-between gap-3 flex-none">
            <div className="flex items-center gap-2 min-w-0">
              <SageMark size="md" variant="solid" />
              <div className="min-w-0">
                <div className="font-title text-heading-md text-ink-900 truncate">
                  {selectedThread?.title || t("chat.newConversation")}
                </div>
                <div className="text-body-sm text-ink-500">
                  {t("chat.assistantName", {
                    name: business.ai_settings?.ai_name || "Sage",
                  })}
                </div>
              </div>
            </div>
            {selectedThread ? (
              <ThreadActionsMenu
                title={selectedThread.title}
                pinned={Boolean(selectedThread.pinned)}
                onRename={(next) =>
                  handleRenameThread(selectedThread.id, next)
                }
                onPinToggle={() =>
                  handlePinToggle(selectedThread.id, !selectedThread.pinned)
                }
                onArchive={() => handleArchiveThread(selectedThread.id)}
                onDelete={() => handleDeleteThread(selectedThread.id)}
                onExport={() => {
                  window.open(
                    exportDirectorThreadURL(business.id, selectedThread.id),
                    "_blank",
                  );
                }}
              />
            ) : null}
          </div>

          <div className="relative flex min-h-0 flex-1 flex-col">
          <div
            ref={messageListScrollRef}
            data-testid="dc-message-list"
            role="log"
            aria-label={t("title")}
            className="min-h-0 flex-1 space-y-3 overflow-y-auto pr-1"
          >
            {loadingMessages ? <DirectorSkeleton /> : null}

            {!loadingMessages && messages.length === 0 ? (
              <ChatEmptyState
                suggestions={quickPrompts}
                onPick={(prompt: string) => handleSendMessage(prompt)}
              />
            ) : null}

            <div>
              {(() => {
                // Pre-compute the id of the first assistant message so the
                // first-reply styling (expanded Action Plan) only triggers
                // once. Indexing into the mixed user+assistant array would
                // mis-target index 0 (always a user message in real chats).
                const firstAssistantId = messages.find(
                  (m) => m.role === "assistant",
                )?.id;
                return messages.map((message, idx) => {
                  // Conversation rhythm keyed on the role *transition*, not just
                  // the current role: an answer hugs the question it responds to
                  // (tight `mt-3`); a new question opens a fresh exchange with
                  // generous air (`mt-10`); and two same-speaker turns in a row
                  // (e.g. back-to-back assistant replies) get a middle `mt-6` so
                  // they stay distinct rather than fusing into one block.
                  // Uniform spacing read as a flat, templated list; this cadence
                  // gives the transcript deliberate ~3:1 contrast.
                  const prevRole = idx > 0 ? messages[idx - 1].role : null;
                  const gap =
                    idx === 0
                      ? ""
                      : message.role === "user"
                        ? "mt-10"
                        : prevRole === "user"
                          ? "mt-3"
                          : "mt-6";
                  return (
                    <div key={message.id} className={gap}>
                      {message.role === "user" ? (
                        <UserBubble
                          content={message.content}
                          onCopy={handleCopy}
                          onEdit={handleEdit}
                        />
                      ) : (
                        <AssistantMessage
                          message={message}
                          isFirstAssistant={message.id === firstAssistantId}
                          isTrailing={isTrailingAssistant(
                            messages,
                            message.id,
                          )}
                          assistantName={
                            business.ai_settings?.ai_name || "Sage"
                          }
                          onActionClick={handleActionClick}
                          onFeedback={handleFeedback}
                          onFollowUpClick={handleFollowUpClick}
                          onCopy={handleCopy}
                          onRegenerate={handleRegenerate}
                        />
                      )}
                    </div>
                  );
                });
              })()}

              {proposals.length > 0 && !pending ? (
                <div
                  data-testid="dc-proposals"
                  // Proposals belong to the answer directly above, so they hug
                  // it (`mt-3`) and sit on the SAME rail geometry as the answer
                  // (transparent 2px border + pl-4), so their left edge lines up
                  // exactly with the assistant text instead of 2px short of it.
                  className="mt-3 space-y-3 border-l-2 border-transparent pl-4"
                >
                  {proposals.map((proposal) => (
                    <ProposalCard
                      key={proposal.id}
                      proposal={proposal}
                      businessId={business.id}
                      t={t}
                      locale={locale}
                      onViewInMenu={() => openTab("menu")}
                      onDismiss={handleDismissProposal}
                      onApplied={(proposalId) => {
                        // Remember the applied card so it stays pinned on its
                        // origin thread (Undo is the only way back). (L4-19)
                        setAppliedProposalIds((prev) => {
                          const next = new Set(prev);
                          next.add(proposalId);
                          return next;
                        });
                        if (selectedThreadId != null) {
                          setAppliedThreadById((prev) => ({
                            ...prev,
                            [proposalId]: selectedThreadId,
                          }));
                        }
                        trackClick(
                          "director-console-proposal-applied",
                          "director_console",
                          proposalId,
                        );
                      }}
                    />
                  ))}
                </div>
              ) : null}

              {pending ? (
                <div className={messages.length > 0 ? "mt-10" : ""}>
                  {!pending.regenerate ? (
                  <div
                    data-testid="dc-pending-user"
                    className="flex justify-end"
                  >
                    <div className="rounded-2xl rounded-br-md bg-brand/10 text-ink-900 px-4 py-3 max-w-[72ch] whitespace-pre-wrap text-body">
                      {pending.question}
                    </div>
                  </div>
                  ) : null}
                  {/* The live placeholder mirrors a settled assistant turn: the
                      same brand rail (no per-turn logo stamp), hugging its
                      question with `mt-3`. */}
                  <div
                    data-testid="dc-assistant-placeholder"
                    className="mt-3 max-w-[72ch] border-l-2 border-brand/50 pl-4 py-1"
                  >
                    {toolCallEvents.length > 0 ? (
                      <div className="mb-2">
                        <ToolTrace toolCalls={toolCallEvents} />
                      </div>
                    ) : null}
                    <div role="status" className="text-body-sm text-ink-600">
                      {toolCallEvents.length > 0
                        ? t("chat.composing", {
                            name: business.ai_settings?.ai_name || "Sage",
                          })
                        : t("chat.thinking", {
                            name: business.ai_settings?.ai_name || "Sage",
                          }) || "Thinking…"}
                    </div>
                  </div>
                </div>
              ) : null}
            </div>

            {/* Bottom sentinel for layout/tests; scroll is owned by useStickToBottom. */}
            <div ref={messagesEndRef} data-testid="dc-message-end" />
          </div>
          {showJumpToLatest ? (
            <div className="pointer-events-none absolute inset-x-0 bottom-3 z-10 flex justify-center">
              <Button
                size="sm"
                color="primary"
                variant="solid"
                className="pointer-events-auto font-semibold shadow-md"
                onPress={jumpToLatest}
                aria-label={t("chat.newReplyAria")}
                data-testid="dc-jump-latest"
              >
                {t("chat.newReply")}
              </Button>
            </div>
          ) : null}
          </div>

          {/* Composer is rendered as the next flex child below the
                flex-1 message list. The chat column is a true flex
                column at viewport height, so the composer naturally
                pins to the bottom — no sticky chrome required. */}
          <Composer
            ref={composerRef}
            value={messageInput}
            onChange={setMessageInput}
            onSend={() => handleSendMessage()}
            onAbort={() => {
              // The hook owns the AbortController; the `aborted` useEffect
              // clears the optimistic placeholder and restores composer
              // text. We just trigger the abort here.
              abortStream();
            }}
            sending={streaming}
            assistantName={business.ai_settings?.ai_name || "Sage"}
            placeholder={t("chat.inputPlaceholder")}
            sendLabel={t("chat.send", {
              name: business.ai_settings?.ai_name || "Sage",
            })}
            sendingLabel={t("chat.sending")}
            stopLabel={t("chat.stop")}
            hint={t("chat.composerHint") || "⌘↵ to send"}
            paletteItems={paletteItems}
            // L4-12c: insert-not-send — Composer already writes the item into
            // the input; do not auto-fire a turn from the palette.
            onPaletteSelect={() => {
              /* selection is applied by Composer via onChange */
            }}
            slashPaletteAria={t("composer.slashPaletteAria")}
          />
        </div>
      </div>
    </div>
  );
}
