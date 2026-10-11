"use client";

import React, { useCallback, useMemo, useRef, useState } from "react";
import { Button, Input } from "@nextui-org/react";
import { AlertTriangle, X } from "lucide-react";
import { useDialogKeyboard } from "./useDialogKeyboard";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import type { Position } from "@/api/positions";
import type { StaffMember } from "@/api/staff";
import type { Shift } from "@/api/schedule";
import type { TimeOffRequest } from "@/api/availability";
import { intlLocaleFor } from "@/utils/intlLocale";
import { humanizeDurationMinutes } from "@/utils/humanizeDuration";
import { PositionSelect, StaffSelect, TimeField } from "@/components/ui/fields";
import { DecimalInput } from "@/components/ui/DecimalInput";
import {
  detectShiftConflicts,
  formatMinuteRange,
  type TeamAvailability,
} from "./scheduleAvailability";

export interface ShiftEditorLabels {
  createTitle: string;
  editTitle: string;
  startTime: string;
  endTime: string;
  position: string;
  assignee: string;
  unassignedOption: string; // "— Open shift —"
  breakMinutes: string;
  /** L5-27 localized negative break error. */
  breakMinutesInvalid: string;
  /** L5-28: Fin equals Inicio is not a valid shift. */
  endEqualsStart: string;
  notes: string;
  save: string;
  saving: string;
  delete: string;
  deleting: string;
  cancel: string;
  positionRequired: string;
  close: string;
  deleteConfirmTitle: string;
  deleteConfirm: string;
  deleteKeep: string;
  /** Carries a "{duration}" placeholder — the live shift-length readout. */
  duration: string;
  /** Heading for the repeat-on-days pill row (create only). */
  repeatLabel: string;
  /** Wave 4: link from an assigned shift to the Team roster. */
  viewTeamProfile: string;
  /**
   * L5-30: discard-confirm when Esc/close would drop dirty edits.
   * Required — an omitted label would render English in every locale, and the
   * only way to notice would be a Spanish operator seeing "Discard changes?".
   */
  discardConfirmTitle: string;
  discardConfirmDescription: string;
  discardConfirm: string;
  discardKeep: string;
}

export interface ShiftConflictLabels {
  heading: string;
  timeoff: string;
  /** Carries a "{range}" placeholder replaced with the window's time range. */
  unavailable: string;
  unavailableAllDay: string;
  /** Same-staff overlapping shift. Carries a "{day}" placeholder. */
  doublebook: string;
  overrideHint: string;
  /** Prefix shown before a per-day conflict list when repeating across days. */
  dayPrefix?: string;
}

export interface ShiftFormValues {
  dayKey: string;
  /** Every day the shift should land on (always includes dayKey; create only —
   *  editing never fans out). Week-order sorted. */
  days: string[];
  startTime: string;
  endTime: string;
  positionId: number;
  staffId: number | null;
  breakMinutes: number;
  notes: string;
}

export interface ShiftEditorInitial {
  startTime: string;
  endTime: string;
  positionId: number | null;
  staffId: number | null;
  breakMinutes: number;
  notes: string;
}

export interface ShiftEditorModalProps {
  isEdit: boolean;
  dayKey: string;
  /** The week's 7 day-keys in display order; enables the repeat-on-days row. */
  weekDays?: string[];
  initial: ShiftEditorInitial;
  positions: Position[];
  staff: StaffMember[];
  labels: ShiftEditorLabels;
  /** Recurring team availability windows grouped by staff_id (overlay source). */
  team: TeamAvailability;
  /** Approved time-off requests for the business (overlay source). */
  approvedTimeOff: TimeOffRequest[];
  /** The week's shifts — enables the same-staff double-booking check. */
  weekShifts?: Shift[];
  /** In edit mode, the id of the shift being edited (skipped in double-book). */
  editingShiftId?: number;
  conflictLabels: ShiftConflictLabels;
  locale: string;
  submitting: boolean;
  deleting: boolean;
  onCancel: () => void;
  onSubmit: (values: ShiftFormValues) => void;
  onDelete?: () => void;
  /** Rendered when the week is already published: changes go live immediately. */
  publishedNote?: string;
  /** Wave 4: jump to Team roster focused on this assignee's name. */
  onViewStaff?: (staffName: string) => void;
}

function dayHeading(dayKey: string, locale: string): string {
  const [y, m, d] = dayKey.split("-").map(Number);
  return new Intl.DateTimeFormat(intlLocaleFor(locale), {
    weekday: "long",
    month: "short",
    day: "numeric",
  }).format(new Date(y, m - 1, d));
}

function dayShort(dayKey: string, locale: string): string {
  const [y, m, d] = dayKey.split("-").map(Number);
  return new Intl.DateTimeFormat(intlLocaleFor(locale), {
    weekday: "short",
  }).format(new Date(y, m - 1, d));
}

/** "HH:MM" → minute-of-day, or NaN on malformed input. */
function toMinutes(hhmm: string): number {
  const [h, m] = hhmm.split(":").map(Number);
  return h * 60 + (m || 0);
}

/** Net shift length via shared humanize; overnight (end < start) wraps +24h.
 * Equal start/end is rejected at submit (L5-28) — display shows null then. */
function formatDuration(
  startTime: string,
  endTime: string,
  breakMinutes: number,
): string | null {
  const start = toMinutes(startTime);
  const end = toMinutes(endTime);
  if (!Number.isFinite(start) || !Number.isFinite(end)) return null;
  if (end === start) return null;
  let total = end - start;
  if (total < 0) total += 24 * 60;
  total -= Math.max(0, breakMinutes);
  if (total <= 0) return null;
  return humanizeDurationMinutes(total);
}

export default function ShiftEditorModal({
  isEdit,
  dayKey,
  weekDays,
  initial,
  positions,
  staff,
  labels,
  team,
  approvedTimeOff,
  weekShifts,
  editingShiftId,
  conflictLabels,
  locale,
  submitting,
  deleting,
  onCancel,
  onSubmit,
  onDelete,
  publishedNote,
  onViewStaff,
}: ShiftEditorModalProps) {
  // Position by the assignee's role: "Dana (server)" almost always works the
  // Server position, so it pre-selects until the operator touches the field.
  // Memoized because the `dirty` comparison below calls it: as a bare render
  // closure it could not be declared as a dependency, and the memo then went
  // stale whenever a staff member's role changed under it.
  const matchPositionForStaff = useCallback(
    (sid: number | null): number | null => {
      if (sid == null) return null;
      const role = staff.find((s) => s.id === sid)?.role;
      if (!role) return null;
      return (
        positions.find((p) => p.name.toLowerCase() === role.toLowerCase())
          ?.id ?? null
      );
    },
    [staff, positions],
  );

  const [startTime, setStartTime] = useState(initial.startTime);
  const [endTime, setEndTime] = useState(initial.endTime);
  // Smart defaults, weakest to strongest: a single-position business pre-selects
  // it; otherwise a set assignee pre-selects their role's position. Editing
  // always honors the shift's own position; a manual pick always wins.
  const [positionId, setPositionId] = useState<number | null>(
    initial.positionId ??
      (positions.length === 1
        ? positions[0].id
        : matchPositionForStaff(initial.staffId)),
  );
  const [positionTouched, setPositionTouched] = useState(
    initial.positionId != null,
  );
  const [staffId, setStaffIdState] = useState<number | null>(initial.staffId);
  const setStaffId = (sid: number | null) => {
    setStaffIdState(sid);
    if (!positionTouched && !isEdit) {
      const match = matchPositionForStaff(sid);
      if (match != null) setPositionId(match);
    }
  };
  // Repeat-on-days (create only): the clicked day is locked on; extra days fan
  // the new shift out across the week on save.
  const [repeatDays, setRepeatDays] = useState<Set<string>>(
    () => new Set([dayKey]),
  );
  const toggleRepeatDay = (key: string) =>
    setRepeatDays((cur) => {
      if (key === dayKey) return cur;
      const next = new Set(cur);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  const [breakMinutes, setBreakMinutes] = useState(
    String(initial.breakMinutes),
  );
  const [notes, setNotes] = useState(initial.notes);
  // Position-required error (shared under PositionSelect).
  const [error, setError] = useState("");
  // L5-28: Fin === Inicio is field-scoped S-5 on the end TimeField — not the
  // position control.
  const [endError, setEndError] = useState("");
  // Two-step delete: the first press arms an inline confirmation instead of
  // firing the (irreversible, no-undo) delete mutation straight away.
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  // L5-30: Esc must not silently discard dirty form state.
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);

  const dirty = useMemo(() => {
    const initialBreak = String(initial.breakMinutes);
    const initialStaff = initial.staffId;
    const initialPos =
      initial.positionId ??
      (positions.length === 1
        ? positions[0].id
        : matchPositionForStaff(initial.staffId));
    if (startTime !== initial.startTime) return true;
    if (endTime !== initial.endTime) return true;
    if (positionId !== initialPos) return true;
    if (staffId !== initialStaff) return true;
    if (breakMinutes !== initialBreak) return true;
    if (notes !== initial.notes) return true;
    if (!isEdit && (repeatDays.size !== 1 || !repeatDays.has(dayKey))) {
      return true;
    }
    return false;
  }, [
    startTime,
    endTime,
    positionId,
    staffId,
    breakMinutes,
    notes,
    initial,
    positions,
    matchPositionForStaff,
    isEdit,
    repeatDays,
    dayKey,
  ]);

  const requestClose = () => {
    if (dirty) {
      setConfirmingDiscard(true);
      return;
    }
    onCancel();
  };

  // Escape-to-close + Tab focus trap for this hand-rolled dialog.
  // Disable while discard ConfirmationModal is open so Tab stays on the confirm.
  const dialogRef = useRef<HTMLDivElement | null>(null);
  useDialogKeyboard(dialogRef, requestClose, !confirmingDiscard);

  // The days this shift will actually land on: every selected repeat day in
  // create mode (so the fan-out doesn't skip conflict checks), just the anchor
  // day when editing. Week-order sorted for stable per-day rows.
  const activeDays = useMemo(() => {
    if (!isEdit && weekDays) {
      const selected = weekDays.filter((d) => repeatDays.has(d));
      return selected.length > 0 ? selected : [dayKey];
    }
    return [dayKey];
  }, [isEdit, weekDays, repeatDays, dayKey]);

  // Availability/time-off/double-booking conflicts for the CURRENT form state,
  // computed PER active day (a repeat-day fan-out must warn on each day it hits,
  // not just the anchor). Informative only; never blocks save.
  const dayConflicts = useMemo(
    () =>
      activeDays
        .map((d) => ({
          day: d,
          conflicts: detectShiftConflicts(
            {
              staffId,
              dayKey: d,
              startTime,
              endTime,
              existingShifts: weekShifts,
              ignoreShiftId: editingShiftId,
            },
            team,
            approvedTimeOff,
          ),
        }))
        .filter((entry) => entry.conflicts.length > 0),
    [
      activeDays,
      staffId,
      startTime,
      endTime,
      weekShifts,
      editingShiftId,
      team,
      approvedTimeOff,
    ],
  );
  const hasConflicts = dayConflicts.length > 0;
  const hasTimeOffConflict = dayConflicts.some((e) =>
    e.conflicts.some((c) => c.kind === "timeoff"),
  );
  // Show a per-day heading only when the fan-out spans more than one active day.
  const showDayHeadings = activeDays.length > 1;

  const handlePositionChange = (id: number | null) => {
    setPositionId(id);
    setPositionTouched(true);
    if (id) setError("");
  };

  const duration = formatDuration(
    startTime,
    endTime,
    Number(breakMinutes) || 0,
  );

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // L5-26: in-flight guard (Enter can re-enter while buttons are loading).
    if (submitting) return;
    if (!positionId) {
      setError(labels.positionRequired);
      setEndError("");
      return;
    }
    // L5-28: Fin === Inicio is not overnight — S-5 on the Fin field only.
    if (startTime && endTime && startTime === endTime) {
      setEndError(labels.endEqualsStart);
      setError("");
      return;
    }
    setError("");
    setEndError("");
    // L5-27: refuse negative / garbage break — do not silently clamp to 0.
    const parsedBreak = Number(breakMinutes);
    if (
      breakMinutes.trim() !== "" &&
      (!Number.isFinite(parsedBreak) || parsedBreak < 0)
    ) {
      return;
    }
    const days =
      !isEdit && weekDays
        ? weekDays.filter((d) => repeatDays.has(d))
        : [dayKey];
    onSubmit({
      dayKey,
      days: days.length > 0 ? days : [dayKey],
      startTime,
      endTime,
      positionId,
      staffId,
      breakMinutes:
        Number.isFinite(parsedBreak) && parsedBreak > 0
          ? Math.round(parsedBreak)
          : 0,
      notes: notes.trim(),
    });
  };

  return (
    <div
      ref={dialogRef}
      className="fixed inset-0 z-50 flex items-end justify-center bg-ink-950/40 p-0 backdrop-blur-sm motion-safe:animate-[cmdk-fade_120ms_ease-out] sm:items-center sm:p-4"
      role="dialog"
      aria-modal="true"
      aria-label={isEdit ? labels.editTitle : labels.createTitle}
    >
      <form
        noValidate
        onSubmit={handleSubmit}
        data-testid="shift-editor-sheet"
        className="flex max-h-screen max-h-[100dvh] w-full max-w-md flex-col overflow-hidden rounded-t-2xl border border-warm-200 bg-white shadow-xl motion-safe:animate-[cmdk-pop_150ms_cubic-bezier(0.16,1,0.3,1)] sm:max-h-[min(40rem,92vh)] sm:rounded-2xl"
      >
        <div className="flex shrink-0 items-start justify-between border-b border-warm-200 px-5 pb-4 pt-5">
          <div>
            <h3 className="font-title text-base text-ink-900">
              {isEdit ? labels.editTitle : labels.createTitle}
            </h3>
            <p className="text-xs capitalize text-ink-500">
              {dayHeading(dayKey, locale)}
            </p>
          </div>
          <button
            type="button"
            onClick={requestClose}
            aria-label={labels.close}
            className="rounded-lg p-1 text-ink-400 transition hover:bg-warm-100 hover:text-ink-700"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div
          data-testid="shift-editor-body"
          className="min-h-0 flex-1 overflow-y-auto px-5 py-4"
        >
        <div className="grid grid-cols-2 gap-3">
          <TimeField
            label={labels.startTime}
            value={startTime}
            onChange={(v) => {
              setStartTime(v);
              if (endError) setEndError("");
            }}
            isRequired
          />
          <TimeField
            label={labels.endTime}
            value={endTime}
            onChange={(v) => {
              setEndTime(v);
              if (endError) setEndError("");
            }}
            isRequired
            isInvalid={Boolean(endError)}
            errorMessage={endError || undefined}
            data-testid="shift-end-time"
          />
        </div>
        {duration ? (
          <p className="mt-1.5 text-xs text-ink-500" aria-live="polite">
            {labels.duration.replace("{duration}", duration)}
          </p>
        ) : null}

        <div className="mt-4">
          <PositionSelect
            label={labels.position}
            positions={positions}
            value={positionId}
            onChange={handlePositionChange}
            isInvalid={Boolean(error)}
          />
          {error ? (
            <p role="alert" className="mt-1.5 text-xs font-medium text-red-600">
              {error}
            </p>
          ) : null}
        </div>

        <div className="mt-4">
          <StaffSelect
            label={labels.assignee}
            staff={staff}
            value={staffId}
            onChange={setStaffId}
            unassignedLabel={labels.unassignedOption}
          />
          {isEdit && staffId != null && onViewStaff
            ? (() => {
                const member = staff.find((s) => s.id === staffId);
                return member ? (
                  <button
                    type="button"
                    onClick={() => onViewStaff(member.name)}
                    className="mt-1 rounded text-xs font-medium text-brand underline decoration-brand/40 underline-offset-2 hover:text-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                  >
                    {labels.viewTeamProfile}
                  </button>
                ) : null;
              })()
            : null}
        </div>

        {!isEdit && weekDays && weekDays.length > 0 ? (
          <fieldset className="mt-4">
            <legend className="text-sm font-medium text-ink-700">
              {labels.repeatLabel}
            </legend>
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {weekDays.map((d) => {
                const selected = repeatDays.has(d);
                const locked = d === dayKey;
                return (
                  <button
                    key={d}
                    type="button"
                    role="checkbox"
                    aria-checked={selected}
                    aria-label={`${dayShort(d, locale)} ${d}`}
                    disabled={locked}
                    onClick={() => toggleRepeatDay(d)}
                    className={`rounded-full border px-2.5 py-1 text-xs font-medium capitalize transition ${
                      selected
                        ? "border-brand/30 bg-brand/10 text-brand-dark"
                        : "border-warm-200 bg-white text-ink-600 hover:border-brand/30 hover:text-ink-900"
                    } ${locked ? "cursor-default opacity-90" : ""}`}
                  >
                    {dayShort(d, locale)}
                  </button>
                );
              })}
            </div>
          </fieldset>
        ) : null}

        {hasConflicts ? (
          <div
            role="status"
            className={`mt-4 rounded-lg border p-3 motion-safe:animate-[cmdk-pop_160ms_ease-out] ${
              hasTimeOffConflict
                ? "border-red-200 bg-red-50"
                : "border-amber-200 bg-amber-50"
            }`}
          >
            <p
              className={`flex items-center gap-1.5 text-xs font-semibold ${
                hasTimeOffConflict ? "text-red-800" : "text-amber-800"
              }`}
            >
              <AlertTriangle className="h-3.5 w-3.5" />
              {conflictLabels.heading}
            </p>
            <ul
              className={`mt-1.5 space-y-1 text-xs ${
                hasTimeOffConflict ? "text-red-700" : "text-amber-700"
              }`}
            >
              {dayConflicts.flatMap((entry) =>
                entry.conflicts.map((c, i) => {
                  const body =
                    c.kind === "timeoff"
                      ? conflictLabels.timeoff
                      : c.kind === "doublebook"
                        ? conflictLabels.doublebook.replace(
                            "{day}",
                            dayShort(entry.day, locale),
                          )
                        : c.window &&
                            c.window.start_min <= 0 &&
                            c.window.end_min >= 1440
                          ? conflictLabels.unavailableAllDay
                          : conflictLabels.unavailable.replace(
                              "{range}",
                              c.window
                                ? formatMinuteRange(
                                    c.window.start_min,
                                    c.window.end_min,
                                    intlLocaleFor(locale),
                                  )
                                : "",
                            );
                  return (
                    <li key={`${entry.day}-${i}`}>
                      {showDayHeadings ? (
                        <span className="font-semibold capitalize">
                          {(conflictLabels.dayPrefix || "{day}").replace(
                            "{day}",
                            dayShort(entry.day, locale),
                          )}
                          {": "}
                        </span>
                      ) : null}
                      {body}
                    </li>
                  );
                }),
              )}
            </ul>
            <p
              className={`mt-1.5 text-[11px] ${
                hasTimeOffConflict ? "text-red-600" : "text-amber-600"
              }`}
            >
              {conflictLabels.overrideHint}
            </p>
          </div>
        ) : null}

        <div className="mt-4 grid grid-cols-2 gap-3">
          <DecimalInput
            // L5-27 / A3b: keep raw string on change; show error for negatives
            // (errorMessage was unreachable when clamp-on-change zeroed them).
            // Clamp to ≥0 only on blur via min={0}.
            label={labels.breakMinutes}
            value={breakMinutes}
            onValueChange={setBreakMinutes}
            min={0}
            maxFractionDigits={0}
            inputMode="numeric"
            isInvalid={
              breakMinutes !== "" &&
              breakMinutes !== "-" &&
              (Number(breakMinutes) < 0 ||
                !Number.isFinite(Number(breakMinutes)))
            }
            errorMessage={
              breakMinutes !== "" &&
              breakMinutes !== "-" &&
              (Number(breakMinutes) < 0 ||
                !Number.isFinite(Number(breakMinutes)))
                ? labels.breakMinutesInvalid
                : undefined
            }
            labelPlacement="outside"
            variant="bordered"
            radius="lg"
            classNames={{ label: "text-sm font-medium text-ink-700" }}
            data-testid="shift-break-minutes"
          />
          <Input
            label={labels.notes}
            value={notes}
            onValueChange={setNotes}
            labelPlacement="outside"
            variant="bordered"
            radius="lg"
            classNames={{ label: "text-sm font-medium text-ink-700" }}
          />
        </div>

        {publishedNote ? (
          <div className="mt-4 rounded-lg border border-brand/15 bg-brand/5 p-3">
            <p className="text-xs text-ink-700">{publishedNote}</p>
          </div>
        ) : null}
        </div>

        {confirmingDelete && isEdit && onDelete ? (
          <div className="mt-0 flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-red-200 bg-red-50 px-5 py-3">
            <p className="text-sm font-medium text-red-800">
              {labels.deleteConfirmTitle}
            </p>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                size="sm"
                variant="bordered"
                onPress={() => setConfirmingDelete(false)}
              >
                {labels.deleteKeep}
              </Button>
              <Button
                type="button"
                size="sm"
                color="danger"
                onPress={onDelete}
                isLoading={deleting}
              >
                {deleting ? labels.deleting : labels.deleteConfirm}
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex shrink-0 items-center justify-between gap-2 border-t border-warm-200 px-5 py-4">
            {isEdit && onDelete ? (
              <Button
                type="button"
                variant="light"
                color="danger"
                onPress={() => setConfirmingDelete(true)}
                isLoading={deleting}
              >
                {deleting ? labels.deleting : labels.delete}
              </Button>
            ) : (
              <span />
            )}
            <div className="flex items-center gap-2">
              <Button type="button" variant="bordered" onPress={requestClose}>
                {labels.cancel}
              </Button>
              <Button type="submit" color="primary" isLoading={submitting}>
                {submitting ? labels.saving : labels.save}
              </Button>
            </div>
          </div>
        )}
      </form>

      <ConfirmationModal
        isOpen={confirmingDiscard}
        onOpenChange={() => setConfirmingDiscard(false)}
        title={labels.discardConfirmTitle}
        description={labels.discardConfirmDescription}
        confirmLabel={labels.discardConfirm}
        cancelLabel={labels.discardKeep}
        isDanger
        onConfirm={() => {
          setConfirmingDiscard(false);
          onCancel();
        }}
      />
    </div>
  );
}
