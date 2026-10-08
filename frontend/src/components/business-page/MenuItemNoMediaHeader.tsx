import React from "react";
import { Utensils } from "lucide-react";

interface MenuItemNoMediaHeaderProps {
  label: string;
}

export default function MenuItemNoMediaHeader({ label }: MenuItemNoMediaHeaderProps) {
  // Label uses ink-700 (≥5.5:1 under card opacity-80 composites) so public-menu
  // axe release smoke stays AA when closed-mode cards dim. ink-500 is only
  // 3.5:1 once an ancestor applies opacity-80 over the cream page canvas.
  return (
    <div
      className="relative w-full aspect-[4/3] overflow-hidden bg-ink-100 flex flex-col items-center justify-center gap-2"
      role="img"
      aria-label={label}
    >
      <Utensils className="w-9 h-9 text-ink-500" aria-hidden />
      <span className="text-xs font-medium uppercase tracking-wider text-ink-700">
        {label}
      </span>
    </div>
  );
}
