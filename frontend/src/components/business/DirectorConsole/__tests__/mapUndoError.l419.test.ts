/**
 * L4-19 — dedicated commit attribution for shared undo mapping + 24h window
 * + applied-card thread scoping (implementation lives in proposalActions.ts
 * and DirectorConsoleDashboard).
 */
import {
  filterProposalsForThread,
  mapUndoErrorKey,
  UNDO_WINDOW_HOURS,
} from "../proposalActions";

describe("L4-19 mapUndoError + applied thread scope", () => {
  it("maps statuses to distinct keys and advertises 24h undo window", () => {
    expect(UNDO_WINDOW_HOURS).toBe(24);
    expect(mapUndoErrorKey({ status: 410 })).toBe(
      "proposal.errors.undoExpired",
    );
    expect(mapUndoErrorKey({ status: 409 })).toBe("proposal.errors.undoStale");
    expect(mapUndoErrorKey({ code: "already_undone" })).toBe(
      "proposal.errors.alreadyUndone",
    );
  });

  it("does not leak applied cards into a foreign thread", () => {
    const proposals = [{ id: "p1" }, { id: "p2" }];
    const out = filterProposalsForThread(proposals, {
      threadId: 99,
      appliedThreadById: { p1: 1, p2: 99 },
      appliedIds: new Set(["p1", "p2"]),
    });
    expect(out.map((p) => p.id)).toEqual(["p2"]);
  });
});
