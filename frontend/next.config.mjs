import { withSentryConfig } from "@sentry/nextjs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { readFileSync } from "node:fs";
import aliasRedirects from "./src/config/routeAliasRedirects.cjs";

const { routeAliasRedirects } = aliasRedirects;

const imageOrigins = JSON.parse(
  readFileSync(new URL("./src/config/imageOrigins.json", import.meta.url), "utf8"),
);
for (const origin of imageOrigins) {
  if (origin.protocol !== "https" || !origin.hostname || !origin.pathname) {
    throw new Error(`invalid remote image origin: ${JSON.stringify(origin)}`);
  }
}

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Public URLs (PUBLIC_URL, API_URL, RPC_URL, ...) are resolved at RUNTIME by
// src/config/publicConfig.ts and validated at server start (src/instrumentation.ts),
// so one image serves any origin. Nothing deployment-specific is validated or
// inlined here. See docs/self-hosting/frontend-config.md.

/** @type {import('next').NextConfig} */
const useStandaloneOutput =
  process.env.NEXT_OUTPUT_STANDALONE === "1" || process.env.CI === "true";
// Builds are strict by default. Set NEXT_STRICT_BUILD=0 to relax for local WIP.
const relaxedBuild = process.env.NEXT_STRICT_BUILD === "0";
const optimizeCss = process.env.NEXT_OPTIMIZE_CSS === "1";
// Production source maps add ~10–20% to JS bundle size. Default off; flip on
// for an explicit build (NEXT_PROD_SOURCEMAPS=1) when DevTools-level
// debugability of a deployed bundle is needed.
const emitProdSourceMaps = process.env.NEXT_PROD_SOURCEMAPS === "1";

// CSP is now set dynamically per-request in middleware.ts with a nonce.
// The connect-src origins helper moved there as well.

const nextConfig = {
  pageExtensions: ['ts', 'tsx', 'js', 'jsx'],
  // Strip the `x-powered-by: Next.js` response header — it leaks the
  // framework/version and gives an attacker free reconnaissance.
  poweredByHeader: false,
  ...(useStandaloneOutput ? { output: "standalone" } : {}),
  // Off by default — bundles ~10–20% smaller. Set NEXT_PROD_SOURCEMAPS=1 for
  // a build when DevTools-level debugability is required.
  productionBrowserSourceMaps: emitProdSourceMaps,
  experimental: {
    webpackBuildWorker: true,
    // Tree-shake barrel exports for libraries that PageSpeed flagged as
    // duplicated/oversized chunks. Without this, importing a single
    // component (e.g. `import { Button } from "@nextui-org/react"`) pulls
    // the entire package into every chunk that touches it.
    optimizePackageImports: [
      "@nextui-org/react",
      "@nextui-org/theme",
      "@nextui-org/button",
      "@nextui-org/select",
      "@nextui-org/accordion",
      "lucide-react",
      "react-icons",
      "framer-motion",
      "lodash",
      "date-fns",
    ],
    // Inline critical CSS only when explicitly requested. Critters' selector
    // parser emits build warnings for valid web-component/Tailwind selectors
    // used in the app, so normal local builds keep this off to stay
    // warning-clean.
    optimizeCss,
    // Preserve CSS import order strictly. Default ("loose") merges sibling
    // CSS chunks aggressively, which produced a single ~42 KiB chunk
    // covering every route; strict mode keeps per-route CSS distinct so
    // public pages don't pay for admin/dashboard utility classes.
    cssChunking: "strict",
  },
  compiler: {
    removeConsole: {
      exclude: ['error', 'warn'],
    },
  },
  typescript: {
    ignoreBuildErrors: relaxedBuild,
  },
  eslint: {
    ignoreDuringBuilds: relaxedBuild,
  },
  trailingSlash: false,
  async redirects() {
    return [
      { source: '/register', destination: '/business/register', permanent: false },
      ...routeAliasRedirects(),
    ];
  },
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: [
          // Content-Security-Policy is now set dynamically in middleware.ts
          // with a per-request nonce for script-src. See src/middleware.ts.
          {
            key: "X-Frame-Options",
            value: "DENY",
          },
          {
            key: "X-Content-Type-Options",
            value: "nosniff",
          },
          {
            key: "Referrer-Policy",
            value: "strict-origin-when-cross-origin",
          },
          {
            key: "Permissions-Policy",
            value: "camera=(), microphone=(), geolocation=()",
          },
          // HTTP Strict Transport Security — also set in Caddy, but this
          // belt-and-suspenders covers non-Caddy paths (local prod, preview
          // deploys hitting Next directly). 1-year max-age + includeSubDomains.
          // Do not add `preload` until the production domain is accepted on hstspreload.org.
          {
            key: "Strict-Transport-Security",
            value: "max-age=31536000; includeSubDomains",
          },
          // Cross-Origin-Opener-Policy — origin isolation. `same-origin-allow-popups`
          // keeps OAuth popup flows working (Google sign-in opens a popup that
          // must remain reachable via window.opener); pure `same-origin` would
          // sever that link and break the OAuth callback bridge.
          {
            key: "Cross-Origin-Opener-Policy",
            value: "same-origin-allow-popups",
          },
        ],
      },
    ];
  },
  images: {
    // AVIF first, WebP fallback. Cuts ~25–40% off PNG/JPEG bytes for venue
    // hero and menu images served through the Next/Image pipeline.
    formats: ["image/avif", "image/webp"],
    qualities: [50, 75, 100],
    // Cap device widths at 1200. Larger fallback `src` values (1920/2048/3840)
    // produced optimizer decode failures / grey placeholders on feature assets
    // and the footer logo (#51/#58/#666).
    deviceSizes: [640, 750, 828, 1080, 1200],
    imageSizes: [16, 32, 48, 64, 96, 128, 256, 384],
    // 1-year cache for transformed /_next/image responses. Combined with the
    // immutable URL hash Next ships, repeat visitors skip the network round
    // trip entirely and PageSpeed stops flagging cache TTL on logos.
    minimumCacheTTL: 31536000,
    // `localPatterns` is deliberately unset: every same-origin path is
    // optimizable, including uploads proxied from the backend at /media/**.
    // `remotePatterns` is BUILD-TIME (Next freezes it into the image): only
    // the generic hosts in src/config/imageOrigins.json. A deployment that
    // serves uploads from its own S3/CDN host adds it to MEDIA_ORIGINS at
    // runtime (CSP img-src); such images render unoptimized unless the host
    // is also added to the manifest and the image rebuilt.
    // See docs/self-hosting/frontend-config.md.
    remotePatterns: imageOrigins,
  },
  webpack: (config, { isServer, webpack }) => {
    // @sentry/node (pulled in by @sentry/nextjs on the server) depends on
    // @prisma/instrumentation → @opentelemetry/instrumentation, whose
    // auto-instrumentation uses a dynamic require() that webpack cannot
    // statically resolve. That surfaces as "Critical dependency: the request of
    // a dependency is an expression". It is a known, benign false positive — the
    // path is server-only and resolves correctly at runtime — so we suppress it
    // here to keep builds warning-clean. (Sentry's own docs recommend this.)
    config.ignoreWarnings = [
      ...(config.ignoreWarnings || []),
      { module: /@opentelemetry\/instrumentation/ },
      { module: /@prisma\/instrumentation/ },
    ];

    if (isServer) {
      config.externals = config.externals || [];
      config.externals.push("pino-pretty", "lokijs", "encoding");
    } else {
      // Drop Next's hard-coded legacy polyfill chunk on the client. Next 14
      // bundles polyfill-module unconditionally (~11 KiB of Array.at/flat/
      // flatMap/Object.fromEntries/Object.hasOwn/String.trim* shims) by
      // adding it as a webpack entry, so a plain alias doesn't catch it.
      // Our supported floor is iOS 14 + evergreen Chrome/Firefox/Safari/
      // Edge 90+, all of which ship these natively. Strip the polyfill
      // file from every entry that references it.
      const originalEntry = config.entry;
      config.entry = async () => {
        const entries =
          typeof originalEntry === "function" ? await originalEntry() : originalEntry;
        // Drop the dedicated polyfills entry — that chunk is loaded with
        // `nomodule` so modern browsers skip it, but we don't ship to any
        // non-module browser anyway.
        delete entries["polyfills"];
        return entries;
      };

      // Next 14 also imports polyfill-module.js into the main client chunk
      // (visible in PageSpeed as ~11 KiB of Array.at/flat/flatMap/Object.
      // fromEntries/Object.hasOwn/String.trim* shims sitting inside the
      // shared 2117-*.js bundle). Replace it with an empty module so the
      // shims don't ride along on the LCP critical path.
      config.plugins = config.plugins || [];
      config.plugins.push(
        new webpack.NormalModuleReplacementPlugin(
          /[\\/]next[\\/]dist[\\/](esm[\\/])?build[\\/]polyfills[\\/]polyfill-(module|nomodule)\.js$/,
          path.resolve(__dirname, "src/empty-polyfill.js"),
        ),
      );

      // PageSpeed flagged framer-motion (~20 KiB) and @nextui-org/theme
      // duplicated across 3+ chunks because each dynamic-imported homepage
      // section imports them independently. Hoist them into dedicated
      // shared chunks so the bytes ship exactly once.
      config.optimization = config.optimization || {};
      const existing = config.optimization.splitChunks;
      const baseSplitChunks =
        existing && typeof existing === "object" ? existing : { chunks: "all" };
      const baseCacheGroups = (baseSplitChunks && baseSplitChunks.cacheGroups) || {};
      config.optimization.splitChunks = {
        ...baseSplitChunks,
        cacheGroups: {
          ...baseCacheGroups,
          framerMotion: {
            test: /[\\/]node_modules[\\/]framer-motion[\\/]/,
            name: "framer-motion",
            chunks: "all",
            priority: 30,
            reuseExistingChunk: true,
          },
          nextuiTheme: {
            test: /[\\/]node_modules[\\/]@nextui-org[\\/](theme|system|aria-utils)[\\/]/,
            name: "nextui-theme",
            chunks: "all",
            priority: 25,
            reuseExistingChunk: true,
          },
        },
      };
    }
    return config;
  },
};

const sentryUploadOrg = (process.env.SENTRY_ORG || "").trim();
const sentryAuthToken = (process.env.SENTRY_AUTH_TOKEN || "").trim();

const sentryBuildOptions = {
  silent: !process.env.CI,
  // Route Sentry ingestion through the app origin to dodge ad-blockers/CSP.
  tunnelRoute: "/monitoring",
  widenClientFileUpload: true,
  disableLogger: true,
  // Upload source maps for readable stack traces, then delete them from the
  // build output so they're never shipped in the public bundle.
  sourcemaps: { deleteSourcemapsAfterUpload: true },
  org: process.env.SENTRY_ORG || "",
  project: process.env.SENTRY_FRONTEND_PROJECT || "payverge-frontend",
  // authToken is only threaded when BOTH the org and the token are present.
  // withSentryConfig attempts a source-map upload whenever authToken is set;
  // with an empty org that upload fails and can break the build. Requiring a
  // non-empty org keeps the upload a silent no-op until both are configured.
  ...(sentryUploadOrg && sentryAuthToken
    ? { authToken: sentryAuthToken }
    : {}),
};

// The Sentry build plugin (tunnel route, release injection, source-map
// upload) is opt-in at BUILD time. The browser/server SDK itself is configured
// at runtime (FRONTEND_SENTRY_DSN via src/config/publicConfig.ts), so a
// self-hosted image without the plugin can still report to any DSN directly
// (the CSP adds the DSN host to connect-src). NEXT_PUBLIC_SENTRY_DSN at build
// time keeps the legacy behaviour.
const enableSentryBuildPlugin =
  process.env.SENTRY_BUILD_PLUGIN === "1" ||
  // eslint-disable-next-line no-restricted-syntax -- build-time switch read by next build, never shipped to the browser
  Boolean((process.env.NEXT_PUBLIC_SENTRY_DSN || "").trim());

export default enableSentryBuildPlugin
  ? withSentryConfig(nextConfig, sentryBuildOptions)
  : nextConfig;
