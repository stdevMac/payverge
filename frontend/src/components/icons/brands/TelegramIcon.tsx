import React from "react";

/**
 * Telegram brand glyph. Inline simple-icons path so the app needs no external
 * brand-icon dependency for the handful of logos lucide can't supply.
 *
 * Drop-in for the former vendored mark: `fill="currentColor"` keeps the
 * existing `text-*` color classes working, and the default `1em` size mirrors
 * the previous font-size-driven sizing so `text-xl` / `w-5 h-5` callers render
 * identically.
 */
export const TelegramIcon = ({
  className,
  size = "1em",
  ...props
}: { className?: string; size?: number | string } & React.SVGProps<SVGSVGElement>) => (
  <svg
    viewBox="0 0 24 24"
    width={size}
    height={size}
    fill="currentColor"
    className={className}
    aria-hidden="true"
    {...props}
  >
    <path d="m9.417 15.181-.397 5.584c.568 0 .814-.244 1.109-.537l2.663-2.545 5.518 4.041c1.012.564 1.725.267 1.998-.931L23.93 3.821l.001-.001c.321-1.496-.541-2.081-1.527-1.714l-21.29 8.151c-1.453.564-1.431 1.374-.247 1.741l5.443 1.693L18.953 5.16c.598-.397 1.142-.177.694.22z" />
  </svg>
);
