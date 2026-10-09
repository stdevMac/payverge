import { publicPathForLocale } from "@/i18n/metadata";
import { PUBLIC_PAGE_LOCALES } from "@/i18n/publicPageRoutes";

export type AssistantUrlKind = "blocked" | "internal" | "external";

const ASCII_CONTROL = /[\u0000-\u001f\u007f]/;

/** Public instance pages an assistant reply may link to without a prefix. */
const STATIC_PUBLIC_ROUTES = new Set([
  "/",
  "/dashboard",
  "/register",
  "/business/register",
  "/privacy-policy",
  "/refund",
  "/terms-and-conditions",
]);

/** Public pages that also exist under a locale prefix (`/es`, `/es-ar`). */
const PREFIXABLE_PUBLIC_ROUTES = ["/", "/business/register"] as const;

const LOCALIZED_STATIC_PUBLIC_ROUTES = new Set(
  PUBLIC_PAGE_LOCALES.flatMap((locale) =>
    PREFIXABLE_PUBLIC_ROUTES.map((route) => publicPathForLocale(locale, route)),
  ),
);

function hasUnsafeRawCharacters(value: string): boolean {
  if (
    value !== value.trim() ||
    ASCII_CONTROL.test(value) ||
    value.includes("\\")
  ) {
    return true;
  }

  return false;
}

function decodeUrlPart(value: string): string | null {
  try {
    return decodeURIComponent(value);
  } catch {
    return null;
  }
}

function hasUnsafeDecodedCharacters(value: string): boolean {
  return ASCII_CONTROL.test(value) || value.includes("\\");
}

function isSafeCanonicalPath(pathname: string): boolean {
  if (hasUnsafeDecodedCharacters(pathname) || pathname.startsWith("//")) {
    return false;
  }

  return !pathname
    .split("/")
    .some((segment) => segment === "." || segment === "..");
}

function isRegisteredPublicPath(pathname: string): boolean {
  return (
    STATIC_PUBLIC_ROUTES.has(pathname) ||
    LOCALIZED_STATIC_PUBLIC_ROUTES.has(pathname)
  );
}

function isAllowedInternalUrl(value: string): boolean {
  if (!value.startsWith("/") || value.startsWith("//")) return false;

  const suffixIndex = value.search(/[?#]/);
  const rawPathname = suffixIndex === -1 ? value : value.slice(0, suffixIndex);
  const pathname = decodeUrlPart(rawPathname);
  if (!pathname) return false;

  if (suffixIndex !== -1) {
    const decodedSuffix = decodeUrlPart(value.slice(suffixIndex));
    if (decodedSuffix === null || hasUnsafeDecodedCharacters(decodedSuffix)) {
      return false;
    }
  }

  return isSafeCanonicalPath(pathname) && isRegisteredPublicPath(pathname);
}

function isAllowedExternalUrl(value: string): boolean {
  if (!/^https?:\/\//i.test(value)) return false;

  const authority = value.slice(value.indexOf("//") + 2).split(/[/?#]/, 1)[0];
  if (!/^[\u0021-\u007e]+$/.test(authority) || authority.includes("%")) {
    return false;
  }

  const decoded = decodeUrlPart(value);
  if (decoded === null || hasUnsafeDecodedCharacters(decoded)) return false;

  try {
    const parsed = new URL(value);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.host.length > 0 &&
      parsed.username.length === 0 &&
      parsed.password.length === 0
    );
  } catch {
    return false;
  }
}

export function classifyAssistantHref(
  value: string | undefined,
): AssistantUrlKind {
  if (!value || hasUnsafeRawCharacters(value) || value.startsWith("//")) {
    return "blocked";
  }

  if (isAllowedInternalUrl(value)) return "internal";
  if (isAllowedExternalUrl(value)) return "external";
  return "blocked";
}

export function assistantUrlTransform(value: string): string {
  return classifyAssistantHref(value) === "blocked" ? "" : value;
}
