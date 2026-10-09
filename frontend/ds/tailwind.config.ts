/**
 * Tailwind config for the design-sync bundle.
 *
 * Inherits the app's theme verbatim from ../tailwind.config.ts — the brand
 * palette, warm-neutral scale, type scale and shadow recipes are defined
 * there and must not be duplicated here. Only `content` and `safelist`
 * differ.
 *
 * Why the content globs are wide: claude.ai/design's agent composes NEW
 * layouts with Payverge's utility vocabulary, not just the classes the
 * exported components happen to use. Scanning the whole app captures the
 * vocabulary that is actually idiomatic here; the safelist then guarantees
 * every systematic variation of a design token resolves even if no file
 * currently uses it.
 *
 * Run from frontend/: npx tailwindcss -c ds/tailwind.config.ts -i ds/styles.src.css -o ds/styles.css
 */
import type { Config } from "tailwindcss";
import base from "../tailwind.config";

const config: Config = {
  ...base,
  content: [
    "./node_modules/@nextui-org/theme/dist/**/*.{js,ts,jsx,tsx}",
    "./src/**/*.{js,ts,jsx,tsx,mdx}",
    "./ds/**/*.{ts,tsx}",
  ],
  safelist: [
    // Every design-token color across every utility that takes one, so the
    // design agent can reach for `bg-brand-50` or `border-warm-200` whether
    // or not the app already uses that exact pair.
    {
      pattern:
        /^(bg|text|border|ring|divide|from|via|to|fill|stroke|outline|decoration|placeholder|accent|caret|shadow)-(brand|warm|ink|gray)(-(50|100|200|300|400|500|600|700|800|900|950|light|dark))?$/,
      variants: ["hover", "focus", "focus-visible", "active", "disabled", "group-hover", "sm", "md", "lg"],
    },
    // Named type scale (display / heading / body / label).
    {
      pattern:
        /^text-(display-(2xl|xl|lg|md)|heading-(lg|md|sm)|body(-lg|-sm)?|label)$/,
      variants: ["sm", "md", "lg"],
    },
    // Named elevation recipes.
    { pattern: /^shadow-(panel|card|panel-highlight|cta-glow)$/, variants: ["hover"] },
    // Font families bound in styles.src.css.
    { pattern: /^font-(sans|inter|poppins|title)$/ },
    // Brand motion.
    { pattern: /^animate-(subtle-bounce|subtle-bounce-reverse)$/ },
  ],
};

export default config;
