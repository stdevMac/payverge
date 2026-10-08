"use client";

import React, { useEffect, useState } from "react";
import {
  Button,
  Chip,
  Drawer,
  DrawerBody,
  DrawerContent,
  Input,
  Spinner,
  Textarea,
} from "@nextui-org/react";
import { useReducedMotion } from "framer-motion";
import {
  ExternalLink,
  MapPin,
  Phone,
  Truck,
  User,
  X,
} from "lucide-react";
import {
  deliveryApi,
  type DeliveryDriver,
  type DeliveryOrder,
  type DeliveryStatus,
} from "@/api/delivery";
import { formatCurrency } from "@/api/currency";
import { formatBusinessTime } from "@/utils/businessTime";
import { DriverAssignment } from "./DriverAssignment";
import { getOperatorNextStatus } from "./OrderRow";

export interface OrderDetailDrawerProps {
  businessId: number;
  orderId: number | null;
  isOpen: boolean;
  onClose: () => void;
  drivers: DeliveryDriver[];
  onAdvance: (orderId: number, nextStatus: DeliveryStatus) => Promise<void>;
  onAssignDriver: (orderId: number, driverId: number) => Promise<void>;
  onCancel: (order: DeliveryOrder) => void;
  tString: (key: string, vars?: Record<string, string>) => string;
  currency?: string;
  locale?: string;
  businessTimezone?: string | null;
  /** Optional callback after a successful detail mutation so the board refreshes. */
  onMutated?: () => void;
}

function formatAddress(order: DeliveryOrder): string {
  const a = order.delivery_address;
  if (!a) return "";
  if (a.formatted_address) return a.formatted_address;
  return [a.street, a.apartment, a.city, a.state, a.postal_code, a.country]
    .filter(Boolean)
    .join(", ");
}

/**
 * Operator order detail drawer. Uses NextUI Drawer (focus trap, Esc/backdrop
 * dismiss) and disables animation under prefers-reduced-motion — same contract
 * as DirectorConsole/InsightsDrawer.
 */
export default function OrderDetailDrawer({
  businessId,
  orderId,
  isOpen,
  onClose,
  drivers,
  onAdvance,
  onAssignDriver,
  onCancel,
  tString,
  currency = "USD",
  locale = "en",
  businessTimezone = null,
  onMutated,
}: OrderDetailDrawerProps) {
  const prefersReducedMotion = !!useReducedMotion();
  const [detail, setDetail] = useState<DeliveryOrder | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [advancing, setAdvancing] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editPhone, setEditPhone] = useState("");
  const [editApartment, setEditApartment] = useState("");
  const [editInstructions, setEditInstructions] = useState("");
  const [savingEdit, setSavingEdit] = useState(false);

  useEffect(() => {
    if (!isOpen || orderId == null) {
      setDetail(null);
      setError(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(false);
    deliveryApi
      .getDeliveryOrder(businessId, orderId)
      .then((order) => {
        if (!cancelled) setDetail(order);
      })
      .catch(() => {
        if (!cancelled) setError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, orderId, businessId]);

  const nextStatus = detail ? getOperatorNextStatus(detail.status) : null;
  const isTerminal = ["delivered", "cancelled", "failed"].includes(
    detail?.status ?? "",
  );
  const history = [...(detail?.status_history ?? [])].sort(
    (a, b) =>
      new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );

  const handleAdvance = async () => {
    if (!detail || !nextStatus) return;
    setAdvancing(true);
    try {
      await onAdvance(detail.id, nextStatus);
      onMutated?.();
      const refreshed = await deliveryApi.getDeliveryOrder(businessId, detail.id);
      setDetail(refreshed);
    } catch {
      // Parent surfaces toast.
    } finally {
      setAdvancing(false);
    }
  };

  const handleAssign = async (driverId: number) => {
    if (!detail) return;
    await onAssignDriver(detail.id, driverId);
    onMutated?.();
    const refreshed = await deliveryApi.getDeliveryOrder(businessId, detail.id);
    setDetail(refreshed);
  };

  // Operator dashboard deep-link (same shape Kitchen uses) — /business/:id
  // without /dashboard is the guest-facing page.
  const billHref =
    detail?.bill_id != null
      ? `/business/${businessId}/dashboard?tab=bills&billId=${detail.bill_id}`
      : null;

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      placement="right"
      size="lg"
      disableAnimation={prefersReducedMotion}
      classNames={{
        base: "bg-warm-50",
        closeButton: "text-ink-500 hover:bg-warm-100",
      }}
    >
      <DrawerContent
        data-testid="delivery-order-detail-drawer"
        aria-label={tString("dispatch.detail.title") || "Delivery order detail"}
      >
        {() => (
          <DrawerBody className="px-5 py-6">
            <div className="mb-4 flex items-start justify-between gap-3">
              <div>
                <h2 className="font-title text-heading-md text-ink-900">
                  {detail
                    ? `#${detail.delivery_number}`
                    : tString("dispatch.detail.title")}
                </h2>
                {detail && (
                  <Chip size="sm" variant="flat" className="mt-2">
                    {tString(`dispatch.columns.${detail.status}`) ||
                      detail.status}
                  </Chip>
                )}
              </div>
              <button
                type="button"
                data-testid="delivery-order-detail-close"
                onClick={onClose}
                aria-label={tString("dispatch.detail.close") || "Close"}
                className="rounded-full p-1.5 text-ink-500 transition-colors hover:bg-warm-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {loading && (
              <div
                className="flex justify-center py-16"
                data-testid="delivery-order-detail-loading"
              >
                <Spinner size="lg" color="primary" />
              </div>
            )}

            {error && !loading && (
              <div
                data-testid="delivery-order-detail-error"
                className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
              >
                {tString("dispatch.detail.loadError")}
              </div>
            )}

            {detail && !loading && (
              <div className="space-y-6">
                {/* Customer */}
                <section className="space-y-2">
                  <h3 className="text-label font-semibold uppercase tracking-wide text-ink-500">
                    {tString("dispatch.detail.customer")}
                  </h3>
                  <div className="flex items-center gap-2 text-ink-900">
                    <User className="h-4 w-4 text-ink-400" />
                    <span className="font-medium">{detail.customer_name}</span>
                  </div>
                  {detail.customer_phone && (
                    <a
                      href={`tel:${detail.customer_phone}`}
                      className="inline-flex items-center gap-2 text-brand hover:underline"
                      data-testid="delivery-order-detail-phone"
                    >
                      <Phone className="h-4 w-4" />
                      {detail.customer_phone}
                    </a>
                  )}
                </section>

                {/* Address */}
                <section className="space-y-2">
                  <h3 className="text-label font-semibold uppercase tracking-wide text-ink-500">
                    {tString("dispatch.detail.address")}
                  </h3>
                  <div className="flex items-start gap-2 text-sm text-ink-800">
                    <MapPin className="mt-0.5 h-4 w-4 shrink-0 text-ink-400" />
                    <div>
                      <p>{formatAddress(detail)}</p>
                      {detail.delivery_address?.apartment && (
                        <p className="mt-1 text-ink-600">
                          {tString("dispatch.detail.apartment")}:{" "}
                          {detail.delivery_address.apartment}
                        </p>
                      )}
                    </div>
                  </div>
                  {detail.delivery_instructions && (
                    <p className="rounded-lg bg-warm-100 px-3 py-2 text-sm text-ink-700">
                      {detail.delivery_instructions}
                    </p>
                  )}
                  <div className="flex flex-wrap gap-2">
                    {detail.contactless_delivery && (
                      <Chip size="sm" variant="flat" color="primary">
                        {tString("dispatch.detail.contactless")}
                      </Chip>
                    )}
                    {detail.leave_at_door && (
                      <Chip size="sm" variant="flat" color="secondary">
                        {tString("dispatch.detail.leaveAtDoor")}
                      </Chip>
                    )}
                  </div>
                </section>

                {/* Money + payment */}
                <section className="grid grid-cols-2 gap-3 text-sm">
                  <div className="rounded-xl border border-warm-200 bg-white p-3">
                    <p className="text-ink-500">
                      {tString("dispatch.detail.fee")}
                    </p>
                    <p className="mt-1 font-medium text-ink-900">
                      {formatCurrency(Number(detail.delivery_fee ?? 0), currency)}
                    </p>
                  </div>
                  <div className="rounded-xl border border-warm-200 bg-white p-3">
                    <p className="text-ink-500">
                      {tString("dispatch.detail.tip")}
                    </p>
                    <p className="mt-1 font-medium text-ink-900">
                      {formatCurrency(Number(detail.driver_tip ?? 0), currency)}
                    </p>
                  </div>
                  <div className="col-span-2 rounded-xl border border-warm-200 bg-white p-3">
                    <p className="text-ink-500">
                      {tString("dispatch.detail.payment")}
                    </p>
                    <p className="mt-1 font-medium text-ink-900">
                      {detail.payment_mode_stored
                        ? tString(
                            `dispatch.detail.paymentMode.${detail.payment_mode_stored}`,
                          ) || detail.payment_mode_stored
                        : tString("dispatch.detail.paymentUnknown")}
                    </p>
                  </div>
                </section>

                {/* Bill deep-link */}
                {billHref && (
                  <a
                    href={billHref}
                    data-testid="delivery-order-detail-bill-link"
                    className="inline-flex items-center gap-2 text-sm font-medium text-brand hover:underline"
                  >
                    <ExternalLink className="h-4 w-4" />
                    {tString("dispatch.detail.viewBill", {
                      id: String(detail.bill_id),
                    })}
                  </a>
                )}

                {/* Driver */}
                <section className="space-y-2">
                  <h3 className="text-label font-semibold uppercase tracking-wide text-ink-500">
                    {tString("dispatch.detail.driver")}
                  </h3>
                  {detail.driver ? (
                    <div className="flex items-center gap-2 text-sm text-ink-800">
                      <Truck className="h-4 w-4 text-ink-400" />
                      <span>
                        {detail.driver.name ||
                          detail.driver.email ||
                          tString("dispatch.detail.driverUnnamed")}
                      </span>
                      {detail.driver.phone && (
                        <a
                          href={`tel:${detail.driver.phone}`}
                          className="text-brand hover:underline"
                        >
                          {detail.driver.phone}
                        </a>
                      )}
                    </div>
                  ) : (
                    <p className="text-sm text-ink-500">
                      {tString("dispatch.detail.noDriver")}
                    </p>
                  )}
                  {!isTerminal && !detail.driver && (
                    <DriverAssignment
                      drivers={drivers}
                      onAssign={handleAssign}
                      tString={tString}
                    />
                  )}
                </section>

                {/* Timeline */}
                <section className="space-y-2">
                  <h3 className="text-label font-semibold uppercase tracking-wide text-ink-500">
                    {tString("dispatch.detail.timeline")}
                  </h3>
                  {history.length === 0 ? (
                    <p className="text-sm text-ink-500">
                      {tString("dispatch.detail.timelineEmpty")}
                    </p>
                  ) : (
                    <ol
                      className="relative space-y-3 border-l border-warm-200 pl-4"
                      data-testid="delivery-order-detail-timeline"
                    >
                      {history.map((entry) => (
                        <li key={entry.id} className="relative text-sm">
                          <span className="absolute -left-[1.3rem] top-1 h-2.5 w-2.5 rounded-full bg-brand" />
                          <p className="font-medium text-ink-900">
                            {tString(`dispatch.columns.${entry.status}`) ||
                              entry.status}
                          </p>
                          <p className="text-xs text-ink-500">
                            {formatBusinessTime(
                              entry.created_at,
                              locale,
                              businessTimezone,
                              {
                                month: "short",
                                day: "numeric",
                                hour: "numeric",
                                minute: "2-digit",
                              },
                            )}
                            {entry.changed_by
                              ? ` · ${entry.changed_by}`
                              : ""}
                          </p>
                          {entry.notes && (
                            <p className="mt-0.5 text-ink-600">{entry.notes}</p>
                          )}
                        </li>
                      ))}
                    </ol>
                  )}
                </section>

                {/* Operator edit — contact / apartment / instructions */}
                {!isTerminal && (
                  <section className="space-y-2 border-t border-warm-200 pt-4">
                    {!editing ? (
                      <Button
                        size="sm"
                        variant="flat"
                        data-testid="delivery-order-detail-edit"
                        onPress={() => {
                          setEditPhone(detail.customer_phone || "");
                          setEditApartment(
                            detail.delivery_address?.apartment || "",
                          );
                          setEditInstructions(
                            detail.delivery_instructions || "",
                          );
                          setEditing(true);
                        }}
                      >
                        {tString("dispatch.detail.edit")}
                      </Button>
                    ) : (
                      <div className="space-y-3" data-testid="delivery-order-detail-edit-form">
                        <Input
                          label={tString("dispatch.detail.editPhone")}
                          value={editPhone}
                          onValueChange={setEditPhone}
                          variant="bordered"
                        />
                        <Input
                          label={tString("dispatch.detail.editApartment")}
                          value={editApartment}
                          onValueChange={setEditApartment}
                          variant="bordered"
                        />
                        <Textarea
                          label={tString("dispatch.detail.editInstructions")}
                          value={editInstructions}
                          onValueChange={setEditInstructions}
                          variant="bordered"
                          minRows={2}
                        />
                        <div className="flex gap-2">
                          <Button
                            size="sm"
                            color="primary"
                            isLoading={savingEdit}
                            data-testid="delivery-order-detail-edit-save"
                            onPress={() => {
                              void (async () => {
                                setSavingEdit(true);
                                try {
                                  const updated =
                                    await deliveryApi.patchDeliveryOrder(
                                      businessId,
                                      detail.id,
                                      {
                                        customer_phone: editPhone,
                                        apartment: editApartment,
                                        delivery_instructions: editInstructions,
                                      },
                                    );
                                  setDetail(updated);
                                  setEditing(false);
                                  onMutated?.();
                                } catch {
                                  // Parent toast path not wired; surface via loadError pattern.
                                  setError(true);
                                } finally {
                                  setSavingEdit(false);
                                }
                              })();
                            }}
                          >
                            {tString("dispatch.detail.saveEdit")}
                          </Button>
                          <Button
                            size="sm"
                            variant="light"
                            onPress={() => setEditing(false)}
                          >
                            {tString("dispatch.detail.cancelEdit")}
                          </Button>
                        </div>
                      </div>
                    )}
                  </section>
                )}

                {/* Actions */}
                {!isTerminal && (
                  <div className="flex flex-wrap gap-2 border-t border-warm-200 pt-4">
                    {nextStatus && (
                      <Button
                        color="primary"
                        isLoading={advancing}
                        data-testid="delivery-order-detail-advance"
                        onPress={() => void handleAdvance()}
                      >
                        {tString("dispatch.actions.advance", {
                          next:
                            tString(`dispatch.columns.${nextStatus}`) ||
                            nextStatus,
                        })}
                      </Button>
                    )}
                    <Button
                      color="danger"
                      variant="flat"
                      data-testid="delivery-order-detail-cancel"
                      onPress={() => onCancel(detail)}
                    >
                      {tString("dispatch.actions.cancel")}
                    </Button>
                  </div>
                )}
              </div>
            )}
          </DrawerBody>
        )}
      </DrawerContent>
    </Drawer>
  );
}
