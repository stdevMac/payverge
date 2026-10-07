"use client";

import React from "react";
import { useInstance } from "@/hooks/useInstance";

/**
 * The operator's logo (GET /api/v1/instance logo_url, from LOGO_URL) as a
 * plain <img> (any origin the CSP allows, so not next/image), else the
 * caller's bundled fallback. The alt text is always the instance product name.
 */
export default function InstanceLogo({
  imgClassName,
  style,
  fallback,
}: {
  imgClassName?: string;
  style?: React.CSSProperties;
  fallback: (alt: string) => React.ReactNode;
}) {
  const { instance, productName } = useInstance();
  const logoUrl = instance?.logo_url || "";
  if (!logoUrl) return <>{fallback(productName)}</>;
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={logoUrl}
      alt={productName}
      className={imgClassName}
      style={style}
      data-testid="instance-logo"
    />
  );
}
