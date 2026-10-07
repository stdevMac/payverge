"use client";

import React, { useCallback, useMemo } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { pollsApi, type PollResultView } from "@/api/engagement";
import { queryKeys } from "@/api/queryKeys";
import PollsList, { type PollCardView, type PollsListLabels } from "./PollsList";

// Data container for the staff "More → Polls" surface. Mounts only when the staff
// member opens Polls (StaffMore render-prop). Owns the list query; results are
// fetched only for polls the caller has already voted in OR that are closed (the
// tally surfaces there). A successful vote writes the inline result view straight
// into the results cache, so the bars appear without a round-trip. Self-resolves
// the staffEngagement.polls namespace. NO money on this wire.

export default function PollsContainer({ businessId }: { businessId: string }) {
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

  const labels: PollsListLabels = useMemo(
    () => ({
      title: t("polls.title"),
      empty: t("polls.empty"),
      emptyHint: t("polls.emptyHint"),
      loading: t("polls.loading"),
      vote: t("polls.vote"),
      voted: t("polls.voted"),
      closed: t("polls.closed"),
      anonymous: t("polls.anonymous"),
      votesCount: t("polls.votesCount"),
    }),
    [t],
  );

  const listKey = queryKeys.engagement.polls(businessId);
  const listQuery = useQuery({ queryKey: listKey, queryFn: () => pollsApi.list(businessId) });

  const polls = useMemo(() => listQuery.data ?? [], [listQuery.data]);

  // Results only matter where the tally is shown: voted-in or closed polls.
  const resultPollIds = useMemo(
    () => polls.filter((p) => p.status === "closed" || p.my_vote != null).map((p) => p.id),
    [polls],
  );

  const resultsQueries = useQueries({
    queries: resultPollIds.map((pollId) => ({
      queryKey: queryKeys.engagement.pollResults(businessId, pollId),
      queryFn: () => pollsApi.results(businessId, pollId),
      staleTime: 30 * 1000,
    })),
  });

  const resultsByPoll = useMemo(() => {
    const map = new Map<number, PollResultView>();
    resultsQueries.forEach((q, i) => {
      if (q.data) map.set(resultPollIds[i], q.data);
    });
    return map;
  }, [resultsQueries, resultPollIds]);

  const voteMutation = useMutation({
    mutationFn: (vars: { pollId: number; optionId: number }) =>
      pollsApi.vote(businessId, vars.pollId, vars.optionId),
    onSuccess: (view, vars) => {
      qc.setQueryData(queryKeys.engagement.pollResults(businessId, vars.pollId), view);
      toast.showSuccess(t("polls.voted"));
      void qc.invalidateQueries({ queryKey: listKey });
    },
    onError: () => toast.showError(t("polls.voteError")),
  });

  const cards: PollCardView[] = useMemo(
    () =>
      polls.map((p) => {
        const res = resultsByPoll.get(p.id);
        const votesByOption = new Map<number, number>();
        res?.options.forEach((o) => votesByOption.set(o.option_id, o.votes));
        return {
          id: p.id,
          question: p.question,
          isAnonymous: p.is_anonymous,
          status: p.status,
          myVote: p.my_vote,
          options: p.options.map((o) => ({
            optionId: o.id,
            label: o.label,
            votes: votesByOption.has(o.id) ? (votesByOption.get(o.id) ?? 0) : null,
          })),
        };
      }),
    [polls, resultsByPoll],
  );

  return (
    <PollsList
      polls={cards}
      labels={labels}
      loading={listQuery.isLoading}
      votingPollId={voteMutation.isPending ? (voteMutation.variables?.pollId ?? null) : null}
      onVote={(pollId, optionId) => voteMutation.mutate({ pollId, optionId })}
    />
  );
}
