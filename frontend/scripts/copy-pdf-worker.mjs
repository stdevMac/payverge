// frontend/scripts/copy-pdf-worker.mjs
import { copyFileSync, mkdirSync, existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

// Resolve the worker from the installed pdfjs-dist so the version can never
// drift from what the app imports (legacy build matches PDFDigitizer's import).
const workerSrc = require.resolve("pdfjs-dist/legacy/build/pdf.worker.min.mjs");
const outDir = resolve(root, "public/pdf");
const outFile = resolve(outDir, "pdf.worker.min.mjs");

if (!existsSync(workerSrc)) {
  console.error(`[copy-pdf-worker] worker not found at ${workerSrc}`);
  process.exit(1);
}
mkdirSync(outDir, { recursive: true });
copyFileSync(workerSrc, outFile);
console.log(`[copy-pdf-worker] copied -> ${outFile}`);
