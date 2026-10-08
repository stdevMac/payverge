"use client";

import React from "react";
import { Button, Chip } from "@nextui-org/react";
import { Check, ExternalLink, FileText } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonList } from "@/components/ui/skeletons";
import type { DocumentRow } from "@/api/engagement";

// Presentational documents surface (Slice 9). Labels-prop contract; house style;
// shared EmptyState; role="status" loading. Each row shows the title, a version
// chip, an inline body or an "Open" link, and — when acknowledgement is required —
// an "I have read this" button that flips to "Acknowledged". NO money here.

export interface DocumentsListLabels {
  title: string;
  empty: string;
  emptyHint: string;
  loading: string;
  open: string;
  version: string; // "Version {n}"
  ackRequired: string;
  acknowledge: string;
  acknowledged: string;
}

export interface DocumentsListProps {
  docs: DocumentRow[];
  labels: DocumentsListLabels;
  loading?: boolean;
  ackedIds: Set<number>;
  ackingId: number | null;
  onAck: (docId: number) => void;
}

export default function DocumentsList({
  docs,
  labels,
  loading,
  ackedIds,
  ackingId,
  onAck,
}: DocumentsListProps) {
  if (loading) {
    return <SkeletonList rows={3} ariaLabel={labels.loading} />;
  }

  if (docs.length === 0) {
    return <EmptyState icon={FileText} title={labels.empty} subtitle={labels.emptyHint} />;
  }

  return (
    <section aria-label={labels.title} className="space-y-3">
      <h2 className="font-title text-base text-gray-900">{labels.title}</h2>
      <ul className="space-y-3">
        {docs.map((doc) => {
          const isAcked = ackedIds.has(doc.id);
          const busy = ackingId === doc.id;
          return (
            <li key={doc.id} className="rounded-xl border border-gray-200 p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-gray-900">{doc.title}</p>
                  <p className="text-xs text-gray-400">
                    {labels.version.replace("{n}", String(doc.version))}
                  </p>
                </div>
                {doc.require_ack ? (
                  <Chip
                    size="sm"
                    variant="flat"
                    color={isAcked ? "success" : "warning"}
                    startContent={isAcked ? <Check className="h-3.5 w-3.5" /> : undefined}
                  >
                    {isAcked ? labels.acknowledged : labels.ackRequired}
                  </Chip>
                ) : null}
              </div>

              {doc.content ? (
                <p className="mt-2 whitespace-pre-wrap break-words text-sm text-gray-700">
                  {doc.content}
                </p>
              ) : null}

              {doc.url ? (
                <a
                  href={doc.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="mt-2 inline-flex items-center gap-1 text-sm font-medium text-brand transition hover:text-brand-800"
                >
                  <ExternalLink className="h-4 w-4" aria-hidden="true" />
                  {labels.open}
                </a>
              ) : null}

              {doc.require_ack && !isAcked ? (
                <div className="mt-3">
                  <Button
                    size="sm"
                    color="primary"
                    variant="flat"
                    isLoading={busy}
                    isDisabled={busy}
                    startContent={busy ? undefined : <Check className="h-4 w-4" />}
                    onPress={() => onAck(doc.id)}
                  >
                    {labels.acknowledge}
                  </Button>
                </div>
              ) : null}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
