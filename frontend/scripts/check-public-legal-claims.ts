/** Fail-closed scan for unsupported public claims in every locale JSON catalog. */
import * as fs from "node:fs";
import * as path from "node:path";

export const REQUIRED_CLAIM_CLASSES = [
  "fiat_conversion",
  "setup_time",
  "monitoring",
  "security",
  "support",
  "uptime",
  "tax",
  "fiscal",
  "pci",
  "compliance",
] as const;

export type ClaimClass = (typeof REQUIRED_CLAIM_CLASSES)[number];

export interface ClaimFinding {
  claimClass: ClaimClass;
  file: string;
  locale: string;
  jsonPath: string;
  text: string;
}

export interface ClaimScanResult {
  claimClassesChecked: readonly ClaimClass[];
  filesScanned: number;
  localesScanned: string[];
  findings: ClaimFinding[];
}

interface ClaimRule {
  claimClass: ClaimClass;
  patterns: RegExp[];
}

// These expressions target promises that need evidence or counsel review. They
// deliberately avoid generic words such as "security", "support", and "tax",
// which also appear in factual UI labels.
const RULES: ClaimRule[] = [
  {
    claimClass: "fiat_conversion",
    patterns: [
      /(?:auto(?:matic(?:ally)?)?|instant(?:ly)?)\s+(?:convert|converts|converted)\b.{0,80}\bfiat\b/i,
      /\bfiat\b.{0,80}\b(?:instant(?:ly)?|automatic(?:ally)?)\b/i,
      /(?:convierte|conversi[oó]n).{0,80}\bfiat\b.{0,40}(?:instant[aá]ne|autom[aá]tic)/i,
    ],
  },
  {
    claimClass: "setup_time",
    patterns: [
      /\b(?:setup|set up|onboard(?:ing)?)\b.{0,40}\b(?:under|in|within)\s+\d+\s*(?:minutes?|hours?)\b/i,
      /\b(?:instant|one-click|quick)\s+(?:setup|onboarding)\b/i,
      /\b(?:configuraci[oó]n|puesta en marcha)\b.{0,40}\b(?:en|menos de)\s+\d+\s*(?:minutos?|horas?)\b/i,
      /\bconfiguraci[oó]n\s+(?:instant[aá]nea|r[aá]pida)\b/i,
    ],
  },
  {
    claimClass: "monitoring",
    patterns: [
      /\b(?:24\s*[\/-]\s*7|24x7|round-the-clock|continuous)\b.{0,50}\bmonitor(?:ing|ed)?\b/i,
      /\bmonitoreo\b.{0,50}\b(?:24\s*[\/-]\s*7|continuo|permanente)\b/i,
    ],
  },
  {
    claimClass: "security",
    patterns: [
      /\b(?:100%|completely|totally)\s+secure\b/i,
      /\b(?:bank-grade|military-grade|unhackable)\b/i,
      /\bregular\b.{0,30}\b(?:penetration tests?|security audits?)\b/i,
      /\b(?:seguridad bancaria|imposible de hackear|100% segur[oa])\b/i,
      /\b(?:pruebas de penetraci[oó]n|auditor[ií]as de seguridad)\b.{0,30}\b(?:regulares|peri[oó]dicas)\b/i,
    ],
  },
  {
    claimClass: "support",
    patterns: [
      /\b(?:24\s*[\/-]\s*7|24x7|round-the-clock)\b.{0,40}\b(?:customer )?support\b/i,
      /\b(?:instant|guaranteed)\s+(?:support|response)\b/i,
      /\bsoporte\b.{0,40}\b(?:24\s*[\/-]\s*7|instant[aá]neo|garantizado)\b/i,
    ],
  },
  {
    claimClass: "uptime",
    patterns: [
      /\b(?:guaranteed\s+)?\d{2,3}(?:\.\d+)?%\s+uptime\b/i,
      /\b(?:guaranteed uptime|always online|zero downtime)\b/i,
      /\b(?:disponibilidad|uptime)\b.{0,30}\b(?:garantizada|\d{2,3}(?:[.,]\d+)?%)\b/i,
    ],
  },
  {
    claimClass: "tax",
    patterns: [
      /\b(?:fully|automatically|always)\b.{0,30}\b(?:tax compliant|files? (?:all )?taxes)\b/i,
      /\bcomplies with all\b.{0,30}\btax (?:laws?|regulations?)\b/i,
      /\b(?:cumple con todas|totalmente conforme)\b.{0,40}\b(?:leyes?|normas?) (?:tributarias?|impositivas?)\b/i,
    ],
  },
  {
    claimClass: "fiscal",
    patterns: [
      /\b(?:fully|automatically|always)\b.{0,30}\b(?:fiscal(?:ly)? compliant|fiscal compliance)\b/i,
      /\b(?:government|tax authority) approved\b/i,
      /\b(?:cumplimiento fiscal|fiscalmente conforme)\b.{0,30}\b(?:total|autom[aá]tico|garantizado)\b/i,
      /\baprobado por (?:el gobierno|la autoridad fiscal)\b/i,
    ],
  },
  {
    claimClass: "pci",
    patterns: [
      /\bPayverge\b.{0,35}\b(?:is|es|est[aá])\b.{0,20}\bPCI(?:[- ]DSS)?\b.{0,20}\b(?:certified|compliant|certificado|cumple)\b/i,
      /\bour payment processing is PCI(?:[- ]DSS)? compliant\b/i,
      /\bnuestro procesamiento de pagos cumple con PCI(?:[- ]DSS)?\b/i,
    ],
  },
  {
    claimClass: "compliance",
    patterns: [
      /\b(?:GDPR|RGPD|PDPL|CCPA|SOC\s*2|ISO\s*27001)\s+(?:certified|compliant)\b/i,
      /\b(?:fully compliant|complies with all applicable laws)\b/i,
      /\b(?:cumple|cumplimos)\b.{0,25}\b(?:GDPR|RGPD|PDPL|CCPA|todas? las leyes aplicables)\b/i,
    ],
  },
];

function listJsonFiles(root: string): string[] {
  if (!fs.existsSync(root)) return [];
  const out: string[] = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const full = path.join(root, entry.name);
    if (entry.isDirectory()) out.push(...listJsonFiles(full));
    else if (entry.isFile() && entry.name.endsWith(".json")) out.push(full);
  }
  return out.sort();
}

function localeForFile(file: string): string | null {
  const segments = file.split(path.sep);
  const messagesIndex = segments.lastIndexOf("messages");
  if (messagesIndex >= 0 && messagesIndex + 1 < segments.length) {
    const locale = segments[messagesIndex + 1];
    return locale === "__tests__" || locale.startsWith(".") ? null : locale;
  }
  const guestIndex = segments.lastIndexOf("guest-messages");
  if (guestIndex >= 0) {
    const locale = path.basename(file, ".json");
    return locale.startsWith(".") ? null : locale;
  }
  return null;
}

function flattenStrings(
  value: unknown,
  jsonPath = "",
  out: Array<{ jsonPath: string; text: string }> = [],
): Array<{ jsonPath: string; text: string }> {
  if (typeof value === "string") {
    out.push({ jsonPath, text: value });
  } else if (Array.isArray(value)) {
    value.forEach((item, index) =>
      flattenStrings(item, `${jsonPath}[${index}]`, out),
    );
  } else if (value && typeof value === "object") {
    for (const [key, child] of Object.entries(value)) {
      flattenStrings(child, jsonPath ? `${jsonPath}.${key}` : key, out);
    }
  }
  return out;
}

export function scanLocaleClaims(roots: string[]): ClaimScanResult {
  const files = [...new Set(roots.flatMap(listJsonFiles))].sort();
  const locales = new Set<string>();
  const findings: ClaimFinding[] = [];

  for (const file of files) {
    const locale = localeForFile(file);
    if (!locale) continue;
    locales.add(locale);
    const document = JSON.parse(fs.readFileSync(file, "utf8")) as unknown;
    for (const leaf of flattenStrings(document)) {
      for (const rule of RULES) {
        if (rule.patterns.some((pattern) => pattern.test(leaf.text))) {
          findings.push({
            claimClass: rule.claimClass,
            file,
            locale,
            jsonPath: leaf.jsonPath,
            text: leaf.text,
          });
        }
      }
    }
  }

  return {
    claimClassesChecked: REQUIRED_CLAIM_CLASSES,
    filesScanned: files.filter((file) => localeForFile(file) !== null).length,
    localesScanned: [...locales].sort(),
    findings,
  };
}

export function scanRepositoryLocaleClaims(repoRoot: string): ClaimScanResult {
  return scanLocaleClaims([
    path.join(repoRoot, "frontend", "src", "i18n", "messages"),
    path.join(repoRoot, "frontend", "src", "i18n", "guest-messages"),
  ]);
}

if (require.main === module) {
  const repoRoot = path.resolve(__dirname, "..", "..");
  const result = scanRepositoryLocaleClaims(repoRoot);
  if (result.findings.length > 0) {
    for (const finding of result.findings) {
      const relative = path.relative(repoRoot, finding.file);
      process.stderr.write(
        `${finding.claimClass}: ${relative}:${finding.jsonPath}: ${finding.text}\n`,
      );
    }
    process.exit(1);
  }
  process.stdout.write(
    `public-legal-claims: PASS (${result.filesScanned} files, ${result.localesScanned.length} locales, ${result.claimClassesChecked.length} claim classes)\n`,
  );
}
