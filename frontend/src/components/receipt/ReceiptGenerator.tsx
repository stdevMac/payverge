/* eslint-disable no-restricted-syntax -- html2canvas / jsPDF require hex color strings */
"use client";

import React, { useRef, useState } from "react";
import { Button } from "@nextui-org/react";
import Image from "next/image";
import { canOptimizeImageSrc } from "@/config/imageOrigins";
import toast from "react-hot-toast";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import {
  formatGuestCurrency,
  formatGuestRate,
} from "@/utils/guestCurrencyFormatter";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { operatorBillDisplayNumber } from "@/lib/operatorBillNumber";
import { emailBillReceipt } from "@/api/bills";

interface ReceiptItem {
  name: string;
  quantity: number;
  subtotal: number;
}

interface ReceiptData {
  business: {
    name: string;
    logo?: string;
    address?: {
      street?: string;
      city?: string;
      state?: string;
      postal_code?: string;
      country?: string;
    };
    phone?: string;
    email?: string;
    tax_rate?: number;
    service_fee_rate?: number;
    /** Venue IANA timezone for bill/receipt timestamps (not device TZ). */
    timezone?: string;
    google_reviews_enabled?: boolean;
    google_review_link?: string;
    trustpilot_enabled?: boolean;
    trustpilot_review_url?: string;
  };
  bill: {
    bill_number: string;
    created_at: string;
    subtotal: number;
    tax_amount: number;
    service_fee_amount: number;
    // Loyalty redemption baked into total_amount by the backend (dollars).
    // Rendered as a deduction line so the receipt's items reconcile with the
    // total. Optional/absent or 0 when no points were redeemed.
    loyalty_discount?: number;
    total_amount: number;
  };
  table: {
    name: string;
  };
  items: ReceiptItem[];
  paymentDetails?: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId?: string;
  };
}

interface ReceiptGeneratorProps {
  data: ReceiptData;
  onDownloadReceipt?: (pdfBlob: Blob) => void;
  className?: string;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  /** When present, renders the email-my-receipt form (guest bill surface). */
  emailReceipt?: { billToken: string };
}

export default function ReceiptGenerator({
  data,
  onDownloadReceipt,
  className = "",
  currency = "USD",
  emailReceipt,
}: ReceiptGeneratorProps) {
  const receiptRef = useRef<HTMLDivElement>(null);
  const { t, currentLanguage } = useGuestTranslation();
  const fmtCurrency = (amount: number, code: string) =>
    formatGuestCurrency(amount, code, currentLanguage);
  const [email, setEmail] = useState("");
  const [emailSending, setEmailSending] = useState(false);
  const [emailSent, setEmailSent] = useState(false);

  const handleEmailReceipt = async () => {
    if (!emailReceipt) return;
    const trimmed = email.trim();
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)) {
      toast.error(t("receipt.emailInvalid"));
      return;
    }
    setEmailSending(true);
    try {
      await emailBillReceipt(emailReceipt.billToken, trimmed, currentLanguage, {
        paymentMethod: data.paymentDetails?.paymentMethod,
        transactionId: data.paymentDetails?.transactionId,
      });
      setEmailSent(true);
      toast.success(t("receipt.emailSent"));
    } catch {
      toast.error(t("receipt.emailFailed"));
    } finally {
      setEmailSending(false);
    }
  };

  const generatePDF = async (): Promise<Blob> => {
    if (!receiptRef.current) throw new Error("Receipt ref not available");

    // Lazy-load the heavy PDF/canvas libraries only when a receipt is actually
    // generated, keeping them out of the initial guest bundle. Output shape is
    // unchanged: html2canvas is the module default; jspdf exports `{ jsPDF }`.
    const html2canvas = (await import("html2canvas")).default;
    const { jsPDF } = await import("jspdf");

    const canvas = await html2canvas(receiptRef.current, {
      scale: 2,
      useCORS: true,
      allowTaint: true,
      backgroundColor: "#ffffff",
    });

    const imgData = canvas.toDataURL("image/png");
    const pdf = new jsPDF("p", "mm", "a4");

    // Calculate dimensions to fit on single page with margins
    const pageWidth = 210; // A4 width in mm
    const pageHeight = 297; // A4 height in mm
    const margin = 10; // 10mm margin on all sides
    const maxWidth = pageWidth - margin * 2;
    const maxHeight = pageHeight - margin * 2;

    // Calculate the aspect ratio and fit the image
    const imgAspectRatio = canvas.width / canvas.height;
    let imgWidth = maxWidth;
    let imgHeight = imgWidth / imgAspectRatio;

    // If height exceeds page, scale down based on height
    if (imgHeight > maxHeight) {
      imgHeight = maxHeight;
      imgWidth = imgHeight * imgAspectRatio;
    }

    // Center the image on the page
    const xPosition = (pageWidth - imgWidth) / 2;
    const yPosition = (pageHeight - imgHeight) / 2;

    pdf.addImage(imgData, "PNG", xPosition, yPosition, imgWidth, imgHeight);

    return pdf.output("blob");
  };

  const handleDownloadReceipt = async () => {
    try {
      const pdfBlob = await generatePDF();
      if (onDownloadReceipt) {
        onDownloadReceipt(pdfBlob);
      } else {
        // Fallback: create download link
        const url = URL.createObjectURL(pdfBlob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `receipt-${data.bill.bill_number}.pdf`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
      }
    } catch (error) {
      console.error("Error generating PDF:", error);
      toast.error(t("receipt.generateError") || "Error generating receipt. Please try again.");
    }
  };

  const formatDate = (dateString: string) => {
    // Guest language for month/hour names; venue timezone for wall-clock so a
    // receipt generated on a device abroad still matches restaurant time
    // (FIND-023 family — same honesty as GuestBill + delivery track).
    return formatBusinessDateTime(
      dateString,
      currentLanguage || "en",
      data.business.timezone ?? null,
      DATE_TIME_SHORT,
    );
  };

  const formatAddress = (address: any) => {
    if (!address) return "";
    const parts = [
      address.street,
      address.city,
      address.state,
      address.postal_code,
      address.country,
    ].filter(Boolean);
    return parts.join(", ");
  };

  return (
    <div className={className}>
      {/* Hidden Receipt for Generation */}
      <div
        ref={receiptRef}
        className="bg-white p-6 max-w-md mx-auto"
        style={{
          position: "absolute",
          left: "-9999px",
          top: "-9999px",
          width: "384px", // Fixed width for consistent generation
        }}
      >
        {/* Business Header */}
        <div className="text-center mb-4">
          {data.business.logo && (
            <Image
              src={data.business.logo}
              unoptimized={!canOptimizeImageSrc(data.business.logo)}
              alt={data.business.name}
              width={48}
              height={48}
              className="mx-auto mb-2 object-contain"
            />
          )}
          <h1 className="text-xl font-bold text-gray-900 mb-1">
            {data.business.name}
          </h1>
          {data.business.address && (
            <p className="text-sm text-gray-600 mb-1">
              {formatAddress(data.business.address)}
            </p>
          )}
          {data.business.phone && (
            <p className="text-sm text-gray-600 mb-1">
              {t("receipt.phone")}: {data.business.phone}
            </p>
          )}
          {data.business.email && (
            <p className="text-sm text-gray-600">
              {t("receipt.email")}: {data.business.email}
            </p>
          )}
        </div>

        <div className="border-t border-b border-gray-300 py-3 mb-3">
          <div className="flex justify-between mb-1">
            <span className="font-semibold text-sm">
              {t("receipt.receiptNumber")}:
            </span>
            <span className="text-sm">
              {operatorBillDisplayNumber({
                bill_number: data.bill.bill_number,
              })}
            </span>
          </div>
          <div className="flex justify-between mb-1">
            <span className="font-semibold text-sm">{t("receipt.table")}:</span>
            <span className="text-sm">{data.table.name}</span>
          </div>
          <div className="flex justify-between mb-1">
            <span className="font-semibold text-sm">{t("receipt.date")}:</span>
            <span className="text-sm">{formatDate(data.bill.created_at)}</span>
          </div>
          {data.paymentDetails?.transactionId && (
            <div className="flex justify-between">
              <span className="font-semibold text-sm">
                {t("receipt.transactionId")}:
              </span>
              <span className="text-xs">
                {data.paymentDetails.transactionId}
              </span>
            </div>
          )}
        </div>

        {/* Items */}
        <div className="mb-3">
          <h3 className="font-semibold mb-2 text-sm">
            {t("receipt.orderDetails")}
          </h3>
          {data.items.map((item, index) => (
            <div
              key={index}
              className="flex justify-between items-center py-1 border-b border-gray-100"
            >
              <div className="flex-1">
                <div className="font-medium text-sm">{item.name}</div>
                <div className="text-xs text-gray-600">
                  {t("receipt.qty")}: {item.quantity}
                </div>
              </div>
              <div className="font-medium text-sm">
                {fmtCurrency(item.subtotal, currency)}
              </div>
            </div>
          ))}
        </div>

        {/* Totals */}
        <div className="border-t border-gray-300 pt-3 space-y-1">
          <div className="flex justify-between text-sm">
            <span>{t("receipt.subtotal")}:</span>
            <span>{fmtCurrency(data.bill.subtotal, currency)}</span>
          </div>
          {data.bill.tax_amount > 0 && (
            <div className="flex justify-between text-sm">
              <span>
                {t("receipt.tax")}{" "}
                {data.business.tax_rate
                  ? `(${formatGuestRate(data.business.tax_rate, currentLanguage)}%)`
                  : ""}
                :
              </span>
              <span>{fmtCurrency(data.bill.tax_amount, currency)}</span>
            </div>
          )}
          {data.bill.service_fee_amount > 0 && (
            <div className="flex justify-between text-sm">
              <span>
                {t("receipt.serviceFee")}{" "}
                {data.business.service_fee_rate
                  ? `(${formatGuestRate(data.business.service_fee_rate, currentLanguage)}%)`
                  : ""}
                :
              </span>
              <span>{fmtCurrency(data.bill.service_fee_amount, currency)}</span>
            </div>
          )}
          {(data.bill.loyalty_discount ?? 0) > 0 && (
            <div className="flex justify-between text-green-600 text-sm">
              {/* Reuses the already-translated bill.loyaltyDiscount key (present
                  in all 21 guest bundles) rather than adding a receipt.* key. */}
              <span>{t("bill.loyaltyDiscount")}:</span>
              <span>
                -{fmtCurrency(data.bill.loyalty_discount ?? 0, currency)}
              </span>
            </div>
          )}
          {data.paymentDetails?.tipAmount &&
            data.paymentDetails.tipAmount > 0 && (
              <div className="flex justify-between text-green-600 text-sm">
                <span>{t("receipt.tip")}:</span>
                <span>{fmtCurrency(data.paymentDetails.tipAmount, currency)}</span>
              </div>
            )}
          <div className="flex justify-between font-bold border-t border-gray-300 pt-2">
            <span>{t("receipt.totalPaid")}:</span>
            <span>
              {fmtCurrency(
                data.paymentDetails?.totalPaid ?? data.bill.total_amount,
                currency,
              )}
            </span>
          </div>
        </div>

        {/* Payment Method */}
        {data.paymentDetails?.paymentMethod && (
          <div className="mt-3 pt-3 border-t border-gray-300">
            <div className="flex justify-between text-sm">
              <span className="font-semibold">
                {t("receipt.paymentMethod")}:
              </span>
              <span>{data.paymentDetails.paymentMethod}</span>
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="text-center mt-4 pt-3 border-t border-gray-300">
          <p className="font-semibold mb-1">{t("receipt.thankYou")}!</p>
          <p className="text-sm text-gray-600">
            {t("receipt.thankYouMessage", { businessName: data.business.name })}
          </p>
          <p className="text-sm text-gray-600">
            {t("receipt.hopeToSeeYouSoon")}
          </p>

          {/* Review Links */}
          {(data.business.google_reviews_enabled &&
            data.business.google_review_link &&
            data.business.google_review_link.trim() !== "") ||
          (data.business.trustpilot_enabled &&
            data.business.trustpilot_review_url &&
            data.business.trustpilot_review_url.trim() !== "") ? (
            <div className="mt-3 pt-2 border-t border-gray-200">
              <p className="text-xs text-gray-600 mb-2">
                {t("receipt.leaveReview")}:
              </p>
              {data.business.google_reviews_enabled &&
                data.business.google_review_link &&
                data.business.google_review_link.trim() !== "" && (
                  <p className="text-xs text-brand mb-1">
                    Google: {data.business.google_review_link}
                  </p>
                )}
              {data.business.trustpilot_enabled &&
                data.business.trustpilot_review_url &&
                data.business.trustpilot_review_url.trim() !== "" && (
                  <p className="text-xs text-brand mb-1">
                    Trustpilot: {data.business.trustpilot_review_url}
                  </p>
                )}
            </div>
          ) : null}

          <div className="mt-3 text-xs text-gray-500">
            <p>{t("receipt.poweredBy")}</p>
            <p>{t("receipt.digitalReceiptSystem")}</p>
          </div>
        </div>
      </div>

      {/* Action Button */}
      <div className="flex justify-center">
        <Button
          size="lg"
          className="bg-brand text-white hover:bg-brand-dark min-w-48"
          startContent={
            <svg
              className="w-5 h-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
              />
            </svg>
          }
          onPress={handleDownloadReceipt}
        >
          {t("receipt.downloadReceipt")}
        </Button>
      </div>
      {emailReceipt && !emailSent && (
        <div className="mt-4 flex flex-col items-center gap-2 sm:flex-row sm:justify-center">
          <input
            type="email"
            inputMode="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder={t("receipt.emailPlaceholder")}
            aria-label={t("receipt.emailPlaceholder")}
            className="h-11 w-full max-w-xs rounded-xl border border-warm-200 bg-white px-3 text-sm text-ink-900 outline-none focus:border-brand"
          />
          <Button
            variant="flat"
            isLoading={emailSending}
            onPress={() => void handleEmailReceipt()}
            className="h-11"
          >
            {t("receipt.emailMyReceipt")}
          </Button>
        </div>
      )}
      {emailReceipt && emailSent && (
        <p className="mt-4 text-center text-sm text-emerald-700">
          {t("receipt.emailSent")}
        </p>
      )}
    </div>
  );
}
