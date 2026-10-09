"use client";

import React, { useState, useEffect } from "react";
import { Chip } from "@nextui-org/react";
import { convertAmount, formatCurrency } from "../../api/currency";

interface CurrencyConverterProps {
  amount: number;
  fromCurrency: string; // Business default currency (pricing)
  displayCurrency: string; // What customer sees
  showUSDCConversion?: boolean; // Show USDC equivalent
  className?: string;
  /**
   * Resolved BCP-47 tag for number/currency grouping (PG-8 / PG-20).
   * Required so guest/operator money cannot silently fall back to en-US
   * while line items use another locale on the same screen.
   */
  locale: string;
}

interface ConversionRates {
  displayAmount: number;
  usdcAmount: number;
}

export default function CurrencyConverter({
  amount,
  fromCurrency,
  displayCurrency,
  showUSDCConversion = true,
  className = "",
  locale,
}: CurrencyConverterProps) {
  const [rates, setRates] = useState<ConversionRates>({
    displayAmount: amount,
    usdcAmount: amount,
  });
  const [loading, setLoading] = useState(false);
  // True when the live rate fetch failed: we keep the amount in its known-
  // correct source currency (fromCurrency) instead of mislabeling the
  // unconverted magnitude as displayCurrency. (Audit D-01.)
  const [conversionFailed, setConversionFailed] = useState(false);

  useEffect(() => {
    // Cancellation guard (Audit D-02): `amount` changes (tip presets, the 10s
    // bill poll) re-run this effect while a fetch is in flight; without it an
    // earlier slower response can resolve after a newer one and show a stale
    // converted value.
    let cancelled = false;

    const loadConversions = async () => {
      if (amount <= 0) return;

      try {
        setLoading(true);

        let displayAmount = amount;
        let usdcAmount = amount;

        // Convert from business default currency to display currency
        if (fromCurrency !== displayCurrency) {
          const displayResult = await convertAmount(
            amount,
            fromCurrency,
            displayCurrency,
          );
          displayAmount = displayResult.converted_amount;
        }

        // Convert from display currency to USDC (for payment)
        if (displayCurrency !== "USDC") {
          const usdcResult = await convertAmount(
            displayAmount,
            displayCurrency,
            "USDC",
          );
          usdcAmount = usdcResult.converted_amount;
        }

        if (cancelled) return;
        setConversionFailed(false);
        setRates({ displayAmount, usdcAmount });
      } catch (error) {
        console.error("Currency conversion failed:", error);
        if (cancelled) return;
        // Keep the amount in fromCurrency and flag failure so the render labels
        // it correctly and drops the (uncomputable) USDC estimate.
        setConversionFailed(true);
        setRates({ displayAmount: amount, usdcAmount: amount });
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    loadConversions().catch((err) => console.error("loadConversions failed:", err));
    return () => {
      cancelled = true;
    };
  }, [amount, fromCurrency, displayCurrency]);

  if (loading) {
    return (
      <span className={`inline-block animate-pulse ${className}`}>
        <span className="inline-block h-6 bg-gray-200 rounded w-20"></span>
      </span>
    );
  }

  return (
    <div className={`space-y-1 ${className}`}>
      {/* Primary display price. On a failed conversion the magnitude is still
          in fromCurrency, so label it as such rather than as displayCurrency. */}
      <div className="font-semibold text-lg">
        {formatCurrency(
          rates.displayAmount,
          conversionFailed ? fromCurrency : displayCurrency,
          undefined,
          locale,
        )}
      </div>

      {/* USDC conversion (if different and requested). Hidden on failure — we
          couldn't compute it, and showing the raw source amount as USDC would
          misstate what the guest owes. */}
      {showUSDCConversion && !conversionFailed && displayCurrency !== "USDC" && (
        <div className="flex items-center gap-2">
          <Chip size="sm" variant="flat" color="success">
            ≈ {formatCurrency(rates.usdcAmount, "USDC", undefined, locale)}
          </Chip>
        </div>
      )}
    </div>
  );
}

// Simplified version for just displaying converted price
export function CurrencyPrice({
  amount,
  fromCurrency,
  displayCurrency,
  className = "",
  locale,
}: Omit<CurrencyConverterProps, "showUSDCConversion">) {
  const [displayAmount, setDisplayAmount] = useState(amount);
  const [loading, setLoading] = useState(false);
  // See CurrencyConverter: on a failed live-rate fetch, label the unconverted
  // amount in fromCurrency rather than mislabeling it as displayCurrency. (D-01)
  const [conversionFailed, setConversionFailed] = useState(false);

  useEffect(() => {
    let cancelled = false; // stale-response guard (Audit D-02)

    const convertPrice = async () => {
      if (amount <= 0 || fromCurrency === displayCurrency) {
        setDisplayAmount(amount);
        setConversionFailed(false);
        return;
      }

      try {
        setLoading(true);
        const result = await convertAmount(
          amount,
          fromCurrency,
          displayCurrency,
        );
        if (cancelled) return;
        setConversionFailed(false);
        setDisplayAmount(result.converted_amount);
      } catch (error) {
        console.error("Price conversion failed:", error);
        if (cancelled) return;
        setConversionFailed(true);
        setDisplayAmount(amount);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void convertPrice();
    return () => {
      cancelled = true;
    };
  }, [amount, fromCurrency, displayCurrency]);

  if (loading) {
    // Inline-block so the skeleton is valid HTML wherever the value renders —
    // CurrencyPrice is used inside <p> elements (e.g. GuestBill per-item rows),
    // and a block <div> inside <p> auto-closes the <p> and trips React
    // hydration warnings + layout jumps.
    return (
      <span
        className={`inline-block animate-pulse h-6 bg-gray-200 rounded w-16 align-middle ${className}`}
      ></span>
    );
  }

  return (
    <span className={className}>
      {formatCurrency(
        displayAmount,
        conversionFailed ? fromCurrency : displayCurrency,
        undefined,
        locale,
      )}
    </span>
  );
}
