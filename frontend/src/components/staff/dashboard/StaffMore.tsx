"use client";

import React, { useCallback, useState } from "react";
import {
  ArrowLeftRight,
  Award,
  BarChart3,
  CalendarClock,
  CalendarOff,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  Clock,
  FileText,
  GraduationCap,
  Megaphone,
  NotebookPen,
  User,
  Download,
  type LucideIcon,
} from "lucide-react";

export interface StaffMoreLabels {
  title: string;
  profileEntry: string;
  profileHint: string;
  installAppEntry: string;
  installAppHint: string;
  availabilityEntry: string;
  availabilityHint: string;
  timeOffEntry: string;
  timeOffHint: string;
  coverageEntry: string;
  coverageHint: string;
  announcementsEntry: string;
  announcementsHint: string;
  logbookEntry: string;
  logbookHint: string;
  checklistsEntry: string;
  checklistsHint: string;
  onboardingEntry: string;
  onboardingHint: string;
  documentsEntry: string;
  documentsHint: string;
  hoursEntry: string;
  hoursHint: string;
  recognitionEntry: string;
  recognitionHint: string;
  pollsEntry: string;
  pollsHint: string;
  // Small uppercase group headers that break the flat 12-item list into scannable
  // clusters. The top action items render header-less; these label the rest.
  groupResources: string;
  groupCommunity: string;
  groupAccount: string;
  back: string;
}

export type Section =
  | "menu"
  | "profile"
  | "availability"
  | "timeoff"
  | "coverage"
  | "announcements"
  | "logbook"
  | "hours"
  | "checklists"
  | "onboarding"
  | "documents"
  | "recognition"
  | "polls";

const SECTION_VALUES: ReadonlySet<string> = new Set<Section>([
  "menu",
  "profile",
  "availability",
  "timeoff",
  "coverage",
  "announcements",
  "logbook",
  "hours",
  "checklists",
  "onboarding",
  "documents",
  "recognition",
  "polls",
]);

export function isStaffMoreSection(v: string): v is Section {
  return v !== "menu" && SECTION_VALUES.has(v);
}

export interface StaffMoreProps {
  labels: StaffMoreLabels;
  initialSection?: Section;
  /**
   * Controlled section. When provided, the parent owns which sub-surface is
   * open (used by the Today-home glance cards to deep-link into a section at
   * runtime) and must pair it with `onSectionChange`. Omit for the default
   * self-managed behavior seeded from `initialSection`.
   */
  section?: Section;
  onSectionChange?: (section: Section) => void;
  // "Needs attention" counts surfaced as pills on the menu entries so buried work
  // is visible without opening the surface (Phase 5 · Slice 5a). Money-free.
  announcementsBadge?: number;
  checklistsBadge?: number;
  onInstallApp?: () => void;
  installAppPending?: boolean;
  // Render-prop sections so each child (and its data fetch) mounts only when the
  // staff member actually opens it — the menu itself does no fetching.
  renderProfile: () => React.ReactNode;
  renderAvailability: () => React.ReactNode;
  renderTimeOff: () => React.ReactNode;
  renderCoverage: () => React.ReactNode;
  renderAnnouncements: () => React.ReactNode;
  renderLogbook: () => React.ReactNode;
  renderChecklists: () => React.ReactNode;
  renderOnboarding: () => React.ReactNode;
  renderDocuments: () => React.ReactNode;
  renderRecognition: () => React.ReactNode;
  renderHours: () => React.ReactNode;
  renderPolls: () => React.ReactNode;
}

function MenuEntry({
  icon: Icon,
  label,
  hint,
  onSelect,
  badge,
  disabled = false,
  busy = false,
}: {
  icon: LucideIcon;
  label: string;
  hint: string;
  onSelect: () => void;
  // A "needs attention" count (unacked announcements, pending checklists). Only
  // rendered when > 0 — mirrors the notification bell's numeric pill. Money-free.
  badge?: number;
  disabled?: boolean;
  busy?: boolean;
}) {
  const showBadge = typeof badge === "number" && badge > 0;
  return (
    <li>
      <button
        type="button"
        onClick={onSelect}
        disabled={disabled}
        aria-busy={busy}
        className="flex w-full items-center gap-3 rounded-xl border border-gray-200 p-4 text-left transition hover:border-brand-300 disabled:cursor-wait disabled:opacity-60"
      >
        <span
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand-50 text-brand"
          aria-hidden="true"
        >
          <Icon className="h-5 w-5" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium text-gray-900">
            {label}
          </span>
          <span className="block truncate text-xs text-gray-500">{hint}</span>
        </span>
        {showBadge ? (
          <span
            data-testid="menu-badge"
            className="flex min-w-[20px] shrink-0 items-center justify-center rounded-full bg-brand px-1.5 text-[11px] font-semibold text-white"
            aria-live="polite"
          >
            {badge > 99 ? "99+" : badge}
          </span>
        ) : null}
        <ChevronRight
          className="h-4 w-4 shrink-0 text-gray-400"
          aria-hidden="true"
        />
      </button>
    </li>
  );
}

export default function StaffMore({
  labels,
  initialSection,
  section: controlledSection,
  onSectionChange,
  announcementsBadge,
  checklistsBadge,
  onInstallApp,
  installAppPending = false,
  renderProfile,
  renderAvailability,
  renderTimeOff,
  renderCoverage,
  renderAnnouncements,
  renderLogbook,
  renderChecklists,
  renderOnboarding,
  renderDocuments,
  renderRecognition,
  renderHours,
  renderPolls,
}: StaffMoreProps) {
  const [internalSection, setInternalSection] = useState<Section>(
    () => initialSection ?? "menu",
  );
  // Controlled when the parent supplies `section`; otherwise self-managed. The
  // back button and menu entries route through one setter either way.
  const section = controlledSection ?? internalSection;
  const setSection = useCallback(
    (next: Section) => {
      onSectionChange?.(next);
      if (controlledSection === undefined) setInternalSection(next);
    },
    [controlledSection, onSectionChange],
  );

  // Dispatch render-props by section so the menu scales without a deep ternary.
  const renderers: Record<Exclude<Section, "menu">, () => React.ReactNode> = {
    profile: renderProfile,
    availability: renderAvailability,
    timeoff: renderTimeOff,
    coverage: renderCoverage,
    announcements: renderAnnouncements,
    logbook: renderLogbook,
    hours: renderHours,
    checklists: renderChecklists,
    onboarding: renderOnboarding,
    documents: renderDocuments,
    recognition: renderRecognition,
    polls: renderPolls,
  };

  if (section !== "menu") {
    return (
      <div className="space-y-4">
        <button
          type="button"
          onClick={() => setSection("menu")}
          className="inline-flex items-center gap-1 text-sm font-medium text-brand transition hover:text-brand-800"
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          {labels.back}
        </button>
        {renderers[section]()}
      </div>
    );
  }

  return (
    <section className="space-y-5" aria-label={labels.title}>
      <h2 className="font-title text-base text-gray-900">{labels.title}</h2>

      {/* Top action items — the day-to-day surfaces a staffer reaches for most.
          Header-less so they read as the primary list. */}
      <ul className="space-y-2">
        <MenuEntry
          icon={CalendarClock}
          label={labels.availabilityEntry}
          hint={labels.availabilityHint}
          onSelect={() => setSection("availability")}
        />
        <MenuEntry
          icon={CalendarOff}
          label={labels.timeOffEntry}
          hint={labels.timeOffHint}
          onSelect={() => setSection("timeoff")}
        />
        <MenuEntry
          icon={ArrowLeftRight}
          label={labels.coverageEntry}
          hint={labels.coverageHint}
          onSelect={() => setSection("coverage")}
        />
        <MenuEntry
          icon={Clock}
          label={labels.hoursEntry}
          hint={labels.hoursHint}
          onSelect={() => setSection("hours")}
        />
      </ul>

      <MenuGroup label={labels.groupResources}>
        <MenuEntry
          icon={ClipboardList}
          label={labels.checklistsEntry}
          hint={labels.checklistsHint}
          onSelect={() => setSection("checklists")}
          badge={checklistsBadge}
        />
        <MenuEntry
          icon={FileText}
          label={labels.documentsEntry}
          hint={labels.documentsHint}
          onSelect={() => setSection("documents")}
        />
        <MenuEntry
          icon={GraduationCap}
          label={labels.onboardingEntry}
          hint={labels.onboardingHint}
          onSelect={() => setSection("onboarding")}
        />
        <MenuEntry
          icon={NotebookPen}
          label={labels.logbookEntry}
          hint={labels.logbookHint}
          onSelect={() => setSection("logbook")}
        />
      </MenuGroup>

      <MenuGroup label={labels.groupCommunity}>
        <MenuEntry
          icon={Megaphone}
          label={labels.announcementsEntry}
          hint={labels.announcementsHint}
          onSelect={() => setSection("announcements")}
          badge={announcementsBadge}
        />
        <MenuEntry
          icon={Award}
          label={labels.recognitionEntry}
          hint={labels.recognitionHint}
          onSelect={() => setSection("recognition")}
        />
        <MenuEntry
          icon={BarChart3}
          label={labels.pollsEntry}
          hint={labels.pollsHint}
          onSelect={() => setSection("polls")}
        />
      </MenuGroup>

      <MenuGroup label={labels.groupAccount}>
        {onInstallApp ? (
          <MenuEntry
            icon={Download}
            label={labels.installAppEntry}
            hint={labels.installAppHint}
            onSelect={onInstallApp}
            disabled={installAppPending}
            busy={installAppPending}
          />
        ) : null}
        <MenuEntry
          icon={User}
          label={labels.profileEntry}
          hint={labels.profileHint}
          onSelect={() => setSection("profile")}
        />
      </MenuGroup>
    </section>
  );
}

function MenuGroup({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <h3 className="px-1 text-[11px] font-semibold uppercase tracking-wide text-gray-400">
        {label}
      </h3>
      <ul className="space-y-2">{children}</ul>
    </div>
  );
}
