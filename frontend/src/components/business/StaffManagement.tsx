"use client";

import React, { useState, useEffect, useCallback, useMemo } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { Button, useDisclosure } from "@nextui-org/react";
import { Plus, Users, Briefcase, MessagesSquare } from "lucide-react";
import { toast } from "react-hot-toast";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import * as StaffAPI from "../../api/staff";
import { positionsApi } from "@/api/positions";
import { queryKeys } from "@/api/queryKeys";
import { useApiErrorMessage } from "@/i18n/useApiErrorMessage";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useAuth } from "@/providers/HybridAuthProvider";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import { hasPerm } from "@/constants/permissions";
import {
  DATE_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { buildSearchWithParam } from "@/hooks/urlState";
import DashboardLockedTabView from "./DashboardLockedTabView";
import ConfirmationModal from "./modals/ConfirmationModal";

// Import sub-components
import StaffSearchFilter from "../staff/StaffSearchFilter";
import StaffTable from "../staff/StaffTable";
import InvitationTable from "../staff/InvitationTable";
import StaffInviteModal from "../staff/StaffInviteModal";
import InviteLinkFallbackModal from "@/components/staff/InviteLinkFallbackModal";
import { inviteLinkToShare } from "@/components/staff/inviteLinkFallback";
import { useInstance } from "@/hooks/useInstance";
import StaffRoleManagement from "../staff/StaffRoleManagement";
import PositionsManager from "@/components/staff/PositionsManager";
import SegmentedTabs from "./shared/SegmentedTabs";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";
import SetupChecklist from "./shared/SetupChecklist";
import { buildSetupSteps } from "./shared/setupSteps";
import { DashboardTabTransition, PremiumPanel } from "./premium";
import DashboardTabShell from "./shared/DashboardTabShell";
import { TeamSkeleton } from "./TeamSkeleton";

// Relocated here from the Schedule tab — communication belongs with the people
// it concerns, not stacked under the weekly grid. This tab is manager/owner
// only (see getAllowedStaffTabs), the same audience that could moderate chat
// on the old Schedule tab, so the move is RBAC-neutral.
import TeamChatPanel from "./chat/TeamChatPanel";
import AnnouncementComposer from "./chat/AnnouncementComposer";
import EngagementPanel from "./engagement/EngagementPanel";
import TipsByStaffPanel from "@/components/staff/TipsByStaffPanel";
// Who's on the floor tonight — same pulse as Schedule. Self-hides on empty/403
// (timeclock:manage). Thin MVP: role stays in the table; no PIN/rate/payroll.
import LiveFloorBoard from "./schedule/LiveFloorBoard";

// Use types from API
type Staff = StaffAPI.StaffMember;
type PendingInvitation = StaffAPI.StaffInvitation;

export type TeamSubTab = "people" | "positions" | "communication";

interface StaffManagementProps {
  businessId: string;
  // Sub-tab is mirrored onto `?sub=` by the dashboard page so a refresh or a
  // cross-tab deep-link (e.g. the Schedule setup guide → "Add positions")
  // resolves to the right section. Defaults keep the component self-contained.
  subTab?: TeamSubTab;
  onSubTabChange?: (sub: TeamSubTab) => void;
  // Manager/owner gate for the communication tools (chat moderation,
  // announcement composer, engagement authoring). The backend re-checks too.
  canManageCommunication?: boolean;
  // Wave 4: owner or staff with financial permission may see tips-by-staff.
  canViewTips?: boolean;
  // Currency for the tips rollup amounts (ISO 4217).
  currency?: string;
  // Top-level dashboard tab navigator (the page's handleSetActiveTab). Used by
  // the cold-start setup checklist to jump from "Invite your team" → Schedule.
  onNavigate?: (spec: string) => void;
  // Business IANA timezone (R17). Staff/invitation dates render in the
  // business's wall-clock, never the operator device's, falling back to UTC
  // when absent/invalid. Optional so existing callers keep compiling.
  businessTimezone?: string | null;
  /** L5-25: owner wallet for audit actor resolution. */
  ownerAddress?: string | null;
  /** L5-25: owner display name for audit actor resolution. */
  ownerName?: string | null;
}

// Role colors remain constant
const roleLabels = {
  manager: "Manager",
  server: "Server",
  host: "Host",
  kitchen: "Kitchen",
};


/**
 * Header "pending invitations" count (L5-14).
 * Counts only still-open invites: status pending (or omitted) and not past expires_at.
 * The invitation list may still show expired rows for resend; the header must not.
 */
export function countPendingInvitations(
  invites: ReadonlyArray<{
    status?: "pending" | "accepted" | "expired" | "revoked";
    expires_at: string;
  }>,
  now: Date = new Date(),
): number {
  return invites.filter((inv) => {
    const status = inv.status ?? "pending";
    if (status !== "pending") return false;
    const expires = new Date(inv.expires_at);
    if (!Number.isFinite(expires.getTime())) return false;
    return expires.getTime() >= now.getTime();
  }).length;
}

export default function StaffManagement({
  businessId,
  subTab = "people",
  onSubTabChange,
  canManageCommunication = false,
  canViewTips = false,
  currency = "USD",
  onNavigate,
  businessTimezone = null,
  ownerAddress = null,
  ownerName = null,
}: StaffManagementProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const localizeError = useApiErrorMessage();
  const emailOff = useInstance().isOff("email");

  // Owner == not a staff principal (mirrors payroll-management gate).
  const { isStaffUser } = useAuth();
  const isOwner = !isStaffUser;
  // Custom grant/revoke is owner-or-staff:permissions. Owners have empty context
  // perms by design; managers with staff:permissions get the UI via context.
  const {
    permissions: actorPermissions,
    rolePermissions: actorRolePermissions,
  } = useStaffPermissionsContext();
  const canEditPermissions =
    isOwner || hasPerm(actorPermissions, "staff:permissions");
  // Staff actors may only grant perms in their role defaults (BE allowlist).
  // Owners get the full catalog (undefined → unfiltered in the modal).
  const grantablePermissions = isOwner ? null : actorRolePermissions;

  // Operational lock (admin suspend/close) and RBAC access
  const {
    hasAccess,
    loading: accessLoading,
    isError: accessIsError,
  } = useBusinessAccess(businessId);
  // #818: a FAILED access probe is not a lock. Mirror the dashboard
  // shell (page.tsx effectiveAccess): fail open so the roster still loads and
  // the operator never gets a fake lock screen off a transient access-fetch error.
  const effectiveAccess = hasAccess || accessIsError;

  // Communication nested view (Messages / Announcements / Engagement). Kept as
  // local state — deep-linking down to the Communication tab is enough; the
  // nested default is Messages.
  const [commView, setCommView] = useState<
    "messages" | "announcements" | "engagement"
  >("messages");

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.staffManagement.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Resolver for the shared cold-start setup checklist (own namespace so the
  // copy is identical on the Schedule and Team tabs).
  const setupT = useCallback(
    (key: string): string => {
      const result = getTranslation(`dashboardSetup.${key}`, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Positions count drives the "Add positions" step's done state. Shares the
  // ["positions", businessId] key with PositionsManager / ScheduleBuilder, so
  // it dedupes whenever one of those is already mounted.
  const positionsQuery = useQuery({
    queryKey: ["positions", businessId],
    queryFn: () => positionsApi.list(businessId),
    enabled: !accessLoading && effectiveAccess,
    staleTime: 5 * 60 * 1000,
  });
  const positionsCount = positionsQuery.data?.length ?? 0;

  // Translated role labels and descriptions
  const getRoleLabel = useCallback(
    (role: Staff["role"]): string => tString(`roles.${role}.label`),
    [tString],
  );

  const getRoleDescription = useCallback(
    (role: Staff["role"]): string => tString(`roles.${role}.description`),
    [tString],
  );

  const [inviteLoading, setInviteLoading] = useState(false);

  // Search and filter state — staffSearch deep-link seeds the roster filter.
  // L5-15: write back on every change so the URL stays the source of truth.
  const [searchQuery, setSearchQuery] = useState(
    () => searchParams?.get("staffSearch") ?? "",
  );
  const [roleFilter, setRoleFilter] = useState<"all" | Staff["role"]>("all");

  useEffect(() => {
    if (!pathname) return;
    const currentVal = searchParams?.get("staffSearch") ?? "";
    const nextVal = searchQuery.trim();
    if (currentVal === nextVal) return;
    const nextSearch = buildSearchWithParam(
      searchParams?.toString() ?? "",
      "staffSearch",
      nextVal || null,
    );
    const href = nextSearch ? `${pathname}?${nextSearch}` : pathname;
    router.replace(href, { scroll: false });
  }, [searchQuery, pathname, router, searchParams]);

  const { isOpen, onOpen, onClose } = useDisclosure();
  const {
    isOpen: isRoleModalOpen,
    onOpen: onRoleModalOpen,
    onClose: onRoleModalClose,
  } = useDisclosure();
  const {
    isOpen: isRemoveModalOpen,
    onOpen: onRemoveModalOpen,
    onClose: onRemoveModalClose,
    onOpenChange: onRemoveModalOpenChange,
  } = useDisclosure();
  const {
    isOpen: isResendModalOpen,
    onOpen: onResendModalOpen,
    onClose: onResendModalClose,
    onOpenChange: onResendModalOpenChange,
  } = useDisclosure();
  const {
    isOpen: isRevokeModalOpen,
    onOpen: onRevokeModalOpen,
    onClose: onRevokeModalClose,
    onOpenChange: onRevokeModalOpenChange,
  } = useDisclosure();

  // Invite form state
  const [inviteForm, setInviteForm] = useState({
    email: "",
    name: "",
    role: "server" as Staff["role"],
  });

  // Staff management state
  const [selectedStaff, setSelectedStaff] = useState<Staff | null>(null);
  const [staffToRemove, setStaffToRemove] = useState<{
    id: number;
    name: string;
  } | null>(null);
  const [removeLoading, setRemoveLoading] = useState(false);
  const [invitationToResend, setInvitationToResend] = useState<{
    id: number;
    name: string;
  } | null>(null);
  const [inviteFallback, setInviteFallback] = useState<{
    email: string;
    url: string;
  } | null>(null);
  const [resendLoading, setResendLoading] = useState(false);
  const [invitationToRevoke, setInvitationToRevoke] = useState<{
    id: number;
    name: string;
  } | null>(null);
  const [revokeLoading, setRevokeLoading] = useState(false);

  const queryClient = useQueryClient();

  // Single source of truth: the SHARED react-query key that ScheduleBuilder,
  // ApprovalsPanel and TimesheetReview also read. This replaces the old
  // useState + hand-synced setQueryData/invalidate (a dual source that could
  // drift). Mutations here invalidate this key; every consumer refetches together.
  const staffQuery = useQuery({
    queryKey: queryKeys.staff.list(businessId),
    queryFn: () => StaffAPI.getBusinessStaff(businessId),
    enabled: !accessLoading && effectiveAccess,
    staleTime: 60 * 1000,
  });
  const staff: Staff[] = useMemo(
    () => staffQuery.data?.staff ?? [],
    [staffQuery.data],
  );
  const invitations: PendingInvitation[] = useMemo(
    () => staffQuery.data?.pending_invitations ?? [],
    [staffQuery.data],
  );
  const loading = staffQuery.isLoading;

  // Refetch the shared roster (used after an invite/remove/role/invitation change).
  const loadStaffData = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: queryKeys.staff.list(businessId),
    });
  }, [queryClient, businessId]);

  // Surface a load error the same way the old imperative loader did.
  useEffect(() => {
    if (staffQuery.isError) {
      toast.error(tString("error.loadStaffData"));
    }
  }, [staffQuery.isError, tString]);

  // Handle staff invitation
  const handleInviteStaff = async () => {
    if (!inviteForm.email || !inviteForm.name || !inviteForm.role) {
      toast.error(tString("error.fillAllFields"));
      return;
    }

    try {
      setInviteLoading(true);
      const result = await StaffAPI.inviteStaff(businessId, inviteForm);

      const inviteLink = inviteLinkToShare(result, emailOff);
      if (inviteLink) {
        // P2-21: invitation created but the email did not go out — hand the
        // inviter a copyable link instead of celebrating a phantom send.
        setInviteFallback({
          email: inviteForm.email,
          url: inviteLink,
        });
      } else {
        toast.success(
          tString("success.invitationSentBody").replace(
            "{email}",
            inviteForm.email,
          ),
        );
      }
      setInviteForm({ email: "", name: "", role: "server" });
      onClose();
      loadStaffData(); // Reload to show new invitation
    } catch (error) {
      console.error("Error inviting staff:", error);
      // Code-first localization via useApiErrorMessage (falls back to a
      // localized generic when the envelope has no code / catalog entry).
      toast.error(localizeError(error));
    } finally {
      setInviteLoading(false);
    }
  };

  // Handle staff removal - open confirmation modal
  const handleRemoveStaff = (staffId: number, staffName: string) => {
    setStaffToRemove({ id: staffId, name: staffName });
    onRemoveModalOpen();
  };

  // Confirm staff removal
  const confirmRemoveStaff = async () => {
    if (!staffToRemove) return;

    try {
      setRemoveLoading(true);
      await StaffAPI.removeStaff(businessId, staffToRemove.id);
      toast.success(tString("success.staffRemoved"));
      loadStaffData(); // Reload staff list
      onRemoveModalClose();
      setStaffToRemove(null);
    } catch (error) {
      console.error("Error removing staff:", error);
      toast.error(tString("error.removeStaff"));
    } finally {
      setRemoveLoading(false);
    }
  };

  // Handle resend invitation
  const handleResendInvitation = async (
    invitationId: number,
    staffName: string,
  ) => {
    setInvitationToResend({ id: invitationId, name: staffName });
    onResendModalOpen();
  };

  const confirmResendInvitation = async () => {
    if (!invitationToResend) return;
    try {
      setResendLoading(true);
      const result = await StaffAPI.resendInvitation(
        businessId,
        invitationToResend.id,
      );
      const inviteLink = inviteLinkToShare(result, emailOff);
      if (inviteLink) {
        const invitationEmail =
          invitations.find((i) => i.id === invitationToResend.id)?.email ?? "";
        setInviteFallback({
          email: invitationEmail,
          url: inviteLink,
        });
      } else {
        toast.success(tString("success.invitationResent"));
      }
      loadStaffData(); // Reload to show updated expiry
      onResendModalClose();
      setInvitationToResend(null);
    } catch (error) {
      console.error("Error resending invitation:", error);
      toast.error(localizeError(error));
    } finally {
      setResendLoading(false);
    }
  };

  // Handle revoke invitation — open confirmation modal.
  const handleRevokeInvitation = (invitationId: number, staffName: string) => {
    setInvitationToRevoke({ id: invitationId, name: staffName });
    onRevokeModalOpen();
  };

  const confirmRevokeInvitation = async () => {
    if (!invitationToRevoke) return;
    try {
      setRevokeLoading(true);
      await StaffAPI.revokeInvitation(businessId, invitationToRevoke.id);
      toast.success(tString("success.invitationRevoked"));
      loadStaffData(); // Reload so the row reflects the revoked/gone state.
      onRevokeModalClose();
      setInvitationToRevoke(null);
    } catch (error) {
      console.error("Error revoking invitation:", error);
      toast.error(localizeError(error));
    } finally {
      setRevokeLoading(false);
    }
  };

  // Handle staff management (simplified)
  const handleManageStaff = (staff: Staff) => {
    setSelectedStaff(staff);
    onRoleModalOpen();
  };

  // Handle invite modal
  const handleInviteModalOpen = () => {
    setInviteForm({
      email: "",
      name: "",
      role: "server",
    });
    onOpen();
  };

  const formatDate = (dateString: string) => {
    return formatBusinessDateTime(dateString, currentLocale, businessTimezone, DATE_SHORT);
  };

  const isInvitationExpired = (expiresAt: string) => {
    return new Date(expiresAt) < new Date();
  };

  // Filter and search logic
  const filteredStaff = React.useMemo(() => {
    if (!searchQuery.trim() && roleFilter === "all") {
      return staff;
    }

    const query = searchQuery.toLowerCase().trim();

    return staff.filter((member) => {
      // Text search across name, email, and role. Match BOTH the localized role
      // label (what the operator sees) and the English fallback so a Spanish
      // operator searching "cocina"/"mesero" matches by role (audit L6 #12).
      const nameMatches = !query || member.name.toLowerCase().includes(query);
      const emailMatches = !query || member.email.toLowerCase().includes(query);
      const roleMatches =
        !query ||
        getRoleLabel(member.role).toLowerCase().includes(query) ||
        (roleLabels[member.role]?.toLowerCase().includes(query) ?? false);

      const textMatches = nameMatches || emailMatches || roleMatches;

      // Role filter
      const roleFilterMatches =
        roleFilter === "all" || member.role === roleFilter;

      return textMatches && roleFilterMatches;
    });
  }, [staff, searchQuery, roleFilter, getRoleLabel]);

  const filteredInvitations = React.useMemo(() => {
    if (!searchQuery.trim() && roleFilter === "all") {
      return invitations;
    }

    const query = searchQuery.toLowerCase().trim();

    return invitations.filter((invitation) => {
      // Text search across name, email, and role (localized label + English
      // fallback) — see filteredStaff above (audit L6 #12).
      const nameMatches =
        !query || invitation.name.toLowerCase().includes(query);
      const emailMatches =
        !query || invitation.email.toLowerCase().includes(query);
      const roleMatches =
        !query ||
        getRoleLabel(invitation.role).toLowerCase().includes(query) ||
        (roleLabels[invitation.role]?.toLowerCase().includes(query) ?? false);

      const textMatches = nameMatches || emailMatches || roleMatches;

      // Role filter
      const roleFilterMatches =
        roleFilter === "all" || invitation.role === roleFilter;

      return textMatches && roleFilterMatches;
    });
  }, [invitations, searchQuery, roleFilter, getRoleLabel]);

  const clearSearch = () => {
    setSearchQuery("");
    setRoleFilter("all");
  };

  // L5-15: filtered empty framing when search/role filter zeros the list.
  const isStaffFiltered =
    searchQuery.trim().length > 0 || roleFilter !== "all";
  const staffNoMatchesTitle = (() => {
    const raw = getTranslation("urlState.staffNoMatchesTitle", currentLocale, {
      query: searchQuery.trim(),
    });
    const s = Array.isArray(raw) ? raw[0] : (raw as string);
    return s && s !== "urlState.staffNoMatchesTitle"
      ? s
      : tString("table.noStaffFound");
  })();
  const staffNoMatchesBody = (() => {
    const raw = getTranslation("urlState.staffNoMatchesBody", currentLocale);
    const s = Array.isArray(raw) ? raw[0] : (raw as string);
    return s && s !== "urlState.staffNoMatchesBody"
      ? s
      : tString("table.adjustSearch");
  })();

  // If the business is suspended or closed, show lockdown (after all hooks). #818: only a
  // SUCCESSFUL access read saying "no access" may lock — a failed probe must not.
  if (!accessLoading && !effectiveAccess) {
    return (
      <DashboardLockedTabView
        title={tString("title")}
        subtitle={tString("subtitle")}
        businessId={businessId}
      />
    );
  }

  const teamTabs = [
    { key: "people", label: tString("tabs.people"), icon: Users },
    { key: "positions", label: tString("tabs.positions"), icon: Briefcase },
    {
      key: "communication",
      label: tString("tabs.communication"),
      icon: MessagesSquare,
    },
  ];

  const commTabs = [
    { key: "messages", label: tString("communication.messages") },
    ...(canManageCommunication
      ? [
          {
            key: "announcements",
            label: tString("communication.announcements"),
          },
          { key: "engagement", label: tString("communication.engagement") },
        ]
      : []),
  ];

  // Cold start: no teammates yet (neither active staff nor pending invites).
  // The People sub-tab swaps its overview/search/tables for the guided setup
  // path until the first teammate exists.
  const teamEmpty = staff.length === 0 && invitations.length === 0;
  const peopleSetupSteps = buildSetupSteps(
    setupT,
    { hasPositions: positionsCount > 0, hasTeam: !teamEmpty },
    {
      addPositions: () => onSubTabChange?.("positions"),
      invite: handleInviteModalOpen,
      goToSchedule: () => onNavigate?.("schedule"),
    },
  );
  // "Active this week" used to live in a second stat strip below the hero that
  // repeated Active staff + Pending invitations; the hero row is now the single
  // home for team stats.
  const oneWeekAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
  const activeThisWeek = staff.filter((member) => {
    if (member.is_active === false) return false;
    if (!member.last_login_at) return false;
    const lastLogin = new Date(member.last_login_at).getTime();
    return Number.isFinite(lastLogin) && lastLogin >= oneWeekAgo;
  }).length;
  const pendingInvitationCount = countPendingInvitations(invitations);
  const shellStats = teamEmpty
    ? []
    : [
        {
          label: tString("shell.activeStaff"),
          value: staff.filter((member) => member.is_active !== false).length,
        },
        {
          // "1 Pending invites" reads as a bug — pick the label by count.
          // L5-14: count only still-open pending invites (exclude expired).
          label: tString(
            pendingInvitationCount === 1
              ? "shell.pendingInvitation"
              : "shell.pendingInvitations",
          ),
          value: pendingInvitationCount,
        },
        { label: tString("shell.positions"), value: positionsCount },
        { label: tString("overview.activeThisWeek"), value: activeThisWeek },
      ];

  return (
    <DashboardTabShell
      // L5-21: Comunicación (chat) must not wait on staffQuery. Only people /
      // positions need the staff list skeleton; chat owns its own spinner.
      // #818: never pin the skeleton over a roster we already have (the shared
      // staff key is warm after a Schedule visit) or over a load error — the
      // skeleton is only honest while a first load is genuinely in flight.
      loading={
        (subTab === "people" || subTab === "positions") &&
        !staffQuery.isError &&
        !staffQuery.data &&
        (loading || accessLoading)
          ? <TeamSkeleton />
          : null
      }
      header={{
        title: tString("title"),
        subtitle: tString("subtitle"),
        stats: shellStats,
        /* Invite CTA stays visible even at cold start — the setup checklist
           guides, but the primary action must never be unreachable. */
        actions:
          subTab === "people" ? (
            <Button
              radius="full"
              className={btnPrimaryNextUI}
              onPress={handleInviteModalOpen}
              startContent={<Plus className="w-4 h-4" />}
            >
              {tString("buttons.inviteStaff")}
            </Button>
          ) : undefined,
      }}
      tabs={{
        items: teamTabs,
        activeKey: subTab,
        onChange: (key) => onSubTabChange?.(key as TeamSubTab),
        ariaLabel: tString("title"),
      }}
    >
      <DashboardTabTransition tabKey={subTab}>
        <div className="space-y-5">
          {subTab === "people" && staffQuery.isError && !staffQuery.data && (
            /* #818: a failed roster load is an error, not a cold start. Say so
               honestly and offer a retry instead of the setup checklist. */
            <PremiumPanel className="p-4" withTexture={false}>
              <div className="flex flex-col items-start gap-3">
                <p className="text-sm text-ink-600">
                  {tString("error.loadStaffData")}
                </p>
                <Button
                  radius="full"
                  size="sm"
                  variant="bordered"
                  onPress={() => {
                    void staffQuery.refetch();
                  }}
                >
                  {tString("error.retry")}
                </Button>
              </div>
            </PremiumPanel>
          )}
          {subTab === "people" &&
            !(staffQuery.isError && !staffQuery.data) &&
            (teamEmpty ? (
              <SetupChecklist
                title={setupT("title")}
                subtitle={setupT("subtitle")}
                steps={peopleSetupSteps}
              />
            ) : (
              <>
                <StaffSearchFilter
                  searchQuery={searchQuery}
                  setSearchQuery={setSearchQuery}
                  roleFilter={roleFilter}
                  setRoleFilter={setRoleFilter}
                  filteredStaff={filteredStaff}
                  filteredInvitations={filteredInvitations}
                  getRoleLabel={getRoleLabel}
                  tString={tString}
                  clearSearch={clearSearch}
                />

                {/* Dueño pulse: who's clocked in / late / no-show right now.
                    Reuses the Schedule live-floor board — money-free, no
                    timeclock epic on this tab (#164 / #246 thin MVP). */}
                <LiveFloorBoard
                  businessId={businessId}
                  businessTimezone={businessTimezone}
                />

                <StaffTable
                  staff={filteredStaff}
                  getRoleLabel={getRoleLabel}
                  getRoleDescription={getRoleDescription}
                  tString={tString}
                  formatDate={formatDate}
                  handleUpdateRole={handleManageStaff}
                  handleRemoveStaff={handleRemoveStaff}
                  onInviteClick={handleInviteModalOpen}
                  isFiltered={isStaffFiltered}
                  filterQuery={searchQuery.trim()}
                  onClearFilter={clearSearch}
                  filteredEmptyTitle={staffNoMatchesTitle}
                  filteredEmptyBody={staffNoMatchesBody}
                />

                <InvitationTable
                  invitations={filteredInvitations}
                  businessId={businessId}
                  getRoleLabel={getRoleLabel}
                  tString={tString}
                  formatDate={formatDate}
                  isInvitationExpired={isInvitationExpired}
                  handleResendInvitation={handleResendInvitation}
                  handleRevokeInvitation={handleRevokeInvitation}
                />

                {canViewTips ? (
                  <TipsByStaffPanel
                    businessId={businessId}
                    currency={currency}
                  />
                ) : null}
              </>
            ))}

          {subTab === "positions" && (
            <PositionsManager
              businessId={businessId}
              labels={{
                title: tString("positions.title"),
                subtitle: tString("positions.subtitle"),
                addButton: tString("positions.addButton"),
                namePlaceholder: tString("positions.namePlaceholder"),
                departmentPlaceholder: tString(
                  "positions.departmentPlaceholder",
                ),
                departmentHint: tString("positions.departmentHint"),
                suggestionsLabel: tString("positions.suggestionsLabel"),
                suggestionNames: tString("positions.suggestionNames"),
                empty: tString("positions.empty"),
                retire: tString("positions.retire"),
                retireConfirmTitle: tString("positions.retireConfirmTitle"),
                retireConfirmDescription: tString(
                  "positions.retireConfirmDescription",
                ),
                retireConfirmAction: tString("positions.retireConfirmAction"),
                saved: tString("positions.saved"),
                saveError: tString("positions.saveError"),
                duplicateName: tString("positions.duplicateName"),
                retired: tString("positions.retired"),
                retireError: tString("positions.retireError"),
                loadError: tString("positions.loadError"),
                retry: tString("positions.retry"),
                loading: tString("positions.loading"),
              }}
            />
          )}

          {subTab === "communication" && (
            <PremiumPanel className="space-y-6 p-5" withTexture={false}>
              <div>
                <p className="text-sm text-ink-600">
                  {tString("communication.subtitle")}
                </p>
              </div>
              <SegmentedTabs
                tabs={commTabs}
                activeKey={commView}
                onChange={(key) => setCommView(key as typeof commView)}
                size="sm"
                ariaLabel={tString("tabs.communication")}
              />
              {commView === "messages" && (
                <TeamChatPanel
                  businessId={businessId}
                  canModerate={canManageCommunication}
                />
              )}
              {commView === "announcements" && canManageCommunication && (
                <AnnouncementComposer businessId={businessId} />
              )}
              {commView === "engagement" && canManageCommunication && (
                <EngagementPanel businessId={businessId} />
              )}
            </PremiumPanel>
          )}
        </div>
      </DashboardTabTransition>

      {/* Staff Invitation Modal */}
      <StaffInviteModal
        isOpen={isOpen}
        onClose={onClose}
        inviteForm={inviteForm}
        setInviteForm={setInviteForm}
        inviteLoading={inviteLoading}
        getRoleLabel={getRoleLabel}
        getRoleDescription={getRoleDescription}
        tString={tString}
        handleInviteStaff={handleInviteStaff}
      />

      <InviteLinkFallbackModal
        isOpen={inviteFallback !== null}
        email={inviteFallback?.email ?? ""}
        invitationUrl={inviteFallback?.url ?? ""}
        onClose={() => setInviteFallback(null)}
        tString={tString}
      />

      {/* Staff Role Management Modal */}
      {selectedStaff && (
        <StaffRoleManagement
          isOpen={isRoleModalOpen}
          onClose={onRoleModalClose}
          staffMember={selectedStaff}
          businessId={businessId}
          onStaffUpdated={loadStaffData}
          isOwner={isOwner}
          canEditPermissions={canEditPermissions}
          grantablePermissions={grantablePermissions}
          teamMembers={staff.map((s) => ({
            id: s.id,
            name: s.name,
            email: s.email,
          }))}
          ownerAddresses={ownerAddress ? [ownerAddress] : []}
          ownerLabel={ownerName}
        />
      )}

      {/* Soft-remove confirmation — BE SoftDelete is reversible via re-invite.
          Reuses ConfirmationModal (L5-24 Path A); prior copy falsely claimed
          permanent destroy. */}
      <ConfirmationModal
        isOpen={isRemoveModalOpen}
        onOpenChange={() => {
          if (isRemoveModalOpen) {
            setStaffToRemove(null);
          }
          onRemoveModalOpenChange();
        }}
        isDanger
        isLoading={removeLoading}
        title={tString("removeModal.title")}
        description={
          staffToRemove
            ? `${tString("confirmRemove").replace("{name}", staffToRemove.name)} ${tString("removeModal.description")}`
            : tString("removeModal.description")
        }
        confirmLabel={tString("buttons.removeStaff")}
        cancelLabel={tString("buttons.cancel")}
        onConfirm={() => confirmRemoveStaff()}
      />

      {/* L5-17: resend/revoke use ConfirmationModal (same as remove). */}
      <ConfirmationModal
        isOpen={isResendModalOpen}
        onOpenChange={() => {
          if (isResendModalOpen) {
            setInvitationToResend(null);
          }
          onResendModalOpenChange();
        }}
        isLoading={resendLoading}
        title={tString("resendModal.title")}
        description={
          invitationToResend
            ? `${tString("confirmResend").replace("{name}", invitationToResend.name)} ${tString("resendModal.description")}`
            : tString("resendModal.description")
        }
        confirmLabel={tString("resendModal.confirmButton")}
        cancelLabel={tString("buttons.cancel")}
        onConfirm={() => confirmResendInvitation()}
      />

      <ConfirmationModal
        isOpen={isRevokeModalOpen}
        onOpenChange={() => {
          if (isRevokeModalOpen) {
            setInvitationToRevoke(null);
          }
          onRevokeModalOpenChange();
        }}
        isDanger
        isLoading={revokeLoading}
        title={tString("revokeModal.title")}
        description={
          invitationToRevoke
            ? `${tString("confirmRevoke").replace("{name}", invitationToRevoke.name)} ${tString("revokeModal.description")}`
            : tString("revokeModal.description")
        }
        confirmLabel={tString("revokeModal.confirmButton")}
        cancelLabel={tString("buttons.cancel")}
        onConfirm={() => confirmRevokeInvitation()}
      />
    </DashboardTabShell>
  );
}
