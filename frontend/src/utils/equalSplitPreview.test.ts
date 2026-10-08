import {
  countEqualSeatsTaken,
  equalPreviewCents,
  equalPreviewDollars,
  EQUAL_SHARE_META_KEY,
} from "./equalSplitPreview";

const NOW = 1_700_000_000_000;

describe("countEqualSeatsTaken (BE equalSplitSeatsTakenTx dual)", () => {
  it.each([
    {
      name: "empty shares",
      shares: [],
      nowMs: NOW,
      want: 0,
    },
    {
      name: "N held non-expired seats",
      shares: [
        { mode: "equal", status: "held", hold_expires_at: new Date(NOW + 60_000).toISOString() },
        { mode: "equal", status: "held", hold_expires_at: new Date(NOW + 60_000).toISOString() },
        { mode: "equal", status: "held", hold_expires_at: new Date(NOW + 60_000).toISOString() },
      ],
      nowMs: NOW,
      want: 3,
    },
    {
      name: "held + past expiry ignored",
      shares: [
        {
          mode: "equal",
          status: "held",
          hold_expires_at: new Date(NOW - 1).toISOString(),
        },
        {
          mode: "equal",
          status: "held",
          hold_expires_at: new Date(NOW + 60_000).toISOString(),
        },
      ],
      nowMs: NOW,
      want: 1,
    },
    {
      name: "advance now past expiry without changing shares",
      shares: [
        {
          mode: "equal",
          status: "held",
          hold_expires_at: new Date(NOW + 30_000).toISOString(),
          claimed_fractions: { [EQUAL_SHARE_META_KEY]: "1" },
        },
        {
          mode: "equal",
          status: "held",
          hold_expires_at: new Date(NOW + 30_000).toISOString(),
          claimed_fractions: { [EQUAL_SHARE_META_KEY]: "1" },
        },
      ],
      nowMs: NOW + 31_000,
      want: 0,
    },
    {
      name: "held + future expiry counts",
      shares: [
        {
          mode: "equal",
          status: "held",
          hold_expires_at: new Date(NOW + 1).toISOString(),
        },
      ],
      nowMs: NOW,
      want: 1,
    },
    {
      name: "held + nil expiry counts",
      shares: [{ mode: "equal", status: "held", hold_expires_at: null }],
      nowMs: NOW,
      want: 1,
    },
    {
      name: "settled always counts even if past expiry field present",
      shares: [
        {
          mode: "equal",
          status: "settled",
          hold_expires_at: new Date(NOW - 999_999).toISOString(),
        },
      ],
      nowMs: NOW,
      want: 1,
    },
    {
      name: "multi-seat meta __equal_shares__: 2",
      shares: [
        {
          mode: "equal",
          status: "held",
          hold_expires_at: new Date(NOW + 60_000).toISOString(),
          claimed_fractions: { [EQUAL_SHARE_META_KEY]: "2" },
        },
      ],
      nowMs: NOW,
      want: 2,
    },
    {
      name: "ignores non-equal and released",
      shares: [
        { mode: "custom", status: "held" },
        { mode: "equal", status: "released" },
        { mode: "items", status: "held" },
      ],
      nowMs: NOW,
      want: 0,
    },
  ])("$name", ({ shares, nowMs, want }) => {
    expect(countEqualSeatsTaken(shares, nowMs)).toBe(want);
  });
});

describe("equalPreviewCents (BE calculateEqualSplitCents dual)", () => {
  it.each([
    {
      name: "invalid available",
      available: 0,
      people: 3,
      covered: 1,
      want: 0,
    },
    {
      name: "remainingPeople <= 0",
      available: 100,
      people: 0,
      covered: 1,
      want: 0,
    },
    {
      name: "covered > remainingPeople",
      available: 100,
      people: 2,
      covered: 3,
      want: 0,
    },
    {
      name: "covered === remainingPeople takes full available",
      available: 10001,
      people: 3,
      covered: 3,
      want: 10001,
    },
    {
      name: "covered < remaining uses floor",
      available: 10000,
      people: 3,
      covered: 2,
      want: Math.floor(10000 / 3) * 2,
    },
    {
      name: "covered=1 simultaneous-safe floor",
      available: 101,
      people: 2,
      covered: 1,
      want: 50,
    },
    {
      name: "penny drain when available < people",
      available: 2,
      people: 12,
      covered: 1,
      want: 1,
    },
    {
      name: "penny drain covered > available",
      available: 2,
      people: 12,
      covered: 5,
      want: 2,
    },
  ])("$name", ({ available, people, covered, want }) => {
    expect(equalPreviewCents(available, people, covered)).toBe(want);
  });
});

describe("equalPreviewDollars (seats + cents composition)", () => {
  it("returns 0 when all seats taken but leftover available", () => {
    const shares = [
      { mode: "equal", status: "held" as const, claimed_fractions: { [EQUAL_SHARE_META_KEY]: "1" } },
      { mode: "equal", status: "held" as const, claimed_fractions: { [EQUAL_SHARE_META_KEY]: "1" } },
      { mode: "equal", status: "settled" as const, claimed_fractions: { [EQUAL_SHARE_META_KEY]: "1" } },
    ];
    expect(
      equalPreviewDollars({
        availableCents: 3,
        requestedPeople: 3,
        sharesCovered: 1,
        shares,
        nowMs: NOW,
      }),
    ).toBe(0);
  });

  it("frees seats when nowMs advances past hold_expires_at without share mutation", () => {
    const expires = new Date(NOW + 30_000).toISOString();
    const shares = [
      {
        mode: "equal",
        status: "held",
        hold_expires_at: expires,
        claimed_fractions: { [EQUAL_SHARE_META_KEY]: "1" },
      },
      {
        mode: "equal",
        status: "held",
        hold_expires_at: expires,
        claimed_fractions: { [EQUAL_SHARE_META_KEY]: "1" },
      },
    ];
    expect(
      equalPreviewDollars({
        availableCents: 5000,
        requestedPeople: 2,
        sharesCovered: 1,
        shares,
        nowMs: NOW,
      }),
    ).toBe(0);
    expect(
      equalPreviewDollars({
        availableCents: 5000,
        requestedPeople: 2,
        sharesCovered: 1,
        shares,
        nowMs: NOW + 31_000,
      }),
    ).toBe(25);
  });
});
