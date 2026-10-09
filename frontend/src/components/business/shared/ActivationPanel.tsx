"use client";

import React from "react";
import { Check } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { PremiumPanel } from "../premium";
import IconTile from "@/components/ui/IconTile";

export interface ActivationFeature {
  title: string;
  description?: string;
}

interface ActivationPanelProps {
  icon: LucideIcon;
  title: string;
  description: string;
  /** Plain checkmark lines — never boxed mini-cards. Keep to 2–4. */
  features?: ActivationFeature[];
  /** The activate/enable CTA. Caller-owned so loading/disabled states stay local. */
  action: React.ReactNode;
  /** Small reassurance line under the CTA ("You can disable this anytime…"). */
  footnote?: string;
  className?: string;
}

/**
 * The single activation / feature-off state shared by every dashboard tab
 * (Kitchen, Reservations, Delivery, Counter, Inventory, CRM…): centered icon,
 * serif title, one sentence, quiet feature checklist, one CTA.
 *
 * Its title is the second (and last) serif slot the tab contract allows (see
 * DashboardTabShell); the first belongs to PageHeader.
 */
export default function ActivationPanel({
  icon: Icon,
  title,
  description,
  features,
  action,
  footnote,
  className = "",
}: ActivationPanelProps) {
  return (
    <PremiumPanel as="section" className={className}>
      <div className="mx-auto flex max-w-xl flex-col items-center px-6 py-14 text-center sm:py-16">
        <IconTile icon={Icon} size="lg" />
        <h2 className="mt-5 font-title text-2xl tracking-tight text-ink-900">{title}</h2>
        <p className="mt-2 text-sm leading-6 text-ink-500">{description}</p>
        {features && features.length > 0 ? (
          <ul className="mt-5 space-y-2 text-left">
            {features.map((feature) => (
              <li key={feature.title} className="flex items-start gap-2.5 text-sm">
                <Check aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-brand" />
                <span className="text-ink-700">
                  {feature.title}
                  {feature.description ? (
                    <span className="text-ink-500"> — {feature.description}</span>
                  ) : null}
                </span>
              </li>
            ))}
          </ul>
        ) : null}
        <div className="mt-6">{action}</div>
        {footnote ? <p className="mt-3 text-xs text-ink-500">{footnote}</p> : null}
      </div>
    </PremiumPanel>
  );
}
