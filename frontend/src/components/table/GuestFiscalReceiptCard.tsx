"use client";

import React, { useEffect, useRef, useState } from "react";
import { FileText, ExternalLink } from "lucide-react";
import {
  getGuestFiscalReceipt,
  guestFiscalReceiptPdfUrl,
  type GuestFiscalReceipt,
} from "@/api/bills";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";

// GuestFiscalReceiptCard — shows the legal AFIP/ARCA factura for a settled
// bill on the guest thank-you view. Issuance is asynchronous (outbox worker,
// normally seconds), so while status is none/pending we poll a bounded number
// of times, then give up silently: the card renders nothing for businesses
// without fiscal invoicing, and the factura still arrives by email/print.
const POLL_INTERVAL_MS = 5000;
const MAX_POLLS = 12; // ~60s

const RECEIPT_TYPE_LABELS: Record<string, string> = {
  factura_a: "Factura A",
  factura_b: "Factura B",
  factura_c: "Factura C",
  nota_de_credito_a: "Nota de Crédito A",
  nota_de_credito_b: "Nota de Crédito B",
  nota_de_credito_c: "Nota de Crédito C",
};

interface GuestFiscalReceiptCardProps {
  billToken?: string;
  className?: string;
}

function GuestFiscalReceiptCard({
  billToken,
  className = "",
}: GuestFiscalReceiptCardProps) {
  const { t } = useGuestTranslation();
  const [receipt, setReceipt] = useState<GuestFiscalReceipt | null>(null);
  const pollCount = useRef(0);

  useEffect(() => {
    if (!billToken) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const poll = async () => {
      try {
        const data = await getGuestFiscalReceipt(billToken);
        if (cancelled) return;
        if (data.status === "authorized") {
          setReceipt(data);
          return;
        }
        pollCount.current += 1;
        if (pollCount.current < MAX_POLLS) {
          timer = setTimeout(poll, POLL_INTERVAL_MS);
        }
      } catch {
        // Endpoint unavailable / no fiscal setup — stay hidden.
      }
    };
    void poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [billToken]);

  if (!billToken || !receipt || receipt.status !== "authorized") {
    return null;
  }

  const typeLabel =
    RECEIPT_TYPE_LABELS[receipt.receipt_type ?? ""] ??
    (receipt.receipt_type ?? "").toUpperCase();

  return (
    <div
      className={`mb-6 rounded-2xl border border-warm-200 bg-white p-4 ${className}`}
      data-testid="guest-fiscal-receipt-card"
    >
      <div className="mb-2 flex items-center gap-2 text-brand-dark">
        <FileText className="h-4 w-4" strokeWidth={1.75} />
        <span className="text-sm font-semibold">
          {t("menu.fiscalReceipt.title")}
        </span>
      </div>
      <dl className="space-y-1 text-sm text-ink-700">
        <div className="flex justify-between gap-2">
          <dt>{t("menu.fiscalReceipt.type")}</dt>
          <dd className="font-medium">{typeLabel}</dd>
        </div>
        {receipt.receipt_number && (
          <div className="flex justify-between gap-2">
            <dt>{t("menu.fiscalReceipt.number")}</dt>
            <dd className="font-medium">{receipt.receipt_number}</dd>
          </div>
        )}
        {receipt.auth_code && (
          <div className="flex justify-between gap-2">
            <dt>{t("menu.fiscalReceipt.authCode")}</dt>
            <dd className="font-medium">{receipt.auth_code}</dd>
          </div>
        )}
      </dl>
      <div className="mt-3 flex flex-wrap gap-3">
        {receipt.pdf_available && (
          <a
            href={guestFiscalReceiptPdfUrl(billToken)}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 rounded-xl bg-brand px-3 py-2 text-sm font-medium text-white"
          >
            <FileText className="h-4 w-4" strokeWidth={1.75} />
            {t("menu.fiscalReceipt.downloadPdf")}
          </a>
        )}
        {receipt.qr_payload && (
          <a
            href={receipt.qr_payload}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 rounded-xl border border-warm-200 px-3 py-2 text-sm font-medium text-brand-dark"
          >
            <ExternalLink className="h-4 w-4" strokeWidth={1.75} />
            {t("menu.fiscalReceipt.verify")}
          </a>
        )}
      </div>
    </div>
  );
}

export default GuestFiscalReceiptCard;
