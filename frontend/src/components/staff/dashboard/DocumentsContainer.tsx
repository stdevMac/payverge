"use client";

import React, { useCallback, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { documentsApi } from "@/api/engagement";
import { queryKeys } from "@/api/queryKeys";
import DocumentsList, { type DocumentsListLabels } from "./DocumentsList";

// Data container for the staff "More → Documents" surface. Mounts only when the
// staff member opens Documents (StaffMore render-prop), so the list fetch stays
// dormant until then. Owns the list query + the ack mutation (idempotent
// server-side). The list now carries the caller's per-document ack state (`acked`,
// keyed by doc id, true iff acked at the CURRENT version), so a confirmation stays
// confirmed across reloads; the client-session set below is unioned in only for
// instant feedback between an ack and the next refetch — mirrors AnnouncementsFeed.
// Self-resolves the staffEngagement namespace. NO money.

export default function DocumentsContainer({ businessId }: { businessId: string }) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const qc = useQueryClient();
  const [acked, setAcked] = useState<Set<number>>(() => new Set());

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffEngagement.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  const labels: DocumentsListLabels = useMemo(
    () => ({
      title: t("documents.title"),
      empty: t("documents.empty"),
      emptyHint: t("documents.emptyHint"),
      loading: t("documents.loading"),
      open: t("documents.open"),
      version: t("documents.version"),
      ackRequired: t("documents.ackRequired"),
      acknowledge: t("documents.acknowledge"),
      acknowledged: t("documents.acknowledged"),
    }),
    [t],
  );

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.engagement.documents(businessId),
    queryFn: () => documentsApi.list(businessId),
  });

  const docs = useMemo(() => data?.documents ?? [], [data]);
  const serverAcked = data?.acked;

  // Server ack state (authoritative, survives reload) unioned with the
  // client-session set (instant feedback before the next refetch).
  const ackedIds = useMemo(() => {
    const set = new Set<number>(acked);
    if (serverAcked) {
      for (const doc of docs) if (serverAcked[doc.id]) set.add(doc.id);
    }
    return set;
  }, [acked, docs, serverAcked]);

  const ackMutation = useMutation({
    mutationFn: (docId: number) => documentsApi.ack(businessId, docId),
    onSuccess: (_data, docId) => {
      setAcked((prev) => new Set(prev).add(docId));
      toast.showSuccess(t("documents.acknowledged"));
      void qc.invalidateQueries({ queryKey: queryKeys.engagement.documents(businessId) });
    },
    onError: () => toast.showError(t("documents.ackError")),
  });

  return (
    <DocumentsList
      docs={docs}
      labels={labels}
      loading={isLoading}
      ackedIds={ackedIds}
      ackingId={ackMutation.isPending ? (ackMutation.variables ?? null) : null}
      onAck={(docId) => ackMutation.mutate(docId)}
    />
  );
}
