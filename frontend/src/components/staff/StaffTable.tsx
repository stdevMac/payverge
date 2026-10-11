"use client";

import React from "react";
import {
  Button,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
} from "@nextui-org/react";
import { Users, Trash2, Edit, UserPlus } from "lucide-react";
import * as StaffAPI from "../../api/staff";
import IconTile from "@/components/ui/IconTile";
import { EmptyState } from "@/components/ui/EmptyState";
import { tableClassNames } from "../business/shared/tableStyles";
import { sectionHeadingClass } from "@/components/ui/headingStyles";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";
import { StatusChip, type StatusTone } from "../ui/StatusChip";

type Staff = StaffAPI.StaffMember;

interface StaffTableProps {
  staff: Staff[];
  getRoleLabel: (role: Staff["role"]) => string;
  getRoleDescription: (role: Staff["role"]) => string;
  tString: (key: string) => string;
  formatDate: (dateString: string) => string;
  handleUpdateRole: (staff: Staff) => void;
  handleRemoveStaff: (staffId: number, staffName: string) => void;
  onInviteClick?: () => void;
  /**
   * L5-15: when a search/filter is active and the list is empty, show the
   * filtered empty framing (not the cold-start "no team yet" setup copy).
   */
  isFiltered?: boolean;
  filterQuery?: string;
  onClearFilter?: () => void;
  /** Optional translated title/body for filtered empty (P-owned keys). */
  filteredEmptyTitle?: string;
  filteredEmptyBody?: string;
}

// Role no longer carries a rainbow of colors: only manager is elevated (info),
// every other role reads as neutral so the table scans calmly.
const roleTones: Record<string, StatusTone> = {
  manager: "info",
};

const roleTone = (role: Staff["role"]): StatusTone =>
  roleTones[role] ?? "neutral";

function staffActionName(template: string, name: string): string {
  return template.includes("{name}")
    ? template.replace("{name}", name)
    : `${template} ${name}`;
}

export default function StaffTable({
  staff,
  getRoleLabel,
  getRoleDescription,
  tString,
  formatDate,
  handleUpdateRole,
  handleRemoveStaff,
  onInviteClick,
  isFiltered = false,
  filterQuery = "",
  onClearFilter,
  filteredEmptyTitle,
  filteredEmptyBody,
}: StaffTableProps) {
  if (staff.length === 0) {
    if (isFiltered) {
      const title =
        filteredEmptyTitle ||
        tString("table.noStaffFound");
      const subtitle =
        filteredEmptyBody ||
        tString("table.adjustSearch");
      return (
        <div className="mb-6">
          <EmptyState
            panel
            icon={Users}
            title={title}
            subtitle={subtitle}
            action={
              onClearFilter ? (
                <Button
                  radius="full"
                  className={btnPrimaryNextUI}
                  onPress={onClearFilter}
                >
                  {tString("search.clear") || "Clear search"}
                </Button>
              ) : undefined
            }
          />
        </div>
      );
    }
    return (
      <div className="mb-6">
        <EmptyState
          panel
          icon={Users}
          title={tString("table.noStaffYet")}
          subtitle={tString("table.inviteFirstStaff")}
          action={
            onInviteClick ? (
              <Button
                radius="full"
                className={btnPrimaryNextUI}
                onPress={onInviteClick}
                startContent={<UserPlus className="w-4 h-4" />}
              >
                {tString("buttons.inviteStaff")}
              </Button>
            ) : undefined
          }
        />
      </div>
    );
  }

  return (
    <div className="mb-6 rounded-2xl border border-warm-200 bg-white">
      <div className="border-b border-warm-200 p-6">
        <div className="flex items-center gap-3">
          <IconTile icon={Users} size="lg" />
          <div>
            <h2 className={sectionHeadingClass}>
              {tString("table.staffTitle")}
            </h2>
            <p className="text-ink-600 font-light text-sm">
              {tString("table.staffSubtitle")}
            </p>
          </div>
        </div>
      </div>
      <div className="p-6">
        <Table
          aria-label={tString("table.tableAria")}
          removeWrapper
          classNames={tableClassNames}
        >
          <TableHeader>
            <TableColumn>{tString("table.columns.name")}</TableColumn>
            <TableColumn>{tString("table.columns.role")}</TableColumn>
            <TableColumn>{tString("table.columns.joined")}</TableColumn>
            <TableColumn>{tString("table.columns.actions")}</TableColumn>
          </TableHeader>
          <TableBody>
            {staff.map((member) => (
              <TableRow key={member.id}>
                <TableCell>
                  {/* Name leads; email is secondary under the name so long
                      demo/internal addresses never dominate the row (#189). */}
                  <div className="flex min-w-0 flex-col gap-0.5">
                    <div className="flex items-center gap-2">
                      <p className="font-medium text-ink-900">{member.name}</p>
                      {member.is_active === false && (
                        <StatusChip
                          tone="neutral"
                          label={tString("table.inactive")}
                        />
                      )}
                    </div>
                    {member.email ? (
                      <p
                        className="max-w-[14rem] truncate text-xs text-ink-500"
                        title={member.email}
                      >
                        {member.email}
                      </p>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell>
                  {/* The description repeated under every row was wallpaper —
                      it stays one hover away and lives in the invite/edit flows. */}
                  <span title={getRoleDescription(member.role)}>
                    <StatusChip
                      tone={roleTone(member.role)}
                      label={getRoleLabel(member.role)}
                    />
                  </span>
                </TableCell>
                <TableCell>
                  <span className="text-ink-600">
                    {formatDate(member.created_at)}
                  </span>
                </TableCell>
                <TableCell>
                  <div className="flex gap-2">
                    <Button
                      isIconOnly
                      aria-label={staffActionName(
                        tString("table.manageStaffNamed"),
                        member.name,
                      )}
                      size="sm"
                      variant="light"
                      onPress={() => handleUpdateRole(member)}
                      className="text-ink-600 hover:text-brand"
                      title={staffActionName(
                        tString("table.manageStaffNamed"),
                        member.name,
                      )}
                    >
                      <Edit className="w-4 h-4" />
                    </Button>
                    <Button
                      isIconOnly
                      aria-label={staffActionName(
                        tString("table.removeStaffNamed"),
                        member.name,
                      )}
                      size="sm"
                      variant="light"
                      onPress={() => handleRemoveStaff(member.id, member.name)}
                      className="text-ink-600 hover:text-red-600"
                      title={staffActionName(
                        tString("table.removeStaffNamed"),
                        member.name,
                      )}
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
