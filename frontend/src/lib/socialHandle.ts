/**
 * Canonical social-handle normalization.
 *
 * Merchants paste bare handles, @handles, host/path, or full URLs into
 * ContactEditor. Without a single normalizer, BusinessContactTab concatenates
 * `base + value` and produces `facebook.com/https://facebook.com/payverge`.
 *
 * Storage and display both use the bare handle (no scheme, host, leading @,
 * or trailing slash). Profile URLs are rebuilt from the network + handle.
 */

export type SocialNetwork =
  | "instagram"
  | "facebook"
  | "twitter"
  | "linkedin"
  | "youtube"
  | "tiktok";

export type NormalizeSocialHandleResult =
  | { ok: true; handle: string }
  | { ok: false; error: string };

type NetworkConfig = {
  /** Hostnames owned by this network (lowercase, no www.). */
  hosts: readonly string[];
  /** Path prefixes stripped after the host (e.g. company/, @). */
  pathPrefixes: readonly string[];
  /** Allowed characters for the remaining handle. */
  charset: RegExp;
  maxLen: number;
};

const NETWORKS: Record<SocialNetwork, NetworkConfig> = {
  instagram: {
    hosts: ["instagram.com"],
    pathPrefixes: [],
    charset: /^[A-Za-z0-9._]+$/,
    maxLen: 30,
  },
  facebook: {
    hosts: ["facebook.com", "fb.com", "fb.me", "m.facebook.com"],
    pathPrefixes: [],
    charset: /^[A-Za-z0-9.]+$/,
    maxLen: 50,
  },
  twitter: {
    hosts: ["twitter.com", "x.com", "mobile.twitter.com"],
    pathPrefixes: [],
    charset: /^[A-Za-z0-9_]+$/,
    maxLen: 15,
  },
  linkedin: {
    hosts: ["linkedin.com"],
    pathPrefixes: ["company/", "in/", "school/"],
    charset: /^[A-Za-z0-9-]+$/,
    maxLen: 100,
  },
  youtube: {
    hosts: ["youtube.com", "youtu.be", "m.youtube.com"],
    pathPrefixes: ["@", "c/", "channel/", "user/"],
    charset: /^[A-Za-z0-9._-]+$/,
    maxLen: 100,
  },
  tiktok: {
    hosts: ["tiktok.com", "vm.tiktok.com"],
    pathPrefixes: ["@"],
    charset: /^[A-Za-z0-9._]+$/,
    maxLen: 24,
  },
};

/** Flat map: hostname → owning network (for wrong-network detection). */
const HOST_OWNER: Map<string, SocialNetwork> = (() => {
  const map = new Map<string, SocialNetwork>();
  (Object.keys(NETWORKS) as SocialNetwork[]).forEach((network) => {
    for (const host of NETWORKS[network].hosts) {
      map.set(host, network);
    }
  });
  return map;
})();

const PROFILE_BASE: Record<SocialNetwork, string> = {
  instagram: "https://instagram.com/",
  facebook: "https://facebook.com/",
  twitter: "https://twitter.com/",
  linkedin: "https://linkedin.com/company/",
  youtube: "https://youtube.com/@",
  tiktok: "https://tiktok.com/@",
};

/**
 * Normalize a raw social input to a bare handle for the given network.
 *
 * Strips scheme, `www.`, known host, leading `@`, trailing slash, query, and
 * fragment. Validates the remaining handle against the network charset.
 * Returns a validation error when the value is a URL for a *different*
 * network, or when the remaining handle fails charset/length checks.
 */
export function normalize(
  network: SocialNetwork,
  raw: string,
): NormalizeSocialHandleResult {
  const trimmed = (raw ?? "").trim();
  if (!trimmed) {
    return { ok: true, handle: "" };
  }

  // Strip scheme.
  let rest = trimmed.replace(/^https?:\/\//i, "");
  // Strip www.
  rest = rest.replace(/^www\./i, "");

  // Detect a leading hostname (with optional path).
  const hostPath = rest.match(/^([^/?#]+)([/?#].*)?$/);
  if (hostPath) {
    const hostCandidate = hostPath[1].toLowerCase();
    const owner = HOST_OWNER.get(hostCandidate);
    if (owner) {
      if (owner !== network) {
        return {
          ok: false,
          error: `Wrong network: URL belongs to ${owner}, expected ${network}`,
        };
      }
      // Drop host; keep path/query/hash (path may be empty).
      rest = (hostPath[2] ?? "").replace(/^[/?#]/, "");
    }
  }

  // Strip query + fragment before path-prefix handling.
  rest = rest.split(/[?#]/)[0] ?? "";
  // Leading slash noise.
  rest = rest.replace(/^\/+/, "");

  // Network-specific path prefixes (company/, @, c/, …).
  for (const prefix of NETWORKS[network].pathPrefixes) {
    if (rest.toLowerCase().startsWith(prefix.toLowerCase())) {
      rest = rest.slice(prefix.length);
      break;
    }
  }

  // Leading @ (bare handles and residual after path strip).
  rest = rest.replace(/^@+/, "");
  // Trailing slash(es).
  rest = rest.replace(/\/+$/, "");
  // If multi-segment path remains (e.g. pages/Category/Name), take first
  // segment — vanity URLs are single-segment for all networks we support.
  if (rest.includes("/")) {
    rest = rest.split("/")[0] ?? "";
  }

  if (!rest) {
    return { ok: true, handle: "" };
  }

  const cfg = NETWORKS[network];
  if (rest.length > cfg.maxLen || !cfg.charset.test(rest)) {
    return {
      ok: false,
      error: `Invalid ${network} handle: characters or length not allowed`,
    };
  }

  return { ok: true, handle: rest };
}

/** Rebuild a profile URL from a bare handle (empty → ""). */
export function socialProfileUrl(
  network: SocialNetwork,
  handle: string,
): string {
  const h = (handle ?? "").trim().replace(/^@+/, "");
  if (!h) return "";
  return `${PROFILE_BASE[network]}${h}`;
}
