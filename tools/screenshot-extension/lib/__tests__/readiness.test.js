import {
  evaluateReadiness,
  BOOT_GATE_SELECTOR,
  SKELETON_PULSE_SELECTOR,
  MIN_SKELETON_AREA_PX,
} from "../readiness.js";

// Minimal fake document. `matching` maps selector -> array of fake elements
// (each may carry a rect). querySelector returns the first match or null.
function fakeDoc(matching = {}) {
  return {
    querySelector: (sel) => (matching[sel] && matching[sel][0]) || null,
    querySelectorAll: (sel) => matching[sel] || [],
  };
}

function pulseEl(width, height) {
  return { getBoundingClientRect: () => ({ width, height }) };
}

test("not ready while the aria-busy boot gate is present", () => {
  const doc = fakeDoc({ [BOOT_GATE_SELECTOR]: [{}] });
  const result = evaluateReadiness(doc, {});
  expect(result.ready).toBe(false);
  expect(result.reason).toBe("loading");
});

test("not ready while a large pulsing skeleton block is visible", () => {
  const doc = fakeDoc({ [SKELETON_PULSE_SELECTOR]: [pulseEl(600, 24)] });
  const result = evaluateReadiness(doc, {});
  expect(result.ready).toBe(false);
  expect(result.reason).toBe("skeleton");
});

test("a tiny pulsing live-indicator dot does NOT block readiness", () => {
  // e.g. LiveBills' 8x8 green "Live" dot pulses forever by design.
  const doc = fakeDoc({ [SKELETON_PULSE_SELECTOR]: [pulseEl(8, 8)] });
  const result = evaluateReadiness(doc, {});
  expect(result.ready).toBe(true);
});

test("skeleton area threshold is respected exactly", () => {
  const justUnder = fakeDoc({
    [SKELETON_PULSE_SELECTOR]: [pulseEl(1, MIN_SKELETON_AREA_PX - 1)],
  });
  expect(evaluateReadiness(justUnder, {}).ready).toBe(true);
  const atThreshold = fakeDoc({
    [SKELETON_PULSE_SELECTOR]: [pulseEl(1, MIN_SKELETON_AREA_PX)],
  });
  expect(evaluateReadiness(atThreshold, {}).ready).toBe(false);
});

test("pulse elements without a measurable rect are ignored", () => {
  const doc = fakeDoc({ [SKELETON_PULSE_SELECTOR]: [{}] });
  expect(evaluateReadiness(doc, {}).ready).toBe(true);
});

test("not ready when a required waitFor selector is absent", () => {
  const doc = fakeDoc({});
  const result = evaluateReadiness(doc, { waitFor: '[data-testid="dashboard-shell"]' });
  expect(result.ready).toBe(false);
  expect(result.reason).toBe("awaiting-selector");
});

test("ready when nothing is loading and waitFor target is present", () => {
  const doc = fakeDoc({ '[data-testid="dashboard-shell"]': [{}] });
  const result = evaluateReadiness(doc, { waitFor: '[data-testid="dashboard-shell"]' });
  expect(result.ready).toBe(true);
  expect(result.reason).toBe("ready");
});

test("ready when nothing is loading and no waitFor requested", () => {
  const doc = fakeDoc({});
  const result = evaluateReadiness(doc, {});
  expect(result.ready).toBe(true);
  expect(result.reason).toBe("ready");
});
