"use client";

import React, { useEffect, useState } from "react";
import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import type {
  CreateReservationRequest,
  ReservationTableOption,
} from "@/api/reservations";
import type { ReservationTimeEntryMode } from "./ReservationTimeField";
import {
  ReservationFormFields,
  type ReservationFormFieldLabels,
} from "./ReservationFormFields";

const reservationModalClassNames = {
  base: "border border-warm-200 bg-white shadow-[0_28px_80px_rgba(46,42,37,0.18)]",
  header: "border-b border-warm-200/80",
  body: "py-6",
  footer: "border-t border-warm-200/80 bg-warm-50/50",
};
const reservationModalCopyClass = "text-sm leading-6 text-ink-600";

export type ReservationFormModalProps = {
  mode: "create" | "edit";
  isOpen: boolean;
  onClose: () => void;
  title: string;
  subtitle: string;
  cancelLabel: string;
  submitLabel: string;
  formData: CreateReservationRequest;
  onFormDataChange: (next: CreateReservationRequest) => void;
  fieldLabels: ReservationFormFieldLabels;
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
  isSubmitting: boolean;
  onSubmit: () => void;
  /** L1-11: notify when party size clears an undersized table selection. */
  onTableClearedForPartySize?: (reason: string) => void;
};

/**
 * Shared create/edit reservation modal shell parameterized by `mode`.
 */
export function ReservationFormModal({
  mode,
  isOpen,
  onClose,
  title,
  subtitle,
  cancelLabel,
  submitLabel,
  formData,
  onFormDataChange,
  fieldLabels,
  timeEntryMode,
  onTimeEntryModeChange,
  businessTimeZone,
  deviceTimeZone,
  minPartySize,
  maxPartySize,
  defaultDuration,
  tableOptions,
  occupancyOverrideAcknowledged,
  onOccupancyOverrideChange,
  recommendedTablesSlot,
  isSubmitting,
  onSubmit,
  onTableClearedForPartySize,
}: ReservationFormModalProps) {
  const [showRequiredErrors, setShowRequiredErrors] = useState(false);

  useEffect(() => {
    if (isOpen) setShowRequiredErrors(false);
  }, [isOpen]);

  const requiresOverride = Boolean(
    tableOptions.find((option) => option.id === formData.table_id)
      ?.requires_occupancy_override && !occupancyOverrideAcknowledged,
  );

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="3xl"
      classNames={reservationModalClassNames}
      data-testid={`reservation-form-modal-${mode}`}
      disableAnimation
    >
      <ModalContent>
        {(close) => (
          <>
            <ModalHeader className="flex flex-col gap-1 pb-4">
              <h3 className="text-xl font-semibold text-ink-950">{title}</h3>
              <p className={reservationModalCopyClass}>{subtitle}</p>
            </ModalHeader>
            <ModalBody className="py-6">
              <ReservationFormFields
                formData={formData}
                onChange={onFormDataChange}
                labels={fieldLabels}
                timeEntryMode={timeEntryMode}
                onTimeEntryModeChange={onTimeEntryModeChange}
                businessTimeZone={businessTimeZone}
                deviceTimeZone={deviceTimeZone}
                minPartySize={minPartySize}
                maxPartySize={maxPartySize}
                defaultDuration={defaultDuration}
                tableOptions={tableOptions}
                occupancyOverrideAcknowledged={occupancyOverrideAcknowledged}
                onOccupancyOverrideChange={onOccupancyOverrideChange}
                recommendedTablesSlot={recommendedTablesSlot}
                enforceTodayMin={mode === "create"}
                onTableClearedForPartySize={onTableClearedForPartySize}
                showRequiredErrors={showRequiredErrors}
              />
            </ModalBody>
            <ModalFooter>
              <Button
                variant="light"
                className="text-ink-700 hover:bg-warm-100 hover:text-ink-950"
                onPress={close}
              >
                {cancelLabel}
              </Button>
              <Button
                className="bg-brand text-white hover:bg-brand-dark"
                onPress={() => {
                  setShowRequiredErrors(true);
                  onSubmit();
                }}
                isLoading={isSubmitting}
                isDisabled={isSubmitting || requiresOverride}
              >
                {submitLabel}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
