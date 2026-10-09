"use client";

import React, { useState } from "react";
import { Button, Select, SelectItem, Textarea } from "@nextui-org/react";
import { Award } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonLine } from "@/components/ui/skeletons";
import type { Shoutout, ShoutoutVisibility } from "@/api/engagement";

// Presentational recognition surface (Slice 9): the team shout-out feed plus a
// compose form. Labels-prop contract; house style; shared EmptyState;
// role="status" loading. Recipient + author names are resolved by the parent
// (`nameById`) so this view stays fetch-free and easy to test. NO money here.

const EMOJI_PRESETS = ["🙌", "👏", "🔥", "⭐", "💪", "🎉"];

export interface ShoutoutsFeedLabels {
  title: string;
  empty: string;
  emptyHint: string;
  loading: string;
  send: string;
  recipient: string;
  recipientPlaceholder: string;
  message: string;
  messagePlaceholder: string;
  emoji: string;
  visibility: string;
  team: string;
  private: string;
  submit: string;
  sending: string;
}

export interface ShoutoutTeammate {
  id: number;
  name: string;
}

export interface ShoutoutsFeedProps {
  shoutouts: Shoutout[];
  teammates: ShoutoutTeammate[];
  nameById: (id: number) => string;
  labels: ShoutoutsFeedLabels;
  loading?: boolean;
  sending?: boolean;
  onSend: (input: {
    to_staff_id: number;
    message: string;
    emoji?: string;
    visibility: ShoutoutVisibility;
  }) => void;
}

export default function ShoutoutsFeed({
  shoutouts,
  teammates,
  nameById,
  labels,
  loading,
  sending,
  onSend,
}: ShoutoutsFeedProps) {
  const [toStaffId, setToStaffId] = useState("");
  const [message, setMessage] = useState("");
  const [emoji, setEmoji] = useState("");
  const [visibility, setVisibility] = useState<ShoutoutVisibility>("team");

  const submit = () => {
    const to = Number(toStaffId);
    const trimmed = message.trim();
    if (!to || !trimmed) return;
    onSend({
      to_staff_id: to,
      message: trimmed,
      emoji: emoji || undefined,
      visibility,
    });
    setMessage("");
    setEmoji("");
  };

  return (
    <section aria-label={labels.title} className="space-y-4">
      <h2 className="font-title text-base text-ink-900">{labels.title}</h2>

      {/* Compose */}
      <form
        // flex gap, not space-y: sibling margin-top would override the
        // NextUI outside-label headroom and clip the labels.
        className="flex flex-col gap-3 rounded-xl border border-warm-200 p-4"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <h3 className="text-sm font-medium text-ink-900">{labels.send}</h3>

        <Select
          label={labels.recipient}
          placeholder={labels.recipientPlaceholder}
          selectedKeys={toStaffId ? [toStaffId] : []}
          onChange={(e) => setToStaffId(e.target.value)}
          labelPlacement="outside"
          variant="bordered"
          radius="lg"
          classNames={{ label: "text-sm font-medium text-ink-700" }}
        >
          {teammates.map((m) => (
            <SelectItem key={String(m.id)} textValue={m.name}>
              {m.name}
            </SelectItem>
          ))}
        </Select>

        <Textarea
          aria-label={labels.message}
          label={labels.message}
          placeholder={labels.messagePlaceholder}
          value={message}
          onValueChange={setMessage}
          minRows={2}
          labelPlacement="outside"
          variant="bordered"
          radius="lg"
          classNames={{ label: "text-sm font-medium text-ink-700" }}
        />

        <div>
          <span className="mb-1 block text-sm font-medium text-ink-700">
            {labels.emoji}
          </span>
          <div className="flex flex-wrap gap-2">
            {EMOJI_PRESETS.map((e) => (
              <button
                key={e}
                type="button"
                aria-pressed={emoji === e}
                onClick={() => setEmoji((cur) => (cur === e ? "" : e))}
                className={`flex h-9 w-9 items-center justify-center rounded-full border text-lg transition ${
                  emoji === e
                    ? "border-brand bg-brand-50"
                    : "border-gray-200 hover:border-brand-300"
                }`}
              >
                {e}
              </button>
            ))}
          </div>
        </div>

        <Select
          label={labels.visibility}
          selectedKeys={[visibility]}
          onChange={(e) =>
            e.target.value &&
            setVisibility(e.target.value as ShoutoutVisibility)
          }
          labelPlacement="outside"
          variant="bordered"
          radius="lg"
          classNames={{ label: "text-sm font-medium text-ink-700" }}
        >
          <SelectItem key="team" textValue={labels.team}>
            {labels.team}
          </SelectItem>
          <SelectItem key="private" textValue={labels.private}>
            {labels.private}
          </SelectItem>
        </Select>

        <div className="flex justify-end">
          <Button
            type="submit"
            color="primary"
            isLoading={sending}
            isDisabled={!toStaffId || message.trim() === ""}
          >
            {sending ? labels.sending : labels.submit}
          </Button>
        </div>
      </form>

      {/* Feed */}
      {loading ? (
        <div
          role="status"
          aria-live="polite"
          aria-label={labels.loading}
          className="space-y-3"
        >
          {[0, 1].map((i) => (
            <div
              key={i}
              className="rounded-xl border border-warm-200 p-4 motion-safe:animate-pulse"
            >
              <SkeletonLine width="70%" height="0.875rem" />
              <SkeletonLine width="30%" height="0.75rem" className="mt-2" />
            </div>
          ))}
        </div>
      ) : shoutouts.length === 0 ? (
        <EmptyState
          icon={Award}
          title={labels.empty}
          subtitle={labels.emptyHint}
        />
      ) : (
        <ul className="space-y-3">
          {shoutouts.map((s) => (
            <li key={s.id} className="rounded-xl border border-gray-200 p-4">
              <p className="text-sm text-gray-900">
                {s.emoji ? <span className="mr-1.5">{s.emoji}</span> : null}
                {s.message}
              </p>
              <p className="mt-1 text-xs text-gray-400">
                {nameById(s.from_staff_id)} → {nameById(s.to_staff_id)}
              </p>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
