"use client";

import React from "react";
import { ArrowRight, FileText, PenLine, Sparkles, type LucideIcon } from "lucide-react";
import { useInstance } from "@/hooks/useInstance";

export interface MenuFrontDoorProps {
  tString: (key: string) => string;
  onAddCategoryOpen: () => void;
  onOpenAIFeature?: (feature: "pdf" | "wizard") => void;
}

interface FrontDoorTile {
  key: "scan" | "describe" | "manual";
  title: string;
  description: string;
  badge: string;
  icon: LucideIcon;
  onPress: () => void;
}

/**
 * AI-first "bring your own menu" front door. Replaces seeded/demo menu data
 * with three equally-weighted ways to get a real menu in fast: scan an
 * existing printed menu, describe the restaurant to AI, or build by hand.
 * AI tiles are offered only when this install has an LLM provider.
 */
export default function MenuFrontDoor({
  tString,
  onAddCategoryOpen,
  onOpenAIFeature,
}: MenuFrontDoorProps) {
  // No LLM provider on this install: scan/describe cannot work, so only the
  // manual path is offered (GET /instance features.ai).
  const aiOff = useInstance().isOff("ai");
  const allTiles: FrontDoorTile[] = [
    {
      key: "scan",
      title: tString("ai.emptyState.pdf.title"),
      description: tString("ai.emptyState.pdf.description"),
      badge: tString("ai.emptyState.badges.ai"),
      icon: FileText,
      onPress: () => {
        onOpenAIFeature?.("pdf");
      },
    },
    {
      key: "describe",
      title: tString("ai.emptyState.wizard.title"),
      description: tString("ai.emptyState.wizard.description"),
      badge: tString("ai.emptyState.badges.ai"),
      icon: Sparkles,
      onPress: () => {
        onOpenAIFeature?.("wizard");
      },
    },
    {
      key: "manual",
      title: tString("ai.emptyState.manual.title"),
      description: tString("ai.emptyState.manual.description"),
      badge: tString("ai.emptyState.badges.manual"),
      icon: PenLine,
      onPress: onAddCategoryOpen,
    },
  ];
  const tiles = aiOff ? allTiles.filter((tile) => tile.key === "manual") : allTiles;

  return (
    <div
      className={`grid w-full gap-3 ${aiOff ? "max-w-xs" : "max-w-2xl md:grid-cols-3"}`}
    >
      {tiles.map(({ key, title, description, badge, icon: Icon, onPress }) => (
        <button
          key={key}
          type="button"
          onClick={onPress}
          className={`group flex min-h-44 flex-col rounded-2xl border border-warm-200/90 bg-white p-4 text-left shadow-sm transition-all hover:-translate-y-0.5 hover:border-brand/30 hover:shadow-lg`}
        >
          <div className="flex items-center justify-between gap-2">
            <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-brand text-white shadow-sm">
              <Icon className="h-5 w-5" aria-hidden="true" />
            </div>
            <ArrowRight
              className="h-4 w-4 text-ink-400 transition-transform group-hover:translate-x-0.5 group-hover:text-brand"
              aria-hidden="true"
            />
          </div>
          <div className="mt-4 min-h-0 flex-1">
            <h4 className="font-semibold text-ink-900">{title}</h4>
            <p className="mt-1 text-xs leading-5 text-ink-600">{description}</p>
          </div>
          <span className="mt-4 inline-flex w-fit rounded-full bg-brand-50 px-2 py-0.5 text-[10px] font-semibold text-brand-700">
            {badge}
          </span>
        </button>
      ))}
    </div>
  );
}
