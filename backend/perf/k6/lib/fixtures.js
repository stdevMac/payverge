// Seeded business slugs. Must match backend/perf/seed/main.go output
// (BusinessId + CustomURL = perf-seed-001 .. perf-seed-020).
export const seededBusinessSlugs = [
  'perf-seed-001', 'perf-seed-002', 'perf-seed-003', 'perf-seed-004',
  'perf-seed-005', 'perf-seed-006', 'perf-seed-007', 'perf-seed-008',
  'perf-seed-009', 'perf-seed-010', 'perf-seed-011', 'perf-seed-012',
  'perf-seed-013', 'perf-seed-014', 'perf-seed-015', 'perf-seed-016',
  'perf-seed-017', 'perf-seed-018', 'perf-seed-019', 'perf-seed-020',
];

// Deterministic per-VU/iter pick that spreads load across all 20 slugs.
export function pickBusiness(vu, iter) {
  const idx = ((vu * 1009) + iter) % seededBusinessSlugs.length;
  return seededBusinessSlugs[idx];
}
