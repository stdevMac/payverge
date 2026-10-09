import React from "react";
import { BusinessLockNotice } from "./BusinessLockNotice";

interface DashboardLockedTabViewProps {
  title: string;
  subtitle?: string;
  businessId: number | string;
}

export default function DashboardLockedTabView({
  title,
  subtitle,
  businessId,
}: DashboardLockedTabViewProps) {
  return (
    <div className="mx-auto max-w-6xl p-4">
      <div className="mb-6">
        <h1 className="font-title text-2xl text-ink-900">
          {title}
        </h1>
        {subtitle ? (
          <p className="mt-1 text-sm text-ink-600">{subtitle}</p>
        ) : null}
      </div>

      <BusinessLockNotice businessId={businessId} />
    </div>
  );
}
