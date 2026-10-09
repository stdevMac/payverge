/**
 * Browser environment shim for the design-sync bundle.
 *
 * MUST be the first import in index.tsx: ES module initialization runs in
 * source order, so this file executes before any component module reads
 * `process.env` at module scope.
 *
 * Why it is needed: two exported components (TopMenuShell, ErrorBoundaryShell)
 * import `next/link` / `next/image`. In the app those modules' `process.env.*`
 * reads are replaced at `next build` time by webpack's DefinePlugin. The
 * design-sync bundle is plain esbuild output with no Next build step, so the
 * reads survive into the browser, where `process` does not exist — every
 * component in the bundle then failed with "ReferenceError: process is not
 * defined" before `window.Payverge` was ever assigned.
 *
 * This shims the environment, not the components. Nothing here changes what a
 * component renders; it supplies the values Next's own client runtime expects.
 *
 * Keep in sync with `grep -oE 'process\.env\.[A-Za-z_]+' ds-bundle/_ds_bundle.js`
 * after adding any component that pulls in more of the Next client runtime.
 */

const env: Record<string, unknown> = {
  NODE_ENV: "production",

  // Public config the app inlines at build time. Only used for link targets in
  // the bundle; no request is ever made from a design preview.
  NEXT_PUBLIC_API_URL: "https://api.example.com",

  // next/image: normally the resolved `images` config object. `unoptimized`
  // makes it emit a plain <img> with the original src — correct here, since
  // there is no Next image optimizer behind the design environment.
  __NEXT_IMAGE_OPTS: {
    deviceSizes: [640, 750, 828, 1080, 1200],
    imageSizes: [16, 32, 48, 64, 96, 128, 256, 384],
    path: "/_next/image",
    loader: "default",
    dangerouslyAllowSVG: false,
    unoptimized: true,
    domains: [],
    remotePatterns: [],
    qualities: [75],
  },

  // next/link + next/router feature flags. All false/empty: no basepath, no
  // i18n routing, no trailing-slash rewriting in the design environment.
  __NEXT_I18N_SUPPORT: false,
  __NEXT_ROUTER_BASEPATH: "",
  __NEXT_TRAILING_SLASH: false,
  __NEXT_MANUAL_TRAILING_SLASH: false,
  __NEXT_MANUAL_CLIENT_BASE_PATH: false,
  __NEXT_LINK_NO_TOUCH_START: false,
  NEXT_DEPLOYMENT_ID: "",

  // Undefined in the browser (set only on the server/edge runtimes).
  NEXT_RUNTIME: undefined,
};

const g = globalThis as unknown as { process?: { env?: Record<string, unknown> } };
g.process ??= { env: {} };
g.process.env ??= {};
for (const [k, v] of Object.entries(env)) {
  if (g.process.env[k] === undefined) g.process.env[k] = v;
}

export {};
