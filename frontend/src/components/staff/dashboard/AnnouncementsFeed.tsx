"use client";

import React, { useCallback, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Chip } from "@nextui-org/react";
import { BellRing, Check, Megaphone } from "lucide-react";
import { chatApi } from "@/api/chat";
import { queryKeys } from "@/api/queryKeys";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { useToast } from "@/contexts/ToastContext";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonList } from "@/components/ui/skeletons";
import { intlLocaleFor } from "@/utils/intlLocale";

// Staff "More → Announcements" surface. The audience-filtered feed (the server
// scopes it to the caller's role/department) with an idempotent acknowledge action
// when a notice requires confirmation. Self-resolves the staffChat namespace and is
// money-free. The feed carries the caller's server-side ack state (so a confirmed
// notice stays confirmed across reloads); the client-session set below is unioned
// in only for instant feedback between an ack and the next refetch.

export interface AnnouncementsFeedProps {
  businessId: string;
}

export default function AnnouncementsFeed({ businessId }: AnnouncementsFeedProps) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [acked, setAcked] = React.useState<Set<number>>(() => new Set());

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffChat.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const annKey = queryKeys.chat.announcements(businessId);
  const query = useQuery({
    queryKey: annKey,
    queryFn: () => chatApi.listAnnouncements(businessId),
  });

  useStaffRealtime({
    businessId: Number(businessId),
    onChatAnnouncement: () => {
      void queryClient.invalidateQueries({ queryKey: annKey });
    },
    // Announcements posted during an SSE gap were never delivered — resync.
    onReconnect: () => {
      void queryClient.invalidateQueries({ queryKey: annKey });
    },
  });

  const ackMutation = useMutation({
    mutationFn: (id: number) => chatApi.ackAnnouncement(businessId, id),
    onSuccess: (_data, id) => {
      setAcked((prev) => {
        const next = new Set(prev);
        next.add(id);
        return next;
      });
      toast.showSuccess(t("announcements.acknowledged"));
    },
    onError: () => toast.showError(t("announcements.ackError")),
  });

  const formatDate = useMemo(() => {
    const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
    return (iso: string) => fmt.format(new Date(iso));
  }, [locale]);

  const announcements = query.data?.announcements ?? [];
  const serverAcked = query.data?.acked ?? {};

  return (
    <section className="space-y-3" aria-label={t("announcements.title")}>
      <h2 className="font-title text-base text-gray-900">{t("announcements.title")}</h2>

      {query.isLoading ? (
        <SkeletonList rows={3} ariaLabel={t("announcements.loading")} />
      ) : query.isError ? (
        <div className="rounded-xl border border-gray-200 p-4">
          <p className="text-sm text-gray-600">{t("announcements.error")}</p>
        </div>
      ) : announcements.length === 0 ? (
        <div className="rounded-xl border border-gray-200">
          <EmptyState
            icon={Megaphone}
            title={t("announcements.emptyTitle")}
            subtitle={t("announcements.emptySubtitle")}
          />
        </div>
      ) : (
        <ul className="space-y-3">
          {announcements.map((a) => {
            const isAcked = acked.has(a.id) || Boolean(serverAcked[a.id]);
            const busy = ackMutation.isPending && ackMutation.variables === a.id;
            return (
              <li key={a.id} className="rounded-xl border border-gray-200 p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="text-sm font-semibold text-gray-900">{a.title}</p>
                    <p className="text-xs text-gray-400">{formatDate(a.created_at)}</p>
                  </div>
                  {a.require_ack ? (
                    <Chip
                      size="sm"
                      variant="flat"
                      color={isAcked ? "success" : "warning"}
                      startContent={
                        isAcked ? <Check className="h-3.5 w-3.5" /> : <BellRing className="h-3.5 w-3.5" />
                      }
                    >
                      {isAcked ? t("announcements.acknowledged") : t("announcements.requireAck")}
                    </Chip>
                  ) : null}
                </div>
                {a.content ? (
                  <p className="mt-2 whitespace-pre-wrap break-words text-sm text-gray-700">
                    {a.content}
                  </p>
                ) : null}
                {a.require_ack && !isAcked ? (
                  <div className="mt-3">
                    <Button
                      size="sm"
                      color="primary"
                      variant="flat"
                      isLoading={busy}
                      isDisabled={busy}
                      startContent={busy ? undefined : <Check className="h-4 w-4" />}
                      onPress={() => ackMutation.mutate(a.id)}
                    >
                      {busy ? t("announcements.acknowledging") : t("announcements.acknowledge")}
                    </Button>
                  </div>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
