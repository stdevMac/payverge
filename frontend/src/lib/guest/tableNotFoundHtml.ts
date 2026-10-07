import { resolveErrorBoundaryCopyForLocale } from "@/i18n/errorBoundaryCopy";
import {
  getLocaleDirection,
  type StorefrontLocale,
} from "@/i18n/localeRegistry";

function escapeHtmlText(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function escapeHtmlAttribute(value: string): string {
  return escapeHtmlText(value).replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

/**
 * Hard 404 document for unknown /t/:code routes.
 * App Router notFound() under the root layout can stream HTTP 200 first.
 */
export function guestTableNotFoundHtml(
  locale: StorefrontLocale,
  title: string,
): string {
  const direction = getLocaleDirection(locale);
  const copy = resolveErrorBoundaryCopyForLocale(locale);
  const safeTitle = escapeHtmlText(title);
  const heading = escapeHtmlText(copy.tableNotFoundTitle);
  const body = escapeHtmlText(copy.tableNotFoundBody);
  return `<!DOCTYPE html>
<html lang="${escapeHtmlAttribute(locale)}" dir="${escapeHtmlAttribute(direction)}">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <meta name="robots" content="noindex, nofollow"/>
  <title>${safeTitle}</title>
  <script>window.__PAYVERGE_BUILD__=window.__PAYVERGE_BUILD__||{surface:"guest-table-404"};</script>
  <style>
    body{font-family:system-ui,sans-serif;margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#faf9f6;color:#1c1917}
    main{max-width:28rem;padding:2rem;text-align:center}
    h1{font-size:1.5rem;margin:0 0 .5rem}
    p{color:#57534e;margin:0 0 1.5rem}
    a{color:#1a6b6a;font-weight:600}
  </style>
</head>
<body>
  <main>
    <h1>${heading}</h1>
    <p>${body}</p>
    <p><a href="/">${escapeHtmlText(copy.goHome)}</a></p>
  </main>
</body>
</html>`;
}
