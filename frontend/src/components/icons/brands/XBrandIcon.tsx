import React from "react";

/**
 * X (formerly Twitter) brand glyph. Named `XBrandIcon` to avoid colliding with
 * lucide's `X` close-icon. Inline simple-icons path; replaces both the old
 * `FaXTwitter` mark and the legacy `FaTwitter` bird — Twitter is X now.
 *
 * See TelegramIcon for the `currentColor` / `1em`-default drop-in contract.
 */
export const XBrandIcon = ({
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
    <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
  </svg>
);
