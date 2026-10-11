"use client";

import React, { useEffect, useState } from "react";
import { Button } from "@nextui-org/react";
import { axiosInstance } from "@/api/tools/instance";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { trackEvent } from "@/utils/analytics";

interface SetupStatus {
  required_done: boolean;
  completed_count: number;
  total_count: number;
}

interface OpsAssistantProactiveNudgeProps {
  businessId: number;
  activeTab: string;
  onOpenAssistant: () => void;
}

export default function OpsAssistantProactiveNudge({
  businessId,
  activeTab,
  onOpenAssistant,
}: OpsAssistantProactiveNudgeProps) {
  const { locale } = useSimpleLocale();
  const t = (k: string, params?: Record<string, string>) =>
    String(getTranslation(`opsAssistant.${k}`, locale, params));
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    if (activeTab !== "overview") return;
    axiosInstance
      .get<SetupStatus>(`/inside/businesses/${businessId}/setup-status`)
      .then(({ data }) => setStatus(data))
      .catch(() => setStatus(null));
  }, [businessId, activeTab]);

  if (activeTab !== "overview" || dismissed || !status || status.required_done) return null;

  return (
    <div className="mb-4 rounded-xl border border-brand/20 bg-brand/5 px-4 py-3 flex flex-wrap items-center justify-between gap-3">
      <p className="text-sm text-ink-700">
        {t("nudge.setupIncomplete", {
          completed: String(status.completed_count),
          total: String(status.total_count),
        })}
      </p>
      <Button
        size="sm"
        color="primary"
        variant="flat"
        onPress={() => {
          trackEvent("ops_assistant_nudge_clicked", { tab: activeTab });
          onOpenAssistant();
        }}
      >
        {t("nudge.cta")}
      </Button>
      <button type="button" className="text-xs text-ink-500 underline" onClick={() => setDismissed(true)}>
        {t("nudge.dismiss")}
      </button>
    </div>
  );
}
