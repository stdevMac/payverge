import React from "react";
import { Link } from "@nextui-org/react";
import { Star, ExternalLink } from "lucide-react";

interface ReviewsEmptyStateProps {
  primaryColor: string;
  radiusClass: string;
  reviewLink?: string;
  ctaLabel: string;
  bodyText: string;
}

export default function ReviewsEmptyState({
  primaryColor,
  radiusClass,
  reviewLink,
  ctaLabel,
  bodyText,
}: ReviewsEmptyStateProps) {
  return (
    <div className="min-h-[280px] flex flex-col items-center justify-center gap-4 text-center px-6 py-10">
      <p className="text-gray-600">{bodyText}</p>
      {reviewLink && (
        <Link
          href={reviewLink}
          isExternal
          className={`inline-flex items-center gap-2 px-5 py-2.5 ${radiusClass} text-sm font-medium text-white transition-colors active:translate-y-[1px]`}
          style={{ backgroundColor: primaryColor }}
        >
          <Star className="w-4 h-4" aria-hidden />
          {ctaLabel}
          <ExternalLink className="w-3.5 h-3.5" aria-hidden />
        </Link>
      )}
    </div>
  );
}
