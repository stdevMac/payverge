"use client";

import Image from "next/image";
import { Check } from "lucide-react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { useInstance, type FeatureName } from "@/hooks/useInstance";

const PRODUCT_CROP_SRC = "/onboarding/analytics.webp";

interface Bullet {
  title: string;
  description: string;
  /** Instance feature the bullet advertises; hidden when the server has it off. */
  feature?: FeatureName;
}

function useValuePropCopy() {
  const { locale } = useSimpleLocale();
  const { isOff } = useInstance();
  const title = getTranslation("dashboard.authentication.valueProp.title", locale);
  const imageAlt = getTranslation(
    "dashboard.authentication.valueProp.imageAlt",
    locale,
  );
  const rawBullets = getTranslation(
    "dashboard.authentication.valueProp.bullets",
    locale,
  );
  const bullets: Bullet[] = (
    Array.isArray(rawBullets) ? (rawBullets as unknown as Bullet[]) : []
  ).filter((b) => !b.feature || !isOff(b.feature));
  const compact = bullets
    .map((b) => b.title)
    .filter((t) => typeof t === "string" && t.length > 0)
    .join(" · ");
  return {
    title: typeof title === "string" ? title : "",
    imageAlt: typeof imageAlt === "string" ? imageAlt : "",
    bullets,
    compact,
  };
}

/** One Director / AI / fees line under the sign-in CTA on narrow viewports. */
export function AuthValuePropCompact() {
  const { compact } = useValuePropCopy();
  if (!compact) return null;
  return (
    <p
      data-testid="auth-value-prop-compact"
      className="md:hidden text-sm text-ink-600 leading-relaxed"
    >
      {compact}
    </p>
  );
}

export function AuthValueProp() {
  const { title, imageAlt, bullets } = useValuePropCopy();

  return (
    <aside className="hidden md:flex flex-col justify-center h-full px-10 py-16 bg-gradient-to-br from-brand/5 via-warm-50 to-warm-100 border-l border-warm-200">
      <div className="relative rounded-3xl bg-warm-100 aspect-[4/3] min-h-[14rem] mb-10 overflow-hidden ring-1 ring-warm-200">
        <Image
          src={PRODUCT_CROP_SRC}
          alt={imageAlt}
          fill
          className="object-contain object-top"
          sizes="(min-width: 768px) 50vw, 0px"
          priority
        />
      </div>
      <h2 className="font-title text-2xl md:text-3xl text-ink-950 mb-6">{title}</h2>
      <ul className="space-y-5 max-w-md">
        {bullets.map((b) => (
          <li key={b.title} className="flex gap-3">
            <span className="mt-1 flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-brand/15 text-brand">
              <Check className="h-3.5 w-3.5" strokeWidth={3} />
            </span>
            <div>
              <p className="font-semibold text-ink-900">{b.title}</p>
              <p className="text-sm text-ink-700 leading-relaxed">{b.description}</p>
            </div>
          </li>
        ))}
      </ul>
    </aside>
  );
}
