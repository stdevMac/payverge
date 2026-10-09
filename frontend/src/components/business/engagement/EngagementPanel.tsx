"use client";

import React, { useCallback, useState } from "react";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import SegmentedTabs from "../shared/SegmentedTabs";
import ChecklistTemplateComposer from "./ChecklistTemplateComposer";
import DocumentComposer from "./DocumentComposer";
import PollComposer from "./PollComposer";

// Operator engagement authoring (Slice 9), mounted in the staff-ops Schedule tab
// next to the announcement composer. A simple tab strip hosts the three composers;
// only the active tab's composer mounts, so list queries stay lazy. Self-resolves
// the dashboard.engagement namespace. Money-free — engagement carries no dollars.

type EngagementTab = "checklists" | "documents" | "polls";

const TABS: EngagementTab[] = ["checklists", "documents", "polls"];

export default function EngagementPanel({ businessId }: { businessId: string }) {
  const { locale } = useSimpleLocale();
  const [tab, setTab] = useState<EngagementTab>("checklists");

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboard.engagement.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  return (
    <section
      aria-label={t("title")}
      className="overflow-hidden rounded-3xl border border-warm-200 bg-warm-50/60 shadow-sm shadow-warm-900/5"
    >
      <div className="border-b border-warm-200/80 bg-white/80 p-4">
        <h2 className="text-sm font-semibold text-ink-900">{t("title")}</h2>
        <p className="text-sm text-ink-500">{t("subtitle")}</p>
      </div>

      <div className="border-b border-warm-200/80 bg-white/70 px-4 py-3">
        <SegmentedTabs
          size="sm"
          ariaLabel={t("title")}
          activeKey={tab}
          onChange={(key) => setTab(key as EngagementTab)}
          tabs={TABS.map((key) => ({ key, label: t(`tabs.${key}`) }))}
        />
      </div>

      <div className="p-4">
        {tab === "checklists" ? <ChecklistTemplateComposer businessId={businessId} /> : null}
        {tab === "documents" ? <DocumentComposer businessId={businessId} /> : null}
        {tab === "polls" ? <PollComposer businessId={businessId} /> : null}
      </div>
    </section>
  );
}
