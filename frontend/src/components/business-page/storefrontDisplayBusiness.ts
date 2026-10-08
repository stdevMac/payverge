import type { PublicBusiness } from "@/api/publicBusiness";

/**
 * Pick the storefront business payload for the active guest locale.
 * Switching back to the business default (usually `en`) must not reuse a
 * Spanish-seeded SSR object — wait for the matching `?language=` fetch.
 */
export function resolveStorefrontDisplayBusiness(opts: {
  seed: PublicBusiness;
  localized?: PublicBusiness | null;
  currentLanguage: string;
  seedLanguage: string;
}): PublicBusiness {
  if (opts.localized) return opts.localized;
  if (!opts.currentLanguage || opts.currentLanguage === opts.seedLanguage) {
    return opts.seed;
  }
  return {
    ...opts.seed,
    description: "",
    welcome_message: "",
    about_story: "",
    special_features: [],
    gallery_images: [],
  };
}
