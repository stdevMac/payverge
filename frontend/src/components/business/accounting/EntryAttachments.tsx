"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Paperclip, Trash2, Download } from "lucide-react";
import {
  accountingApi,
  type LedgerAttachment,
} from "@/api/accounting";
import {
  isPeriodLockedError,
  periodLockedMessage,
} from "./periodLockErrors";
import { accountingSecondaryButtonClass } from "./accountingShared";

export default function EntryAttachments({
  businessId,
  entryId,
  canWrite,
  t,
}: {
  businessId: string;
  entryId: number;
  canWrite: boolean;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const [items, setItems] = useState<LedgerAttachment[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);

  const load = useCallback(async () => {
    try {
      const rows = await accountingApi.listEntryAttachments(businessId, entryId);
      setItems(rows);
      setError(null);
    } catch (err) {
      setError(
        isPeriodLockedError(err)
          ? periodLockedMessage(err, t)
          : t("attachments.loadError"),
      );
    }
  }, [businessId, entryId, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const onUpload = async (file: File | null) => {
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      await accountingApi.uploadEntryAttachment(businessId, entryId, file);
      await load();
    } catch (err) {
      setError(
        isPeriodLockedError(err)
          ? periodLockedMessage(err, t)
          : t("attachments.uploadError"),
      );
    } finally {
      setUploading(false);
    }
  };

  const onDelete = async (attachmentId: number) => {
    try {
      await accountingApi.deleteEntryAttachment(businessId, entryId, attachmentId);
      await load();
    } catch (err) {
      setError(
        isPeriodLockedError(err)
          ? periodLockedMessage(err, t)
          : t("attachments.deleteError"),
      );
    }
  };

  return (
    <section className="space-y-2" data-testid="entry-attachments">
      <div className="flex items-center justify-between gap-2">
        <h4 className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-ink-500">
          <Paperclip className="h-3.5 w-3.5" aria-hidden />
          {t("attachments.title")}
        </h4>
        {canWrite ? (
          <label className={`${accountingSecondaryButtonClass} cursor-pointer`}>
            <input
              type="file"
              className="sr-only"
              accept=".pdf,.png,.jpg,.jpeg,.webp,.heic,application/pdf,image/*"
              disabled={uploading}
              onChange={(e) => {
                const f = e.target.files?.[0] ?? null;
                void onUpload(f);
                e.target.value = "";
              }}
            />
            {uploading ? t("attachments.uploading") : t("attachments.upload")}
          </label>
        ) : null}
      </div>
      {error ? (
        <p role="alert" className="text-sm text-rose-700">
          {error}
        </p>
      ) : null}
      {items.length === 0 ? (
        <p className="text-sm text-ink-500">{t("attachments.empty")}</p>
      ) : (
        <ul className="space-y-1.5">
          {items.map((a) => (
            <li
              key={a.id}
              className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-warm-200 bg-white px-2.5 py-2 text-sm"
              data-testid={`attachment-${a.id}`}
            >
              <span className="min-w-0 truncate font-medium text-ink-900">
                {a.file_name}
              </span>
              <div className="flex items-center gap-2">
                <a
                  href={accountingApi.downloadEntryAttachmentUrl(
                    businessId,
                    entryId,
                    a.id,
                  )}
                  className="inline-flex items-center gap-1 text-xs font-medium text-brand-700"
                  target="_blank"
                  rel="noreferrer"
                >
                  <Download className="h-3.5 w-3.5" aria-hidden />
                  {t("attachments.download")}
                </a>
                {canWrite ? (
                  <button
                    type="button"
                    className="inline-flex items-center gap-1 text-xs text-rose-700"
                    onClick={() => void onDelete(a.id)}
                  >
                    <Trash2 className="h-3.5 w-3.5" aria-hidden />
                    {t("attachments.delete")}
                  </button>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
