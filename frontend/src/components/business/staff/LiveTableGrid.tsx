"use client";

import React from "react";
import { Users, Clock } from "lucide-react";
import type { Table } from "@/api/business";
import { humanizeDurationMinutes } from "@/utils/humanizeDuration";

export interface ActiveBillInfo {
  count: number;
  oldest_bill_id: number;
  oldest_minutes: number;
}

interface LiveTableGridProps {
  tables: Table[];
  activeBillsByTableId: Record<number, ActiveBillInfo | undefined>;
  onStartNewOrder: (tableId: number) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

export default function LiveTableGrid({
  tables,
  activeBillsByTableId,
  onStartNewOrder,
  t,
}: LiveTableGridProps) {
  if (!tables.length) {
    return (
      <div className="rounded-2xl border border-dashed border-warm-200 bg-warm-50 p-10 text-center">
        <p className="text-label uppercase text-ink-500 mb-2">
          {t("tableGrid.emptyLabel")}
        </p>
        <h3 className="font-title text-heading-md text-ink-900 mb-2">
          {t("tableGrid.emptyTitle")}
        </h3>
        <p className="text-body-sm text-ink-600">{t("tableGrid.emptyHelp")}</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
      {tables.map((table) => {
        const bill = activeBillsByTableId[table.id];
        const isOccupied = !!bill;
        const ageLabel = bill
          ? humanizeDurationMinutes(bill.oldest_minutes)
          : "";
        return (
          <button
            key={table.id}
            type="button"
            onClick={() => onStartNewOrder(table.id)}
            className={`relative flex flex-col items-start gap-2 rounded-2xl border p-4 text-left transition-all ${
              isOccupied
                ? "border-brand/30 bg-brand/5 hover:border-brand/50"
                : "border-warm-200 bg-white hover:border-warm-300 hover:-translate-y-0.5"
            }`}
          >
            <div className="flex items-center justify-between w-full">
              <span className="font-title text-heading-md text-ink-950">
                {table.name}
              </span>
              <span className="text-label uppercase text-ink-500">
                {isOccupied ? t("tableGrid.occupied") : t("tableGrid.available")}
              </span>
            </div>
            <div className="flex items-center gap-4 text-body-sm text-ink-600">
              <span className="inline-flex items-center gap-1">
                <Users className="w-3.5 h-3.5" />
                {table.capacity}
              </span>
              {isOccupied && bill ? (
                <span className="inline-flex items-center gap-1 text-brand">
                  <Clock className="w-3.5 h-3.5" />
                  {bill.count > 1
                    ? t("tableGrid.multipleBills", {
                        count: bill.count,
                        oldest: ageLabel,
                      })
                    : t("tableGrid.minutesOpen", { n: ageLabel })}
                </span>
              ) : null}
            </div>
            <span className="mt-1 text-xs font-semibold text-brand">
              {isOccupied ? t("tableGrid.addItems") : t("tableGrid.startOrder")} →
            </span>
          </button>
        );
      })}
    </div>
  );
}
