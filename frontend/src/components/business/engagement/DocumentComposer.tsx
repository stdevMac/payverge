"use client";

import React, { useCallback, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Chip, Input, Spinner, Switch, Textarea } from "@nextui-org/react";
import { FileText, Pencil } from "lucide-react";
import { documentsApi, type DocumentRow } from "@/api/engagement";
import { positionsApi } from "@/api/positions";
import { queryKeys } from "@/api/queryKeys";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { EmptyState } from "@/components/ui/EmptyState";
import { AudienceSelect } from "./AudienceSelect";

// Operator document composer (mounted in the EngagementPanel). Publishes a
// versioned policy/handbook (doc:manage — manager/owner) as inline body OR a
// pasted link, optionally requiring acknowledgement, targeted at everyone / a role
// / a department. Editing an existing document publishes a NEW version (the backend
// bumps `version`, re-opening acknowledgement for everyone). Audience uses the
// shared NextUI AudienceSelect. Self-resolves the dashboard.engagement namespace.
// Money-free.

export default function DocumentComposer({ businessId }: { businessId: string }) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const qc = useQueryClient();

  const [editingId, setEditingId] = useState<number | null>(null);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [link, setLink] = useState("");
  const [requireAck, setRequireAck] = useState(false);
  const [audience, setAudience] = useState("all");
  const [titleError, setTitleError] = useState(false);

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboard.engagement.documents.${key}`, locale);
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

  const docsKey = queryKeys.engagement.documents(businessId);
  const listQuery = useQuery({
    queryKey: docsKey,
    queryFn: () => documentsApi.list(businessId),
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

  const docs = useMemo(() => listQuery.data?.documents ?? [], [listQuery.data]);

  const resetForm = useCallback(() => {
    setEditingId(null);
    setTitle("");
    setBody("");
    setLink("");
    setRequireAck(false);
    setAudience("all");
    setTitleError(false);
  }, []);

  const beginEdit = useCallback((doc: DocumentRow) => {
    setEditingId(doc.id);
    setTitle(doc.title);
    setBody(doc.content);
    setLink(doc.url);
    setRequireAck(doc.require_ack);
    setAudience(doc.audience_filter || "all");
    setTitleError(false);
  }, []);

  const createMutation = useMutation({
    mutationFn: () =>
      documentsApi.create(businessId, {
        title: title.trim(),
        content: body.trim() || undefined,
        url: link.trim() || undefined,
        require_ack: requireAck,
        audience_filter: audience,
      }),
    onSuccess: () => {
      toast.showSuccess(t("saved"));
      resetForm();
      void qc.invalidateQueries({ queryKey: docsKey });
    },
    onError: () => toast.showError(t("saveError")),
  });

  const updateMutation = useMutation({
    mutationFn: (docId: number) =>
      documentsApi.update(businessId, docId, {
        title: title.trim(),
        content: body.trim() || undefined,
        url: link.trim() || undefined,
        require_ack: requireAck,
        audience_filter: audience,
      }),
    onSuccess: () => {
      toast.showSuccess(t("updated"));
      resetForm();
      void qc.invalidateQueries({ queryKey: docsKey });
    },
    onError: () => toast.showError(t("updateError")),
  });

  const saving = createMutation.isPending || updateMutation.isPending;

  const submit = useCallback(() => {
    if (title.trim() === "" || (body.trim() === "" && link.trim() === "")) {
      setTitleError(title.trim() === "");
      return;
    }
    setTitleError(false);
    if (editingId != null) updateMutation.mutate(editingId);
    else createMutation.mutate();
  }, [title, body, link, editingId, updateMutation, createMutation]);

  return (
    <section aria-label={editingId != null ? t("editHeading") : t("new")} className="space-y-4">
      <form
        className="space-y-4 rounded-3xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <h3 className="text-sm font-semibold text-ink-950">
          {editingId != null ? t("editHeading") : t("new")}
        </h3>
        <Input
          label={t("docTitle")}
          placeholder={t("titlePlaceholder")}
          value={title}
          onValueChange={(v) => {
            setTitle(v);
            if (titleError && v.trim() !== "") setTitleError(false);
          }}
          isInvalid={titleError}
        />
        <Textarea
          label={t("body")}
          placeholder={t("bodyPlaceholder")}
          value={body}
          onValueChange={setBody}
          minRows={3}
        />
        <Input
          label={t("link")}
          placeholder={t("linkPlaceholder")}
          value={link}
          onValueChange={setLink}
        />
        <AudienceSelect
          label={ta("label")}
          value={audience}
          onChange={setAudience}
          departments={departments}
          t={ta}
        />
        <div className="flex items-center justify-between gap-3 rounded-2xl border border-warm-200 bg-warm-50/60 p-3">
          <p className="text-sm font-semibold text-ink-900">{t("requireAck")}</p>
          <Switch
            isSelected={requireAck}
            onValueChange={setRequireAck}
            aria-label={t("requireAck")}
            classNames={{
              wrapper: "group-data-[selected=true]:bg-brand",
            }}
          />
        </div>
        <div className="flex justify-end gap-2">
          {editingId != null ? (
            <Button
              type="button"
              variant="light"
              onPress={resetForm}
              isDisabled={saving}
              className="font-semibold text-ink-700 hover:bg-warm-100"
            >
              {t("cancel")}
            </Button>
          ) : null}
          <Button
            type="submit"
            className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
            isLoading={saving}
            startContent={saving ? undefined : <FileText className="h-4 w-4" />}
          >
            {editingId != null
              ? saving
                ? t("updating")
                : t("update")
              : saving
                ? t("saving")
                : t("save")}
          </Button>
        </div>
      </form>

      {listQuery.isLoading ? (
        <div role="status" aria-label={t("empty")} className="flex justify-center py-8">
          <Spinner aria-hidden="true" />
        </div>
      ) : docs.length === 0 ? (
        <EmptyState icon={FileText} title={t("empty")} subtitle={t("emptyHint")} />
      ) : (
        <ul className="space-y-2">
          {docs.map((doc) => (
            <li
              key={doc.id}
              className="flex items-center justify-between gap-3 rounded-2xl border border-warm-200 bg-white p-3 shadow-sm shadow-warm-900/5"
            >
              <span className="min-w-0 truncate text-sm font-semibold text-ink-900">{doc.title}</span>
              <div className="flex shrink-0 items-center gap-2">
                <Chip
                  size="sm"
                  variant="flat"
                  className="border border-warm-200 bg-warm-100 text-ink-600"
                >
                  {t("version").replace("{n}", String(doc.version))}
                </Chip>
                <Button
                  size="sm"
                  variant="light"
                  startContent={<Pencil className="h-3.5 w-3.5" />}
                  onPress={() => beginEdit(doc)}
                  className="font-semibold text-ink-700 hover:bg-warm-100"
                >
                  {t("edit")}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
