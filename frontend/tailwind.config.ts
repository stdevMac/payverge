import type { Config } from "tailwindcss";
import { nextui } from "@nextui-org/react";

// SINGLE SOURCE OF TRUTH for the warm-neutral text/surface palette.
// `warm`, `ink`, and `gray` (Tailwind's stock cool-gray, overridden) all
// resolve to these hex values, so changing a shade here cascades to every
// `text-warm-*`, `text-ink-*`, `text-gray-*`, `bg-*`, and `border-*` use
// across the app. Keep 50–300 as backgrounds/borders and 400–950 as text.
// Contrast on cream (#faf9f6): 400=3.9:1 (AA-large), 500=5.6:1 (AA),
// 600=7.9:1 (AAA), 700+=10:1.
const neutralScale = {
  50:  '#faf9f6',
  100: '#f3f1ec',
  200: '#e4e0d8',
  300: '#cfc9bd',
  400: '#857d6e',
  500: '#6b6358',
  600: '#544d44',
  700: '#403b34',
  800: '#2e2a25',
  900: '#1c1917',
  950: '#1c1917',
} as const;

const config: Config = {
  content: [
    "./node_modules/@nextui-org/theme/dist/**/*.{js,ts,jsx,tsx}",
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        // `--font-inter` and `--font-title` are CSS variables set in globals.css.
        // Keep the legacy variable names because the app-wide Tailwind tokens
        // already reference them.
        sans: ['var(--font-inter)', 'DM Sans', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        inter: ['var(--font-inter)', 'DM Sans', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        poppins: ['var(--font-poppins)', 'DM Sans', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        title: ['var(--font-title)', 'DM Serif Display', 'Georgia', 'serif'],
      },
      backgroundImage: {
        "gradient-radial": "radial-gradient(var(--tw-gradient-stops))",
        "gradient-conic":
          "conic-gradient(from 180deg at 50% 50%, var(--tw-gradient-stops))",
      },
      // Named recipes for the arbitrary box-shadow values that recurred as
      // exact duplicates across multiple files. Each value below is
      // byte-identical to the arbitrary `shadow-[...]` string it replaces
      // (underscores in the utility class decode to the spaces here), so
      // `shadow-<token>` compiles to the exact same CSS — no visual change.
      boxShadow: {
        // Deep warm panel elevation — was shadow-[0_24px_80px_rgba(46,42,37,0.12)]
        panel: '0 24px 80px rgba(46,42,37,0.12)',
        // Hairline card seat — was shadow-[0_1px_2px_rgba(46,42,37,0.05)]
        card: '0 1px 2px rgba(46,42,37,0.05)',
        // Inset top highlight for premium surfaces — was shadow-[inset_0_1px_0_rgba(255,255,255,0.72)]
        'panel-highlight': 'inset 0 1px 0 rgba(255,255,255,0.72)',
        // Brand teal CTA glow — was shadow-[0_12px_24px_rgba(26,107,106,0.16)]
        'cta-glow': '0 12px 24px rgba(26,107,106,0.16)',
      },
      colors: {
        // All three neutral aliases share `neutralScale` so any future shade
        // tweak happens in ONE place at the top of this file. `gray` overrides
        // Tailwind's stock cool-gray with our warm scale — the brand is
        // warm-toned and cool gray clashed with the cream canvas.
        warm: neutralScale,
        gray: neutralScale,
        ink: { DEFAULT: neutralScale[950], ...neutralScale },
        brand: {
          // Runtime brand colour (BRAND_COLOR via GET /api/v1/instance): the
          // root layout sets --pv-brand-rgb on <html> when an install
          // overrides it; the fallback is the upstream teal (#1a6b6a). Only
          // DEFAULT and 600 follow it; the other shades stay teal.
          DEFAULT: 'rgb(var(--pv-brand-rgb, 26 107 106) / <alpha-value>)',
          light:   '#2a8b8a',
          dark:    '#145554',
          50:  '#f0f7f7',
          100: '#d9ebea',
          200: '#b3d6d5',
          300: '#84bcbb',
          400: '#4f9e9d',
          500: '#2a8b8a',
          600: 'rgb(var(--pv-brand-rgb, 26 107 106) / <alpha-value>)',
          700: '#145554',
          800: '#0f4342',
          900: '#0a3231',
          950: '#041e1d',
        },
      },
      fontSize: {
        'display-2xl': ['4.5rem',  { lineHeight: '1.05', letterSpacing: '-0.03em', fontWeight: '400' }],
        'display-xl':  ['3.5rem',  { lineHeight: '1.08', letterSpacing: '-0.03em', fontWeight: '400' }],
        'display-lg':  ['2.5rem',  { lineHeight: '1.1',  letterSpacing: '-0.02em', fontWeight: '400' }],
        'display-md':  ['2rem',    { lineHeight: '1.15', letterSpacing: '-0.02em', fontWeight: '500' }],
        'heading-lg':  ['1.5rem',  { lineHeight: '1.25', letterSpacing: '-0.015em', fontWeight: '600' }],
        'heading-md':  ['1.25rem', { lineHeight: '1.3',  letterSpacing: '-0.01em',  fontWeight: '600' }],
        'heading-sm':  ['1.0625rem',{ lineHeight: '1.35', letterSpacing: '-0.005em',fontWeight: '600' }],
        'body-lg':     ['1.125rem',{ lineHeight: '1.6',  fontWeight: '400' }],
        'body':        ['1rem',    { lineHeight: '1.55', fontWeight: '400' }],
        'body-sm':     ['0.875rem',{ lineHeight: '1.5',  fontWeight: '400' }],
        'label':       ['0.75rem', { lineHeight: '1.3',  letterSpacing: '0.08em',  fontWeight: '600' }],
      },
      keyframes: {
        'subtle-bounce': {
          '0%, 100%': { transform: 'translateY(0)' },
          '50%': { transform: 'translateY(-1px)' },
        },
        'subtle-bounce-reverse': {
          '0%, 100%': { transform: 'translateY(0)' },
          '50%': { transform: 'translateY(1px)' },
        },
      },
      animation: {
        'subtle-bounce': 'subtle-bounce 2s ease-in-out infinite',
        'subtle-bounce-reverse': 'subtle-bounce-reverse 2s ease-in-out infinite',
      },
    },
    scrollbar: ["rounded"],
  },
  darkMode: "class",
  plugins: [
    nextui({
      themes: {
        light: {
          colors: {
            primary: {
              DEFAULT: '#1a6b6a',
              foreground: '#ffffff',
              50:  '#f0f7f7',
              100: '#d9ebea',
              200: '#b3d6d5',
              300: '#84bcbb',
              400: '#4f9e9d',
              500: '#2a8b8a',
              600: '#1a6b6a',
              700: '#145554',
              800: '#0f4342',
              900: '#0a3231',
            },
          },
        },
        dark: {
          colors: {
            primary: {
              DEFAULT: '#4f9e9d',
              foreground: '#0e1414',
              50:  '#0a3231',
              100: '#0f4342',
              200: '#145554',
              300: '#1a6b6a',
              400: '#2a8b8a',
              500: '#4f9e9d',
              600: '#84bcbb',
              700: '#b3d6d5',
              800: '#d9ebea',
              900: '#f0f7f7',
            },
          },
        },
      },
    }),
    // Root B (L1-12 / L3-13): NextUI ModalContent's fixed inset-0
    // `[data-slot="wrapper"]` has no pointer-events-none and animates
    // opacity 0→1 via scaleInOut (~0.4s enter). Theme `themes` cannot
    // override modal slots, so enforce the contract here for all ~101
    // <Modal> call sites: wrapper is click-through; dialog panel (child)
    // remains interactive; opacity forced opaque so panels never ghost.
    //
    // INVARIANT — the click-through rule MUST stay scoped with :has() to
    // dismissable modals only. In @nextui-org/modal 2.2.7 the backdrop (a
    // SIBLING of the wrapper) carries an unconditional
    // `onClick: () => state.close()` (getBackdropProps); it is only
    // unreachable for `isDismissable={false}` modals because the wrapper
    // covers it. getDialogProps stamps the dialog <section> (the wrapper's
    // direct child) with `data-dismissable="true"` when dismissable, and
    // OMITS the attribute entirely when `isDismissable={false}`
    // (dataAttr → undefined) — so the :has() below matches exactly the
    // wrappers that are safe to make click-through. Un-scoping this rule
    // would let backdrop clicks close the 11 non-dismissable in-flight-money
    // modals (ManagerPinModal, RenewalModal, FiscalDashboard, …).
    // `aria-modal="true"` is always stamped on the dialog, which keeps every
    // selector off NextUI Pagination — the only other component that emits
    // `data-slot="wrapper"`.
    // Note: a `scrollBehavior="outside"` modal would lose wrapper scrollbar
    // dragging under pointer-events:none; all 34 current usages are "inside".
    function payvergeModalOverlayRootB({
      addBase,
    }: {
      addBase: (styles: Record<string, Record<string, string>>) => void;
    }) {
      addBase({
        '[data-slot="wrapper"]:has(> [aria-modal="true"][data-dismissable="true"])':
          {
            pointerEvents: "none",
          },
        '[data-slot="wrapper"]:has(> [aria-modal="true"][data-dismissable="true"]) > *':
          {
            pointerEvents: "auto",
          },
        // Ghosting fix applies to every modal wrapper, dismissable or not.
        '[data-slot="wrapper"]:has(> [aria-modal="true"])': {
          opacity: "1 !important",
        },
      });
    },
    require("tailwind-scrollbar"),
    // NOTE: do NOT add @tailwindcss/aspect-ratio here. That plugin is
    // deprecated and DISABLES Tailwind's native aspect-* utilities
    // (aspect-square, aspect-[5/4], …), silently collapsing any element
    // that relies on them to height:0 — e.g. the menu item-detail modal's
    // image carousel and image uploaders. Native aspect-ratio (TW 3.0+)
    // is all we use; keep the plugin out.
  ],
};
export default config;
