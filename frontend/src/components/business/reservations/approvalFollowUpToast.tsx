"use client";

import React from "react";
import toast from "react-hot-toast";

/**
 * Wave 4: success toast with a follow-up action. Used after approving a
 * reservation so the operator can jump straight to the Tables tab instead of
 * dead-ending on a plain toast.
 */
export function showApprovedFollowUpToast(options: {
  message: string;
  actionLabel: string;
  onAction: () => void;
}) {
  const { message, actionLabel, onAction } = options;
  toast.success(
    (tst) => (
      <span className="flex items-center gap-3">
        <span>{message}</span>
        <button
          type="button"
          className="whitespace-nowrap font-semibold text-brand underline underline-offset-2"
          onClick={() => {
            toast.dismiss(tst.id);
            onAction();
          }}
        >
          {actionLabel}
        </button>
      </span>
    ),
    { duration: 6000 },
  );
}
