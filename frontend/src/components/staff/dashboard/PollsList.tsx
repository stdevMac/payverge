"use client";

import React from "react";
import { Button, Chip } from "@nextui-org/react";
import { BarChart3, Check } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonList } from "@/components/ui/skeletons";

// Presentational polls surface (Slice 9): vote cards. Labels-prop contract;
// house style; shared EmptyState; role="status" loading. An open poll the caller
// hasn't voted in shows option buttons; once voted (or closed) it shows the
// tally bars. Anonymous polls only ever carry `votes` — the view never renders a
// voter list. NO money here.

interface PollOptionView {
  optionId: number;
  label: string;
  votes: number | null; // null = tally not available yet (open, not voted)
}

export interface PollCardView {
  id: number;
  question: string;
  isAnonymous: boolean;
  status: "open" | "closed";
  myVote: number | null;
  options: PollOptionView[];
}

export interface PollsListLabels {
  title: string;
  empty: string;
  emptyHint: string;
  loading: string;
  vote: string;
  voted: string;
  closed: string;
  anonymous: string;
  votesCount: string; // "{n} votes"
}

export interface PollsListProps {
  polls: PollCardView[];
  labels: PollsListLabels;
  loading?: boolean;
  votingPollId: number | null;
  onVote: (pollId: number, optionId: number) => void;
}

export default function PollsList({ polls, labels, loading, votingPollId, onVote }: PollsListProps) {
  if (loading) {
    return <SkeletonList rows={3} ariaLabel={labels.loading} />;
  }

  if (polls.length === 0) {
    return <EmptyState icon={BarChart3} title={labels.empty} subtitle={labels.emptyHint} />;
  }

  return (
    <section aria-label={labels.title} className="space-y-3">
      <h2 className="font-title text-base text-gray-900">{labels.title}</h2>
      <ul className="space-y-3">
        {polls.map((poll) => {
          const closed = poll.status === "closed";
          const voted = poll.myVote != null;
          const showResults = closed || voted;
          const total = poll.options.reduce((sum, o) => sum + (o.votes ?? 0), 0);
          const busy = votingPollId === poll.id;
          return (
            <li key={poll.id} className="rounded-xl border border-gray-200 p-4">
              <div className="mb-3 flex items-start justify-between gap-3">
                <p className="text-sm font-semibold text-gray-900">{poll.question}</p>
                <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">
                  {poll.isAnonymous ? (
                    <Chip size="sm" variant="flat">
                      {labels.anonymous}
                    </Chip>
                  ) : null}
                  {closed ? (
                    <Chip size="sm" variant="flat" color="default">
                      {labels.closed}
                    </Chip>
                  ) : voted ? (
                    <Chip size="sm" variant="flat" color="success" startContent={<Check className="h-3.5 w-3.5" />}>
                      {labels.voted}
                    </Chip>
                  ) : null}
                </div>
              </div>

              {showResults ? (
                <ul className="space-y-2">
                  {poll.options.map((opt) => {
                    const votes = opt.votes ?? 0;
                    const pct = total > 0 ? Math.round((votes / total) * 100) : 0;
                    const isMine = poll.myVote === opt.optionId;
                    return (
                      <li key={opt.optionId}>
                        <div className="mb-1 flex items-center justify-between text-sm">
                          <span className={isMine ? "font-medium text-brand-800" : "text-gray-800"}>
                            {isMine ? <Check className="mr-1 inline h-3.5 w-3.5" aria-hidden="true" /> : null}
                            {opt.label}
                          </span>
                          <span className="text-xs text-gray-500">
                            {labels.votesCount.replace("{n}", String(votes))}
                          </span>
                        </div>
                        <div className="h-2 w-full overflow-hidden rounded-full bg-gray-100">
                          <div
                            className={isMine ? "h-full bg-brand" : "h-full bg-brand-300"}
                            style={{ width: `${pct}%` }}
                          />
                        </div>
                      </li>
                    );
                  })}
                </ul>
              ) : (
                <div className="flex flex-wrap gap-2">
                  {poll.options.map((opt) => (
                    <Button
                      key={opt.optionId}
                      size="sm"
                      variant="bordered"
                      isDisabled={busy}
                      onPress={() => onVote(poll.id, opt.optionId)}
                    >
                      {opt.label}
                    </Button>
                  ))}
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
