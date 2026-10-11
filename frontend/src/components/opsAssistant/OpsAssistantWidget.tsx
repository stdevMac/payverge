"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useRouter } from "next/navigation";
import { HelpCircle } from "lucide-react";
import { motion, AnimatePresence, useReducedMotion } from "framer-motion";
import ChatShell from "@/components/chat/ChatShell";
import type { AssistantMessageLabels } from "@/components/assistant/AssistantMessage";
import {
  askOpsAssistant,
  listOpsThreadMessages,
  newClientRequestId,
  readOpsSession,
  submitOpsFeedback,
  writeDirectorHandoff,
  writeOpsSession,
  type ChatMessage,
  OpsHistoryContractMismatchError,
  OpsResponseContractMismatchError,
} from "@/api/opsAssistant";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { isSafeHref } from "@/components/chat/extractLeakedActions";
import { trackEvent } from "@/utils/analytics";
import { brandLinks } from "@/config/brand";
import type { AssistantAction, AssistantResponse } from "@/types/assistant";
import {
  assistantEventProperties,
  type AssistantAnalyticsContext,
  trackAssistantEvent,
} from "@/components/assistant/assistantAnalytics";

const OPS_DASHBOARD_TABS = [
  "overview",
  "bills",
  "cash-register",
  "kitchen",
  "reservations",
  "menu",
  "tables",
  "ai-waiter",
  "director-console",
  "marketing",
  "analytics",
  "crm",
  "delivery",
  "counter",
  "inventory",
  "staff",
  "schedule",
  "business-page",
  "accounting",
  "fiscal",
  "plugins",
  "settings",
] as const;

function isCanonicalOpsHref(businessId: number, href: string): boolean {
  if (href === `/business/${businessId}/settings/printers`) return true;
  return OPS_DASHBOARD_TABS.some(
    (tab) => href === `/business/${businessId}/dashboard?tab=${tab}`,
  );
}

const statusLabelKeys: Record<AssistantResponse["status"], string> = {
  complete: "assistant.statusComplete",
  needs_clarification: "assistant.statusNeedsClarification",
  blocked: "assistant.statusBlocked",
  degraded: "assistant.statusDegraded",
};

interface OpsAssistantWidgetProps {
  businessId: number;
  activeTab: string;
  isStaffUser?: boolean;
  openSignal?: number;
  /** Anchor inside dashboard main column instead of the viewport corner. */
  dock?: "viewport" | "content";
  /** Hide the floating Help pill when another labeled Help control is on screen. */
  hideFab?: boolean;
}

export default function OpsAssistantWidget({
  businessId,
  activeTab,
  isStaffUser,
  openSignal = 0,
  dock = "content",
  hideFab = false,
}: OpsAssistantWidgetProps) {
  const router = useRouter();
  const { locale } = useSimpleLocale();
  const reduceMotion = useReducedMotion();
  const t = useCallback(
    (k: string) => String(getTranslation(`opsAssistant.${k}`, locale)),
    [locale],
  );
  const formatLabel = useCallback(
    (key: string, values: Record<string, string | number>) => {
      let value = t(key);
      for (const [name, replacement] of Object.entries(values)) {
        value = value.replaceAll(`{${name}}`, String(replacement));
      }
      return value;
    },
    [t],
  );
  const assistantLabels = useMemo<AssistantMessageLabels>(
    () => ({
      usedSources: (count) =>
        formatLabel(
          count === 1 ? "assistant.usedSource" : "assistant.usedSources",
          { count },
        ),
      sourcesRegion: (count) =>
        formatLabel(
          count === 1 ? "assistant.sourceRegion" : "assistant.sourcesRegion",
          { count },
        ),
      sourceOrigin: () => t("assistant.verifiedSource"),
      externalSource: (hostname) =>
        formatLabel("assistant.externalSource", { hostname }),
      actions: t("assistant.actions"),
      steps: t("assistant.steps"),
      entities: t("assistant.entities"),
      entityAvailability: (availability) =>
        formatLabel("assistant.availability", { availability }),
      followUps: t("assistant.continuation"),
      notices: t("assistant.notices"),
      noticeKind: () => t("assistant.notice"),
      status: (status) => t(statusLabelKeys[status]),
      workflowProgress: ({ current, total }) =>
        formatLabel("assistant.workflowProgress", { current, total }),
      disabledActionReason: (reason) => reason ?? t("assistant.disabledAction"),
      renderError: t("assistant.renderError"),
    }),
    [formatLabel, t],
  );
  const suggestions = useMemo(() => {
    const raw = getTranslation("opsAssistant.suggestions", locale);
    return Array.isArray(raw) ? raw.map(String) : [];
  }, [locale]);

  const [open, setOpen] = useState(false);
  const assistantAnalytics = useMemo<AssistantAnalyticsContext>(
    () => ({ surface: "ops", locale }),
    [locale],
  );
  const assistantAnalyticsRef = useRef(assistantAnalytics);
  assistantAnalyticsRef.current = assistantAnalytics;
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [threadId, setThreadId] = useState<number | undefined>();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [failedMessage, setFailedMessage] = useState<string | null>(null);
  /** Stable id reused for retry so backend claim ledger dedupes. */
  const [pendingRequestId, setPendingRequestId] = useState<string | null>(null);
  const [lastAssistantMessageId, setLastAssistantMessageId] = useState<
    number | undefined
  >();

  const fabRef = useRef<HTMLButtonElement>(null);
  const prevOpen = useRef(false);
  const assistantOpenRef = useRef(false);
  const handledOpenSignalRef = useRef(0);
  const requestGenerationRef = useRef(0);
  const openAssistant = useCallback(() => {
    if (assistantOpenRef.current) return;
    assistantOpenRef.current = true;
    setOpen(true);
    trackAssistantEvent(
      "assistant_opened",
      assistantEventProperties(assistantAnalyticsRef.current),
    );
  }, []);
  const closeAssistant = useCallback(() => {
    if (!assistantOpenRef.current) return;
    assistantOpenRef.current = false;
    setOpen(false);
    trackAssistantEvent(
      "assistant_closed",
      assistantEventProperties(assistantAnalyticsRef.current),
    );
  }, []);
  useEffect(() => {
    if (openSignal <= 0 || handledOpenSignalRef.current === openSignal) return;
    handledOpenSignalRef.current = openSignal;
    openAssistant();
  }, [openAssistant, openSignal]);

  // Restore active thread from same-tab sessionStorage (Wave 3 continuity).
  useEffect(() => {
    let current = true;
    const generation = requestGenerationRef.current + 1;
    requestGenerationRef.current = generation;
    const saved = readOpsSession(businessId);
    setInput("");
    setLoading(false);
    setFailedMessage(null);
    setPendingRequestId(null);
    setMessages([]);
    setLastAssistantMessageId(undefined);
    if (!saved?.threadId) {
      setThreadId(undefined);
      return () => {
        current = false;
        if (requestGenerationRef.current === generation) {
          requestGenerationRef.current += 1;
        }
      };
    }
    setThreadId(saved.threadId);
    void listOpsThreadMessages(businessId, saved.threadId)
      .then((res) => {
        if (!current || requestGenerationRef.current !== generation) return;
        const restored = (res.messages || []).map((m) => ({
          role:
            m.role === "assistant" ? ("assistant" as const) : ("user" as const),
          content: m.response_v2?.answer.content ?? m.content,
          response: m.role === "assistant" ? m.response_v2 : undefined,
          contractVersion: m.contract_version,
        }));
        setMessages(restored);
        const lastAssistant = [...res.messages]
          .reverse()
          .find((message) => message.role === "assistant");
        setLastAssistantMessageId(lastAssistant?.id);
        trackEvent("ops_assistant_thread_restored", {});
      })
      .catch((error: unknown) => {
        if (!current || requestGenerationRef.current !== generation) return;
        if (error instanceof OpsHistoryContractMismatchError) {
          trackAssistantEvent(
            "assistant_history_contract_mismatch",
            assistantEventProperties({
              ...assistantAnalyticsRef.current,
              contract_version: "v2",
            }),
          );
          trackAssistantEvent(
            "assistant_render_fallback",
            assistantEventProperties({
              ...assistantAnalyticsRef.current,
              contract_version: "v2",
            }),
          );
        }
        writeOpsSession(businessId, undefined);
        setThreadId(undefined);
      });
    return () => {
      current = false;
      if (requestGenerationRef.current === generation) {
        requestGenerationRef.current += 1;
      }
    };
  }, [activeTab, businessId]);

  useEffect(() => {
    // Restore focus to the launcher when the panel closes.
    if (prevOpen.current && !open) fabRef.current?.focus();
    prevOpen.current = open;
  }, [open]);

  const onAssistantAction = useCallback(
    (action: AssistantAction) => {
      if (action.state !== "ready" || action.confirmation !== "none") {
        return "failed" as const;
      }
      if (action.type === "director_handoff") {
        const prompt =
          messages.filter((m) => m.role === "user").slice(-1)[0]?.content ?? "";
        writeDirectorHandoff(businessId, prompt);
        router.push(`/business/${businessId}/dashboard?tab=director-console`);
        closeAssistant();
        return "completed" as const;
      }
      if (
        action.type === "navigate" &&
        isCanonicalOpsHref(businessId, action.target.href)
      ) {
        router.push(action.target.href);
        closeAssistant();
        return "completed" as const;
      }
      if (action.type === "external_link" && isSafeHref(action.target.href)) {
        window.open(action.target.href, "_blank", "noopener");
        return "completed" as const;
      }
      return "failed" as const;
    },
    [businessId, closeAssistant, messages, router],
  );

  const runAsk = useCallback(
    async (trimmed: string, requestId: string) => {
      const generation = requestGenerationRef.current;
      setLoading(true);
      setFailedMessage(null);
      try {
        const res = await askOpsAssistant(businessId, {
          message: trimmed,
          thread_id: threadId,
          locale,
          active_tab: activeTab,
          client_request_id: requestId,
        });
        if (requestGenerationRef.current !== generation) return;
        setThreadId(res.thread.id);
        writeOpsSession(businessId, res.thread.id);
        setLastAssistantMessageId(res.assistant_message?.id);
        setPendingRequestId(null);
        setMessages((m) => [
          ...m,
          {
            role: "assistant",
            content: res.response_v2.answer.content,
            response: res.response_v2,
            contractVersion: res.contract_version,
          },
        ]);
      } catch (error: unknown) {
        if (requestGenerationRef.current !== generation) return;
        if (error instanceof OpsResponseContractMismatchError) {
          trackAssistantEvent(
            "assistant_render_fallback",
            assistantEventProperties({
              ...assistantAnalyticsRef.current,
              contract_version: "v2",
            }),
          );
        }
        // Remember the failed prompt + request id so retry reuses the claim.
        setFailedMessage(trimmed);
        setPendingRequestId(requestId);
      } finally {
        if (requestGenerationRef.current === generation) {
          setLoading(false);
        }
      }
    },
    [activeTab, businessId, locale, threadId],
  );

  const sendMessage = useCallback(
    async (text: string) => {
      const trimmed = text.trim();
      if (!trimmed || loading) return;
      setInput("");
      setMessages((m) => [...m, { role: "user", content: trimmed }]);
      const requestId = newClientRequestId();
      setPendingRequestId(requestId);
      await runAsk(trimmed, requestId);
    },
    [loading, runAsk],
  );

  const onSend = () => void sendMessage(input);

  const onRetry = useCallback(() => {
    if (!failedMessage) return;
    // Reuse the same client_request_id for idempotent ledger claim.
    const rid = pendingRequestId || newClientRequestId();
    void runAsk(failedMessage, rid);
  }, [failedMessage, pendingRequestId, runAsk]);

  const startNewConversation = useCallback(() => {
    requestGenerationRef.current += 1;
    setThreadId(undefined);
    setMessages([]);
    setFailedMessage(null);
    setPendingRequestId(null);
    setLastAssistantMessageId(undefined);
    setLoading(false);
    writeOpsSession(businessId, undefined);
    trackEvent("ops_assistant_new_conversation", {});
  }, [businessId]);

  const onFeedback = useCallback(
    async (vote: "up" | "down") => {
      if (!lastAssistantMessageId) return;
      try {
        await submitOpsFeedback(businessId, lastAssistantMessageId, vote);
        return true;
      } catch {
        return false;
      }
    },
    [businessId, lastAssistantMessageId],
  );

  const anchorClass = dock === "content" ? "absolute" : "fixed";
  const cornerClass = "bottom-5 right-4 sm:bottom-6 sm:right-6";
  const panelHeightClass =
    dock === "content"
      ? "h-[min(560px,calc(100%-5rem))]"
      : "h-[min(560px,calc(100vh-6rem))]";

  return (
    <>
      {!open && !hideFab && (
        <motion.button
          ref={fabRef}
          type="button"
          data-testid="ops-assistant-fab"
          onClick={() => {
            openAssistant();
          }}
          initial={reduceMotion ? false : { opacity: 0, scale: 0.85, y: 16 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          transition={{
            type: "spring",
            stiffness: 420,
            damping: 26,
            delay: reduceMotion ? 0 : 0.2,
          }}
          whileHover={reduceMotion ? undefined : { scale: 1.05 }}
          whileTap={reduceMotion ? undefined : { scale: 0.96 }}
          className={`${anchorClass} ${cornerClass} z-40 flex h-12 items-center justify-center gap-2 overflow-hidden rounded-full border border-warm-200 bg-white px-4 text-brand shadow-[0_12px_32px_rgba(46,42,37,0.12)] hover:border-brand/30 hover:bg-brand/5 transition-colors sm:h-14 sm:px-5`}
          aria-label={t("fabLabel")}
        >
          {!reduceMotion ? (
            <motion.span
              aria-hidden
              className="pointer-events-none absolute inset-0 rounded-full border-2 border-brand/30"
              animate={{ scale: [1, 1.14, 1], opacity: [0.6, 0, 0.6] }}
              transition={{
                duration: 2.6,
                repeat: Infinity,
                ease: "easeInOut",
              }}
            />
          ) : null}
          <HelpCircle className="relative h-5 w-5 sm:h-6 sm:w-6" aria-hidden />
          <span className="relative text-sm font-semibold tracking-wide">
            {t("fabLabel")}
          </span>
        </motion.button>
      )}
      <AnimatePresence>
        {open ? (
          <ChatShell
            title={t("title")}
            subtitle={isStaffUser ? t("subtitleStaff") : t("subtitle")}
            dialogLabel={t("title")}
            closeLabel={t("close")}
            messages={messages}
            input={input}
            onInputChange={setInput}
            onSend={onSend}
            onClose={closeAssistant}
            loading={loading}
            onAssistantAction={onAssistantAction}
            placeholder={t("placeholder")}
            sendLabel={t("send")}
            welcomeMessage={t("welcome")}
            welcomeHint={t("welcomeHint")}
            suggestions={suggestions}
            onSuggestionClick={(text) => void sendMessage(text)}
            loadingLabel={t("thinking")}
            assistantLabels={assistantLabels}
            conversationLabel={t("assistant.conversation")}
            jumpToLatestLabel={t("assistant.jumpToLatest")}
            completionAnnouncement={t("assistant.completion")}
            characterCountLabel={(current, maximum) =>
              formatLabel("assistant.characterCount", { current, maximum })
            }
            // Portaled to document.body so background inert covers the app tree.
            className={`${anchorClass} ${cornerClass} z-50 w-[min(380px,calc(100vw-2rem))] ${panelHeightClass} sm:w-[min(400px,calc(100%-2rem))] flex flex-col`}
            errorMessage={failedMessage ? t("errors.generic") : null}
            onRetry={onRetry}
            retryLabel={t("errors.retry")}
            errorHelp={
              <span>
                {t("errors.contactPrompt")}{" "}
                <a
                  href={brandLinks.supportUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-brand underline hover:text-brand-700"
                >
                  {t("errors.contactCta")}
                </a>
              </span>
            }
            onNewConversation={startNewConversation}
            newConversationLabel={t("newConversation")}
            feedback={
              lastAssistantMessageId
                ? {
                    onVote: onFeedback,
                    upLabel: t("feedbackUp"),
                    downLabel: t("feedbackDown"),
                  }
                : null
            }
            assistantAnalytics={assistantAnalytics}
          />
        ) : null}
      </AnimatePresence>
    </>
  );
}
