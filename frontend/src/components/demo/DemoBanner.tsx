"use client";

import { useEffect, useId, useRef, useState } from "react";
import { useInstance } from "@/hooks/useInstance";
import { isPublicDemo } from "@/lib/instance/instanceInfo";
import { PROJECT_REPO_URL } from "@/config/brand";
import { fetchDemoTables, type DemoVenue } from "@/api/demo";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";

export const DEMO_INSTALL_URL = `${PROJECT_REPO_URL}/blob/main/docs/self-hosting/install.md`;

// "/" is the demo venue's storefront, which has no sign-in of its own: the
// banner links to the dashboard's sign-in gate, which opens the one-click
// demo buttons straight away.
export const DEMO_ENTER_HREF = "/dashboard?auth=signin";

const COPY = {
  en: {
    region: "Public demo notice",
    notice: (time: string) =>
      `Public demo — anyone can see changes, everything resets nightly at ${time} UTC`,
    enter: "Enter as owner or staff",
    github: "GitHub",
    install: "Install your own",
    guest: "Try as a guest",
    hideGuest: "Hide guest tables",
    guestIntro:
      "Open a table like a diner would: scan the QR code with your phone or follow a link. Order and split the bill with no sign-in, then pay at the counter. To see the payment land, enter the demo as the owner in another tab and confirm it on the bill: no real money moves.",
    storefront: "Storefront",
    venue: (n: number) => `Demo venue ${n}`,
    loading: "Loading tables…",
    failed: "Could not load the demo tables.",
    qrAlt: (table: string) => `QR code for ${table}`,
  },
  es: {
    region: "Aviso de demo pública",
    notice: (time: string) =>
      `Demo pública — cualquiera puede ver los cambios, todo se reinicia cada noche a las ${time} UTC`,
    enter: "Entrar como dueño o staff",
    github: "GitHub",
    install: "Instalá el tuyo",
    guest: "Probar como cliente",
    hideGuest: "Ocultar mesas",
    guestIntro:
      "Abrí una mesa como lo haría un comensal: escaneá el QR con tu teléfono o seguí un enlace. Pedí y dividí la cuenta sin iniciar sesión, y pagá en caja. Para ver el pago, entrá a la demo como dueño en otra pestaña y confirmalo en la cuenta: no se mueve dinero real.",
    storefront: "Vidriera",
    venue: (n: number) => `Local demo ${n}`,
    loading: "Cargando mesas…",
    failed: "No se pudieron cargar las mesas de la demo.",
    qrAlt: (table: string) => `Código QR de ${table}`,
  },
} as const;

type Copy = (typeof COPY)[keyof typeof COPY];

function copyFor(locale: string | undefined): Copy {
  return locale?.toLowerCase().startsWith("es") ? COPY.es : COPY.en;
}

function TableQr({ href, alt }: { href: string; alt: string }) {
  const [src, setSrc] = useState("");
  useEffect(() => {
    let alive = true;
    const absolute =
      typeof window === "undefined" ? href : new URL(href, window.location.origin).toString();
    // Loaded on demand: the banner sits in the root layout, and the QR
    // encoder is only needed once a visitor opens the guest panel.
    import("qrcode")
      .then((mod) => mod.default.toDataURL(absolute, { margin: 1, width: 112 }))
      .then((url: string) => {
        if (alive) setSrc(url);
      })
      .catch(() => {
        if (alive) setSrc("");
      });
    return () => {
      alive = false;
    };
  }, [href]);
  if (!src) return <div className="h-28 w-28 rounded bg-warm-100" aria-hidden="true" />;
  // eslint-disable-next-line @next/next/no-img-element -- data: URL generated on the client
  return <img src={src} alt={alt} width={112} height={112} className="h-28 w-28 rounded" />;
}

function GuestTables({ copy }: { copy: Copy }) {
  const [state, setState] = useState<{ venues: DemoVenue[]; status: "loading" | "ok" | "error" }>({
    venues: [],
    status: "loading",
  });
  useEffect(() => {
    let alive = true;
    fetchDemoTables().then(
      (venues) => alive && setState({ venues, status: "ok" }),
      () => alive && setState({ venues: [], status: "error" }),
    );
    return () => {
      alive = false;
    };
  }, []);

  if (state.status === "loading") return <p className="text-sm">{copy.loading}</p>;
  if (state.status === "error") return <p className="text-sm">{copy.failed}</p>;
  return (
    <div className="space-y-4">
      <p className="text-sm text-ink-700">{copy.guestIntro}</p>
      {/* A fixed label, never the venue name: the banner sits on every page,
          and a shared demo session must not be able to put words in it. */}
      {state.venues.map((venue, index) => (
        <section key={venue.custom_url || index} aria-label={copy.venue(index + 1)}>
          <h3 className="text-sm font-semibold text-ink-900">
            {copy.venue(index + 1)}
            {venue.custom_url ? (
              <>
                {" · "}
                <a className="underline" href={`/b/${encodeURIComponent(venue.custom_url)}`}>
                  {copy.storefront}
                </a>
              </>
            ) : null}
          </h3>
          <ul className="mt-2 flex flex-wrap gap-3">
            {venue.tables.map((table) => {
              const href = `/t/${encodeURIComponent(table.code)}`;
              return (
                <li key={table.code} className="flex flex-col items-center gap-1 rounded-lg border border-warm-200 bg-white p-2">
                  <TableQr href={href} alt={copy.qrAlt(table.name || table.code)} />
                  <a className="text-xs font-medium underline" href={href}>
                    {table.name || table.code}
                  </a>
                </li>
              );
            })}
          </ul>
        </section>
      ))}
    </div>
  );
}

// The site header (TopMenuShell) is `fixed top-0`, so the in-flow banner
// would sit on top of it. While the banner is on screen, the header moves
// down by the part of the banner still visible and slides back to the top as
// the banner scrolls away. Scoped to the demo so a normal install is untouched.
const DEMO_HEADER_OFFSET_CSS = `html[data-public-demo] nav.fixed.top-0{top:var(--demo-banner-offset,0px);transition-property:background-color,border-color,box-shadow,backdrop-filter}`;

function useHeaderOffset(ref: React.RefObject<HTMLDivElement | null>, active: boolean) {
  useEffect(() => {
    const el = ref.current;
    if (!active || !el) return;
    const root = document.documentElement;
    root.setAttribute("data-public-demo", "");
    let frame = 0;
    const update = () => {
      frame = 0;
      const bottom = Math.max(0, Math.round(el.getBoundingClientRect().bottom));
      root.style.setProperty("--demo-banner-offset", `${bottom}px`);
    };
    const schedule = () => {
      if (!frame) frame = window.requestAnimationFrame(update);
    };
    update();
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule);
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(schedule);
    observer?.observe(el);
    return () => {
      if (frame) window.cancelAnimationFrame(frame);
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
      observer?.disconnect();
      root.removeAttribute("data-public-demo");
      root.style.removeProperty("--demo-banner-offset");
    };
  }, [ref, active]);
}

/**
 * The public-demo notice (DEMO_MODE only): shown at the top of every page —
 * operator, staff, admin and storefront — so nobody mistakes the shared demo
 * for a private install. Renders nothing on a normal install.
 */
export default function DemoBanner() {
  const { instance } = useInstance();
  const { locale } = useSimpleLocale();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const bannerRef = useRef<HTMLDivElement>(null);
  const active = isPublicDemo(instance);
  useHeaderOffset(bannerRef, active);
  if (!active) return null;
  const copy = copyFor(locale);
  const resetTime = instance?.demo.reset_utc || "03:00";

  return (
    <div
      ref={bannerRef}
      role="region"
      aria-label={copy.region}
      data-testid="demo-banner"
      className="relative z-[60] border-b border-amber-200 bg-amber-50 px-4 py-2 text-sm text-ink-900"
    >
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-4 gap-y-1">
        <p className="min-w-0 flex-1 font-medium">{copy.notice(resetTime)}</p>
        <nav className="flex flex-wrap items-center gap-x-3 gap-y-1" aria-label={copy.region}>
          <a className="font-semibold underline underline-offset-2" href={DEMO_ENTER_HREF}>
            {copy.enter}
          </a>
          <button
            type="button"
            className="underline underline-offset-2"
            aria-expanded={open}
            aria-controls={panelId}
            onClick={() => setOpen((v) => !v)}
          >
            {open ? copy.hideGuest : copy.guest}
          </button>
          <a className="underline underline-offset-2" href={PROJECT_REPO_URL} target="_blank" rel="noopener noreferrer">
            {copy.github}
          </a>
          <a
            className="rounded-md bg-ink-900 px-2.5 py-1 font-medium text-white hover:bg-ink-800"
            href={DEMO_INSTALL_URL}
            target="_blank"
            rel="noopener noreferrer"
          >
            {copy.install}
          </a>
        </nav>
      </div>
      <style>{DEMO_HEADER_OFFSET_CSS}</style>
      {open ? (
        <div id={panelId} className="mx-auto mt-3 max-w-6xl pb-2">
          <GuestTables copy={copy} />
        </div>
      ) : null}
    </div>
  );
}
