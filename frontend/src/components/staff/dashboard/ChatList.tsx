"use client";

import React, { useCallback, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronRight,
  Hash,
  MessageSquare,
  MessagesSquare,
} from "lucide-react";
import { chatApi, type ChatChannel } from "@/api/chat";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonList } from "@/components/ui/skeletons";
import ChatThread, { type ChatThreadLabels } from "./ChatThread";

// Staff chat surface (the bottom-nav "Chat" tab). Lists the caller's channels —
// their role channel + one per department they staff + manual DMs — with an unread
// dot, and opens a ChatThread on tap. Self-resolves the staffChat namespace (like
// the operator surfaces) and is entirely money-free. The unread badge is sourced
// from the server `unread` map on the channels response; a content-free
// `chat.message` nudge just refetches it, and opening a channel optimistically
// clears its dot (ChatThread POSTs the read marker, so the next fetch reads 0).

export interface ChatListProps {
  businessId: string;
  /** Acting staff id — forwarded to ChatThread for own-message alignment. */
  currentStaffId: number;
}

export default function ChatList({
  businessId,
  currentStaffId,
}: ChatListProps) {
  const { locale } = useSimpleLocale();
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<ChatChannel | null>(null);
  // Channels the caller opened this session — optimistically cleared until the
  // next channels fetch reflects the server-side read marker as 0 unread.
  const [cleared, setCleared] = useState<Set<number>>(() => new Set());
  // Read in the SSE callback without resubscribing on every selection change.
  const selectedRef = useRef<ChatChannel | null>(null);
  selectedRef.current = selected;

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffChat.${key}`, locale);
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

  // A `chat.message` nudge refetches the channels (re-reads the server unread map);
  // for a channel I'm not viewing, un-clear it so a fresh message can re-badge.
  useStaffRealtime({
    businessId: Number(businessId),
    onChatMessage: (p) => {
      const cid = p.channel_id;
      if (cid && selectedRef.current?.id !== cid) {
        setCleared((prev) => {
          if (!prev.has(cid)) return prev;
          const next = new Set(prev);
          next.delete(cid);
          return next;
        });
      }
      void queryClient.invalidateQueries({ queryKey: channelsKey });
    },
    // Nudges dropped during an SSE gap are gone — re-read the unread map.
    onReconnect: () => {
      void queryClient.invalidateQueries({ queryKey: channelsKey });
    },
  });

  const open = useCallback(
    (ch: ChatChannel) => {
      setSelected(ch);
      setCleared((prev) => {
        const next = new Set(prev);
        next.add(ch.id);
        return next;
      });
      // ChatThread POSTs the read marker; refetch so the badge reconciles to 0.
      void queryClient.invalidateQueries({ queryKey: channelsKey });
    },
    [queryClient, channelsKey],
  );

  const channelName = useCallback(
    (ch: ChatChannel): string => {
      if (ch.type === "role") {
        if (ch.ref_key.startsWith("role:")) {
          return t(`list.roles.${ch.ref_key.slice(5)}`) || ch.name;
        }
        if (ch.ref_key.startsWith("dept:")) {
          return ch.name || ch.ref_key.slice(5);
        }
      }
      if (ch.type === "direct") return ch.name || t("list.fallback.direct");
      if (ch.type === "group") return ch.name || t("list.fallback.group");
      return ch.name || t("list.fallback.channel");
    },
    [t],
  );

  const channelKind = useCallback(
    (ch: ChatChannel): string => {
      if (ch.type === "role") {
        return ch.ref_key.startsWith("dept:")
          ? t("list.kind.dept")
          : t("list.kind.role");
      }
      if (ch.type === "direct") return t("list.kind.direct");
      if (ch.type === "group") return t("list.kind.group");
      return t("list.kind.role");
    },
    [t],
  );

  const threadLabels = useMemo<ChatThreadLabels>(
    () => ({
      back: t("thread.back"),
      loading: t("thread.loading"),
      error: t("thread.error"),
      emptyTitle: t("thread.emptyTitle"),
      emptySubtitle: t("thread.emptySubtitle"),
      loadOlder: t("thread.loadOlder"),
      you: t("thread.you"),
      composerPlaceholder: t("thread.composerPlaceholder"),
      send: t("thread.send"),
      sending: t("thread.sending"),
      sendError: t("thread.sendError"),
    }),
    [t],
  );

  if (selected) {
    return (
      <ChatThread
        businessId={businessId}
        channelId={selected.id}
        currentStaffId={currentStaffId}
        title={channelName(selected)}
        onBack={() => setSelected(null)}
        locale={locale}
        labels={threadLabels}
      />
    );
  }

  const channels = channelsQuery.data?.channels ?? [];
  const unreadMap = channelsQuery.data?.unread ?? {};
  const isUnread = (id: number) =>
    !cleared.has(id) && (unreadMap[String(id)] ?? 0) > 0;

  return (
    <section className="space-y-3" aria-label={t("tab.title")}>
      <header>
        <h2 className="font-title text-base text-gray-900">{t("tab.title")}</h2>
        <p className="text-sm text-gray-500">{t("tab.subtitle")}</p>
      </header>

      {channelsQuery.isLoading ? (
        <SkeletonList rows={4} ariaLabel={t("list.loading")} />
      ) : channelsQuery.isError ? (
        <div className="rounded-xl border border-gray-200 p-4">
          <p className="text-sm text-gray-600">{t("list.error")}</p>
        </div>
      ) : channels.length === 0 ? (
        <div className="rounded-xl border border-gray-200">
          <EmptyState
            icon={MessagesSquare}
            title={t("list.emptyTitle")}
            subtitle={t("list.emptySubtitle")}
          />
        </div>
      ) : (
        <ul className="space-y-2">
          {channels.map((ch) => {
            const isDirect = ch.type === "direct" || ch.type === "group";
            const Icon = isDirect ? MessageSquare : Hash;
            const hasUnread = isUnread(ch.id);
            return (
              <li key={ch.id}>
                <button
                  type="button"
                  onClick={() => open(ch)}
                  className="flex w-full items-center gap-3 rounded-xl border border-gray-200 p-4 text-left transition hover:border-brand-300"
                >
                  <span
                    className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand-50 text-brand"
                    aria-hidden="true"
                  >
                    <Icon className="h-5 w-5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium text-gray-900">
                      {channelName(ch)}
                    </span>
                    <span className="block truncate text-xs text-gray-500">
                      {channelKind(ch)}
                    </span>
                  </span>
                  {hasUnread ? (
                    <span
                      className="h-2.5 w-2.5 shrink-0 rounded-full bg-brand"
                      aria-label={t("list.unread")}
                    />
                  ) : null}
                  <ChevronRight
                    className="h-4 w-4 shrink-0 text-gray-400"
                    aria-hidden="true"
                  />
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
