"use client";

import React, { useCallback, useRef, useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
  useDisclosure,
} from "@nextui-org/react";
import {
  Check,
  Copy,
  Download,
  ExternalLink,
  Minus,
  Plus,
  Settings2,
  Trash2,
  Users,
} from "lucide-react";
import QRCodeWithText from "../QRCodeWithText";
import { formatCurrency } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { StatusChip } from "@/components/ui/StatusChip";
import ConfirmationModal from "../modals/ConfirmationModal";
import { TableRowData } from "./TableRow";
import { markQRPreviewed } from "@/api/onboarding";
import { formatPreparedUnits } from "@/utils/formatPreparedUnits";
// L3-25: single source for the name cap — the same constant backs the BE
// binding tag, so the input must not carry its own literal.
import { TABLE_NAME_MAX_LENGTH } from "../tableNameLimits";

interface TableModalQRDefaults {
  qr_logo_url?: string;
  qr_foreground_color?: string;
  qr_background_color?: string;
  qr_logo_size?: number;
  qr_show_business_name?: boolean;
  qr_show_table_name?: boolean;
  qr_text_font?: string;
}

interface TableDetailModalProps {
  table: TableRowData & {
    qr_logo_url?: string;
    qr_logo_size?: number;
    qr_show_business_name?: boolean;
    qr_show_table_name?: boolean;
    qr_text_font?: string;
  };
  open: boolean;
  onClose: () => void;
  businessId: number;
  businessName?: string;
  // Business-level QR defaults used as a fallback when the row has no
  // per-table override yet.
  qrDefaults?: TableModalQRDefaults;
  // Currency code (USD / AED / EUR …) for the "Active bill" line.
  currency?: string;
  onDelete?: () => void;
  // Callbacks may return a promise; the modal awaits it to show a saving
  // indicator and, on rejection, resync its local draft to server truth.
  onToggleActive?: () => void | Promise<void>;
  onRename?: (name: string) => void | Promise<void>;
  // Persist a new seat count. Fires when the operator clicks +/- or
  // commits the input on blur. Omitted = capacity is read-only.
  onCapacityChange?: (capacity: number) => void | Promise<void>;
  onCustomizeQR?: (tableId: number) => void;
}

function pickWithDefault<T>(
  value: T | undefined,
  fallback: T | undefined,
  hard: T,
): T {
  if (value !== undefined && value !== null) return value;
  if (fallback !== undefined && fallback !== null) return fallback;
  return hard;
}

export default function TableDetailModal({
  table,
  open,
  onClose,
  businessId,
  businessName = "",
  qrDefaults,
  currency = "USD",
  onDelete,
  onToggleActive,
  onRename,
  onCapacityChange,
  onCustomizeQR,
}: TableDetailModalProps) {
  const { locale } = useSimpleLocale();
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.tableManager.detailModal.${key}`;
      const result = getTranslation(fullKey, locale);
      return typeof result === "string" && result.length > 0 ? result : key;
    },
    [locale],
  );
  // L3-29 / Root D: same branding key as QRCustomizationModal (required prop).
  const poweredByText = (() => {
    const result = getTranslation(
      "businessDashboard.dashboard.tableManager.qrCustomization.poweredBy",
      locale,
    );
    return typeof result === "string" && result.length > 0
      ? result
      : "Powered by Payverge";
  })();
  const url = `${typeof window !== "undefined" ? window.location.origin : ""}/t/${table.table_code}`;
  const [copied, setCopied] = useState(false);
  const [nameDraft, setNameDraft] = useState(table.name);
  const [capacityDraft, setCapacityDraft] = useState<number>(
    Number.isFinite(table.capacity) && table.capacity > 0 ? table.capacity : 4,
  );
  // Tracks an in-flight rename/capacity/activation write so we can show a
  // brief saving indicator and resync drafts to server truth on failure.
  const [saving, setSaving] = useState(false);
  // L3-25 residual: whitespace-only / empty rename needs visible feedback.
  const [renameError, setRenameError] = useState<string | null>(null);
  const canvasWrapRef = useRef<HTMLDivElement>(null);
  const recordedPreviewRef = useRef<string | null>(null);

  const handleQRRendered = useCallback(() => {
    if (!open || !table.table_code.trim()) return;
    const previewKey = `${businessId}:${table.id}`;
    if (recordedPreviewRef.current === previewKey) return;
    recordedPreviewRef.current = previewKey;
    void markQRPreviewed(businessId, table.id).catch(() => {
      // Keep the QR usable. Clearing the local guard lets a later reopen retry,
      // while server-side checkout continues to fail closed.
      recordedPreviewRef.current = null;
    });
  }, [businessId, open, table.id, table.table_code]);

  /* eslint-disable no-restricted-syntax -- canvas QR library expects hex,
     not Tailwind tokens. */
  const fg = pickWithDefault(
    (table as TableDetailModalProps["table"]).qr_foreground_color,
    qrDefaults?.qr_foreground_color,
    "#000000",
  );
  const bg = pickWithDefault(
    (table as TableDetailModalProps["table"]).qr_background_color,
    qrDefaults?.qr_background_color,
    "#FFFFFF",
  );
  /* eslint-enable no-restricted-syntax */
  const logoUrl = pickWithDefault(
    table.qr_logo_url,
    qrDefaults?.qr_logo_url,
    "",
  );
  const logoSize = pickWithDefault(
    table.qr_logo_size,
    qrDefaults?.qr_logo_size,
    20,
  );
  const showBusinessName = pickWithDefault(
    table.qr_show_business_name,
    qrDefaults?.qr_show_business_name,
    false,
  );
  const showTableName = pickWithDefault(
    table.qr_show_table_name,
    qrDefaults?.qr_show_table_name,
    false,
  );
  const font = pickWithDefault(
    table.qr_text_font,
    qrDefaults?.qr_text_font,
    "Verdana",
  );

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard?.writeText(url);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard blocked; the URL is also visible in the input field.
    }
  }, [url]);

  const handleDownload = useCallback(() => {
    const canvas = canvasWrapRef.current?.querySelector("canvas");
    if (!canvas) return;
    const link = document.createElement("a");
    link.download = `${table.table_code || "table"}-qr.png`;
    link.href = canvas.toDataURL("image/png");
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  }, [table.table_code]);

  const handleRenameCommit = useCallback(async () => {
    const next = nameDraft.trim();
    // L3-25 residual: whitespace-only or no-op renames must not leave the
    // draft showing text the server never accepted. Resync + surface why.
    if (!next) {
      setNameDraft(table.name);
      setRenameError(tString("nameRequired"));
      return;
    }
    if (next === table.name) {
      // Collapse pure whitespace / no-op edits back to server truth.
      if (nameDraft !== table.name) setNameDraft(table.name);
      setRenameError(null);
      return;
    }
    if (!onRename) return;
    setSaving(true);
    setRenameError(null);
    try {
      await onRename(next);
    } catch {
      // Parent already toasted; resync the draft to server truth so the
      // field doesn't keep showing the failed edit.
      setNameDraft(table.name);
    } finally {
      setSaving(false);
    }
  }, [nameDraft, table.name, onRename, tString]);

  const commitCapacity = useCallback(
    async (next: number) => {
      if (!Number.isFinite(next)) return;
      const bounded = Math.max(1, Math.min(99, Math.trunc(next)));
      setCapacityDraft(bounded);
      if (bounded === table.capacity || !onCapacityChange) return;
      setSaving(true);
      try {
        await onCapacityChange(bounded);
      } catch {
        // Resync the stepper to the last-persisted value on failure.
        setCapacityDraft(
          Number.isFinite(table.capacity) && table.capacity > 0
            ? table.capacity
            : 4,
        );
      } finally {
        setSaving(false);
      }
    },
    [table.capacity, onCapacityChange],
  );

  // Deactivate (active → inactive) hides the table from the floor plan and
  // reads as deletion — ConfirmationModal first (L3-28). Activate stays
  // single-click (restoring a table is not destructive).
  const {
    isOpen: isDeactivateOpen,
    onOpen: onDeactivateOpen,
    onOpenChange: onDeactivateOpenChange,
  } = useDisclosure();

  const runToggleActive = useCallback(async () => {
    if (!onToggleActive) return;
    setSaving(true);
    try {
      await onToggleActive();
    } catch {
      // Parent toasted; nothing local to resync (activation reads from table).
    } finally {
      setSaving(false);
    }
  }, [onToggleActive]);

  const handleToggleActiveClick = useCallback(() => {
    if (!onToggleActive) return;
    if (table.is_active) {
      onDeactivateOpen();
      return;
    }
    void runToggleActive();
  }, [onToggleActive, onDeactivateOpen, runToggleActive, table.is_active]);

  return (
    <>
    <Modal
      isOpen={open}
      onClose={onClose}
      size="2xl"
      scrollBehavior="inside"
      placement="center"
    >
      <ModalContent>
        {(close) => (
          <>
            <ModalHeader className="flex flex-col gap-1 pb-3">
              <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-ink-500 font-mono">
                {table.table_code}
              </p>
              <div className="flex items-center justify-between gap-3">
                <h2 className="font-title text-2xl text-ink-950">
                  {table.name}
                </h2>
                <StatusChip
                  kind="table"
                  status={table.status}
                  labelOverride={tString(`status.${table.status}`)}
                />
              </div>
            </ModalHeader>

            <ModalBody className="gap-5">
              {/* QR preview */}
              <div
                ref={canvasWrapRef}
                className="rounded-2xl border border-warm-100 bg-warm-50 p-4 flex items-center justify-center"
              >
                <QRCodeWithText
                  tableCode={table.table_code}
                  businessName={businessName}
                  tableName={table.name}
                  showBusinessName={showBusinessName}
                  showTableName={showTableName}
                  logoUrl={logoUrl || undefined}
                  foregroundColor={fg}
                  backgroundColor={bg}
                  logoSize={logoSize}
                  size={240}
                  textFont={font}
                  poweredByText={poweredByText}
                  ariaLabel={`${tString("qrCodeAria")} — ${table.name}`}
                  onRendered={handleQRRendered}
                />
              </div>

              {/* URL row + Copy */}
              <div className="flex items-center gap-2 rounded-xl border border-warm-100 bg-white px-3 py-2">
                <p className="text-[11px] font-semibold uppercase tracking-wider text-ink-500 shrink-0">
                  URL
                </p>
                <p className="text-sm text-ink-800 font-mono truncate flex-1">
                  {url}
                </p>
                <button
                  type="button"
                  onClick={handleCopy}
                  aria-label={tString("copyTableUrl")}
                  className="inline-flex items-center gap-1.5 text-xs font-semibold text-brand hover:text-brand-dark transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand rounded px-2 py-1"
                >
                  {copied ? (
                    <Check className="w-3.5 h-3.5" />
                  ) : (
                    <Copy className="w-3.5 h-3.5" />
                  )}
                  {copied ? tString("copied") : tString("copy")}
                </button>
              </div>

              {/* QR actions */}
              <div className="flex flex-wrap gap-2">
                <Button
                  className="bg-brand text-white hover:bg-brand-dark"
                  startContent={<Download className="w-4 h-4" />}
                  onPress={handleDownload}
                >
                  {tString("downloadPng")}
                </Button>
                {onCustomizeQR && (
                  <Button
                    variant="bordered"
                    startContent={<Settings2 className="w-4 h-4" />}
                    onPress={() => onCustomizeQR(table.id)}
                  >
                    {tString("customize")}
                  </Button>
                )}
                <Button
                  as="a"
                  href={`/t/${table.table_code}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="light"
                  startContent={<ExternalLink className="w-4 h-4" />}
                >
                  {tString("openPreview")}
                </Button>
              </div>

              {/* Quick details */}
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 rounded-xl border border-warm-100 bg-warm-50/60 p-3">
                <div className="flex items-center gap-2">
                  <Users
                    className="w-4 h-4 text-ink-400 shrink-0"
                    aria-hidden
                  />
                  <div>
                    <p className="text-[11px] font-semibold uppercase tracking-wider text-ink-500 mb-0.5">
                      {tString("seats")}
                    </p>
                    <p className="text-sm text-ink-900 tabular-nums">
                      {table.capacity > 0 ? table.capacity : "—"}
                    </p>
                  </div>
                </div>
                <div>
                  <p className="text-[11px] font-semibold uppercase tracking-wider text-ink-500 mb-0.5">
                    {tString("activeBill")}
                  </p>
                  {table.active_bill ? (
                    <p className="text-sm text-ink-900 tabular-nums">
                      {formatCurrency(
                        table.active_bill.total,
                        currency,
                        undefined,
                        intlLocaleFor(locale),
                      )}{" "}
                      ·{" "}
                      {formatPreparedUnits(
                        table.active_bill.physical_item_quantity,
                        tString,
                        "billItems",
                      )}
                    </p>
                  ) : (
                    <p className="text-sm text-ink-500">
                      {tString("noOpenBill")}
                    </p>
                  )}
                </div>
                <div>
                  <p className="text-[11px] font-semibold uppercase tracking-wider text-ink-500 mb-0.5">
                    {tString("activity")}
                  </p>
                  <p className="text-sm text-ink-700">
                    {table.is_active
                      ? tString("listedForGuests")
                      : tString("hiddenFromGuests")}
                  </p>
                </div>
              </div>

              {/* Edit details: rename + seats stepper. Both commit on
                  blur (or stepper click) so there's no separate Save
                  button to mind. */}
              {(onRename || onCapacityChange) && (
                <div className="rounded-xl border border-warm-100 bg-white px-3 py-3 space-y-4">
                  {onRename && (
                    <Input
                      label={tString("tableName")}
                      labelPlacement="outside"
                      placeholder={table.name}
                      value={nameDraft}
                      onValueChange={(v) => {
                        setNameDraft(v);
                        if (renameError) setRenameError(null);
                      }}
                      onBlur={handleRenameCommit}
                      maxLength={TABLE_NAME_MAX_LENGTH}
                      isInvalid={Boolean(renameError)}
                      errorMessage={renameError ?? undefined}
                      classNames={{
                        label: "text-xs uppercase tracking-wider text-ink-500",
                      }}
                      description={
                        saving
                          ? tString("saving")
                          : `${nameDraft.length}/${TABLE_NAME_MAX_LENGTH} · ${tString("blurToCommit")}`
                      }
                      data-testid="rename-table-name"
                    />
                  )}
                  {onCapacityChange && (
                    <div>
                      <label
                        htmlFor="table-seats"
                        className="block text-xs uppercase tracking-wider text-ink-500 mb-1.5"
                      >
                        {tString("seats")}
                      </label>
                      <div className="flex items-center gap-2">
                        <button
                          type="button"
                          onClick={() => commitCapacity(capacityDraft - 1)}
                          disabled={capacityDraft <= 1}
                          aria-label={tString("decreaseSeats")}
                          className="inline-flex items-center justify-center w-9 h-9 rounded-lg border border-warm-200 text-ink-700 hover:bg-warm-50 transition-colors disabled:opacity-40 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                        >
                          <Minus className="w-4 h-4" />
                        </button>
                        <input
                          id="table-seats"
                          type="number"
                          min={1}
                          max={99}
                          value={capacityDraft}
                          onChange={(e) =>
                            setCapacityDraft(parseInt(e.target.value, 10) || 1)
                          }
                          onBlur={(e) =>
                            commitCapacity(parseInt(e.target.value, 10) || 1)
                          }
                          aria-label={tString("seatsAtTable")}
                          className="w-16 text-center font-medium text-ink-900 tabular-nums border border-warm-200 rounded-lg py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                        />
                        <button
                          type="button"
                          onClick={() => commitCapacity(capacityDraft + 1)}
                          disabled={capacityDraft >= 99}
                          aria-label={tString("increaseSeats")}
                          className="inline-flex items-center justify-center w-9 h-9 rounded-lg border border-warm-200 text-ink-700 hover:bg-warm-50 transition-colors disabled:opacity-40 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                        >
                          <Plus className="w-4 h-4" />
                        </button>
                        <p className="text-xs text-ink-500 ml-2">
                          {tString("seatsHelp")}
                        </p>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </ModalBody>

            <ModalFooter className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex flex-wrap items-center gap-2">
                {onToggleActive && (
                  <Button
                    size="sm"
                    variant="light"
                    isLoading={saving}
                    onPress={handleToggleActiveClick}
                  >
                    {table.is_active
                      ? tString("deactivateTable")
                      : tString("activateTable")}
                  </Button>
                )}
                {onDelete && (
                  <Button
                    size="sm"
                    variant="light"
                    color="danger"
                    startContent={<Trash2 className="w-4 h-4" />}
                    onPress={onDelete}
                  >
                    {tString("deleteTable")}
                  </Button>
                )}
              </div>
              <Button variant="bordered" onPress={close}>
                {tString("close")}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>

      <ConfirmationModal
        isOpen={isDeactivateOpen}
        onOpenChange={onDeactivateOpenChange}
        isDanger
        title={tString("confirmDeactivateTitle")}
        description={tString("confirmDeactivateDescription")}
        confirmLabel={tString("confirmDeactivateAction")}
        cancelLabel={tString("close")}
        onConfirm={() => runToggleActive()}
      />
    </>
  );
}
