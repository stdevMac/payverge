"use client";

import React from "react";
import {
  AppWindow,
  ArrowUpDown,
  Circle,
  Columns3,
  DoorOpen,
  GlassWater,
  Grip,
  LayoutGrid,
  Minus,
  MoreHorizontal,
  RectangleHorizontal,
  Square,
  Tag,
  Toilet,
  UtensilsCrossed,
  Waves,
  type LucideIcon,
} from "lucide-react";
import type { LayoutElementType, TableShape } from "@/api/spaces";
import { ELEMENT_TYPES, TABLE_SHAPES } from "./types";

interface ElementPaletteProps {
  t: (key: string) => string;
  onAddTableShape: (shape: TableShape) => void;
  onAddElement: (type: LayoutElementType) => void;
  onStartRegion: () => void;
  onStartRoomRect: () => void;
  onStartRoomPolygon: () => void;
  disabled?: boolean;
}

const TABLE_ICONS: Record<string, LucideIcon> = {
  round: Circle,
  square: Square,
  rectangle: RectangleHorizontal,
  oval: Circle,
  bar: Waves,
};

const ELEMENT_ICONS: Partial<Record<LayoutElementType, LucideIcon>> = {
  wall: Minus,
  door: DoorOpen,
  window: AppWindow,
  column: Columns3,
  bar: GlassWater,
  counter: UtensilsCrossed,
  entrance: DoorOpen,
  stairs: ArrowUpDown,
  service_station: UtensilsCrossed,
  restroom: Toilet,
  divider: Grip,
  obstacle: Square,
  label: Tag,
};

function RailButton({
  label,
  icon: Icon,
  onClick,
  disabled,
  testId,
}: {
  label: string;
  icon: LucideIcon;
  onClick: () => void;
  disabled?: boolean;
  testId?: string;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      title={label}
      aria-label={label}
      data-testid={testId}
      className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-ink-700 transition-colors hover:bg-brand/10 hover:text-brand disabled:cursor-not-allowed disabled:opacity-50"
    >
      <Icon className="h-4 w-4" strokeWidth={1.75} />
    </button>
  );
}

const PRIMARY_TABLE_SHAPES: TableShape[] = ["square", "round", "rectangle"];

export function ElementPalette({
  t,
  onAddTableShape,
  onAddElement,
  onStartRegion,
  onStartRoomRect,
  onStartRoomPolygon,
  disabled,
}: ElementPaletteProps) {
  return (
    <aside
      className="flex h-full w-full min-w-0 flex-col items-center gap-0.5 overflow-y-auto bg-warm-50/80 py-2"
      data-testid="element-palette"
      data-palette-density="compact"
      aria-label={t("editor.paletteAria")}
    >
      <div
        className="flex flex-col items-center gap-0.5"
        role="group"
        aria-label={t("editor.palette.room")}
      >
        <RailButton
          label={t("editor.palette.roomRect")}
          icon={RectangleHorizontal}
          onClick={onStartRoomRect}
          disabled={disabled}
          testId="palette-room-rect"
        />
        <RailButton
          label={t("editor.palette.roomPoly")}
          icon={LayoutGrid}
          onClick={onStartRoomPolygon}
          disabled={disabled}
          testId="palette-room-poly"
        />
        <RailButton
          label={t("editor.palette.region")}
          icon={LayoutGrid}
          onClick={onStartRegion}
          disabled={disabled}
          testId="palette-region"
        />
      </div>

      <div className="my-1 h-px w-7 bg-warm-200" aria-hidden />

      <div
        className="flex flex-col items-center gap-0.5"
        role="group"
        aria-label={t("editor.palette.tables")}
      >
        {PRIMARY_TABLE_SHAPES.map((shape) => {
          const Icon = TABLE_ICONS[shape] || Square;
          return (
            <RailButton
              key={shape}
              label={t(`editor.shapes.${shape}`)}
              icon={Icon}
              onClick={() => onAddTableShape(shape)}
              disabled={disabled}
              testId={`palette-table-${shape}`}
            />
          );
        })}
      </div>

      <details className="group w-full" data-testid="palette-more-tools">
        <summary
          className="mx-auto flex h-9 w-9 cursor-pointer list-none items-center justify-center rounded-lg text-ink-500 hover:bg-brand/10 hover:text-brand"
          title={t("editor.palette.moreTools")}
          aria-label={t("editor.palette.moreTools")}
        >
          <MoreHorizontal className="h-4 w-4" strokeWidth={1.75} />
        </summary>
        <div
          className="mt-1 flex flex-col items-center gap-0.5 border-t border-warm-200 pt-1"
          role="group"
          aria-label={t("editor.palette.elements")}
        >
          {ELEMENT_TYPES.map((type) => {
            const Icon = ELEMENT_ICONS[type] || Square;
            return (
              <RailButton
                key={type}
                label={t(`editor.elements.${type}`)}
                icon={Icon}
                onClick={() => onAddElement(type)}
                disabled={disabled}
                testId={`palette-el-${type}`}
              />
            );
          })}
          {TABLE_SHAPES.filter((s) => !PRIMARY_TABLE_SHAPES.includes(s)).map(
            (shape) => {
              const Icon = TABLE_ICONS[shape] || Square;
              return (
                <RailButton
                  key={shape}
                  label={t(`editor.shapes.${shape}`)}
                  icon={Icon}
                  onClick={() => onAddTableShape(shape)}
                  disabled={disabled}
                  testId={`palette-table-${shape}`}
                />
              );
            },
          )}
        </div>
      </details>
    </aside>
  );
}
