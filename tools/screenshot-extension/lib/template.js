// Parse Payverge route context out of a full page URL. Only keys that are
// present in the URL are returned, so callers can spread over defaults.
export function extractContext(url) {
  const ctx = {};
  try {
    ctx.origin = new URL(url).origin;
  } catch {
    // Non-URL input: leave origin undefined; callers fall back to the tab URL.
  }
  const business = url.match(/\/business\/([^/?#]+)/);
  if (business) ctx.businessId = business[1];
  const table = url.match(/\/t\/([^/?#]+)/);
  if (table) ctx.tableCode = table[1];
  const storefront = url.match(/\/b\/([^/?#]+)/);
  if (storefront) ctx.customUrl = storefront[1];
  return ctx;
}

// Replace {token} placeholders in a path with values from context. Throws a
// named error if a placeholder has no matching (truthy) context value so a
// misconfigured shot fails loudly instead of navigating to a broken URL.
export function fillUrl(templatePath, context) {
  return templatePath.replace(/\{(\w+)\}/g, (_, key) => {
    const value = context[key];
    if (value === undefined || value === null || value === "") {
      throw new Error(`missing context: ${key}`);
    }
    return String(value);
  });
}
