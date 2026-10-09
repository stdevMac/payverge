import Link from "next/link";
import type { HomeVenue } from "@/lib/instance/serverHome";
import { guestPublicPath } from "@/lib/seo/guestUrls";

/**
 * Instance root for a multi-venue install: one card per published storefront
 * (name, logo, city) linking to its /b/<slug> page. Server component; the
 * copy is short and kept in the en/es/es-AR tier with an English fallback because
 * every venue page it links to carries the full 21-locale guest experience.
 */

const COPY = {
  en: {
    eyebrow: "Our venues",
    heading: "Choose a location",
    lede: "Menus, ordering, reservations and payments for each of our venues.",
    open: "Open",
    staff: "Staff sign-in",
  },
  // Neutral Spanish (tú); the Rioplatense voseo lives in es-AR only.
  es: {
    eyebrow: "Nuestros locales",
    heading: "Elige un local",
    lede: "Menú, pedidos, reservas y pagos de cada uno de nuestros locales.",
    open: "Abrir",
    staff: "Acceso del equipo",
  },
  "es-AR": {
    eyebrow: "Nuestros locales",
    heading: "Elegí un local",
    lede: "Menú, pedidos, reservas y pagos de cada uno de nuestros locales.",
    open: "Abrir",
    staff: "Acceso del equipo",
  },
} as const;

export function directoryCopy(locale: string) {
  const code = locale.toLowerCase();
  if (code === "es-ar") return COPY["es-AR"];
  return code.startsWith("es") ? COPY.es : COPY.en;
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  return (parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "");
}

export interface VenueDirectoryProps {
  venues: HomeVenue[];
  locale: string;
  title: string;
}

export default function VenueDirectory({
  venues,
  locale,
  title,
}: VenueDirectoryProps) {
  const copy = directoryCopy(locale);
  return (
    <main
      id="main-content"
      tabIndex={-1}
      className="min-h-screen bg-warm-50 text-ink-950"
    >
      <div className="mx-auto max-w-5xl px-4 py-16 sm:px-6 sm:py-24">
        <header className="max-w-2xl">
          <p className="text-sm font-semibold uppercase tracking-wider text-brand">
            {copy.eyebrow}
          </p>
          <h1 className="mt-3 font-serif text-4xl leading-tight sm:text-5xl">
            {title}
          </h1>
          <p className="mt-4 text-lg text-warm-600">{copy.lede}</p>
        </header>

        <h2 className="sr-only">{copy.heading}</h2>
        <ul
          className="mt-12 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3"
          aria-label={copy.heading}
        >
          {venues.map((venue) => (
            <li key={venue.id}>
              <Link
                href={guestPublicPath(locale, `/b/${venue.custom_url}`)}
                className="group flex h-full items-center gap-4 rounded-2xl border border-black/5 bg-white p-5 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 motion-reduce:transform-none motion-reduce:transition-none"
              >
                {venue.logo ? (
                  // eslint-disable-next-line @next/next/no-img-element -- venue logos are operator-hosted on arbitrary media origins
                  <img
                    src={venue.logo}
                    alt=""
                    width={56}
                    height={56}
                    loading="lazy"
                    className="h-14 w-14 flex-none rounded-xl object-cover"
                  />
                ) : (
                  <span
                    aria-hidden="true"
                    className="flex h-14 w-14 flex-none items-center justify-center rounded-xl bg-brand/10 font-serif text-xl uppercase text-brand"
                  >
                    {initials(venue.name)}
                  </span>
                )}
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-lg font-semibold">
                    {venue.name}
                  </span>
                  {venue.city ? (
                    <span className="block truncate text-sm text-warm-600">
                      {venue.city}
                    </span>
                  ) : null}
                </span>
                <span
                  aria-hidden="true"
                  className="text-sm font-medium text-brand opacity-70 transition group-hover:opacity-100"
                >
                  {copy.open} →
                </span>
              </Link>
            </li>
          ))}
        </ul>

        <footer className="mt-16 border-t border-black/5 pt-6 text-sm text-warm-600">
          <Link href="/dashboard" className="hover:text-ink-950 underline-offset-4 hover:underline">
            {copy.staff}
          </Link>
        </footer>
      </div>
    </main>
  );
}
