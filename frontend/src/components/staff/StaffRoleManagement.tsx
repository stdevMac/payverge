"use client";

import React, { useState, useEffect, useCallback, useMemo } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Select,
  SelectItem,
  Textarea,
  Chip,
  Card,
  CardBody,
  CardHeader,
  Accordion,
  AccordionItem,
  Spinner,
  useDisclosure,
} from "@nextui-org/react";
import { Shield, UserCheck, UserX, History, KeyRound, Plus, Minus } from "lucide-react";
import { toast } from "react-hot-toast";
import { useQueryClient } from "@tanstack/react-query";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import * as RBACAPI from "../../api/rbac";
import type { StaffPermissions } from "../../api/rbac";
import { SENSITIVE_PERMISSIONS } from "../../api/rbac";
import { queryKeys } from "@/api/queryKeys";
import {
  PERMISSION_CATEGORIES,
  permissionDescription,
  permissionLabel,
} from "@/constants/permissions";
import StaffCompensationSection from "./StaffCompensationSection";
import { formatStaffActor } from "./formatStaffActor";
import { SkeletonCard } from "@/components/ui/skeletons";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";

interface StaffRoleManagementProps {
  isOpen: boolean;
  onClose: () => void;
  staffMember: {
    id: number;
    name: string;
    email: string;
    role: "kitchen" | "host" | "server" | "manager";
    is_active?: boolean;
  };
  businessId: string;
  onStaffUpdated: () => void;
  isOwner?: boolean;
  /** When true, show custom grant/revoke UI (staff:permissions actors). */
  canEditPermissions?: boolean;
  /**
   * Permissions the current actor may grant (BE allowlists staff actors to
   * their own role defaults). Owners pass undefined → full catalog.
   */
  grantablePermissions?: readonly string[] | null;
  /**
   * L5-25: full team roster so audit actors resolve beyond the subject.
   * Match by email (case-insensitive) or exact name.
   */
  teamMembers?: readonly { id: number; name: string; email: string }[];
  /** Business owner wallet(s) — any 0x match shows ownerLabel. */
  ownerAddresses?: readonly string[];
  /** Display name for owner wallet actors (defaults to localized "Owner"). */
  ownerLabel?: string | null;
}



// Role descriptions will be loaded from translations

export default function StaffRoleManagement({
  isOpen,
  onClose,
  staffMember,
  businessId,
  onStaffUpdated,
  isOwner = false,
  canEditPermissions = false,
  grantablePermissions = null,
  teamMembers = [],
  ownerAddresses = [],
  ownerLabel = null,
}: StaffRoleManagementProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const queryClient = useQueryClient();

  const [loading, setLoading] = useState(false);
  const [newRole, setNewRole] = useState<
    "kitchen" | "host" | "server" | "manager"
  >(staffMember.role);
  const [roleChangeReason, setRoleChangeReason] = useState("");
  const [deactivateReason, setDeactivateReason] = useState("");
  const [auditLogs, setAuditLogs] = useState<RBACAPI.RBACAuditLog[]>([]);
  const [permState, setPermState] = useState<StaffPermissions | null>(null);
  const [permsLoading, setPermsLoading] = useState(false);
  const [permActionLoading, setPermActionLoading] = useState<string | null>(
    null,
  );
  const [permissionReason, setPermissionReason] = useState("");

  // Sensitive grant/revoke/deny changes route through a styled confirmation
  // modal (replaces native window.confirm, which browsers can silently
  // suppress on repeat, auto-cancelling the safety gate). The pending action
  // is captured verbatim so we run the exact same mutation on confirm.
  const {
    isOpen: isConfirmOpen,
    onOpen: openConfirm,
    onOpenChange: onConfirmOpenChange,
  } = useDisclosure();
  const [pendingConfirm, setPendingConfirm] = useState<{
    permission: string;
    action: string;
    run: () => Promise<void>;
  } | null>(null);

  const canManageCustomPerms = isOwner || canEditPermissions;

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Translation helper with parameter substitution
  const tString = useCallback(
    (key: string, params?: Record<string, string>): string => {
      const fullKey = `businessDashboard.dashboard.staffManagement.rbac.${key}`;
      const rawResult = getTranslation(fullKey, currentLocale);
      let result = Array.isArray(rawResult)
        ? rawResult[0] || key
        : (rawResult as string);

      if (params) {
        Object.entries(params).forEach(([paramKey, paramValue]) => {
          result = result.replace(new RegExp(`{${paramKey}}`, "g"), paramValue);
        });
      }

      return result;
    },
    [currentLocale],
  );

  // Translated, human-readable role label (e.g. 'Kitchen Staff' / 'Manager')
  // instead of the raw lowercase enum. Reuses the same keys the role <Select>
  // already uses so the summary lines stay consistent with the dropdown.
  const roleLabel = (role: StaffRoleManagementProps["staffMember"]["role"]): string =>
    tString(`changeRole.roles.${role}.title`);

  // L5-23: human labels for permission slugs; raw slug fallback for unknown.
  const permT = useCallback(
    (key: string) => {
      const v = getTranslation(key, currentLocale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [currentLocale],
  );
  const formatPermission = useCallback(
    (perm: string) => permissionLabel(perm, permT),
    [permT],
  );
  // L5-23: descriptions exist in rbacPermissions.*.description — surface them.
  const formatPermissionDesc = useCallback(
    (perm: string) => permissionDescription(perm, permT),
    [permT],
  );

  // L5-25: resolve changed_by against the full team roster + owner wallet,
  // not only the subject of this modal (see formatStaffActor pure helper).
  const formatActor = useCallback(
    (changedBy: string) =>
      formatStaffActor(changedBy, {
        subject: {
          id: staffMember.id,
          name: staffMember.name,
          email: staffMember.email || "",
        },
        teamMembers,
        ownerAddresses,
        ownerLabel,
        ownerFallback: tString("recentActivity.ownerActor") || "Owner",
      }),
    [
      ownerAddresses,
      ownerLabel,
      staffMember.email,
      staffMember.id,
      staffMember.name,
      teamMembers,
      tString,
    ],
  );

  const formatAuditAction = useCallback(
    (action: string) => {
      const key = `recentActivity.actions.${action}`;
      const label = tString(key);
      // Fallback to raw action when translation is missing (echoes key).
      if (!label || label === key || label.includes("recentActivity.actions.")) {
        return action;
      }
      return label;
    },
    [tString],
  );

  const loadAuditLog = useCallback(async () => {
    try {
      const auditData = await RBACAPI.getStaffAuditLog(
        businessId,
        staffMember.id.toString(),
        5,
      );
      setAuditLogs(auditData.audit_logs);
    } catch (error) {
      console.error("Failed to load audit log:", error);
    }
  }, [businessId, staffMember.id]);

  const loadPermissions = useCallback(async () => {
    if (!canManageCustomPerms) return;
    setPermsLoading(true);
    try {
      const data = await RBACAPI.getStaffPermissions(
        businessId,
        String(staffMember.id),
      );
      setPermState(data);
    } catch (error) {
      console.error("Failed to load staff permissions:", error);
      toast.error(tString("customPermissions.loadFailed"));
      setPermState(null);
    } finally {
      setPermsLoading(false);
    }
  }, [businessId, staffMember.id, canManageCustomPerms, tString]);

  // Load audit log (+ permissions when allowed) when modal opens
  useEffect(() => {
    if (isOpen) {
      loadAuditLog();
      setNewRole(staffMember.role);
      setPermissionReason("");
      void loadPermissions();
    } else {
      setPermState(null);
    }
  }, [isOpen, staffMember.role, loadAuditLog, loadPermissions]);

  // Reset role when staff member changes
  useEffect(() => {
    setNewRole(staffMember.role);
  }, [staffMember.role]);

  const effectivePermSet = useMemo(
    () => new Set(permState?.permissions ?? []),
    [permState],
  );
  const rolePermSet = useMemo(
    () => new Set(permState?.role_permissions ?? []),
    [permState],
  );
  const customGrantSet = useMemo(
    () => new Set(permState?.custom_grants ?? []),
    [permState],
  );
  const customDenySet = useMemo(
    () => new Set(permState?.custom_denies ?? []),
    [permState],
  );

  // Gate a sensitive permission mutation behind the styled confirmation modal.
  // Non-sensitive permissions run immediately; sensitive ones stash the exact
  // mutation (`run`) plus the permission/action for the confirm copy, then only
  // execute once the operator confirms in the modal.
  const runWithSensitiveConfirm = (
    permission: string,
    action: string,
    run: () => Promise<void>,
  ): void => {
    if (!SENSITIVE_PERMISSIONS.has(permission)) {
      void run();
      return;
    }
    setPendingConfirm({ permission, action, run });
    openConfirm();
  };

  const handleConfirmSensitive = () => {
    const pending = pendingConfirm;
    if (!pending) return;
    setPendingConfirm(null);
    void pending.run();
  };

  // Owners may grant any catalog permission; staff actors only those in their
  // role defaults (matches BE GrantCustomPermission allowlist).
  const grantableSet = useMemo(() => {
    if (isOwner || grantablePermissions == null) return null;
    return new Set(grantablePermissions);
  }, [isOwner, grantablePermissions]);

  const catalogByCategory = useMemo(() => {
    const entries = Object.entries(PERMISSION_CATEGORIES).map(
      ([category, perms]) => {
        const visible = grantableSet
          ? perms.filter((p) => grantableSet.has(p))
          : [...perms];
        return [category, visible] as const;
      },
    );
    return entries.filter(([, perms]) => perms.length > 0);
  }, [grantableSet]);

  const refreshPermissionsAfterMutation = async () => {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.staff.permissions(businessId, staffMember.id),
    });
    await loadPermissions();
  };

  const handleGrantPermission = (permission: string) => {
    if (grantableSet && !grantableSet.has(permission)) {
      toast.error(tString("customPermissions.grantNotAllowed"));
      return;
    }
    runWithSensitiveConfirm(
      permission,
      tString("customPermissions.grant"),
      async () => {
        setPermActionLoading(`grant:${permission}`);
        try {
          await RBACAPI.grantCustomPermission(
            businessId,
            String(staffMember.id),
            {
              permission,
              reason: permissionReason.trim() || undefined,
            },
          );
          toast.success(tString("customPermissions.grantSuccess"));
          setPermissionReason("");
          await refreshPermissionsAfterMutation();
        } catch (error) {
          console.error("Failed to grant permission:", error);
          toast.error(tString("customPermissions.grantFailed"));
        } finally {
          setPermActionLoading(null);
        }
      },
    );
  };

  const handleRevokePermission = (permission: string) => {
    // Only custom grants may be revoked — role defaults stay with the role.
    if (!customGrantSet.has(permission)) return;

    runWithSensitiveConfirm(
      permission,
      tString("customPermissions.revoke"),
      async () => {
        setPermActionLoading(`revoke:${permission}`);
        try {
          await RBACAPI.revokeCustomPermission(
            businessId,
            String(staffMember.id),
            {
              permission,
              reason: permissionReason.trim() || undefined,
            },
          );
          toast.success(tString("customPermissions.revokeSuccess"));
          setPermissionReason("");
          await refreshPermissionsAfterMutation();
        } catch (error) {
          console.error("Failed to revoke permission:", error);
          toast.error(tString("customPermissions.revokeFailed"));
        } finally {
          setPermActionLoading(null);
        }
      },
    );
  };

  const handleDenyPermission = (permission: string) => {
    runWithSensitiveConfirm(
      permission,
      tString("customPermissions.deny"),
      async () => {
        setPermActionLoading(`deny:${permission}`);
        try {
          await RBACAPI.denyPermission(businessId, String(staffMember.id), {
            permission,
            reason: permissionReason.trim() || undefined,
          });
          toast.success(tString("customPermissions.denySuccess"));
          setPermissionReason("");
          await refreshPermissionsAfterMutation();
        } catch (error) {
          console.error("Failed to deny permission:", error);
          toast.error(tString("customPermissions.denyFailed"));
        } finally {
          setPermActionLoading(null);
        }
      },
    );
  };

  const handleRemoveDeny = (permission: string) => {
    runWithSensitiveConfirm(
      permission,
      tString("customPermissions.clearDeny"),
      async () => {
        setPermActionLoading(`clear-deny:${permission}`);
        try {
          await RBACAPI.removePermissionDeny(businessId, String(staffMember.id), {
            permission,
            reason: permissionReason.trim() || undefined,
          });
          toast.success(tString("customPermissions.clearDenySuccess"));
          setPermissionReason("");
          await refreshPermissionsAfterMutation();
        } catch (error) {
          console.error("Failed to clear permission deny:", error);
          toast.error(tString("customPermissions.clearDenyFailed"));
        } finally {
          setPermActionLoading(null);
        }
      },
    );
  };

  const handleRoleChange = async () => {
    if (newRole === staffMember.role) {
      toast.error(tString("errors.selectDifferentRole"));
      return;
    }

    setLoading(true);
    try {
      await RBACAPI.changeStaffRole(businessId, staffMember.id.toString(), {
        // Send only the operator's own words; omit a hardcoded English default so
        // the audit ledger doesn't accumulate locale-unstable machine strings
        // (the server records the role delta regardless).
        new_role: newRole,
        reason: roleChangeReason.trim(),
      });

      // Role defaults change the effective set; drop cached permissions for the target.
      await queryClient.invalidateQueries({
        queryKey: queryKeys.staff.permissions(businessId, staffMember.id),
      });

      toast.success(tString("success.roleUpdated"));
      onStaffUpdated();
      onClose();
    } catch (error) {
      console.error("Failed to change role:", error);
      toast.error(tString("errors.changeRoleFailed"));
    } finally {
      setLoading(false);
    }
  };

  // Soft-deactivate (revokes access; reactivate restores it). Reason stays on
  // the form; ConfirmationModal gates the submit so a single danger click does
  // not fire the API (L5-24 Path B).
  const {
    isOpen: isDeactivateConfirmOpen,
    onOpen: openDeactivateConfirm,
    onOpenChange: onDeactivateConfirmOpenChange,
  } = useDisclosure();

  const requestDeactivateStaff = () => {
    if (!deactivateReason.trim()) {
      toast.error(tString("errors.provideDeactivationReason"));
      return;
    }
    openDeactivateConfirm();
  };

  const confirmDeactivateStaff = async () => {
    if (!deactivateReason.trim()) {
      toast.error(tString("errors.provideDeactivationReason"));
      return;
    }

    setLoading(true);
    try {
      await RBACAPI.deactivateStaff(businessId, staffMember.id.toString(), {
        reason: deactivateReason,
      });

      toast.success(tString("success.staffDeactivated"));
      onStaffUpdated();
      onClose();
    } catch (error) {
      console.error("Failed to deactivate staff:", error);
      toast.error(tString("errors.deactivateFailed"));
    } finally {
      setLoading(false);
    }
  };

  const handleReactivateStaff = async () => {
    setLoading(true);
    try {
      await RBACAPI.reactivateStaff(businessId, staffMember.id.toString(), {
        // Omit a hardcoded English reason from the audit ledger (see role change).
        reason: "",
      });

      toast.success(tString("success.staffReactivated"));
      onStaffUpdated();
      onClose();
    } catch (error) {
      console.error("Failed to reactivate staff:", error);
      toast.error(tString("errors.reactivateFailed"));
    } finally {
      setLoading(false);
    }
  };

  const formatDate = (dateString: string) => {
    // Route through the operator locale so es/es-AR audit timestamps format with
    // the localized date+time convention instead of the browser's en-US default.
    return new Intl.DateTimeFormat(intlLocaleFor(locale), {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }).format(new Date(dateString));
  };

  const getActionColor = (action: string) => {
    switch (action) {
      case "role_changed":
        return "primary";
      case "staff_deactivated":
        return "danger";
      case "staff_reactivated":
        return "success";
      case "staff_created":
        return "default";
      default:
        return "default";
    }
  };

  const getRoleColor = (role: string) => {
    switch (role) {
      case "manager":
        return "primary";
      case "server":
        return "success";
      case "host":
        return "warning";
      case "kitchen":
        return "secondary";
      default:
        return "default";
    }
  };

  return (
    <>
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="3xl"
      scrollBehavior="inside"
      classNames={{
        base: "max-h-[90vh]",
        body: "py-6",
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <Shield className="w-5 h-5" />
            <span>
              {tString("title")} - {staffMember.name}
            </span>
          </div>
          <p className="text-sm text-gray-500 font-normal">
            {staffMember.email} • {tString("labels.currentRole")}{" "}
            {roleLabel(staffMember.role)}
          </p>
        </ModalHeader>

        <ModalBody>
          {loading && (
            <div role="status" aria-live="polite" className="space-y-4 py-2">
              <SkeletonCard lines={2} hasHeader />
              <SkeletonCard lines={3} hasHeader />
            </div>
          )}

          {!loading && (
            <div className="space-y-6">
              {/* Current Role Display */}
              <Card>
                <CardHeader>
                  <h4 className="text-lg font-semibold">
                    {tString("currentRoleHeader")}
                  </h4>
                </CardHeader>
                <CardBody>
                  <div className="flex items-start gap-4">
                    <Chip
                      size="lg"
                      variant="flat"
                      color={getRoleColor(staffMember.role)}
                    >
                      {roleLabel(staffMember.role)}
                    </Chip>
                    <div className="flex-1">
                      <p className="text-sm text-gray-600">
                        {tString(`roleDescriptions.${staffMember.role}`)}
                      </p>
                    </div>
                  </div>
                </CardBody>
              </Card>

              {/* Role Change */}
              <Card>
                <CardHeader>
                  <h4 className="text-lg font-semibold">
                    {tString("changeRole.title")}
                  </h4>
                </CardHeader>
                <CardBody>
                  <div className="space-y-4">
                    <Select
                      label={tString("changeRole.label")}
                      selectedKeys={new Set([newRole])}
                      onSelectionChange={(keys) =>
                        setNewRole(Array.from(keys)[0] as any)
                      }
                      className="w-full"
                      placeholder={tString("changeRole.placeholder")}
                      disallowEmptySelection
                    >
                      <SelectItem
                        key="kitchen"
                        value="kitchen"
                        textValue={tString("changeRole.roles.kitchen.title")}
                      >
                        <div className="flex flex-col">
                          <span className="font-medium">
                            {tString("changeRole.roles.kitchen.title")}
                          </span>
                          <span className="text-xs text-gray-500">
                            {tString("changeRole.roles.kitchen.description")}
                          </span>
                        </div>
                      </SelectItem>
                      <SelectItem
                        key="host"
                        value="host"
                        textValue={tString("changeRole.roles.host.title")}
                      >
                        <div className="flex flex-col">
                          <span className="font-medium">
                            {tString("changeRole.roles.host.title")}
                          </span>
                          <span className="text-xs text-gray-500">
                            {tString("changeRole.roles.host.description")}
                          </span>
                        </div>
                      </SelectItem>
                      <SelectItem
                        key="server"
                        value="server"
                        textValue={tString("changeRole.roles.server.title")}
                      >
                        <div className="flex flex-col">
                          <span className="font-medium">
                            {tString("changeRole.roles.server.title")}
                          </span>
                          <span className="text-xs text-gray-500">
                            {tString("changeRole.roles.server.description")}
                          </span>
                        </div>
                      </SelectItem>
                      <SelectItem
                        key="manager"
                        value="manager"
                        textValue={tString("changeRole.roles.manager.title")}
                      >
                        <div className="flex flex-col">
                          <span className="font-medium">
                            {tString("changeRole.roles.manager.title")}
                          </span>
                          <span className="text-xs text-gray-500">
                            {tString("changeRole.roles.manager.description")}
                          </span>
                        </div>
                      </SelectItem>
                    </Select>

                    {newRole !== staffMember.role && (
                      <div className="p-3 bg-brand/10 rounded-lg border border-brand/20">
                        <p className="text-sm text-brand-dark">
                          <strong>
                            {tString("changeRole.newRolePreview")}
                          </strong>{" "}
                          {roleLabel(newRole)}
                        </p>
                      </div>
                    )}

                    <Textarea
                      label={tString("changeRole.reason")}
                      placeholder={tString("changeRole.reasonPlaceholder")}
                      value={roleChangeReason}
                      onValueChange={setRoleChangeReason}
                      className="w-full"
                      minRows={2}
                      maxRows={4}
                    />

                    <Button
                      color="primary"
                      onPress={handleRoleChange}
                      isDisabled={newRole === staffMember.role}
                      isLoading={loading}
                      startContent={<UserCheck className="w-4 h-4" />}
                    >
                      {tString("changeRole.button")}
                    </Button>
                  </div>
                </CardBody>
              </Card>

              {/* Custom permission grants (owner or staff:permissions) */}
              {canManageCustomPerms && (
                <Card>
                  <CardHeader>
                    <div className="flex items-center gap-2">
                      <KeyRound className="w-5 h-5 text-brand" />
                      <h4 className="text-lg font-semibold">
                        {tString("customPermissions.title")}
                      </h4>
                    </div>
                  </CardHeader>
                  <CardBody>
                    {permsLoading && !permState ? (
                      <div className="flex items-center justify-center py-6">
                        <Spinner size="sm" color="primary" />
                      </div>
                    ) : (
                      <div className="space-y-5">
                        {/* Active custom grants */}
                        <div className="space-y-2">
                          <p className="text-sm font-medium text-ink-800">
                            {tString("customPermissions.grantsTitle")}
                          </p>
                          {(permState?.custom_grants ?? []).length === 0 ? (
                            <p className="text-sm text-ink-500">
                              {tString("customPermissions.empty")}
                            </p>
                          ) : (
                            <ul className="space-y-2">
                              {(permState?.custom_grants ?? []).map((perm) => (
                                <li
                                  key={perm}
                                  className="flex items-center justify-between gap-3 rounded-lg border border-warm-200 bg-warm-50/60 px-3 py-2"
                                >
                                  <span className="text-sm text-ink-800">
                                    {formatPermission(perm)}
                                  </span>
                                  <Button
                                    size="sm"
                                    color="danger"
                                    variant="flat"
                                    onPress={() =>
                                      void handleRevokePermission(perm)
                                    }
                                    isLoading={
                                      permActionLoading === `revoke:${perm}`
                                    }
                                    isDisabled={!!permActionLoading}
                                    startContent={
                                      <Minus className="w-3.5 h-3.5" />
                                    }
                                    data-testid={`revoke-perm-${perm}`}
                                  >
                                    {tString("customPermissions.revoke")}
                                  </Button>
                                </li>
                              ))}
                            </ul>
                          )}
                        </div>

                        {/* Active explicit denies */}
                        <div className="space-y-2">
                          <p className="text-sm font-medium text-ink-800">
                            {tString("customPermissions.deniesTitle")}
                          </p>
                          {(permState?.custom_denies ?? []).length === 0 ? (
                            <p className="text-sm text-ink-500">
                              {tString("customPermissions.deniesEmpty")}
                            </p>
                          ) : (
                            <ul className="space-y-2">
                              {(permState?.custom_denies ?? []).map((perm) => (
                                <li
                                  key={perm}
                                  className="flex items-center justify-between gap-3 rounded-lg border border-rose-200 bg-rose-50/50 px-3 py-2"
                                >
                                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                                    <span className="text-sm text-ink-800">
                                      {formatPermission(perm)}
                                    </span>
                                    <Chip size="sm" variant="flat" color="danger">
                                      {tString("customPermissions.denied")}
                                    </Chip>
                                  </div>
                                  <Button
                                    size="sm"
                                    color="primary"
                                    variant="flat"
                                    onPress={() => void handleRemoveDeny(perm)}
                                    isLoading={
                                      permActionLoading === `clear-deny:${perm}`
                                    }
                                    isDisabled={!!permActionLoading}
                                    data-testid={`clear-deny-${perm}`}
                                  >
                                    {tString("customPermissions.clearDeny")}
                                  </Button>
                                </li>
                              ))}
                            </ul>
                          )}
                        </div>

                        <Textarea
                          label={tString("customPermissions.reason")}
                          placeholder={tString(
                            "customPermissions.reasonPlaceholder",
                          )}
                          value={permissionReason}
                          onValueChange={setPermissionReason}
                          className="w-full"
                          minRows={2}
                          maxRows={3}
                        />

                        {/* Catalog by category (filtered to actor grantable set) */}
                        <div className="space-y-2">
                          <p className="text-sm font-medium text-ink-800">
                            {tString("customPermissions.browseTitle")}
                          </p>
                          {catalogByCategory.length === 0 ? (
                            <p className="text-sm text-ink-500">
                              {tString("customPermissions.catalogEmpty")}
                            </p>
                          ) : (
                          <Accordion
                            variant="bordered"
                            selectionMode="multiple"
                            className="px-0"
                          >
                            {catalogByCategory.map(
                              ([category, perms]) => (
                                <AccordionItem
                                  key={category}
                                  aria-label={tString(
                                    `rbacCategories.${category}`,
                                  )}
                                  title={
                                    <span className="text-sm font-medium text-ink-900">
                                      {tString(`rbacCategories.${category}`)}
                                    </span>
                                  }
                                  classNames={{
                                    title: "text-sm",
                                    content: "pb-3",
                                  }}
                                >
                                  <ul className="space-y-2">
                                    {perms.map((perm) => {
                                      const isDenied =
                                        customDenySet.has(perm);
                                      const isCustom =
                                        customGrantSet.has(perm);
                                      const isRoleDefault =
                                        rolePermSet.has(perm);
                                      const isHeld =
                                        effectivePermSet.has(perm);
                                      // Grant only when not held and not already denied
                                      // (clear deny first, then grant if needed).
                                      const canGrant =
                                        !isHeld && !isDenied && !isCustom;
                                      const canRevoke = isCustom && !isDenied;
                                      // Deny when currently effective (role or grant)
                                      // or when role/grant exists but not yet denied.
                                      const canDeny =
                                        !isDenied &&
                                        (isRoleDefault || isCustom || isHeld);
                                      const canClearDeny = isDenied;

                                      return (
                                        <li
                                          key={perm}
                                          className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-warm-100 px-2 py-1.5"
                                        >
                                          <div className="flex min-w-0 flex-wrap items-center gap-2">
                                            <div className="min-w-0">
                                              <span className="truncate text-xs text-ink-700">
                                                {formatPermission(perm)}
                                              </span>
                                              {formatPermissionDesc(perm) ? (
                                                <p className="mt-0.5 text-[11px] leading-snug text-ink-500">
                                                  {formatPermissionDesc(perm)}
                                                </p>
                                              ) : null}
                                            </div>
                                            {isRoleDefault && !isDenied && (
                                              <Chip
                                                size="sm"
                                                variant="flat"
                                                color="default"
                                              >
                                                {tString(
                                                  "customPermissions.roleDefault",
                                                )}
                                              </Chip>
                                            )}
                                            {isCustom && !isDenied && (
                                              <Chip
                                                size="sm"
                                                variant="flat"
                                                color="primary"
                                              >
                                                {tString(
                                                  "customPermissions.customGrant",
                                                )}
                                              </Chip>
                                            )}
                                            {isDenied && (
                                              <Chip
                                                size="sm"
                                                variant="flat"
                                                color="danger"
                                                data-testid={`denied-chip-${perm}`}
                                              >
                                                {tString(
                                                  "customPermissions.denied",
                                                )}
                                              </Chip>
                                            )}
                                          </div>
                                          <div className="flex shrink-0 gap-1">
                                            {canGrant && (
                                              <Button
                                                size="sm"
                                                color="primary"
                                                variant="flat"
                                                onPress={() =>
                                                  void handleGrantPermission(
                                                    perm,
                                                  )
                                                }
                                                isLoading={
                                                  permActionLoading ===
                                                  `grant:${perm}`
                                                }
                                                isDisabled={!!permActionLoading}
                                                startContent={
                                                  <Plus className="w-3.5 h-3.5" />
                                                }
                                                data-testid={`grant-perm-${perm}`}
                                              >
                                                {tString(
                                                  "customPermissions.grant",
                                                )}
                                              </Button>
                                            )}
                                            {canRevoke && (
                                              <Button
                                                size="sm"
                                                color="danger"
                                                variant="light"
                                                onPress={() =>
                                                  void handleRevokePermission(
                                                    perm,
                                                  )
                                                }
                                                isLoading={
                                                  permActionLoading ===
                                                  `revoke:${perm}`
                                                }
                                                isDisabled={!!permActionLoading}
                                                data-testid={`revoke-perm-cat-${perm}`}
                                              >
                                                {tString(
                                                  "customPermissions.revoke",
                                                )}
                                              </Button>
                                            )}
                                            {canDeny && (
                                              <Button
                                                size="sm"
                                                color="danger"
                                                variant="flat"
                                                onPress={() =>
                                                  void handleDenyPermission(perm)
                                                }
                                                isLoading={
                                                  permActionLoading ===
                                                  `deny:${perm}`
                                                }
                                                isDisabled={!!permActionLoading}
                                                data-testid={`deny-perm-${perm}`}
                                              >
                                                {tString(
                                                  "customPermissions.deny",
                                                )}
                                              </Button>
                                            )}
                                            {canClearDeny && (
                                              <Button
                                                size="sm"
                                                color="primary"
                                                variant="flat"
                                                onPress={() =>
                                                  void handleRemoveDeny(perm)
                                                }
                                                isLoading={
                                                  permActionLoading ===
                                                  `clear-deny:${perm}`
                                                }
                                                isDisabled={!!permActionLoading}
                                                data-testid={`clear-deny-cat-${perm}`}
                                              >
                                                {tString(
                                                  "customPermissions.clearDeny",
                                                )}
                                              </Button>
                                            )}
                                            {isRoleDefault &&
                                              !canRevoke &&
                                              !canDeny &&
                                              !canClearDeny && (
                                              <Button
                                                size="sm"
                                                variant="light"
                                                isDisabled
                                                data-testid={`role-default-${perm}`}
                                              >
                                                {tString(
                                                  "customPermissions.roleDefault",
                                                )}
                                              </Button>
                                            )}
                                          </div>
                                        </li>
                                      );
                                    })}
                                  </ul>
                                </AccordionItem>
                              ),
                            )}
                          </Accordion>
                          )}
                        </div>
                      </div>
                    )}
                  </CardBody>
                </Card>
              )}

              {/* Staff Status Management */}
              <Card>
                <CardHeader>
                  <h4 className="text-lg font-semibold">
                    {tString("staffStatus.title")}
                  </h4>
                </CardHeader>
                <CardBody>
                  <div className="space-y-4">
                    {staffMember.is_active !== false ? (
                      <div className="space-y-3">
                        <p className="text-sm text-gray-600">
                          {tString("staffStatus.deactivate.description")}
                        </p>
                        <Textarea
                          label={tString("staffStatus.deactivate.reason")}
                          placeholder={tString(
                            "staffStatus.deactivate.reasonPlaceholder",
                          )}
                          value={deactivateReason}
                          onValueChange={setDeactivateReason}
                          className="w-full"
                          minRows={2}
                          maxRows={4}
                        />
                        <Button
                          color="danger"
                          onPress={requestDeactivateStaff}
                          isLoading={loading}
                          startContent={<UserX className="w-4 h-4" />}
                        >
                          {tString("staffStatus.deactivate.button")}
                        </Button>
                      </div>
                    ) : (
                      <div className="space-y-3">
                        <p className="text-sm text-gray-600">
                          {tString("staffStatus.reactivate.description")}
                        </p>
                        <Button
                          color="success"
                          onPress={handleReactivateStaff}
                          isLoading={loading}
                          startContent={<UserCheck className="w-4 h-4" />}
                        >
                          {tString("staffStatus.reactivate.button")}
                        </Button>
                      </div>
                    )}
                  </div>
                </CardBody>
              </Card>

              {/* Recent Activity */}
              {auditLogs.length > 0 && (
                <Card>
                  <CardHeader>
                    <div className="flex items-center gap-2">
                      <History className="w-5 h-5" />
                      <h4 className="text-lg font-semibold">
                        {tString("recentActivity.title")}
                      </h4>
                    </div>
                  </CardHeader>
                  <CardBody>
                    <div className="space-y-3">
                      {auditLogs.slice(0, 5).map((log) => (
                        <div
                          key={log.id}
                          className="flex items-start gap-3 p-3 bg-gray-50 rounded-lg"
                        >
                          <Chip
                            size="sm"
                            color={getActionColor(log.action)}
                            variant="flat"
                          >
                            {formatAuditAction(log.action)}
                          </Chip>
                          <div className="flex-1 min-w-0">
                            <div className="text-sm">
                              {log.old_role && log.new_role && (
                                <p>
                                  {tString(
                                    "recentActivity.messages.roleChanged",
                                    {
                                      oldRole: roleLabel(
                                        log.old_role as StaffRoleManagementProps["staffMember"]["role"],
                                      ),
                                      newRole: roleLabel(
                                        log.new_role as StaffRoleManagementProps["staffMember"]["role"],
                                      ),
                                    },
                                  )}
                                </p>
                              )}
                              {log.action === "staff_created" && (
                                <p>
                                  {tString(
                                    "recentActivity.messages.staffCreated",
                                  )}
                                </p>
                              )}
                              {log.action === "staff_deactivated" && (
                                <p>
                                  {tString(
                                    "recentActivity.messages.staffDeactivated",
                                  )}
                                </p>
                              )}
                              {log.action === "staff_reactivated" && (
                                <p>
                                  {tString(
                                    "recentActivity.messages.staffReactivated",
                                  )}
                                </p>
                              )}
                            </div>
                            <div className="text-xs text-gray-500 mt-1">
                              <p>
                                {tString("recentActivity.changedBy", {
                                  name: formatActor(log.changed_by),
                                })}
                              </p>
                              <p>{formatDate(log.created_at)}</p>
                              {log.reason && (
                                <p>
                                  {tString("recentActivity.reason", {
                                    reason: log.reason,
                                  })}
                                </p>
                              )}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </CardBody>
                </Card>
              )}
            </div>
          )}

          {isOwner ? (
            <StaffCompensationSection
              businessId={businessId}
              staffId={staffMember.id}
              labels={{
                title: tString("compensation.title"),
                subtitle: tString("compensation.subtitle"),
                employmentType: tString("compensation.employmentType"),
                employmentTypes: {
                  unset: tString("compensation.employmentTypes.unset"),
                  hourly: tString("compensation.employmentTypes.hourly"),
                  salaried: tString("compensation.employmentTypes.salaried"),
                },
                hourlyRate: tString("compensation.hourlyRate"),
                annualSalary: tString("compensation.annualSalary"),
                save: tString("compensation.save"),
                saved: tString("compensation.saved"),
                saveError: tString("compensation.saveError"),
                loadError: tString("compensation.loadError"),
              }}
            />
          ) : null}
        </ModalBody>

        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            {tString("buttons.close")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>

      <ConfirmationModal
        isOpen={isConfirmOpen}
        onOpenChange={() => {
          // Dismissing without confirming (backdrop, ESC, Cancel) drops the
          // pending sensitive action so the gate stays closed.
          setPendingConfirm(null);
          onConfirmOpenChange();
        }}
        isDanger
        title={tString("customPermissions.sensitiveConfirmTitle")}
        description={tString("customPermissions.sensitiveConfirm", {
          action: pendingConfirm?.action ?? "",
          permission: pendingConfirm?.permission ?? "",
        })}
        confirmLabel={tString("customPermissions.sensitiveConfirmButton")}
        cancelLabel={tString("customPermissions.sensitiveConfirmCancel")}
        onConfirm={handleConfirmSensitive}
      />

      <ConfirmationModal
        isOpen={isDeactivateConfirmOpen}
        onOpenChange={onDeactivateConfirmOpenChange}
        isDanger
        title={tString("staffStatus.deactivate.confirmTitle")}
        description={tString("staffStatus.deactivate.confirmDescription", {
          name: staffMember.name,
        })}
        confirmLabel={tString("staffStatus.deactivate.confirmAction")}
        onConfirm={() => confirmDeactivateStaff()}
      />
    </>
  );
}
