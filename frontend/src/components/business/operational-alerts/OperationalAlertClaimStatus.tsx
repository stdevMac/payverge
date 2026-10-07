"use client";

import React from "react";
import type { OperationalAlertResourceType } from "@/api/operationalAlerts";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { useOptionalOperationalAlerts } from "./useOperationalAlerts";

interface OperationalAlertClaimStatusProps {
  resourceType: OperationalAlertResourceType;
  resourceId: number | null | undefined;
  className?: string;
}

export default function OperationalAlertClaimStatus({
  resourceType,
  resourceId,
  className = "",
}: OperationalAlertClaimStatusProps) {
  const { locale } = useSimpleLocale();
  const alerts = useOptionalOperationalAlerts();
  if (!alerts || !resourceId) return null;

  const alert = alerts.getAlertForResource(resourceType, resourceId);
  if (!alert || alert.status !== "claimed" || !alert.claimed_by_name) {
    return null;
  }

  const label = String(
    getTranslation("businessSettings.notifications.claimedBy", locale),
  ).replace("{{name}}", alert.claimed_by_name);

  return (
    <span
      className={`inline-flex items-center rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-800 ring-1 ring-amber-200 ${className}`}
      title={label}
    >
      {label}
    </span>
  );
}
