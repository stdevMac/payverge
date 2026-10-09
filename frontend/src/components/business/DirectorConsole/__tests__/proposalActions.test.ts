import {
  filterProposalsForThread,
  formatProposalExpiry,
  mapUndoErrorKey,
  UNDO_WINDOW_HOURS,
} from "../proposalActions";

describe("proposalActions (L4-18 / L4-19)", () => {
  it("maps undo errors with distinct keys", () => {
    expect(mapUndoErrorKey({ code: "already_undone" })).toBe(
      "proposal.errors.alreadyUndone",
    );
    expect(mapUndoErrorKey({ status: 409 })).toBe("proposal.errors.undoStale");
    expect(mapUndoErrorKey({ status: 410 })).toBe(
      "proposal.errors.undoExpired",
    );
    expect(mapUndoErrorKey({ status: 500 })).toBe(
      "proposal.errors.undoFailed",
    );
  });

  it("documents the 24h undo window constant", () => {
    expect(UNDO_WINDOW_HOURS).toBe(24);
  });

  it("formats expiry countdown and expired state", () => {
    const t = (key: string, params?: Record<string, string | number>) =>
      params ? `${key}:${JSON.stringify(params)}` : key;
    const now = Date.parse("2026-08-05T12:00:00Z");
    expect(
      formatProposalExpiry("2026-08-05T12:45:00Z", now, t),
    ).toContain("expiresInMinutes");
    expect(
      formatProposalExpiry("2026-08-05T15:00:00Z", now, t),
    ).toContain("expiresInHours");
    expect(formatProposalExpiry("2026-08-05T11:00:00Z", now, t)).toBe(
      "proposal.expired",
    );
    expect(formatProposalExpiry("", now, t)).toBeNull();
  });

  it("keeps applied cards only on their origin thread", () => {
    const proposals = [{ id: "a" }, { id: "b" }, { id: "c" }];
    const appliedIds = new Set(["a", "b"]);
    const appliedThreadById = { a: 1, b: 2 };
    // Staged "c" always visible; applied "a" only on thread 1.
    expect(
      filterProposalsForThread(proposals, {
        threadId: 1,
        appliedThreadById,
        appliedIds,
      }).map((p) => p.id),
    ).toEqual(["a", "c"]);
    expect(
      filterProposalsForThread(proposals, {
        threadId: 2,
        appliedThreadById,
        appliedIds,
      }).map((p) => p.id),
    ).toEqual(["b", "c"]);
  });
});
