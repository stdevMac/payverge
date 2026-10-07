"use client";

import React from "react";
import { Button, Input, Select, SelectItem } from "@nextui-org/react";
import { Search, X } from "lucide-react";
import * as StaffAPI from "../../api/staff";

type Staff = StaffAPI.StaffMember;
type PendingInvitation = StaffAPI.StaffInvitation;

interface StaffSearchFilterProps {
  searchQuery: string;
  setSearchQuery: (query: string) => void;
  roleFilter: "all" | Staff["role"];
  setRoleFilter: (role: "all" | Staff["role"]) => void;
  filteredStaff: Staff[];
  filteredInvitations: PendingInvitation[];
  getRoleLabel: (role: Staff["role"]) => string;
  tString: (key: string) => string;
  clearSearch: () => void;
}

// A search box and a role filter — so it renders as exactly that: one inline
// toolbar row. The old titled "Search & Filter" card (icon + heading +
// subtitle) was chrome wrapped around a textbox; it's gone. The result count
// only appears while a query/filter is active, as a quiet line beneath.
export default function StaffSearchFilter({
  searchQuery,
  setSearchQuery,
  roleFilter,
  setRoleFilter,
  filteredStaff,
  filteredInvitations,
  getRoleLabel,
  tString,
  clearSearch,
}: StaffSearchFilterProps) {
  const isActive = Boolean(searchQuery) || roleFilter !== "all";

  return (
    <div className="mb-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <Input
          aria-label={tString("search.placeholder")}
          placeholder={tString("search.placeholder")}
          value={searchQuery}
          onValueChange={setSearchQuery}
          startContent={<Search className="h-4 w-4 text-default-400" />}
          size="md"
          variant="bordered"
          radius="lg"
          className="flex-1"
          classNames={{ inputWrapper: "bg-white border-gray-200" }}
          isClearable
          onClear={() => setSearchQuery("")}
        />

        <div className="flex items-center gap-2">
          <Select
            aria-label={tString("search.filterByRole")}
            placeholder={tString("search.filterByRole")}
            selectedKeys={[roleFilter]}
            onSelectionChange={(keys) =>
              setRoleFilter(Array.from(keys)[0] as any)
            }
            className="w-full sm:w-44"
            size="md"
            variant="bordered"
            radius="lg"
            classNames={{ trigger: "bg-white border-gray-200" }}
          >
            <SelectItem key="all" value="all">
              {tString("search.allRoles")}
            </SelectItem>
            <SelectItem key="manager" value="manager">
              {getRoleLabel("manager")}
            </SelectItem>
            <SelectItem key="server" value="server">
              {getRoleLabel("server")}
            </SelectItem>
            <SelectItem key="host" value="host">
              {getRoleLabel("host")}
            </SelectItem>
            <SelectItem key="kitchen" value="kitchen">
              {getRoleLabel("kitchen")}
            </SelectItem>
          </Select>

          {isActive && (
            <Button
              variant="light"
              size="md"
              startContent={<X className="h-4 w-4" />}
              onPress={clearSearch}
              className="shrink-0 text-gray-600"
            >
              {tString("buttons.clear")}
            </Button>
          )}
        </div>
      </div>

      {isActive && (
        <p className="mt-2 text-xs text-gray-500">
          {tString("search.showing")}{" "}
          <strong className="font-semibold text-ink-900">
            {filteredStaff.length}
          </strong>{" "}
          {tString("search.staffMembers")}
          {filteredInvitations.length > 0 && (
            <>
              {" "}
              {tString("search.and")}{" "}
              <strong className="font-semibold text-ink-900">
                {filteredInvitations.length}
              </strong>{" "}
              {tString("search.pendingInvitations")}
            </>
          )}
          {roleFilter !== "all" && (
            <>
              {" "}
              · {getRoleLabel(roleFilter as Staff["role"])}{" "}
              {tString("search.only")}
            </>
          )}
        </p>
      )}
    </div>
  );
}
