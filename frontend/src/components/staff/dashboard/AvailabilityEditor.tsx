"use client";

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Select, SelectItem } from "@nextui-org/react";
import { CalendarClock, Plus, X } from "lucide-react";
import {
  availabilityApi,
  type AvailabilityKind,
  type StaffAvailability,
} from "@/api/availability";
import { queryKeys } from "@/api/queryKeys";
import { EmptyState } from "@/components/ui/EmptyState";
import { TimeField } from "@/components/ui/fields";
import { SkeletonLine } from "@/components/ui/skeletons";
import { useToast } from "@/contexts/ToastContext";
import { intlLocaleFor } from "@/utils/intlLocale";
import type { StaffData } from "@/utils/staffAuth";

export interface AvailabilityEditorLabels {
  title: string;
  subtitle: string;
  loading: string;
  error: string;
  emptyTitle: string;
  emptySubtitle: string;
  addTitle: string;
  weekdayLabel: string;
  kindLabel: string;
  kindPreferred: string;
  kindUnavailable: string;
  startLabel: string;
  endLabel: string;
  add: string;
  remove: string;
  invalidRange: string;
  save: string;
  saving: string;
  saved: string;
  saveError: string;
}

export interface AvailabilityEditorProps {
  staff: StaffData;
  labels: AvailabilityEditorLabels;
  /** Display locale for Intl day/time formatting (not a translated string). */
  locale: string;
}

interface EditableWindow {
  key: string;
  weekday: number;
  start_min: number;
  end_min: number;
  kind: AvailabilityKind;
}

// 2023-01-01 was a Sunday, so adding `weekday` (0=Sun..6=Sat) lands on the right
// day for an Intl weekday label without any timezone math.
function weekdayName(weekday: number, locale: string): string {
  const d = new Date(2023, 0, 1 + weekday);
  return new Intl.DateTimeFormat(intlLocaleFor(locale), { weekday: "long" }).format(d);
}

function minuteLabel(min: number, locale: string): string {
  const h = Math.floor(min / 60);
  const m = min % 60;
  return new Intl.DateTimeFormat(intlLocaleFor(locale), {
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(2023, 0, 1, h, m));
}

function minutesFromTimeInput(value: string): number {
  const [h, m] = value.split(":").map(Number);
  return (h || 0) * 60 + (m || 0);
}

const WEEKDAYS = [0, 1, 2, 3, 4, 5, 6];

export default function AvailabilityEditor({ staff, labels, locale }: AvailabilityEditorProps) {
  const businessId = String(staff.business_id);
  const toast = useToast();
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: queryKeys.availability.mine(businessId, staff.id),
    queryFn: () => availabilityApi.getMine(businessId),
  });

  // Editable working copy. Seeded once from the server, then resynced from each
  // successful save's response (the PUT returns the canonical stored windows).
  const [windows, setWindows] = useState<EditableWindow[]>([]);
  const seeded = useRef(false);
  const nextId = useRef(0);

  const toEditable = useCallback((rows: StaffAvailability[]): EditableWindow[] => {
    return rows.map((w) => ({
      key: `w${nextId.current++}`,
      weekday: w.weekday,
      start_min: w.start_min,
      end_min: w.end_min,
      kind: w.kind,
    }));
  }, []);

  // A different principal or business is a different working copy.
  const seedScope = `${businessId}:${staff.id}`;
  const seededScope = useRef(seedScope);
  if (seededScope.current !== seedScope) {
    seededScope.current = seedScope;
    seeded.current = false;
  }

  // Seed only from a fetch made after this mount, never from data already in
  // the cache, so the editor can't start from another session's rows.
  useEffect(() => {
    if (!seeded.current && query.data && query.isFetchedAfterMount) {
      setWindows(toEditable(query.data));
      seeded.current = true;
    }
  }, [query.data, query.isFetchedAfterMount, seedScope, toEditable]);

  // Add-window form.
  const [addWeekday, setAddWeekday] = useState(1);
  const [addKind, setAddKind] = useState<AvailabilityKind>("preferred");
  const [addStart, setAddStart] = useState("09:00");
  const [addEnd, setAddEnd] = useState("17:00");

  const addInvalid = minutesFromTimeInput(addStart) >= minutesFromTimeInput(addEnd);

  const handleAdd = useCallback(() => {
    if (addInvalid) return;
    setWindows((prev) => [
      ...prev,
      {
        key: `w${nextId.current++}`,
        weekday: addWeekday,
        start_min: minutesFromTimeInput(addStart),
        end_min: minutesFromTimeInput(addEnd),
        kind: addKind,
      },
    ]);
  }, [addInvalid, addWeekday, addStart, addEnd, addKind]);

  const handleRemove = useCallback((key: string) => {
    setWindows((prev) => prev.filter((w) => w.key !== key));
  }, []);

  const saveMutation = useMutation({
    mutationFn: () =>
      availabilityApi.putMine(
        businessId,
        windows
          .slice()
          .sort((a, b) => a.weekday - b.weekday || a.start_min - b.start_min)
          .map((w) => ({
            weekday: w.weekday,
            start_min: w.start_min,
            end_min: w.end_min,
            kind: w.kind,
          })),
      ),
    onSuccess: (rows) => {
      queryClient.setQueryData(queryKeys.availability.mine(businessId, staff.id), rows);
      setWindows(toEditable(rows));
      toast.showSuccess(labels.saved);
    },
    onError: () => toast.showError(labels.saveError),
  });

  // Group windows by weekday for display (only days that have windows render).
  const byDay = useMemo(() => {
    const map = new Map<number, EditableWindow[]>();
    for (const w of windows) {
      const bucket = map.get(w.weekday);
      if (bucket) bucket.push(w);
      else map.set(w.weekday, [w]);
    }
    for (const bucket of map.values()) bucket.sort((a, b) => a.start_min - b.start_min);
    return WEEKDAYS.filter((d) => map.has(d)).map((d) => ({ weekday: d, items: map.get(d)! }));
  }, [windows]);

  return (
    <section className="space-y-4" aria-label={labels.title}>
      <header>
        <h2 className="font-title text-base text-gray-900">{labels.title}</h2>
        <p className="text-sm text-gray-500">{labels.subtitle}</p>
      </header>

      {query.isLoading ? (
        <div
          role="status"
          aria-live="polite"
          aria-label={labels.loading}
          className="space-y-3"
        >
          <div className="rounded-xl border border-warm-200 p-4 motion-safe:animate-pulse">
            <SkeletonLine width="30%" height="0.875rem" />
            <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
              {[0, 1, 2, 3].map((i) => (
                <SkeletonLine key={i} height="2.75rem" className="rounded-lg" />
              ))}
            </div>
          </div>
          <div className="rounded-xl border border-warm-200 p-4 motion-safe:animate-pulse">
            <SkeletonLine width="25%" height="0.875rem" />
            <SkeletonLine width="55%" height="1rem" className="mt-3" />
          </div>
        </div>
      ) : query.isError ? (
        <div className="rounded-xl border border-warm-200 p-4">
          <p className="text-sm text-ink-500">{labels.error}</p>
        </div>
      ) : (
        <>
          {/* Add a window */}
          <div className="rounded-xl border border-warm-200 p-4">
            <h3 className="text-sm font-semibold text-ink-900">{labels.addTitle}</h3>
            <div className="mt-3 grid grid-cols-2 items-start gap-3 sm:grid-cols-4">
              <Select
                label={labels.weekdayLabel}
                selectedKeys={[String(addWeekday)]}
                onChange={(e) => e.target.value && setAddWeekday(Number(e.target.value))}
                aria-label={labels.weekdayLabel}
                labelPlacement="outside"
                variant="bordered"
                radius="lg"
                classNames={{ label: "text-sm font-medium text-ink-700" }}
              >
                {WEEKDAYS.map((d) => (
                  <SelectItem
                    key={String(d)}
                    textValue={weekdayName(d, locale)}
                    className="capitalize"
                  >
                    {weekdayName(d, locale)}
                  </SelectItem>
                ))}
              </Select>
              <Select
                label={labels.kindLabel}
                selectedKeys={[addKind]}
                onChange={(e) => e.target.value && setAddKind(e.target.value as AvailabilityKind)}
                aria-label={labels.kindLabel}
                labelPlacement="outside"
                variant="bordered"
                radius="lg"
                classNames={{ label: "text-sm font-medium text-ink-700" }}
              >
                <SelectItem key="preferred" textValue={labels.kindPreferred}>
                  {labels.kindPreferred}
                </SelectItem>
                <SelectItem key="unavailable" textValue={labels.kindUnavailable}>
                  {labels.kindUnavailable}
                </SelectItem>
              </Select>
              <TimeField
                label={labels.startLabel}
                value={addStart}
                onChange={setAddStart}
              />
              <TimeField label={labels.endLabel} value={addEnd} onChange={setAddEnd} />
            </div>
            {addInvalid ? (
              <p className="mt-2 text-xs text-rose-600">{labels.invalidRange}</p>
            ) : null}
            <Button
              size="sm"
              variant="bordered"
              className="mt-3"
              startContent={<Plus className="h-4 w-4" />}
              isDisabled={addInvalid}
              onPress={handleAdd}
            >
              {labels.add}
            </Button>
          </div>

          {byDay.length === 0 ? (
            <div className="rounded-xl border border-gray-200">
              <EmptyState
                icon={CalendarClock}
                title={labels.emptyTitle}
                subtitle={labels.emptySubtitle}
              />
            </div>
          ) : (
            <ul className="space-y-3">
              {byDay.map((group) => (
                <li key={group.weekday} className="rounded-xl border border-gray-200 p-4">
                  <h3 className="text-sm font-semibold capitalize text-gray-900">
                    {weekdayName(group.weekday, locale)}
                  </h3>
                  <ul className="mt-3 space-y-2">
                    {group.items.map((w) => (
                      <li
                        key={w.key}
                        className="flex items-center justify-between gap-3 border-t border-gray-100 pt-2 first:border-0 first:pt-0"
                      >
                        <div className="flex items-center gap-2">
                          <span
                            className={`inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium ${
                              w.kind === "preferred"
                                ? "bg-brand-50 text-brand"
                                : "bg-warm-100 text-ink-600"
                            }`}
                          >
                            {w.kind === "preferred"
                              ? labels.kindPreferred
                              : labels.kindUnavailable}
                          </span>
                          <span className="text-sm font-medium text-gray-900">
                            {minuteLabel(w.start_min, locale)} – {minuteLabel(w.end_min, locale)}
                          </span>
                        </div>
                        <button
                          type="button"
                          aria-label={labels.remove}
                          onClick={() => handleRemove(w.key)}
                          className="rounded-lg p-1 text-gray-400 transition hover:bg-gray-100 hover:text-gray-600"
                        >
                          <X className="h-4 w-4" />
                        </button>
                      </li>
                    ))}
                  </ul>
                </li>
              ))}
            </ul>
          )}

          <Button
            color="primary"
            className="w-full"
            isLoading={saveMutation.isPending}
            onPress={() => saveMutation.mutate()}
          >
            {saveMutation.isPending ? labels.saving : labels.save}
          </Button>
        </>
      )}
    </section>
  );
}
