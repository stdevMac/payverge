"use client";

import React from "react";
import { Sparkles } from "lucide-react";

export interface SageMarkProps {
  size?: "sm" | "md" | "lg";
  variant?: "solid" | "soft";
  className?: string;
}

// Sage's identity mark: a rounded-square brand badge with a Sparkles glyph.
// This is the single source of truth for "this is Sage" across the Director
// Console — it replaces the generic `Bot` robot everywhere. The Sparkles glyph
// already reads as Sage's mark in the Pre-Shift empty state, so we standardize
// on it here. The glyph is decorative; lucide emits `aria-hidden` by default.
const SIZE_STYLES: Record<
  NonNullable<SageMarkProps["size"]>,
  { box: string; icon: string }
> = {
  sm: { box: "w-8 h-8 rounded-lg", icon: "w-4 h-4" },
  md: { box: "w-10 h-10 rounded-xl", icon: "w-5 h-5" },
  lg: { box: "w-12 h-12 rounded-2xl", icon: "w-6 h-6" },
};

const VARIANT_STYLES: Record<NonNullable<SageMarkProps["variant"]>, string> = {
  solid: "bg-brand text-white shadow-sm shadow-brand/20",
  soft: "bg-brand/10 text-brand",
};

export default function SageMark({
  size = "md",
  variant = "solid",
  className = "",
}: SageMarkProps) {
  const sizeStyle = SIZE_STYLES[size];
  return (
    <div
      className={`flex items-center justify-center flex-none ${sizeStyle.box} ${VARIANT_STYLES[variant]} ${className}`}
    >
      <Sparkles className={sizeStyle.icon} strokeWidth={1.75} />
    </div>
  );
}
