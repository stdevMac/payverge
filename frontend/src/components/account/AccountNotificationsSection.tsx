"use client";

import Link from "next/link";
import { Bell, ChevronRight, Building2 } from "lucide-react";

import { getBusinessNotificationSettingsPath } from "@/utils/businessUrl";
import { sectionHeadingClass } from "@/components/ui/headingStyles";

// Pre-localised copy, passed in from the account page's translation helper —
// mirrors AccountPrivacyPanel so this component stays presentational and
// trivially testable.
export interface AccountNotificationsCopy {
  sectionTitle: string;
  sectionDescription: string;
  manageLink: string;
  emptyTitle: string;
  emptyDescription: string;
  errorMessage: string;
  loadingLabel: string;
}

interface AccountNotificationsBusiness {
  id: number;
  business_id?: string;
  name: string;
}

export interface AccountNotificationsSectionProps {
  businesses: AccountNotificationsBusiness[];
  loading: boolean;
  error: boolean;
  copy: AccountNotificationsCopy;
}

/**
 * "Email & notifications" block on the operator account page. Operator email
 * preferences live per-business (Settings → Notifications), so this section
 * lists the operator's businesses and deep-links each one straight to its
 * notification settings. It is the landing target for the "Manage preferences
 * or unsubscribe" link in operator emails.
 */
export function AccountNotificationsSection({
  businesses,
  loading,
  error,
  copy,
}: AccountNotificationsSectionProps) {
  return (
    <section className="mb-12">
      <div className="flex items-center gap-3 mb-2">
        <div className="w-9 h-9 rounded-xl bg-ink-50 border border-ink-100 flex items-center justify-center text-ink-500 shrink-0">
          <Bell className="w-4 h-4" />
        </div>
        <h2 className={sectionHeadingClass}>
          {copy.sectionTitle}
        </h2>
      </div>
      <p className="text-sm text-ink-600 mb-4 ml-12">
        {copy.sectionDescription}
      </p>

      {loading ? (
        <div
          role="status"
          className="border border-ink-200 rounded-2xl px-6 py-8 text-sm text-ink-500 flex items-center gap-3"
        >
          <span className="h-4 w-4 animate-spin rounded-full border-2 border-ink-200 border-t-ink-500 motion-safe:animate-spin" />
          {copy.loadingLabel}
        </div>
      ) : error ? (
        <div className="border border-rose-200 bg-rose-50 rounded-2xl px-6 py-5 text-sm text-rose-800">
          {copy.errorMessage}
        </div>
      ) : businesses.length === 0 ? (
        <div className="border border-ink-200 rounded-2xl px-6 py-8 text-center">
          <p className="text-sm font-medium text-ink-900">{copy.emptyTitle}</p>
          <p className="text-sm text-ink-500 mt-1">{copy.emptyDescription}</p>
        </div>
      ) : (
        <ul className="border border-ink-200 rounded-2xl divide-y divide-ink-100 overflow-hidden">
          {businesses.map((business) => (
            <li key={business.id}>
              <Link
                href={getBusinessNotificationSettingsPath(business)}
                className="group flex items-center gap-3 px-6 py-4 hover:bg-ink-50/60 transition-colors"
              >
                <div className="w-9 h-9 rounded-lg bg-ink-50 border border-ink-100 flex items-center justify-center text-ink-400 shrink-0">
                  <Building2 className="w-4 h-4" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-ink-900 truncate">
                    {business.name}
                  </p>
                  <p className="text-xs text-ink-500">{copy.manageLink}</p>
                </div>
                <ChevronRight className="w-4 h-4 text-ink-400 group-hover:text-ink-600 transition-colors shrink-0" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
