"use client";

import React, { useCallback, useRef } from "react";
import { Input, Textarea } from "@nextui-org/react";
import { Mail, Phone, Users } from "lucide-react";
import type {
  CreateReservationRequest,
  ReservationTableOption,
} from "@/api/reservations";
import {
  clampPartySize,
  reconcileTableForPartySize,
} from "../reservationFormHelpers";
import { isValidPhone } from "@/lib/fieldValidation";
import {
  ReservationTimeField,
  type ReservationTimeEntryMode,
} from "./ReservationTimeField";
import { ReservationTablePicker } from "./ReservationTablePicker";
import {
  RESERVATION_CUSTOMER_NAME_MAX_LENGTH,
  RESERVATION_CUSTOMER_PHONE_MAX_LENGTH,
  RESERVATION_SPECIAL_REQUESTS_MAX_LENGTH,
} from "./reservationFieldLimits";

const reservationSectionEyebrowClass =
  "text-xs font-semibold uppercase tracking-[0.18em] text-brand-700";
const reservationFieldIconClass = "w-4 h-4 text-ink-400";
const reservationSoftBoxClass =
  "rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4";
// Stable identities so NextUI Input does not rebuild startContent on every keystroke.
const phoneStartContent = <Phone className={reservationFieldIconClass} />;
const mailStartContent = <Mail className={reservationFieldIconClass} />;
const usersStartContent = <Users className={reservationFieldIconClass} />;

export type ReservationFormFieldLabels = {
  customerInfo: string;
  customerName: string;
  customerPhone: string;
  customerEmail: string;
  reservationDetails: string;
  partySize: string;
  dateTime: string;
  duration: string;
  minutes: string;
  additionalInfo: string;
  specialRequests: string;
  specialRequestsPlaceholder: string;
  notes: string;
  notesPlaceholder: string;
  /** Inline phone format error (L1-16). */
  phoneInvalid?: string;
  /** Shown when phone is empty after submit attempt. */
  phoneRequired?: string;
  timeEntry: {
    modeLabel: string;
    business: string;
    device: string;
    preview: string;
  };
  occupancy: {
    available: string;
    occupied: string;
    staleOccupied: string;
    override: string;
    overrideRequired: string;
    reservationConflict: string;
    capacityConflict: string;
    /** L1-11: why the selected table was dropped after party-size change. */
    tableClearedForPartySize?: string;
  };
};

export type ReservationFormFieldsProps = {
  formData: CreateReservationRequest;
  onChange: (next: CreateReservationRequest) => void;
  labels: ReservationFormFieldLabels;
  timeEntryMode: ReservationTimeEntryMode;
  onTimeEntryModeChange: (mode: ReservationTimeEntryMode) => void;
  businessTimeZone: string;
  deviceTimeZone: string;
  minPartySize?: number;
  maxPartySize?: number;
  defaultDuration?: number;
  tableOptions: ReservationTableOption[];
  occupancyOverrideAcknowledged: boolean;
  onOccupancyOverrideChange: (value: boolean) => void;
  recommendedTablesSlot?: React.ReactNode;
  /** Floor the datetime picker at today — create mode only (L1-1). */
  enforceTodayMin?: boolean;
  /** L1-11: toast when party size invalidates the selected table. */
  onTableClearedForPartySize?: (reason: string) => void;
  /** When true, empty required phone shows inline error (after submit attempt). */
  showRequiredErrors?: boolean;
};

/**
 * Shared create/edit reservation form body. Mode-specific copy is passed in
 * via `labels` so create and edit modals stay one implementation.
 */
export function ReservationFormFields({
  formData,
  onChange,
  labels,
  timeEntryMode,
  onTimeEntryModeChange,
  businessTimeZone,
  deviceTimeZone,
  minPartySize,
  maxPartySize,
  defaultDuration = 120,
  tableOptions,
  occupancyOverrideAcknowledged,
  onOccupancyOverrideChange,
  recommendedTablesSlot,
  enforceTodayMin,
  onTableClearedForPartySize,
  showRequiredErrors = false,
}: ReservationFormFieldsProps) {
  const formDataRef = useRef(formData);
  formDataRef.current = formData;
  const patch = useCallback(
    (partial: Partial<CreateReservationRequest>) => {
      const next = { ...formDataRef.current, ...partial };
      formDataRef.current = next;
      onChange(next);
    },
    [onChange],
  );

  const handlePartySizeChange = (value: string) => {
    const nextSize = clampPartySize(
      Number.parseInt(value || "1", 10) || 1,
      minPartySize,
      maxPartySize,
    );
    // Prefer live table options (capacity at this slot); keep selection when
    // it still fits the new party size (L1-11).
    const { tableId, cleared } = reconcileTableForPartySize(
      formData.table_id,
      nextSize,
      tableOptions,
    );
    patch({
      party_size: nextSize,
      ...(cleared ? { table_id: tableId } : {}),
    });
    if (cleared) {
      const reason =
        labels.occupancy.tableClearedForPartySize ||
        labels.occupancy.capacityConflict;
      onTableClearedForPartySize?.(reason);
    }
  };

  const phoneValue = formData.customer_phone ?? "";
  const emailValue = formData.customer_email ?? "";
  const phoneTrimmed = phoneValue.trim();
  const phoneFormatInvalid =
    phoneTrimmed.length > 0 && !isValidPhone(phoneValue);
  const phoneMissing = showRequiredErrors && !phoneTrimmed;
  const phoneInvalid = phoneFormatInvalid || phoneMissing;
  const phoneErrorMessage = phoneMissing
    ? labels.phoneRequired
    : phoneFormatInvalid
      ? labels.phoneInvalid
      : undefined;

  return (
    <div className="space-y-6">
      <div>
        <h4 className={`${reservationSectionEyebrowClass} mb-3`}>
          {labels.customerInfo}
        </h4>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            label={labels.customerName}
            value={formData.customer_name}
            onChange={(event) =>
              patch({ customer_name: event.target.value })
            }
            maxLength={RESERVATION_CUSTOMER_NAME_MAX_LENGTH}
            isRequired
            validationBehavior="aria"
          />
          {/* Native onChange only: NextUI onValueChange can emit "" when
              startContent / isInvalid remounts the inner input. */}
          <Input
            label={labels.customerPhone}
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            value={phoneValue}
            onChange={(event) =>
              patch({ customer_phone: event.target.value })
            }
            maxLength={RESERVATION_CUSTOMER_PHONE_MAX_LENGTH}
            startContent={phoneStartContent}
            isRequired
            isInvalid={phoneInvalid}
            errorMessage={phoneErrorMessage}
            validationBehavior="aria"
            data-testid="reservation-customer-phone"
          />
          <Input
            label={labels.customerEmail}
            type="text"
            inputMode="email"
            autoComplete="email"
            value={emailValue}
            onChange={(event) =>
              patch({ customer_email: event.target.value })
            }
            startContent={mailStartContent}
            validationBehavior="aria"
            className="md:col-span-2"
            data-testid="reservation-customer-email"
          />
        </div>
      </div>

      <div>
        <h4 className={`${reservationSectionEyebrowClass} mb-3`}>
          {labels.reservationDetails}
        </h4>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            label={labels.partySize}
            type="number"
            value={formData.party_size.toString()}
            onValueChange={handlePartySizeChange}
            min={minPartySize || 1}
            max={maxPartySize || 20}
            isRequired
            startContent={usersStartContent}
          />
          <ReservationTimeField
            value={formData.reservation_time}
            mode={timeEntryMode}
            businessTimeZone={businessTimeZone}
            deviceTimeZone={deviceTimeZone}
            labels={{
              entryMode: labels.timeEntry.modeLabel,
              businessTime: labels.timeEntry.business,
              deviceTime: labels.timeEntry.device,
              dateTime: labels.dateTime,
              conversionPreview: labels.timeEntry.preview,
            }}
            onValueChange={(value) => patch({ reservation_time: value })}
            onModeChange={onTimeEntryModeChange}
            enforceTodayMin={enforceTodayMin}
          />
          <Input
            label={labels.duration}
            type="number"
            value={String(formData.duration || defaultDuration)}
            onValueChange={(value) =>
              patch({
                duration:
                  Math.max(
                    Number.parseInt(
                      value || String(defaultDuration),
                      10,
                    ) || defaultDuration,
                    0,
                  ) || defaultDuration,
              })
            }
            endContent={
              <span className="text-sm text-ink-500">{labels.minutes}</span>
            }
          />
          <div className="md:col-span-2">
            <ReservationTablePicker
              options={tableOptions}
              selectedTableId={formData.table_id ?? undefined}
              overrideAcknowledged={occupancyOverrideAcknowledged}
              labels={{
                available: labels.occupancy.available,
                occupied: labels.occupancy.occupied,
                staleOccupied: labels.occupancy.staleOccupied,
                override: labels.occupancy.override,
                overrideRequired: labels.occupancy.overrideRequired,
                reservationConflict: labels.occupancy.reservationConflict,
                capacityConflict: labels.occupancy.capacityConflict,
              }}
              onSelect={(tableId) => patch({ table_id: tableId })}
              onOverrideChange={onOccupancyOverrideChange}
            />
          </div>
        </div>

        {tableOptions.length === 0 && recommendedTablesSlot ? (
          <div className={`mt-4 ${reservationSoftBoxClass}`}>
            {recommendedTablesSlot}
          </div>
        ) : null}
      </div>

      <div>
        <h4 className={`${reservationSectionEyebrowClass} mb-3`}>
          {labels.additionalInfo}
        </h4>
        <div className="space-y-4">
          <Textarea
            label={labels.specialRequests}
            value={formData.special_requests}
            onValueChange={(value) => patch({ special_requests: value })}
            maxLength={RESERVATION_SPECIAL_REQUESTS_MAX_LENGTH}
            placeholder={labels.specialRequestsPlaceholder}
            minRows={2}
          />
          <Textarea
            label={labels.notes}
            value={formData.notes}
            onValueChange={(value) => patch({ notes: value })}
            placeholder={labels.notesPlaceholder}
            minRows={2}
          />
        </div>
      </div>
    </div>
  );
}
