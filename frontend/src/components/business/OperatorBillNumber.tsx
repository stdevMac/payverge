"use client";

import React, { useCallback, useState } from "react";
import { Copy, Check } from "lucide-react";
import {
  operatorBillDisplayNumber,
  operatorBillSupportId,
  type OperatorBillIdentity,
} from "@/lib/operatorBillNumber";

type OperatorBillNumberProps = OperatorBillIdentity & {
  className?: string;
  /** Prefix character, defaults to "#". Pass "" for bare number. */
  prefix?: string;
  /**
   * Optional translated label rendered in the same text node as the number
   * (e.g. "Bill" → "Bill #1001") so screen readers and getByText see one string.
   */
  label?: string;
  /** Accessible label for the copy control (already translated). */
  copyLabel: string;
  /** Accessible label after a successful copy (already translated). */
  copiedLabel?: string;
  /** Hide the number text and only render the copy control. */
  copyOnly?: boolean;
  /** Optional test id on the display root. */
  "data-testid"?: string;
};

/**
 * Honest operator bill identity: display number + copy-to-clipboard for support.
 * Never renders a DEMO- prefix (stripped by the helper).
 */
export function OperatorBillNumber({
  id,
  bill_number,
  className,
  prefix = "#",
  label,
  copyLabel,
  copiedLabel,
  copyOnly = false,
  "data-testid": testId,
}: OperatorBillNumberProps) {
  const display = operatorBillDisplayNumber({ id, bill_number });
  const supportId = operatorBillSupportId({ id, bill_number });
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(
    async (event: React.MouseEvent | React.KeyboardEvent) => {
      event.preventDefault();
      event.stopPropagation();
      if (!supportId || typeof navigator === "undefined" || !navigator.clipboard) {
        return;
      }
      try {
        await navigator.clipboard.writeText(supportId);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      } catch {
        // Clipboard blocked — display still shows the number.
      }
    },
    [supportId],
  );

  const numberText = label
    ? `${label} ${prefix}${display}`
    : `${prefix}${display}`;

  return (
    <span
      className={`inline-flex items-center gap-1.5 ${className ?? ""}`}
      data-testid={testId}
    >
      {!copyOnly ? (
        <span className={label ? undefined : "font-mono"}>{numberText}</span>
      ) : null}
      {supportId ? (
        <button
          type="button"
          onClick={handleCopy}
          className="inline-flex rounded p-0.5 text-ink-500 hover:bg-warm-100 hover:text-ink-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          aria-label={copied ? copiedLabel ?? copyLabel : copyLabel}
          title={copied ? copiedLabel ?? copyLabel : copyLabel}
          data-testid="operator-bill-copy"
        >
          {copied ? (
            <Check className="h-3.5 w-3.5 text-emerald-600" aria-hidden />
          ) : (
            <Copy className="h-3.5 w-3.5" aria-hidden />
          )}
        </button>
      ) : null}
    </span>
  );
}
