"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Spinner } from "@nextui-org/react";
import {
  ChevronLeft,
  ChevronRight,
  Hash,
  MessageSquare,
  MessagesSquare,
  SendHorizonal,
  Trash2,
} from "lucide-react";
import { chatApi, type ChatChannel, type ChatMessage } from "@/api/chat";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { useToast } from "@/contexts/ToastContext";
import { EmptyState } from "@/components/ui/EmptyState";
import { intlLocaleFor } from "@/utils/intlLocale";
import { PremiumPanel } from "../premium";
import ConfirmationModal from "../modals/ConfirmationModal";
import { CHAT_CONTENT_MAX_LENGTH } from "./chatFieldLimits";

// Operator team-chat surface (mounted in the Schedule tab alongside Approvals /
// Timesheet review). Managers/owners read role + department channels and post to
// them; with chat:moderate they can soft-delete a message — but NEVER on a DM
// channel (the server 403s a DM delete, so the affordance is hidden there). Money
// free. Self-resolves the dashboardChat namespace.

const PAGE = 50;

/**
 * Compact "how long ago" for the channel-list vitality strip: minutes → hours →
 * days via Intl.RelativeTimeFormat (narrow), falling back to a short date past a
 * week so old rooms read as dates, not "-93d".
 */
function relativeTime(iso: string, locale: string): string {
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "";
  const intl = intlLocaleFor(locale);
  const diffMin = Math.round((Date.now() - then) / 60000);
  const rtf = new Intl.RelativeTimeFormat(intl, { style: "narrow" });
  if (diffMin < 1) return rtf.format(0, "minute");
  if (diffMin < 60) return rtf.format(-diffMin, "minute");
  const diffH = Math.round(diffMin / 60);
  if (diffH < 24) return rtf.format(-diffH, "hour");
  const diffD = Math.round(diffH / 24);
  if (diffD <= 7) return rtf.format(-diffD, "day");
  return new Intl.DateTimeFormat(intl, {
    month: "short",
    day: "numeric",
  }).format(new Date(iso));
}

interface TeamChatPanelProps {
  businessId: string;
  /** chat:moderate holder (manager/owner). Gates the per-message delete action. */
  canModerate: boolean;
}

export default function TeamChatPanel({
  businessId,
  canModerate,
}: TeamChatPanelProps) {
  const { locale } = useSimpleLocale();
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<ChatChannel | null>(null);

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardChat.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  // Shared cache key with useChatUnreadCount (Team sidebar badge) — keep
  // queryFn semantics (throw on error, full response shape) in sync.
  const channelsKey = queryKeys.chat.channels(businessId);
  const channelsQuery = useQuery({
    queryKey: channelsKey,
    queryFn: () => chatApi.listChannels(businessId),
  });

  // A new message anywhere refreshes the channel list (unread map); the open
  // thread refetches itself below.
  useStaffRealtime({
    businessId: Number(businessId),
    onChatMessage: () => {
      void queryClient.invalidateQueries({ queryKey: channelsKey });
    },
    // Nudges dropped during an SSE gap are gone — resync the unread map.
    onReconnect: () => {
      void queryClient.invalidateQueries({ queryKey: channelsKey });
    },
  });

  const channelName = useCallback(
    (ch: ChatChannel): string => {
      if (ch.type === "role") {
        if (ch.ref_key.startsWith("role:")) {
          return t(`chat.roles.${ch.ref_key.slice(5)}`) || ch.name;
        }
        if (ch.ref_key.startsWith("dept:")) {
          return ch.name || ch.ref_key.slice(5);
        }
      }
      if (ch.type === "direct") return ch.name || t("chat.fallback.direct");
      if (ch.type === "group") return ch.name || t("chat.fallback.group");
      return ch.name || t("chat.fallback.channel");
    },
    [t],
  );

  const channelKind = useCallback(
    (ch: ChatChannel): string => {
      if (ch.type === "role") {
        return ch.ref_key.startsWith("dept:")
          ? t("chat.kind.dept")
          : t("chat.kind.role");
      }
      if (ch.type === "direct") return t("chat.kind.direct");
      if (ch.type === "group") return t("chat.kind.group");
      return t("chat.kind.role");
    },
    [t],
  );

  if (selected) {
    return (
      <OperatorThread
        businessId={businessId}
        channel={selected}
        // The server 403s a DM moderation delete, so only non-DM channels are
        // moderatable here even for a chat:moderate holder.
        canModerate={canModerate && selected.type !== "direct"}
        title={channelName(selected)}
        locale={locale}
        t={t}
        onBack={() => setSelected(null)}
        onMutated={() =>
          queryClient.invalidateQueries({ queryKey: channelsKey })
        }
      />
    );
  }

  const channels = channelsQuery.data?.channels ?? [];
  const unreadMap = channelsQuery.data?.unread ?? {};
  const previews = channelsQuery.data?.previews ?? {};

  return (
    <PremiumPanel
      as="section"
      aria-label={t("chat.title")}
      className="overflow-hidden"
      withTexture
    >
      <div className="border-b border-warm-200 bg-warm-50/70 p-4">
        <h2 className="text-base font-semibold tracking-0 text-ink-950">
          {t("chat.title")}
        </h2>
        <p className="text-sm text-ink-600">{t("chat.subtitle")}</p>
      </div>

      <div className="p-4">
        {channelsQuery.isLoading ? (
          <div
            role="status"
            aria-live="polite"
            className="flex min-h-[20vh] items-center justify-center"
          >
            <Spinner aria-label={t("chat.loading")} />
          </div>
        ) : channelsQuery.isError ? (
          <p className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {t("chat.error")}
          </p>
        ) : channels.length === 0 ? (
          <EmptyState
            icon={MessagesSquare}
            title={t("chat.emptyTitle")}
            subtitle={t("chat.emptySubtitle")}
          />
        ) : (
          <>
            <p className="mb-3 text-sm text-ink-600">
              {t("chat.selectPrompt")}
            </p>
            <ul className="space-y-2">
              {channels.map((ch) => {
                const isDirect = ch.type === "direct" || ch.type === "group";
                const Icon = isDirect ? MessageSquare : Hash;
                const unreadCount = unreadMap[String(ch.id)] ?? 0;
                const preview = previews[String(ch.id)];
                return (
                  <li key={ch.id}>
                    <button
                      type="button"
                      onClick={() => setSelected(ch)}
                      className="flex w-full items-center gap-3 rounded-2xl border border-warm-200/90 bg-white/75 p-3 text-left shadow-sm transition hover:border-brand/40 hover:bg-brand/5"
                    >
                      <span
                        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl border border-warm-200 bg-warm-50 text-brand"
                        aria-hidden="true"
                      >
                        <Icon className="h-4 w-4" />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex items-baseline justify-between gap-2">
                          <span className="truncate text-sm font-semibold text-ink-950">
                            {channelName(ch)}
                          </span>
                          {preview ? (
                            <span className="shrink-0 text-[11px] text-ink-500">
                              {relativeTime(preview.created_at, locale)}
                            </span>
                          ) : null}
                        </span>
                        <span className="block truncate text-xs text-ink-600">
                          {preview
                            ? `${preview.sender_name}: ${preview.snippet}`
                            : channelKind(ch)}
                        </span>
                      </span>
                      {unreadCount > 0 ? (
                        <span className="inline-flex h-5 min-w-[20px] shrink-0 items-center justify-center rounded-full bg-brand px-1.5 text-[11px] font-semibold text-white">
                          {unreadCount > 99 ? "99+" : unreadCount}
                        </span>
                      ) : null}
                      <ChevronRight
                        className="h-4 w-4 shrink-0 text-ink-400"
                        aria-hidden="true"
                      />
                    </button>
                  </li>
                );
              })}
            </ul>
          </>
        )}
      </div>
    </PremiumPanel>
  );
}

function OperatorThread({
  businessId,
  channel,
  canModerate,
  title,
  locale,
  t,
  onBack,
  onMutated,
}: {
  businessId: string;
  channel: ChatChannel;
  canModerate: boolean;
  title: string;
  locale: string;
  t: (key: string) => string;
  onBack: () => void;
  onMutated: () => void;
}) {
  const toast = useToast();
  const channelId = channel.id;
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading",
  );
  const [hasMore, setHasMore] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [pendingDeleteId, setPendingDeleteId] = useState<number | null>(null);

  const loadLatest = useCallback(async () => {
    try {
      const page = await chatApi.listMessages(businessId, channelId, {
        limit: PAGE,
      });
      setMessages(page.slice().reverse());
      setHasMore(page.length >= PAGE);
      setStatus("ready");
      // Reading the channel clears its unread badge (mirrors staff ChatThread).
      // Best-effort: a failed marker just leaves the badge. Refresh the channel
      // list afterward so the Team unread map / sidebar badge reflect the read.
      void chatApi
        .markRead(businessId, channelId)
        .then(() => onMutated())
        .catch(() => {});
    } catch {
      setStatus("error");
    }
  }, [businessId, channelId, onMutated]);

  useEffect(() => {
    setStatus("loading");
    void loadLatest();
  }, [loadLatest]);

  useStaffRealtime({
    businessId: Number(businessId),
    onChatMessage: (p) => {
      if (p.channel_id === channelId) void loadLatest();
    },
    // Messages that landed during an SSE gap never nudged us — reload the tail.
    onReconnect: () => {
      void loadLatest();
    },
  });

  const loadOlder = useCallback(async () => {
    const oldest = messages[0];
    if (!oldest || loadingOlder) return;
    setLoadingOlder(true);
    try {
      const page = await chatApi.listMessages(businessId, channelId, {
        cursor: oldest.id,
        limit: PAGE,
      });
      setMessages((prev) => [...page.slice().reverse(), ...prev]);
      setHasMore(page.length >= PAGE);
    } catch {
      toast.showError(t("chat.error"));
    } finally {
      setLoadingOlder(false);
    }
  }, [messages, businessId, channelId, loadingOlder, toast, t]);

  const send = useCallback(async () => {
    const content = draft.trim();
    if (!content || sending) return;
    setSending(true);
    try {
      const msg = await chatApi.postMessage(businessId, channelId, content);
      setMessages((prev) => [...prev, msg]);
      setDraft("");
      // Advance the read marker past our own message so the badge doesn't count
      // it, then refresh the channel list (unread map + preview).
      void chatApi.markRead(businessId, channelId, msg.id).catch(() => {});
      onMutated();
    } catch {
      toast.showError(t("chat.error"));
    } finally {
      setSending(false);
    }
  }, [draft, sending, businessId, channelId, toast, t, onMutated]);

  const remove = useCallback(
    async (messageId: number) => {
      setDeletingId(messageId);
      try {
        await chatApi.deleteMessage(businessId, messageId);
        setMessages((prev) => prev.filter((m) => m.id !== messageId));
        toast.showSuccess(t("chat.deleted"));
        onMutated();
      } catch {
        toast.showError(t("chat.deleteError"));
      } finally {
        setDeletingId(null);
        setPendingDeleteId(null);
      }
    },
    [businessId, toast, t, onMutated],
  );

  const formatTime = useMemo(() => {
    const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
    return (iso: string) => fmt.format(new Date(iso));
  }, [locale]);

  return (
    <PremiumPanel
      as="section"
      aria-label={title}
      className="overflow-hidden"
      withTexture
    >
      <div className="flex items-center gap-2 border-b border-warm-200 bg-warm-50/70 p-4">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center gap-1 text-sm font-semibold text-brand transition hover:text-brand-dark"
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          {t("chat.back")}
        </button>
        <span aria-hidden="true" className="text-ink-400">
          /
        </span>
        <h2 className="truncate text-base font-semibold tracking-0 text-ink-950">
          {title}
        </h2>
      </div>

      <div className="p-4">
        {status === "loading" ? (
          <div
            role="status"
            aria-live="polite"
            className="flex min-h-[20vh] items-center justify-center"
          >
            <Spinner aria-label={t("chat.threadLoading")} />
          </div>
        ) : status === "error" ? (
          <p className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {t("chat.error")}
          </p>
        ) : messages.length === 0 ? (
          <EmptyState
            icon={MessageSquare}
            title={t("chat.threadEmptyTitle")}
            subtitle={t("chat.threadEmptySubtitle")}
          />
        ) : (
          <div className="space-y-2">
            {hasMore ? (
              <div className="flex justify-center">
                <Button
                  size="sm"
                  variant="light"
                  isLoading={loadingOlder}
                  onPress={loadOlder}
                >
                  {t("chat.loadOlder")}
                </Button>
              </div>
            ) : null}
            <ul className="space-y-2">
              {messages.map((m) => (
                <li
                  key={m.id}
                  className="group flex items-start justify-between gap-3 rounded-2xl border border-warm-200/90 bg-white/75 p-3 shadow-sm"
                >
                  <div className="min-w-0">
                    <p className="text-[11px] font-semibold text-ink-500">
                      {m.sender_name}
                      <span className="ml-2 font-normal text-ink-500">
                        {formatTime(m.created_at)}
                      </span>
                    </p>
                    <p className="whitespace-pre-wrap break-words text-sm text-ink-950">
                      {m.content}
                    </p>
                  </div>
                  {canModerate ? (
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      color="danger"
                      aria-label={t("chat.delete")}
                      isLoading={deletingId === m.id}
                      isDisabled={deletingId === m.id}
                      onPress={() => setPendingDeleteId(m.id)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          </div>
        )}

        <form
          className="mt-3 flex items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
        >
          <textarea
            value={draft}
            maxLength={CHAT_CONTENT_MAX_LENGTH}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void send();
              }
            }}
            rows={1}
            aria-label={t("chat.title")}
            className="min-h-[44px] flex-1 resize-none rounded-xl border border-warm-200 bg-white px-3 py-2 text-sm text-ink-950 shadow-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand/15"
          />
          <Button
            type="submit"
            className="bg-brand font-semibold text-white shadow-sm hover:bg-brand-dark"
            isLoading={sending}
            isDisabled={sending || draft.trim() === ""}
            endContent={
              sending ? undefined : <SendHorizonal className="h-4 w-4" />
            }
          >
            {sending ? t("chat.sending") : t("chat.send")}
          </Button>
        </form>
      </div>

      <ConfirmationModal
        isOpen={pendingDeleteId !== null}
        onOpenChange={() => setPendingDeleteId(null)}
        isDanger
        title={t("chat.deleteConfirmTitle")}
        description={t("chat.deleteConfirm")}
        confirmLabel={t("chat.deleteConfirmAction")}
        cancelLabel={t("chat.deleteConfirmCancel")}
        onConfirm={() => {
          if (pendingDeleteId !== null) void remove(pendingDeleteId);
        }}
      />
    </PremiumPanel>
  );
}
