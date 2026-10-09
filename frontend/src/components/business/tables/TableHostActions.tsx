"use client";

import React, { useMemo, useState } from "react";
import Link from "next/link";
import {
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Button,
  Input,
  Select,
  SelectItem,
} from "@nextui-org/react";
import {
  ArrowRightLeft,
  ChefHat,
  Combine,
  MoreHorizontal,
  Receipt,
  UserPlus,
  Eraser,
} from "lucide-react";
import toast from "react-hot-toast";
import {
  clearTable,
  mergeTable,
  seatTable,
  transferTable,
} from "@/api/tableFloor";
import {
  getApiErrorCode,
  getApiErrorData,
  getApiErrorMessage,
} from "@/utils/apiError";
import type { TableRowData } from "./TableRow";

export interface HostActionTarget {
  id: number;
  name: string;
  status: TableRowData["status"];
  is_active: boolean;
  has_open_bill: boolean;
}

interface TableHostActionsProps {
  businessId: number;
  table: TableRowData;
  targets: HostActionTarget[];
  onChanged: () => void | Promise<void>;
  tString: (key: string, params?: Record<string, string | number>) => string;
}

type PickerMode = "transfer" | "merge" | null;

function floorErrorMessage(
  err: unknown,
  tString: TableHostActionsProps["tString"],
): string {
  const data = getApiErrorData(err) as { code?: string; error?: string } | null;
  const code = getApiErrorCode(err) || data?.code;
  if (code === "settle_required") return tString("hostActions.errors.settleRequired");
  if (code === "table_occupied") return tString("hostActions.errors.tableOccupied");
  if (code === "target_occupied") return tString("hostActions.errors.targetOccupied");
  if (code === "no_active_bill") return tString("hostActions.errors.noActiveBill");
  if (code === "merge_payments_block") return tString("hostActions.errors.mergePayments");
  if (code === "reservation_claimed") return tString("hostActions.errors.reservationClaimed");
  if (code === "same_table") return tString("hostActions.errors.sameTable");
  if (code === "kitchen_tickets_live")
    return tString("hostActions.errors.kitchenTicketsLive");
  if (code === "orders_pending_approval")
    return tString("hostActions.errors.ordersPendingApproval");
  return getApiErrorMessage(err) || tString("hostActions.errors.generic");
}

/**
 * Host/encargado Live View actions: seat, clear, transfer, merge.
 * Kept out of the QR/guest helpers so the floor list answers dinner-service asks.
 */
export default function TableHostActions({
  businessId,
  table,
  targets,
  onChanged,
  tString,
}: TableHostActionsProps) {
  const [busy, setBusy] = useState(false);
  const [seatOpen, setSeatOpen] = useState(false);
  const [covers, setCovers] = useState("");
  const [pickerMode, setPickerMode] = useState<PickerMode>(null);
  const [targetId, setTargetId] = useState<string>("");

  const canSeat =
    table.is_active &&
    !table.active_bill &&
    (table.status === "available" || table.status === "reserved");
  // Occupied without an open check is the #704 orphan: a closed/abandoned
  // bill still has tickets in the pass. Liberar must stay tappable so the
  // API can return kitchen_tickets_live instead of hiding the table as done.
  const canClear = table.is_active && (!!table.active_bill || table.status === "occupied");
  const canMove = table.is_active && !!table.active_bill;

  // #794: Floor actions used to open with Liberar as the only live control on
  // a leftover table (T2/T3/T5: occupied 1-3d, bills_count 0, tickets still in
  // the pass). The host had no way to answer "why is this table occupied?"
  // from the same panel, and Liberar is not the answer — it destroys the only
  // evidence. Lead with a destination: the open check when there is one, the
  // kitchen queue when the leftover tickets are the reason, and plain
  // "No open bill" when there is genuinely nothing to open.
  const openDestination = table.active_bill
    ? {
        href: `?tab=bills&billId=${table.active_bill.id}`,
        label: tString("hostActions.openBill"),
        description: undefined as string | undefined,
        icon: <Receipt className="w-4 h-4" />,
      }
    : table.status === "occupied"
      ? {
          // The row carries no ticket status, and a leftover ticket can be
          // approved / in_kitchen / ready — land on the unfiltered queue so it
          // is never an empty filtered view (mirrors TableRow O5).
          href: "?tab=kitchen&kitchenStatus=all",
          label: tString("hostActions.openKitchenTickets"),
          description: tString("hostActions.noOpenBill"),
          icon: <ChefHat className="w-4 h-4" />,
        }
      : null;

  const pickerTargets = useMemo(() => {
    return targets.filter((t) => {
      if (t.id === table.id || !t.is_active) return false;
      if (pickerMode === "transfer") {
        // Occupied-without-open-check is kitchen/queue orphan work — not a
        // free transfer target.
        return !t.has_open_bill && t.status !== "occupied";
      }
      // Merge can target free tables (becomes transfer) or occupied ones.
      return true;
    });
  }, [targets, table.id, pickerMode]);

  const run = async (fn: () => Promise<void>, successKey: string) => {
    if (busy) return;
    setBusy(true);
    try {
      await fn();
      toast.success(tString(successKey));
      await onChanged();
    } catch (err) {
      toast.error(floorErrorMessage(err, tString));
    } finally {
      setBusy(false);
    }
  };

  const handleSeat = async () => {
    const partySize = Number.parseInt(covers, 10);
    const reservationId = table.next_reservation?.id;
    await run(async () => {
      await seatTable(businessId, table.id, {
        party_size:
          Number.isFinite(partySize) && partySize > 0 ? partySize : undefined,
        reservation_id: reservationId,
      });
      setSeatOpen(false);
      setCovers("");
    }, "hostActions.toasts.seated");
  };

  const handleClear = async () => {
    // Always hit the clear door. A client-side settle short-circuit hid
    // kitchen_tickets_live / orders_pending_approval on an unpaid open check
    // (live T9 / bill 761). The API owns the priority: kitchen, then queue,
    // then settle_required.
    await run(async () => {
      await clearTable(businessId, table.id);
    }, "hostActions.toasts.cleared");
  };

  const handlePickerConfirm = async () => {
    const id = Number.parseInt(targetId, 10);
    if (!Number.isFinite(id) || id <= 0) {
      toast.error(tString("hostActions.errors.pickTarget"));
      return;
    }
    await run(async () => {
      if (pickerMode === "transfer") {
        await transferTable(businessId, table.id, { target_table_id: id });
      } else {
        await mergeTable(businessId, table.id, { target_table_id: id });
      }
      setPickerMode(null);
      setTargetId("");
    }, pickerMode === "transfer" ? "hostActions.toasts.transferred" : "hostActions.toasts.merged");
  };

  if (!table.is_active) return null;

  return (
    <>
      <Dropdown placement="bottom-end" isDisabled={busy}>
        <DropdownTrigger>
          <button
            type="button"
            data-testid="table-host-actions"
            onClick={(e) => e.stopPropagation()}
            aria-label={tString("hostActions.menuAria").replace("{name}", table.name)}
            title={tString("hostActions.menuTitle")}
            className="inline-flex items-center justify-center w-8 h-8 rounded-md text-ink-500 hover:text-brand hover:bg-warm-100 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-50"
          >
            <MoreHorizontal className="w-4 h-4" />
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label={tString("hostActions.menuTitle")}
          onAction={(key) => {
            if (key === "seat") setSeatOpen(true);
            if (key === "clear") void handleClear();
            if (key === "transfer") {
              setPickerMode("transfer");
              setTargetId("");
            }
            if (key === "merge") {
              setPickerMode("merge");
              setTargetId("");
            }
          }}
        >
          <DropdownItem
            key="open"
            startContent={
              openDestination?.icon ?? <Receipt className="w-4 h-4" />
            }
            isDisabled={!openDestination}
            description={openDestination?.description}
            textValue={openDestination?.label ?? tString("hostActions.noOpenBill")}
            {...(openDestination
              ? { as: Link, href: openDestination.href }
              : {})}
          >
            {openDestination?.label ?? tString("hostActions.noOpenBill")}
          </DropdownItem>
          <DropdownItem
            key="seat"
            startContent={<UserPlus className="w-4 h-4" />}
            isDisabled={!canSeat}
            textValue={tString("hostActions.seat")}
          >
            {table.next_reservation
              ? tString("hostActions.seatReservation")
              : tString("hostActions.seat")}
          </DropdownItem>
          <DropdownItem
            key="clear"
            startContent={<Eraser className="w-4 h-4" />}
            isDisabled={!canClear}
            textValue={tString("hostActions.clear")}
          >
            {tString("hostActions.clear")}
          </DropdownItem>
          <DropdownItem
            key="transfer"
            startContent={<ArrowRightLeft className="w-4 h-4" />}
            isDisabled={!canMove}
            textValue={tString("hostActions.transfer")}
          >
            {tString("hostActions.transfer")}
          </DropdownItem>
          <DropdownItem
            key="merge"
            startContent={<Combine className="w-4 h-4" />}
            isDisabled={!canMove}
            textValue={tString("hostActions.merge")}
          >
            {tString("hostActions.merge")}
          </DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Modal
        isOpen={seatOpen}
        onClose={() => !busy && setSeatOpen(false)}
        size="sm"
        placement="center"
      >
        <ModalContent onClick={(e) => e.stopPropagation()}>
          <ModalHeader className="flex flex-col gap-1">
            <span>{tString("hostActions.seatModal.title")}</span>
            <span className="text-body-sm font-normal text-ink-500">
              {table.next_reservation
                ? tString("hostActions.seatModal.reservationHint", {
                    name: table.next_reservation.customer_name,
                    covers: table.next_reservation.party_size,
                  })
                : tString("hostActions.seatModal.walkInHint", {
                    name: table.name,
                  })}
            </span>
          </ModalHeader>
          <ModalBody>
            {!table.next_reservation ? (
              <Input
                type="number"
                min={1}
                max={99}
                label={tString("hostActions.seatModal.coversLabel")}
                placeholder={tString("hostActions.seatModal.coversPlaceholder")}
                value={covers}
                onValueChange={setCovers}
                isDisabled={busy}
              />
            ) : null}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setSeatOpen(false)} isDisabled={busy}>
              {tString("hostActions.cancel")}
            </Button>
            <Button color="primary" onPress={() => void handleSeat()} isLoading={busy}>
              {tString("hostActions.seatConfirm")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal
        isOpen={pickerMode != null}
        onClose={() => !busy && setPickerMode(null)}
        size="sm"
        placement="center"
      >
        <ModalContent onClick={(e) => e.stopPropagation()}>
          <ModalHeader className="flex flex-col gap-1">
            <span>
              {pickerMode === "transfer"
                ? tString("hostActions.transferModal.title")
                : tString("hostActions.mergeModal.title")}
            </span>
            <span className="text-body-sm font-normal text-ink-500">
              {pickerMode === "transfer"
                ? tString("hostActions.transferModal.hint", { name: table.name })
                : tString("hostActions.mergeModal.hint", { name: table.name })}
            </span>
          </ModalHeader>
          <ModalBody>
            {pickerTargets.length === 0 ? (
              <p className="text-body-sm text-ink-600">
                {tString("hostActions.errors.noTargets")}
              </p>
            ) : (
              <Select
                label={tString("hostActions.targetLabel")}
                selectedKeys={targetId ? new Set([targetId]) : new Set()}
                onSelectionChange={(keys) => {
                  const value = Array.from(keys as Set<string>)[0] || "";
                  setTargetId(value);
                }}
                isDisabled={busy}
              >
                {pickerTargets.map((t) => (
                  <SelectItem key={String(t.id)} textValue={t.name}>
                    {t.name}
                    {t.has_open_bill
                      ? ` · ${tString("hostActions.targetOccupiedSuffix")}`
                      : ` · ${tString("hostActions.targetFreeSuffix")}`}
                  </SelectItem>
                ))}
              </Select>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setPickerMode(null)} isDisabled={busy}>
              {tString("hostActions.cancel")}
            </Button>
            <Button
              color="primary"
              onPress={() => void handlePickerConfirm()}
              isLoading={busy}
              isDisabled={pickerTargets.length === 0}
            >
              {pickerMode === "transfer"
                ? tString("hostActions.transferConfirm")
                : tString("hostActions.mergeConfirm")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
}
