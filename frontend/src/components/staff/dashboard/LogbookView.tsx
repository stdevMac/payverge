"use client";

import React, { useState } from "react";
import { Button, Chip, Select, SelectItem, Textarea } from "@nextui-org/react";
import { ClipboardList } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonList } from "@/components/ui/skeletons";
import type { ShiftNote, ShiftNoteCategory } from "@/api/logbook";

// Presentational logbook surface (Slice 8). Labels-prop contract: every string
// comes from `labels` — no inline copy. House style: rounded-xl/border cards,
// font-title heading, teal accent, shared EmptyState, role="status" loading.
// NO money here — handover notes never carry dollars.

const CATEGORY_ORDER: ShiftNoteCategory[] = ["sales", "guests", "staffing", "maintenance", "other"];

export interface LogbookLabels {
  title: string;
  subtitle: string;
  composeLabel: string;
  categoryLabel: string;
  contentLabel: string;
  contentPlaceholder: string;
  submit: string;
  emptyTitle: string;
  emptySubtitle: string;
  loading: string;
  categories: Record<ShiftNoteCategory, string>; // localized category labels
}

export interface LogbookViewProps {
  notes: ShiftNote[];
  labels: LogbookLabels;
  loading?: boolean;
  submitting?: boolean;
  onSubmit: (input: { category: ShiftNoteCategory; content: string }) => void;
}

function noteTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export default function LogbookView({ notes, labels, loading, submitting, onSubmit }: LogbookViewProps) {
  const [category, setCategory] = useState<ShiftNoteCategory>("sales");
  const [content, setContent] = useState("");

  const handleSubmit = () => {
    const trimmed = content.trim();
    if (!trimmed) return;
    onSubmit({ category, content: trimmed });
    setContent("");
  };

  return (
    <section aria-label={labels.title} className="space-y-4">
      <header>
        <h2 className="font-title text-base text-gray-900">{labels.title}</h2>
        <p className="text-sm text-gray-500">{labels.subtitle}</p>
      </header>

      {/* Compose */}
      <div className="rounded-xl border border-gray-200 p-4">
        <h3 className="mb-3 text-sm font-medium text-gray-900">{labels.composeLabel}</h3>
        <Select
          aria-label={labels.categoryLabel}
          label={labels.categoryLabel}
          selectedKeys={[category]}
          onChange={(e) => setCategory(e.target.value as ShiftNoteCategory)}
          className="mb-3"
        >
          {CATEGORY_ORDER.map((key) => (
            <SelectItem key={key} value={key}>
              {labels.categories[key]}
            </SelectItem>
          ))}
        </Select>
        <Textarea
          aria-label={labels.contentLabel}
          label={labels.contentLabel}
          placeholder={labels.contentPlaceholder}
          value={content}
          onValueChange={setContent}
          minRows={2}
          className="mb-3"
        />
        <Button
          color="primary"
          className="w-full bg-brand-dark"
          onPress={handleSubmit}
          isDisabled={!content.trim()}
          isLoading={submitting}
        >
          {labels.submit}
        </Button>
      </div>

      {/* Feed */}
      {loading ? (
        <SkeletonList rows={3} ariaLabel={labels.loading} />
      ) : notes.length === 0 ? (
        <EmptyState icon={ClipboardList} title={labels.emptyTitle} subtitle={labels.emptySubtitle} />
      ) : (
        <ul className="space-y-3">
          {notes.map((n) => (
            <li key={n.id} className="rounded-xl border border-gray-200 p-4">
              <div className="mb-2 flex items-center justify-between">
                <Chip size="sm" variant="flat" className="bg-brand-50 text-brand">
                  {labels.categories[n.category]}
                </Chip>
                <span className="text-xs text-gray-400">{noteTime(n.created_at)}</span>
              </div>
              <p className="whitespace-pre-wrap text-sm text-gray-900">{n.content}</p>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
