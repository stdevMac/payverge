"use client";

import React from "react";
import {
  Button,
  Chip,
  Select,
  SelectItem,
} from "@nextui-org/react";
import { Edit, Mail, Phone, Users, XCircle } from "lucide-react";
import type { Reservation, ReservationStatus } from "@/api/reservations";
import type { Table as TableType } from "@/api/business";
import { StatusChip } from "@/components/ui/StatusChip";
import { PremiumPanel } from "../premium";
import { formatReservationHistoryNote } from "@/lib/reservationHistoryNote";
import { reservationDisplayStatus } from "./reservationArrivalClock";

const reservationSectionEyebrowClass =
  "text-xs font-semibold uppercase tracking-[0.18em] text-brand-700";
const reservationFieldIconClass = "w-4 h-4 text-ink-400";
const reservationDetailRowClass =
  "flex items-center justify-between gap-3 rounded-xl border border-warm-100 bg-warm-50/60 px-3 py-2";

export type ReservationDetailDrawerProps = {
  reservation: Reservation;
  drawerRef: React.RefObject<HTMLDivElement | null>;
  onClose: () => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  getStatusLabel: (status: ReservationStatus | string) => string;
  formatDateTime: (value: string) => string;
  canEdit: boolean;
  canDelete: boolean;
  tables: TableType[];
  actionLoadingId: number | null;
  renderRecommendedTableButtons: (
    reservation: Pick<Reservation, "party_size" | "table_id">,
    reservationId?: number,
  ) => React.ReactNode;
  renderQuickActionButtons: (reservation: Reservation) => React.ReactNode[];
  onAssignTable: (reservationId: number, tableId: number) => void;
  onEdit: (reservation: Reservation) => void;
  onCancel: (reservation: Reservation) => void;
};

export function ReservationDetailDrawer({
  reservation: selectedReservation,
  drawerRef: detailsDrawerRef,
  onClose,
  t,
  getStatusLabel,
  formatDateTime,
  canEdit,
  canDelete,
  tables,
  actionLoadingId,
  renderRecommendedTableButtons,
  renderQuickActionButtons,
  onAssignTable,
  onEdit,
  onCancel,
}: ReservationDetailDrawerProps) {
  const reservationCaps = { canEdit, canDelete };
  const handleAssignTable = async (reservationId: number, tableId: number) => {
    onAssignTable(reservationId, tableId);
  };
  const handleEditReservation = onEdit;
  const handleCancelReservation = onCancel;
  const setIsViewOpen = (open: boolean) => {
    if (!open) onClose();
  };

  return (
        // Backdrop: clicking it (but not the panel it contains) dismisses the
        // drawer. Keyboard dismissal is handled by useDialogBehavior (Escape),
        // so the backdrop itself needs no key handler — it is presentational
        // chrome, not a control.
        <div
          role="presentation"
          className="fixed inset-0 z-50 flex justify-end bg-black/30 backdrop-blur-sm"
          onClick={(event) => {
            if (event.target === event.currentTarget) {
              setIsViewOpen(false);
            }
          }}
        >
          <div
            ref={detailsDrawerRef}
            role="dialog"
            aria-modal="true"
            aria-label={t("guestDetails")}
            tabIndex={-1}
            className="h-full w-full max-w-xl overflow-y-auto border-l border-warm-200 bg-warm-50 shadow-2xl outline-none"
          >
            <div className="sticky top-0 z-10 border-b border-warm-200 bg-white/95 px-6 py-5 backdrop-blur">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="flex items-center gap-2">
                    <StatusChip
                      kind="reservation"
                      status={reservationDisplayStatus(selectedReservation)}
                      labelOverride={getStatusLabel(
                        reservationDisplayStatus(selectedReservation),
                      )}
                    />
                    {!selectedReservation.table_id ? (
                      <Chip size="sm" variant="flat" color="warning">
                        {t("needsTable")}
                      </Chip>
                    ) : null}
                  </div>
                  <h3 className="mt-3 text-xl font-semibold text-ink-950">
                    {selectedReservation.customer_name}
                  </h3>
                  <p className="mt-1 text-sm text-ink-600">
                    {formatDateTime(selectedReservation.reservation_time)} ·{" "}
                    {selectedReservation.party_size} {t("guests")}
                  </p>
                </div>
                <Button
                  variant="light"
                  className="text-ink-600 hover:bg-brand/5 hover:text-brand-700"
                  onPress={() => setIsViewOpen(false)}
                >
                  {t("close")}
                </Button>
              </div>
            </div>

            <div className="space-y-6 p-6">
              <PremiumPanel className="p-4" withTexture={false}>
                <div className="mb-3">
                  <h4 className={reservationSectionEyebrowClass}>
                    {t("guestDetails")}
                  </h4>
                </div>
                <div className="space-y-3 text-sm text-ink-700">
                  <div className="flex items-center gap-2">
                    <Users className={reservationFieldIconClass} />
                    <span>
                      {selectedReservation.party_size} {t("guests")}
                    </span>
                  </div>
                  {selectedReservation.customer_phone ? (
                    <div className="flex items-center gap-2">
                      <Phone className={reservationFieldIconClass} />
                      <span>{selectedReservation.customer_phone}</span>
                    </div>
                  ) : null}
                  {selectedReservation.customer_email ? (
                    <div className="flex items-center gap-2">
                      <Mail className={reservationFieldIconClass} />
                      <span>{selectedReservation.customer_email}</span>
                    </div>
                  ) : null}
                  {selectedReservation.confirmation_code ? (
                    <div className="rounded-xl border border-warm-200 bg-warm-50/80 px-3 py-2 text-xs text-ink-700">
                      {t("confirmationCodeLabel")}:{" "}
                      {selectedReservation.confirmation_code}
                    </div>
                  ) : null}
                </div>
              </PremiumPanel>

              <PremiumPanel className="p-4" withTexture={false}>
                <div className="mb-3">
                  <h4 className={reservationSectionEyebrowClass}>
                    {t("serviceDetails.title")}
                  </h4>
                </div>
                <div className="space-y-3 text-sm text-ink-700">
                  <div className={reservationDetailRowClass}>
                    <span>{t("serviceDetails.assignedTable")}</span>
                    <span className="font-medium text-ink-950">
                      {selectedReservation.table?.name ||
                        t("serviceDetails.unassigned")}
                    </span>
                  </div>
                  <div className={reservationDetailRowClass}>
                    <span>{t("serviceDetails.duration")}</span>
                    <span className="font-medium text-ink-950">
                      {selectedReservation.duration} {t("minutes")}
                    </span>
                  </div>
                  <div className={reservationDetailRowClass}>
                    <span>{t("serviceDetails.source")}</span>
                    <span className="font-medium capitalize text-ink-950">
                      {selectedReservation.source || t("serviceDetails.manual")}
                    </span>
                  </div>
                  {selectedReservation.waitlist_position ? (
                    <div className={reservationDetailRowClass}>
                      <span>{t("serviceDetails.waitlistPosition")}</span>
                      <span className="font-medium text-ink-950">
                        {selectedReservation.waitlist_position}
                      </span>
                    </div>
                  ) : null}
                </div>
              </PremiumPanel>

              {reservationCaps.canEdit &&
              !selectedReservation.table_id &&
              ["pending", "confirmed", "waitlist"].includes(
                selectedReservation.status,
              ) ? (
                <PremiumPanel className="p-4" withTexture={false}>
                  <div className="mb-3">
                    <h4 className={reservationSectionEyebrowClass}>
                      {t("assignTable")}
                    </h4>
                  </div>
                  <div className="space-y-4">
                    {renderRecommendedTableButtons(
                      selectedReservation,
                      selectedReservation.id,
                    )}
                    <Select
                      label={t("form.assignFromAllTables")}
                      selectedKeys={[]}
                      onChange={(e) => {
                        const nextTableId = Number.parseInt(e.target.value, 10);
                        if (!Number.isNaN(nextTableId)) {
                          void handleAssignTable(
                            selectedReservation.id,
                            nextTableId,
                          );
                        }
                      }}
                    >
                      {tables.map((table) => (
                        <SelectItem
                          key={String(table.id)}
                          value={String(table.id)}
                        >
                          {table.name} ({table.capacity} {t("seats")})
                        </SelectItem>
                      ))}
                    </Select>
                  </div>
                </PremiumPanel>
              ) : null}

              {selectedReservation.special_requests ? (
                <PremiumPanel className="p-4" withTexture={false}>
                  <div className="mb-3">
                    <h4 className={reservationSectionEyebrowClass}>
                      {t("specialRequests")}
                    </h4>
                  </div>
                  <div className="text-sm leading-6 text-ink-700">
                    {selectedReservation.special_requests}
                  </div>
                </PremiumPanel>
              ) : null}

              {selectedReservation.notes ? (
                <PremiumPanel className="p-4" withTexture={false}>
                  <div className="mb-3">
                    <h4 className={reservationSectionEyebrowClass}>
                      {t("internalNotes")}
                    </h4>
                  </div>
                  <div className="text-sm leading-6 text-ink-700">
                    {selectedReservation.notes}
                  </div>
                </PremiumPanel>
              ) : null}

              {(reservationCaps.canEdit || reservationCaps.canDelete) && (
                <PremiumPanel
                  data-testid="reservation-detail-quick-actions"
                  className="p-4"
                  withTexture={false}
                >
                  <div className="mb-3 flex items-center justify-between gap-3">
                    <h4 className={reservationSectionEyebrowClass}>
                      {t("quickActions")}
                    </h4>
                    {reservationCaps.canEdit && (
                      <Button
                        aria-label={`${t("edit")} ${selectedReservation.customer_name}, ${formatDateTime(selectedReservation.reservation_time)}`}
                        size="sm"
                        variant="flat"
                        startContent={<Edit className="w-4 h-4" />}
                        onPress={() =>
                          handleEditReservation(selectedReservation)
                        }
                      >
                        {t("edit")}
                      </Button>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {renderQuickActionButtons(selectedReservation)}
                    {reservationCaps.canDelete && (
                      <Button
                        aria-label={`${t("cancel")} ${selectedReservation.customer_name}, ${formatDateTime(selectedReservation.reservation_time)}`}
                        size="sm"
                        variant="flat"
                        color="danger"
                        startContent={<XCircle className="w-4 h-4" />}
                        onPress={() =>
                          handleCancelReservation(selectedReservation)
                        }
                        isLoading={actionLoadingId === selectedReservation.id}
                      >
                        {t("cancel")}
                      </Button>
                    )}
                  </div>
                </PremiumPanel>
              )}

              <PremiumPanel className="p-4" withTexture={false}>
                <div className="mb-3">
                  <h4 className={reservationSectionEyebrowClass}>
                    {t("statusHistory")}
                  </h4>
                </div>
                <div className="space-y-3">
                  {selectedReservation.status_history?.length ? (
                    selectedReservation.status_history.map((entry) => (
                      <div
                        key={entry.id}
                        className="rounded-2xl border border-warm-200 bg-white/80 px-4 py-3"
                      >
                        <div className="flex items-center justify-between gap-3">
                          <p className="font-medium text-ink-950">
                            {getStatusLabel(entry.status)}
                          </p>
                          <p className="text-xs text-ink-600">
                            {formatDateTime(entry.created_at)}
                          </p>
                        </div>
                        {entry.table?.name ? (
                          <p className="mt-1 text-sm text-ink-700">
                            {t("tableLabel")}: {entry.table.name}
                          </p>
                        ) : null}
                        {entry.notes ? (
                          <p className="mt-1 text-sm text-ink-700">
                            {formatReservationHistoryNote(entry.notes, t)}
                          </p>
                        ) : null}
                        {entry.changed_by ? (
                          <p className="mt-1 text-xs text-ink-500">
                            {entry.changed_by}
                          </p>
                        ) : null}
                      </div>
                    ))
                  ) : (
                    <p className="text-sm text-ink-600">{t("noHistory")}</p>
                  )}
                </div>
              </PremiumPanel>
            </div>
          </div>
        </div>

  );
}
