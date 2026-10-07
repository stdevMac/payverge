/**
 * Public URL aliases. next.config.mjs spreads this into `redirects()`.
 * CommonJS (.cjs) on purpose: a typeless .js file made Node reparse it as ESM
 * and print MODULE_TYPELESS_PACKAGE_JSON on every build.
 * Locale prefixes mirror the operator tier (en unprefixed, es, es-ar).
 */

const LOCALE_PREFIXES = ["", "/es", "/es-ar"];

// The sign-in dialog lives on /dashboard ("/" serves the venue). /dashboard
// has no locale-prefixed variant, so a prefixed alias carries its language
// as ?lang=, which the operator tier honours on /dashboard.
const PREFIX_LANG = { "/es": "es", "/es-ar": "es-AR" };

function signInDestination(prefix) {
  const lang = PREFIX_LANG[prefix];
  return lang
    ? `/dashboard?auth=signin&lang=${lang}`
    : "/dashboard?auth=signin";
}

function routeAliasRedirects() {
  const redirects = [];

  for (const prefix of LOCALE_PREFIXES) {
    redirects.push({
      source: `${prefix}/terms`,
      destination: `${prefix}/terms-and-conditions`,
      permanent: true,
    });
    redirects.push({
      source: `${prefix}/privacy`,
      destination: `${prefix}/privacy-policy`,
      permanent: true,
    });
    // Sign-in is a dialog, not a page, and owner sign-up is the register
    // wizard, so the usual spellings would otherwise 404.
    for (const alias of ["/login", "/signin", "/sign-in"]) {
      redirects.push({
        source: `${prefix}${alias}`,
        destination: signInDestination(prefix),
        permanent: false,
      });
    }
    for (const alias of ["/signup", "/sign-up"]) {
      redirects.push({
        source: `${prefix}${alias}`,
        destination: "/business/register",
        permanent: false,
      });
    }
    // /docs points at the docs/ tree of the public repository. Temporary,
    // not permanent: an install may later serve its own docs.
    redirects.push({
      source: `${prefix}/docs`,
      destination: "https://github.com/stdevMac/payverge/tree/main/docs",
      permanent: false,
    });
  }

  redirects.push(
    { source: "/en", destination: "/", permanent: true },
    { source: "/en/:path*", destination: "/:path*", permanent: true },
    {
      source: "/manifest.json",
      destination: "/site.webmanifest",
      permanent: true,
    },
    {
      source: "/manifest.webmanifest",
      destination: "/site.webmanifest",
      permanent: true,
    },
  );

  return redirects;
}

module.exports = { routeAliasRedirects };
