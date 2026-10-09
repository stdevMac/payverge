// Reduce an arbitrary shot name to a filesystem-safe slug.
export function sanitizeName(name) {
  return String(name)
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9-_]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

// chrome.downloads.download filename: dated subfolder + slug + .png.
// dateStr is passed in (not computed here) so this stays pure and testable.
export function buildFilename(name, dateStr) {
  return `payverge-shots/${dateStr}/${sanitizeName(name)}.png`;
}
