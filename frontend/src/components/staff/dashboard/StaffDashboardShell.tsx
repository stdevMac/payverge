"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Spinner } from "@nextui-org/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/providers/HybridAuthProvider";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
  clearStaffSession,
  redirectToStaffLogin,
  type StaffData,
} from "@/utils/staffAuth";
import { coverageApi, type CancelKind } from "@/api/coverage";
import { engagementBadgesApi } from "@/api/engagementBadges";
import { positionsApi, type Position } from "@/api/positions";
import { queryKeys } from "@/api/queryKeys";
import { intlLocaleFor } from "@/utils/intlLocale";
import { useToast } from "@/contexts/ToastContext";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import StaffBottomNav, { type StaffTab } from "./StaffBottomNav";
import StaffTodayHome from "./StaffTodayHome";
import StaffProfile from "./StaffProfile";
import MyScheduleView from "./MyScheduleView";
import StaffMore, { isStaffMoreSection, type Section } from "./StaffMore";
import AvailabilityEditor from "./AvailabilityEditor";
import TimeOffRequests from "./TimeOffRequests";
import CoverageBoard, { type CoverageBoardLabels } from "./CoverageBoard";
import ChatList from "./ChatList";
import AnnouncementsFeed from "./AnnouncementsFeed";
import LogbookContainer from "./LogbookContainer";
import ChecklistRunnerContainer from "./ChecklistRunnerContainer";
import OnboardingChecklist from "./OnboardingChecklist";
import DocumentsContainer from "./DocumentsContainer";
import ShoutoutsContainer from "./ShoutoutsContainer";
import PollsContainer from "./PollsContainer";
import NotificationBell from "./NotificationBell";
import HoursHistory from "./HoursHistory";
import { usePushSubscription } from "@/hooks/usePushSubscription";
import { usePwaInstall } from "@/providers/PwaInstallProvider";
import DashboardPwaRecorder from "@/components/pwa/DashboardPwaRecorder";

export default function StaffDashboardShell() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { isInitialized, isLoading, isStaffUser, staffData } = useAuth();
  const { locale } = useSimpleLocale();
  const pwaInstall = usePwaInstall();
  const [installPending, setInstallPending] = useState(false);
  const installPendingRef = useRef(false);
  const mountedRef = useRef(true);
  const searchParams = useSearchParams();
  const [tab, setTab] = useState<StaffTab>(() => {
    const v = searchParams?.get("tab") ?? "";
    if (v === "today" || v === "schedule" || v === "chat" || v === "more")
      return v;
    if (isStaffMoreSection(v)) return "more";
    return "today";
  });
  // Which "More" sub-surface is open — controlled here so the Today-home glance
  // cards can deep-link into a section (e.g. Coverage) at runtime, not just at
  // mount from the URL.
  const [moreSection, setMoreSection] = useState<Section>(() => {
    const v = searchParams?.get("tab") ?? "";
    return isStaffMoreSection(v) ? v : "menu";
  });
  const openSection = useCallback((section: Section) => {
    setMoreSection(section);
    setTab("more");
  }, []);
  const [signingOut, setSigningOut] = useState(false);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const handleInstall = useCallback(async () => {
    if (installPendingRef.current) return;

    installPendingRef.current = true;
    setInstallPending(true);
    try {
      await pwaInstall.requestInstall();
    } catch {
      // Release the local guard after browser prompt failures; the provider
      // owns user-facing install state and help.
    } finally {
      installPendingRef.current = false;
      if (mountedRef.current) setInstallPending(false);
    }
  }, [pwaInstall]);
  const [pushDismissed, setPushDismissed] = useState(() => {
    if (typeof localStorage === "undefined") return false;
    return localStorage.getItem("staff_push_dismissed") === "1";
  });
  const {
    isSupported: pushSupported,
    isSubscribed: pushSubscribed,
    subscribe: pushSubscribe,
  } = usePushSubscription(staffData?.business_id ?? 0);

  // Staff-nav badge counts (Phase 5 · Slice 5a): buried unacked announcements +
  // pending checklists, polled so the More menu / bottom-nav dot surface work the
  // staffer would otherwise miss. Non-blocking: any error falls back to zeros
  // (the nav simply shows no badges). retry:false — a transient failure just skips
  // a tick; the next poll recovers.
  const badgesBusinessId = staffData ? String(staffData.business_id) : "";
  const { data: engagementBadges } = useQuery({
    queryKey: queryKeys.engagement.badges(badgesBusinessId),
    queryFn: () => engagementBadgesApi.get(badgesBusinessId),
    enabled: Boolean(staffData),
    retry: false,
    refetchInterval: 60_000,
  });
  const announcementsBadge = engagementBadges?.unacked_announcements ?? 0;
  const checklistsBadge = engagementBadges?.pending_checklists ?? 0;
  const moreHasBadge = announcementsBadge > 0 || checklistsBadge > 0;

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffHome.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Schedule tab strings live in their own namespace; the shell resolves them
  // and hands MyScheduleView a fully-localized `labels` object (labels-prop
  // contract — the child holds no inline strings).
  const ts = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffSchedule.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Availability + time-off strings (Slice 3) — the shell resolves them and
  // hands each "More" sub-surface a fully-localized `labels` object.
  const ta = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffAvailability.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Time-clock strings (Slice 4) — own namespace; folded into the TodayCard
  // labels object (labels-prop contract; the child holds no inline strings).
  const tc = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffTimeclock.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Coverage strings (Slice 5) — own namespace; the shell resolves them and
  // hands the "More → Coverage" sub-surface a fully-localized `labels` object.
  const tcv = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffCoverage.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Chat & announcements strings (Slice 7) — own namespace. The chat surface lives
  // in the bottom-nav "Chat" tab (ChatList self-resolves), so the shell only needs
  // the announcements menu-entry labels for the "More" section here.
  const tch = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffChat.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Logbook strings (Slice 8) — own namespace. The shell only needs the menu
  // entry/hint here; LogbookContainer self-resolves the in-view strings.
  const tlb = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffLogbook.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Engagement strings (Slice 9) — own namespace. The shell resolves the "More"
  // menu entries/hints and the per-surface label bundles, then hands each
  // engagement sub-surface a fully-localized `labels` object (labels-prop
  // contract). NO money on any engagement wire.
  const te = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffEngagement.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Notification inbox strings (Phase 1) — own namespace.
  const tn = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffNotifications.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Hours-history strings (Phase 1) — own namespace.
  const th = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffHours.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Today-home strings (Phase 2) — own namespace. The shell resolves them and
  // hands StaffTodayHome a fully-localized `labels` object (labels-prop
  // contract; the child holds no inline strings).
  const tt = useCallback(
    (key: string): string => {
      const v = getTranslation(`staffToday.${key}`, locale);
      return typeof v === "string" ? v : key;
    },
    [locale],
  );

  // Auth guard: once auth has resolved, a non-staff visitor is bounced to login.
  useEffect(() => {
    if (isInitialized && !isLoading && !isStaffUser) {
      router.replace("/staff/login");
    }
  }, [isInitialized, isLoading, isStaffUser, router]);

  const roleName = useMemo(() => {
    if (!staffData) return "";
    return t(`profile.roles.${staffData.role}`);
  }, [staffData, t]);

  // Abort in-flight reads before the cookie goes, then leave with a full page
  // load: a router navigation kept the auth refresh timer and dashboard
  // polling alive long enough to fire 401s against the cleared session.
  const handleSignOut = useCallback(async () => {
    setSigningOut(true);
    try {
      await queryClient.cancelQueries();
      await clearStaffSession();
    } finally {
      redirectToStaffLogin();
    }
  }, [queryClient]);

  if (!isInitialized || isLoading || !staffData) {
    return (
      <div
        role="status"
        aria-live="polite"
        className="flex min-h-[60vh] items-center justify-center"
      >
        <Spinner aria-label={t("today.loading")} />
      </div>
    );
  }

  // Render the staff name as its own node rather than interpolating it into the
  // translated greeting string. Splitting the localized "Hi, {name}" template
  // around the token keeps the phrasing localized while guaranteeing the
  // operator's name is always surfaced (even if a locale's template were to
  // drop the token).
  const [greetingBefore, greetingAfter = ""] = t("greeting").split("{name}");
  const businessLine = t("businessLine").replace(
    "{business}",
    staffData.business_name || "",
  );

  return (
    <div className="mx-auto min-h-screen max-w-md px-4 pb-24 pt-4 md:max-w-2xl md:pb-8">
      <DashboardPwaRecorder
        ready={Boolean(isInitialized && !isLoading && isStaffUser && staffData)}
      />
      <header className="mb-4 flex items-start justify-between">
        <div>
          <h1 className="font-title text-xl text-gray-900">
            {greetingBefore}
            <span>{staffData.name}</span>
            {greetingAfter}
          </h1>
          {staffData.business_name ? (
            <p className="text-sm text-gray-500">{businessLine}</p>
          ) : null}
        </div>
        <NotificationBell
          businessId={String(staffData.business_id)}
          staffId={staffData.id}
          locale={intlLocaleFor(locale)}
          labels={{
            bellAria: tn("bell.aria"),
            title: tn("inbox.title"),
            empty: tn("inbox.empty"),
            markAll: tn("inbox.markAll"),
          }}
        />
      </header>

      {pushSupported && !pushSubscribed && !pushDismissed ? (
        <div className="mb-4 rounded-2xl border border-brand-200 bg-brand-50 p-4">
          <p className="mb-2 text-sm text-brand-900">{tn("push.prompt")}</p>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => pushSubscribe()}
              className="rounded-full bg-brand px-4 py-1.5 text-sm font-semibold text-white transition hover:bg-brand-dark"
            >
              {tn("push.enable")}
            </button>
            <button
              type="button"
              onClick={() => {
                localStorage.setItem("staff_push_dismissed", "1");
                setPushDismissed(true);
              }}
              className="rounded-full px-4 py-1.5 text-sm text-gray-600 transition hover:text-gray-900"
            >
              {tn("push.dismiss")}
            </button>
          </div>
        </div>
      ) : null}

      <div>
        {tab === "today" ? (
          <StaffTodayHome
            staff={staffData}
            locale={locale}
            onOpenSchedule={() => setTab("schedule")}
            onOpenSection={openSection}
            labels={{
              nextShiftTitle: tt("nextShiftTitle"),
              onShiftNow: tt("onShiftNow"),
              startsInMinutes: tt("startsInMinutes"),
              startsInHours: tt("startsInHours"),
              startsTomorrow: tt("startsTomorrow"),
              noNextShiftTitle: tt("noNextShiftTitle"),
              noNextShiftHint: tt("noNextShiftHint"),
              moreThisWeek: tt("moreThisWeek"),
              seeSchedule: tt("seeSchedule"),
              positionFallback: tt("positionFallback"),
              loading: tt("loading"),
              openShiftsTitle: tt("openShiftsTitle"),
              claim: tt("claim"),
              claiming: tt("claiming"),
              claimSuccess: tt("claimSuccess"),
              seeAllOpen: tt("seeAllOpen"),
              pendingTitle: tt("pendingTitle"),
              pendingCoverage: tt("pendingCoverage"),
              pendingTimeOff: tt("pendingTimeOff"),
              manageRequests: tt("manageRequests"),
              checklistTitle: tt("checklistTitle"),
              checklistRemaining: tt("checklistRemaining"),
              checklistDone: tt("checklistDone"),
              openChecklist: tt("openChecklist"),
              announcementTitle: tt("announcementTitle"),
              acknowledge: tt("acknowledge"),
              acknowledging: tt("acknowledging"),
              acknowledged: tt("acknowledged"),
              ackSuccess: tt("ackSuccess"),
              allAnnouncements: tt("allAnnouncements"),
              actionError: tt("actionError"),
            }}
            clockLabels={{
              title: t("today.title"),
              serviceToolsTitle: t("serviceTools.title"),
              serviceToolsDescription: t("serviceTools.description"),
              serviceToolsOpen: t("serviceTools.open"),
              loading: t("today.loading"),
              clockIn: tc("clockIn"),
              clockOut: tc("clockOut"),
              clockingIn: tc("clockingIn"),
              clockingOut: tc("clockingOut"),
              onTheClock: tc("onTheClock"),
              sinceTemplate: tc("sinceTemplate"),
              notClockedIn: tc("notClockedIn"),
              workedToday: tc("workedToday"),
              breakLabel: tc("breakLabel"),
              breakPreset: tc("breakPreset"),
              hoursUnit: tc("hoursUnit"),
              minutesUnit: tc("minutesUnit"),
              clockInError: tc("clockInError"),
              clockOutError: tc("clockOutError"),
              breakError: tc("breakError"),
            }}
          />
        ) : null}

        {tab === "schedule" ? (
          <MyScheduleView
            staff={staffData}
            locale={locale}
            labels={{
              title: ts("title"),
              subtitle: ts("subtitle"),
              loading: ts("loading"),
              error: ts("error"),
              emptyTitle: ts("emptyTitle"),
              emptySubtitle: ts("emptySubtitle"),
              hoursUnit: ts("hoursUnit"),
              minutesUnit: ts("minutesUnit"),
              breakTemplate: ts("breakTemplate"),
              positionFallback: ts("positionFallback"),
              thisWeek: ts("thisWeek"),
              nextWeek: ts("nextWeek"),
              // Coverage-initiation strings live in the coverage namespace (tcv).
              offerAction: tcv("offerAction"),
              requested: tcv("requested"),
              offerSuccess: tcv("offerSuccess"),
              giveUpSuccess: tcv("giveUpSuccess"),
              actionError: tcv("actionError"),
              offer: {
                title: tcv("offer.title"),
                subtitle: tcv("offer.subtitle"),
                offerCover: tcv("offer.offerCover"),
                offerCoverHint: tcv("offer.offerCoverHint"),
                giveUp: tcv("offer.giveUp"),
                giveUpHint: tcv("offer.giveUpHint"),
                cancel: tcv("offer.cancel"),
                close: tcv("offer.close"),
              },
            }}
          />
        ) : null}

        {tab === "chat" ? (
          <ChatList
            businessId={String(staffData.business_id)}
            currentStaffId={staffData.id}
          />
        ) : null}

        {tab === "more" ? (
          <StaffMore
            section={moreSection}
            onSectionChange={setMoreSection}
            announcementsBadge={announcementsBadge}
            checklistsBadge={checklistsBadge}
            onInstallApp={
              pwaInstall.state === "installed"
                ? undefined
                : () => void handleInstall()
            }
            installAppPending={installPending}
            labels={{
              title: ta("menu.title"),
              profileEntry: ta("menu.profileEntry"),
              profileHint: ta("menu.profileHint"),
              installAppEntry: String(
                getTranslation("pwa.account.label", locale),
              ),
              installAppHint: String(
                getTranslation("pwa.account.hint", locale),
              ),
              availabilityEntry: ta("menu.availabilityEntry"),
              availabilityHint: ta("menu.availabilityHint"),
              timeOffEntry: ta("menu.timeOffEntry"),
              timeOffHint: ta("menu.timeOffHint"),
              coverageEntry: tcv("menu.entry"),
              coverageHint: tcv("menu.hint"),
              announcementsEntry: tch("announcements.menuEntry"),
              announcementsHint: tch("announcements.menuHint"),
              logbookEntry: tlb("menu.entry"),
              logbookHint: tlb("menu.hint"),
              checklistsEntry: te("menu.checklistsEntry"),
              checklistsHint: te("menu.checklistsHint"),
              onboardingEntry: te("menu.onboardingEntry"),
              onboardingHint: te("menu.onboardingHint"),
              documentsEntry: te("menu.documentsEntry"),
              documentsHint: te("menu.documentsHint"),
              recognitionEntry: te("menu.recognitionEntry"),
              recognitionHint: te("menu.recognitionHint"),
              hoursEntry: ta("menu.hoursEntry"),
              hoursHint: ta("menu.hoursHint"),
              pollsEntry: te("menu.pollsEntry"),
              pollsHint: te("menu.pollsHint"),
              groupResources: ta("menu.groupResources"),
              groupCommunity: ta("menu.groupCommunity"),
              groupAccount: ta("menu.groupAccount"),
              back: ta("menu.back"),
            }}
            renderProfile={() => (
              <StaffProfile
                staff={staffData}
                onSignOut={handleSignOut}
                signingOut={signingOut}
                labels={{
                  title: t("profile.title"),
                  roleLabel: t("profile.roleLabel"),
                  businessLabel: t("profile.businessLabel"),
                  emailLabel: t("profile.emailLabel"),
                  lastLoginLabel: t("profile.lastLoginLabel"),
                  signOut: t("profile.signOut"),
                  roleName,
                }}
              />
            )}
            renderAvailability={() => (
              <AvailabilityEditor
                staff={staffData}
                locale={locale}
                labels={{
                  title: ta("availability.title"),
                  subtitle: ta("availability.subtitle"),
                  loading: ta("availability.loading"),
                  error: ta("availability.error"),
                  emptyTitle: ta("availability.emptyTitle"),
                  emptySubtitle: ta("availability.emptySubtitle"),
                  addTitle: ta("availability.addTitle"),
                  weekdayLabel: ta("availability.weekdayLabel"),
                  kindLabel: ta("availability.kindLabel"),
                  kindPreferred: ta("availability.kindPreferred"),
                  kindUnavailable: ta("availability.kindUnavailable"),
                  startLabel: ta("availability.startLabel"),
                  endLabel: ta("availability.endLabel"),
                  add: ta("availability.add"),
                  remove: ta("availability.remove"),
                  invalidRange: ta("availability.invalidRange"),
                  save: ta("availability.save"),
                  saving: ta("availability.saving"),
                  saved: ta("availability.saved"),
                  saveError: ta("availability.saveError"),
                }}
              />
            )}
            renderTimeOff={() => (
              <TimeOffRequests
                staff={staffData}
                locale={locale}
                labels={{
                  title: ta("timeOff.title"),
                  subtitle: ta("timeOff.subtitle"),
                  loading: ta("timeOff.loading"),
                  error: ta("timeOff.error"),
                  emptyTitle: ta("timeOff.emptyTitle"),
                  emptySubtitle: ta("timeOff.emptySubtitle"),
                  formTitle: ta("timeOff.formTitle"),
                  startLabel: ta("timeOff.startLabel"),
                  endLabel: ta("timeOff.endLabel"),
                  reasonLabel: ta("timeOff.reasonLabel"),
                  reasonPlaceholder: ta("timeOff.reasonPlaceholder"),
                  submit: ta("timeOff.submit"),
                  submitting: ta("timeOff.submitting"),
                  submitError: ta("timeOff.submitError"),
                  invalidRange: ta("timeOff.invalidRange"),
                  statusPending: ta("timeOff.statusPending"),
                  statusApproved: ta("timeOff.statusApproved"),
                  statusDenied: ta("timeOff.statusDenied"),
                  statusCancelled: ta("timeOff.statusCancelled"),
                }}
              />
            )}
            renderCoverage={() => (
              <CoverageSection
                staff={staffData}
                locale={locale}
                labels={{
                  title: tcv("title"),
                  openShifts: tcv("openShifts"),
                  swapInbox: tcv("swapInbox"),
                  myRequests: tcv("myRequests"),
                  claim: tcv("claim"),
                  accept: tcv("accept"),
                  cancel: tcv("cancel"),
                  rowSwap: tcv("row.swap"),
                  rowGiveup: tcv("row.giveup"),
                  rowClaim: tcv("row.claim"),
                  statusPending: tcv("status.pending"),
                  statusAccepted: tcv("status.accepted"),
                  statusPendingApproval: tcv("status.pendingApproval"),
                  statusApproved: tcv("status.approved"),
                  statusDenied: tcv("status.denied"),
                  statusCancelled: tcv("status.cancelled"),
                  statusWithdrawn: tcv("status.withdrawn"),
                  emptyOpenTitle: tcv("empty.openTitle"),
                  emptyOpenSubtitle: tcv("empty.openSubtitle"),
                  emptyInboxTitle: tcv("empty.inboxTitle"),
                  emptyInboxSubtitle: tcv("empty.inboxSubtitle"),
                  emptyMineTitle: tcv("empty.mineTitle"),
                  emptyMineSubtitle: tcv("empty.mineSubtitle"),
                  loading: tcv("loading"),
                  positionFallback: tcv("positionFallback"),
                  claimSuccess: tcv("claimSuccess"),
                  acceptSuccess: tcv("acceptSuccess"),
                  cancelSuccess: tcv("cancelSuccess"),
                  actionError: tcv("actionError"),
                }}
              />
            )}
            renderAnnouncements={() => (
              <AnnouncementsFeed businessId={String(staffData.business_id)} />
            )}
            renderLogbook={() => (
              <LogbookContainer businessId={String(staffData.business_id)} />
            )}
            renderChecklists={() => (
              <ChecklistRunnerContainer
                businessId={String(staffData.business_id)}
                locale={locale}
                labels={{
                  title: te("checklists.title"),
                  empty: te("checklists.empty"),
                  emptyHint: te("checklists.emptyHint"),
                  loading: te("checklists.loading"),
                  statusPending: te("checklists.statusPending"),
                  statusInProgress: te("checklists.statusInProgress"),
                  statusComplete: te("checklists.statusComplete"),
                  back: te("checklists.back"),
                  required: te("checklists.required"),
                  itemsEmpty: te("checklists.itemsEmpty"),
                  openRun: te("checklists.openRun"),
                  tickError: te("checklists.tickError"),
                }}
              />
            )}
            renderOnboarding={() => (
              <OnboardingChecklist
                businessId={String(staffData.business_id)}
                locale={locale}
                labels={{
                  title: te("onboarding.title"),
                  empty: te("onboarding.empty"),
                  emptyHint: te("onboarding.emptyHint"),
                  loading: te("onboarding.loading"),
                  statusPending: te("checklists.statusPending"),
                  statusInProgress: te("checklists.statusInProgress"),
                  statusComplete: te("checklists.statusComplete"),
                  back: te("checklists.back"),
                  required: te("checklists.required"),
                  itemsEmpty: te("checklists.itemsEmpty"),
                  openRun: te("checklists.openRun"),
                  tickError: te("checklists.tickError"),
                }}
              />
            )}
            renderDocuments={() => (
              <DocumentsContainer businessId={String(staffData.business_id)} />
            )}
            renderRecognition={() => (
              <ShoutoutsContainer
                businessId={String(staffData.business_id)}
                currentStaffId={staffData.id}
              />
            )}
            renderHours={() => (
              <HoursHistory
                businessId={String(staffData.business_id)}
                locale={intlLocaleFor(locale)}
                labels={{
                  title: th("title"),
                  subtitle: th("subtitle"),
                  loading: th("loading"),
                  empty: th("empty"),
                  weekTotal: th("weekTotal"),
                  hoursUnit: th("hoursUnit"),
                  statusPending: th("statusPending"),
                  statusApproved: th("statusApproved"),
                }}
              />
            )}
            renderPolls={() => (
              <PollsContainer businessId={String(staffData.business_id)} />
            )}
          />
        ) : null}
      </div>

      <StaffBottomNav
        active={tab}
        onChange={setTab}
        badgedTabs={{ more: moreHasBadge }}
        labels={{
          label: t("nav.label"),
          today: t("nav.today"),
          schedule: t("nav.schedule"),
          chat: t("nav.chat"),
          more: t("nav.more"),
        }}
      />
    </div>
  );
}

interface CoverageSectionLabels extends CoverageBoardLabels {
  positionFallback: string;
  claimSuccess: string;
  acceptSuccess: string;
  cancelSuccess: string;
  actionError: string;
}

/**
 * Data container for the staff "More → Coverage" surface. Mounts only when the
 * staff member opens Coverage (StaffMore render-prop), so the three coverage
 * fetches stay dormant until then. Owns the open/mine queries, the claim/accept
 * mutations (with an optimistic `busyId` on the in-flight row), position-name
 * resolution, and the locale time formatter — then hands a fully-resolved
 * `labels` object to the presentational CoverageBoard. NO money on this wire.
 */
function CoverageSection({
  staff,
  locale,
  labels,
}: {
  staff: StaffData;
  locale: string;
  labels: CoverageSectionLabels;
}) {
  const businessId = String(staff.business_id);
  const queryClient = useQueryClient();
  const toast = useToast();

  const openKey = queryKeys.coverage.open(businessId);
  const mineKey = queryKeys.coverage.mine(businessId, staff.id);

  const openQuery = useQuery({
    queryKey: openKey,
    queryFn: () => coverageApi.listOpen(businessId),
  });
  const mineQuery = useQuery({
    queryKey: mineKey,
    queryFn: () => coverageApi.listMine(businessId),
  });
  const positionsQuery = useQuery({
    queryKey: ["positions", businessId],
    queryFn: () => positionsApi.list(businessId),
    staleTime: 5 * 60 * 1000,
  });

  const invalidateCoverage = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: openKey });
    void queryClient.invalidateQueries({ queryKey: mineKey });
  }, [queryClient, openKey, mineKey]);

  // Any coverage event (a coworker claims/accepts, a manager decides) shifts what
  // I can pick up and the status of my own requests — live-refresh both lists.
  useStaffRealtime({
    businessId: staff.business_id,
    onOpenShiftClaimed: invalidateCoverage,
    onOpenShiftDecided: invalidateCoverage,
    onSwapRequested: invalidateCoverage,
    onSwapAccepted: invalidateCoverage,
    onSwapDecided: invalidateCoverage,
    // Coverage events emitted during an SSE gap were never delivered — resync.
    onReconnect: invalidateCoverage,
  });

  const positionsById = useMemo(() => {
    const map = new Map<number, Position>();
    (positionsQuery.data ?? []).forEach((p) => map.set(p.id, p));
    return map;
  }, [positionsQuery.data]);

  const claimMutation = useMutation({
    mutationFn: (shiftId: number) => coverageApi.claim(businessId, shiftId),
    onSuccess: () => toast.showSuccess(labels.claimSuccess),
    onError: () => toast.showError(labels.actionError),
    onSettled: () => invalidateCoverage(),
  });
  const acceptMutation = useMutation({
    mutationFn: (swapId: number) => coverageApi.acceptSwap(businessId, swapId),
    onSuccess: () => toast.showSuccess(labels.acceptSuccess),
    onError: () => toast.showError(labels.actionError),
    onSettled: () => invalidateCoverage(),
  });
  const cancelMutation = useMutation({
    mutationFn: (vars: { requestId: number; kind: CancelKind }) =>
      coverageApi.cancel(businessId, vars.requestId, vars.kind),
    onSuccess: () => toast.showSuccess(labels.cancelSuccess),
    onError: () => toast.showError(labels.actionError),
    onSettled: () => invalidateCoverage(),
  });

  // Only the row whose mutation is in flight is disabled.
  const busyId = claimMutation.isPending
    ? (claimMutation.variables ?? null)
    : acceptMutation.isPending
      ? (acceptMutation.variables ?? null)
      : null;

  // The cancel row key mirrors CoverageBoard's My-requests keys ("sw-"/"cl-"), so
  // an in-flight cancel disables exactly its own row (swap and claim id-spaces are
  // distinct tables — key by kind to avoid a cross-table id collision).
  const cancelBusyKey =
    cancelMutation.isPending && cancelMutation.variables
      ? (cancelMutation.variables.kind === "open_claim" ? "cl-" : "sw-") +
        cancelMutation.variables.requestId
      : null;

  const positionName = useCallback(
    (positionId: number) =>
      positionsById.get(positionId)?.name || labels.positionFallback,
    [positionsById, labels.positionFallback],
  );

  const formatRange = useCallback(
    (startsAt: string, endsAt: string) => {
      const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
        weekday: "short",
        hour: "numeric",
        minute: "2-digit",
      });
      return `${fmt.format(new Date(startsAt))} – ${fmt.format(new Date(endsAt))}`;
    },
    [locale],
  );

  return (
    <CoverageBoard
      open={openQuery.data ?? null}
      mine={mineQuery.data ?? null}
      loading={openQuery.isLoading || mineQuery.isLoading}
      busyId={busyId}
      cancelBusyKey={cancelBusyKey}
      onClaim={(shiftId) => claimMutation.mutate(shiftId)}
      onAccept={(swapId) => acceptMutation.mutate(swapId)}
      onCancel={(requestId, kind) => cancelMutation.mutate({ requestId, kind })}
      positionName={positionName}
      formatRange={formatRange}
      labels={labels}
    />
  );
}
