"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Input, Select, SelectItem, Button } from "@nextui-org/react";
import { toast } from "react-hot-toast";
import {
  staffCompensationApi,
  type EmploymentType,
} from "@/api/staffCompensation";
import { asDollars } from "@/types/money";

export interface StaffCompensationSectionProps {
  businessId: string;
  staffId: number;
  labels: {
    title: string;
    subtitle: string;
    employmentType: string;
    employmentTypes: { unset: string; hourly: string; salaried: string };
    hourlyRate: string;
    annualSalary: string;
    save: string;
    saved: string;
    saveError: string;
    /** Shown when the initial load fails for a real reason (not a 403). */
    loadError?: string;
  };
}

// Narrow an unknown thrown value to its HTTP status without an `any` cast.
function errStatus(err: unknown): number | undefined {
  if (err && typeof err === "object" && "response" in err) {
    return (err as { response?: { status?: number } }).response?.status;
  }
  return undefined;
}

const EMPLOYMENT_TYPES: readonly EmploymentType[] = ["", "hourly", "salaried"];

export default function StaffCompensationSection({
  businessId,
  staffId,
  labels,
}: StaffCompensationSectionProps) {
  const [employmentType, setEmploymentType] = useState<EmploymentType>("");
  const [hourlyRate, setHourlyRate] = useState("");
  const [annualSalary, setAnnualSalary] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  // A real load failure (network/500) must not arm Save with zeros — that would
  // overwrite an existing pay record. A 403 (non-owner) is expected: the fields
  // stay empty and Save stays available (the server is the real gate, and an
  // owner who can read will have loaded the record). Only a non-403 error blocks.
  const [loadFailed, setLoadFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setLoadFailed(false);
    staffCompensationApi
      .getCompensation(businessId, String(staffId))
      .then((comp) => {
        if (cancelled) return;
        setEmploymentType(comp.employment_type ?? "");
        setHourlyRate(comp.hourly_rate ? String(comp.hourly_rate) : "");
        setAnnualSalary(comp.annual_salary ? String(comp.annual_salary) : "");
      })
      .catch((err) => {
        if (cancelled) return;
        // 403 = non-owner (expected, no record to clobber). Any other failure
        // means we don't know the current values → block Save so we can't
        // overwrite a real record with zeros.
        if (errStatus(err) !== 403) setLoadFailed(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, staffId]);

  const handleSave = useCallback(async () => {
    if (loadFailed) return;
    setSaving(true);
    try {
      await staffCompensationApi.updateCompensation(businessId, String(staffId), {
        employment_type: employmentType,
        hourly_rate: asDollars(Number(hourlyRate) || 0),
        annual_salary: asDollars(Number(annualSalary) || 0),
      });
      toast.success(labels.saved);
    } catch {
      toast.error(labels.saveError);
    } finally {
      setSaving(false);
    }
  }, [
    businessId,
    staffId,
    employmentType,
    hourlyRate,
    annualSalary,
    labels,
    loadFailed,
  ]);

  return (
    <div className="mt-4 rounded-xl border border-gray-200 p-4">
      <p className="text-sm font-semibold text-gray-900">{labels.title}</p>
      <p className="text-[11px] text-gray-500">{labels.subtitle}</p>

      <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Select
          aria-label={labels.employmentType}
          label={labels.employmentType}
          size="sm"
          selectedKeys={[employmentType]}
          onSelectionChange={(keys) => {
            const next = String(Array.from(keys)[0] ?? "");
            if ((EMPLOYMENT_TYPES as readonly string[]).includes(next)) {
              setEmploymentType(next as EmploymentType);
            }
          }}
          isDisabled={loading || loadFailed}
        >
          <SelectItem key="">{labels.employmentTypes.unset}</SelectItem>
          <SelectItem key="hourly">{labels.employmentTypes.hourly}</SelectItem>
          <SelectItem key="salaried">{labels.employmentTypes.salaried}</SelectItem>
        </Select>

        <Input
          type="number"
          inputMode="decimal"
          min="0"
          step="0.01"
          size="sm"
          label={labels.hourlyRate}
          aria-label={labels.hourlyRate}
          value={hourlyRate}
          onValueChange={setHourlyRate}
          isDisabled={loading || loadFailed}
        />

        <Input
          type="number"
          inputMode="decimal"
          min="0"
          step="1"
          size="sm"
          label={labels.annualSalary}
          aria-label={labels.annualSalary}
          value={annualSalary}
          onValueChange={setAnnualSalary}
          isDisabled={loading || loadFailed}
        />
      </div>

      {loadFailed ? (
        <p className="mt-2 text-xs text-rose-600" role="alert">
          {labels.loadError ?? labels.saveError}
        </p>
      ) : null}

      <div className="mt-3 flex justify-end">
        <Button
          size="sm"
          color="primary"
          onPress={handleSave}
          isLoading={saving}
          isDisabled={loading || loadFailed}
        >
          {labels.save}
        </Button>
      </div>
    </div>
  );
}
