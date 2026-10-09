// Dotted operator-catalog paths that root-layout chrome needs on EVERY route,
// diner routes included: skip link, maintenance screen, 404 body, PWA install
// surface, auth gate, toasts, modal close label, session-expired modal and the
// verify-email banner. Kept in its own dependency-free module so the slim
// catalog loader and its parity test share one list.

export const OPERATOR_CHROME_PATHS: readonly string[] = [
  "common.maintenance",
  "common.skipToMainContent",
  "common.loadingSession",
  "common.sessionCheckFailed",
  "common.sessionCheckFailedHint",
  "common.retry",
  "common.dismissToast",
  "common.close",
  "notFound",
  "pwa",
  "authModal.sessionExpired",
  "authModal.verifyBannerResent",
  "authModal.verifyBannerResendFailed",
  "authModal.verifyBannerText",
  "authModal.verifyBannerResend",
  "authModal.verifyBannerDismiss",
];

type Tree = Record<string, unknown>;

/** Deep-pick dotted paths from a catalog tree; missing paths are omitted. */
export function pickOperatorChromeTree(
  tree: Tree,
  paths: readonly string[] = OPERATOR_CHROME_PATHS,
): Tree {
  const out: Tree = {};
  for (const path of paths) {
    const parts = path.split(".");
    let src: unknown = tree;
    for (const part of parts) {
      src =
        src && typeof src === "object"
          ? (src as Tree)[part]
          : undefined;
    }
    if (src === undefined) continue;
    let dst = out;
    for (const part of parts.slice(0, -1)) {
      if (!dst[part] || typeof dst[part] !== "object") dst[part] = {};
      dst = dst[part] as Tree;
    }
    dst[parts[parts.length - 1]] = src;
  }
  return out;
}
