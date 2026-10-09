"use client";

import React, { useCallback, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { logbookApi, type ShiftNoteCategory } from "@/api/logbook";
import { queryKeys } from "@/api/queryKeys";
import { localDateKey } from "@/lib/localDate";
import LogbookView, { type LogbookLabels } from "./LogbookView";

// Data container for the staff "More → Logbook" surface. Mounts only when the
// staff member opens Logbook (StaffMore render-prop), so the feed fetch stays
// dormant until then. Owns the date-scoped list query + the create mutation
// (invalidate-on-success), and builds the localized `labels` object from the
// staffLogbook namespace before handing it to the presentational LogbookView.
// NO money on this wire.

const CATS: ShiftNoteCategory[] = ["sales", "guests", "staffing", "maintenance", "other"];

export default function LogbookContainer({ businessId }: { businessId: string }) {
  const { locale } = useSimpleLocale();
  const qc = useQueryClient();
  const today = useMemo(() => localDateKey(), []);

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffLogbook.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  const labels: LogbookLabels = useMemo(
    () => ({
      title: t("title"),
      subtitle: t("subtitle"),
      composeLabel: t("composeLabel"),
      categoryLabel: t("categoryLabel"),
      contentLabel: t("contentLabel"),
      contentPlaceholder: t("contentPlaceholder"),
      submit: t("submit"),
      emptyTitle: t("emptyTitle"),
      emptySubtitle: t("emptySubtitle"),
      loading: t("loading"),
      categories: CATS.reduce(
        (acc, c) => ({ ...acc, [c]: t(`categories.${c}`) }),
        {} as Record<ShiftNoteCategory, string>,
      ),
    }),
    [t],
  );

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.logbook.list(businessId, today),
    queryFn: () => logbookApi.list(businessId, today),
  });

  const create = useMutation({
    mutationFn: (input: { category: ShiftNoteCategory; content: string }) =>
      logbookApi.create(businessId, { ...input, date: today }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.logbook.list(businessId, today) }),
  });

  return (
    <LogbookView
      notes={data ?? []}
      labels={labels}
      loading={isLoading}
      submitting={create.isPending}
      onSubmit={(input) => create.mutate(input)}
    />
  );
}
