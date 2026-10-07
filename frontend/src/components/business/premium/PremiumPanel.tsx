"use client";

import React from "react";
import { NoiseTexture } from "./NoiseTexture";

type PremiumPanelTone = "default" | "accent" | "urgent" | "danger" | "bare";

type PremiumPanelElement = "div" | "button" | "section" | "article" | "a";

type CommonPremiumPanelProps = {
  as?: PremiumPanelElement;
  tone?: PremiumPanelTone;
  interactive?: boolean;
  withTexture?: boolean;
};

type PremiumPanelProps = CommonPremiumPanelProps &
  (
    | ({ as?: "div" } & React.HTMLAttributes<HTMLDivElement>)
    | ({ as: "button" } & React.ButtonHTMLAttributes<HTMLButtonElement>)
    | ({ as: "a" } & React.AnchorHTMLAttributes<HTMLAnchorElement>)
    | ({ as: "section" } & React.HTMLAttributes<HTMLElement>)
    | ({ as: "article" } & React.HTMLAttributes<HTMLElement>)
  );

const toneClass: Record<PremiumPanelTone, string> = {
  // Crisp borders with a whisper of depth: large-radius drop shadows on every
  // panel made the whole dashboard feel like it was floating off the page.
  default: "border-warm-200/90 bg-white shadow-card",
  accent: "border-brand/20 bg-brand/5 shadow-[0_1px_2px_rgba(26,107,106,0.06)]",
  urgent:
    "border-amber-200 bg-amber-50/90 shadow-[0_1px_2px_rgba(146,64,14,0.05)]",
  danger:
    "border-rose-200 bg-rose-50/90 shadow-[0_1px_2px_rgba(159,18,57,0.06)]",
  bare: "border-warm-200/80 bg-warm-50/70",
};

function panelClass({
  tone,
  interactive,
  className,
}: {
  tone: PremiumPanelTone;
  interactive: boolean;
  className?: string;
}) {
  return [
    // No backdrop-blur here: with a near-opaque bg it is visually negligible,
    // and dozens of blurred panels per tab made every repaint expensive.
    // `block` matters for as="a"/as="button": an inline anchor with border +
    // padding + overflow-hidden fragments around its block children (stray
    // white pill, clipped text) instead of rendering as one card box.
    "relative block overflow-hidden rounded-2xl border",
    "before:pointer-events-none before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-white/80",
    toneClass[tone],
    interactive
      ? "transition-all duration-200 hover:border-warm-300 hover:shadow-[0_2px_8px_rgba(46,42,37,0.08)] active:scale-[0.995] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");
}

export function PremiumPanel({
  as = "div",
  tone = "default",
  interactive = false,
  withTexture = true,
  className = "",
  children,
  ...rest
}: PremiumPanelProps) {
  const shared = {
    className: panelClass({
      tone,
      interactive,
      className,
    }),
    children: (
      <>
        {withTexture ? <NoiseTexture /> : null}
        {children}
      </>
    ),
  };

  if (as === "button") {
    const buttonRest = rest as React.ButtonHTMLAttributes<HTMLButtonElement>;
    return (
      <button
        {...buttonRest}
        {...shared}
        type={buttonRest.type ?? "button"}
      />
    );
  }

  if (as === "section") {
    return (
      <section
        {...(rest as React.HTMLAttributes<HTMLElement>)}
        {...shared}
      />
    );
  }

  if (as === "article") {
    return (
      <article
        {...(rest as React.HTMLAttributes<HTMLElement>)}
        {...shared}
      />
    );
  }

  if (as === "a") {
    return (
      <a
        {...(rest as React.AnchorHTMLAttributes<HTMLAnchorElement>)}
        {...shared}
      />
    );
  }

  return (
    <div
      {...(rest as React.HTMLAttributes<HTMLDivElement>)}
      {...shared}
    />
  );
}
