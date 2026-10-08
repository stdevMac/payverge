"use client";

import React from "react";
import { Info } from "lucide-react";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface AiProviderNoticeProps {
  businessId: number;
  className?: string;
}

/**
 * Inline note for AI surfaces whose text generation needs an LLM provider.
 *
 * `ai_enabled` is the plan entitlement; `ai_configured` reports whether the
 * server has LLM_API_KEY / LLM_BASE_URL (or OPENROUTER_API_KEY) wired. When a
 * self-hosted instance has no provider, the data views still work but the
 * generate actions answer 503 `ai_not_configured` — this says so up front.
 * Renders nothing unless the backend explicitly reports `ai_configured=false`.
 */
export function AiProviderNotice({
  businessId,
  className,
}: AiProviderNoticeProps) {
  const { locale } = useSimpleLocale();
  const { aiConfigured, loading } = useBusinessAccess(businessId);
  if (loading || aiConfigured !== false) return null;

  const tr = (key: string) => {
    const r = getTranslation(`common.aiProviderNotice.${key}`, locale);
    return Array.isArray(r) ? r[0] || key : (r as string);
  };

  return (
    <div
      role="note"
      data-testid="ai-provider-notice"
      className={`flex items-start gap-3 rounded-2xl border border-amber-200 bg-amber-50/80 px-4 py-3 text-sm text-amber-900 ${className ?? ""}`}
    >
      <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <div>
        <p className="font-medium">{tr("title")}</p>
        <p className="mt-0.5 text-amber-800">{tr("body")}</p>
      </div>
    </div>
  );
}
