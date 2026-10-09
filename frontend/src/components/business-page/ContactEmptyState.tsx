import React from "react";
import { MapPin } from "lucide-react";
import { hexWithAlpha } from "@/lib/storefront/theme";

interface ContactEmptyStateProps {
  primaryColor: string;
  radiusClass: string;
  title: string;
  body: string;
}

/**
 * Friendly placeholder for the contact tab when a business has shared no
 * address, phone, website, socials, or hours (plan 1.4). Mirrors the
 * ReviewsEmptyState pattern: tinted icon chip, title, body.
 */
export default function ContactEmptyState({
  primaryColor,
  radiusClass,
  title,
  body,
}: ContactEmptyStateProps) {
  return (
    <div className="flex min-h-[280px] flex-col items-center justify-center gap-4 px-6 py-10 text-center">
      <div
        className={`flex h-12 w-12 items-center justify-center ${radiusClass}`}
        style={{
          backgroundColor: hexWithAlpha(primaryColor, "14"),
          color: primaryColor,
        }}
      >
        <MapPin className="h-5 w-5" aria-hidden />
      </div>
      <h3 className="text-lg font-semibold text-gray-900">{title}</h3>
      <p className="max-w-md text-sm text-gray-600">{body}</p>
    </div>
  );
}
