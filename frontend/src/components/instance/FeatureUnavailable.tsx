"use client";

import React from "react";
import { PlugZap } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import type { InstanceFeatures } from "@/lib/instance/instanceInfo";
import { FEATURE_DOCS_URL } from "@/lib/instance/featureGates";

/**
 * The one empty state for a surface whose integration is not configured on
 * this install (GET /api/v1/instance features.<name> = false). It names what
 * is missing and links the self-hosting docs; it never upsells.
 */
export default function FeatureUnavailable({
  feature,
  className,
}: {
  feature: keyof InstanceFeatures;
  className?: string;
}) {
  const { locale } = useSimpleLocale();
  const tr = (key: string) => {
    const r = getTranslation(`common.featureUnavailable.${key}`, locale);
    return Array.isArray(r) ? r[0] || key : (r as string);
  };
  const docs = FEATURE_DOCS_URL[feature];
  return (
    <EmptyState
      panel
      icon={PlugZap}
      title={tr(`${feature}.title`)}
      subtitle={tr(`${feature}.body`)}
      action={
        docs ? (
          <a
            href={docs}
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm font-medium text-brand underline underline-offset-2"
          >
            {tr("docsLink")}
          </a>
        ) : undefined
      }
      className={className}
      data-testid={`feature-unavailable-${feature}`}
    />
  );
}
