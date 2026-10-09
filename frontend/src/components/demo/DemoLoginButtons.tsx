"use client";

import { useState } from "react";
import { demoLogin, type DemoRole, type DemoStaff } from "@/api/demo";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";

const COPY = {
  en: {
    heading: "Explore the public demo",
    owner: "Enter demo as Owner",
    kitchen: "Enter demo as Staff (kitchen)",
    waiter: "Enter demo as Staff (waiter)",
    hint: "No account needed. Everyone shares this demo and it resets nightly.",
    starting: "Starting…",
  },
  es: {
    heading: "Explorá la demo pública",
    owner: "Entrar a la demo como Dueño",
    kitchen: "Entrar a la demo como Staff (cocina)",
    waiter: "Entrar a la demo como Staff (mozo)",
    hint: "No hace falta cuenta. Todos comparten esta demo y se reinicia cada noche.",
    starting: "Entrando…",
  },
} as const;

export interface DemoLoginButtonsProps {
  /** Called after the owner session cookie is set; hydrate + navigate. */
  onOwnerSignedIn: (redirect: string) => void | Promise<void>;
  /** Called after a staff session cookie is set, with the staff payload. */
  onStaffSignedIn: (staff: DemoStaff) => void | Promise<void>;
}

/**
 * One-click sign-in for the public demo (DEMO_MODE): owner, kitchen staff or
 * waiter. The server issues a normal session for a fixed demo identity; no
 * password exists or is shown. Callers render it only when
 * isPublicDemo(instance).
 */
export default function DemoLoginButtons({ onOwnerSignedIn, onStaffSignedIn }: DemoLoginButtonsProps) {
  const { locale } = useSimpleLocale();
  const copy = locale?.toLowerCase().startsWith("es") ? COPY.es : COPY.en;
  const [busy, setBusy] = useState<DemoRole | null>(null);
  const [error, setError] = useState("");

  const enter = async (role: DemoRole) => {
    if (busy) return;
    setBusy(role);
    setError("");
    try {
      const result = await demoLogin(role);
      if (result.kind === "owner") {
        await onOwnerSignedIn(result.redirect);
      } else {
        await onStaffSignedIn(result.staff);
      }
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : "Could not start the demo");
    } finally {
      setBusy(null);
    }
  };

  const buttons: { role: DemoRole; label: string; primary: boolean }[] = [
    { role: "owner", label: copy.owner, primary: true },
    { role: "kitchen", label: copy.kitchen, primary: false },
    { role: "waiter", label: copy.waiter, primary: false },
  ];

  return (
    <section
      aria-labelledby="demo-login-heading"
      data-testid="demo-login"
      className="mb-4 rounded-xl border border-amber-200 bg-amber-50 p-4"
    >
      <h3 id="demo-login-heading" className="text-sm font-semibold text-ink-900">
        {copy.heading}
      </h3>
      <div className="mt-3 flex flex-col gap-2">
        {buttons.map((b) => (
          <button
            key={b.role}
            type="button"
            disabled={busy !== null}
            onClick={() => void enter(b.role)}
            className={
              b.primary
                ? "w-full rounded-lg bg-ink-900 px-4 py-2.5 text-sm font-medium text-white hover:bg-ink-800 disabled:opacity-60"
                : "w-full rounded-lg border border-ink-300 bg-white px-4 py-2.5 text-sm font-medium text-ink-900 hover:bg-warm-50 disabled:opacity-60"
            }
          >
            {busy === b.role ? copy.starting : b.label}
          </button>
        ))}
      </div>
      <p className="mt-2 text-xs text-ink-700">{copy.hint}</p>
      {error ? (
        <p role="alert" className="mt-2 text-sm text-rose-700">
          {error}
        </p>
      ) : null}
    </section>
  );
}
