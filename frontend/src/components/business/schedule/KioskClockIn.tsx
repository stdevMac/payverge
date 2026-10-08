"use client";

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@nextui-org/react";
import { X, Delete, CheckCircle2, KeyRound, LogIn, LogOut, ChevronLeft } from "lucide-react";
import {
  timeclockApi,
  type KioskStaffMember,
  type KioskPunchResult,
} from "@/api/timeclock";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonLine } from "@/components/ui/skeletons";
import { useDialogKeyboard } from "./useDialogKeyboard";

interface KioskClockInProps {
  businessId: string;
  onClose: () => void;
}

const PIN_MIN = 4;
const PIN_MAX = 12;

// Narrow an unknown thrown value to its HTTP status without an `any` cast.
function errStatus(err: unknown): number | undefined {
  if (err && typeof err === "object" && "response" in err) {
    return (err as { response?: { status?: number } }).response?.status;
  }
  return undefined;
}

/**
 * KioskClockIn is the shared-terminal clock-in surface (Phase 4b). A manager
 * launches it on a counter tablet; the device stays on the manager's session
 * (timeclock:manage) and each staffer taps their name + enters a PIN to TOGGLE
 * their own punch. No independent staff session is minted — the PIN only
 * attributes the punch. Money-free: names + hours-context only, never a pay
 * figure. The server is the real gate; a non-manager's reads/writes 403 and the
 * surface simply shows nothing / an error.
 */
export default function KioskClockIn({ businessId, onClose }: KioskClockInProps) {
  const { locale } = useSimpleLocale();
  const queryClient = useQueryClient();

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardKiosk.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const fmtTime = useCallback(
    (iso?: string): string => {
      if (!iso) return "";
      return new Intl.DateTimeFormat(intlLocaleFor(locale), {
        hour: "numeric",
        minute: "2-digit",
      }).format(new Date(iso));
    },
    [locale],
  );

  const rosterQuery = useQuery({
    queryKey: queryKeys.kiosk.roster(businessId),
    queryFn: () => timeclockApi.kioskRoster(businessId),
    refetchInterval: 30_000, // reflect punches from other people / devices
    refetchOnWindowFocus: true,
    retry: false, // a non-manager 403 must not retry-thrash
  });

  const roster = useMemo<KioskStaffMember[]>(
    () => rosterQuery.data ?? [],
    [rosterQuery.data],
  );
  const enrolled = useMemo(() => roster.filter((s) => s.has_pin), [roster]);
  const needPin = roster.length - enrolled.length;

  const [selected, setSelected] = useState<KioskStaffMember | null>(null);
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [flash, setFlash] = useState<KioskPunchResult | null>(null);
  const flashTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (flashTimer.current) clearTimeout(flashTimer.current);
    },
    [],
  );

  const backToGrid = useCallback(() => {
    setSelected(null);
    setPin("");
    setError(null);
  }, []);

  // Escape-to-close + Tab focus trap for this full-screen hand-rolled dialog.
  const dialogRef = useRef<HTMLDivElement | null>(null);
  useDialogKeyboard(dialogRef, onClose);

  const punch = useMutation({
    mutationFn: (vars: { staffId: number; pin: string }) =>
      timeclockApi.kioskPunch(businessId, vars.staffId, vars.pin),
    onSuccess: (res) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.kiosk.roster(businessId),
      });
      setSelected(null);
      setPin("");
      setError(null);
      setFlash(res);
      if (flashTimer.current) clearTimeout(flashTimer.current);
      flashTimer.current = setTimeout(() => setFlash(null), 2400);
    },
    onError: (err) => {
      setPin("");
      const status = errStatus(err);
      // #883: this map IS the kiosk's error copy — the server's `message` is
      // never rendered here, so a status the map misses is a status with no
      // real copy. 401 is the gap that matters: the tablet holds the MANAGER's
      // session, and when it dies mid-shift every punch 401s. That fell to
      // "couldn't record that, try again", which sends a staffer re-tapping a
      // PIN that can never work instead of fetching whoever can sign back in.
      // 400/500 stay on the generic arm on purpose — neither is a statement
      // about the PIN, so "try again" is the honest advice for both.
      if (status === 429) setError(t("errLocked"));
      else if (status === 403) setError(t("errWrongPin"));
      else if (status === 404) setError(t("errNotFound"));
      else if (status === 409) setError(t("errNoPin"));
      else if (status === 401) setError(t("errSessionExpired"));
      else setError(t("errGeneric"));
    },
  });

  const pressDigit = useCallback((d: string) => {
    setError(null);
    setPin((p) => (p.length >= PIN_MAX ? p : p + d));
  }, []);
  const backspace = useCallback(() => {
    setError(null);
    setPin((p) => p.slice(0, -1));
  }, []);
  const submit = useCallback(() => {
    if (!selected || pin.length < PIN_MIN || punch.isPending) return;
    punch.mutate({ staffId: selected.staff_id, pin });
  }, [selected, pin, punch]);

  // ---- success flash ------------------------------------------------------
  const flashView = flash ? (
    <div className="flex flex-1 flex-col items-center justify-center gap-4 p-8 text-center">
      <CheckCircle2 className="h-16 w-16 text-emerald-500" aria-hidden />
      <p className="font-title text-2xl text-ink-900">
        {(flash.action === "clocked_in"
          ? t("clockedInFlash")
          : t("clockedOutFlash")
        ).replace("{name}", flash.name)}
      </p>
    </div>
  ) : null;

  // ---- PIN pad ------------------------------------------------------------
  const padView =
    selected && !flash ? (
      <div className="flex flex-1 flex-col items-center justify-center gap-6 p-6">
        <div className="text-center">
          <button
            type="button"
            onClick={backToGrid}
            className="mb-3 inline-flex items-center gap-1 text-sm text-ink-500 transition hover:text-ink-800"
          >
            <ChevronLeft className="h-4 w-4" />
            {t("back")}
          </button>
          <h3 className="font-title text-2xl text-ink-900">{selected.name}</h3>
          <p className="mt-1 text-sm text-ink-500">{t("enterPin")}</p>
        </div>

        <div
          className="flex h-8 items-center gap-2"
          aria-label={t("pinEntry")}
          role="status"
        >
          {Array.from({ length: Math.max(pin.length, PIN_MIN) }).map((_, i) => (
            <span
              key={i}
              className={`h-3 w-3 rounded-full ${
                i < pin.length ? "bg-warm-900" : "bg-warm-200"
              }`}
            />
          ))}
        </div>

        <p
          className={`h-5 text-sm ${error ? "text-red-600" : "text-transparent"}`}
          role={error ? "alert" : undefined}
        >
          {error || "·"}
        </p>

        <div className="grid w-full max-w-xs grid-cols-3 gap-3">
          {["1", "2", "3", "4", "5", "6", "7", "8", "9"].map((d) => (
            <button
              key={d}
              type="button"
              onClick={() => pressDigit(d)}
              className="rounded-xl border border-warm-200 py-4 text-2xl font-medium text-ink-900 transition hover:bg-warm-50 active:bg-warm-100"
            >
              {d}
            </button>
          ))}
          <button
            type="button"
            onClick={backToGrid}
            className="rounded-xl py-4 text-sm text-ink-500 transition hover:bg-warm-50"
          >
            {t("cancel")}
          </button>
          <button
            type="button"
            onClick={() => pressDigit("0")}
            className="rounded-xl border border-warm-200 py-4 text-2xl font-medium text-ink-900 transition hover:bg-warm-50 active:bg-warm-100"
          >
            0
          </button>
          <button
            type="button"
            onClick={backspace}
            aria-label={t("backspace")}
            className="flex items-center justify-center rounded-xl py-4 text-ink-500 transition hover:bg-warm-50"
          >
            <Delete className="h-6 w-6" />
          </button>
        </div>

        <Button
          color="primary"
          size="lg"
          className="w-full max-w-xs"
          isDisabled={pin.length < PIN_MIN}
          isLoading={punch.isPending}
          startContent={
            !punch.isPending ? (
              selected.on_clock ? (
                <LogOut className="h-5 w-5" />
              ) : (
                <LogIn className="h-5 w-5" />
              )
            ) : undefined
          }
          onPress={submit}
        >
          {selected.on_clock ? t("clockOut") : t("clockIn")}
        </Button>
      </div>
    ) : null;

  // ---- roster grid --------------------------------------------------------
  const gridView =
    !selected && !flash ? (
      <div className="flex-1 overflow-y-auto p-6">
        {rosterQuery.isLoading ? (
          <div
            role="status"
            aria-live="polite"
            className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4"
          >
            {Array.from({ length: 8 }).map((_, i) => (
              <div
                key={i}
                className="space-y-2 rounded-xl border border-warm-200 p-4 motion-safe:animate-pulse"
              >
                <SkeletonLine width="70%" height="1rem" />
                <SkeletonLine width="45%" height="0.75rem" />
              </div>
            ))}
          </div>
        ) : rosterQuery.isError ? (
          // A roster fetch failure must NOT read as "no PINs enrolled" — surface
          // the error with a retry instead of the misleading empty state.
          <div className="flex flex-col items-center gap-3 p-8 text-center">
            <p className="text-sm text-ink-600">{t("loadError")}</p>
            <Button
              size="sm"
              variant="bordered"
              onPress={() => rosterQuery.refetch()}
            >
              {t("retry")}
            </Button>
          </div>
        ) : enrolled.length === 0 ? (
          <EmptyState
            icon={KeyRound}
            title={t("emptyTitle")}
            subtitle={t("emptyBody")}
          />
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
              {enrolled.map((s) => (
                <button
                  key={s.staff_id}
                  type="button"
                  onClick={() => {
                    setSelected(s);
                    setPin("");
                    setError(null);
                  }}
                  className="flex flex-col items-start gap-2 rounded-xl border border-warm-200 p-4 text-left transition hover:border-brand-400 hover:bg-brand-50/40"
                >
                  <span className="w-full truncate font-medium text-ink-900">
                    {s.name}
                  </span>
                  {s.on_clock ? (
                    <span className="inline-flex items-center gap-1.5 text-xs text-emerald-600">
                      <span className="h-2 w-2 rounded-full bg-emerald-500" />
                      {t("since").replace("{time}", fmtTime(s.clock_in_at))}
                    </span>
                  ) : (
                    <span className="text-xs text-ink-500">{t("offClock")}</span>
                  )}
                </button>
              ))}
            </div>
            {needPin > 0 ? (
              <p className="mt-6 text-center text-xs text-ink-500">
                {t("needPinNote").replace("{count}", String(needPin))}
              </p>
            ) : null}
          </>
        )}
      </div>
    ) : null;

  return (
    <div
      ref={dialogRef}
      className="fixed inset-0 z-[60] flex flex-col bg-warm-50"
      role="dialog"
      aria-modal="true"
      aria-label={t("title")}
    >
      <header className="flex items-center justify-between border-b border-warm-200 bg-white px-5 py-4">
        <div>
          <h2 className="font-title text-lg text-ink-900">{t("title")}</h2>
          <p className="text-xs text-ink-500">{t("subtitle")}</p>
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label={t("close")}
          className="rounded-lg p-2 text-ink-400 transition hover:bg-warm-100 hover:text-ink-700"
        >
          <X className="h-5 w-5" />
        </button>
      </header>

      {flashView}
      {padView}
      {gridView}
    </div>
  );
}
