"use client";

import React, { useCallback, useMemo, useState } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button, Chip, Input, Spinner, Switch } from "@nextui-org/react";
import { Megaphone, Pencil, Trash2, X } from "lucide-react";
import { chatApi, type Announcement } from "@/api/chat";
import { positionsApi } from "@/api/positions";
import { queryKeys } from "@/api/queryKeys";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { useToast } from "@/contexts/ToastContext";
import { EmptyState } from "@/components/ui/EmptyState";
import { intlLocaleFor } from "@/utils/intlLocale";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import { PremiumPanel } from "../premium";
import { AudienceSelect } from "../engagement/AudienceSelect";

// Operator announcement composer (mounted in the Schedule tab). Posts a broadcast
// (chat:announce — manager/owner) targeting everyone, a role, or a department, and
// — when confirmation is required — surfaces the "X of Y confirmed" summary via a
// SINGLE batch call per refresh for the VISIBLE require_ack notices (replacing the
// old per-announcement 30s AckRoster poll). The full unacked roster loads on
// expand. Announcements are editable + deletable. Money-free.

// Announcements older than this stop being polled for fresh ack counts — an old
// notice's roster is effectively final, so it no longer nags the network.
const ACK_POLL_MAX_AGE_MS = 14 * 24 * 60 * 60 * 1000;

interface AnnouncementComposerProps {
  businessId: string;
}

export default function AnnouncementComposer({ businessId }: AnnouncementComposerProps) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const queryClient = useQueryClient();

  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [requireAck, setRequireAck] = useState(false);
  const [audience, setAudience] = useState("all");
  const [titleError, setTitleError] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Announcement | null>(null);

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardChat.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );
  // The shared audience picker resolves the announce.audience.* namespace.
  const audienceT = useCallback((key: string): string => t(`announce.audience.${key}`), [t]);

  const annKey = queryKeys.chat.announcements(businessId);
  const feedQuery = useQuery({
    queryKey: annKey,
    queryFn: () => chatApi.listAnnouncements(businessId),
  });

  const announcements = useMemo(
    () => feedQuery.data?.announcements ?? [],
    [feedQuery.data],
  );

  // The require_ack notices young enough to still be worth polling for fresh ack
  // counts — bounded (the feed is server-capped) and only the ones that need a
  // live count. This drives the SINGLE batch ack-summary call.
  const pollableAckIds = useMemo(() => {
    const cutoff = Date.now() - ACK_POLL_MAX_AGE_MS;
    return announcements
      .filter((a) => a.require_ack && new Date(a.created_at).getTime() >= cutoff)
      .map((a) => a.id);
  }, [announcements]);

  const historicalAckIds = useMemo(() => {
    const cutoff = Date.now() - ACK_POLL_MAX_AGE_MS;
    return announcements
      .filter((a) => a.require_ack && new Date(a.created_at).getTime() < cutoff)
      .map((a) => a.id);
  }, [announcements]);

  const summariesKey = queryKeys.chat.ackSummaries(businessId, pollableAckIds);
  const summariesQuery = useQuery({
    queryKey: summariesKey,
    queryFn: () => chatApi.announcementAckSummaries(businessId, pollableAckIds),
    enabled: pollableAckIds.length > 0,
    // ONE call per refresh for the visible pollable notices — the whole feed's
    // rosters in a single request instead of one poll per card.
    refetchInterval: 30 * 1000,
  });
  const historicalKey = queryKeys.chat.ackSummaries(businessId, historicalAckIds);
  const historicalQuery = useQuery({
    queryKey: historicalKey,
    queryFn: () => chatApi.announcementAckSummaries(businessId, historicalAckIds),
    enabled: historicalAckIds.length > 0,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchInterval: false,
  });
  const summaries = {
    ...(historicalQuery.data ?? {}),
    ...(summariesQuery.data ?? {}),
  };

  // A colleague's announcement (content-free nudge) refreshes the feed live; the
  // composer's own posts already invalidate on success.
  useStaffRealtime({
    businessId: Number(businessId),
    onChatAnnouncement: () => {
      void queryClient.invalidateQueries({ queryKey: annKey });
    },
    // A staff confirmation lands → refresh the batch summaries so the counts tick
    // up live (also invalidate any open full roster).
    onChatAnnouncementAck: (p) => {
      void queryClient.invalidateQueries({ queryKey: ["chat", businessId, "ackSummaries"] });
      if (p.announcement_id) {
        void queryClient.invalidateQueries({
          queryKey: queryKeys.chat.acks(businessId, p.announcement_id),
        });
      }
    },
    // Announcements/acks posted during an SSE gap were never delivered — resync
    // the feed, batch summaries, and any open full roster.
    onReconnect: () => {
      void queryClient.invalidateQueries({ queryKey: annKey });
      void queryClient.invalidateQueries({ queryKey: ["chat", businessId, "ackSummaries"] });
      void queryClient.invalidateQueries({ queryKey: ["chat", businessId, "acks"] });
    },
  });

  // Distinct, non-empty departments power the "by department" audience options.
  const positionsQuery = useQuery({
    queryKey: ["positions", businessId],
    queryFn: () => positionsApi.list(businessId),
    staleTime: 5 * 60 * 1000,
  });
  const departments = useMemo(() => {
    const set = new Set<string>();
    (positionsQuery.data ?? []).forEach((p) => {
      if (p.department) set.add(p.department);
    });
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [positionsQuery.data]);

  const resetForm = useCallback(() => {
    setTitle("");
    setContent("");
    setRequireAck(false);
    setAudience("all");
    setTitleError(false);
    setEditingId(null);
  }, []);

  const createMutation = useMutation({
    mutationFn: () =>
      chatApi.createAnnouncement(businessId, {
        title: title.trim(),
        content: content.trim(),
        require_ack: requireAck,
        audience_filter: audience,
      }),
    onSuccess: () => {
      toast.showSuccess(t("announce.postSuccess"));
      resetForm();
      void queryClient.invalidateQueries({ queryKey: annKey });
    },
    onError: () => toast.showError(t("announce.postError")),
  });

  const updateMutation = useMutation({
    mutationFn: (id: number) =>
      chatApi.updateAnnouncement(businessId, id, {
        title: title.trim(),
        content: content.trim(),
        require_ack: requireAck,
        audience_filter: audience,
      }),
    onSuccess: () => {
      toast.showSuccess(t("announce.updateSuccess"));
      resetForm();
      void queryClient.invalidateQueries({ queryKey: annKey });
      void queryClient.invalidateQueries({ queryKey: ["chat", businessId, "ackSummaries"] });
    },
    onError: () => toast.showError(t("announce.postError")),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => chatApi.deleteAnnouncement(businessId, id),
    onSuccess: () => {
      toast.showSuccess(t("announce.deleteSuccess"));
      void queryClient.invalidateQueries({ queryKey: annKey });
      void queryClient.invalidateQueries({ queryKey: ["chat", businessId, "ackSummaries"] });
    },
    onError: () => toast.showError(t("announce.deleteError")),
  });

  const submit = useCallback(() => {
    if (title.trim() === "") {
      setTitleError(true);
      return;
    }
    setTitleError(false);
    if (editingId != null) {
      updateMutation.mutate(editingId);
    } else {
      createMutation.mutate();
    }
  }, [title, editingId, createMutation, updateMutation]);

  const beginEdit = useCallback((a: Announcement) => {
    setEditingId(a.id);
    setTitle(a.title);
    setContent(a.content);
    setRequireAck(a.require_ack);
    setAudience(a.audience_filter || "all");
    setTitleError(false);
    if (typeof window !== "undefined") {
      window.scrollTo({ top: 0, behavior: "smooth" });
    }
  }, []);

  const formatDate = useMemo(() => {
    const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
    return (iso: string) => fmt.format(new Date(iso));
  }, [locale]);

  const isSaving = createMutation.isPending || updateMutation.isPending;

  return (
    <PremiumPanel
      as="section"
      aria-label={t("announce.title")}
      className="overflow-hidden"
      withTexture
    >
      <div className="border-b border-warm-200 bg-warm-50/70 p-4">
        <h2 className="text-base font-semibold tracking-0 text-ink-950">
          {editingId != null ? t("announce.editTitle") : t("announce.title")}
        </h2>
        <p className="text-sm text-ink-600">{t("announce.subtitle")}</p>
      </div>

      <form
        className="space-y-4 p-4"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Input
          label={t("announce.titleLabel")}
          placeholder={t("announce.titlePlaceholder")}
          value={title}
          onValueChange={(v) => {
            setTitle(v);
            if (titleError && v.trim() !== "") setTitleError(false);
          }}
          isInvalid={titleError}
          errorMessage={titleError ? t("announce.titleRequired") : undefined}
        />

        <div>
          <label
            htmlFor="announcement-content"
            className="mb-1 block text-sm font-semibold text-ink-700"
          >
            {t("announce.contentLabel")}
          </label>
          <textarea
            id="announcement-content"
            value={content}
            onChange={(e) => setContent(e.target.value)}
            rows={3}
            placeholder={t("announce.contentPlaceholder")}
            className="w-full resize-y rounded-xl border border-warm-200 bg-white px-3 py-2 text-sm text-ink-950 shadow-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand/15"
          />
        </div>

        <div>
          <AudienceSelect
            label={t("announce.audienceLabel")}
            value={audience}
            onChange={setAudience}
            departments={departments}
            t={audienceT}
          />
          {!positionsQuery.isLoading && departments.length === 0 ? (
            <p className="mt-1 text-xs text-ink-500">{t("announce.noDeptsHint")}</p>
          ) : null}
        </div>

        <div className="flex items-center justify-between gap-3 rounded-2xl border border-warm-200 bg-warm-50/70 p-3">
          <div className="min-w-0">
            <p className="text-sm font-semibold text-ink-950">{t("announce.requireAck")}</p>
            <p className="text-xs text-ink-600">{t("announce.requireAckHint")}</p>
          </div>
          <Switch
            isSelected={requireAck}
            onValueChange={setRequireAck}
            aria-label={t("announce.requireAck")}
          />
        </div>

        <div className="flex justify-end gap-2">
          {editingId != null ? (
            <Button
              type="button"
              variant="light"
              onPress={resetForm}
              startContent={<X className="h-4 w-4" />}
            >
              {t("announce.cancelEdit")}
            </Button>
          ) : null}
          <Button
            type="submit"
            className="bg-brand font-semibold text-white shadow-sm hover:bg-brand-dark"
            isLoading={isSaving}
            startContent={isSaving ? undefined : <Megaphone className="h-4 w-4" />}
          >
            {editingId != null
              ? isSaving
                ? t("announce.saving")
                : t("announce.saveEdit")
              : isSaving
                ? t("announce.posting")
                : t("announce.post")}
          </Button>
        </div>
      </form>

      <div className="border-t border-warm-200 p-4">
        <h3 className="mb-3 text-sm font-semibold tracking-0 text-ink-950">
          {t("announce.feedTitle")}
        </h3>
        {feedQuery.isLoading ? (
          <div
            role="status"
            aria-live="polite"
            className="flex min-h-[12vh] items-center justify-center"
          >
            <Spinner aria-label={t("announce.feedLoading")} />
          </div>
        ) : announcements.length === 0 ? (
          <EmptyState
            icon={Megaphone}
            title={t("announce.feedEmpty")}
            subtitle={t("announce.subtitle")}
          />
        ) : (
          <ul className="space-y-3">
            {announcements.map((a) => (
              <li
                key={a.id}
                className="rounded-2xl border border-warm-200/90 bg-white/75 p-3 shadow-sm"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="text-sm font-semibold text-ink-950">{a.title}</p>
                    <p className="text-xs text-ink-500">{formatDate(a.created_at)}</p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {a.require_ack ? (
                      <Chip size="sm" variant="flat" color="warning">
                        {t("announce.requireAckBadge")}
                      </Chip>
                    ) : null}
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      aria-label={t("announce.edit")}
                      onPress={() => beginEdit(a)}
                    >
                      <Pencil className="h-4 w-4 text-ink-500" />
                    </Button>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      aria-label={t("announce.delete")}
                      onPress={() => setPendingDelete(a)}
                    >
                      <Trash2 className="h-4 w-4 text-danger" />
                    </Button>
                  </div>
                </div>
                {a.content ? (
                  <p className="mt-1 whitespace-pre-wrap break-words text-sm text-ink-700">
                    {a.content}
                  </p>
                ) : null}
                {a.require_ack ? (
                  <AckSummaryRow
                    businessId={businessId}
                    announcement={a}
                    summary={summaries[String(a.id)]}
                    historical={historicalAckIds.includes(a.id)}
                    settled={
                      historicalAckIds.includes(a.id)
                        ? historicalQuery.isFetched && !historicalQuery.isFetching
                        : summariesQuery.isFetched && !summariesQuery.isFetching
                    }
                    failed={
                      historicalAckIds.includes(a.id)
                        ? historicalQuery.isError
                        : summariesQuery.isError
                    }
                    t={t}
                  />
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>

      <ConfirmationModal
        isOpen={pendingDelete != null}
        onOpenChange={() => setPendingDelete(null)}
        title={t("announce.deleteConfirmTitle")}
        description={t("announce.deleteConfirmBody")}
        confirmLabel={t("announce.deleteConfirmAction")}
        isDanger
        onConfirm={() => {
          if (pendingDelete) deleteMutation.mutate(pendingDelete.id);
          setPendingDelete(null);
        }}
      />
    </PremiumPanel>
  );
}

// The compact "X of Y confirmed" summary for one announcement, fed by the batch
// summary (no per-card poll). The full unacked roster loads on demand when the
// operator expands it (a single fetch, not a poll).
function AckSummaryRow({
  businessId,
  announcement,
  summary,
  historical,
  settled,
  failed,
  t,
}: {
  businessId: string;
  announcement: Announcement;
  summary?: { acked: number; total_eligible: number; first_acker_names: string[] };
  historical: boolean;
  settled: boolean;
  failed: boolean;
  t: (key: string) => string;
}) {
  const [expanded, setExpanded] = useState(false);
  const rosterQuery = useQuery({
    queryKey: queryKeys.chat.acks(businessId, announcement.id),
    queryFn: () => chatApi.announcementAcks(businessId, announcement.id),
    enabled: expanded,
  });

  if (!summary) {
    if (failed && settled) {
      return (
        <p className="mt-2 text-xs text-ink-500">{t("announce.ackSummaryError")}</p>
      );
    }
    if (historical && settled) {
      return (
        <p className="mt-2 text-xs text-ink-500">
          {t("announce.ackHistoryUnavailable")}
        </p>
      );
    }
    return <p className="mt-2 text-xs text-ink-500">{t("announce.feedLoading")}</p>;
  }

  const { acked, total_eligible, first_acker_names } = summary;
  const allConfirmed = total_eligible > 0 && acked >= total_eligible;
  const names = first_acker_names.join(", ");

  return (
    <div className="mt-2 border-t border-warm-100 pt-2">
      <div className="flex flex-wrap items-center gap-2">
        <Chip size="sm" variant="flat" color={allConfirmed ? "success" : "default"}>
          {t("announce.ackRoster")
            .replace("{acked}", String(acked))
            .replace("{total}", String(total_eligible))}
        </Chip>
        {allConfirmed ? (
          <span className="text-xs text-ink-600">{t("announce.ackAll")}</span>
        ) : names ? (
          <span className="truncate text-xs text-ink-600">
            {t("announce.ackConfirmedBy").replace("{names}", names)}
          </span>
        ) : null}
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
          className="ml-auto inline-flex items-center gap-1 text-xs font-semibold text-brand transition hover:text-brand-dark"
        >
          {expanded ? t("announce.hideRoster") : t("announce.viewRoster")}
        </button>
      </div>
      {expanded ? (
        <div className="mt-2 rounded-xl bg-warm-50/70 p-2 text-xs text-ink-600">
          {rosterQuery.isLoading || !rosterQuery.data ? (
            <span>{t("announce.feedLoading")}</span>
          ) : rosterQuery.data.unacked.length === 0 ? (
            <span>{t("announce.ackAll")}</span>
          ) : (
            <span>
              {t("announce.ackPending").replace(
                "{names}",
                rosterQuery.data.unacked.map((u) => u.name).join(", "),
              )}
            </span>
          )}
        </div>
      ) : null}
    </div>
  );
}
