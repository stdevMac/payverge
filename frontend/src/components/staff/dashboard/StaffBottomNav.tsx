"use client";

import React from "react";
import { CalendarDays, MessageSquare, Home, Menu as MenuIcon } from "lucide-react";

export type StaffTab = "today" | "schedule" | "chat" | "more";

export interface StaffBottomNavLabels {
  label: string;
  today: string;
  schedule: string;
  chat: string;
  more: string;
}

export interface StaffBottomNavProps {
  active: StaffTab;
  onChange: (tab: StaffTab) => void;
  labels: StaffBottomNavLabels;
  /**
   * Tabs that should show an unobtrusive attention dot (e.g. "More" when buried
   * announcements/checklists are waiting behind it). The exact count lives inside
   * the surface; the nav only signals "there's something here". Decorative —
   * marked aria-hidden.
   */
  badgedTabs?: Partial<Record<StaffTab, boolean>>;
}

const ITEMS: { key: StaffTab; icon: React.ComponentType<{ className?: string }> }[] = [
  { key: "today", icon: Home },
  { key: "schedule", icon: CalendarDays },
  { key: "chat", icon: MessageSquare },
  { key: "more", icon: MenuIcon },
];

export default function StaffBottomNav({ active, onChange, labels, badgedTabs }: StaffBottomNavProps) {
  return (
    <nav
      className="fixed inset-x-0 bottom-0 z-40 border-t border-gray-200 bg-white/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:static md:mx-auto md:max-w-md md:rounded-xl md:border"
      aria-label={labels.label}
    >
      <ul className="mx-auto flex max-w-md items-stretch justify-around">
        {ITEMS.map(({ key, icon: Icon }) => {
          const isActive = active === key;
          const badged = Boolean(badgedTabs?.[key]);
          return (
            <li key={key} className="flex-1">
              <button
                type="button"
                aria-current={isActive ? "page" : undefined}
                onClick={() => onChange(key)}
                className={`flex w-full flex-col items-center gap-1 py-2 text-[11px] font-medium transition-colors ${
                  isActive ? "text-brand" : "text-gray-500 hover:text-gray-700"
                }`}
              >
                <span className="relative">
                  <Icon className="h-5 w-5" />
                  {badged ? (
                    <span
                      data-testid={`nav-dot-${key}`}
                      className="absolute -right-1 -top-0.5 h-2 w-2 rounded-full bg-brand ring-2 ring-white"
                      aria-hidden="true"
                    />
                  ) : null}
                </span>
                {labels[key]}
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
