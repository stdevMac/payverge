"use client";

import React from "react";
import {
  ArrowRight,
  CheckCircle2,
  Circle,
  Copy,
  RefreshCcw,
  ThumbsDown,
  ThumbsUp,
} from "lucide-react";
import { Chip } from "@nextui-org/react";
import type {
  DirectorAction,
  DirectorThreadMessage,
} from "@/api/directorConsole";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";

import DirectorMessage from "./DirectorMessage";
import {
  shouldHideUngroundedRevenueAdvice,
  structuredAdviceText,
} from "./revenueHonesty";

export interface AssistantMessageProps {
  message: DirectorThreadMessage;
  isFirstAssistant: boolean;
  /**
   * L4-15: regenerate replaces the LAST assistant turn server-side, so the
   * affordance may only appear on the trailing assistant message. Offering it
   * on earlier turns would hard-delete an unrelated trailing answer and append
   * a misplaced reply. Defaults to hidden (safe).
   */
  isTrailing?: boolean;
  /** Sage's display name — voiced as an sr-only speaker label so screen-reader
   *  users can tell turns apart now that the per-turn avatar is gone. */
  assistantName?: string;
  onActionClick: (action: DirectorAction) => void;
  onFeedback: (messageId: number, vote: "up" | "down") => void;
  onFollowUpClick: (followUp: string) => void;
  onCopy: (text: string) => void;
  onRegenerate: (messageId: number) => void;
}

// Priority encoded as a Pre-Shift-style tone icon-circle (warm fill, never a
// clinical dot). High = urgent rose, medium = watch amber, low = calm brand.
const PRIORITY_TONE: Record<string, string> = {
  high: "bg-rose-50 text-rose-600",
  medium: "bg-amber-50 text-amber-600",
  low: "bg-brand/10 text-brand",
};

export default function AssistantMessage({
  message,
  isFirstAssistant,
  isTrailing = false,
  assistantName = "Sage",
  onActionClick,
  onFeedback,
  onFollowUpClick,
  onCopy,
  onRegenerate,
}: AssistantMessageProps) {
  const { locale } = useSimpleLocale();
  const t = React.useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const value = getTranslation(
        `directorConsole.${key}`,
        locale,
        params,
      );
      return Array.isArray(value) ? value[0] || key : (value as string);
    },
    [locale],
  );

  // A structured_response can arrive as an all-empty object (older rows,
  // seeded messages) — rendering it wins over `content` and paints a blank
  // transcript. Only take the structured path when it actually says something.
  const rawStructured = message.structured_response;
  const structured =
    rawStructured &&
    (rawStructured.summary?.trim() ||
      (rawStructured.actions || []).length > 0 ||
      rawStructured.diagnosis?.trim() ||
      (rawStructured.evidence || []).length > 0 ||
      rawStructured.expected_impact?.trim() ||
      (rawStructured.follow_ups || []).length > 0)
      ? rawStructured
      : null;
  // Actions are always shown directly. The quiet "See the details" disclosure
  // opens by default only on the first reply (otherwise stays collapsed).
  const expandedDefault = isFirstAssistant;

  const hidePlainRevenue =
    !structured && shouldHideUngroundedRevenueAdvice(message.content);
  const hideStructuredRevenue =
    !!structured &&
    shouldHideUngroundedRevenueAdvice(structuredAdviceText(structured));

  if (hidePlainRevenue || hideStructuredRevenue) {
    const honesty = t("honesty.notEnoughData");
    return (
      <div className="group max-w-[72ch] border-l-2 border-brand/50 pl-4 space-y-3">
        <span className="sr-only">{assistantName}: </span>
        <p
          data-testid="dc-revenue-not-enough-data"
          className="text-body-lg font-medium text-ink-900 leading-snug"
        >
          {honesty}
        </p>
        <p className="text-body text-ink-600">{t("honesty.notEnoughDataDetail")}</p>
        <button
          type="button"
          data-testid="dc-revenue-open-analytics"
          onClick={() =>
            onActionClick({
              title: t("honesty.openAnalytics"),
              description: honesty,
              deep_link: `/business/${message.business_id}/dashboard?tab=analytics`,
              priority: "medium",
            })
          }
          className="inline-flex items-center rounded-full border border-brand/30 bg-brand/[0.06] px-3 py-1.5 text-body-sm font-semibold text-brand transition-colors hover:bg-brand/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
        >
          {t("honesty.openAnalytics")}
        </button>
        <Actions
          t={t}
          onCopy={() => onCopy(honesty)}
          onRegenerate={() => onRegenerate(message.id)}
          showRegenerate={isTrailing}
          onFeedback={(v) => onFeedback(message.id, v)}
          vote={message.feedback_vote}
        />
      </div>
    );
  }

  if (!structured) {
    return (
      <div className="group max-w-[72ch] border-l-2 border-brand/50 pl-4">
        <span className="sr-only">{assistantName}: </span>
        <DirectorMessage content={message.content} role="assistant" />
        <Actions
          t={t}
          onCopy={() => onCopy(message.content)}
          onRegenerate={() => onRegenerate(message.id)}
          showRegenerate={isTrailing}
          onFeedback={(v) => onFeedback(message.id, v)}
          vote={message.feedback_vote}
        />
      </div>
    );
  }

  return (
    <div className="group max-w-[72ch] border-l-2 border-brand/50 pl-4 space-y-3">
      {/* 3.1 — the takeaway reads like a spoken line, not a serif headline. */}
      <p className="text-body-lg font-medium text-ink-900 leading-snug">
        <span className="sr-only">{assistantName}: </span>
        {structured.summary}
      </p>

      {/* 3.2 — the actionable core, surfaced directly as Pre-Shift-style cards. */}
      {(structured.actions || []).length > 0 ? (
        <div className="space-y-2">
          {structured.actions.map((action, idx) => (
            <button
              type="button"
              key={`${message.id}-act-${idx}`}
              onClick={() => onActionClick(action)}
              aria-label={`${t("actions.open")} · ${action.title}`}
              className="group/action w-full text-left rounded-xl border border-warm-100 bg-white px-4 py-3 flex flex-row items-center gap-4 transition-colors hover:bg-warm-50/70 hover:border-warm-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-white"
            >
              <span
                data-testid="dc-action-priority"
                title={action.priority}
                className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full ${
                  PRIORITY_TONE[action.priority] || "bg-warm-100 text-ink-500"
                }`}
              >
                <Circle className="w-2.5 h-2.5 fill-current" />
                <span className="sr-only">{action.priority}</span>
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-body-sm font-semibold text-ink-900">
                  {action.title}
                </span>
                <span className="block text-label text-ink-600">
                  {action.description}
                </span>
              </span>
              <span className="flex flex-shrink-0 items-center gap-1 rounded-full px-3 py-1 text-body-sm font-semibold text-brand transition-colors group-hover/action:bg-brand/10">
                {t("actions.open")}
                <ArrowRight className="w-4 h-4" />
              </span>
            </button>
          ))}
        </div>
      ) : null}

      {/* 3.3 — one quiet disclosure replaces the lab accordions. */}
      {structured.diagnosis ||
      (structured.evidence || []).length > 0 ||
      structured.expected_impact ? (
        <details open={expandedDefault}>
          <summary className="cursor-pointer text-body-sm text-ink-500 hover:text-ink-700">
            {t("chat.details")}
          </summary>
          <div className="mt-3 space-y-3">
            {structured.diagnosis ? (
              <div>
                <div className="text-body-sm font-medium text-ink-500">
                  {t("sections.context")}
                </div>
                <p className="mt-1 text-body text-ink-800">
                  {structured.diagnosis}
                </p>
              </div>
            ) : null}

            {(structured.evidence || []).length > 0 ? (
              <div>
                <div className="text-body-sm font-medium text-ink-500">
                  {t("sections.signals")}
                </div>
                <ul className="mt-1 list-disc pl-5 text-body text-ink-700 space-y-1">
                  {structured.evidence.map((item, idx) => (
                    <li key={`${message.id}-evi-${idx}`}>{item}</li>
                  ))}
                </ul>
              </div>
            ) : null}

            {structured.expected_impact ? (
              <div>
                <div className="text-body-sm font-medium text-ink-500">
                  {t("sections.outlook")}
                </div>
                <p className="mt-1 text-body text-ink-800">
                  {structured.expected_impact}
                </p>
              </div>
            ) : null}
          </div>
        </details>
      ) : null}

      {(structured.follow_ups || []).length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {structured.follow_ups.map((followUp, idx) => (
            <Chip
              key={`${message.id}-fu-${idx}`}
              as="button"
              type="button"
              size="sm"
              variant="flat"
              className="cursor-pointer hover:opacity-80"
              onClick={() => onFollowUpClick(followUp)}
            >
              {followUp}
            </Chip>
          ))}
        </div>
      ) : null}

      <Actions
        t={t}
        onCopy={() => onCopy(structured.summary)}
        onRegenerate={() => onRegenerate(message.id)}
        showRegenerate={isTrailing}
        onFeedback={(v) => onFeedback(message.id, v)}
        vote={message.feedback_vote}
      />
    </div>
  );
}

function Actions({
  t,
  onCopy,
  onRegenerate,
  showRegenerate,
  onFeedback,
  vote,
}: {
  t: (k: string) => string;
  onCopy: () => void;
  onRegenerate: () => void;
  /** L4-15: regenerate only exists on the trailing assistant turn. */
  showRegenerate: boolean;
  onFeedback: (v: "up" | "down") => void;
  vote?: "up" | "down";
}) {
  return (
    <div className="pt-1 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity flex items-center gap-1 text-ink-400">
      <button
        aria-label={t("feedback.helpful")}
        onClick={() => onFeedback("up")}
        className={`p-1 rounded hover:text-emerald-600 ${vote === "up" ? "text-emerald-600" : ""}`}
      >
        {vote === "up" ? (
          <CheckCircle2 className="w-4 h-4" />
        ) : (
          <ThumbsUp className="w-4 h-4" />
        )}
      </button>
      <button
        aria-label={t("feedback.notHelpful")}
        onClick={() => onFeedback("down")}
        className={`p-1 rounded hover:text-rose-600 ${vote === "down" ? "text-rose-600" : ""}`}
      >
        <ThumbsDown className="w-4 h-4" />
      </button>
      <button
        aria-label={t("actions.copy")}
        onClick={onCopy}
        className="p-1 rounded hover:text-ink-700"
      >
        <Copy className="w-4 h-4" />
      </button>
      {showRegenerate ? (
        <button
          aria-label={t("actions.regenerate")}
          onClick={onRegenerate}
          className="p-1 rounded hover:text-ink-700"
        >
          <RefreshCcw className="w-4 h-4" />
        </button>
      ) : null}
    </div>
  );
}
