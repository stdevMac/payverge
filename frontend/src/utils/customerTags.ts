/** Keep first-seen casing; drop later case-insensitive duplicates (L5-7). */
const dedupeTagsCaseInsensitive = (tags: string[]): string[] => {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of tags) {
    const tag = raw.trim();
    if (!tag) continue;
    const key = tag.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(tag);
  }
  return out;
};

export const parseCustomerTags = (tags?: string | null): string[] => {
  const trimmedTags = tags?.trim();
  if (!trimmedTags) {
    return [];
  }

  try {
    const parsed = JSON.parse(trimmedTags);
    if (Array.isArray(parsed)) {
      return dedupeTagsCaseInsensitive(
        parsed
          .filter((tag): tag is string => typeof tag === "string")
          .map((tag) => tag.trim())
          .filter(Boolean),
      );
    }
  } catch {
    // Older rows may contain comma-separated tags instead of the JSON array.
  }

  return dedupeTagsCaseInsensitive(
    trimmedTags
      .split(",")
      .map((tag) => tag.trim())
      .filter(Boolean),
  );
};

export const serializeCustomerTagsInput = (tags: string): string => {
  return JSON.stringify(
    dedupeTagsCaseInsensitive(
      tags
        .split(",")
        .map((tag) => tag.trim())
        .filter(Boolean),
    ),
  );
};
