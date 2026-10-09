"use client";

import React, { useState } from "react";
import {
  Button,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
} from "@nextui-org/react";
import { Ban, Check, Link2, Mail, RefreshCw } from "lucide-react";
import * as StaffAPI from "../../api/staff";
import IconTile from "@/components/ui/IconTile";
import { tableClassNames } from "../business/shared/tableStyles";
import { sectionHeadingClass } from "@/components/ui/headingStyles";
import { StatusChip, type StatusTone } from "../ui/StatusChip";

type PendingInvitation = StaffAPI.StaffInvitation;
type Staff = StaffAPI.StaffMember;

interface InvitationTableProps {
  invitations: PendingInvitation[];
  businessId: string;
  getRoleLabel: (role: Staff["role"]) => string;
  tString: (key: string) => string;
  formatDate: (dateString: string) => string;
  isInvitationExpired: (expiresAt: string) => boolean;
  handleResendInvitation: (invitationId: number, staffName: string) => void;
  handleRevokeInvitation?: (invitationId: number, staffName: string) => void;
}

const statusTones: Record<string, StatusTone> = {
  pending: "warn",
  accepted: "success",
  expired: "danger",
  revoked: "neutral",
};

const statusTone = (status: string): StatusTone =>
  statusTones[status] ?? "neutral";

// Role no longer carries a rainbow of colors: only manager is elevated (info),
// every other role reads as neutral so the table scans calmly.
const roleTones: Record<string, StatusTone> = {
  manager: "info",
};

const roleTone = (role: Staff["role"]): StatusTone =>
  roleTones[role] ?? "neutral";

export default function InvitationTable({
  invitations,
  businessId,
  getRoleLabel,
  tString,
  formatDate,
  isInvitationExpired,
  handleResendInvitation,
  handleRevokeInvitation,
}: InvitationTableProps) {
  // Copy-link fallback for lost/spam-filtered emails. Tokens are never listed
  // on GET staff (staff:read); authorized inviters fetch the URL via
  // staff:invite-gated getInvitationLink.
  const [copiedId, setCopiedId] = useState<number | null>(null);
  const [copyingId, setCopyingId] = useState<number | null>(null);
  const copyInviteLink = async (invitation: PendingInvitation) => {
    try {
      setCopyingId(invitation.id);
      const { invitation_url: url } = await StaffAPI.getInvitationLink(
        businessId,
        invitation.id,
      );
      await navigator.clipboard.writeText(url);
      setCopiedId(invitation.id);
      setTimeout(() => setCopiedId((cur) => (cur === invitation.id ? null : cur)), 2000);
    } catch {
      // Link fetch or clipboard failed — leave the button as-is.
    } finally {
      setCopyingId((cur) => (cur === invitation.id ? null : cur));
    }
  };
  // Expired invitations stay visible (with an "expired" chip + one-tap "Invite
  // again") — the backend resend endpoint mints a fresh token and 7-day expiry
  // for them, so hiding the row made a recoverable state look like a dead end.
  // Accepted/revoked rows are the only truly-final ones the API filters out.
  const visibleInvitations = invitations;

  if (visibleInvitations.length === 0) {
    return (
      <div className="mb-6 rounded-2xl border border-warm-200 bg-white">
        <div className="text-center py-12">
          <div className="mx-auto mb-6 flex justify-center">
            <IconTile icon={Mail} size="lg" />
          </div>
          <h3 className="text-lg font-semibold text-ink-900 mb-2">
            {tString("invitations.noPending")}
          </h3>
          <p className="text-ink-600 font-light text-sm">
            {tString("invitations.allAcceptedOrExpired")}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="mb-6 rounded-2xl border border-warm-200 bg-white">
      <div className="border-b border-warm-200 p-6">
        <div className="flex items-center gap-3">
          <IconTile icon={Mail} size="lg" />
          <div>
            <h2 className={sectionHeadingClass}>
              {tString("invitations.title")}
            </h2>
            <p className="text-ink-600 font-light text-sm">
              {tString("invitations.subtitle")}
            </p>
            <p className="mt-0.5 text-xs text-ink-500">
              {tString("invitations.expiryHint")}
            </p>
          </div>
        </div>
      </div>
      <div className="p-6">
        <Table
          aria-label={tString("invitations.tableAria")}
          removeWrapper
          classNames={tableClassNames}
        >
          <TableHeader>
            <TableColumn>{tString("invitations.columns.name")}</TableColumn>
            <TableColumn>{tString("invitations.columns.email")}</TableColumn>
            <TableColumn>{tString("invitations.columns.role")}</TableColumn>
            <TableColumn>{tString("invitations.columns.status")}</TableColumn>
            <TableColumn>{tString("invitations.columns.expires")}</TableColumn>
            <TableColumn>{tString("invitations.columns.actions")}</TableColumn>
          </TableHeader>
          <TableBody>
            {visibleInvitations.map((invitation) => {
              const expired = isInvitationExpired(invitation.expires_at);
              const status = expired
                ? "expired"
                : invitation.status || "pending";

              return (
                <TableRow
                  key={invitation.id}
                  // Visually separate expired invitations — a muted row so the
                  // active ones read first.
                  className={expired ? "opacity-60" : undefined}
                >
                  <TableCell>
                    <div className="flex flex-col">
                      <p className="font-medium text-ink-900">
                        {invitation.name}
                      </p>
                    </div>
                  </TableCell>
                  <TableCell>
                    <span className="text-ink-600">{invitation.email}</span>
                  </TableCell>
                  <TableCell>
                    <StatusChip
                      tone={roleTone(invitation.role)}
                      label={getRoleLabel(invitation.role)}
                    />
                  </TableCell>
                  <TableCell>
                    <StatusChip
                      tone={statusTone(status)}
                      label={tString(`invitations.status.${status}`)}
                    />
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-ink-600">
                      {formatDate(invitation.expires_at)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-2">
                      {(status === "pending" || status === "expired") && (
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          isLoading={copyingId === invitation.id}
                          aria-label={tString(
                            copiedId === invitation.id
                              ? "invitations.copied"
                              : "invitations.copyLink",
                          )}
                          onPress={() => void copyInviteLink(invitation)}
                          className="text-ink-600 hover:text-brand"
                        >
                          {copiedId === invitation.id ? (
                            <Check className="w-4 h-4 text-emerald-600" />
                          ) : (
                            <Link2 className="w-4 h-4" />
                          )}
                        </Button>
                      )}
                      {status === "pending" && (
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          aria-label={tString("invitations.resendTooltip")}
                          onPress={() =>
                            handleResendInvitation(
                              invitation.id,
                              invitation.name,
                            )
                          }
                          className="text-ink-600 hover:text-brand"
                        >
                          <RefreshCw className="w-4 h-4" />
                        </Button>
                      )}
                      {status === "expired" && (
                        <Button
                          size="sm"
                          variant="flat"
                          startContent={<RefreshCw className="w-3.5 h-3.5" />}
                          onPress={() =>
                            handleResendInvitation(
                              invitation.id,
                              invitation.name,
                            )
                          }
                          className="bg-brand/10 text-xs font-medium text-brand"
                        >
                          {tString("invitations.inviteAgain")}
                        </Button>
                      )}
                      {/* Revoke: kill the live link for a pending invite, or
                          dismiss an expired one. Confirmed in the parent. */}
                      {(status === "pending" || status === "expired") &&
                      handleRevokeInvitation ? (
                        <Button
                          isIconOnly
                          size="sm"
                          variant="light"
                          aria-label={tString("invitations.revokeTooltip")}
                          onPress={() =>
                            handleRevokeInvitation(
                              invitation.id,
                              invitation.name,
                            )
                          }
                          className="text-ink-600 hover:text-danger"
                        >
                          <Ban className="w-4 h-4" />
                        </Button>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
