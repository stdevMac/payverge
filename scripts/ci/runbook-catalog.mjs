import assert from "node:assert/strict";

const DAY_MS = 24 * 60 * 60 * 1000;
const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;

function parseUTCDate(value, label) {
  assert.equal(typeof value, "string", `${label} must be an ISO calendar date`);
  assert.match(value, ISO_DATE, `${label} must be an ISO calendar date`);
  const timestamp = Date.parse(`${value}T00:00:00Z`);
  assert.ok(Number.isFinite(timestamp), `${label} must be an ISO calendar date`);
  assert.equal(
    new Date(timestamp).toISOString().slice(0, 10),
    value,
    `${label} must be an ISO calendar date`,
  );
  return timestamp;
}

/**
 * Validates the runbook catalog against the tracked runbooks. Structure
 * (owner, ISO dates, a review interval of at most 184 days) always fails
 * hard. An overdue review fails only with `enforceDue` (the default for direct
 * callers); otherwise its path is returned in `overdue` so a contributor's CI
 * does not turn red on a calendar date alone.
 */
export function validateRunbookCatalog(
  catalog,
  tracked,
  nowMs = Date.now(),
  { enforceDue = true } = {},
) {
  assert.ok(Number.isFinite(nowMs), "runbook review clock must be a UTC timestamp");
  assert.ok(Array.isArray(catalog?.runbooks), "runbook catalog must contain runbooks");
  assert.ok(Array.isArray(tracked), "tracked runbooks must be an array");

  const overdue = [];
  const listed = catalog.runbooks.map((entry) => entry.path).sort();
  assert.deepEqual(listed, [...tracked].sort());
  for (const entry of catalog.runbooks) {
    assert.equal(typeof entry.owner, "string", `${entry.path} owner`);
    assert.doesNotMatch(
      entry.owner.trim().toLowerCase(),
      /^(?:|tbd|todo|unassigned)$/,
      `${entry.path} owner`,
    );
    const reviewed = parseUTCDate(entry.reviewed_on, `${entry.path} reviewed_on`);
    const due = parseUTCDate(entry.review_due, `${entry.path} review_due`);
    assert.ok(due > reviewed, `${entry.path} review_due must follow reviewed_on`);
    assert.ok(
      due - reviewed <= 184 * DAY_MS,
      `${entry.path} review interval exceeds 184 days`,
    );
    if (nowMs >= due + DAY_MS) {
      assert.ok(!enforceDue, `${entry.path} review is overdue`);
      overdue.push(entry.path);
    }
  }
  return { overdue };
}
