"use client";

import React, { useEffect, useRef, useState } from "react";

interface GuestQuoteStatusProps {
  pending: boolean;
  error: boolean;
  blocked: boolean;
  pendingLabel: string;
  errorLabel: string;
  blockedLabel: string;
  settledLabel: string;
}

/**
 * Dedicated quote/total status — never reuse the page-level "Loading menu…"
 * string. The live region sits outside the cart button so pending copy cannot
 * pollute the cart destination name.
 */
export default function GuestQuoteStatus({
  pending,
  error,
  blocked,
  pendingLabel,
  errorLabel,
  blockedLabel,
  settledLabel,
}: GuestQuoteStatusProps) {
  const [settledAnnouncement, setSettledAnnouncement] = useState("");
  const wasPending = useRef(false);

  useEffect(() => {
    if (pending) {
      wasPending.current = true;
      setSettledAnnouncement("");
      return;
    }
    if (wasPending.current && !error && !blocked && settledLabel) {
      setSettledAnnouncement(settledLabel);
    }
    wasPending.current = false;
  }, [pending, error, blocked, settledLabel]);

  const message = pending
    ? pendingLabel
    : error
      ? errorLabel
      : blocked
        ? blockedLabel
        : settledAnnouncement;

  if (!message) return null;

  return (
    <span
      role="status"
      aria-live="polite"
      className="sr-only"
      data-testid="guest-quote-status"
    >
      {message}
    </span>
  );
}
