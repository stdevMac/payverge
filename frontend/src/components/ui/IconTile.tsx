"use client";

import React from "react";
import type { LucideIcon } from "lucide-react";

interface IconTileProps {
  icon: LucideIcon;
  /** md = section headers (h-10). lg = empty/activation states (h-12). */
  size?: "md" | "lg";
  className?: string;
  "data-testid"?: string;
}

const SIZE_CLASSES: Record<NonNullable<IconTileProps["size"]>, string> = {
  md: "h-10 w-10 rounded-xl",
  lg: "h-12 w-12 rounded-2xl",
};

const ICON_SIZE: Record<NonNullable<IconTileProps["size"]>, string> = {
  md: "h-5 w-5",
  lg: "h-6 w-6",
};

/**
 * The one icon-tile recipe: tinted brand square, no border, no shadow, never
 * grey, never solid-filled. Replaces the three divergent tile styles the
 * cohesion audit found (bordered+shadowed, gray-50, solid bg-brand).
 */
export default function IconTile({
  icon: Icon,
  size = "md",
  className = "",
  "data-testid": dataTestId,
}: IconTileProps) {
  return (
    <span
      data-testid={dataTestId}
      className={`flex shrink-0 items-center justify-center bg-brand/10 text-brand ${SIZE_CLASSES[size]} ${className}`.trim()}
    >
      <Icon className={ICON_SIZE[size]} aria-hidden="true" />
    </span>
  );
}
