"use client";

import { getPublicConfig } from "@/config/publicConfig";
import { Suspense, useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { RouteLoadingFallback } from "@/components/ui/AsyncState";

const apiBase = () => getPublicConfig().apiUrl;

// Copy travels with the email that linked here (lang stamped at send time by
// the backend footer): the audience is operators and email ships eng/es/es_ar,
// so this page deliberately avoids the 21-locale guest tier.
const COPY = {
  eng: {
    title: "Unsubscribe from marketing emails",
    confirm: "Unsubscribe",
    confirming: "Unsubscribing…",
    done: "You're unsubscribed. We'll only email you about billing and account-critical events.",
    prompt: (email: string) => `Stop marketing emails to ${email}?`,
    invalid:
      "This unsubscribe link is invalid or has expired. You can manage all email preferences from your account settings.",
    error: "Something went wrong. Please try again.",
  },
  es: {
    title: "Cancelar la suscripción a los correos de marketing",
    confirm: "Cancelar suscripción",
    confirming: "Cancelando…",
    done: "Listo, cancelamos tu suscripción. Solo te escribiremos por facturación y eventos críticos de tu cuenta.",
    prompt: (email: string) => `¿Dejar de enviar correos de marketing a ${email}?`,
    invalid:
      "Este enlace no es válido o ya venció. Puedes gestionar tus preferencias de correo desde la configuración de tu cuenta.",
    error: "Algo salió mal. Inténtalo de nuevo.",
  },
  es_ar: {
    title: "Desuscribite de los mails de marketing",
    confirm: "Desuscribirme",
    confirming: "Cancelando…",
    done: "Listo, te desuscribimos. Solo te vamos a escribir por facturación y eventos críticos de tu cuenta.",
    prompt: (email: string) => `¿Dejamos de mandarte mails de marketing a ${email}?`,
    invalid:
      "Este enlace no es válido o ya venció. Podés gestionar tus preferencias de mail desde la configuración de tu cuenta.",
    error: "Algo salió mal. Probá de nuevo.",
  },
} as const;

type CopyLang = keyof typeof COPY;

function UnsubscribeInner() {
  const searchParams = useSearchParams();
  const token = searchParams?.get("token") || "";
  const langParam = (searchParams?.get("lang") || "eng") as CopyLang;
  const copy = COPY[langParam] ?? COPY.eng;

  const [state, setState] = useState<
    "loading" | "ready" | "working" | "done" | "invalid" | "error"
  >("loading");
  const [email, setEmail] = useState("");

  useEffect(() => {
    if (!token) {
      setState("invalid");
      return;
    }
    fetch(`${apiBase()}/email/unsubscribe?token=${encodeURIComponent(token)}`)
      .then(async (res) => {
        if (!res.ok) throw new Error("invalid");
        const body = await res.json();
        setEmail(body.email || "");
        setState("ready");
      })
      .catch(() => setState("invalid"));
  }, [token]);

  const confirm = useCallback(() => {
    setState("working");
    fetch(`${apiBase()}/email/unsubscribe?token=${encodeURIComponent(token)}`, {
      method: "POST",
    })
      .then((res) => setState(res.ok ? "done" : "error"))
      .catch(() => setState("error"));
  }, [token]);

  return (
    <section className="mx-auto flex min-h-[60vh] max-w-lg flex-col items-center justify-center gap-6 px-6 py-16 text-center">
      <h1 className="font-serif text-2xl text-ink-900">{copy.title}</h1>
      {state === "loading" && <p className="text-ink-500">…</p>}
      {state === "invalid" && <p className="text-ink-600">{copy.invalid}</p>}
      {state === "error" && <p className="text-rose-700">{copy.error}</p>}
      {state === "done" && <p className="text-ink-700">{copy.done}</p>}
      {(state === "ready" || state === "working" || state === "error") && email && (
        <>
          <p className="text-ink-700">{copy.prompt(email)}</p>
          <button
            type="button"
            onClick={confirm}
            disabled={state === "working"}
            className="rounded-md bg-brand px-5 py-2.5 font-medium text-white disabled:opacity-60"
          >
            {state === "working" ? copy.confirming : copy.confirm}
          </button>
        </>
      )}
    </section>
  );
}

export default function UnsubscribePage() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <UnsubscribeInner />
    </Suspense>
  );
}
