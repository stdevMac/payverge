"use client";
import Link from "next/link";
import Image from "next/image";
import InstanceLogo from "@/components/instance/InstanceLogo";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { localizedPublicHref } from "@/i18n/publicPageRoutes";
import { useClickTracking } from "@/hooks/useAnalytics";
import { useCookieConsent } from "@/contexts/CookieConsentContext";
import { useInstance } from "@/hooks/useInstance";
import { siteNameOf } from "@/lib/instance/instanceInfo";

export function Footer() {
  const { locale } = useSimpleLocale();
  const trackClick = useClickTracking();
  const { reset: resetCookieConsent, setOpener } = useCookieConsent();
  // The copyright holder is the operator running this site, not the software.
  const siteName = siteNameOf(useInstance().instance);
  const href = (route: string) => localizedPublicHref(locale, route);
  // /dashboard has no locale-prefixed variant; it reads the operator locale
  // from the cookie the prefixed page already set.
  const dashboardHref = "/dashboard";

  const tString = (key: string): string => {
    const result = getTranslation(key, locale);
    return Array.isArray(result) ? result.join(" ") : result;
  };

  return (
    <footer className="py-16 bg-ink-900 text-warm-300">
      <div className="container mx-auto px-6">
        <div className="max-w-6xl mx-auto">
          {/* Main Footer Content */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-12 mb-12">
            {/* Brand Column */}
            <div className="md:col-span-2">
              <div className="mb-6 relative w-40 h-10 md:w-48 md:h-12">
                <InstanceLogo
                  imgClassName="h-full w-full object-contain object-left"
                  fallback={(alt) => (
                    <Image
                      src="/images/PayvergeLogo.png"
                      alt={alt}
                      fill
                      sizes="(max-width: 768px) 160px, 192px"
                      className="object-contain object-left"
                    />
                  )}
                />
              </div>
            </div>

            {/* Links Column */}
            <div>
              <h3 className="text-white font-semibold mb-4 tracking-wide">
                {tString("footer.links")}
              </h3>
              <ul className="space-y-3">
                <li>
                  <Link
                    href={dashboardHref}
                    onClick={() =>
                      trackClick(
                        "footer-dashboard",
                        "navigation",
                        dashboardHref,
                      )
                    }
                    className="block py-2 text-warm-300 hover:text-white transition-colors duration-200 text-sm font-light rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-900"
                  >
                    {tString("footer.dashboard")}
                  </Link>
                </li>
                <li>
                  <Link
                    href={href("/staff/login")}
                    onClick={() =>
                      trackClick(
                        "footer-staff-login",
                        "navigation",
                        href("/staff/login"),
                      )
                    }
                    className="block py-2 text-warm-300 hover:text-white transition-colors duration-200 text-sm font-light rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-900"
                  >
                    {tString("footer.staffLogin")}
                  </Link>
                </li>
                <li>
                  <Link
                    href={href("/terms-and-conditions")}
                    onClick={() =>
                      trackClick(
                        "footer-terms",
                        "navigation",
                        href("/terms-and-conditions"),
                      )
                    }
                    className="block py-2 text-warm-300 hover:text-white transition-colors duration-200 text-sm font-light rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-900"
                  >
                    {tString("footer.termsAndConditions")}
                  </Link>
                </li>
                <li>
                  <Link
                    href={href("/refund")}
                    onClick={() =>
                      trackClick("footer-refund", "navigation", href("/refund"))
                    }
                    className="block py-2 text-warm-300 hover:text-white transition-colors duration-200 text-sm font-light rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-900"
                  >
                    {tString("footer.refundPolicy")}
                  </Link>
                </li>
                <li>
                  <Link
                    href={href("/privacy-policy")}
                    onClick={() =>
                      trackClick(
                        "footer-privacy",
                        "navigation",
                        href("/privacy-policy"),
                      )
                    }
                    className="block py-2 text-warm-300 hover:text-white transition-colors duration-200 text-sm font-light rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-900"
                  >
                    {tString("footer.privacyPolicy")}
                  </Link>
                </li>
                <li>
                  <button
                    type="button"
                    onMouseDown={(event) => {
                      // Keep the opener from winning the click-focus race
                      // against the remounted preferences dialog (#437).
                      event.preventDefault();
                    }}
                    onClick={(event) => {
                      trackClick(
                        "footer-cookie-preferences",
                        "navigation",
                        "cookie-preferences",
                      );
                      setOpener(event.currentTarget);
                      resetCookieConsent();
                    }}
                    className="block w-full text-left py-2 text-warm-300 hover:text-white transition-colors duration-200 text-sm font-light rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-900"
                  >
                    {tString("cookies.footer.preferences")}
                  </button>
                </li>
              </ul>
            </div>
          </div>

          {/* Bottom Bar */}
          <div className="pt-8 border-t border-ink-700">
            <div className="flex flex-col md:flex-row justify-between items-center gap-4">
              <p className="text-sm text-warm-400 font-light">
                © {new Date().getFullYear()} {siteName}.{" "}
                {tString("footer.allRightsReserved")}
              </p>
            </div>
          </div>
        </div>
      </div>
    </footer>
  );
}
