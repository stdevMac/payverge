"use client";

import React, { useCallback, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { recognitionApi, type ShoutoutVisibility } from "@/api/engagement";
import { getBusinessStaff } from "@/api/staff";
import { queryKeys } from "@/api/queryKeys";
import ShoutoutsFeed, { type ShoutoutsFeedLabels, type ShoutoutTeammate } from "./ShoutoutsFeed";

// Data container for the staff "More → Recognition" surface. Mounts only when the
// staff member opens Recognition (StaffMore render-prop). Owns the feed query,
// the (all-staff) roster query used to resolve author/recipient names + populate
// the recipient picker (self excluded), and the send mutation. Self-resolves the
// staffEngagement.recognition namespace. NO money on this wire — the roster shape
// (id/name/email/role) carries no pay.

export default function ShoutoutsContainer({
  businessId,
  currentStaffId,
}: {
  businessId: string;
  currentStaffId: number;
}) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const qc = useQueryClient();

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffEngagement.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  const labels: ShoutoutsFeedLabels = useMemo(
    () => ({
      title: t("recognition.title"),
      empty: t("recognition.empty"),
      emptyHint: t("recognition.emptyHint"),
      loading: t("recognition.loading"),
      send: t("recognition.send"),
      recipient: t("recognition.recipient"),
      recipientPlaceholder: t("recognition.recipientPlaceholder"),
      message: t("recognition.message"),
      messagePlaceholder: t("recognition.messagePlaceholder"),
      emoji: t("recognition.emoji"),
      visibility: t("recognition.visibility"),
      team: t("recognition.team"),
      private: t("recognition.private"),
      submit: t("recognition.submit"),
      sending: t("recognition.sending"),
    }),
    [t],
  );

  const feedKey = queryKeys.engagement.shoutouts(businessId);
  const feedQuery = useQuery({
    queryKey: feedKey,
    queryFn: () => recognitionApi.list(businessId),
  });

  const rosterQuery = useQuery({
    queryKey: queryKeys.staff.list(businessId),
    queryFn: () => getBusinessStaff(businessId),
    staleTime: 5 * 60 * 1000,
  });

  const nameMap = useMemo(() => {
    const map = new Map<number, string>();
    (rosterQuery.data?.staff ?? []).forEach((m) => map.set(m.id, m.name));
    return map;
  }, [rosterQuery.data]);

  const teammates: ShoutoutTeammate[] = useMemo(
    () =>
      (rosterQuery.data?.staff ?? [])
        .filter((m) => m.id !== currentStaffId)
        .map((m) => ({ id: m.id, name: m.name })),
    [rosterQuery.data, currentStaffId],
  );

  const nameById = useCallback((id: number) => nameMap.get(id) ?? "—", [nameMap]);

  const sendMutation = useMutation({
    mutationFn: (input: {
      to_staff_id: number;
      message: string;
      emoji?: string;
      visibility: ShoutoutVisibility;
    }) => recognitionApi.send(businessId, input),
    onSuccess: () => {
      toast.showSuccess(t("recognition.sent"));
      void qc.invalidateQueries({ queryKey: feedKey });
    },
    onError: () => toast.showError(t("recognition.sendError")),
  });

  return (
    <ShoutoutsFeed
      shoutouts={feedQuery.data ?? []}
      teammates={teammates}
      nameById={nameById}
      labels={labels}
      loading={feedQuery.isLoading}
      sending={sendMutation.isPending}
      onSend={(input) => sendMutation.mutate(input)}
    />
  );
}
