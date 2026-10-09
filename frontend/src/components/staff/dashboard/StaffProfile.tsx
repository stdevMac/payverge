"use client";

import React from "react";
import { Chip, Button } from "@nextui-org/react";
import type { StaffData } from "@/utils/staffAuth";

interface StaffProfileLabels {
  title: string;
  roleLabel: string;
  businessLabel: string;
  emailLabel: string;
  lastLoginLabel: string;
  signOut: string;
  roleName: string; // already-localized display name for staff.role
}

export interface StaffProfileProps {
  staff: StaffData;
  labels: StaffProfileLabels;
  onSignOut: () => void;
  signingOut?: boolean;
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between border-b border-gray-100 py-3 last:border-0">
      <span className="text-sm text-gray-500">{label}</span>
      <span className="text-sm font-medium text-gray-900">{value}</span>
    </div>
  );
}

export default function StaffProfile({ staff, labels, onSignOut, signingOut }: StaffProfileProps) {
  const lastLogin = staff.last_login_at
    ? new Date(staff.last_login_at).toLocaleString()
    : "—";
  return (
    <section className="rounded-xl border border-gray-200 p-4" aria-label={labels.title}>
      <div className="mb-3 flex items-center gap-3">
        <div
          className="flex h-12 w-12 items-center justify-center rounded-full bg-brand-50 font-title text-lg text-brand"
          aria-hidden="true"
        >
          {staff.name?.charAt(0)?.toUpperCase() || "?"}
        </div>
        <div>
          <h2 className="font-title text-base text-gray-900">{staff.name}</h2>
          <Chip size="sm" variant="flat" className="mt-1">
            {labels.roleName}
          </Chip>
        </div>
      </div>

      <Row label={labels.businessLabel} value={staff.business_name || "—"} />
      <Row label={labels.emailLabel} value={staff.email} />
      <Row label={labels.lastLoginLabel} value={lastLogin} />

      <Button
        className="mt-4 w-full"
        variant="bordered"
        color="danger"
        onPress={onSignOut}
        isLoading={signingOut}
      >
        {labels.signOut}
      </Button>
    </section>
  );
}
