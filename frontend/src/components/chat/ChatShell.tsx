"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { Button } from "@nextui-org/react";
import { X, Sparkles, RotateCcw, ThumbsUp, ThumbsDown } from "lucide-react";
import { motion, AnimatePresence, useReducedMotion } from "framer-motion";
import type { ChatAction, ChatMessage } from "@/components/chat/types";
import {
  AssistantComposer,
  type AssistantComposerLabels,
} from "@/components/assistant/AssistantComposer";
import {
  AssistantMessage,
  type AssistantMessageLabels,
} from "@/components/assistant/AssistantMessage";
import { AssistantViewport } from "@/components/assistant/AssistantViewport";
import {
  adaptLegacyResponse,
  type AssistantAction,
  type AssistantResponse,
} from "@/types/assistant";
import {
  extractLeakedActions,
  isSafeHref,
} from "@/components/chat/extractLeakedActions";
import {
  assistantEventProperties,
  type AssistantAnalyticsContext,
  trackAssistantEvent,
} from "@/components/assistant/assistantAnalytics";

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
const DEFAULT_MESSAGE_LIMIT = 2_000;

function listFocusable(root: HTMLElement): HTMLElement[] {
  return Array.from(
    root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
  ).filter(
    (element) =>
      !element.hasAttribute("disabled") &&
      element.getAttribute("aria-hidden") !== "true",
  );
}

export interface ChatShellProps {
  title: string;
  subtitle?: string;
  messages: ChatMessage[];
  input: string;
  onInputChange: (value: string) => void;
  onSend: () => void;
  onClose: () => void;
  loading?: boolean;
  onActionClick?: (action: ChatAction) => void;
  /** Native V2 action callback; receives the complete action without coercion. */
  onAssistantAction?: (action: AssistantAction) => void;
  /** Required + localized by callers — no hardcoded English fallback. */
  placeholder: string;
  sendLabel: string;
  welcomeMessage?: string;
  welcomeHint?: string;
  suggestions?: string[];
  onSuggestionClick?: (text: string) => void;
  loadingLabel: string;
  /** Localized aria-label for the dialog + the close button. */
  dialogLabel: string;
  closeLabel: string;
  /** Error surface: when set, shows a recoverable error bubble. */
  errorMessage?: string | null;
  onRetry?: () => void;
  retryLabel?: string;
  /** Localized "reach us directly" affordance rendered under the error. */
  errorHelp?: React.ReactNode;
  /**
   * Positioning/size classes for the portaled dialog root. Defaults to a
   * bottom-right floating panel. Portal-to-body is required so background
   * inert marks every other body child (not only siblings under an app root).
   */
  className?: string;
  /** When false, skip portal (tests only). Default true. */
  portal?: boolean;
  /** Renders a restart icon button in the header when provided. */
  onNewConversation?: () => void;
  newConversationLabel?: string;
  /** Renders thumb vote buttons under the latest assistant message. */
  feedback?: {
    onVote: (vote: "up" | "down") => boolean | void | Promise<boolean | void>;
    upLabel: string;
    downLabel: string;
  } | null;
  /** Shared primitives need surface-owned labels; later surface plans supply them. */
  assistantLabels?: AssistantMessageLabels;
  conversationLabel?: string;
  jumpToLatestLabel?: string;
  completionAnnouncement?: string;
  characterCountLabel?: AssistantComposerLabels["characterCount"];
  maxInputLength?: number;
  /** Explicit override for tests/embedded shells; otherwise coarse pointers are mobile. */
  mobileComposer?: boolean;
  /** Trusted surface metadata supplied by the owning shell. */
  assistantAnalytics?: AssistantAnalyticsContext;
}

/**
 * Compatibility export for older isolated callers/tests. ChatShell messages
 * themselves render actions through AssistantMessage.
 */
export function ActionCard({
  action,
  onClick,
}: {
  action: ChatAction;
  onClick?: (action: ChatAction) => void;
}) {
  const inner = (
    <>
      <span className="text-sm">{action.label}</span>
      {action.disabled_reason ? (
        <span className="block text-xs text-ink-500">
          {action.disabled_reason}
        </span>
      ) : null}
    </>
  );

  if (
    action.kind === "external" &&
    !action.disabled &&
    isSafeHref(action.href)
  ) {
    return (
      <motion.div
        initial={{ opacity: 0, y: 6 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2 }}
      >
        <a
          href={action.href}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex flex-col items-start justify-start rounded-medium bg-default/40 px-3 py-2 text-left text-tiny text-default-700 transition-colors hover:opacity-hover"
        >
          {inner}
        </a>
      </motion.div>
    );
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2 }}
    >
      <Button
        size="sm"
        variant="flat"
        className="h-auto justify-start px-3 py-2 text-left"
        isDisabled={action.disabled}
        onPress={() => onClick?.(action)}
      >
        {inner}
      </Button>
    </motion.div>
  );
}

function TypingIndicator({ label }: { label: string }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 4 }}
      className="flex justify-start"
    >
      <div className="inline-flex items-center gap-1.5 rounded-2xl rounded-tl-sm border border-warm-200 bg-warm-50 px-4 py-3">
        <span className="sr-only">{label}</span>
        <span
          className="h-1.5 w-1.5 animate-bounce rounded-full bg-brand/70 [animation-delay:-0.3s]"
          aria-hidden="true"
        />
        <span
          className="h-1.5 w-1.5 animate-bounce rounded-full bg-brand/70 [animation-delay:-0.15s]"
          aria-hidden="true"
        />
        <span
          className="h-1.5 w-1.5 animate-bounce rounded-full bg-brand/70"
          aria-hidden="true"
        />
      </div>
    </motion.div>
  );
}

function stableTextID(prefix: string, text: string, index: number): string {
  let hash = 2_166_136_261;
  for (const character of text) {
    hash ^= character.codePointAt(0) ?? 0;
    hash = Math.imul(hash, 16_777_619);
  }
  return `${prefix}-${index}-${(hash >>> 0).toString(36)}`;
}

function legacyResponseForMessage(
  message: ChatMessage,
  index: number,
): AssistantResponse {
  const sanitized = extractLeakedActions(message.content);
  const seen = new Set<string>();
  const actions = [...(message.actions ?? []), ...sanitized.actions].filter(
    (action) => {
      if (!action.href || !isSafeHref(action.href) || seen.has(action.href)) {
        return false;
      }
      seen.add(action.href);
      return true;
    },
  );

  return adaptLegacyResponse(
    stableTextID("legacy-response", sanitized.content, index),
    {
      answer: sanitized.content,
      actions,
      steps: message.steps ?? [],
      follow_ups: message.followUps ?? [],
    },
  );
}

function toLegacyAction(action: AssistantAction): ChatAction | null {
  if (action.confirmation !== "none") return null;

  let kind: ChatAction["kind"];
  if (action.type === "navigate") {
    kind = "navigate";
  } else if (action.type === "external_link") {
    kind = "external";
  } else if (action.type === "director_handoff") {
    kind = "handoff";
  } else {
    return null;
  }
  return {
    label: action.label,
    href: action.target.href,
    kind,
    disabled: action.state !== "ready",
    disabled_reason: action.disabled_reason ?? undefined,
  };
}

function compatibilityAssistantLabels({
  response,
  title: _title,
  dialogLabel,
}: {
  response: AssistantResponse;
  title: string;
  dialogLabel: string;
}): AssistantMessageLabels {
  // Sources carry only their own names — never prefix with the shell brand title
  // (avoids "Payverge Concierge: …" / duplicate sender labels in Concierge).
  const sourceNames = Array.from(
    new Set(
      response.sources
        .map(({ title: sourceTitle }) => sourceTitle.trim())
        .filter(Boolean),
    ),
  );
  const sourceGroupLabel =
    sourceNames.length > 0
      ? sourceNames.join(", ")
      : `${response.sources.length} sources`;

  const statusLabel = (status: AssistantResponse["status"]): string => {
    switch (status) {
      case "needs_clarification":
        return "Needs clarification";
      case "blocked":
        return "Unavailable";
      case "degraded":
        return "Limited response";
      default:
        return "Complete";
    }
  };

  return {
    usedSources: () => sourceGroupLabel,
    sourcesRegion: () => sourceGroupLabel,
    sourceOrigin: () => "Verified source",
    externalSource: (hostname) => hostname,
    // Neutral section chrome — do not reuse the widget brand title as headings.
    actions: "Actions",
    steps: "Steps",
    entities: "Related",
    entityAvailability: () => "Availability",
    followUps: "Suggestions",
    notices: "Notices",
    noticeKind: () => "Notice",
    status: statusLabel,
    workflowProgress: ({ current, total }) => `${current}/${total}`,
    disabledActionReason: (reason) => reason ?? "Unavailable",
    renderError: dialogLabel,
  };
}

type RenderedMessage = {
  message: ChatMessage;
  response: AssistantResponse | null;
  stableID: string;
};

export default function ChatShell({
  title,
  subtitle,
  messages,
  input,
  onInputChange,
  onSend,
  onClose,
  loading = false,
  onActionClick,
  onAssistantAction,
  placeholder,
  sendLabel,
  welcomeMessage,
  welcomeHint,
  suggestions = [],
  onSuggestionClick,
  loadingLabel,
  dialogLabel,
  closeLabel,
  errorMessage,
  onRetry,
  retryLabel,
  errorHelp,
  className,
  portal = true,
  onNewConversation,
  newConversationLabel,
  feedback,
  assistantLabels,
  conversationLabel,
  jumpToLatestLabel,
  completionAnnouncement = "",
  characterCountLabel,
  maxInputLength = DEFAULT_MESSAGE_LIMIT,
  mobileComposer,
  assistantAnalytics,
}: ChatShellProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);
  const reduceMotion = useReducedMotion();
  const [mounted, setMounted] = useState(false);
  const [coarsePointer, setCoarsePointer] = useState(false);
  const [responseStartNode, setResponseStartNode] =
    useState<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);
  const liveTurnIDsRef = useRef(new Set<string>());
  const feedbackPendingRef = useRef(false);

  const submitFeedback = useCallback(
    (vote: "up" | "down", outcome: "positive" | "negative") => {
      if (!feedback || feedbackPendingRef.current) return;
      feedbackPendingRef.current = true;
      void Promise.resolve()
        .then(() => feedback.onVote(vote))
        .then((accepted) => {
          if (!assistantAnalytics || accepted === false) return;
          trackAssistantEvent(
            "assistant_feedback_submitted",
            assistantEventProperties(assistantAnalytics, { outcome }),
          );
        })
        .catch(() => undefined)
        .finally(() => {
          feedbackPendingRef.current = false;
        });
    },
    [assistantAnalytics, feedback],
  );

  const renderedMessages = useMemo<RenderedMessage[]>(
    () =>
      messages.map((message, index) => {
        if (message.role === "user") {
          return {
            message,
            response: null,
            stableID: stableTextID("user", message.content, index),
          };
        }
        const response =
          message.response ?? legacyResponseForMessage(message, index);
        return {
          message,
          response,
          stableID: response.response_id,
        };
      }),
    [messages],
  );

  let latestUserIndex = -1;
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].role === "user") {
      latestUserIndex = index;
      break;
    }
  }
  let activeAssistantIndex = -1;
  for (
    let index = Math.max(0, latestUserIndex + 1);
    index < renderedMessages.length;
    index += 1
  ) {
    if (renderedMessages[index].response) activeAssistantIndex = index;
  }
  if (latestUserIndex < 0) {
    for (let index = renderedMessages.length - 1; index >= 0; index -= 1) {
      if (renderedMessages[index].response) {
        activeAssistantIndex = index;
        break;
      }
    }
  }
  const latestTurnID =
    latestUserIndex >= 0
      ? stableTextID("turn", messages[latestUserIndex].content, latestUserIndex)
      : activeAssistantIndex >= 0
        ? `turn-${renderedMessages[activeAssistantIndex].stableID}`
        : null;
  if (loading && latestTurnID) liveTurnIDsRef.current.add(latestTurnID);

  const setLatestResponseStart = useCallback((node: HTMLElement | null) => {
    setResponseStartNode((current) => (current === node ? current : node));
  }, []);

  useEffect(() => {
    if (activeAssistantIndex < 0) setResponseStartNode(null);
  }, [activeAssistantIndex]);

  const contentChange = useMemo(
    () =>
      latestTurnID
        ? {
            turnId: latestTurnID,
            responseStart: activeAssistantIndex >= 0 ? responseStartNode : null,
            responseComplete:
              !loading && (activeAssistantIndex >= 0 || Boolean(errorMessage)),
          }
        : undefined,
    [
      activeAssistantIndex,
      errorMessage,
      latestTurnID,
      loading,
      responseStartNode,
    ],
  );
  const contentChangeOrigin =
    latestTurnID && liveTurnIDsRef.current.has(latestTurnID)
      ? "live"
      : "restored";

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    if (mobileComposer !== undefined) return;
    const query = window.matchMedia?.("(pointer: coarse)");
    if (!query) return;
    const update = () => setCoarsePointer(query.matches);
    update();
    query.addEventListener?.("change", update);
    return () => query.removeEventListener?.("change", update);
  }, [mobileComposer]);

  // Focus trap, inert background, focus restore on unmount (AI-20 / WCAG dialog).
  useEffect(() => {
    const node = containerRef.current;
    if (!node) return;

    previouslyFocusedRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;

    const inerted: HTMLElement[] = [];
    Array.from(document.body.children).forEach((child) => {
      if (!(child instanceof HTMLElement)) return;
      if (child === node || child.contains(node) || node.contains(child))
        return;
      if (child.hasAttribute("data-chat-shell-root")) return;
      if (!child.inert) {
        child.inert = true;
        inerted.push(child);
      }
    });

    const focusable = listFocusable(node);
    const composer = node.querySelector<HTMLElement>(
      "textarea:not([disabled]), input:not([disabled])",
    );
    (composer ?? focusable[0] ?? node).focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        event.preventDefault();
        onCloseRef.current();
        return;
      }
      if (event.key !== "Tab") return;
      const items = listFocusable(node);
      if (items.length === 0) {
        event.preventDefault();
        node.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement as HTMLElement | null;
      if (event.shiftKey) {
        if (active === first || !node.contains(active)) {
          event.preventDefault();
          last.focus();
        }
      } else if (active === last || !node.contains(active)) {
        event.preventDefault();
        first.focus();
      }
    };

    node.addEventListener("keydown", onKeyDown);
    return () => {
      node.removeEventListener("keydown", onKeyDown);
      inerted.forEach((element) => {
        element.inert = false;
      });
      const previous = previouslyFocusedRef.current;
      if (
        previous &&
        typeof previous.focus === "function" &&
        document.contains(previous)
      ) {
        previous.focus();
      }
    };
    // Install once. Escape reads the latest callback through onCloseRef so
    // controlled-input re-renders cannot yank focus back to the header.
  }, [mounted]);

  const showWelcome =
    messages.length === 0 && !loading && !errorMessage && welcomeMessage;
  let latestAssistantIndex = -1;
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].role === "assistant") {
      latestAssistantIndex = index;
      break;
    }
  }

  const rootClass =
    className ??
    "fixed bottom-5 right-4 z-50 flex h-[min(560px,calc(100vh-6rem))] w-[min(400px,calc(100vw-2rem))] flex-col";
  const composerLabels: AssistantComposerLabels = {
    textarea: placeholder,
    send: sendLabel,
    busy: loadingLabel,
    characterCount:
      characterCountLabel ?? ((current, maximum) => `${current}/${maximum}`),
  };
  // Compatibility fallback: pair the caller-localized dialog name with a
  // conventional direction glyph until each surface injects its precise copy.
  const resolvedJumpLabel = jumpToLatestLabel ?? `${dialogLabel} ↓`;

  const dialog = (
    <div
      ref={containerRef}
      data-chat-shell-root
      role="dialog"
      aria-modal="true"
      aria-label={dialogLabel}
      tabIndex={-1}
      className={`${rootClass} overflow-hidden rounded-2xl border border-warm-200 bg-white shadow-2xl focus:outline-none`}
    >
      <header className="flex items-start justify-between gap-3 border-b border-warm-100 bg-gradient-to-r from-warm-50/90 to-brand/5 px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold text-ink-900">{title}</h2>
          {subtitle ? (
            <p className="mt-0.5 text-xs text-ink-500">{subtitle}</p>
          ) : null}
        </div>
        <div className="flex items-center gap-1">
          {onNewConversation && newConversationLabel ? (
            <button
              type="button"
              onClick={onNewConversation}
              className="text-ink-400 transition-colors hover:text-ink-700"
              aria-label={newConversationLabel}
              title={newConversationLabel}
            >
              <RotateCcw className="h-4 w-4" />
            </button>
          ) : null}
          <button
            type="button"
            onClick={onClose}
            className="text-ink-400 transition-colors hover:text-ink-700"
            aria-label={closeLabel}
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </header>

      <AssistantViewport
        className="flex-1"
        conversationLabel={conversationLabel ?? title}
        jumpToLatestLabel={resolvedJumpLabel}
        completionAnnouncement={errorMessage ? "" : completionAnnouncement}
        contentChange={contentChange}
        contentChangeOrigin={contentChangeOrigin}
        analytics={assistantAnalytics}
      >
        <div className="space-y-3 px-4 py-3 pb-8">
          <AnimatePresence initial={false}>
            {showWelcome ? (
              <motion.div
                key="welcome"
                initial={reduceMotion ? false : { opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -8 }}
                transition={{ duration: 0.35, ease: [0.22, 1, 0.36, 1] }}
                className="rounded-2xl border border-warm-200/80 bg-gradient-to-br from-white to-warm-50/80 p-4 shadow-sm"
              >
                <div className="flex items-start gap-3">
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-brand/10 text-brand">
                    <Sparkles className="h-4 w-4" />
                  </div>
                  <div className="min-w-0 space-y-1">
                    <p className="text-sm font-medium text-ink-900">
                      {welcomeMessage}
                    </p>
                    {welcomeHint ? (
                      <p className="text-xs leading-relaxed text-ink-500">
                        {welcomeHint}
                      </p>
                    ) : null}
                  </div>
                </div>
                {suggestions.length > 0 ? (
                  <div className="mt-3 flex flex-wrap gap-2">
                    {suggestions.map((suggestion, index) => (
                      <motion.button
                        key={suggestion}
                        type="button"
                        initial={
                          reduceMotion ? false : { opacity: 0, scale: 0.96 }
                        }
                        animate={{ opacity: 1, scale: 1 }}
                        transition={{
                          delay: reduceMotion ? 0 : 0.08 * (index + 1),
                          duration: 0.25,
                        }}
                        className="rounded-full border border-warm-200 bg-white px-3 py-1.5 text-left text-xs text-ink-700 shadow-sm transition-colors hover:border-brand/30 hover:bg-brand/5 hover:text-brand"
                        onClick={() => onSuggestionClick?.(suggestion)}
                      >
                        {suggestion}
                      </motion.button>
                    ))}
                  </div>
                ) : null}
              </motion.div>
            ) : null}
          </AnimatePresence>

          {renderedMessages.map(({ message, response, stableID }, index) => (
            <div
              key={stableID}
              ref={
                response && index === activeAssistantIndex
                  ? setLatestResponseStart
                  : undefined
              }
              data-testid={
                response && index === activeAssistantIndex
                  ? "assistant-response-start"
                  : undefined
              }
              className={message.role === "user" ? "text-right" : "text-left"}
            >
              <motion.div
                initial={
                  reduceMotion ? false : { opacity: 0, y: 10, scale: 0.98 }
                }
                animate={{ opacity: 1, y: 0, scale: 1 }}
                transition={{ duration: 0.28, ease: [0.22, 1, 0.36, 1] }}
              >
                {response ? (
                  <div className="inline-block max-w-[90%] rounded-2xl rounded-tl-sm border border-warm-200/60 bg-warm-100 px-3 py-2 text-left text-sm text-ink-900 shadow-sm">
                    <AssistantMessage
                      response={
                        index === latestAssistantIndex ||
                        response.follow_ups.length === 0
                          ? response
                          : { ...response, follow_ups: [] }
                      }
                      labels={
                        assistantLabels ??
                        compatibilityAssistantLabels({
                          response,
                          title,
                          dialogLabel,
                        })
                      }
                      onAction={(action) => {
                        if (onAssistantAction) {
                          onAssistantAction(action);
                          return;
                        }
                        const legacyAction = toLegacyAction(action);
                        if (legacyAction) onActionClick?.(legacyAction);
                      }}
                      onFollowUp={(followUp) =>
                        onSuggestionClick?.(followUp.prompt)
                      }
                      analytics={
                        assistantAnalytics
                          ? {
                              ...assistantAnalytics,
                              contract_version:
                                message.contractVersion ??
                                assistantAnalytics.contract_version,
                            }
                          : undefined
                      }
                    />
                  </div>
                ) : (
                  <div className="inline-block max-w-[90%] whitespace-pre-wrap rounded-2xl rounded-tr-sm bg-brand px-3 py-2 text-sm text-white shadow-sm">
                    {message.content}
                  </div>
                )}

                {index === latestAssistantIndex && feedback ? (
                  <div className="mt-1.5 flex items-center gap-1">
                    <button
                      type="button"
                      className="rounded-full border border-warm-200 bg-white p-1.5 text-ink-500 shadow-sm transition-colors hover:border-brand/30 hover:text-brand"
                      onClick={() => submitFeedback("up", "positive")}
                      aria-label={feedback.upLabel}
                      title={feedback.upLabel}
                    >
                      <ThumbsUp className="h-3.5 w-3.5" />
                    </button>
                    <button
                      type="button"
                      className="rounded-full border border-warm-200 bg-white p-1.5 text-ink-500 shadow-sm transition-colors hover:border-brand/30 hover:text-brand"
                      onClick={() => submitFeedback("down", "negative")}
                      aria-label={feedback.downLabel}
                      title={feedback.downLabel}
                    >
                      <ThumbsDown className="h-3.5 w-3.5" />
                    </button>
                  </div>
                ) : null}
              </motion.div>
            </div>
          ))}

          <AnimatePresence>
            {loading ? <TypingIndicator label={loadingLabel} /> : null}
          </AnimatePresence>

          {errorMessage ? (
            <div
              role="alert"
              className="flex flex-col items-start gap-2 text-left"
            >
              <div className="inline-block max-w-[90%] rounded-2xl rounded-tl-sm border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800 shadow-sm">
                {errorMessage}
              </div>
              {onRetry && retryLabel ? (
                <Button
                  size="sm"
                  variant="flat"
                  color="primary"
                  startContent={<RotateCcw className="h-3.5 w-3.5" />}
                  onPress={() => {
                    onRetry();
                    if (assistantAnalytics) {
                      trackAssistantEvent(
                        "assistant_retry_requested",
                        assistantEventProperties(assistantAnalytics),
                      );
                    }
                  }}
                >
                  {retryLabel}
                </Button>
              ) : null}
              {errorHelp ? (
                <div className="text-xs leading-relaxed text-ink-500">
                  {errorHelp}
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
      </AssistantViewport>

      <AssistantComposer
        labels={composerLabels}
        maxLength={maxInputLength}
        value={input}
        onValueChange={onInputChange}
        busy={loading}
        mobile={mobileComposer ?? coarsePointer}
        onSend={() => {
          onSend();
          if (assistantAnalytics) {
            trackAssistantEvent(
              "assistant_message_sent",
              assistantEventProperties(assistantAnalytics, {
                message_size_bucket: Array.from(input.trim()).length,
              }),
            );
          }
        }}
      />
    </div>
  );

  if (portal) {
    if (!mounted || typeof document === "undefined") return null;
    return createPortal(dialog, document.body);
  }
  return dialog;
}
