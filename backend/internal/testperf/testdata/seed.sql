-- testperf seed: applied AFTER the schema exists. Benches build it with
-- genesisdb.Start (genesis baseline plus pending numbered migrations).
--
-- Goal: provide the minimum fixture surface needed by bench targets in Tasks
-- 7-8: one menu route (needs businesses.custom_url + a menus row) per
-- business, and 50 historical paid bills for business 1 so analytics-shaped
-- benches have something to read. Idempotent via ON CONFLICT DO NOTHING on
-- the natural unique columns.

INSERT INTO businesses (business_id, owner_address, name, settlement_addr, tipping_addr, custom_url, business_page_enabled, is_active, timezone, created_at, updated_at)
VALUES
  ('perf-bench-001', '0x000000000000000000000000000000000000bEEF', 'Perf Bench 001', '0x000000000000000000000000000000000000bEEF', '0x000000000000000000000000000000000000bEEF', 'perf-bench-001', TRUE, TRUE, 'UTC', NOW(), NOW()),
  ('perf-bench-002', '0x000000000000000000000000000000000000bEEF', 'Perf Bench 002', '0x000000000000000000000000000000000000bEEF', '0x000000000000000000000000000000000000bEEF', 'perf-bench-002', TRUE, TRUE, 'UTC', NOW(), NOW()),
  ('perf-bench-003', '0x000000000000000000000000000000000000bEEF', 'Perf Bench 003', '0x000000000000000000000000000000000000bEEF', '0x000000000000000000000000000000000000bEEF', 'perf-bench-003', TRUE, TRUE, 'UTC', NOW(), NOW())
ON CONFLICT (business_id) DO NOTHING;

-- One menu per business with an empty category list. Benches that need
-- items can extend this seed; the menu route only needs the row to exist.
INSERT INTO menus (business_id, categories, is_active, version, created_at, updated_at)
SELECT b.id, '[]', TRUE, 1, NOW(), NOW()
FROM businesses b
WHERE b.business_id IN ('perf-bench-001', 'perf-bench-002', 'perf-bench-003')
  AND NOT EXISTS (SELECT 1 FROM menus m WHERE m.business_id = b.id);

-- One table per business, deterministic table_code so bench code can fetch it.
INSERT INTO tables (business_id, table_code, name, capacity, is_active, qr_foreground_color, qr_background_color, qr_logo_size, qr_text_font, created_at, updated_at)
SELECT b.id, b.business_id || '-t01', 'T01', 4, TRUE, '#000000', '#FFFFFF', 20, 'Verdana', NOW(), NOW()
FROM businesses b
WHERE b.business_id IN ('perf-bench-001', 'perf-bench-002', 'perf-bench-003')
ON CONFLICT (table_code) DO NOTHING;

-- 50 paid bills for business 1, spread across the last 30 days. total_amount
-- is in cents (see Money wire contract); 1000..5000 cents = $10..$50.
INSERT INTO bills (business_id, table_id, bill_number, items, subtotal, tax_amount, service_fee_amount, total_amount, paid_amount, tip_amount, status, settlement_addr, tipping_addr, created_at, updated_at)
SELECT
  b.id,
  t.id,
  'PERF-BENCH-001-' || LPAD(gs::text, 5, '0'),
  '[]',
  1000 + (gs * 80) % 4000,
  0, 0,
  1000 + (gs * 80) % 4000,
  1000 + (gs * 80) % 4000,
  0,
  'paid',
  '0x000000000000000000000000000000000000bEEF',
  '0x000000000000000000000000000000000000bEEF',
  NOW() - ((gs % 30) || ' days')::interval,
  NOW() - ((gs % 30) || ' days')::interval
FROM businesses b
JOIN tables t ON t.business_id = b.id AND t.table_code = 'perf-bench-001-t01'
CROSS JOIN generate_series(1, 50) AS gs
WHERE b.business_id = 'perf-bench-001'
ON CONFLICT (bill_number) DO NOTHING;
