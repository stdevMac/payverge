import path from "node:path";

import { readRegistry, repoRootFromFrontend } from "./common";
import {
  runLocaleValidation,
  type LocaleValidationError,
} from "./validate-locales";

interface LocaleCounts {
  errors: number;
  warnings: number;
}

function main(): void {
  const repoRoot = repoRootFromFrontend();
  const registry = readRegistry(repoRoot);
  const validation = runLocaleValidation({ repoRoot });
  const counts = buildLocaleCounts(validation.errors, validation.warnings);

  console.log("Payverge locale report");
  console.log(`Default locale: ${registry.defaultLocale}`);
  console.log(
    [
      "Locale".padEnd(10),
      "Path".padEnd(10),
      "Source".padEnd(10),
      "Publishable".padEnd(12),
      "Surfaces".padEnd(8),
      "Errors".padEnd(6),
      "Warnings",
    ].join("  "),
  );

  for (const locale of Object.values(registry.locales)) {
    const localeCounts = counts.get(locale.canonical) ?? {
      errors: 0,
      warnings: 0,
    };

    console.log(
      [
        locale.canonical.padEnd(10),
        locale.pathSegment.padEnd(10),
        locale.sourceFolder.padEnd(10),
        String(locale.publishable).padEnd(12),
        String(locale.requiredSurfaces.length).padEnd(8),
        String(localeCounts.errors).padEnd(6),
        String(localeCounts.warnings),
      ].join("  "),
    );
  }

  console.log(
    `Validation summary: ${validation.errors.length} errors, ${validation.warnings.length} warnings`,
  );

  printEntries("Warnings", "[warn]", validation.warnings);
  printEntries("Errors", "[error]", validation.errors);
  process.exitCode = 0;
}

function buildLocaleCounts(
  errors: LocaleValidationError[],
  warnings: LocaleValidationError[],
): Map<string, LocaleCounts> {
  const counts = new Map<string, LocaleCounts>();

  for (const error of errors) {
    const localeCounts = counts.get(error.locale) ?? { errors: 0, warnings: 0 };
    localeCounts.errors += 1;
    counts.set(error.locale, localeCounts);
  }

  for (const warning of warnings) {
    const localeCounts = counts.get(warning.locale) ?? { errors: 0, warnings: 0 };
    localeCounts.warnings += 1;
    counts.set(warning.locale, localeCounts);
  }

  return counts;
}

function printEntries(
  heading: string,
  prefix: "[warn]" | "[error]",
  entries: LocaleValidationError[],
): void {
  if (entries.length === 0) {
    return;
  }

  console.log("");
  console.log(`${heading}:`);

  for (const entry of entries) {
    const pathSuffix = entry.path ? ` (${entry.path})` : "";
    console.log(
      `${prefix} ${entry.locale} ${entry.surface} ${entry.code}: ${entry.message}${pathSuffix}`,
    );
  }
}

if (
  process.argv[1] &&
  /^report-locales\.[jt]s$/.test(path.basename(process.argv[1]))
) {
  main();
}
