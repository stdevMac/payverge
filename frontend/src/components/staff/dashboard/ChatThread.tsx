"use client";

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@nextui-org/react";
import { ChevronLeft, MessageSquare, SendHorizonal } from "lucide-react";
import { chatApi, type ChatMessage } from "@/api/chat";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { useToast } from "@/contexts/ToastContext";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonList } from "@/components/ui/skeletons";
import { intlLocaleFor } from "@/utils/intlLocale";
import { CHAT_CONTENT_MAX_LENGTH } from "@/components/business/chat/chatFieldLimits";

// Staff chat thread: a flat, keyset-paginated message list ("load older") plus a
// composer. Fully labels-driven (the container resolves every string) and money
// free — team messaging only, never pay. Refetches on mount and on a `chat.message`
// nudge for THIS channel; POSTs the read marker on view and after sending.

const PAGE = 50; // matches the backend chatMessageListMax keyset window.

export interface ChatThreadLabels {
  back: string;
  loading: string;
  error: string;
  emptyTitle: string;
  emptySubtitle: string;
  loadOlder: string;
  you: string;
  composerPlaceholder: string;
  send: string;
  sending: string;
  sendError: string;
}

export interface ChatThreadProps {
  businessId: string;
  channelId: number;
  /** Acting staff id — own messages render aligned + labelled "You". */
  currentStaffId: number;
  /** Channel display name shown in the thread header. */
  title: string;
  onBack: () => void;
  locale: string;
  labels: ChatThreadLabels;
}

export default function ChatThread({
  businessId,
  channelId,
  currentStaffId,
  title,
  onBack,
  locale,
  labels,
}: ChatThreadProps) {
  const toast = useToast();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [hasMore, setHasMore] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const endRef = useRef<HTMLDivElement | null>(null);

  // Newest-first page → reverse to oldest→newest for the chat transcript, then
  // mark the channel read (best-effort; a failed marker just leaves the badge).
  const loadLatest = useCallback(async () => {
    try {
      const page = await chatApi.listMessages(businessId, channelId, { limit: PAGE });
      setMessages(page.slice().reverse());
      setHasMore(page.length >= PAGE);
      setStatus("ready");
      void chatApi.markRead(businessId, channelId).catch(() => {});
    } catch {
      setStatus("error");
    }
  }, [businessId, channelId]);

  useEffect(() => {
    setStatus("loading");
    void loadLatest();
  }, [loadLatest]);

  // A content-free `chat.message` nudge for THIS channel → refetch the latest page
  // (the event body is intentionally absent, so we read the authorized path).
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
      toast.showError(labels.error);
    } finally {
      setLoadingOlder(false);
    }
  }, [messages, businessId, channelId, loadingOlder, toast, labels.error]);

  const send = useCallback(async () => {
    const content = draft.trim();
    if (!content || sending) return;
    setSending(true);
    try {
      const msg = await chatApi.postMessage(businessId, channelId, content);
      setMessages((prev) => [...prev, msg]);
      setDraft("");
      void chatApi.markRead(businessId, channelId, msg.id).catch(() => {});
    } catch {
      toast.showError(labels.sendError);
    } finally {
      setSending(false);
    }
  }, [draft, sending, businessId, channelId, toast, labels.sendError]);

  // Keep the newest message in view as the transcript grows / loads.
  useEffect(() => {
    if (status === "ready" && !loadingOlder) {
      try {
        endRef.current?.scrollIntoView({ block: "nearest" });
      } catch {
        // scrollIntoView is unimplemented under jsdom — harmless in tests.
      }
    }
  }, [messages.length, status, loadingOlder]);

  const formatTime = useMemo(() => {
    const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
      hour: "numeric",
      minute: "2-digit",
    });
    return (iso: string) => fmt.format(new Date(iso));
  }, [locale]);

  return (
    <section className="flex flex-col" aria-label={title}>
      <div className="mb-3 flex items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center gap-1 text-sm font-medium text-brand transition hover:text-brand-800"
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          {labels.back}
        </button>
        <span aria-hidden="true" className="text-gray-400">
          /
        </span>
        <h2 className="truncate font-title text-base text-gray-900">{title}</h2>
      </div>

      {status === "loading" ? (
        <SkeletonList rows={4} ariaLabel={labels.loading} />
      ) : status === "error" ? (
        <div className="rounded-xl border border-gray-200 p-4">
          <p className="text-sm text-gray-600">{labels.error}</p>
        </div>
      ) : messages.length === 0 ? (
        <div className="rounded-xl border border-gray-200">
          <EmptyState
            icon={MessageSquare}
            title={labels.emptyTitle}
            subtitle={labels.emptySubtitle}
          />
        </div>
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
                {labels.loadOlder}
              </Button>
            </div>
          ) : null}
          <ul className="space-y-2">
            {messages.map((m) => {
              const mine = m.sender_staff_id === currentStaffId;
              return (
                <li
                  key={m.id}
                  className={`flex ${mine ? "justify-end" : "justify-start"}`}
                >
                  <div
                    className={`max-w-[80%] rounded-2xl px-3 py-2 ${
                      mine
                        ? "bg-brand text-white"
                        : "border border-gray-200 bg-white text-gray-900"
                    }`}
                  >
                    <p
                      className={`text-[11px] font-medium ${
                        mine ? "text-brand-50" : "text-gray-500"
                      }`}
                    >
                      {mine ? labels.you : m.sender_name}
                      <span
                        className={`ml-2 font-normal ${
                          mine ? "text-brand-100" : "text-gray-400"
                        }`}
                      >
                        {formatTime(m.created_at)}
                      </span>
                    </p>
                    <p className="whitespace-pre-wrap break-words text-sm">
                      {m.content}
                    </p>
                  </div>
                </li>
              );
            })}
          </ul>
          <div ref={endRef} />
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
            // Enter sends; Shift+Enter inserts a newline.
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void send();
            }
          }}
          rows={1}
          aria-label={labels.composerPlaceholder}
          placeholder={labels.composerPlaceholder}
          className="min-h-[44px] flex-1 resize-none rounded-xl border border-gray-200 px-3 py-2 text-sm focus:border-brand-300 focus:outline-none"
        />
        <Button
          type="submit"
          color="primary"
          isLoading={sending}
          isDisabled={sending || draft.trim() === ""}
          endContent={sending ? undefined : <SendHorizonal className="h-4 w-4" />}
        >
          {sending ? labels.sending : labels.send}
        </Button>
      </form>
    </section>
  );
}
