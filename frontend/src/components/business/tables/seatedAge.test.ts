import { sentenceCaseLeaf } from "@/i18n/sentenceCaseLeaf";
import {
  SEATED_EMPTY,
  firstValidIso,
  isMissingTranslationLeaf,
  resolveSeatedIso,
  seatedSourcesFromStatus,
  translationOrFallback,
} from "./seatedAge";

const LAST_SEEN = "2026-08-19T20:12:00.000Z";
const OPENED = "2026-08-19T18:00:00.000Z";

describe("firstValidIso", () => {
  it("skips empty, blank, invalid, and sentinel timestamps", () => {
    expect(
      firstValidIso(
        null,
        "",
        "  ",
        "not-a-date",
        "0001-01-01T00:00:00Z",
        "1970-01-01T00:00:00.000Z",
        undefined,
      ),
    ).toBeNull();
  });

  it("returns the first usable ISO string", () => {
    expect(firstValidIso("", "nope", OPENED, LAST_SEEN)).toBe(OPENED);
  });
});

describe("resolveSeatedIso", () => {
  it("prefers the open check created_at", () => {
    expect(
      resolveSeatedIso({
        created_at: OPENED,
        last_seen: LAST_SEEN,
      }),
    ).toBe(OPENED);
  });

  it("uses last_seen when created_at is missing, blank, sentinel, or unparseable", () => {
    expect(resolveSeatedIso({ created_at: null, last_seen: LAST_SEEN })).toBe(
      LAST_SEEN,
    );
    expect(resolveSeatedIso({ created_at: "", last_seen: LAST_SEEN })).toBe(
      LAST_SEEN,
    );
    expect(
      resolveSeatedIso({
        created_at: "0001-01-01T00:00:00Z",
        last_seen: LAST_SEEN,
      }),
    ).toBe(LAST_SEEN);
    expect(
      resolveSeatedIso({
        created_at: "Open check warning",
        last_seen: LAST_SEEN,
      }),
    ).toBe(LAST_SEEN);
  });

  it("uses last_seen for an occupied table with no active-bill created_at", () => {
    expect(resolveSeatedIso({ last_seen: LAST_SEEN })).toBe(LAST_SEEN);
  });

  it("returns null only when no candidate is a usable instant", () => {
    expect(
      resolveSeatedIso({
        created_at: "0001-01-01T00:00:00Z",
        last_seen: null,
      }),
    ).toBeNull();
  });
});

describe("seatedSourcesFromStatus", () => {
  it("falls back to updated_at and row last_seen when created_at is a sentinel", () => {
    expect(
      seatedSourcesFromStatus({
        seated_at: null,
        last_seen: LAST_SEEN,
        active_bills: [
          {
            created_at: "0001-01-01T00:00:00Z",
            updated_at: "2026-08-19T20:00:00.000Z",
          },
        ],
      }),
    ).toEqual({
      created_at: null,
      last_seen: "2026-08-19T20:00:00.000Z",
    });
  });

  it("uses the status seated_at when the bill opened_at is a sentinel", () => {
    expect(
      seatedSourcesFromStatus({
        seated_at: LAST_SEEN,
        active_bills: [{ created_at: "0001-01-01T00:00:00Z" }],
      }),
    ).toEqual({ created_at: LAST_SEEN, last_seen: LAST_SEEN });
  });

  it("reads camelCase bill timestamps", () => {
    expect(
      seatedSourcesFromStatus({
        active_bills: [
          { createdAt: OPENED, updatedAt: LAST_SEEN },
        ],
      }),
    ).toEqual({ created_at: OPENED, last_seen: LAST_SEEN });
  });
});

describe("isMissingTranslationLeaf", () => {
  it("detects the key and sentenceCaseLeaf, not an English regex", () => {
    const key = "aging.openCheckWarning";
    expect(isMissingTranslationLeaf(sentenceCaseLeaf(key), key)).toBe(true);
    expect(isMissingTranslationLeaf("openCheckWarning", key)).toBe(true);
    expect(isMissingTranslationLeaf(key, key)).toBe(true);
    expect(
      isMissingTranslationLeaf("This check has been open a long time", key),
    ).toBe(false);
    expect(
      isMissingTranslationLeaf("Esta cuenta lleva mucho tiempo abierta", key),
    ).toBe(false);
  });

  it("does not treat a real seated-ago label as a leaf", () => {
    expect(
      translationOrFallback("2h ago", "timeAgo.hoursAgo", "2h"),
    ).toBe("2h ago");
    expect(
      translationOrFallback(
        sentenceCaseLeaf("timeAgo.hoursAgo"),
        "timeAgo.hoursAgo",
        "2h",
      ),
    ).toBe("2h");
    expect(SEATED_EMPTY).toBe("—");
  });
});
