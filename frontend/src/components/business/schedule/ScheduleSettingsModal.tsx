"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Select, SelectItem, Switch } from "@nextui-org/react";
import { X } from "lucide-react";
import { scheduleSettingsApi } from "@/api/scheduleSettings";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { weekStartDayName } from "@/utils/scheduleWeek";
import { TimeField } from "@/components/ui/fields";
import { DecimalInput } from "@/components/ui/DecimalInput";
import { SkeletonLine } from "@/components/ui/skeletons";
import {
  buildScheduleSettingsUpdate,
  formFromSettings,
  type ScheduleSettingsForm,
} from "./scheduleSettingsForm";
import { useDialogKeyboard } from "./useDialogKeyboard";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";

interface ScheduleSettingsModalProps {
  businessId: string;
  onClose: () => void;
}

// Shared cache key with ScheduleBuilder's settingsQuery, so a week-start change
// re-reads the grid immediately.
const settingsKey = (businessId: string) =>
  ["schedule", businessId, "settings"] as const;

/**
 * L5-33 / NR-2: string-state numeric field. Parse/clamp/step only on blur so
 * intermediate values ("8.", "7,") and half-hour steps stay reachable.
 * Invalid text ("abc") and out-of-range show a visible error and block save —
 * never silently collapse to 0 / parent state.
 */
function NumberField({
  label,
  value,
  onChange,
  unit,
  min,
  max,
  step,
  invalidMessage,
  onValidityChange,
  testId,
}: {
  label: string;
  value: number;
  onChange: (n: number) => void;
  unit: string;
  min?: number;
  max?: number;
  step?: number;
  invalidMessage: string;
  onValidityChange?: (valid: boolean) => void;
  testId?: string;
}) {
  const [text, setText] = useState(String(value));
  // Keep display in sync when parent resets form from server.
  useEffect(() => {
    setText(String(value));
  }, [value]);

  const parsed = tryParseLocaleDecimal(text);
  const empty = text.trim() === "";
  const outOfRange =
    parsed !== null &&
    ((min !== undefined && parsed < min) ||
      (max !== undefined && parsed > max));
  // Valid only when parse succeeds and is within bounds. Empty mid-edit is
  // invalid for save (parent still holds last good number — must not look
  // saveable while the field is blank or garbage).
  const valid = !empty && parsed !== null && !outOfRange;

  useEffect(() => {
    onValidityChange?.(valid);
  }, [valid, onValidityChange]);

  return (
    <DecimalInput
      label={label}
      value={text}
      onValueChange={setText}
      onParsedChange={(n) => {
        // NR-2: no early-return that leaves parent number while the field shows
        // "abc" / "999". Only push a parsed value; clamp is applied by
        // DecimalInput before this callback on blur.
        if (n === null) return;
        onChange(n);
      }}
      min={min}
      max={max}
      step={step}
      maxFractionDigits={step !== undefined && step < 1 ? 2 : 0}
      isInvalid={!valid}
      errorMessage={!valid ? invalidMessage : undefined}
      labelPlacement="outside"
      variant="bordered"
      radius="lg"
      endContent={<span className="shrink-0 text-sm text-ink-500">{unit}</span>}
      // L5-34: same outside-label class convention as LoyaltyTab (L5-12).
      classNames={{ label: "text-sm font-medium text-ink-700" }}
      data-testid={testId}
    />
  );
}

/**
 * ScheduleSettingsModal is the manager editor (schedule:write) for the
 * per-business scheduling defaults the rest of the loop reads: week start,
 * default shift length, reminder lead + quiet hours (what Phase 3 delivers on),
 * the overtime threshold, publish-ahead lead, and the minor curfew. Money-free.
 * The server is the real gate; a non-writer's PUT 403s and surfaces as an error
 * toast.
 */
export default function ScheduleSettingsModal({
  businessId,
  onClose,
}: ScheduleSettingsModalProps) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const queryClient = useQueryClient();

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardScheduleSettings.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const settingsQuery = useQuery({
    queryKey: settingsKey(businessId),
    queryFn: () => scheduleSettingsApi.get(businessId),
    staleTime: 5 * 60 * 1000,
    retry: false,
  });

  const [form, setForm] = useState<ScheduleSettingsForm | null>(null);
  const [baseline, setBaseline] = useState<string | null>(null);
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);
  // NR-2: track per-field validity so Save disables on "abc" / out-of-range.
  const [fieldValidity, setFieldValidity] = useState({
    shiftHours: true,
    reminderLeadHours: true,
    overtimeHours: true,
    postedLeadDays: true,
  });
  const setFieldValid = useCallback(
    (key: keyof typeof fieldValidity, valid: boolean) => {
      setFieldValidity((prev) =>
        prev[key] === valid ? prev : { ...prev, [key]: valid },
      );
    },
    [],
  );
  useEffect(() => {
    if (settingsQuery.data && !form) {
      const next = formFromSettings(settingsQuery.data);
      setForm(next);
      setBaseline(JSON.stringify(next));
      setFieldValidity({
        shiftHours: true,
        reminderLeadHours: true,
        overtimeHours: true,
        postedLeadDays: true,
      });
    }
  }, [settingsQuery.data, form]);

  const upd = useCallback((patch: Partial<ScheduleSettingsForm>) => {
    setForm((f) => (f ? { ...f, ...patch } : f));
  }, []);

  const dirty =
    form != null && baseline != null && JSON.stringify(form) !== baseline;
  const numbersValid = Object.values(fieldValidity).every(Boolean);

  const requestClose = useCallback(() => {
    if (dirty) {
      setConfirmingDiscard(true);
      return;
    }
    onClose();
  }, [dirty, onClose]);

  const dialogRef = useRef<HTMLDivElement | null>(null);
  // Disable while discard ConfirmationModal owns focus (L5-30 Tab trap).
  useDialogKeyboard(dialogRef, requestClose, !confirmingDiscard);

  const save = useMutation({
    mutationFn: () =>
      scheduleSettingsApi.update(businessId, buildScheduleSettingsUpdate(form!)),
    onSuccess: (data) => {
      queryClient.setQueryData(settingsKey(businessId), data);
      toast.showSuccess(t("saved"));
      onClose();
    },
    onError: () => toast.showError(t("saveError")),
  });

  const weekdayName = useCallback(
    (day: number): string => weekStartDayName(day, locale),
    [locale],
  );

  return (
    <div
      ref={dialogRef}
      className="fixed inset-0 z-50 flex items-end justify-center bg-ink-950/40 p-0 backdrop-blur-sm motion-safe:animate-[cmdk-fade_120ms_ease-out] sm:items-center sm:p-4"
      role="dialog"
      aria-modal="true"
      aria-label={t("title")}
    >
      <div className="flex max-h-[92vh] w-full max-w-lg flex-col rounded-t-2xl border border-warm-200 bg-white shadow-xl motion-safe:animate-[cmdk-pop_150ms_cubic-bezier(0.16,1,0.3,1)] sm:rounded-2xl">
        <div className="flex items-start justify-between border-b border-warm-200 p-5">
          <div>
            <h3 className="font-title text-base text-ink-900">{t("title")}</h3>
            <p className="text-xs text-ink-500">{t("subtitle")}</p>
          </div>
          <button
            type="button"
            onClick={requestClose}
            aria-label={t("close")}
            className="rounded-lg p-1 text-ink-400 transition hover:bg-warm-100 hover:text-ink-700"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {!form ? (
          <div className="space-y-6 p-5" aria-busy="true" aria-hidden>
            {[0, 1, 2].map((s) => (
              <div key={s} className="space-y-3 motion-safe:animate-pulse">
                <SkeletonLine width="40%" height="0.75rem" />
                <SkeletonLine height="2.75rem" className="rounded-lg" />
                <SkeletonLine height="2.75rem" className="rounded-lg" />
              </div>
            ))}
          </div>
        ) : (
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault();
              if (!numbersValid) return;
              save.mutate();
            }}
            className="flex min-h-0 flex-1 flex-col"
          >
            <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-5">
              {/* Week & shifts — L5-34: flex+gap+pt-1 (not space-y) so outside
                  labels keep NextUI headroom under the fieldset legend. */}
              <fieldset
                className="flex flex-col gap-4 pt-1"
                data-testid="schedule-settings-week"
              >
                <legend className="text-sm font-semibold text-ink-900">
                  {t("sectionWeek")}
                </legend>
                <Select
                  label={t("weekStart")}
                  selectedKeys={[String(form.weekStartDay)]}
                  onSelectionChange={(keys) => {
                    const value = Array.from(keys as Set<string>)[0];
                    if (value == null || value === "") return;
                    const parsed = parseInt(value, 10);
                    if (!Number.isInteger(parsed)) return;
                    upd({
                      weekStartDay: Math.max(0, Math.min(6, parsed)),
                    });
                  }}
                  onChange={(e) => {
                    if (!e.target.value) return;
                    const parsed = parseInt(e.target.value, 10);
                    if (!Number.isInteger(parsed)) return;
                    upd({
                      weekStartDay: Math.max(0, Math.min(6, parsed)),
                    });
                  }}
                  labelPlacement="outside"
                  variant="bordered"
                  radius="lg"
                  classNames={{ label: "text-sm font-medium text-ink-700" }}
                >
                  {[0, 1, 2, 3, 4, 5, 6].map((d) => (
                    <SelectItem
                      key={String(d)}
                      textValue={weekdayName(d)}
                      className="capitalize"
                    >
                      {weekdayName(d)}
                    </SelectItem>
                  ))}
                </Select>
                <NumberField
                  label={t("shiftLength")}
                  value={form.shiftHours}
                  onChange={(n) => upd({ shiftHours: n })}
                  unit={t("hoursUnit")}
                  min={0}
                  max={168}
                  step={0.5}
                  invalidMessage={t("numberInvalid")}
                  onValidityChange={(v) => setFieldValid("shiftHours", v)}
                  testId="schedule-shift-hours"
                />
              </fieldset>

              {/* Reminders */}
              <fieldset
                className="flex flex-col gap-4 pt-1"
                data-testid="schedule-settings-reminders"
              >
                <legend className="text-sm font-semibold text-ink-900">
                  {t("sectionReminders")}
                </legend>
                <NumberField
                  label={t("reminderLead")}
                  value={form.reminderLeadHours}
                  onChange={(n) => upd({ reminderLeadHours: n })}
                  unit={t("hoursUnit")}
                  min={0}
                  max={168}
                  step={1}
                  invalidMessage={t("numberInvalid")}
                  onValidityChange={(v) => setFieldValid("reminderLeadHours", v)}
                  testId="schedule-reminder-lead"
                />
                <Switch
                  isSelected={form.quietEnabled}
                  onValueChange={(v) => upd({ quietEnabled: v })}
                  size="sm"
                  classNames={{ label: "text-sm font-medium text-ink-700" }}
                >
                  {t("quietHours")}
                </Switch>
                {form.quietEnabled ? (
                  <div className="space-y-2 pl-1">
                    <div className="grid grid-cols-2 gap-3">
                      <TimeField
                        label={t("from")}
                        value={form.quietStart}
                        onChange={(v) => upd({ quietStart: v })}
                      />
                      <TimeField
                        label={t("to")}
                        value={form.quietEnd}
                        onChange={(v) => upd({ quietEnd: v })}
                      />
                    </div>
                    <p className="text-xs text-ink-500">{t("quietHint")}</p>
                  </div>
                ) : null}
              </fieldset>

              {/* Overtime & posting */}
              <fieldset
                className="flex flex-col gap-4 pt-1"
                data-testid="schedule-settings-compliance"
              >
                <legend className="text-sm font-semibold text-ink-900">
                  {t("sectionCompliance")}
                </legend>
                <NumberField
                  label={t("overtime")}
                  value={form.overtimeHours}
                  onChange={(n) => upd({ overtimeHours: n })}
                  unit={t("hoursPerWeek")}
                  min={0}
                  max={168}
                  step={1}
                  invalidMessage={t("numberInvalid")}
                  onValidityChange={(v) => setFieldValid("overtimeHours", v)}
                  testId="schedule-overtime-hours"
                />
                <NumberField
                  label={t("postedLead")}
                  value={form.postedLeadDays}
                  onChange={(n) => upd({ postedLeadDays: n })}
                  unit={t("daysUnit")}
                  min={0}
                  max={60}
                  step={1}
                  invalidMessage={t("numberInvalid")}
                  onValidityChange={(v) => setFieldValid("postedLeadDays", v)}
                  testId="schedule-posted-lead"
                />
                <Switch
                  isSelected={form.minorEnabled}
                  onValueChange={(v) => upd({ minorEnabled: v })}
                  size="sm"
                  classNames={{ label: "text-sm font-medium text-ink-700" }}
                >
                  {t("minorCurfew")}
                </Switch>
                {form.minorEnabled ? (
                  <div className="space-y-2 pl-1">
                    <TimeField
                      label={t("noShiftsPast")}
                      value={form.minorCutoff}
                      onChange={(v) => upd({ minorCutoff: v })}
                    />
                    <p className="text-xs text-ink-500">{t("minorHint")}</p>
                  </div>
                ) : null}
              </fieldset>
            </div>

            <div className="flex items-center justify-end gap-2 border-t border-warm-200 p-4">
              <Button variant="light" onPress={requestClose} type="button">
                {t("cancel")}
              </Button>
              <Button
                color="primary"
                type="submit"
                isLoading={save.isPending}
                isDisabled={!numbersValid}
              >
                {save.isPending ? t("saving") : t("save")}
              </Button>
            </div>
          </form>
        )}
      </div>

      <ConfirmationModal
        isOpen={confirmingDiscard}
        onOpenChange={() => setConfirmingDiscard(false)}
        title={t("discardConfirmTitle")}
        description={t("discardConfirmDescription")}
        confirmLabel={t("discardConfirm")}
        cancelLabel={t("discardKeep")}
        isDanger
        onConfirm={() => {
          setConfirmingDiscard(false);
          onClose();
        }}
      />
    </div>
  );
}
