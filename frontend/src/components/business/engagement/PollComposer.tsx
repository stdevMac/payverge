"use client";

import React, { useCallback, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Chip, Input, Spinner, Switch } from "@nextui-org/react";
import { BarChart3, Plus, X } from "lucide-react";
import { pollsApi, type Poll } from "@/api/engagement";
import { positionsApi } from "@/api/positions";
import { queryKeys } from "@/api/queryKeys";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { EmptyState } from "@/components/ui/EmptyState";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import { AudienceSelect } from "./AudienceSelect";

// Operator poll composer (mounted in the EngagementPanel). Opens a poll
// (poll:manage — manager/owner) with two or more options, an optional anonymous
// flag and close date, targeted at everyone / a role / a department, and lists
// existing polls WITH their live tallies (bars + counts) and a confirm-gated
// Close action. Self-resolves the dashboard.engagement namespace. Money-free.

export default function PollComposer({ businessId }: { businessId: string }) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const qc = useQueryClient();

  const [question, setQuestion] = useState("");
  const [options, setOptions] = useState<string[]>(["", ""]);
  const [anonymous, setAnonymous] = useState(false);
  const [closesAt, setClosesAt] = useState("");
  const [audience, setAudience] = useState("all");
  const [pendingClose, setPendingClose] = useState<Poll | null>(null);

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboard.engagement.polls.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );
  const ta = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboard.engagement.audience.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  const listKey = queryKeys.engagement.polls(businessId);
  const listQuery = useQuery({ queryKey: listKey, queryFn: () => pollsApi.list(businessId) });

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

  const createMutation = useMutation({
    mutationFn: () =>
      pollsApi.create(businessId, {
        question: question.trim(),
        is_anonymous: anonymous,
        closes_at: closesAt ? new Date(closesAt).toISOString() : undefined,
        audience_filter: audience,
        options: options
          .map((label) => label.trim())
          .filter((label) => label !== "")
          .map((label, sort_order) => ({ label, sort_order })),
      }),
    onSuccess: () => {
      toast.showSuccess(t("saved"));
      setQuestion("");
      setOptions(["", ""]);
      setAnonymous(false);
      setClosesAt("");
      setAudience("all");
      void qc.invalidateQueries({ queryKey: listKey });
    },
    onError: () => toast.showError(t("saveError")),
  });

  const closeMutation = useMutation({
    mutationFn: (pollId: number) => pollsApi.close(businessId, pollId),
    onSuccess: (_data, pollId) => {
      void qc.invalidateQueries({ queryKey: listKey });
      void qc.invalidateQueries({ queryKey: queryKeys.engagement.pollResults(businessId, pollId) });
    },
    onError: () => toast.showError(t("saveError")),
  });

  const validOptionCount = options.filter((o) => o.trim() !== "").length;
  const canSubmit = question.trim() !== "" && validOptionCount >= 2;

  const setOption = (idx: number, value: string) =>
    setOptions((cur) => cur.map((o, i) => (i === idx ? value : o)));
  const addOption = () => setOptions((cur) => [...cur, ""]);
  const removeOption = (idx: number) =>
    setOptions((cur) => (cur.length <= 2 ? cur : cur.filter((_, i) => i !== idx)));

  const polls = useMemo(() => listQuery.data ?? [], [listQuery.data]);

  return (
    <section aria-label={t("new")} className="space-y-4">
      <form
        className="space-y-4 rounded-3xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5"
        onSubmit={(e) => {
          e.preventDefault();
          if (canSubmit) createMutation.mutate();
        }}
      >
        <h3 className="text-sm font-semibold text-ink-950">{t("new")}</h3>
        <Input
          label={t("question")}
          placeholder={t("questionPlaceholder")}
          value={question}
          onValueChange={setQuestion}
        />

        <div className="space-y-2">
          {options.map((opt, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <Input
                aria-label={`${t("option")} ${idx + 1}`}
                placeholder={t("optionPlaceholder")}
                value={opt}
                onValueChange={(v) => setOption(idx, v)}
              />
              {options.length > 2 ? (
                <button
                  type="button"
                  aria-label={t("removeOption")}
                  onClick={() => removeOption(idx)}
                  className="rounded-full p-1 text-ink-400 transition hover:bg-warm-100 hover:text-ink-700"
                >
                  <X className="h-4 w-4" />
                </button>
              ) : null}
            </div>
          ))}
          <button
            type="button"
            onClick={addOption}
            className="inline-flex items-center gap-1 rounded-xl px-2 py-1 text-sm font-semibold text-brand-dark transition hover:bg-brand/10"
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t("addOption")}
          </button>
        </div>

        <Input
          type="datetime-local"
          label={t("closesAt")}
          value={closesAt}
          onValueChange={setClosesAt}
        />

        <AudienceSelect
          label={ta("label")}
          value={audience}
          onChange={setAudience}
          departments={departments}
          t={ta}
        />

        <div className="flex items-center justify-between gap-3 rounded-2xl border border-warm-200 bg-warm-50/60 p-3">
          <p className="text-sm font-semibold text-ink-900">{t("anonymous")}</p>
          <Switch
            isSelected={anonymous}
            onValueChange={setAnonymous}
            aria-label={t("anonymous")}
            classNames={{
              wrapper: "group-data-[selected=true]:bg-brand",
            }}
          />
        </div>

        <div className="flex justify-end">
          <Button
            type="submit"
            className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
            isDisabled={!canSubmit || createMutation.isPending}
            isLoading={createMutation.isPending}
            startContent={createMutation.isPending ? undefined : <BarChart3 className="h-4 w-4" />}
          >
            {createMutation.isPending ? t("saving") : t("save")}
          </Button>
        </div>
      </form>

      {listQuery.isLoading ? (
        <div role="status" aria-label={t("empty")} className="flex justify-center py-8">
          <Spinner aria-hidden="true" />
        </div>
      ) : polls.length === 0 ? (
        <EmptyState icon={BarChart3} title={t("empty")} subtitle={t("emptyHint")} />
      ) : (
        <ul className="space-y-2">
          {polls.map((poll) => (
            <li
              key={poll.id}
              className="rounded-2xl border border-warm-200 bg-white p-3 shadow-sm shadow-warm-900/5"
            >
              <div className="flex items-start justify-between gap-3">
                <span className="min-w-0 text-sm font-semibold text-ink-900">
                  {poll.question}
                </span>
                <div className="flex shrink-0 items-center gap-2">
                  <Chip
                    size="sm"
                    variant="flat"
                    className={
                      poll.status === "open"
                        ? "border border-emerald-200 bg-emerald-50 text-emerald-700"
                        : "border border-warm-200 bg-warm-100 text-ink-600"
                    }
                  >
                    {poll.status === "open" ? t("statusOpen") : t("statusClosed")}
                  </Chip>
                  {poll.status === "open" ? (
                    <Button
                      size="sm"
                      variant="light"
                      isLoading={closeMutation.isPending && closeMutation.variables === poll.id}
                      onPress={() => setPendingClose(poll)}
                      className="font-semibold text-ink-700 hover:bg-warm-100"
                    >
                      {t("close")}
                    </Button>
                  ) : null}
                </div>
              </div>
              <PollResultsRow businessId={businessId} poll={poll} t={t} />
            </li>
          ))}
        </ul>
      )}

      <ConfirmationModal
        isOpen={pendingClose != null}
        onOpenChange={() => setPendingClose(null)}
        title={t("closeConfirmTitle")}
        description={t("closeConfirmBody")}
        confirmLabel={t("close")}
        isDanger
        onConfirm={() => {
          if (pendingClose) closeMutation.mutate(pendingClose.id);
          setPendingClose(null);
        }}
      />
    </section>
  );
}

// The live tally for one poll: fetches results and renders per-option bars +
// vote counts. Polls while the poll is OPEN (so the operator watches votes land)
// and reads once when closed. The results endpoint already exists; it was only
// consumed by the staff dashboard before — operators could never see their own
// poll's outcome.
function PollResultsRow({
  businessId,
  poll,
  t,
}: {
  businessId: string;
  poll: Poll;
  t: (key: string) => string;
}) {
  const resultsQuery = useQuery({
    queryKey: queryKeys.engagement.pollResults(businessId, poll.id),
    queryFn: () => pollsApi.results(businessId, poll.id),
    // Live while open; a closed poll's tally is final (no poll).
    refetchInterval: poll.status === "open" ? 15 * 1000 : false,
  });

  if (resultsQuery.isLoading || !resultsQuery.data) {
    return (
      <p className="mt-2 text-xs text-ink-400">{t("resultsLoading")}</p>
    );
  }

  const opts = resultsQuery.data.options;
  const totalVotes = opts.reduce((sum, o) => sum + o.votes, 0);

  return (
    <div className="mt-3 space-y-2">
      {opts.map((o) => {
        const pct = totalVotes > 0 ? Math.round((o.votes / totalVotes) * 100) : 0;
        return (
          <div key={o.option_id}>
            <div className="mb-0.5 flex items-center justify-between gap-2 text-xs">
              <span className="min-w-0 truncate text-ink-700">{o.label}</span>
              <span className="shrink-0 font-medium text-ink-600">
                {t("voteCount").replace("{count}", String(o.votes))}
                {" · "}
                {pct}%
              </span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-warm-100">
              <div
                className="h-full rounded-full bg-brand transition-[width]"
                style={{ width: `${pct}%` }}
                aria-hidden="true"
              />
            </div>
          </div>
        );
      })}
      <p className="text-xs text-ink-400">
        {t("totalVotes").replace("{count}", String(totalVotes))}
      </p>
    </div>
  );
}
