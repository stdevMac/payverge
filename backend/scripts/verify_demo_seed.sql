-- Verifies the trimmed demo testing fixture created by backend/scripts/demo_seed.sql.
--
-- Usage:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f backend/scripts/verify_demo_seed.sql

\echo 'Verifying Payverge demo seed...'

DROP TABLE IF EXISTS demo_seed_checks;
CREATE TEMP TABLE demo_seed_checks (
    check_name text PRIMARY KEY,
    actual numeric NOT NULL,
    expected text NOT NULL,
    passed boolean NOT NULL
);

DROP TABLE IF EXISTS demo_seed_thresholds;
CREATE TEMP TABLE demo_seed_thresholds (
    business_id text PRIMARY KEY,
    min_categories int NOT NULL,
    min_menu_items int NOT NULL,
    min_multi_image_items int NOT NULL,
    min_unavailable_items int NOT NULL,
    min_translations int NOT NULL,
    min_offers int NOT NULL,
    min_bundles int NOT NULL,
    min_tables int NOT NULL,
    min_counters int NOT NULL,
    min_staff int NOT NULL,
    min_invitations int NOT NULL,
    min_reservations int NOT NULL,
    min_crm_links int NOT NULL,
    min_bills int NOT NULL,
    min_paid_bills int NOT NULL,
    min_open_bills int NOT NULL,
    min_orders int NOT NULL,
    min_inventory_items int NOT NULL,
    min_low_stock int NOT NULL,
    min_out_stock int NOT NULL,
    min_inventory_units int NOT NULL,
    min_recipes int NOT NULL,
    min_delivery_orders int NOT NULL,
    min_drivers int NOT NULL,
    min_ledger_entries int NOT NULL,
    min_plugin_count int NOT NULL,
    min_ai_conversations int NOT NULL,
    min_director_threads int NOT NULL
);

INSERT INTO demo_seed_thresholds VALUES
    ('demo-core-business', 6, 36, 6, 3, 20, 6, 5, 10, 3, 6, 3, 18, 14, 65, 45, 5, 22, 24, 5, 2, 6, 18, 10, 4, 8, 8, 0, 0),
    ('demo-ai-pro-business', 8, 48, 8, 6, 50, 6, 5, 16, 4, 11, 3, 28, 24, 100, 70, 5, 40, 36, 5, 2, 6, 28, 16, 6, 6, 9, 16, 5);

INSERT INTO demo_seed_checks
SELECT 'trimmed database has exactly one owner user', count(*), '= 1', count(*) = 1
FROM users;

INSERT INTO demo_seed_checks
SELECT 'owner email exists', count(*), '= 1', count(*) = 1
FROM users
WHERE email = 'demo-owner@example.com'
  AND email_verified = true;

INSERT INTO demo_seed_checks
SELECT 'trimmed database has exactly two businesses', count(*), '= 2', count(*) = 2
FROM businesses;

INSERT INTO demo_seed_checks
SELECT 'owner owns both demo businesses', count(*), '= 2', count(*) = 2
FROM businesses b
JOIN users u ON u.id = b.user_id
WHERE u.email = 'demo-owner@example.com'
  AND b.business_id IN ('demo-core-business', 'demo-ai-pro-business');

INSERT INTO demo_seed_checks
SELECT 'Core business is active without AI', count(*), '= 1', count(*) = 1
FROM businesses
WHERE business_id = 'demo-core-business'
  AND custom_url = 'demo-core-kitchen'
  AND is_active = true
  AND closed_at IS NULL
  AND ai_ai_enabled = false;

INSERT INTO demo_seed_checks
SELECT 'AI business is active with AI', count(*), '= 1', count(*) = 1
FROM businesses
WHERE business_id = 'demo-ai-pro-business'
  AND custom_url = 'demo-ai-lounge'
  AND is_active = true
  AND closed_at IS NULL
  AND ai_ai_enabled = true
  AND ai_business_page_ai_enabled = true;

INSERT INTO demo_seed_checks
SELECT
    'AI Pro audited table route is seeded',
    count(*),
    '= 1',
    count(*) = 1
FROM tables t
JOIN businesses b ON b.id = t.business_id
WHERE b.business_id = 'demo-ai-pro-business'
  AND t.table_code = upper(substr(md5('payverge-fixture-ai-pro-table-2'), 1, 10))
  AND t.is_active = true;

INSERT INTO demo_seed_checks
SELECT
    'active counter bills have unique occupancy',
    count(*),
    '= 0',
    count(*) = 0
FROM (
    SELECT counter_id
    FROM bills
    WHERE counter_id IS NOT NULL
      AND status IN ('open', 'partial')
    GROUP BY counter_id
    HAVING count(*) > 1
) conflicts;

WITH demo AS (
    SELECT b.id, b.business_id
    FROM businesses b
    WHERE b.business_id IN ('demo-core-business', 'demo-ai-pro-business')
),
item_rows AS (
    SELECT
        d.business_id,
        category AS category_json,
        item AS item_json
    FROM demo d
    JOIN menus m ON m.business_id = d.id AND m.is_active = true
    CROSS JOIN LATERAL jsonb_array_elements(m.categories::jsonb) category
    CROSS JOIN LATERAL jsonb_array_elements(category->'items') item
),
item_counts AS (
    SELECT
        business_id,
        count(DISTINCT category_json->>'id') AS categories,
        count(*) AS menu_items,
        count(*) FILTER (WHERE jsonb_array_length(COALESCE(item_json->'images', '[]'::jsonb)) > 1) AS multi_image_items,
        count(*) FILTER (WHERE COALESCE((item_json->>'is_available')::boolean, true) = false) AS unavailable_items,
        count(*) FILTER (WHERE jsonb_array_length(COALESCE(item_json->'options', '[]'::jsonb)) > 0) AS items_with_options
    FROM item_rows
    GROUP BY business_id
),
metrics AS (
    SELECT
        d.business_id,
        COALESCE(ic.categories, 0) AS categories,
        COALESCE(ic.menu_items, 0) AS menu_items,
        COALESCE(ic.multi_image_items, 0) AS multi_image_items,
        COALESCE(ic.unavailable_items, 0) AS unavailable_items,
        COALESCE(ic.items_with_options, 0) AS items_with_options,
        (SELECT count(*) FROM translations x WHERE x.business_id = d.id) AS translations,
        (SELECT count(*) FROM business_gallery_images x WHERE x.business_id = d.id) AS gallery_images,
        (SELECT count(*) FROM business_operating_hours x WHERE x.business_id = d.id) AS operating_hours,
        (SELECT count(*) FROM business_special_features x WHERE x.business_id = d.id) AS special_features,
        (SELECT count(*) FROM business_currencies x WHERE x.business_id = d.id) AS currencies,
        (SELECT count(*) FROM business_languages x WHERE x.business_id = d.id) AS languages,
        (SELECT count(*) FROM offers x WHERE x.business_id = d.id) AS offers,
        (SELECT count(*) FROM offers x WHERE x.business_id = d.id AND x.applicable_to = 'bundle') AS bundle_offers,
        (SELECT count(*) FROM offers x WHERE x.business_id = d.id AND x.is_active = false) AS inactive_offers,
        (SELECT count(*) FROM offers x WHERE x.business_id = d.id AND x.end_date < now()) AS expired_offers,
        (SELECT count(*) FROM bundles x WHERE x.business_id = d.id) AS bundles,
        (SELECT count(*) FROM tables x WHERE x.business_id = d.id) AS tables,
        (SELECT count(*) FROM counters x WHERE x.business_id = d.id) AS counters,
        (SELECT count(*) FROM staff x WHERE x.business_id = d.id) AS staff,
        (SELECT count(*) FROM staff x WHERE x.business_id = d.id AND x.is_active = false) AS inactive_staff,
        (SELECT count(*) FROM staff_invitations x WHERE x.business_id = d.id) AS invitations,
        (SELECT count(*) FROM staff_login_codes x JOIN staff s ON s.id = x.staff_id WHERE s.business_id = d.id) AS staff_login_codes,
        (SELECT count(*) FROM rbac_audit_logs x WHERE x.business_id = d.id) AS rbac_audits,
        (SELECT count(*) FROM table_reservations x WHERE x.business_id = d.id) AS reservations,
        (SELECT count(DISTINCT status) FROM table_reservations x WHERE x.business_id = d.id) AS reservation_statuses,
        (SELECT count(*) FROM table_reservations x WHERE x.business_id = d.id AND x.status = 'pending') AS pending_reservations,
        (SELECT count(*) FROM reservation_status_histories x JOIN table_reservations r ON r.id = x.reservation_id WHERE r.business_id = d.id) AS reservation_history,
        (SELECT count(*) FROM customer_businesses x WHERE x.business_id = d.id) AS crm_links,
        (SELECT count(*) FROM customer_addresses x WHERE x.business_id = d.id) AS customer_addresses,
        (SELECT count(*) FROM customer_visits x JOIN customer_businesses cb ON cb.id = x.customer_business_id WHERE cb.business_id = d.id) AS customer_visits,
        (SELECT count(*) FROM customer_communications x WHERE x.business_id = d.id) AS customer_communications,
        (SELECT count(*) FROM bills x WHERE x.business_id = d.id) AS bills,
        (SELECT count(*) FROM bills x WHERE x.business_id = d.id AND x.status = 'paid') AS paid_bills,
        (SELECT count(*) FROM bills x WHERE x.business_id = d.id AND x.status = 'open') AS open_bills,
        (SELECT count(*) FROM bills x WHERE x.business_id = d.id AND NOT EXISTS (SELECT 1 FROM bill_items bi WHERE bi.bill_id = x.id)) AS bills_without_items,
        (SELECT count(*) FROM bill_history_events x WHERE x.business_id = d.id) AS bill_history_events,
        (SELECT count(*) FROM orders x WHERE x.business_id = d.id) AS orders,
        (SELECT count(DISTINCT status) FROM orders x WHERE x.business_id = d.id) AS order_statuses,
        (SELECT count(*) FROM payments p JOIN bills x ON x.id = p.bill_id WHERE x.business_id = d.id) AS crypto_payments,
        (SELECT count(*) FROM alternative_payments p JOIN bills x ON x.id = p.bill_id WHERE x.business_id = d.id) AS alternative_payments,
        (SELECT count(*) FROM inventory_items x WHERE x.business_id = d.id) AS inventory_items,
        (SELECT count(*) FROM inventory_items x WHERE x.business_id = d.id AND x.current_quantity <= x.reorder_threshold AND x.current_quantity > 0) AS low_stock_items,
        (SELECT count(*) FROM inventory_items x WHERE x.business_id = d.id AND x.current_quantity <= 0) AS out_stock_items,
        (SELECT count(DISTINCT unit) FROM inventory_items x WHERE x.business_id = d.id) AS inventory_units,
        (SELECT count(*) FROM inventory_recipes x WHERE x.business_id = d.id) AS recipes,
        (SELECT count(*) FROM inventory_movements x WHERE x.business_id = d.id) AS inventory_movements,
        (SELECT count(DISTINCT movement_type) FROM inventory_movements x WHERE x.business_id = d.id) AS inventory_movement_types,
        (SELECT count(*) FROM delivery_orders x WHERE x.business_id = d.id) AS delivery_orders,
        (SELECT count(DISTINCT status) FROM delivery_orders x WHERE x.business_id = d.id) AS delivery_statuses,
        (SELECT count(*) FROM delivery_drivers x WHERE x.business_id = d.id) AS drivers,
        (SELECT count(*) FROM delivery_zones x WHERE x.business_id = d.id) AS delivery_zones,
        (SELECT count(*) FROM delivery_status_history x JOIN delivery_orders o ON o.id = x.delivery_order_id WHERE o.business_id = d.id) AS delivery_history,
        (SELECT count(*) FROM manual_ledger_entries x WHERE x.business_id = d.id) AS ledger_entries,
        (SELECT count(*) FROM manual_ledger_entries x WHERE x.business_id = d.id AND x.voided_at IS NOT NULL) AS voided_ledger_entries,
        (SELECT count(*) FROM payroll_runs x WHERE x.business_id = d.id) AS payroll_runs,
        (SELECT count(*) FROM payroll_line_items x WHERE x.business_id = d.id) AS payroll_lines,
        (SELECT count(*) FROM business_plugins x WHERE x.business_id = d.id) AS business_plugins,
        (SELECT count(*) FROM report_schedules x WHERE x.business_id = d.id) AS report_schedules,
        (SELECT count(*) FROM ai_waiter_conversations x WHERE x.business_id = d.id) AS ai_conversations,
        (SELECT count(*) FROM ai_waiter_conversations x WHERE x.business_id = d.id AND x.status = 'closed') AS closed_ai_conversations,
        (SELECT count(*) FROM ai_waiter_conversations x WHERE x.business_id = d.id AND x.is_paused = true) AS paused_ai_conversations,
        (SELECT count(*) FROM ai_waiter_conversations x WHERE x.business_id = d.id AND x.session_id LIKE 'whatsapp:demo:%') AS whatsapp_ai_conversations,
        (SELECT count(*) FROM ai_waiter_messages x JOIN ai_waiter_conversations c ON c.id = x.conversation_id WHERE c.business_id = d.id) AS ai_messages,
        (SELECT count(*) FROM menu_extraction_jobs x WHERE x.business_id = d.id) AS menu_extraction_jobs,
        (SELECT count(*) FROM menu_wizard_sessions x WHERE x.business_id = d.id) AS menu_wizard_sessions,
        (SELECT count(*) FROM director_console_threads x WHERE x.business_id = d.id) AS director_threads,
        (SELECT count(*) FROM director_console_messages x WHERE x.business_id = d.id) AS director_messages
    FROM demo d
    LEFT JOIN item_counts ic ON ic.business_id = d.business_id
)
INSERT INTO demo_seed_checks
SELECT check_name, actual, expected, passed
FROM metrics m
JOIN demo_seed_thresholds t ON t.business_id = m.business_id
CROSS JOIN LATERAL (
    VALUES
        (m.business_id || ' menu categories', m.categories, '>=' || t.min_categories, m.categories >= t.min_categories),
        (m.business_id || ' menu items', m.menu_items, '>=' || t.min_menu_items, m.menu_items >= t.min_menu_items),
        (m.business_id || ' multi-image menu items', m.multi_image_items, '>=' || t.min_multi_image_items, m.multi_image_items >= t.min_multi_image_items),
        (m.business_id || ' unavailable menu items', m.unavailable_items, '>=' || t.min_unavailable_items, m.unavailable_items >= t.min_unavailable_items),
        (m.business_id || ' menu items with options', m.items_with_options, '>= 2', m.items_with_options >= 2),
        (m.business_id || ' translations', m.translations, '>=' || t.min_translations, m.translations >= t.min_translations),
        (m.business_id || ' gallery images', m.gallery_images, CASE WHEN m.business_id = 'demo-core-business' THEN '>= 5' ELSE '>= 7' END, m.gallery_images >= CASE WHEN m.business_id = 'demo-core-business' THEN 5 ELSE 7 END),
        (m.business_id || ' operating hours', m.operating_hours, '= 7', m.operating_hours = 7),
        (m.business_id || ' special features', m.special_features, CASE WHEN m.business_id = 'demo-core-business' THEN '>= 6' ELSE '>= 7' END, m.special_features >= CASE WHEN m.business_id = 'demo-core-business' THEN 6 ELSE 7 END),
        (m.business_id || ' enabled currencies', m.currencies, '>= 4', m.currencies >= 4),
        (m.business_id || ' enabled languages', m.languages, CASE WHEN m.business_id = 'demo-core-business' THEN '>= 4' ELSE '>= 6' END, m.languages >= CASE WHEN m.business_id = 'demo-core-business' THEN 4 ELSE 6 END),
        (m.business_id || ' offers', m.offers, '>=' || t.min_offers, m.offers >= t.min_offers),
        (m.business_id || ' bundle-targeted offers', m.bundle_offers, '>= 1', m.bundle_offers >= 1),
        (m.business_id || ' inactive offers', m.inactive_offers, '>= 1', m.inactive_offers >= 1),
        (m.business_id || ' expired offers', m.expired_offers, '>= 1', m.expired_offers >= 1),
        (m.business_id || ' bundles', m.bundles, '>=' || t.min_bundles, m.bundles >= t.min_bundles),
        (m.business_id || ' tables', m.tables, '= ' || t.min_tables, m.tables = t.min_tables),
        (m.business_id || ' counters', m.counters, '= ' || t.min_counters, m.counters = t.min_counters),
        (m.business_id || ' staff', m.staff, '>=' || t.min_staff, m.staff >= t.min_staff),
        (m.business_id || ' inactive staff', m.inactive_staff, '>= 1', m.inactive_staff >= 1),
        (m.business_id || ' staff invitations', m.invitations, '>=' || t.min_invitations, m.invitations >= t.min_invitations),
        (m.business_id || ' staff login code states', m.staff_login_codes, '>= 2', m.staff_login_codes >= 2),
        (m.business_id || ' RBAC audit logs', m.rbac_audits, '>= 6', m.rbac_audits >= 6),
        (m.business_id || ' reservations', m.reservations, '>=' || t.min_reservations, m.reservations >= t.min_reservations),
        (m.business_id || ' reservation statuses', m.reservation_statuses, '>= 6', m.reservation_statuses >= 6),
        (m.business_id || ' reservation history', m.reservation_history, '>= non-pending reservations', m.reservation_history >= m.reservations - m.pending_reservations),
        (m.business_id || ' CRM customer links', m.crm_links, '>=' || t.min_crm_links, m.crm_links >= t.min_crm_links),
        (m.business_id || ' customer addresses', m.customer_addresses, '>=' || t.min_crm_links, m.customer_addresses >= t.min_crm_links),
        (m.business_id || ' customer visits', m.customer_visits, '>=' || t.min_crm_links, m.customer_visits >= t.min_crm_links),
        (m.business_id || ' customer communications', m.customer_communications, '>=' || t.min_crm_links, m.customer_communications >= t.min_crm_links),
        (m.business_id || ' bills', m.bills, '>=' || t.min_bills, m.bills >= t.min_bills),
        (m.business_id || ' paid bills', m.paid_bills, '>=' || t.min_paid_bills, m.paid_bills >= t.min_paid_bills),
        (m.business_id || ' open bills', m.open_bills, '>=' || t.min_open_bills, m.open_bills >= t.min_open_bills),
        (m.business_id || ' bills all have bill_items', m.bills_without_items, '= 0', m.bills_without_items = 0),
        (m.business_id || ' bill history events', m.bill_history_events, '>= bills', m.bill_history_events >= m.bills),
        (m.business_id || ' orders', m.orders, '>=' || t.min_orders, m.orders >= t.min_orders),
        (m.business_id || ' order statuses', m.order_statuses, '>= 6', m.order_statuses >= 6),
        (m.business_id || ' crypto payments', m.crypto_payments, '>= 5', m.crypto_payments >= 5),
        (m.business_id || ' alternative payments', m.alternative_payments, '>= 5', m.alternative_payments >= 5),
        (m.business_id || ' inventory items', m.inventory_items, '>=' || t.min_inventory_items, m.inventory_items >= t.min_inventory_items),
        (m.business_id || ' low-stock items', m.low_stock_items, '>=' || t.min_low_stock, m.low_stock_items >= t.min_low_stock),
        (m.business_id || ' out-of-stock items', m.out_stock_items, '>=' || t.min_out_stock, m.out_stock_items >= t.min_out_stock),
        (m.business_id || ' inventory unit coverage', m.inventory_units, '>=' || t.min_inventory_units, m.inventory_units >= t.min_inventory_units),
        (m.business_id || ' inventory recipes', m.recipes, '>=' || t.min_recipes, m.recipes >= t.min_recipes),
        (m.business_id || ' inventory movements', m.inventory_movements, '>= inventory items', m.inventory_movements >= m.inventory_items),
        (m.business_id || ' inventory movement types', m.inventory_movement_types, '>= 7', m.inventory_movement_types >= 7),
        (m.business_id || ' delivery orders', m.delivery_orders, '>=' || t.min_delivery_orders, m.delivery_orders >= t.min_delivery_orders),
        (m.business_id || ' delivery statuses', m.delivery_statuses, '>= 10', m.delivery_statuses >= 10),
        (m.business_id || ' delivery drivers', m.drivers, '>=' || t.min_drivers, m.drivers >= t.min_drivers),
        (m.business_id || ' delivery zones', m.delivery_zones, '>= 3', m.delivery_zones >= 3),
        (m.business_id || ' delivery history', m.delivery_history, '>= delivered/cancelled coverage', m.delivery_history >= m.delivery_orders - 1),
        (m.business_id || ' manual ledger entries', m.ledger_entries, '>=' || t.min_ledger_entries, m.ledger_entries >= t.min_ledger_entries),
        (m.business_id || ' voided ledger entry', m.voided_ledger_entries, '>= 1', m.voided_ledger_entries >= 1),
        (m.business_id || ' payroll runs', m.payroll_runs, '>= 2', m.payroll_runs >= 2),
        (m.business_id || ' payroll line items', m.payroll_lines, '>= active staff count', m.payroll_lines >= m.staff - m.inactive_staff),
        (m.business_id || ' business plugins', m.business_plugins, '>=' || t.min_plugin_count, m.business_plugins >= t.min_plugin_count),
        (m.business_id || ' report schedules', m.report_schedules, '>= 2', m.report_schedules >= 2),
        (m.business_id || ' AI conversations', m.ai_conversations, CASE WHEN t.min_ai_conversations = 0 THEN '= 0' ELSE '>=' || t.min_ai_conversations END, CASE WHEN t.min_ai_conversations = 0 THEN m.ai_conversations = 0 ELSE m.ai_conversations >= t.min_ai_conversations END),
        (m.business_id || ' closed AI conversations', m.closed_ai_conversations, CASE WHEN t.min_ai_conversations = 0 THEN '= 0' ELSE '>= 4' END, CASE WHEN t.min_ai_conversations = 0 THEN m.closed_ai_conversations = 0 ELSE m.closed_ai_conversations >= 4 END),
        (m.business_id || ' paused AI conversations', m.paused_ai_conversations, CASE WHEN t.min_ai_conversations = 0 THEN '= 0' ELSE '>= 2' END, CASE WHEN t.min_ai_conversations = 0 THEN m.paused_ai_conversations = 0 ELSE m.paused_ai_conversations >= 2 END),
        (m.business_id || ' WhatsApp-origin AI conversations', m.whatsapp_ai_conversations, CASE WHEN t.min_ai_conversations = 0 THEN '= 0' ELSE '>= 3' END, CASE WHEN t.min_ai_conversations = 0 THEN m.whatsapp_ai_conversations = 0 ELSE m.whatsapp_ai_conversations >= 3 END),
        (m.business_id || ' AI messages', m.ai_messages, CASE WHEN t.min_ai_conversations = 0 THEN '= 0' ELSE '>= 48' END, CASE WHEN t.min_ai_conversations = 0 THEN m.ai_messages = 0 ELSE m.ai_messages >= 48 END),
        (m.business_id || ' AI menu extraction jobs', m.menu_extraction_jobs, CASE WHEN t.min_ai_conversations = 0 THEN '= 0' ELSE '>= 4' END, CASE WHEN t.min_ai_conversations = 0 THEN m.menu_extraction_jobs = 0 ELSE m.menu_extraction_jobs >= 4 END),
        (m.business_id || ' AI menu wizard sessions', m.menu_wizard_sessions, CASE WHEN t.min_ai_conversations = 0 THEN '= 0' ELSE '>= 3' END, CASE WHEN t.min_ai_conversations = 0 THEN m.menu_wizard_sessions = 0 ELSE m.menu_wizard_sessions >= 3 END),
        (m.business_id || ' Director Console threads', m.director_threads, CASE WHEN t.min_director_threads = 0 THEN '= 0' ELSE '>=' || t.min_director_threads END, CASE WHEN t.min_director_threads = 0 THEN m.director_threads = 0 ELSE m.director_threads >= t.min_director_threads END),
        (m.business_id || ' Director Console messages', m.director_messages, CASE WHEN t.min_director_threads = 0 THEN '= 0' ELSE '>= 10' END, CASE WHEN t.min_director_threads = 0 THEN m.director_messages = 0 ELSE m.director_messages >= 10 END)
) checks(check_name, actual, expected, passed);

INSERT INTO demo_seed_checks
SELECT 'global supported currencies', count(*), '>= 6', count(*) >= 6
FROM supported_currencies
WHERE code IN ('USD', 'EUR', 'ARS', 'AED', 'SAR', 'GBP');

INSERT INTO demo_seed_checks
SELECT 'global supported languages', count(*), '>= 6', count(*) >= 6
FROM supported_languages
WHERE code IN ('en', 'es', 'pt', 'ar', 'fr', 'zh');

INSERT INTO demo_seed_checks
SELECT 'global exchange rates', count(*), '>= 6', count(*) >= 6
FROM exchange_rates;

INSERT INTO demo_seed_checks
SELECT 'plugin catalog active and inactive states', count(*), '>= 10 total with inactive present', count(*) >= 10 AND count(*) FILTER (WHERE is_active = false) >= 1
FROM plugins;

INSERT INTO demo_seed_checks
SELECT 'active plugin catalog images use local assets', count(*), '= 0', count(*) = 0
FROM plugins
WHERE is_active = true
  AND image LIKE 'https://dummyimage.com/%';

INSERT INTO demo_seed_checks
SELECT 'webhook fixtures', count(*), '>= 3 with success and failure', count(*) >= 3 AND count(*) FILTER (WHERE status = 'processed') >= 1 AND count(*) FILTER (WHERE status <> 'processed') >= 1
FROM webhook_events
WHERE webhook_id LIKE 'demo_seed_%';

INSERT INTO demo_seed_checks
SELECT 'analytics page views', count(*), '>= 48', count(*) >= 48
FROM page_views
WHERE session_id LIKE 'pvseed-demo-%';

INSERT INTO demo_seed_checks
SELECT 'analytics interactions', count(*), '>= 48', count(*) >= 48
FROM user_interactions
WHERE session_id LIKE 'pvseed-demo-%';

INSERT INTO demo_seed_checks
SELECT 'analytics conversions', count(*), '>= 16', count(*) >= 16
FROM conversion_events
WHERE session_id LIKE 'pvseed-demo-%';

INSERT INTO demo_seed_checks
SELECT 'admin error logs', count(*), '>= 4', count(*) >= 4
FROM error_logs
WHERE request_id LIKE 'pvseed-demo-error-%';

INSERT INTO demo_seed_checks
SELECT 'safe demo plugin configuration has no live secrets', count(*), '= 0', count(*) = 0
FROM business_plugins bp
JOIN businesses b ON b.id = bp.business_id
WHERE b.business_id IN ('demo-core-business', 'demo-ai-pro-business')
  AND bp.config::text ~* '(sk_live|pk_live|-----BEGIN|private[_ -]?key|whatsmeow|session_secret)';

DO $$
DECLARE
    failures text;
BEGIN
    SELECT string_agg(check_name || ' actual=' || actual || ' expected ' || expected, E'\n')
    INTO failures
    FROM demo_seed_checks
    WHERE passed = false;

    IF failures IS NOT NULL THEN
        RAISE EXCEPTION 'Demo seed verification failed:%', E'\n' || failures;
    END IF;
END $$;

SELECT check_name, actual, expected
FROM demo_seed_checks
ORDER BY check_name;

\echo 'Payverge demo seed verification passed.'
