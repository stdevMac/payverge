-- Payverge full demo seed.
--
-- Usage:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f backend/scripts/demo_seed.sql
--
-- This script is intentionally owner-safe:
-- - It attaches demo businesses to demo-owner@example.com.
-- - It does not overwrite unrelated owner businesses.
-- - It uses deterministic demo identifiers and targeted cleanup.
-- - It does not insert live provider credentials, live session tokens, or WhatsApp protocol secrets.

BEGIN;

DO $$
DECLARE
    v_owner_email text := 'demo-owner@example.com';
    v_wallet text := '0x0000000000000000000000000000000000000001';
    v_owner_id bigint;
    v_core_id bigint;
    v_ai_id bigint;
    v_menu_id bigint;
    v_bill_id bigint;
    v_order_id bigint;
    v_table_id bigint;
    v_counter_id bigint;
    v_customer_id bigint;
    v_customer_business_id bigint;
    v_staff_id bigint;
    v_driver_id bigint;
    v_zone_id bigint;
    v_delivery_id bigint;
    v_admin_user_id bigint;
    v_job_id bigint;
    v_wizard_id bigint;
    v_thread_id bigint;
    v_i int;
    v_j int;
    v_rating int;
    v_status text;
    v_business_key text;
    v_subtotal numeric;
    v_tax numeric;
    v_service numeric;
    v_total numeric;
    v_paid numeric;
    v_tip numeric;
    v_bill_created timestamptz;
    v_closed_at timestamptz;
    v_demo_day timestamptz := date_trunc('day', now());
    v_menu_json jsonb;
    v_demo_business_ids bigint[];
    v_demo_customer_ids bigint[];
    v_core_statuses text[] := ARRAY['pending','approved','in_kitchen','ready','delivered','cancelled'];
    v_ai_statuses text[] := ARRAY['pending','approved','in_kitchen','ready','delivered','cancelled'];
    v_delivery_statuses text[] := ARRAY['pending','confirmed','preparing','ready','assigned','picked_up','in_transit','nearby','delivered','cancelled'];
    v_reservation_statuses text[] := ARRAY['confirmed','pending','pending','completed','cancelled','no_show','seated'];
BEGIN
    RAISE NOTICE 'Starting Payverge demo seed';

    CREATE TABLE IF NOT EXISTS bill_items (
        id uuid PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text))::uuid,
        bill_id integer NOT NULL REFERENCES bills(id) ON DELETE CASCADE,
        menu_item_id varchar(255) NOT NULL DEFAULT '',
        name varchar(255) NOT NULL,
        price numeric(10, 2) NOT NULL,
        quantity integer NOT NULL,
        options jsonb,
        item_type text DEFAULT 'menu_item',
        bundle_id integer,
        parent_bundle_id integer,
        source_offer_id integer,
        subtotal numeric(10, 2) NOT NULL,
        created_at timestamptz DEFAULT CURRENT_TIMESTAMP
    );
    CREATE INDEX IF NOT EXISTS idx_bill_items_bill_id ON bill_items(bill_id);
    CREATE INDEX IF NOT EXISTS idx_bill_items_name ON bill_items(name);

    SELECT ARRAY(
        SELECT id
        FROM businesses
        WHERE business_id IN ('demo-core-business', 'demo-ai-pro-business')
           OR custom_url IN ('demo-core-kitchen', 'demo-ai-lounge')
    ) INTO v_demo_business_ids;

    -- The OR ... '.test' branches survive the .test → .example rename so
    -- dev DBs seeded before the rename still get their stale rows cleaned
    -- up. Safe to drop on the next pass through this file (≥1 release).
    SELECT ARRAY(
        SELECT id
        FROM customers
        WHERE email LIKE '%@core-demo.payverge.example'
           OR email LIKE '%@ai-demo.payverge.example'
           OR email LIKE '%@core-demo.payverge.test'
           OR email LIKE '%@ai-demo.payverge.test'
    ) INTO v_demo_customer_ids;

    IF cardinality(v_demo_business_ids) > 0 THEN
        UPDATE counters SET current_bill_id = NULL WHERE business_id = ANY(v_demo_business_ids);
        UPDATE delivery_drivers SET current_delivery_id = NULL WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM ai_waiter_messages
        WHERE conversation_id IN (SELECT id FROM ai_waiter_conversations WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM ai_waiter_conversations WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM director_console_messages WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM director_console_threads WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM menu_extraction_images
        WHERE job_id IN (SELECT id FROM menu_extraction_jobs WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM menu_extraction_jobs WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM menu_wizard_messages
        WHERE session_id IN (SELECT id FROM menu_wizard_sessions WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM menu_wizard_sessions WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM delivery_status_history
        WHERE delivery_order_id IN (SELECT id FROM delivery_orders WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM delivery_orders WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM delivery_drivers WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM delivery_zones WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM delivery_settings WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM inventory_movements WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM inventory_recipes WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM inventory_items WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM inventory_settings WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM payroll_line_items WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM payroll_runs WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM manual_ledger_entries WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM customer_communications WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM customer_visits
        WHERE customer_business_id IN (SELECT id FROM customer_businesses WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM customer_addresses WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM customer_businesses WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM alternative_payments
        WHERE bill_id IN (SELECT id FROM bills WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM payments
        WHERE bill_id IN (SELECT id FROM bills WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM bill_history_events WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM bill_items
        WHERE bill_id IN (SELECT id FROM bills WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM orders WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM bills WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM reservation_status_histories
        WHERE reservation_id IN (SELECT id FROM table_reservations WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM table_reservations WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM reservation_settings WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM staff_login_codes
        WHERE staff_id IN (SELECT id FROM staff WHERE business_id = ANY(v_demo_business_ids));
        DELETE FROM staff_invitations WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM rbac_audit_logs WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM staff WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM business_plugins WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM report_schedules WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM translations WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM business_currencies WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM business_languages WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM offers WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM bundles WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM menus WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM counters WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM tables WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM business_gallery_images WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM business_operating_hours WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM business_special_features WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM withdrawal_histories WHERE business_id = ANY(v_demo_business_ids);

        -- Milestone-email outbox + per-business revenue rollups + plugin
        -- and Telegram side-tables were added after the original seed was
        -- written and FK-reference businesses(id), so they must be cleared
        -- before the business row itself. To prevent another silent
        -- regression next time someone adds a business-scoped table, run
        -- the FK introspection query at the top of this file (or query
        -- information_schema.table_constraints) and confirm every
        -- referenced table has a corresponding DELETE here.
        DELETE FROM business_milestone_events WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM business_revenue_aggregates WHERE business_id = ANY(v_demo_business_ids);
        -- Per-delivery attempt rows must be removed before the parent
        -- delivery rows they reference, otherwise the FK guard in
        -- plugin_notification_delivery_attempts fires.
        DELETE FROM plugin_notification_delivery_attempts
        WHERE delivery_id IN (
            SELECT id FROM plugin_notification_deliveries
            WHERE business_id = ANY(v_demo_business_ids)
        );
        DELETE FROM plugin_notification_deliveries WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM telegram_connection_tokens WHERE business_id = ANY(v_demo_business_ids);
        DELETE FROM telegram_update_receipts WHERE business_id = ANY(v_demo_business_ids);

        DELETE FROM businesses WHERE id = ANY(v_demo_business_ids);
    END IF;

    IF cardinality(v_demo_customer_ids) > 0 THEN
        DELETE FROM customer_addresses WHERE customer_id = ANY(v_demo_customer_ids);
        DELETE FROM customer_preferences WHERE customer_id = ANY(v_demo_customer_ids);
        DELETE FROM customers WHERE id = ANY(v_demo_customer_ids);
    END IF;

    DELETE FROM page_views WHERE session_id LIKE 'pvseed-demo-%';
    DELETE FROM user_interactions WHERE session_id LIKE 'pvseed-demo-%';
    DELETE FROM conversion_events WHERE session_id LIKE 'pvseed-demo-%';
    DELETE FROM session_summaries WHERE session_id LIKE 'pvseed-demo-%';
    DELETE FROM error_logs WHERE request_id LIKE 'pvseed-demo-%';
    DELETE FROM webhook_events WHERE webhook_id LIKE 'demo_seed_%';
    DELETE FROM admin_actions WHERE details::text LIKE '%payverge-demo-seed-fixture%';

    IF to_regclass('public.user_sessions') IS NOT NULL THEN
        DELETE FROM user_sessions WHERE session_token LIKE 'pvseed-demo-revoked-%';
    END IF;

    IF to_regclass('public.scheduler_states') IS NOT NULL THEN
        DELETE FROM scheduler_states WHERE "key" LIKE 'pvseed-demo-%';
    END IF;

    IF to_regclass('public.file_access') IS NOT NULL THEN
        DELETE FROM file_access WHERE access_key LIKE 'pvseed-demo-%';
    END IF;

    UPDATE users SET
        address = CASE WHEN users.address IS NULL OR users.address = '' THEN v_wallet ELSE users.address END,
        name = CASE WHEN users.name IS NULL OR users.name = '' THEN 'Demo Owner' ELSE users.name END,
        email_verified = true,
        language_selected = COALESCE(NULLIF(users.language_selected, ''), 'en'),
        email_enabled = true,
        reports_enabled = true,
        statistics_enabled = true,
        updated_at = now()
    WHERE lower(users.email) = lower(v_owner_email)
    RETURNING id INTO v_owner_id;

    IF NOT FOUND THEN
        INSERT INTO users (
            email, address, name, username, picture, role, auth_method, email_verified,
            language_selected,
            email_enabled,
            news_enabled,
            updates_enabled,
            transactional_enabled,
            security_enabled,
            reports_enabled,
            statistics_enabled,
            created_at, updated_at
        )
        VALUES (
            v_owner_email, v_wallet, 'Demo Owner', 'demo-owner-payverge',
            'https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?w=256&h=256&fit=crop',
            'user', 'google', true, 'en',
            true, true, true, true, true, true, true,
            now(), now()
        )
        RETURNING id INTO v_owner_id;
    END IF;

    SELECT COALESCE(NULLIF(address, ''), v_wallet)
    INTO v_wallet
    FROM users
    WHERE id = v_owner_id;

    IF to_regclass('public.user_auths') IS NOT NULL THEN
        INSERT INTO user_auths (
            user_id, provider, wallet_address, email_verified, created_at, updated_at
        )
        SELECT v_owner_id, 'wallet', v_wallet, true, now(), now()
        WHERE NOT EXISTS (
            SELECT 1 FROM user_auths
            WHERE user_id = v_owner_id AND provider = 'wallet' AND wallet_address = v_wallet
        );
    END IF;

    INSERT INTO supported_currencies (code, name, symbol, is_active, created_at, updated_at) VALUES
        ('USD', 'US Dollar', '$', true, now(), now()),
        ('EUR', 'Euro', 'EUR', true, now(), now()),
        ('GBP', 'British Pound', 'GBP', true, now(), now()),
        ('ARS', 'Argentine Peso', 'ARS', true, now(), now()),
        ('AED', 'UAE Dirham', 'AED', true, now(), now()),
        ('SAR', 'Saudi Riyal', 'SAR', true, now(), now())
    ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, symbol = EXCLUDED.symbol, is_active = true, updated_at = now();

    INSERT INTO supported_languages (code, name, native_name, is_active, created_at, updated_at) VALUES
        ('en', 'English', 'English', true, now(), now()),
        ('es', 'Spanish', 'Espanol', true, now(), now()),
        ('pt', 'Portuguese', 'Portugues', true, now(), now()),
        ('ar', 'Arabic', 'Arabic', true, now(), now()),
        ('fr', 'French', 'Francais', true, now(), now()),
        ('de', 'German', 'Deutsch', true, now(), now()),
        ('zh', 'Chinese', 'Chinese', true, now(), now())
    ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, native_name = EXCLUDED.native_name, is_active = true, updated_at = now();

    INSERT INTO exchange_rates (from_currency, to_currency, rate, source, fetched_at, created_at, updated_at) VALUES
        ('USDC', 'USD', 1.0000, 'demo_seed', now(), now(), now()),
        ('USDC', 'EUR', 0.9200, 'demo_seed', now(), now(), now()),
        ('USDC', 'GBP', 0.7800, 'demo_seed', now(), now(), now()),
        ('USDC', 'ARS', 1085.0000, 'demo_seed', now(), now(), now()),
        ('USDC', 'AED', 3.6725, 'demo_seed', now(), now(), now()),
        ('USDC', 'SAR', 3.7500, 'demo_seed', now(), now(), now());

    INSERT INTO plugins (name, display_name, description, message, image, is_active, coming_soon, category, version, features, config_schema, created_at, updated_at) VALUES
        ('usdc_payment', 'USDC Payments', 'Native USDC settlement for guest bills.', 'Accept stablecoin payments directly from QR bills.', '/images/plugins/usdc.png', true, false, 'payment', '1.0.0', '["crypto checkout","settlement tracking"]', '{}', now(), now()),
        ('cross_chain_payment', 'Cross-Chain Payments', 'LI.FI-powered cross-chain bill payments.', 'Let guests pay from another supported chain while settling on Base.', '/images/plugins/cross-chain.png', true, false, 'payment', '1.0.0', '["route quotes","cross-chain settlement"]', '{}', now(), now()),
        ('stripe', 'Stripe Cards', 'Card payment processing for guest bills.', 'Accept card payments with Stripe test configuration.', '/images/plugins/stripe-logo.png', true, false, 'payment', '1.0.0', '["cards","webhooks","refund references"]', '{}', now(), now()),
        ('paypal', 'PayPal', 'PayPal guest payment integration.', 'Give guests a PayPal payment option during checkout.', '/images/plugins/paypal-logo.png', true, false, 'payment', '1.0.0', '["paypal checkout","return urls"]', '{}', now(), now()),
        ('mercadopago', 'MercadoPago', 'LatAm payment integration.', 'Support MercadoPago payment flows for local customers.', '/images/plugins/mercadopago-logo.png', true, false, 'payment', '1.0.0', '["wallet checkout","webhooks"]', '{}', now(), now()),
        ('telegram', 'Telegram Alerts', 'Operational notifications in Telegram.', 'Send order, payment, report, and inventory alerts to a private chat.', '/images/plugins/telegram-logo.png', true, false, 'integration', '1.0.0', '["order alerts","payment alerts","low stock alerts"]', '{}', now(), now()),
        ('trustpilot', 'Trustpilot Reviews', 'Review invitation automation.', 'Invite happy customers to leave public reviews after paid bills.', '/images/plugins/trustpilot-logo.png', true, false, 'marketing', '1.0.0', '["review invites","post-payment campaigns"]', '{}', now(), now()),
        ('daily_email_report', 'Daily Email Report', 'Daily operations digest.', 'Receive a daily summary of sales, tips, inventory, and live issues.', '/images/plugins/daily-report.png', true, false, 'reporting', '1.0.0', '["daily summary","scheduled email"]', '{}', now(), now()),
        ('weekly_email_report', 'Weekly Email Report', 'Weekly strategy report.', 'Receive weekly sales and operational trends.', '/images/plugins/weekly-report.png', true, false, 'reporting', '1.0.0', '["weekly summary","trend analysis"]', '{}', now(), now()),
        ('demo_disabled_plugin', 'Demo Disabled Plugin', 'Inactive plugin for admin toggle demos.', 'This plugin is intentionally inactive for admin state testing.', '', false, false, 'integration', '0.1.0', '["inactive state"]', '{}', now(), now())
    ON CONFLICT (name) DO UPDATE SET
        display_name = EXCLUDED.display_name,
        description = EXCLUDED.description,
        message = EXCLUDED.message,
        image = EXCLUDED.image,
        is_active = EXCLUDED.is_active,
        coming_soon = EXCLUDED.coming_soon,
        category = EXCLUDED.category,
        version = EXCLUDED.version,
        features = EXCLUDED.features,
        config_schema = EXCLUDED.config_schema,
        updated_at = now();

    DELETE FROM plugin_translations
    WHERE plugin_id IN (SELECT id FROM plugins WHERE name IN ('stripe','telegram','trustpilot','daily_email_report','weekly_email_report'))
      AND language_code IN ('es','ar');

    INSERT INTO plugin_translations (plugin_id, language_code, field_name, content, created_at, updated_at)
    SELECT id, 'es', 'message', 'Activa este plugin para probar pagos, alertas o reportes en la demo.', now(), now()
    FROM plugins WHERE name IN ('stripe','telegram','trustpilot','daily_email_report','weekly_email_report');

    INSERT INTO plugin_translations (plugin_id, language_code, field_name, content, created_at, updated_at)
    SELECT id, 'ar', 'message', 'Demo translated plugin message for Arabic admin views.', now(), now()
    FROM plugins WHERE name IN ('stripe','telegram','trustpilot');

    INSERT INTO businesses (
        business_id, owner_address, user_id, owner_name, name, logo,
        street, city, state, postal_code, country,
        settlement_addr, tipping_addr, tax_rate, service_fee_rate, tax_inclusive, service_inclusive, is_active,
        description, custom_url, phone, email, website, social_media, banner_images,
        business_page_enabled, show_reviews, google_reviews_enabled,
        google_place_id, google_business_name, google_review_link, google_business_url,
        timezone, counter_enabled, counter_count, counter_prefix, kitchen_enabled, orders_enabled, crm_enabled,
        default_currency, display_currency, default_language, source_language,
        design_primary_color, design_secondary_color, design_font_family, design_theme, design_menu_layout,
        design_show_images, design_show_descriptions, design_header_style, design_corner_radius,
        design_shadow_intensity, design_background_pattern, design_pattern_opacity,
        ai_ai_enabled, ai_ai_name, ai_ai_priority, ai_special_instructions, ai_business_page_ai_enabled,
        default_qr_logo_url, default_qr_foreground_color, default_qr_background_color, default_qr_logo_size,
        default_qr_show_business_name, default_qr_show_table_name, default_qr_text_font,
        latitude, longitude, welcome_message, about_story, show_welcome_message, show_about_story,
        show_gallery, show_operating_hours, show_special_features,
        created_at, updated_at
    )
    VALUES (
        'demo-core-business', v_wallet, v_owner_id, 'Demo Owner', 'Demo Core Kitchen',
        'https://images.unsplash.com/photo-1555396273-367ea4eb4db5?w=512&h=512&fit=crop',
        '214 West 14th Street', 'New York', 'NY', '10011', 'United States',
        v_wallet, v_wallet, 8.875, 4.0, false, false, true,
        'A fast-casual neighborhood kitchen in Manhattan serving global bowls, all-day brunch, and weekend pastries.',
        'demo-core-kitchen', '+1 212 555 0144', 'hello@core-kitchen.example.com', 'https://demo.payverge.example/demo-core-kitchen',
        '{"instagram":"https://instagram.com/democorekitchen","tiktok":"https://tiktok.com/@democorekitchen","facebook":"https://facebook.com/democorekitchen","google_maps":"https://maps.google.com/?q=Demo+Core+Kitchen"}',
        '["https://images.unsplash.com/photo-1552566626-52f8b828add9?w=1600&h=900&fit=crop","https://images.unsplash.com/photo-1559339352-11d035aa65de?w=1600&h=900&fit=crop"]',
        true, true, true,
        -- Google review/maps URLs are intentionally left empty: the demo
        -- businesses aren't real and the previous values pointed at
        -- non-existent g.page/maps endpoints, sending demo viewers to
        -- a 404. The frontend gates the "Write a review" CTA on a
        -- truthy google_review_link, so clearing the field hides the
        -- broken button without disabling the feature flag.
        '', 'Demo Core Kitchen', '', '',
        'America/New_York', true, 3, 'C', true, true, true,
        'USD', 'USD', 'en', 'en',
        '#0f766e', '#eab308', 'Inter', 'light', 'grid',
        true, true, 'banner', 'medium', 'subtle', 'none', 0.08,
        false, 'Sage', 'service', 'This demo venue keeps AI surfaces disabled.', false,
        'https://images.unsplash.com/photo-1517248135467-4c7edcad34c4?w=256&q=75', '#0f766e', '#ffffff', 22, true, true, 'Inter',
        40.73820000, -73.99990000,
        'Welcome to Demo Core Kitchen. Scan, order, split, and enjoy.',
        'A modern neighborhood kitchen built around fast service, transparent payments, local sourcing, and strong daily operations.',
        true, true, true, true, true,
        now(), now()
    )
    ON CONFLICT (business_id) DO UPDATE SET
        user_id = EXCLUDED.user_id,
        owner_address = EXCLUDED.owner_address,
        name = EXCLUDED.name,
        custom_url = EXCLUDED.custom_url,
        business_page_enabled = EXCLUDED.business_page_enabled,
        updated_at = now()
    RETURNING id INTO v_core_id;

    INSERT INTO businesses (
        business_id, owner_address, user_id, owner_name, name, logo,
        street, city, state, postal_code, country,
        settlement_addr, tipping_addr, tax_rate, service_fee_rate, tax_inclusive, service_inclusive, is_active,
        description, custom_url, phone, email, website, social_media, banner_images,
        business_page_enabled, show_reviews, google_reviews_enabled,
        google_place_id, google_business_name, google_review_link, google_business_url,
        timezone, counter_enabled, counter_count, counter_prefix, kitchen_enabled, orders_enabled, crm_enabled,
        default_currency, display_currency, default_language, source_language,
        design_primary_color, design_secondary_color, design_font_family, design_theme, design_menu_layout,
        design_show_images, design_show_descriptions, design_header_style, design_corner_radius,
        design_shadow_intensity, design_background_pattern, design_pattern_opacity,
        ai_ai_enabled, ai_ai_name, ai_ai_priority, ai_special_instructions, ai_business_page_ai_enabled,
        default_qr_logo_url, default_qr_foreground_color, default_qr_background_color, default_qr_logo_size,
        default_qr_show_business_name, default_qr_show_table_name, default_qr_text_font,
        latitude, longitude, welcome_message, about_story, show_welcome_message, show_about_story,
        show_gallery, show_operating_hours, show_special_features,
        created_at, updated_at
    )
    VALUES (
        'demo-ai-pro-business', v_wallet, v_owner_id, 'Demo Owner', 'Demo AI Lounge',
        'https://images.unsplash.com/photo-1514933651103-005eec06c04b?w=512&h=512&fit=crop',
        'Gate Village 7, DIFC', 'Dubai', 'Dubai', '00000', 'United Arab Emirates',
        v_wallet, v_wallet, 5.0, 7.5, false, false, true,
        'A modern DIFC lounge where seasonal tasting menus, signature mocktails, and a multilingual AI concierge make every visit feel effortless.',
        'demo-ai-lounge', '+1 212 555 0199', 'concierge@ai-lounge.example.com', 'https://demo.payverge.example/demo-ai-lounge',
        '{"instagram":"https://instagram.com/demoailounge","tiktok":"https://tiktok.com/@demoailounge","facebook":"https://facebook.com/demoailounge","google_maps":"https://maps.google.com/?q=Demo+AI+Lounge"}',
        '["https://images.unsplash.com/photo-1517248135467-4c7edcad34c4?w=1600&h=900&fit=crop","https://images.unsplash.com/photo-1504674900247-0877df9cc836?w=1600&h=900&fit=crop"]',
        true, true, true,
        '', 'Demo AI Lounge', '', '',
        'Asia/Dubai', true, 4, 'A', true, true, true,
        'AED', 'AED', 'en', 'en',
        '#064e3b', '#f59e0b', 'Inter', 'light', 'grid',
        true, true, 'banner', 'small', 'subtle', 'none', 0.05,
        true, 'Sofia', 'balanced',
        'Be concise, multilingual, careful with allergens, proactive with high-margin pairings, and escalate VIP or allergy concerns to staff.',
        true,
        'https://images.unsplash.com/photo-1546069901-d5bfd2cbfb1f?w=256&q=75', '#064e3b', '#ffffff', 24, true, true, 'Inter',
        25.21170000, 55.27950000,
        'Welcome to Demo AI Lounge. Sofia can help with dining, reservations, delivery, and recommendations.',
        'A premium hospitality venue designed for tourist-heavy, multilingual service and owner-level operational intelligence.',
        true, true, true, true, true,
        now(), now()
    )
    ON CONFLICT (business_id) DO UPDATE SET
        user_id = EXCLUDED.user_id,
        owner_address = EXCLUDED.owner_address,
        name = EXCLUDED.name,
        custom_url = EXCLUDED.custom_url,
        business_page_enabled = EXCLUDED.business_page_enabled,
        updated_at = now()
    RETURNING id INTO v_ai_id;

    UPDATE businesses
    SET is_demo = true
    WHERE id IN (v_core_id, v_ai_id);

    INSERT INTO business_currencies (business_id, currency_code, is_preferred, display_order, created_at, updated_at) VALUES
        (v_core_id, 'USD', true, 0, now(), now()),
        (v_core_id, 'EUR', false, 1, now(), now()),
        (v_core_id, 'ARS', false, 2, now(), now()),
        (v_core_id, 'AED', false, 3, now(), now()),
        (v_ai_id, 'AED', true, 0, now(), now()),
        (v_ai_id, 'USD', false, 1, now(), now()),
        (v_ai_id, 'EUR', false, 2, now(), now()),
        (v_ai_id, 'SAR', false, 3, now(), now());

    INSERT INTO business_languages (business_id, language_code, is_default, display_order, created_at, updated_at) VALUES
        (v_core_id, 'en', true, 0, now(), now()),
        (v_core_id, 'es', false, 1, now(), now()),
        (v_core_id, 'pt', false, 2, now(), now()),
        (v_core_id, 'ar', false, 3, now(), now()),
        (v_ai_id, 'en', true, 0, now(), now()),
        (v_ai_id, 'ar', false, 1, now(), now()),
        (v_ai_id, 'es', false, 2, now(), now()),
        (v_ai_id, 'fr', false, 3, now(), now()),
        (v_ai_id, 'de', false, 4, now(), now()),
        (v_ai_id, 'zh', false, 5, now(), now());

    INSERT INTO business_gallery_images (business_id, image_url, caption, display_order, is_active, created_at, updated_at)
    SELECT v_core_id, image_url, caption, ord, true, now(), now()
    FROM unnest(ARRAY[
        'https://images.unsplash.com/photo-1514933651103-005eec06c04b?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1521017432531-fbd92d768814?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1498654896293-37aacf113fd9?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1551218808-94e220e084d2?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1528605248644-14dd04022da1?w=1200&h=800&fit=crop'
    ], ARRAY[
        'Main dining room ready for QR ordering',
        'Pickup counter and coffee service',
        'Local ingredient prep station',
        'Family table service',
        'Outdoor seating for lunch'
    ]) WITH ORDINALITY AS t(image_url, caption, ord);

    INSERT INTO business_gallery_images (business_id, image_url, caption, display_order, is_active, created_at, updated_at)
    SELECT v_ai_id, image_url, caption, ord, true, now(), now()
    FROM unnest(ARRAY[
        'https://images.unsplash.com/photo-1517248135467-4c7edcad34c4?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1552566626-52f8b828add9?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1544148103-0773bf10d330?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1559339352-11d035aa65de?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1470337458703-46ad1756a187?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1504674900247-0877df9cc836?w=1200&h=800&fit=crop',
        'https://images.unsplash.com/photo-1550966871-3ed3cdb5ed0c?w=1200&h=800&fit=crop'
    ], ARRAY[
        'AI concierge lounge entrance',
        'Private dining room',
        'Chef tasting counter',
        'Late-night terrace',
        'Premium mocktail bar',
        'Delivery presentation box',
        'VIP service detail'
    ]) WITH ORDINALITY AS t(image_url, caption, ord);

    FOR v_i IN 0..6 LOOP
        INSERT INTO business_operating_hours (business_id, day_of_week, open_time, close_time, is_closed, created_at, updated_at)
        VALUES (
            v_core_id, v_i,
            CASE WHEN v_i = 0 THEN '00:00' WHEN v_i = 6 THEN '10:00' ELSE '08:00' END,
            CASE WHEN v_i = 0 THEN '00:00' WHEN v_i = 5 THEN '23:00' WHEN v_i = 6 THEN '22:00' ELSE '21:00' END,
            v_i = 0,
            now(), now()
        );

        INSERT INTO business_operating_hours (business_id, day_of_week, open_time, close_time, is_closed, created_at, updated_at)
        VALUES (
            v_ai_id, v_i,
            CASE WHEN v_i = 0 THEN '12:00' ELSE '11:30' END,
            CASE WHEN v_i IN (5,6) THEN '02:00' ELSE '00:00' END,
            false,
            now(), now()
        );
    END LOOP;

    INSERT INTO business_special_features (business_id, title, description, icon, display_order, is_active, created_at, updated_at)
    SELECT v_core_id, title, description, icon, ord, true, now(), now()
    FROM (VALUES
        ('Fast table ordering', 'Guests scan, order, split, and pay from the table.', 'qr-code', 1),
        ('Outdoor seating', 'Patio tables for lunch and weekend traffic.', 'sun', 2),
        ('Local ingredients', 'Seasonal suppliers represented in menu and inventory.', 'leaf', 3),
        ('Family friendly', 'Kids and side options for family visits.', 'users', 4),
        ('Delivery available', 'In-house and third-party delivery flows are enabled.', 'truck', 5),
        ('Loyalty rewards', 'Earn points on every visit and unlock perks tailored to you.', 'star', 6)
    ) AS f(title, description, icon, ord);

    INSERT INTO business_special_features (business_id, title, description, icon, display_order, is_active, created_at, updated_at)
    SELECT v_ai_id, title, description, icon, ord, true, now(), now()
    FROM (VALUES
        ('AI concierge', 'Sofia handles multilingual questions, recommendations, and staff handoff.', 'sparkles', 1),
        ('Private dining', 'VIP room and corporate bookings for high-value guests.', 'lock', 2),
        ('Multilingual staff', 'English, Arabic, Spanish, French, German, and Chinese service flows.', 'languages', 3),
        ('Chef tasting menu', 'Premium tasting bundles and upsell-ready pairings.', 'utensils', 4),
        ('Valet coordination', 'Concierge notes and delivery metadata support guest logistics.', 'car', 5),
        ('Premium mocktail bar', 'High-margin signature drinks for AI upsell testing.', 'glass-water', 6),
        ('Event hosting', 'Reservation and accounting data include event deposits.', 'calendar', 7)
    ) AS f(title, description, icon, ord);

    CREATE TEMP TABLE demo_seed_menu_items (
        business_key text,
        category_id text,
        category_name text,
        category_description text,
        category_sort int,
        item_id text,
        item_name text,
        item_description text,
        price numeric,
        currency text,
        image text,
        is_available boolean,
        allergens jsonb,
        dietary_tags jsonb,
        options jsonb,
        item_sort int
    ) ON COMMIT DROP;

    INSERT INTO demo_seed_menu_items VALUES
        ('core','core-cat-starters','Starters and Shareables','Small plates for tables and counters.',1,'core-starter-001','Citrus Avocado Toast','Sourdough, avocado, citrus, seeds.',10.50,'USD','https://images.unsplash.com/photo-1525351484163-7529414344d8?w=800&h=600&fit=crop',true,'["gluten","sesame"]','["vegetarian"]','[{"id":"core-opt-gluten-free","name":"Gluten-free toast","price_change":1.5,"is_required":false}]',1),
        ('core','core-cat-starters','Starters and Shareables','Small plates for tables and counters.',1,'core-starter-002','Smoky Hummus Plate','Chickpea hummus, paprika oil, warm pita.',9.75,'USD','https://images.unsplash.com/photo-1541518763669-27fef04b14ea?w=800&h=600&fit=crop',true,'["gluten","sesame"]','["vegan"]','[]',2),
        ('core','core-cat-starters','Starters and Shareables','Small plates for tables and counters.',1,'core-starter-003','Crispy Cauliflower Bites','Spiced cauliflower with tahini dip.',11.25,'USD','https://images.unsplash.com/photo-1604908176997-125f25cc6f3d?w=800&h=600&fit=crop',true,'["sesame"]','["vegan"]','[{"id":"core-opt-extra-dip","name":"Extra tahini dip","price_change":1,"is_required":false}]',3),
        ('core','core-cat-starters','Starters and Shareables','Small plates for tables and counters.',1,'core-starter-004','Shrimp Corn Cups','Chilled shrimp, corn, lime, herbs.',13.50,'USD','https://images.unsplash.com/photo-1565299585323-38d6b0865b47?w=800&h=600&fit=crop',true,'["crustaceans"]','[]','[]',4),
        ('core','core-cat-starters','Starters and Shareables','Small plates for tables and counters.',1,'core-starter-005','Tomato Burrata Salad','Tomato, burrata, basil, balsamic.',12.95,'USD','https://images.unsplash.com/photo-1512621776951-a57141f2eefd?w=800&h=600&fit=crop',true,'["dairy"]','["vegetarian"]','[]',5),
        ('core','core-cat-starters','Starters and Shareables','Small plates for tables and counters.',1,'core-starter-006','Beef Empanadas','House-baked, slow-cooked beef with chimichurri.',8.50,'USD','https://images.unsplash.com/photo-1625943553852-781c6dd46faa?w=800&h=600&fit=crop',false,'["gluten","eggs"]','[]','[]',6),
        ('core','core-cat-bowls','Bowls and Mains','Lunch and dinner anchors.',2,'core-main-001','Lemon Chicken Bowl','Chicken, rice, greens, lemon yogurt.',17.50,'USD','https://images.unsplash.com/photo-1546069901-ba9599a7e63c?w=800&h=600&fit=crop',true,'["dairy"]','[]','[{"id":"core-opt-rice","name":"Base choice","price_change":0,"is_required":true},{"id":"core-opt-extra-chicken","name":"Extra chicken","price_change":4,"is_required":false}]',1),
        ('core','core-cat-bowls','Bowls and Mains','Lunch and dinner anchors.',2,'core-main-002','Falafel Power Bowl','Falafel, quinoa, pickles, tahini.',16.25,'USD','https://images.unsplash.com/photo-1512058564366-18510be2db19?w=800&h=600&fit=crop',true,'["sesame"]','["vegan"]','[]',2),
        ('core','core-cat-bowls','Bowls and Mains','Lunch and dinner anchors.',2,'core-main-003','Seared Salmon Bowl','Salmon, greens, quinoa, citrus glaze.',22.00,'USD','https://images.unsplash.com/photo-1467003909585-2f8a72700288?w=800&h=600&fit=crop',true,'["soya"]','["gluten-free"]','[{"id":"core-opt-sauce","name":"Sauce on side","price_change":0,"is_required":false}]',3),
        ('core','core-cat-bowls','Bowls and Mains','Lunch and dinner anchors.',2,'core-main-004','Spicy Beef Rice Bowl','Beef, rice, chili crisp, herbs.',18.75,'USD','https://images.unsplash.com/photo-1604909052743-94e838986d24?w=800&h=600&fit=crop',true,'["soya","sesame"]','[]','[]',4),
        ('core','core-cat-bowls','Bowls and Mains','Lunch and dinner anchors.',2,'core-main-005','Mushroom Grain Bowl','Roasted mushrooms, farro, kale.',15.50,'USD','https://images.unsplash.com/photo-1473093295043-cdd812d0e601?w=800&h=600&fit=crop',true,'["gluten"]','["vegetarian"]','[]',5),
        ('core','core-cat-bowls','Bowls and Mains','Lunch and dinner anchors.',2,'core-main-006','Braised Short Rib Bowl','Low-stock demo item.',21.25,'USD','https://images.unsplash.com/photo-1544025162-d76694265947?w=800&h=600&fit=crop',false,'["soya"]','[]','[]',6),
        ('core','core-cat-sandwiches','Sandwiches and Handhelds','Quick service favorites.',3,'core-handheld-001','Turkey Pesto Melt','Turkey, pesto, provolone, sourdough.',14.75,'USD','https://images.unsplash.com/photo-1528735602780-2552fd46c7af?w=800&h=600&fit=crop',true,'["gluten","dairy","treenuts"]','[]','[]',1),
        ('core','core-cat-sandwiches','Sandwiches and Handhelds','Quick service favorites.',3,'core-handheld-002','Crispy Fish Sandwich','Fish, slaw, pickles, brioche.',15.95,'USD','https://images.unsplash.com/photo-1550547660-d9450f859349?w=800&h=600&fit=crop',true,'["gluten","eggs"]','[]','[]',2),
        ('core','core-cat-sandwiches','Sandwiches and Handhelds','Quick service favorites.',3,'core-handheld-003','Vegan Mushroom Wrap','Mushroom, greens, tahini, tortilla.',13.25,'USD','https://images.unsplash.com/photo-1626700051175-6818013e1d4f?w=800&h=600&fit=crop',true,'["gluten","sesame"]','["vegan"]','[]',3),
        ('core','core-cat-sandwiches','Sandwiches and Handhelds','Quick service favorites.',3,'core-handheld-004','Classic Smash Burger','Beef, cheddar, pickles, sauce.',16.50,'USD','https://images.unsplash.com/photo-1568901346375-23c9450c58cd?w=800&h=600&fit=crop',true,'["gluten","dairy","eggs"]','[]','[{"id":"core-opt-burger-temp","name":"Burger temperature","price_change":0,"is_required":true}]',4),
        ('core','core-cat-sandwiches','Sandwiches and Handhelds','Quick service favorites.',3,'core-handheld-005','Grilled Halloumi Pita','Halloumi, cucumber, tomato, herbs.',13.95,'USD','https://images.unsplash.com/photo-1606755962773-d324e7a7a7ac?w=800&h=600&fit=crop',true,'["gluten","dairy"]','["vegetarian"]','[]',5),
        ('core','core-cat-sandwiches','Sandwiches and Handhelds','Quick service favorites.',3,'core-handheld-006','Sold Out Chicken Arepa','Out-of-stock demo handheld.',12.50,'USD','',false,'["dairy"]','["gluten-free"]','[]',6),
        ('core','core-cat-drinks','Coffee and Cold Drinks','Cafe drinks and low-sugar options.',4,'core-drink-001','Cold Brew Latte','Cold brew, milk, light syrup.',6.25,'USD','https://images.unsplash.com/photo-1461023058943-07fcbe16d735?w=800&h=600&fit=crop',true,'["dairy"]','[]','[{"id":"core-opt-milk","name":"Milk choice","price_change":0,"is_required":true}]',1),
        ('core','core-cat-drinks','Coffee and Cold Drinks','Cafe drinks and low-sugar options.',4,'core-drink-002','Mint Lemonade','Fresh lemon, mint, sparkling water.',5.75,'USD','https://images.unsplash.com/photo-1497534446932-c925b458314e?w=800&h=600&fit=crop',true,'[]','["vegan"]','[]',2),
        ('core','core-cat-drinks','Coffee and Cold Drinks','Cafe drinks and low-sugar options.',4,'core-drink-003','Iced Matcha','Matcha, oat milk, vanilla.',6.95,'USD','https://images.unsplash.com/photo-1515823064-d6e0c04616a7?w=800&h=600&fit=crop',true,'[]','["vegetarian"]','[{"id":"core-opt-oat","name":"Oat milk","price_change":0.75,"is_required":false}]',3),
        ('core','core-cat-drinks','Coffee and Cold Drinks','Cafe drinks and low-sugar options.',4,'core-drink-004','Sparkling Water','Local sparkling mineral water.',3.50,'USD','',true,'[]','["vegan","gluten-free"]','[]',4),
        ('core','core-cat-drinks','Coffee and Cold Drinks','Cafe drinks and low-sugar options.',4,'core-drink-005','House Kombucha','Passion fruit kombucha.',6.50,'USD','https://images.unsplash.com/photo-1605270012917-bf157c5a9541?w=800&h=600&fit=crop',true,'[]','["vegan"]','[]',5),
        ('core','core-cat-drinks','Coffee and Cold Drinks','Cafe drinks and low-sugar options.',4,'core-drink-006','Seasonal Smoothie','Unavailable seasonal smoothie.',7.50,'USD','',false,'["treenuts"]','["vegetarian"]','[]',6),
        ('core','core-cat-desserts','Desserts','Sweet finishers and cafe pairings.',5,'core-dessert-001','Chocolate Lava Cake','Warm chocolate cake, vanilla cream.',8.95,'USD','https://images.unsplash.com/photo-1606313564200-e75d5e30476c?w=800&h=600&fit=crop',true,'["gluten","dairy","eggs"]','["vegetarian"]','[]',1),
        ('core','core-cat-desserts','Desserts','Sweet finishers and cafe pairings.',5,'core-dessert-002','Coconut Lime Cheesecake','Creamy cheesecake, toasted coconut.',8.50,'USD','https://images.unsplash.com/photo-1533134242443-d4fd215305ad?w=800&h=600&fit=crop',true,'["dairy","eggs"]','["vegetarian"]','[]',2),
        ('core','core-cat-desserts','Desserts','Sweet finishers and cafe pairings.',5,'core-dessert-003','Mango Passion Tart','Fruit tart with passion glaze.',7.95,'USD','https://images.unsplash.com/photo-1488477181946-6428a0291777?w=800&h=600&fit=crop',true,'["gluten","dairy","eggs"]','["vegetarian"]','[]',3),
        ('core','core-cat-desserts','Desserts','Sweet finishers and cafe pairings.',5,'core-dessert-004','Berry Chia Cup','Chia pudding, berries, almond.',6.95,'USD','https://images.unsplash.com/photo-1488477304112-4944851de03d?w=800&h=600&fit=crop',true,'["treenuts"]','["vegan","gluten-free"]','[]',4),
        ('core','core-cat-desserts','Desserts','Sweet finishers and cafe pairings.',5,'core-dessert-005','Espresso Mousse','Coffee mousse with cacao.',7.25,'USD','',true,'["dairy","eggs"]','["vegetarian"]','[]',5),
        ('core','core-cat-desserts','Desserts','Sweet finishers and cafe pairings.',5,'core-dessert-006','Almond Cardamom Cookie','Out-of-stock allergen demo.',4.25,'USD','',false,'["treenuts","eggs"]','["vegetarian"]','[]',6),
        ('core','core-cat-kids','Kids and Sides','Family-friendly add-ons.',6,'core-side-001','Kids Chicken Bites','Small chicken bites with fries.',9.50,'USD','https://images.unsplash.com/photo-1562967916-eb82221dfb36?w=800&h=600&fit=crop',true,'["gluten","eggs"]','[]','[]',1),
        ('core','core-cat-kids','Kids and Sides','Family-friendly add-ons.',6,'core-side-002','Crispy Fries','Sea salt fries.',5.25,'USD','https://images.unsplash.com/photo-1573080496219-bb080dd4f877?w=800&h=600&fit=crop',true,'[]','["vegan","gluten-free"]','[]',2),
        ('core','core-cat-kids','Kids and Sides','Family-friendly add-ons.',6,'core-side-003','Side Greens','Small mixed greens.',4.75,'USD','',true,'[]','["vegan","gluten-free"]','[]',3),
        ('core','core-cat-kids','Kids and Sides','Family-friendly add-ons.',6,'core-side-004','Mac and Cheese Cup','Creamy pasta cup.',6.95,'USD','https://images.unsplash.com/photo-1543339494-b4cd4f7ba686?w=800&h=600&fit=crop',true,'["gluten","dairy"]','["vegetarian"]','[]',4),
        ('core','core-cat-kids','Kids and Sides','Family-friendly add-ons.',6,'core-side-005','Fruit Cup','Seasonal fruit.',4.95,'USD','',true,'[]','["vegan","gluten-free"]','[]',5),
        ('core','core-cat-kids','Kids and Sides','Family-friendly add-ons.',6,'core-side-006','Tomato Basil Soup Cup','Sold out soup demo.',5.95,'USD','',false,'["dairy"]','["vegetarian"]','[]',6);

    INSERT INTO demo_seed_menu_items
    SELECT 'ai', category_id, category_name, category_description, category_sort, item_id, item_name, item_description, price, 'AED', image, is_available, allergens, dietary_tags, options, item_sort
    FROM (VALUES
        ('ai-cat-tasting','Chef Tasting','Premium tasting flights for high-value guests.',1,'ai-taste-001','Saffron Garden Amuse','Saffron vegetable bite with herb foam.',42,'https://images.unsplash.com/photo-1551218808-94e220e084d2?w=800&h=600&fit=crop',true,'["dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,1),
        ('ai-cat-tasting','Chef Tasting','Premium tasting flights for high-value guests.',1,'ai-taste-002','Date Glazed Duck Bite','Duck, date glaze, pickled onion.',68,'https://images.unsplash.com/photo-1504674900247-0877df9cc836?w=800&h=600&fit=crop',true,'["soya"]'::jsonb,'[]'::jsonb,'[]'::jsonb,2),
        ('ai-cat-tasting','Chef Tasting','Premium tasting flights for high-value guests.',1,'ai-taste-003','Truffle Labneh Tart','Truffle, labneh, crisp shell.',56,'https://images.unsplash.com/photo-1485963631004-f2f00b1d6606?w=800&h=600&fit=crop',true,'["gluten","dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,3),
        ('ai-cat-tasting','Chef Tasting','Premium tasting flights for high-value guests.',1,'ai-taste-004','Caviar Potato Crisp','Potato crisp with caviar cream.',92,'https://images.unsplash.com/photo-1540189549336-e6e99c3679fe?w=800&h=600&fit=crop',true,'["dairy","eggs"]'::jsonb,'[]'::jsonb,'[]'::jsonb,4),
        ('ai-cat-tasting','Chef Tasting','Premium tasting flights for high-value guests.',1,'ai-taste-005','Mushroom Broth Cup','Roasted mushroom broth.',38,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,5),
        ('ai-cat-tasting','Chef Tasting','Premium tasting flights for high-value guests.',1,'ai-taste-006','Wagyu Nigiri','Limited item unavailable today.',120,'',false,'["soya"]'::jsonb,'[]'::jsonb,'[]'::jsonb,6),
        ('ai-cat-starters','Starters','Premium shareable starters.',2,'ai-starter-001','Gold Tomato Burrata','Heirloom tomato, burrata, basil oil.',64,'https://images.unsplash.com/photo-1512621776951-a57141f2eefd?w=800&h=600&fit=crop',true,'["dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,1),
        ('ai-cat-starters','Starters','Premium shareable starters.',2,'ai-starter-002','Zaartar Prawns','Grilled prawns, zaatar, lemon.',82,'https://images.unsplash.com/photo-1565299585323-38d6b0865b47?w=800&h=600&fit=crop',true,'["crustaceans"]'::jsonb,'[]'::jsonb,'[]'::jsonb,2),
        ('ai-cat-starters','Starters','Premium shareable starters.',2,'ai-starter-003','Spiced Lentil Kibbeh','Plant-forward kibbeh with tahini.',48,'',true,'["sesame","gluten"]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,3),
        ('ai-cat-starters','Starters','Premium shareable starters.',2,'ai-starter-004','Seared Halloumi Fig','Halloumi, figs, pomegranate.',58,'',true,'["dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,4),
        ('ai-cat-starters','Starters','Premium shareable starters.',2,'ai-starter-005','Tuna Citrus Crudo','Tuna, citrus, chili oil.',88,'https://images.unsplash.com/photo-1579584425555-c3ce17fd4351?w=800&h=600&fit=crop',true,'["soya","sesame"]'::jsonb,'[]'::jsonb,'[]'::jsonb,5),
        ('ai-cat-starters','Starters','Premium shareable starters.',2,'ai-starter-006','Foie Gras Bite','Out-of-stock premium starter.',96,'',false,'["dairy"]'::jsonb,'[]'::jsonb,'[]'::jsonb,6),
        ('ai-cat-mains','Premium Mains','High-value dinner anchors.',3,'ai-main-001','Wagyu Striploin','Wagyu, smoked salt, jus.',280,'https://images.unsplash.com/photo-1546833999-b9f581a1996d?w=800&h=600&fit=crop',true,'["dairy"]'::jsonb,'[]'::jsonb,'[{"id":"ai-opt-temp","name":"Temperature","price_change":0,"is_required":true}]'::jsonb,1),
        ('ai-cat-mains','Premium Mains','High-value dinner anchors.',3,'ai-main-002','Saffron Sea Bass','Sea bass, saffron beurre blanc.',210,'https://images.unsplash.com/photo-1467003909585-2f8a72700288?w=800&h=600&fit=crop',true,'["dairy"]'::jsonb,'["gluten-free"]'::jsonb,'[]'::jsonb,2),
        ('ai-cat-mains','Premium Mains','High-value dinner anchors.',3,'ai-main-003','Lamb Shoulder Majlis','Slow lamb, dates, herbs.',190,'https://images.unsplash.com/photo-1544025162-d76694265947?w=800&h=600&fit=crop',true,'[]'::jsonb,'[]'::jsonb,'[]'::jsonb,3),
        ('ai-cat-mains','Premium Mains','High-value dinner anchors.',3,'ai-main-004','Charred Cauliflower Steak','Cauliflower, tahini, chili crisp.',92,'',true,'["sesame"]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,4),
        ('ai-cat-mains','Premium Mains','High-value dinner anchors.',3,'ai-main-005','Black Garlic Chicken','Chicken, black garlic, greens.',135,'',true,'["soya"]'::jsonb,'[]'::jsonb,'[]'::jsonb,5),
        ('ai-cat-mains','Premium Mains','High-value dinner anchors.',3,'ai-main-006','Lobster Tagliatelle','Limited lobster pasta unavailable.',165,'',false,'["crustaceans","gluten","dairy"]'::jsonb,'[]'::jsonb,'[]'::jsonb,6),
        ('ai-cat-plant','Plant-Forward Plates','Vegetarian and vegan premium plates.',4,'ai-plant-001','Truffle Mushroom Risotto','Mushroom, truffle, arborio.',118,'https://images.unsplash.com/photo-1476124369491-e7addf5db371?w=800&h=600&fit=crop',true,'["dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,1),
        ('ai-cat-plant','Plant-Forward Plates','Vegetarian and vegan premium plates.',4,'ai-plant-002','Emirati Grain Garden','Ancient grains, herbs, citrus.',76,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,2),
        ('ai-cat-plant','Plant-Forward Plates','Vegetarian and vegan premium plates.',4,'ai-plant-003','Eggplant Ember Plate','Charred eggplant, tahini, herbs.',72,'',true,'["sesame"]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,3),
        ('ai-cat-plant','Plant-Forward Plates','Vegetarian and vegan premium plates.',4,'ai-plant-004','Green Pea Ravioli','Pea ravioli, mint, lemon.',86,'',true,'["gluten","dairy","eggs"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,4),
        ('ai-cat-plant','Plant-Forward Plates','Vegetarian and vegan premium plates.',4,'ai-plant-005','Spiced Chickpea Tagine','Chickpeas, tomato, preserved lemon.',68,'',true,'[]'::jsonb,'["vegan","gluten-free"]'::jsonb,'[]'::jsonb,5),
        ('ai-cat-plant','Plant-Forward Plates','Vegetarian and vegan premium plates.',4,'ai-plant-006','Vegan Tasting Plate','Unavailable plant tasting menu.',132,'',false,'["treenuts"]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,6),
        ('ai-cat-desserts','Desserts','Premium dessert pairings.',5,'ai-dessert-001','Rose Milk Cake','Rose, pistachio, sponge.',54,'https://images.unsplash.com/photo-1488477181946-6428a0291777?w=800&h=600&fit=crop',true,'["gluten","dairy","treenuts","eggs"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,1),
        ('ai-cat-desserts','Desserts','Premium dessert pairings.',5,'ai-dessert-002','Cardamom Chocolate Dome','Dark chocolate and cardamom.',62,'https://images.unsplash.com/photo-1606313564200-e75d5e30476c?w=800&h=600&fit=crop',true,'["dairy","eggs"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,2),
        ('ai-cat-desserts','Desserts','Premium dessert pairings.',5,'ai-dessert-003','Mango Saffron Tart','Mango, saffron cream, tart shell.',58,'',true,'["gluten","dairy","eggs"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,3),
        ('ai-cat-desserts','Desserts','Premium dessert pairings.',5,'ai-dessert-004','Date Pudding','Dates, caramel, vanilla.',52,'',true,'["gluten","dairy","eggs"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,4),
        ('ai-cat-desserts','Desserts','Premium dessert pairings.',5,'ai-dessert-005','Coconut Sorbet','Coconut sorbet, lime zest.',42,'',true,'[]'::jsonb,'["vegan","gluten-free"]'::jsonb,'[]'::jsonb,5),
        ('ai-cat-desserts','Desserts','Premium dessert pairings.',5,'ai-dessert-006','Pistachio Opera Cake','Sold-out dessert demo.',66,'',false,'["gluten","dairy","treenuts","eggs"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,6),
        ('ai-cat-mocktails','Signature Mocktails','High-margin non-alcoholic signatures.',6,'ai-mocktail-001','Pomegranate Rosemary Fizz','Pomegranate, rosemary, bubbles.',48,'https://images.unsplash.com/photo-1544145945-f90425340c7e?w=800&h=600&fit=crop',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,1),
        ('ai-cat-mocktails','Signature Mocktails','High-margin non-alcoholic signatures.',6,'ai-mocktail-002','Saffron Citrus Cooler','Saffron, citrus, tonic.',52,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,2),
        ('ai-cat-mocktails','Signature Mocktails','High-margin non-alcoholic signatures.',6,'ai-mocktail-003','Mint Cucumber Pearl','Mint, cucumber, lime.',44,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,3),
        ('ai-cat-mocktails','Signature Mocktails','High-margin non-alcoholic signatures.',6,'ai-mocktail-004','Black Tea Peach Spritz','Tea, peach, bubbles.',46,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,4),
        ('ai-cat-mocktails','Signature Mocktails','High-margin non-alcoholic signatures.',6,'ai-mocktail-005','Smoked Date Tonic','Date syrup, tonic, smoke.',58,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,5),
        ('ai-cat-mocktails','Signature Mocktails','High-margin non-alcoholic signatures.',6,'ai-mocktail-006','Tamarind Spark Mocktail','Unavailable mocktail item.',48,'',false,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,6),
        ('ai-cat-coffee','Coffee and Tea','Late service coffee and tea.',7,'ai-coffee-001','Arabic Coffee Service','Arabic coffee with dates.',38,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,1),
        ('ai-cat-coffee','Coffee and Tea','Late service coffee and tea.',7,'ai-coffee-002','Cardamom Latte','Espresso, cardamom milk.',36,'https://images.unsplash.com/photo-1461023058943-07fcbe16d735?w=800&h=600&fit=crop',true,'["dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[{"id":"ai-opt-milk","name":"Milk choice","price_change":0,"is_required":true}]'::jsonb,2),
        ('ai-cat-coffee','Coffee and Tea','Late service coffee and tea.',7,'ai-coffee-003','Jasmine Green Tea','Jasmine tea service.',28,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,3),
        ('ai-cat-coffee','Coffee and Tea','Late service coffee and tea.',7,'ai-coffee-004','Cold Brew Tonic','Cold brew, tonic, citrus.',34,'',true,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,4),
        ('ai-cat-coffee','Coffee and Tea','Late service coffee and tea.',7,'ai-coffee-005','Saffron Chai','Saffron tea, warm spices.',32,'',true,'["dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,5),
        ('ai-cat-coffee','Coffee and Tea','Late service coffee and tea.',7,'ai-coffee-006','Nitro Cold Brew','Unavailable coffee demo.',36,'',false,'[]'::jsonb,'["vegan"]'::jsonb,'[]'::jsonb,6),
        ('ai-cat-late','Late-Night Bites','Late service snacks.',8,'ai-late-001','Midnight Sliders','Mini beef sliders.',82,'https://images.unsplash.com/photo-1550547660-d9450f859349?w=800&h=600&fit=crop',true,'["gluten","dairy","eggs"]'::jsonb,'[]'::jsonb,'[]'::jsonb,1),
        ('ai-cat-late','Late-Night Bites','Late service snacks.',8,'ai-late-002','Truffle Fries','Fries, truffle, parmesan.',48,'https://images.unsplash.com/photo-1573080496219-bb080dd4f877?w=800&h=600&fit=crop',true,'["dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,2),
        ('ai-cat-late','Late-Night Bites','Late service snacks.',8,'ai-late-003','Spiced Chicken Skewers','Chicken, harissa, yogurt.',64,'',true,'["dairy"]'::jsonb,'[]'::jsonb,'[]'::jsonb,3),
        ('ai-cat-late','Late-Night Bites','Late service snacks.',8,'ai-late-004','Crispy Halloumi Bites','Halloumi, herbs, lemon.',54,'',true,'["dairy"]'::jsonb,'["vegetarian"]'::jsonb,'[]'::jsonb,4),
        ('ai-cat-late','Late-Night Bites','Late service snacks.',8,'ai-late-005','Chickpea Crunch Bowl','Crispy chickpeas, herbs.',44,'',true,'[]'::jsonb,'["vegan","gluten-free"]'::jsonb,'[]'::jsonb,5),
        ('ai-cat-late','Late-Night Bites','Late service snacks.',8,'ai-late-006','Oyster Mignonette','Unavailable shellfish demo.',88,'',false,'["crustaceans"]'::jsonb,'[]'::jsonb,'[]'::jsonb,6)
    ) AS x(category_id, category_name, category_description, category_sort, item_id, item_name, item_description, price, image, is_available, allergens, dietary_tags, options, item_sort);

    SELECT jsonb_agg(
        jsonb_build_object(
            'id', category_id,
            'name', category_name,
            'description', category_description,
            'items', items,
            'sort_order', category_sort
        )
        ORDER BY category_sort
    )
    INTO v_menu_json
    FROM (
        SELECT category_id, category_name, category_description, category_sort,
            jsonb_agg(
                jsonb_build_object(
                    'id', item_id,
                    'name', item_name,
                    'description', item_description,
                    'price', price,
                    'currency', currency,
                    'image', image,
                    'images', CASE
                        WHEN image = '' THEN '[]'::jsonb
                        WHEN item_sort IN (1, 2) THEN jsonb_build_array(image, regexp_replace(image, 'w=800', 'w=960'))
                        ELSE jsonb_build_array(image)
                    END,
                    'composition_image', CASE WHEN business_key = 'ai' THEN image ELSE '' END,
                    'options', options,
                    'allergens', allergens,
                    'dietary_tags', dietary_tags,
                    'is_available', is_available,
                    'sort_order', item_sort
                )
                ORDER BY item_sort
            ) AS items
        FROM demo_seed_menu_items
        WHERE business_key = 'core'
        GROUP BY category_id, category_name, category_description, category_sort
    ) cats;

    INSERT INTO menus (business_id, categories, is_active, version, created_at, updated_at)
    VALUES (v_core_id, v_menu_json::text, true, 1, now(), now())
    RETURNING id INTO v_menu_id;

    SELECT jsonb_agg(
        jsonb_build_object(
            'id', category_id,
            'name', category_name,
            'description', category_description,
            'items', items,
            'sort_order', category_sort
        )
        ORDER BY category_sort
    )
    INTO v_menu_json
    FROM (
        SELECT category_id, category_name, category_description, category_sort,
            jsonb_agg(
                jsonb_build_object(
                    'id', item_id,
                    'name', item_name,
                    'description', item_description,
                    'price', price,
                    'currency', currency,
                    'image', image,
                    'images', CASE
                        WHEN image = '' THEN '[]'::jsonb
                        WHEN item_sort IN (1, 2) THEN jsonb_build_array(image, regexp_replace(image, 'w=800', 'w=960'))
                        ELSE jsonb_build_array(image)
                    END,
                    'composition_image', CASE WHEN image = '' THEN 'https://images.unsplash.com/photo-1504674900247-0877df9cc836?w=800&q=75' ELSE image END,
                    'options', options,
                    'allergens', allergens,
                    'dietary_tags', dietary_tags,
                    'is_available', is_available,
                    'sort_order', item_sort
                )
                ORDER BY item_sort
            ) AS items
        FROM demo_seed_menu_items
        WHERE business_key = 'ai'
        GROUP BY category_id, category_name, category_description, category_sort
    ) cats;

    INSERT INTO menus (business_id, categories, is_active, version, created_at, updated_at)
    VALUES (v_ai_id, v_menu_json::text, true, 1, now(), now());

    INSERT INTO translations (business_id, entity_type, entity_id, field_name, language_code, original_text, translated_text, is_auto_translated, translation_source, created_at, updated_at) VALUES
        (v_core_id, 'business', v_core_id, 'welcome_message', 'es', 'Welcome to Demo Core Kitchen. Scan, order, split, and enjoy.', 'Bienvenido a Demo Core Kitchen. Escanea, ordena, divide y disfruta.', true, 'demo_seed', now(), now()),
        (v_core_id, 'business', v_core_id, 'welcome_message', 'pt', 'Welcome to Demo Core Kitchen. Scan, order, split, and enjoy.', 'Bem-vindo ao Demo Core Kitchen. Escaneie, peca, divida e aproveite.', true, 'demo_seed', now(), now()),
        (v_core_id, 'business', v_core_id, 'welcome_message', 'ar', 'Welcome to Demo Core Kitchen. Scan, order, split, and enjoy.', 'Demo Arabic welcome message for Demo Core Kitchen.', true, 'demo_seed', now(), now()),
        (v_ai_id, 'business', v_ai_id, 'welcome_message', 'ar', 'Welcome to Demo AI Lounge. Sofia can help with dining, reservations, delivery, and recommendations.', 'Demo Arabic welcome message for Demo AI Lounge.', true, 'demo_seed', now(), now()),
        (v_ai_id, 'business', v_ai_id, 'welcome_message', 'es', 'Welcome to Demo AI Lounge. Sofia can help with dining, reservations, delivery, and recommendations.', 'Bienvenido a Demo AI Lounge. Sofia puede ayudar con comida, reservas, delivery y recomendaciones.', true, 'demo_seed', now(), now()),
        (v_ai_id, 'business', v_ai_id, 'welcome_message', 'fr', 'Welcome to Demo AI Lounge. Sofia can help with dining, reservations, delivery, and recommendations.', 'Bienvenue au Demo AI Lounge. Sofia peut aider avec le repas, les reservations, la livraison et les recommandations.', true, 'demo_seed', now(), now());

    INSERT INTO translations (business_id, entity_type, entity_id, field_name, language_code, original_text, translated_text, is_auto_translated, translation_source, created_at, updated_at)
    SELECT
        CASE WHEN business_key = 'core' THEN v_core_id ELSE v_ai_id END,
        'category',
        category_sort::integer,
        'name',
        language_code,
        category_name,
        '[' || language_code || ' demo] ' || category_name,
        true,
        'demo_seed',
        now(),
        now()
    FROM (
        SELECT DISTINCT business_key, category_sort, category_name
        FROM demo_seed_menu_items
        WHERE (business_key = 'core' AND category_sort <= 6)
           OR (business_key = 'ai' AND category_sort <= 8)
    ) cats
    CROSS JOIN LATERAL (
        SELECT unnest(CASE WHEN business_key = 'core' THEN ARRAY['es','pt'] ELSE ARRAY['ar','es','fr'] END) AS language_code
    ) langs;

    INSERT INTO translations (business_id, entity_type, entity_id, field_name, language_code, original_text, translated_text, is_auto_translated, translation_source, created_at, updated_at)
    SELECT
        CASE WHEN business_key = 'core' THEN v_core_id ELSE v_ai_id END,
        'menu_item',
        (category_sort * 1000 + item_sort)::integer,
        'name',
        language_code,
        item_name,
        '[' || language_code || ' demo] ' || item_name,
        true,
        'demo_seed',
        now(),
        now()
    FROM demo_seed_menu_items
    CROSS JOIN LATERAL (
        SELECT unnest(CASE WHEN business_key = 'core' THEN ARRAY['es','pt'] ELSE ARRAY['ar','es','fr'] END) AS language_code
    ) langs
    WHERE item_sort <= 2;

    INSERT INTO bundles (business_id, name, description, price, currency, image, items, is_active, created_at, updated_at) VALUES
        (v_core_id, 'Core Lunch Combo', 'Main, side, and drink bundle.', 24.00, 'USD', 'https://images.unsplash.com/photo-1546069901-ba9599a7e63c?w=800&q=75', '[{"menu_item_id":"core-main-001","name":"Lemon Chicken Bowl","quantity":1},{"menu_item_id":"core-side-002","name":"Crispy Fries","quantity":1},{"menu_item_id":"core-drink-002","name":"Mint Lemonade","quantity":1}]', true, now(), now()),
        (v_core_id, 'Core Family Meal', 'Family bundle with mains, sides, and dessert.', 72.00, 'USD', 'https://images.unsplash.com/photo-1565299507177-b0ac66763828?w=800&q=75', '[{"menu_item_id":"core-main-002","name":"Falafel Power Bowl","quantity":2},{"menu_item_id":"core-handheld-004","name":"Classic Smash Burger","quantity":2},{"menu_item_id":"core-dessert-001","name":"Chocolate Lava Cake","quantity":2}]', true, now(), now()),
        (v_core_id, 'Coffee and Dessert', 'Cafe pairing bundle.', 13.00, 'USD', 'https://images.unsplash.com/photo-1509042239860-f550ce710b93?w=800&q=75', '[{"menu_item_id":"core-drink-001","name":"Cold Brew Latte","quantity":1},{"menu_item_id":"core-dessert-002","name":"Coconut Lime Cheesecake","quantity":1}]', true, now(), now()),
        (v_core_id, 'Delivery Dinner Box', 'Delivery-friendly main and drink.', 29.00, 'USD', 'https://images.unsplash.com/photo-1565958011703-44f9829ba187?w=800&q=75', '[{"menu_item_id":"core-main-003","name":"Seared Salmon Bowl","quantity":1},{"menu_item_id":"core-drink-005","name":"House Kombucha","quantity":1}]', true, now(), now()),
        (v_core_id, 'Staff Pick Tasting', 'A staff-curated sampler.', 36.00, 'USD', 'https://images.unsplash.com/photo-1567620905732-2d1ec7ab7445?w=800&q=75', '[{"menu_item_id":"core-starter-003","name":"Crispy Cauliflower Bites","quantity":1},{"menu_item_id":"core-main-005","name":"Mushroom Grain Bowl","quantity":1},{"menu_item_id":"core-dessert-004","name":"Berry Chia Cup","quantity":1}]', false, now(), now()),
        (v_ai_id, 'VIP Tasting Bundle', 'AI Pro VIP tasting with premium pairings.', 420.00, 'AED', 'https://images.unsplash.com/photo-1559339352-11d035aa65de?w=800&q=75', '[{"menu_item_id":"ai-taste-002","name":"Date Glazed Duck Bite","quantity":2},{"menu_item_id":"ai-main-001","name":"Wagyu Striploin","quantity":1},{"menu_item_id":"ai-mocktail-005","name":"Smoked Date Tonic","quantity":2}]', true, now(), now()),
        (v_ai_id, 'Couple Dinner Bundle', 'Two mains, mocktails, and dessert.', 520.00, 'AED', 'https://images.unsplash.com/photo-1414235077428-338989a2e8c0?w=800&q=75', '[{"menu_item_id":"ai-main-002","name":"Saffron Sea Bass","quantity":1},{"menu_item_id":"ai-main-005","name":"Black Garlic Chicken","quantity":1},{"menu_item_id":"ai-dessert-002","name":"Cardamom Chocolate Dome","quantity":2}]', true, now(), now()),
        (v_ai_id, 'Late-Night Mocktail Flight', 'Three signature mocktails.', 126.00, 'AED', 'https://images.unsplash.com/photo-1551024506-0bccd828d307?w=800&q=75', '[{"menu_item_id":"ai-mocktail-001","name":"Pomegranate Rosemary Fizz","quantity":1},{"menu_item_id":"ai-mocktail-002","name":"Saffron Citrus Cooler","quantity":1},{"menu_item_id":"ai-mocktail-004","name":"Black Tea Peach Spritz","quantity":1}]', true, now(), now()),
        (v_ai_id, 'Business Lunch Bundle', 'Efficient corporate lunch package.', 260.00, 'AED', 'https://images.unsplash.com/photo-1513104890138-7c749659a591?w=800&q=75', '[{"menu_item_id":"ai-starter-003","name":"Spiced Lentil Kibbeh","quantity":2},{"menu_item_id":"ai-plant-002","name":"Emirati Grain Garden","quantity":2}]', true, now(), now()),
        (v_ai_id, 'Premium Delivery Box', 'Delivery-ready premium tasting box.', 360.00, 'AED', 'https://images.unsplash.com/photo-1555939594-58d7cb561ad1?w=800&q=75', '[{"menu_item_id":"ai-starter-002","name":"Zaartar Prawns","quantity":1},{"menu_item_id":"ai-main-003","name":"Lamb Shoulder Majlis","quantity":1},{"menu_item_id":"ai-dessert-005","name":"Coconut Sorbet","quantity":2}]', true, now(), now());

    INSERT INTO offers (business_id, name, description, image, discount_type, discount_value, start_date, end_date, is_active, applicable_to, target_id, created_at, updated_at) VALUES
        (v_core_id, 'Core Bowl Boost', '15 percent off bowls this week.', 'https://images.unsplash.com/photo-1540189549336-e6e99c3679fe?w=800&q=75', 'percentage', 15, now() - interval '2 days', now() + interval '9 days', true, 'category', 'core-cat-bowls', now(), now()),
        (v_core_id, 'Cold Brew Add-On', 'Fixed discount on cold brew.', '', 'fixed', 1.50, now() - interval '1 day', now() + interval '14 days', true, 'item', 'core-drink-001', now(), now()),
        (v_core_id, 'Happy Hour', 'All-menu happy hour for live testing.', '', 'percentage', 10, now() - interval '1 day', now() + interval '1 day', true, 'all', NULL, now(), now()),
        (v_core_id, 'Lunch Combo Lift', 'Bundle-targeted Core offer for promotion QA.', '', 'fixed', 3, now() - interval '1 day', now() + interval '14 days', true, 'bundle', (SELECT id::text FROM bundles WHERE business_id = v_core_id AND name = 'Core Lunch Combo' LIMIT 1), now(), now()),
        (v_core_id, 'Expired Family Push', 'Expired offer for state testing.', '', 'percentage', 20, now() - interval '30 days', now() - interval '7 days', true, 'all', NULL, now(), now()),
        (v_core_id, 'Draft Delivery Discount', 'Inactive draft offer.', '', 'fixed', 5, now() + interval '5 days', now() + interval '30 days', false, 'all', NULL, now(), now()),
        (v_ai_id, 'VIP Margin Push', 'AI Pro high-margin category offer.', 'https://images.unsplash.com/photo-1587574293340-e0011c4e8ecf?w=800&q=75', 'percentage', 12, now() - interval '1 day', now() + interval '10 days', true, 'category', 'ai-cat-mocktails', now(), now()),
        (v_ai_id, 'VIP Bundle Push', 'AI Pro bundle-targeted offer for premium bundle QA.', '', 'fixed', 55, now() - interval '1 day', now() + interval '10 days', true, 'bundle', (SELECT id::text FROM bundles WHERE business_id = v_ai_id AND name = 'VIP Tasting Bundle' LIMIT 1), now(), now()),
        (v_ai_id, 'Tasting Urgency', 'Expiring today offer.', '', 'fixed', 42, now() - interval '2 days', now() + interval '12 hours', true, 'category', 'ai-cat-tasting', now(), now()),
        (v_ai_id, 'Future Corporate Lunch', 'Scheduled future offer.', '', 'percentage', 8, now() + interval '7 days', now() + interval '21 days', true, 'category', 'ai-cat-plant', now(), now()),
        (v_ai_id, 'Expired Terrace Push', 'Expired AI Pro offer.', '', 'percentage', 15, now() - interval '28 days', now() - interval '10 days', true, 'all', NULL, now(), now()),
        (v_ai_id, 'Inactive Chef Table', 'Inactive AI Pro draft.', '', 'fixed', 60, now() + interval '14 days', now() + interval '45 days', false, 'all', NULL, now(), now());

    FOR v_i IN 1..10 LOOP
        INSERT INTO tables (business_id, table_code, name, capacity, qr_code, is_active, qr_logo_url, qr_foreground_color, qr_background_color, qr_logo_size, qr_show_business_name, qr_show_table_name, qr_text_font, created_at, updated_at)
        VALUES (
            v_core_id,
            -- High-entropy codes only (#293). Avoid CORE-T0N / demo-{biz}-* patterns.
            upper(substr(md5('payverge-fixture-core-table-' || v_i::text), 1, 10)),
            CASE WHEN v_i <= 4 THEN 'Indoor ' || v_i WHEN v_i <= 7 THEN 'Patio ' || (v_i - 4) WHEN v_i <= 9 THEN 'Bar ' || (v_i - 7) ELSE 'Window 1' END,
            CASE WHEN v_i IN (1,2) THEN 2 WHEN v_i IN (3,4,5,6) THEN 4 WHEN v_i IN (7,8,9) THEN 6 ELSE 8 END,
            'https://demo.payverge.example/t/' || upper(substr(md5('payverge-fixture-core-table-' || v_i::text), 1, 10)),
            true,
            CASE WHEN v_i <= 4 THEN 'https://images.unsplash.com/photo-1414235077428-338989a2e8c0?w=128&q=75' ELSE '' END,
            CASE WHEN v_i % 2 = 0 THEN '#0f766e' ELSE '#111827' END,
            '#ffffff',
            20 + (v_i % 4),
            true,
            true,
            'Inter',
            now(),
            now()
        );
    END LOOP;

    FOR v_i IN 1..16 LOOP
        INSERT INTO tables (business_id, table_code, name, capacity, qr_code, is_active, qr_logo_url, qr_foreground_color, qr_background_color, qr_logo_size, qr_show_business_name, qr_show_table_name, qr_text_font, created_at, updated_at)
        VALUES (
            v_ai_id,
            -- High-entropy codes only (#293). Stable fixture salt is not biz-id-derived.
            upper(substr(md5('payverge-fixture-ai-pro-table-' || v_i::text), 1, 10)),
            CASE WHEN v_i <= 5 THEN 'Main Room ' || v_i WHEN v_i <= 9 THEN 'Terrace ' || (v_i - 5) WHEN v_i <= 12 THEN 'Bar ' || (v_i - 9) ELSE 'VIP Room ' || (v_i - 12) END,
            CASE WHEN v_i <= 4 THEN 2 WHEN v_i <= 9 THEN 4 WHEN v_i <= 13 THEN 6 WHEN v_i <= 15 THEN 8 ELSE 12 END,
            'https://demo.payverge.example/t/' || upper(substr(md5('payverge-fixture-ai-pro-table-' || v_i::text), 1, 10)),
            true,
            'https://images.unsplash.com/photo-1546069901-d5bfd2cbfb1f?w=128&q=75',
            CASE WHEN v_i % 2 = 0 THEN '#064e3b' ELSE '#f59e0b' END,
            '#ffffff',
            22 + (v_i % 4),
            true,
            true,
            'Inter',
            now(),
            now()
        );
    END LOOP;

    INSERT INTO counters (business_id, counter_number, name, is_active, created_at, updated_at) VALUES
        (v_core_id, 1, 'Pickup 1', true, now(), now()),
        (v_core_id, 2, 'Pickup 2', true, now(), now()),
        (v_core_id, 3, 'Barista', true, now(), now()),
        (v_ai_id, 1, 'Host Desk', true, now(), now()),
        (v_ai_id, 2, 'Bar', true, now(), now()),
        (v_ai_id, 3, 'Terrace Pass', true, now(), now()),
        (v_ai_id, 4, 'Pickup', true, now(), now());

    INSERT INTO reservation_settings (
        business_id, enabled, max_advance_days, min_advance_minutes, min_party_size, max_party_size,
        default_duration, slot_interval_minutes, service_buffer_minutes, max_covers_per_slot,
        auto_assign_tables, approval_mode, allow_waitlist, hold_duration_minutes,
        allow_cancellation, cancellation_deadline, no_show_grace_minutes,
        send_confirmation_email, send_reminder_email, reminder_hours_before,
        external_partner_links,
        created_at, updated_at
    )
    VALUES
        (v_core_id, true, 45, 30, 1, 12, 90, 30, 15, 80, true, 'manual', true, 15, true, 24, 15, true, true, 24,
         '[{"name":"OpenTable","url":"https://www.opentable.com/r/demo-core-kitchen-demo","provider_key":"opentable"},{"name":"Resy","url":"https://resy.com/cities/ny/demo-core-kitchen-demo","provider_key":"resy"}]'::jsonb,
         now(), now()),
        (v_ai_id, true, 60, 15, 1, 24, 120, 30, 20, 160, true, 'manual', true, 20, true, 24, 15, true, true, 12,
         '[{"name":"OpenTable","url":"https://www.opentable.com/r/demo-ai-lounge-demo","provider_key":"opentable"},{"name":"Resy","url":"https://resy.com/cities/dubai/demo-ai-lounge-demo","provider_key":"resy"}]'::jsonb,
         now(), now());

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        FOR v_i IN 1..CASE WHEN v_business_key = 'core' THEN 18 ELSE 28 END LOOP
            IF v_business_key = 'core' THEN
                v_table_id := (SELECT id FROM tables WHERE business_id = v_core_id ORDER BY id OFFSET ((v_i - 1) % 10) LIMIT 1);
                v_status := v_reservation_statuses[((v_i - 1) % array_length(v_reservation_statuses, 1)) + 1];
                INSERT INTO table_reservations (
                    business_id, table_id, customer_name, customer_phone, customer_email, party_size,
                    reservation_time, duration, status, source, confirmation_code, special_requests, notes,
                    reminder_sent, created_by, confirmed_at, assigned_at, seated_at, completed_at,
                    cancelled_at, cancelled_by, cancellation_reason, waitlist_position, created_at, updated_at
                )
                VALUES (
                    v_core_id,
                    CASE WHEN v_i % 7 = 3 THEN NULL ELSE v_table_id END,
                    'Core Guest ' || v_i,
                    '+1 555 201 ' || lpad(v_i::text, 4, '0'),
                    'reservation' || lpad(v_i::text, 2, '0') || '@core-demo.payverge.example',
                    1 + (v_i % 12),
                    CASE WHEN v_i <= 9 THEN now() + (v_i || ' days')::interval ELSE now() - ((v_i - 8) || ' days')::interval END,
                    90,
                    v_status,
                    CASE WHEN v_i % 3 = 0 THEN 'staff' ELSE 'customer' END,
                    'CORE-RSV-' || lpad(v_i::text, 4, '0'),
                    CASE WHEN v_i % 5 = 0 THEN 'Nut allergy noted.' ELSE 'Window seat if available.' END,
                    CASE WHEN v_i % 4 = 0 THEN 'Demo reservation note for host.' ELSE '' END,
                    v_i % 2 = 0,
                    CASE WHEN v_i % 3 = 0 THEN 'staff' ELSE 'customer' END,
                    CASE WHEN v_status IN ('confirmed','seated','completed') THEN now() - interval '1 day' ELSE NULL END,
                    CASE WHEN v_status IN ('confirmed','seated','completed') THEN now() - interval '1 day' ELSE NULL END,
                    CASE WHEN v_status IN ('seated','completed') THEN now() - interval '2 hours' ELSE NULL END,
                    CASE WHEN v_status = 'completed' THEN now() - interval '1 hour' ELSE NULL END,
                    CASE WHEN v_status IN ('cancelled','no_show') THEN now() - interval '12 hours' ELSE NULL END,
                    CASE WHEN v_status = 'cancelled' THEN 'guest' ELSE '' END,
                    CASE WHEN v_status = 'cancelled' THEN 'Plans changed.' WHEN v_status = 'no_show' THEN 'No-show after grace period.' ELSE '' END,
                    CASE WHEN v_i % 7 = 3 THEN ((v_i % 4) + 1) ELSE NULL END,
                    now() - ((v_i + 10) || ' days')::interval,
                    now()
                )
                RETURNING id INTO v_delivery_id;
            ELSE
                v_table_id := (SELECT id FROM tables WHERE business_id = v_ai_id ORDER BY id OFFSET ((v_i - 1) % 16) LIMIT 1);
                v_status := v_reservation_statuses[((v_i - 1) % array_length(v_reservation_statuses, 1)) + 1];
                INSERT INTO table_reservations (
                    business_id, table_id, customer_name, customer_phone, customer_email, party_size,
                    reservation_time, duration, status, source, confirmation_code, special_requests, notes,
                    reminder_sent, created_by, confirmed_at, assigned_at, seated_at, completed_at,
                    cancelled_at, cancelled_by, cancellation_reason, waitlist_position, created_at, updated_at
                )
                VALUES (
                    v_ai_id,
                    CASE WHEN v_i % 7 = 3 THEN NULL ELSE v_table_id END,
                    'AI Lounge Guest ' || v_i,
                    '+1 555 402 ' || lpad(v_i::text, 4, '0'),
                    'reservation' || lpad(v_i::text, 2, '0') || '@ai-demo.payverge.example',
                    2 + (v_i % 16),
                    CASE WHEN v_i <= 14 THEN now() + (v_i || ' days')::interval ELSE now() - ((v_i - 13) || ' days')::interval END,
                    120,
                    v_status,
                    CASE WHEN v_i % 4 = 0 THEN 'ai_waiter' WHEN v_i % 3 = 0 THEN 'staff' ELSE 'customer' END,
                    'AI-RSV-' || lpad(v_i::text, 4, '0'),
                    CASE WHEN v_i % 5 = 0 THEN 'VIP guest, Arabic preferred, shellfish allergy.' ELSE 'Terrace or VIP room if available.' END,
                    CASE WHEN v_i % 4 = 0 THEN 'Director Console should reference this VIP reservation.' ELSE '' END,
                    v_i % 2 = 1,
                    CASE WHEN v_i % 4 = 0 THEN 'ai_waiter' ELSE 'customer' END,
                    CASE WHEN v_status IN ('confirmed','seated','completed') THEN now() - interval '1 day' ELSE NULL END,
                    CASE WHEN v_status IN ('confirmed','seated','completed') THEN now() - interval '1 day' ELSE NULL END,
                    CASE WHEN v_status IN ('seated','completed') THEN now() - interval '2 hours' ELSE NULL END,
                    CASE WHEN v_status = 'completed' THEN now() - interval '1 hour' ELSE NULL END,
                    CASE WHEN v_status IN ('cancelled','no_show') THEN now() - interval '12 hours' ELSE NULL END,
                    CASE WHEN v_status = 'cancelled' THEN 'staff' ELSE '' END,
                    CASE WHEN v_status = 'cancelled' THEN 'Deposit moved to future visit.' WHEN v_status = 'no_show' THEN 'VIP no-show flagged.' ELSE '' END,
                    CASE WHEN v_i % 7 = 3 THEN ((v_i % 5) + 1) ELSE NULL END,
                    now() - ((v_i + 8) || ' days')::interval,
                    now()
                )
                RETURNING id INTO v_delivery_id;
            END IF;

            IF v_status <> 'pending' OR v_i % 7 = 3 THEN
                INSERT INTO reservation_status_histories (reservation_id, status, table_id, notes, changed_by, created_at)
                VALUES (
                    v_delivery_id,
                    CASE WHEN v_i % 7 = 3 THEN 'waitlist' ELSE v_status END,
                    CASE WHEN v_i % 7 = 3 THEN NULL ELSE v_table_id END,
                    'Seeded reservation status history.',
                    'demo-seed',
                    now() - interval '1 hour'
                );
            END IF;
        END LOOP;
    END LOOP;

    INSERT INTO staff (business_id, email, name, role, is_active, last_login_at, invited_by, custom_permissions, role_level, permissions_updated_at, permissions_updated_by, created_at, updated_at) VALUES
        (v_core_id, 'manager@core-demo.payverge.example', 'Nina Park', 'manager', true, now() - interval '2 days', v_wallet, '["reports:export","financial:read"]', 80, now() - interval '20 days', v_wallet, now() - interval '80 days', now()),
        (v_core_id, 'server1@core-demo.payverge.example', 'Luis Moreno', 'server', true, now() - interval '5 hours', v_wallet, '[]', 40, now() - interval '30 days', v_wallet, now() - interval '70 days', now()),
        (v_core_id, 'server2@core-demo.payverge.example', 'Ava Stone', 'server', true, now() - interval '1 day', v_wallet, '[]', 40, now() - interval '30 days', v_wallet, now() - interval '65 days', now()),
        (v_core_id, 'host@core-demo.payverge.example', 'Sam Patel', 'host', true, now() - interval '8 hours', v_wallet, '["reservations:write"]', 30, now() - interval '25 days', v_wallet, now() - interval '60 days', now()),
        (v_core_id, 'kitchen@core-demo.payverge.example', 'Marco Silva', 'kitchen', true, now() - interval '3 hours', v_wallet, '[]', 20, now() - interval '25 days', v_wallet, now() - interval '55 days', now()),
        (v_core_id, 'former@core-demo.payverge.example', 'Former Server', 'server', false, now() - interval '50 days', v_wallet, '[]', 40, now() - interval '45 days', v_wallet, now() - interval '90 days', now()),
        (v_ai_id, 'gm@ai-demo.payverge.example', 'Layla Hassan', 'manager', true, now() - interval '2 hours', v_wallet, '["financial:read","reports:export","director:read"]', 90, now() - interval '15 days', v_wallet, now() - interval '100 days', now()),
        (v_ai_id, 'floor@ai-demo.payverge.example', 'Omar Faris', 'manager', true, now() - interval '4 hours', v_wallet, '["reservations:write","delivery:dispatch:write"]', 80, now() - interval '14 days', v_wallet, now() - interval '95 days', now()),
        (v_ai_id, 'server1@ai-demo.payverge.example', 'Maya Chen', 'server', true, now() - interval '5 hours', v_wallet, '[]', 40, now() - interval '30 days', v_wallet, now() - interval '80 days', now()),
        (v_ai_id, 'server2@ai-demo.payverge.example', 'Rafael Cruz', 'server', true, now() - interval '1 day', v_wallet, '[]', 40, now() - interval '30 days', v_wallet, now() - interval '75 days', now()),
        (v_ai_id, 'server3@ai-demo.payverge.example', 'Amira Khan', 'server', true, now() - interval '7 hours', v_wallet, '[]', 40, now() - interval '30 days', v_wallet, now() - interval '70 days', now()),
        (v_ai_id, 'kitchen1@ai-demo.payverge.example', 'Chef Bruno', 'kitchen', true, now() - interval '2 hours', v_wallet, '["orders:status"]', 20, now() - interval '25 days', v_wallet, now() - interval '82 days', now()),
        (v_ai_id, 'kitchen2@ai-demo.payverge.example', 'Chef Noor', 'kitchen', true, now() - interval '6 hours', v_wallet, '["orders:status"]', 20, now() - interval '25 days', v_wallet, now() - interval '82 days', now()),
        (v_ai_id, 'host@ai-demo.payverge.example', 'Zara Ali', 'host', true, now() - interval '3 hours', v_wallet, '["reservations:create","reservations:write"]', 30, now() - interval '22 days', v_wallet, now() - interval '72 days', now()),
        (v_ai_id, 'delivery@ai-demo.payverge.example', 'Imran Malik', 'server', true, now() - interval '9 hours', v_wallet, '["delivery:settings:read","delivery:dispatch:read","delivery:dispatch:write"]', 45, now() - interval '18 days', v_wallet, now() - interval '68 days', now()),
        (v_ai_id, 'analyst@ai-demo.payverge.example', 'Dana Weiss', 'manager', true, now() - interval '2 days', v_wallet, '["analytics:sales","analytics:items","reports:export"]', 75, now() - interval '12 days', v_wallet, now() - interval '64 days', now()),
        (v_ai_id, 'former@ai-demo.payverge.example', 'Former Concierge', 'host', false, now() - interval '44 days', v_wallet, '[]', 30, now() - interval '44 days', v_wallet, now() - interval '100 days', now());

    INSERT INTO staff_invitations (business_id, email, name, role, token, status, invited_by, expires_at, created_at, updated_at) VALUES
        (v_core_id, 'pending.manager@core-demo.payverge.example', 'Pending Manager', 'manager', 'core-demo-invite-pending-manager', 'pending', v_wallet, now() + interval '7 days', now() - interval '1 day', now()),
        (v_core_id, 'expired.server@core-demo.payverge.example', 'Expired Server', 'server', 'core-demo-invite-expired-server', 'expired', v_wallet, now() - interval '2 days', now() - interval '12 days', now()),
        (v_core_id, 'revoked.kitchen@core-demo.payverge.example', 'Revoked Kitchen', 'kitchen', 'core-demo-invite-revoked-kitchen', 'revoked', v_wallet, now() + interval '3 days', now() - interval '4 days', now()),
        (v_ai_id, 'pending.viphost@ai-demo.payverge.example', 'Pending VIP Host', 'host', 'ai-demo-invite-pending-viphost', 'pending', v_wallet, now() + interval '7 days', now() - interval '1 day', now()),
        (v_ai_id, 'expired.server@ai-demo.payverge.example', 'Expired AI Server', 'server', 'ai-demo-invite-expired-server', 'expired', v_wallet, now() - interval '3 days', now() - interval '14 days', now()),
        (v_ai_id, 'revoked.analyst@ai-demo.payverge.example', 'Revoked Analyst', 'manager', 'ai-demo-invite-revoked-analyst', 'revoked', v_wallet, now() + interval '2 days', now() - interval '5 days', now());

    FOR v_staff_id IN SELECT id FROM staff WHERE business_id IN (v_core_id, v_ai_id) ORDER BY id LOOP
        INSERT INTO rbac_audit_logs (staff_id, business_id, action, old_role, new_role, old_permissions, new_permissions, changed_by, reason, ip_address, user_agent, created_at)
        SELECT v_staff_id, business_id, 'staff_created', '', role, '[]', custom_permissions, v_wallet, 'Demo staff fixture', '127.0.0.1', 'Payverge demo seed', created_at
        FROM staff WHERE id = v_staff_id;
    END LOOP;

    FOR v_staff_id IN SELECT id FROM staff WHERE email IN ('manager@core-demo.payverge.example','gm@ai-demo.payverge.example') LOOP
        INSERT INTO staff_login_codes (staff_id, code, expires_at, used, created_at) VALUES
            (v_staff_id, '100' || (v_staff_id % 1000)::text, now() - interval '1 day', false, now() - interval '2 days'),
            (v_staff_id, '200' || (v_staff_id % 1000)::text, now() + interval '1 day', true, now() - interval '1 hour');
    END LOOP;

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        FOR v_i IN 1..CASE WHEN v_business_key = 'core' THEN 14 ELSE 24 END LOOP
            INSERT INTO customers (
                email, password_hash, name, phone, wallet_address, birthday, profile_image_url,
                is_active, email_verified, last_login_at, created_at, updated_at
            )
            VALUES (
                CASE WHEN v_business_key = 'core'
                    THEN 'customer' || lpad(v_i::text, 2, '0') || '@core-demo.payverge.example'
                    ELSE 'customer' || lpad(v_i::text, 2, '0') || '@ai-demo.payverge.example'
                END,
                '',
                CASE WHEN v_business_key = 'core' THEN 'Core Customer ' || v_i ELSE 'AI Guest ' || v_i END,
                CASE WHEN v_business_key = 'core' THEN '+1 555 301 ' || lpad(v_i::text, 4, '0') ELSE '+1 555 502 ' || lpad(v_i::text, 4, '0') END,
                '0x' || md5(v_business_key || '-customer-wallet-' || v_i) || substr(md5('wallet-' || v_i), 1, 8),
                (now() - ((9000 + v_i * 130) || ' days')::interval)::date,
                'https://images.unsplash.com/photo-1565299507177-b0ac66763828?w=128&q=75',
                NOT (v_i = CASE WHEN v_business_key = 'core' THEN 14 ELSE 24 END),
                true,
                now() - ((v_i % 8) || ' days')::interval,
                now() - ((30 + v_i) || ' days')::interval,
                now()
            )
            RETURNING id INTO v_customer_id;

            INSERT INTO customer_preferences (customer_id, preferred_language, preferred_currency, receive_promotions, receive_newsletters, receive_birthday_offers, share_data_with_businesses, created_at, updated_at)
            VALUES (
                v_customer_id,
                CASE WHEN v_business_key = 'ai' AND v_i % 5 = 0 THEN 'ar' WHEN v_i % 4 = 0 THEN 'es' ELSE 'en' END,
                CASE WHEN v_business_key = 'ai' THEN 'AED' ELSE 'USD' END,
                v_i % 3 <> 0, true, true, true, now(), now()
            );

            INSERT INTO customer_businesses (
                customer_id, business_id, loyalty_points, loyalty_tier, total_spent, visit_count, last_visit_at, first_visit_at,
                opt_in_marketing, opt_in_sms, opt_in_email, favorite_items, dietary_preferences, allergies, notes, tags,
                is_active, created_at, updated_at
            )
            VALUES (
                v_customer_id,
                CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
                100 + v_i * 33,
                -- Tier is derived from the same point formula so it always
                -- tracks loyalty_points. Thresholds:
                --   Bronze < 200, Silver 200-449, Gold >= 450.
                CASE
                    WHEN (100 + v_i * 33) >= 450 THEN 'Gold'
                    WHEN (100 + v_i * 33) >= 200 THEN 'Silver'
                    ELSE 'Bronze'
                END,
                CASE WHEN v_business_key = 'core' THEN 55 + v_i * 42 ELSE 220 + v_i * 95 END,
                1 + (v_i % 9),
                now() - ((v_i % 18) || ' days')::interval,
                now() - ((40 + v_i) || ' days')::interval,
                true, v_i % 2 = 0, true,
                CASE WHEN v_business_key = 'core' THEN '["core-main-001","core-drink-001"]' ELSE '["ai-main-001","ai-mocktail-001"]' END,
                CASE WHEN v_i % 4 = 0 THEN '["vegan"]' WHEN v_i % 5 = 0 THEN '["gluten-free"]' ELSE '[]' END,
                CASE WHEN v_i % 5 = 0 THEN '["nuts"]' WHEN v_i % 7 = 0 THEN '["shellfish"]' ELSE '[]' END,
                CASE WHEN v_i <= 3 THEN 'High-value guest for demo segmentation.' WHEN v_i % 5 = 0 THEN 'Allergy note requires careful service.' ELSE 'Seeded demo customer.' END,
                CASE WHEN v_i <= 3 THEN '["high-value","birthday-club"]' WHEN v_i % 4 = 0 THEN '["delivery-heavy","at-risk"]' ELSE '["demo"]' END,
                true, now() - ((40 + v_i) || ' days')::interval, now()
            )
            RETURNING id INTO v_customer_business_id;

            INSERT INTO customer_addresses (
                customer_id, business_id, label, street, apartment, city, state, postal_code, country, formatted_address,
                latitude, longitude, delivery_instructions, contactless_delivery, leave_at_door, contact_name, contact_phone,
                is_default, is_active, last_used_at, usage_count, created_at, updated_at
            )
            VALUES (
                v_customer_id,
                CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
                CASE WHEN v_i % 3 = 0 THEN 'Work' ELSE 'Home' END,
                CASE WHEN v_business_key = 'core' THEN (100 + v_i)::text || ' Demo Avenue' ELSE 'Dubai Marina Tower ' || v_i END,
                CASE WHEN v_i % 2 = 0 THEN 'Apt ' || v_i ELSE '' END,
                CASE WHEN v_business_key = 'core' THEN 'New York' ELSE 'Dubai' END,
                CASE WHEN v_business_key = 'core' THEN 'NY' ELSE 'Dubai' END,
                CASE WHEN v_business_key = 'core' THEN '100' || lpad(v_i::text, 2, '0') ELSE '00000' END,
                CASE WHEN v_business_key = 'core' THEN 'United States' ELSE 'United Arab Emirates' END,
                CASE WHEN v_business_key = 'core' THEN (100 + v_i)::text || ' Demo Avenue, New York, NY' ELSE 'Dubai Marina Tower ' || v_i || ', Dubai' END,
                CASE WHEN v_business_key = 'core' THEN 40.73 + (v_i::numeric / 1000) ELSE 25.20 + (v_i::numeric / 1000) END,
                CASE WHEN v_business_key = 'core' THEN -73.99 - (v_i::numeric / 1000) ELSE 55.27 + (v_i::numeric / 1000) END,
                CASE WHEN v_i % 4 = 0 THEN 'Leave with concierge.' ELSE 'Call on arrival.' END,
                v_i % 2 = 0, v_i % 3 = 0,
                CASE WHEN v_business_key = 'core' THEN 'Core Customer ' || v_i ELSE 'AI Guest ' || v_i END,
                CASE WHEN v_business_key = 'core' THEN '+1 555 301 ' || lpad(v_i::text, 4, '0') ELSE '+1 555 502 ' || lpad(v_i::text, 4, '0') END,
                true, true, now() - ((v_i % 12) || ' days')::interval, 1 + (v_i % 6), now(), now()
            );

            INSERT INTO customer_communications (
                business_id, customer_business_id, type, status, subject, content, sent_at, delivered_at, opened_at, clicked_at, error_message, campaign_id, created_at, updated_at
            )
            VALUES (
                CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
                v_customer_business_id,
                CASE WHEN v_i % 3 = 0 THEN 'sms' ELSE 'email' END,
                CASE WHEN v_i % 9 = 0 THEN 'failed' WHEN v_i % 4 = 0 THEN 'delivered' ELSE 'sent' END,
                CASE WHEN v_i % 5 = 0 THEN 'Birthday reward' WHEN v_i % 4 = 0 THEN 'Review request' ELSE 'Demo follow-up' END,
                'Seeded CRM communication.',
                now() - ((v_i % 10) || ' days')::interval,
                CASE WHEN v_i % 9 = 0 THEN NULL ELSE now() - ((v_i % 10) || ' days')::interval + interval '10 minutes' END,
                CASE WHEN v_i % 2 = 0 THEN now() - ((v_i % 8) || ' days')::interval ELSE NULL END,
                CASE WHEN v_i % 5 = 0 THEN now() - ((v_i % 7) || ' days')::interval ELSE NULL END,
                CASE WHEN v_i % 9 = 0 THEN 'Demo failure for retry UI.' ELSE '' END,
                CASE WHEN v_business_key = 'core' THEN 'core-demo-crm' ELSE 'ai-demo-crm' END,
                now() - ((v_i % 12) || ' days')::interval, now()
            );
        END LOOP;
    END LOOP;

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        FOR v_i IN 1..CASE WHEN v_business_key = 'core' THEN 65 ELSE 100 END LOOP
            v_table_id := (
                SELECT id FROM tables
                WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
                ORDER BY id
                OFFSET ((v_i - 1) % (CASE WHEN v_business_key = 'core' THEN 10 ELSE 16 END))
                LIMIT 1
            );
            v_counter_id := NULL;
            IF (v_business_key = 'core' AND v_i > 62) OR (v_business_key = 'ai' AND v_i > 96) THEN
                SELECT id INTO v_counter_id FROM counters
                WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
				ORDER BY id
				OFFSET CASE WHEN v_business_key = 'core' THEN v_i - 63 ELSE v_i - 97 END
				LIMIT 1;
                -- Counter-service bills are not seated at a table. Keeping the
                -- modulo-selected table here can create a second open bill for
				-- that table and violate idx_bills_active_per_table. Each active
				-- counter bill also needs its own counter to preserve the equivalent
				-- idx_bills_active_per_counter occupancy invariant.
                v_table_id := NULL;
            END IF;
            -- Active (open) bills get a fresh timestamp in the last 24h so
            -- the demo dashboard's Today's Sales and Recent Bills feel
            -- live on every seed apply. Historical paid/closed bills keep
            -- their deterministic offsets so analytics-over-time tests are
            -- unaffected. Open ranges:
            --   core: v_i 54..65 (after 53 paid/closed bills)
            --   ai:   v_i 91..100 (after 90 paid/closed bills)
            IF (v_business_key = 'core' AND v_i > 53)
               OR (v_business_key = 'ai' AND v_i > 90) THEN
                v_bill_created := now() - (random() * interval '24 hours');
            ELSE
                v_bill_created := now() - (((v_i * 3) % 45) || ' days')::interval - ((v_i % 7) || ' hours')::interval;
            END IF;
            -- Bills/payments money columns are stored as int64 cents
            -- (see database/models.go Bill.Subtotal et al). We compute
            -- the underlying dollar amount once, then multiply to cents
            -- and round to whole integers so PostgreSQL doesn't truncate
            -- fractional cents on insert into bigint columns. Previous
            -- versions inserted the raw dollar value (e.g. 30.75), which
            -- the API then divided by 100 again and rendered as $0.30.
            v_subtotal := round(
                (CASE WHEN v_business_key = 'core'
                    THEN 24 + (v_i % 12) * 6.75
                    ELSE 140 + (v_i % 18) * 24.5
                END) * 100
            );
            v_tax := round(v_subtotal * CASE WHEN v_business_key = 'core' THEN 0.08875 ELSE 0.05 END);
            v_service := round(v_subtotal * CASE WHEN v_business_key = 'core' THEN 0.04 ELSE 0.075 END);
            v_total := v_subtotal + v_tax + v_service;
            v_tip := CASE WHEN v_i % 4 = 0 THEN round(v_total * 0.18) ELSE 0 END;
            v_status := CASE
                WHEN v_business_key = 'core' AND v_i <= 45 THEN 'paid'
                WHEN v_business_key = 'core' AND v_i <= 53 THEN 'closed'
                WHEN v_business_key = 'ai' AND v_i <= 78 THEN 'paid'
                WHEN v_business_key = 'ai' AND v_i <= 90 THEN 'closed'
                ELSE 'open'
            END;
            v_paid := CASE WHEN v_status = 'paid' THEN v_total ELSE 0 END;
            v_closed_at := CASE WHEN v_status IN ('paid','closed') THEN v_bill_created + interval '55 minutes' ELSE NULL END;

            INSERT INTO bills (
                business_id, table_id, counter_id, bill_number, notes, items,
                subtotal, tax_amount, service_fee_amount, total_amount, paid_amount, tip_amount, status,
                settlement_addr, tipping_addr, created_at, updated_at, closed_at
            )
            VALUES (
                CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
                v_table_id, v_counter_id,
                CASE WHEN v_business_key = 'core' THEN 'CORE-DEMO-' ELSE 'AI-DEMO-' END || lpad(v_i::text, 3, '0'),
                CASE WHEN v_i % 6 = 0 THEN 'Guest requested allergy review.' WHEN v_i % 5 = 0 THEN 'Bundle and offer demo bill.' ELSE 'Seeded demo bill.' END,
                '[]',
                v_subtotal, v_tax, v_service, v_total, v_paid, v_tip, v_status,
                v_wallet, v_wallet, v_bill_created, v_bill_created + interval '20 minutes', v_closed_at
            )
            RETURNING id INTO v_bill_id;

            INSERT INTO bill_items (bill_id, menu_item_id, name, price, quantity, options, item_type, subtotal, created_at)
            SELECT v_bill_id, item_id, item_name, price, CASE WHEN v_i % 3 = 0 THEN 2 ELSE 1 END, options, 'menu_item',
                   price * CASE WHEN v_i % 3 = 0 THEN 2 ELSE 1 END, v_bill_created
            FROM (
                SELECT *, row_number() OVER (ORDER BY category_sort, item_sort) AS rn, count(*) OVER () AS ct
                FROM demo_seed_menu_items
                WHERE business_key = v_business_key
            ) ranked
            WHERE rn IN (((v_i * 2) % ct) + 1, ((v_i * 2 + 7) % ct) + 1);

            INSERT INTO bill_history_events (bill_id, business_id, event_type, actor, reason, item_name, details, created_at) VALUES
                (v_bill_id, CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END, 'bill.created', 'demo-seed', 'Seeded bill opened.', '', '{"source":"demo_seed"}', v_bill_created),
                (v_bill_id, CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END, 'bill_item.added', 'demo-seed', 'Seeded bill items.', '', '{"source":"demo_seed"}', v_bill_created + interval '5 minutes');

            IF v_status IN ('paid','closed') THEN
                INSERT INTO bill_history_events (bill_id, business_id, event_type, actor, reason, details, created_at)
                VALUES (v_bill_id, CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END, 'bill.closed', 'demo-seed', 'Seeded closed bill.', '{"source":"demo_seed"}', v_closed_at);
            END IF;

            IF (v_business_key = 'core' AND v_i <= 22) OR (v_business_key = 'ai' AND v_i <= 40) THEN
                v_status := CASE WHEN v_business_key = 'core'
                    THEN v_core_statuses[((v_i - 1) % array_length(v_core_statuses, 1)) + 1]
                    ELSE v_ai_statuses[((v_i - 1) % array_length(v_ai_statuses, 1)) + 1]
                END;
                INSERT INTO orders (
                    bill_id, business_id, order_number, status, created_by, approved_by, cancelled_by, notes, items, cancel_reason,
                    created_at, updated_at, approved_at, cancelled_at
                )
                VALUES (
                    v_bill_id,
                    CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
                    CASE WHEN v_business_key = 'core' THEN 'CORE-ORD-' ELSE 'AI-ORD-' END || lpad(v_i::text, 3, '0'),
                    v_status,
                    CASE WHEN v_i % 4 = 0 THEN 'staff' WHEN v_business_key = 'ai' AND v_i % 5 = 0 THEN 'ai_waiter' ELSE 'guest' END,
                    CASE WHEN v_status IN ('approved','in_kitchen','ready','delivered') THEN 'manager@' || CASE WHEN v_business_key = 'core' THEN 'core-demo.payverge.example' ELSE 'ai-demo.payverge.example' END ELSE '' END,
                    CASE WHEN v_status = 'cancelled' THEN 'manager@demo' ELSE '' END,
                    CASE WHEN v_i % 5 = 0 THEN 'No nuts, sauce on side.' ELSE 'Seeded order.' END,
                    (SELECT jsonb_agg(jsonb_build_object('id', item_id, 'item_type', 'menu_item', 'menu_item_id', item_id, 'menu_item_name', item_name, 'quantity', 1, 'price', price, 'options', options, 'special_requests', '', 'subtotal', price))::text
                     FROM (SELECT * FROM demo_seed_menu_items WHERE business_key = v_business_key ORDER BY item_id OFFSET ((v_i * 3) % (CASE WHEN v_business_key = 'core' THEN 36 ELSE 48 END)) LIMIT 2) x),
                    CASE WHEN v_status = 'cancelled' THEN 'Customer changed order.' ELSE '' END,
                    v_bill_created + interval '8 minutes',
                    v_bill_created + interval '16 minutes',
                    CASE WHEN v_status IN ('approved','in_kitchen','ready','delivered') THEN v_bill_created + interval '12 minutes' ELSE NULL END,
                    CASE WHEN v_status = 'cancelled' THEN v_bill_created + interval '15 minutes' ELSE NULL END
                )
                RETURNING id INTO v_order_id;

                INSERT INTO bill_history_events (bill_id, business_id, event_type, actor, reason, order_id, order_number, details, created_at)
                VALUES (
                    v_bill_id,
                    CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
                    CASE WHEN v_status = 'cancelled' THEN 'order.cancelled' ELSE 'order.approved' END,
                    'demo-seed',
                    'Seeded order history.',
                    v_order_id,
                    CASE WHEN v_business_key = 'core' THEN 'CORE-ORD-' ELSE 'AI-ORD-' END || lpad(v_i::text, 3, '0'),
                    '{"source":"demo_seed"}',
                    v_bill_created + interval '13 minutes'
                );
            END IF;

            IF v_status = 'paid' OR v_paid > 0 THEN
                IF v_i % 3 = 0 THEN
                    INSERT INTO payments (bill_id, payer_addr, amount, tip_amount, tx_hash, status, payment_method, source_chain, source_token, settlement_chain, lifi_route_id, created_at, updated_at)
                    VALUES (v_bill_id, '0x' || md5('payer-' || v_business_key || v_i) || substr(md5('payerx-' || v_i), 1, 8), v_total, v_tip,
                            '0x' || md5('payment-' || v_business_key || v_i) || md5('payment-extra-' || v_i), 'confirmed',
                            CASE WHEN v_i % 6 = 0 THEN 'cross-chain' ELSE 'crypto' END,
                            CASE WHEN v_i % 6 = 0 THEN 'polygon' ELSE '' END,
                            CASE WHEN v_i % 6 = 0 THEN 'USDC' ELSE '' END,
                            'base',
                            CASE WHEN v_i % 6 = 0 THEN 'demo-lifi-route-' || v_business_key || '-' || v_i ELSE '' END,
                            v_closed_at, v_closed_at);
                ELSE
                    INSERT INTO alternative_payments (bill_id, participant_addr, participant_name, amount, payment_method, status, confirmed_by, created_at, updated_at, confirmed_at)
                    VALUES (v_bill_id, '0x' || md5('altpayer-' || v_business_key || v_i) || substr(md5('altx-' || v_i), 1, 8),
                            CASE WHEN v_business_key = 'core' THEN 'Core Guest ' || v_i ELSE 'AI Guest ' || v_i END,
                            v_total, CASE WHEN v_i % 4 = 0 THEN 'cash' WHEN v_i % 5 = 0 THEN 'venmo' ELSE 'card' END,
                            'confirmed', v_wallet, v_closed_at, v_closed_at, v_closed_at + interval '3 minutes');
                END IF;
            ELSIF v_i % 7 = 0 THEN
                INSERT INTO payments (bill_id, payer_addr, amount, tip_amount, tx_hash, status, payment_method, settlement_chain, created_at, updated_at)
                VALUES (v_bill_id, '0x' || md5('failed-payer-' || v_business_key || v_i) || substr(md5('failedx-' || v_i), 1, 8),
                        v_total, 0, '0x' || md5('failed-payment-' || v_business_key || v_i) || md5('failed-payment-extra-' || v_i),
                        CASE WHEN v_i % 14 = 0 THEN 'failed' ELSE 'pending' END, 'crypto', 'base', now(), now());
            END IF;
        END LOOP;
    END LOOP;

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        INSERT INTO customer_visits (
            customer_business_id, bill_id, table_id, amount_spent, points_earned,
            items_purchased, visit_date, visit_duration, rating, feedback, created_at
        )
        SELECT
            cb.id,
            bill_sample.id,
            bill_sample.table_id,
            bill_sample.total_amount::numeric / 100.0,
            floor(bill_sample.total_amount::numeric / 100.0)::int,
            CASE WHEN v_business_key = 'core'
                THEN '[{"menu_item_id":"core-main-001","name":"Lemon Chicken Bowl"}]'
                ELSE '[{"menu_item_id":"ai-main-001","name":"Wagyu Striploin"}]'
            END,
            bill_sample.created_at,
            35 + ((cb.rn % 6) * 10),
            CASE WHEN cb.rn % 5 = 0 THEN 4 ELSE 5 END,
            CASE WHEN cb.rn % 5 = 0 THEN 'Demo visit with follow-up opportunity.' ELSE 'Seeded visit tied to a historical bill.' END,
            now()
        FROM (
            SELECT id, row_number() OVER (ORDER BY id) AS rn
            FROM customer_businesses
            WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
        ) cb
        JOIN LATERAL (
            SELECT id, table_id, total_amount, created_at
            FROM bills
            WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
              AND status IN ('paid', 'closed')
            ORDER BY created_at DESC, id DESC
            OFFSET ((cb.rn - 1) % CASE WHEN v_business_key = 'core' THEN 53 ELSE 90 END)
            LIMIT 1
        ) bill_sample ON true;
    END LOOP;

    UPDATE counters c
    SET current_bill_id = b.id
    FROM bills b
    WHERE c.business_id = b.business_id
      AND b.status = 'open'
      AND c.current_bill_id IS NULL
      AND ((c.business_id = v_core_id AND b.bill_number = 'CORE-DEMO-063') OR (c.business_id = v_ai_id AND b.bill_number = 'AI-DEMO-097'));

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        INSERT INTO inventory_settings (business_id, inventory_enabled, auto_deduct_on_order_approval, low_stock_warnings_enabled, availability_sync_mode, created_at, updated_at)
        VALUES (
            CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
            true, true, true, 'warn', now(), now()
        );

        INSERT INTO inventory_items (business_id, name, sku, category, unit, current_quantity, reorder_threshold, cost_per_unit, is_active, created_at, updated_at)
        SELECT
            CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
            name,
            upper(v_business_key) || '-INV-' || lpad(ord::text, 3, '0'),
            category,
            unit_label,
            CASE WHEN ord % 11 = 0 THEN 0 WHEN ord % 5 = 0 OR (v_business_key = 'core' AND ord = 7) THEN 3 ELSE 40 + ord * 2 END,
            CASE WHEN ord % 5 = 0 OR (v_business_key = 'core' AND ord = 7) THEN 8 ELSE 12 END,
            CASE WHEN v_business_key = 'core' THEN 1.25 + ord ELSE 8.5 + ord * 1.7 END,
            true,
            now() - interval '35 days',
            now()
        -- Each item is paired with a category that matches what it actually
        -- is. Previously categories were bucketed by ordinal position which
        -- left items like "Burger buns" labeled as 'beverage'.
        FROM ROWS FROM (
            unnest(
                CASE WHEN v_business_key = 'core' THEN
                    ARRAY[
                        'Chicken breast','Falafel mix','Quinoa','Rice','Salmon fillet',
                        'Beef patty','Mushrooms','Cauliflower','Burrata','Pita bread',
                        'Cold brew concentrate','Lemons','Mint','Oat milk','Chocolate',
                        'Coconut cream','Burger buns','Fries potato','Compostable bowls','Cup lids',
                        'Delivery bags','Napkins','Tahini','Kids boxes'
                    ]
                ELSE
                    ARRAY[
                        'Wagyu striploin','Sea bass','Lamb shoulder','Duck breast','Prawns',
                        'Tuna loin','Saffron','Truffle oil','Labneh','Burrata',
                        'Halloumi','Caviar cream','Mushroom blend','Arborio rice','Eggplant',
                        'Chickpeas','Rose syrup','Cardamom','Pomegranate','Rosemary',
                        'Arabic coffee','Jasmine tea','Mocktail bottles','Premium delivery boxes','VIP napkins',
                        'Event candles','Chef tasting plates','Valet tags','Date syrup','Pistachio',
                        'Coconut sorbet base','Black garlic','Harissa','Cucumber','Peach puree','Tonic water'
                    ]
                END
            ),
            unnest(
                CASE WHEN v_business_key = 'core' THEN
                    ARRAY[
                        'meat','dry_goods','dry_goods','dry_goods','seafood',
                        'meat','produce','produce','dairy','bakery',
                        'beverage','produce','produce','dairy','dry_goods',
                        'dairy','bakery','produce','packaging','packaging',
                        'packaging','packaging','dry_goods','packaging'
                    ]
                ELSE
                    ARRAY[
                        'meat','seafood','meat','meat','seafood',
                        'seafood','dry_goods','dry_goods','dairy','dairy',
                        'dairy','dairy','produce','dry_goods','produce',
                        'dry_goods','dry_goods','dry_goods','produce','produce',
                        'beverage','beverage','beverage','packaging','packaging',
                        'event_supply','event_supply','event_supply','dry_goods','dry_goods',
                        'frozen','produce','dry_goods','produce','beverage','beverage'
                    ]
                END
            ),
            -- Units paired with the item rather than computed from ordinal
            -- position. Previously `ord % 5` could label "Cup lids" as
            -- "liter" or "Pita bread" as "liter" — operators saw inventory
            -- warnings like "Cup lids: 3 liters remaining" which is nonsense.
            unnest(
                CASE WHEN v_business_key = 'core' THEN
                    ARRAY[
                        'kg','kg','kg','kg','kg',
                        'unit','kg','unit','unit','unit',
                        'liter','unit','g','liter','kg',
                        'ml','unit','kg','unit','unit',
                        'unit','box','kg','unit'
                    ]
                ELSE
                    ARRAY[
                        'kg','kg','kg','kg','kg',
                        'kg','g','ml','kg','unit',
                        'kg','ml','kg','kg','kg',
                        'kg','liter','g','kg','g',
                        'g','g','unit','unit','box',
                        'box','unit','unit','liter','kg',
                        'kg','kg','g','kg','liter','liter'
                    ]
                END
            )
        ) WITH ORDINALITY AS t(name, category, unit_label, ord);

        INSERT INTO inventory_recipes (business_id, menu_item_id, menu_item_name, inventory_item_id, quantity_required, created_at, updated_at)
        SELECT
            CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
            mi.item_id,
            mi.item_name,
            inv.id,
            CASE WHEN mi.price > 100 THEN 0.7 ELSE 1.2 END,
            now(),
            now()
        FROM (
            SELECT *, row_number() OVER (ORDER BY category_sort, item_sort) AS rn
            FROM demo_seed_menu_items
            WHERE business_key = v_business_key
            LIMIT CASE WHEN v_business_key = 'core' THEN 18 ELSE 28 END
        ) mi
        JOIN LATERAL (
            SELECT id
            FROM inventory_items
            WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
            ORDER BY id
            OFFSET ((mi.rn - 1) % (CASE WHEN v_business_key = 'core' THEN 24 ELSE 36 END))
            LIMIT 1
        ) inv ON true;

        INSERT INTO inventory_movements (business_id, inventory_item_id, movement_type, quantity_delta, quantity_before, quantity_after, reason, actor, menu_item_id, menu_item_name, created_at, updated_at)
        SELECT business_id, id, 'purchase', current_quantity + 20, 0, current_quantity + 20, 'Initial demo stock purchase', 'demo-seed', '', '', now() - interval '35 days', now() - interval '35 days'
        FROM inventory_items
        WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END;

        INSERT INTO inventory_movements (business_id, inventory_item_id, movement_type, quantity_delta, quantity_before, quantity_after, reason, actor, menu_item_id, menu_item_name, created_at, updated_at)
        SELECT business_id, id, CASE WHEN row_number() OVER (ORDER BY id) % 5 = 0 THEN 'waste' ELSE 'order_consumption' END,
               -5, current_quantity + 5, current_quantity, 'Demo stock movement', 'demo-seed', '', '', now() - interval '5 days', now() - interval '5 days'
        FROM inventory_items
        WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END;

        WITH scoped_items AS (
            SELECT id, business_id, name, current_quantity, row_number() OVER (ORDER BY id) AS rn
            FROM inventory_items
            WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
            ORDER BY id
            LIMIT 4
        ),
        movement_cases AS (
            SELECT 1 AS rn, 'manual_adjustment' AS movement_type, -2::numeric AS delta, 'Manual count adjustment after prep audit.' AS reason
            UNION ALL SELECT 2, 'restock', 18::numeric, 'Supplier restock received.'
            UNION ALL SELECT 3, 'correction', 4::numeric, 'Corrected vendor invoice quantity.'
            UNION ALL SELECT 4, 'order_restoration', 2::numeric, 'Cancelled order restored stock.'
        )
        INSERT INTO inventory_movements (business_id, inventory_item_id, movement_type, quantity_delta, quantity_before, quantity_after, reason, actor, menu_item_id, menu_item_name, created_at, updated_at)
        SELECT
            scoped_items.business_id,
            scoped_items.id,
            movement_cases.movement_type,
            movement_cases.delta,
            scoped_items.current_quantity - movement_cases.delta,
            scoped_items.current_quantity,
            movement_cases.reason,
            'demo-seed',
            '',
            scoped_items.name,
            now() - interval '3 days',
            now() - interval '3 days'
        FROM scoped_items
        JOIN movement_cases ON movement_cases.rn = scoped_items.rn;
    END LOOP;

    INSERT INTO manual_ledger_entries (business_id, entry_type, category, amount, currency, occurred_at, description, notes, reference, created_by_user_id, voided_at, voided_by_user_id, created_at, updated_at) VALUES
        (v_core_id, 'expense', 'rent', 4200, 'USD', now() - interval '31 days', 'Monthly rent', 'Core demo fixed expense', 'CORE-LEDGER-RENT', v_owner_id, NULL, NULL, now(), now()),
        (v_core_id, 'expense', 'utilities', 780, 'USD', now() - interval '26 days', 'Utilities', 'Electricity and water', 'CORE-LEDGER-UTIL', v_owner_id, NULL, NULL, now(), now()),
        (v_core_id, 'expense', 'supplier', 2250, 'USD', now() - interval '22 days', 'Supplier purchase', 'Ingredients and packaging', 'CORE-LEDGER-SUP', v_owner_id, NULL, NULL, now(), now()),
        (v_core_id, 'expense', 'marketing', 640, 'USD', now() - interval '18 days', 'Local marketing', 'QR flyer campaign', 'CORE-LEDGER-MKT', v_owner_id, NULL, NULL, now(), now()),
        (v_core_id, 'expense', 'staff_meal', 210, 'USD', now() - interval '16 days', 'Staff meal expense', 'Team meal and shift food', 'CORE-LEDGER-MEAL', v_owner_id, NULL, NULL, now(), now()),
        (v_core_id, 'income', 'catering', 1850, 'USD', now() - interval '12 days', 'Office catering', 'Manual income entry', 'CORE-LEDGER-CAT', v_owner_id, NULL, NULL, now(), now()),
        (v_core_id, 'income', 'event_deposit', 600, 'USD', now() - interval '7 days', 'Event deposit', 'Future event deposit', 'CORE-LEDGER-EVT', v_owner_id, NULL, NULL, now(), now()),
        (v_core_id, 'expense', 'duplicate', 780, 'USD', now() - interval '26 days', 'Voided duplicate utilities', 'Voided for accounting demo', 'CORE-LEDGER-VOID', v_owner_id, now() - interval '25 days', v_owner_id, now(), now()),
        (v_ai_id, 'expense', 'rent', 58000, 'AED', now() - interval '31 days', 'Premium lounge rent', 'AI Pro fixed expense', 'AI-LEDGER-RENT', v_owner_id, NULL, NULL, now(), now()),
        (v_ai_id, 'expense', 'supplier', 44000, 'AED', now() - interval '24 days', 'Premium supplier purchase', 'Wagyu, seafood, mocktail stock', 'AI-LEDGER-SUP', v_owner_id, NULL, NULL, now(), now()),
        (v_ai_id, 'expense', 'marketing', 12800, 'AED', now() - interval '17 days', 'Influencer campaign', 'AI Pro marketing demo', 'AI-LEDGER-MKT', v_owner_id, NULL, NULL, now(), now()),
        (v_ai_id, 'income', 'private_room', 18500, 'AED', now() - interval '10 days', 'Private room rental', 'High-value income stream', 'AI-LEDGER-ROOM', v_owner_id, NULL, NULL, now(), now()),
        (v_ai_id, 'income', 'event_deposit', 9200, 'AED', now() - interval '6 days', 'Corporate event deposit', 'Future VIP event', 'AI-LEDGER-EVT', v_owner_id, NULL, NULL, now(), now()),
        (v_ai_id, 'expense', 'duplicate', 44000, 'AED', now() - interval '24 days', 'Voided duplicate supplier purchase', 'Voided for accounting demo', 'AI-LEDGER-VOID', v_owner_id, now() - interval '23 days', v_owner_id, now(), now());

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        INSERT INTO payroll_runs (business_id, period_start, period_end, status, paid_at, paid_by_user_id, notes, gross_total, bonus_total, deduction_total, net_total, created_by_user_id, created_at, updated_at)
        VALUES (
            CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
            date_trunc('month', now() - interval '1 month'),
            date_trunc('month', now()) - interval '1 day',
            'paid',
            now() - interval '8 days',
            v_owner_id,
            'Previous-period paid payroll demo.',
            CASE WHEN v_business_key = 'core' THEN 18200 ELSE 88500 END,
            CASE WHEN v_business_key = 'core' THEN 900 ELSE 4500 END,
            CASE WHEN v_business_key = 'core' THEN 650 ELSE 3200 END,
            CASE WHEN v_business_key = 'core' THEN 18450 ELSE 89800 END,
            v_owner_id,
            now() - interval '12 days',
            now()
        )
        RETURNING id INTO v_order_id;

        INSERT INTO payroll_line_items (payroll_run_id, business_id, payee_type, staff_id, payee_name, gross_amount, bonus_amount, deduction_amount, net_amount, notes, created_at, updated_at)
        SELECT v_order_id, s.business_id, 'staff', s.id, s.name,
               CASE WHEN v_business_key = 'core' THEN 2400 + row_number() OVER (ORDER BY s.id) * 200 ELSE 6800 + row_number() OVER (ORDER BY s.id) * 450 END,
               CASE WHEN row_number() OVER (ORDER BY s.id) <= 2 THEN 250 ELSE 0 END,
               CASE WHEN row_number() OVER (ORDER BY s.id) % 3 = 0 THEN 100 ELSE 0 END,
               CASE WHEN v_business_key = 'core' THEN 2400 + row_number() OVER (ORDER BY s.id) * 200 ELSE 6800 + row_number() OVER (ORDER BY s.id) * 450 END,
               'Seeded staff payroll line',
               now(), now()
        FROM staff s
        WHERE s.business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
          AND s.is_active = true
        LIMIT CASE WHEN v_business_key = 'core' THEN 5 ELSE 9 END;

        INSERT INTO payroll_line_items (payroll_run_id, business_id, payee_type, staff_id, payee_name, gross_amount, bonus_amount, deduction_amount, net_amount, notes, created_at, updated_at)
        VALUES (v_order_id, CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END, 'contractor', NULL,
                CASE WHEN v_business_key = 'core' THEN 'Core Cleaning Contractor' ELSE 'AI Event Florist' END,
                CASE WHEN v_business_key = 'core' THEN 650 ELSE 3200 END, 0, 0,
                CASE WHEN v_business_key = 'core' THEN 650 ELSE 3200 END, 'Contractor line item demo', now(), now());

        INSERT INTO payroll_runs (business_id, period_start, period_end, status, notes, gross_total, bonus_total, deduction_total, net_total, created_by_user_id, created_at, updated_at)
        VALUES (
            CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
            date_trunc('month', now()),
            date_trunc('month', now()) + interval '1 month' - interval '1 day',
            'draft',
            'Current-period draft payroll demo.',
            CASE WHEN v_business_key = 'core' THEN 9800 ELSE 44200 END,
            0, 0,
            CASE WHEN v_business_key = 'core' THEN 9800 ELSE 44200 END,
            v_owner_id,
            now() - interval '2 days',
            now()
        );
    END LOOP;

    -- Money columns (default_delivery_fee, free_delivery_threshold,
    -- minimum_order_amount) are int64 cents in the Go model post-Foundation Task 3.
    -- Values here are stored as cents (e.g. $3.99 → 399, $15.00 → 1500).
    -- payment_mode is intentionally omitted: the empty-string default means
    -- "derive at read time" (online when the business has a payment method, else COD).
    INSERT INTO delivery_settings (
        business_id, delivery_enabled, in_house_delivery_enabled, third_party_enabled,
        uber_eats_enabled, doordash_enabled, grubhub_enabled, uber_eats_api_key, doordash_api_key, grubhub_api_key,
        default_delivery_fee, free_delivery_threshold, minimum_order_amount,
        max_delivery_radius, estimated_prep_time, max_concurrent_deliveries, delivery_hours_same_as_business,
        delivery_start_time, delivery_end_time, delivery_instructions, delivery_zones, external_partner_links,
        auto_assign_drivers, created_at, updated_at
    )
    VALUES
        (v_core_id, true, true, true, true, true, false, 'demo_ubereats_key_core', 'demo_doordash_key_core', '',
         399, 5500, 1500, 6.5, 15, 5, false, '10:30', '22:00',
         'Core delivery uses in-house first, partner fallback second.',
         '[{"name":"Near Core","fee":399},{"name":"Midtown","fee":599},{"name":"Out of zone","fee":1299}]'::jsonb,
         '[{"name":"Uber Eats","url":"https://www.ubereats.com/store/demo-core-business","provider_key":"ubereats"},{"name":"DoorDash","url":"https://www.doordash.com/store/demo-core-business","provider_key":"doordash"},{"name":"Deliveroo","url":"https://deliveroo.com/demo-core-business","provider_key":"deliveroo"},{"name":"Talabat","url":"https://www.talabat.com/store/payverge-demo","provider_key":"talabat"},{"name":"Careem","url":"https://www.careem.com/demo-core-business","provider_key":"careem"},{"name":"Zomato","url":"https://www.zomato.com/demo-core-business","provider_key":"zomato"}]'::jsonb,
         true, now(), now()),
        (v_ai_id, true, true, true, true, true, false, 'demo_ubereats_key_ai', 'demo_doordash_key_ai', '',
         1800, 35000, 9000, 15.0, 35, 8, false, '11:30', '01:30',
         'AI Pro delivery includes hotel concierge and premium packaging instructions.',
         '[{"name":"Hotel District","fee":2200},{"name":"Business District","fee":1800},{"name":"Premium Far Zone","fee":4500}]'::jsonb,
         '[{"name":"Careem","url":"https://www.careem.com/demo-ai-business","provider_key":"careem"},{"name":"Deliveroo","url":"https://deliveroo.com/demo-ai-business","provider_key":"deliveroo"},{"name":"DoorDash","url":"https://www.doordash.com/store/demo-ai-business","provider_key":"doordash"},{"name":"Uber Eats","url":"https://www.ubereats.com/store/demo-ai-business","provider_key":"ubereats"},{"name":"Talabat","url":"https://www.talabat.com/store/payverge-ai-demo","provider_key":"talabat"},{"name":"Zomato","url":"https://www.zomato.com/demo-ai-business","provider_key":"zomato"}]'::jsonb,
         true, now(), now())
    ON CONFLICT (business_id) DO UPDATE SET
        delivery_enabled = EXCLUDED.delivery_enabled,
        in_house_delivery_enabled = EXCLUDED.in_house_delivery_enabled,
        third_party_enabled = EXCLUDED.third_party_enabled,
        default_delivery_fee = EXCLUDED.default_delivery_fee,
        free_delivery_threshold = EXCLUDED.free_delivery_threshold,
        minimum_order_amount = EXCLUDED.minimum_order_amount,
        estimated_prep_time = EXCLUDED.estimated_prep_time,
        max_concurrent_deliveries = EXCLUDED.max_concurrent_deliveries,
        external_partner_links = EXCLUDED.external_partner_links,
        updated_at = now();

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        INSERT INTO delivery_zones (business_id, name, description, boundaries, delivery_fee, minimum_order_amount, estimated_time, priority, cutoff_buffer_minutes, is_active, operating_hours, created_at, updated_at)
        SELECT CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
               zone_name, zone_desc,
               z.boundaries,
               fee, min_order, eta, ord, cutoff, true,
               '{"mon":"10:30-22:00","fri":"10:30-23:00"}',
               now(), now()
        -- delivery_fee and minimum_order_amount are int64 cents in Go model.
        -- $3.99→399, $18.00→1800, $25.00→2500, $12.99→1299, $50.00→5000,
        -- $22.00→2200, $90.00→9000, $18.00→1800, $120.00→12000, $45.00→4500, $240.00→24000.
        FROM (
            -- Boundaries use the flat {key: [values]} format that matchDeliveryZone
            -- in delivery_v1.go expects. Earlier versions of this seed wrapped the
            -- values in a GeoJSON FeatureCollection, but the matcher unmarshals as
            -- map[string][]string and silently fell through, producing
            -- "zone_unavailable" for every quote.
            SELECT * FROM (VALUES
                ('Near Core','Fast local delivery',
                 '{"postal_codes":["10001","10011","10014"]}'::text,
                 399,1500,25,10,1),
                ('Midtown','Core mid-distance zone',
                 '{"postal_codes":["10019","10036","10018"]}'::text,
                 599,2500,35,15,2),
                ('Out of zone quote','Manual quote validation zone',
                 '{"cities":["Jersey City","Hoboken"]}'::text,
                 1299,5000,55,25,3)
            ) AS core(zone_name, zone_desc, boundaries, fee, min_order, eta, cutoff, ord)
            WHERE v_business_key = 'core'
            UNION ALL
            SELECT * FROM (VALUES
                ('Hotel District','Hotel concierge delivery',
                 '{"cities":["DIFC","Downtown Dubai"]}'::text,
                 2200,9000,35,20,1),
                ('Business District','Corporate tower deliveries',
                 '{"cities":["Business Bay","Dubai Mall"]}'::text,
                 1800,12000,40,25,2),
                ('Premium Far Zone','Out-of-radius quote scenario',
                 '{"cities":["Marina","JLT","JBR"]}'::text,
                 4500,24000,70,35,3)
            ) AS ai(zone_name, zone_desc, boundaries, fee, min_order, eta, cutoff, ord)
            WHERE v_business_key = 'ai'
        ) z;

        FOR v_i IN 1..CASE WHEN v_business_key = 'core' THEN 4 ELSE 6 END LOOP
            INSERT INTO delivery_drivers (
                business_id, name, phone, email, license_number, vehicle_type, vehicle_plate, status,
                is_available, current_latitude, current_longitude, "current_timestamp",
                total_deliveries, completed_deliveries, average_rating, total_earnings,
                is_active, last_active_at, created_at, updated_at
            )
            VALUES (
                CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
                CASE WHEN v_business_key = 'core' THEN 'Core Driver ' || v_i ELSE 'AI Driver ' || v_i END,
                CASE WHEN v_business_key = 'core' THEN '+1 555 401 ' || lpad(v_i::text,4,'0') ELSE '+1 555 602 ' || lpad(v_i::text,4,'0') END,
                'driver' || lpad(v_i::text,2,'0') || '@' || CASE WHEN v_business_key = 'core' THEN 'core-demo.payverge.example' ELSE 'ai-demo.payverge.example' END,
                upper(v_business_key) || '-LIC-' || lpad(v_i::text,3,'0'),
                CASE WHEN v_i % 5 = 0 THEN 'van' WHEN v_i % 4 = 0 THEN 'car' WHEN v_i % 3 = 0 THEN 'motorcycle' ELSE 'scooter' END,
                upper(v_business_key) || '-' || (100 + v_i),
                (ARRAY['online','busy','on_break','offline','online','busy'])[v_i],
                v_i <> 4,
                CASE WHEN v_business_key = 'core' THEN 40.738 + (v_i::numeric / 1000) ELSE 25.211 + (v_i::numeric / 1000) END,
                CASE WHEN v_business_key = 'core' THEN -73.999 - (v_i::numeric / 1000) ELSE 55.279 + (v_i::numeric / 1000) END,
                now() - interval '5 minutes',
                12 + v_i * 4,
                10 + v_i * 3,
                4.2 + (v_i::numeric / 10),
                CASE WHEN v_business_key = 'core' THEN 120 + v_i * 20 ELSE 600 + v_i * 75 END,
                true,
                now() - ((v_i % 5) || ' hours')::interval,
                now() - interval '60 days',
                now()
            );
        END LOOP;
    END LOOP;

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        FOR v_i IN 1..CASE WHEN v_business_key = 'core' THEN 10 ELSE 16 END LOOP
            v_status := v_delivery_statuses[((v_i - 1) % array_length(v_delivery_statuses, 1)) + 1];
            SELECT id INTO v_bill_id FROM bills
            WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
            ORDER BY id OFFSET (v_i - 1) LIMIT 1;
            SELECT id INTO v_driver_id FROM delivery_drivers
            WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
            ORDER BY id OFFSET ((v_i - 1) % (CASE WHEN v_business_key = 'core' THEN 4 ELSE 6 END)) LIMIT 1;
            SELECT id INTO v_zone_id FROM delivery_zones
            WHERE business_id = CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END
            ORDER BY id OFFSET ((v_i - 1) % 3) LIMIT 1;
            SELECT id INTO v_customer_id FROM customers
            WHERE email LIKE CASE WHEN v_business_key = 'core' THEN '%@core-demo.payverge.example' ELSE '%@ai-demo.payverge.example' END
            ORDER BY id OFFSET ((v_i - 1) % (CASE WHEN v_business_key = 'core' THEN 14 ELSE 24 END)) LIMIT 1;

            INSERT INTO delivery_orders (
                business_id, bill_id, customer_id, zone_id, delivery_number, delivery_type, status, priority, fulfillment_mode,
                driver_id, assigned_at, customer_name, customer_phone, customer_email,
                delivery_street, delivery_apartment, delivery_city, delivery_state, delivery_postal_code, delivery_country, delivery_formatted_address,
                pickup_latitude, pickup_longitude, pickup_timestamp, dropoff_latitude, dropoff_longitude, dropoff_timestamp,
                current_latitude, current_longitude, "current_timestamp",
                estimated_pickup_time, actual_pickup_time, estimated_delivery_time, actual_delivery_time,
                delivery_fee, driver_tip, platform_fee, delivery_instructions, contactless_delivery, leave_at_door,
                signature_url, photo_url, delivery_code, quote_metadata, cutoff_at,
                external_order_id, external_tracking_url, cancelled_at, cancelled_by, cancellation_reason,
                customer_rating, customer_feedback, driver_rating, created_at, updated_at
            )
            VALUES (
                CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
                v_bill_id, v_customer_id, v_zone_id,
                CASE WHEN v_business_key = 'core' THEN 'CORE-DEL-' ELSE 'AI-DEL-' END || lpad(v_i::text, 4, '0'),
                CASE WHEN v_i % 4 = 0 THEN 'doordash' ELSE 'in_house' END,
                v_status,
                CASE WHEN v_i % 5 = 0 THEN 'urgent' WHEN v_i % 3 = 0 THEN 'high' ELSE 'normal' END,
                CASE WHEN v_i % 4 = 0 THEN 'third_party' ELSE 'in_house' END,
                CASE WHEN v_status IN ('assigned','picked_up','in_transit','nearby','delivered') THEN v_driver_id ELSE NULL END,
                CASE WHEN v_status IN ('assigned','picked_up','in_transit','nearby','delivered') THEN now() - interval '45 minutes' ELSE NULL END,
                CASE WHEN v_business_key = 'core' THEN 'Core Delivery Guest ' || v_i ELSE 'AI Delivery Guest ' || v_i END,
                CASE WHEN v_business_key = 'core' THEN '+1 555 501 ' || lpad(v_i::text,4,'0') ELSE '+1 555 702 ' || lpad(v_i::text,4,'0') END,
                'delivery' || lpad(v_i::text,2,'0') || '@' || CASE WHEN v_business_key = 'core' THEN 'core-demo.payverge.example' ELSE 'ai-demo.payverge.example' END,
                CASE WHEN v_business_key = 'core' THEN (200 + v_i)::text || ' Delivery Lane' ELSE 'Hotel Demo ' || v_i END,
                CASE WHEN v_i % 2 = 0 THEN 'Suite ' || v_i ELSE '' END,
                CASE WHEN v_business_key = 'core' THEN 'New York' ELSE 'Dubai' END,
                CASE WHEN v_business_key = 'core' THEN 'NY' ELSE 'Dubai' END,
                CASE WHEN v_business_key = 'core' THEN '100' || lpad(v_i::text,2,'0') ELSE '00000' END,
                CASE WHEN v_business_key = 'core' THEN 'United States' ELSE 'United Arab Emirates' END,
                CASE WHEN v_business_key = 'core' THEN 'Delivery Lane, New York' ELSE 'Hotel Demo, Dubai' END,
                CASE WHEN v_business_key = 'core' THEN 40.7382 ELSE 25.2117 END,
                CASE WHEN v_business_key = 'core' THEN -73.9999 ELSE 55.2795 END,
                now(),
                CASE WHEN v_business_key = 'core' THEN 40.7400 + (v_i::numeric / 1000) ELSE 25.2140 + (v_i::numeric / 1000) END,
                CASE WHEN v_business_key = 'core' THEN -74.0010 - (v_i::numeric / 1000) ELSE 55.2820 + (v_i::numeric / 1000) END,
                now(),
                CASE WHEN v_status IN ('picked_up','in_transit','nearby') THEN CASE WHEN v_business_key = 'core' THEN 40.7390 ELSE 25.2130 END ELSE NULL END,
                CASE WHEN v_status IN ('picked_up','in_transit','nearby') THEN CASE WHEN v_business_key = 'core' THEN -74.0000 ELSE 55.2810 END ELSE NULL END,
                CASE WHEN v_status IN ('picked_up','in_transit','nearby') THEN now() - interval '5 minutes' ELSE NULL END,
                now() + interval '10 minutes',
                CASE WHEN v_status IN ('picked_up','in_transit','nearby','delivered') THEN now() - interval '30 minutes' ELSE NULL END,
                now() + interval '45 minutes',
                CASE WHEN v_status = 'delivered' THEN now() - interval '10 minutes' ELSE NULL END,
                -- delivery_fee, driver_tip, platform_fee stored as int64 cents.
                -- Core: base $3.99 (399¢) + v_i¢ increments. AI: base $18.00 (1800¢) + v_i*2¢.
                CASE WHEN v_business_key = 'core' THEN 399 + v_i ELSE 1800 + v_i * 2 END,
                CASE WHEN v_business_key = 'core' THEN 250 ELSE 1200 END,
                CASE WHEN v_business_key = 'core' THEN 125 ELSE 500 END,
                CASE WHEN v_i % 3 = 0 THEN 'Contactless delivery.' ELSE 'Call on arrival.' END,
                v_i % 2 = 0, v_i % 3 = 0,
                CASE WHEN v_status = 'delivered' THEN 'https://images.unsplash.com/photo-1551782450-a2132b4ba21d?w=600&q=75' ELSE '' END,
                CASE WHEN v_status = 'delivered' THEN 'https://images.unsplash.com/photo-1551782450-a2132b4ba21d?w=600&q=75' ELSE '' END,
                upper(v_business_key) || '-CODE-' || lpad(v_i::text,4,'0'),
                jsonb_build_object('source','demo_seed','provider_key', CASE WHEN v_i % 4 = 0 THEN 'doordash' ELSE 'in_house' END),
                now() + interval '15 minutes',
                CASE WHEN v_i % 4 = 0 THEN 'ext-' || v_business_key || '-' || v_i ELSE '' END,
                CASE WHEN v_i % 4 = 0 THEN 'https://tracking.demo.payverge.example/' || v_business_key || '/' || v_i ELSE '' END,
                CASE WHEN v_status = 'cancelled' THEN now() - interval '20 minutes' ELSE NULL END,
                CASE WHEN v_status = 'cancelled' THEN 'staff' ELSE '' END,
                CASE WHEN v_status = 'cancelled' THEN 'Customer requested cancellation.' ELSE '' END,
                CASE WHEN v_status = 'delivered' THEN 5 ELSE NULL END,
                CASE WHEN v_status = 'delivered' THEN 'Great packaging.' ELSE '' END,
                CASE WHEN v_status = 'delivered' THEN 5 ELSE NULL END,
                now() - ((v_i % 18) || ' days')::interval,
                now()
            )
            RETURNING id INTO v_delivery_id;

            FOR v_j IN 1..COALESCE(array_position(v_delivery_statuses, v_status), 1) LOOP
                INSERT INTO delivery_status_history (delivery_order_id, status, notes, changed_by, created_at)
                VALUES (v_delivery_id, v_delivery_statuses[v_j], 'Seeded delivery status event.', 'demo-seed', now() - ((array_position(v_delivery_statuses, v_status) - v_j) || ' hours')::interval);
            END LOOP;

            IF v_status IN ('assigned','picked_up','in_transit','nearby') THEN
                UPDATE delivery_drivers SET current_delivery_id = v_delivery_id WHERE id = v_driver_id;
            END IF;
        END LOOP;
    END LOOP;

    FOR v_business_key IN SELECT unnest(ARRAY['core','ai']) LOOP
        INSERT INTO report_schedules (business_id, frequency, day_of_week, hour, minute, timezone, is_active, last_sent_at, next_send_at, created_at, updated_at) VALUES
            (CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END, 'daily', 0, 8, 0, CASE WHEN v_business_key = 'core' THEN 'America/New_York' ELSE 'Asia/Dubai' END, true, now() - interval '1 day', now() + interval '1 day', now(), now()),
            (CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END, 'weekly', 1, 9, 0, CASE WHEN v_business_key = 'core' THEN 'America/New_York' ELSE 'Asia/Dubai' END, true, now() - interval '7 days', now() + interval '7 days', now(), now());

        INSERT INTO business_plugins (business_id, plugin_id, is_enabled, config, created_at, updated_at)
        SELECT CASE WHEN v_business_key = 'core' THEN v_core_id ELSE v_ai_id END,
               p.id,
               CASE WHEN p.name IN ('usdc_payment','cross_chain_payment') THEN false ELSE true END,
               jsonb_build_object(
                   'demo', true,
                   'business_key', v_business_key,
                   'api_key', 'demo_' || p.name || '_' || v_business_key,
                   'chat_id', CASE WHEN p.name = 'telegram' THEN 'demo-chat-' || v_business_key ELSE '' END,
                   'alerts', jsonb_build_array('orders','payments','low_stock','reports')
               )::text,
               now(), now()
        FROM plugins p
        WHERE p.name IN ('usdc_payment','cross_chain_payment','stripe','paypal','mercadopago','telegram','trustpilot','daily_email_report','weekly_email_report');
    END LOOP;

    UPDATE business_plugins bp
    SET is_enabled = false,
        updated_at = now()
    FROM plugins p
    WHERE bp.plugin_id = p.id
      AND bp.business_id IN (v_core_id, v_ai_id)
      AND p.name IN ('usdc_payment', 'cross_chain_payment');

    INSERT INTO withdrawal_histories (business_id, transaction_hash, payment_amount, tip_amount, total_amount, withdrawal_address, blockchain_network, status, created_at, updated_at, confirmed_at) VALUES
        (v_core_id, '0x' || md5('core-withdrawal-pending') || md5('core-withdrawal-pending-extra'), 650.00, 92.00, 742.00, v_wallet, 'base-sepolia', 'pending', now() - interval '2 days', now(), NULL),
        (v_core_id, '0x' || md5('core-withdrawal-confirmed') || md5('core-withdrawal-confirmed-extra'), 1240.00, 188.00, 1428.00, v_wallet, 'base-sepolia', 'confirmed', now() - interval '15 days', now(), now() - interval '14 days'),
        (v_core_id, '0x' || md5('core-withdrawal-failed') || md5('core-withdrawal-failed-extra'), 210.00, 20.00, 230.00, v_wallet, 'base-sepolia', 'failed', now() - interval '8 days', now(), NULL),
        (v_ai_id, '0x' || md5('ai-withdrawal-pending') || md5('ai-withdrawal-pending-extra'), 15400.00, 1800.00, 17200.00, v_wallet, 'base-sepolia', 'pending', now() - interval '1 day', now(), NULL),
        (v_ai_id, '0x' || md5('ai-withdrawal-confirmed') || md5('ai-withdrawal-confirmed-extra'), 38200.00, 5200.00, 43400.00, v_wallet, 'base-sepolia', 'confirmed', now() - interval '12 days', now(), now() - interval '11 days'),
        (v_ai_id, '0x' || md5('ai-withdrawal-failed') || md5('ai-withdrawal-failed-extra'), 9200.00, 1200.00, 10400.00, v_wallet, 'base-sepolia', 'failed', now() - interval '5 days', now(), NULL);

    INSERT INTO webhook_events (provider, webhook_id, event_type, status, payload, error, received_at, processed_at, created_at, updated_at) VALUES
        ('stripe', 'demo_seed_plugin_stripe_success', 'checkout.session.completed', 'processed', '{"demo":true,"business":"core"}', '', now() - interval '5 days', now() - interval '5 days' + interval '2 minutes', now(), now()),
        ('paypal', 'demo_seed_paypal_success', 'PAYMENT.CAPTURE.COMPLETED', 'processed', '{"demo":true,"business":"core"}', '', now() - interval '4 days', now() - interval '4 days' + interval '2 minutes', now(), now()),
        ('mercadopago', 'demo_seed_mp_failed', 'payment.updated', 'failed', '{"demo":true,"business":"ai"}', 'Demo webhook validation failure', now() - interval '3 days', NULL, now(), now())
    ON CONFLICT (provider, webhook_id) DO UPDATE SET status = EXCLUDED.status, payload = EXCLUDED.payload, error = EXCLUDED.error, updated_at = now();

    FOR v_i IN 1..64 LOOP
        INSERT INTO page_views (session_id, page, referrer, user_agent, ip_address, country, city, device_type, browser, os, screen_width, screen_height, locale, timestamp, duration)
        VALUES (
            'pvseed-demo-session-' || lpad(v_i::text, 3, '0'),
            (ARRAY['/','/es','/b/demo-core-kitchen','/b/demo-ai-lounge','/b/demo-core-kitchen?lang=es','/dashboard','/business/register'])[((v_i - 1) % 7) + 1],
            CASE WHEN v_i % 4 = 0 THEN 'https://google.com' ELSE 'https://demo.payverge.example' END,
            'Mozilla/5.0 PayvergeDemo',
            '203.0.113.' || (v_i % 250),
            CASE WHEN v_i % 3 = 0 THEN 'United Arab Emirates' WHEN v_i % 2 = 0 THEN 'United States' ELSE 'Argentina' END,
            CASE WHEN v_i % 3 = 0 THEN 'Dubai' WHEN v_i % 2 = 0 THEN 'New York' ELSE 'Buenos Aires' END,
            CASE WHEN v_i % 3 = 0 THEN 'mobile' ELSE 'desktop' END,
            CASE WHEN v_i % 2 = 0 THEN 'Chrome' ELSE 'Safari' END,
            CASE WHEN v_i % 2 = 0 THEN 'macOS' ELSE 'iOS' END,
            CASE WHEN v_i % 3 = 0 THEN 390 ELSE 1440 END,
            CASE WHEN v_i % 3 = 0 THEN 844 ELSE 900 END,
            CASE WHEN v_i % 4 = 0 THEN 'es' WHEN v_i % 5 = 0 THEN 'ar' ELSE 'en' END,
            now() - ((v_i % 30) || ' days')::interval,
            30 + v_i * 3
        );

        INSERT INTO user_interactions (session_id, page, event_type, event_category, event_label, event_value, x_position, y_position, timestamp)
        VALUES (
            'pvseed-demo-session-' || lpad(v_i::text, 3, '0'),
            '/business/' || CASE WHEN v_i % 2 = 0 THEN 'demo-core-kitchen' ELSE 'demo-ai-lounge' END,
            CASE WHEN v_i % 5 = 0 THEN 'form_submit' WHEN v_i % 3 = 0 THEN 'qr_scan' ELSE 'click' END,
            CASE WHEN v_i % 5 = 0 THEN 'reservation' WHEN v_i % 3 = 0 THEN 'table' ELSE 'cta' END,
            CASE WHEN v_i % 2 = 0 THEN 'core-demo-action' ELSE 'ai-demo-action' END,
            jsonb_build_object('seed','payverge-demo-seed-fixture','index',v_i)::text,
            120 + v_i, 240 + v_i, now() - ((v_i % 20) || ' days')::interval
        );

        IF v_i % 4 = 0 THEN
            INSERT INTO conversion_events (session_id, conversion_type, value, metadata, timestamp)
            VALUES (
                'pvseed-demo-session-' || lpad(v_i::text, 3, '0'),
                CASE WHEN v_i % 8 = 0 THEN 'payment' WHEN v_i % 12 = 0 THEN 'reservation' ELSE 'registration' END,
                CASE WHEN v_i % 8 = 0 THEN 49.00 ELSE 0 END,
                jsonb_build_object('seed','payverge-demo-seed-fixture','business', CASE WHEN v_i % 2 = 0 THEN 'core' ELSE 'ai' END)::text,
                now() - ((v_i % 20) || ' days')::interval
            );
        END IF;
    END LOOP;

    INSERT INTO session_summaries (session_id, first_seen, last_seen, total_page_views, total_interactions, total_duration, pages_visited, converted, conversion_type, device_type, country)
    SELECT 'pvseed-demo-summary-' || lpad(i::text, 3, '0'),
           now() - (i || ' days')::interval,
           now() - (i || ' days')::interval + interval '12 minutes',
           2 + (i % 6),
           1 + (i % 5),
           120 + i * 20,
           '["/","/b/demo-ai-lounge","/dashboard"]',
           i % 3 = 0,
           CASE WHEN i % 3 = 0 THEN 'payment' ELSE '' END,
           CASE WHEN i % 2 = 0 THEN 'desktop' ELSE 'mobile' END,
           CASE WHEN i % 3 = 0 THEN 'United Arab Emirates' WHEN i % 2 = 0 THEN 'United States' ELSE 'Argentina' END
    FROM generate_series(1, 18) AS s(i)
    ON CONFLICT (session_id) DO UPDATE SET last_seen = EXCLUDED.last_seen, total_page_views = EXCLUDED.total_page_views;

    INSERT INTO error_logs (timestamp, source, component, function, error, message, stack, user_id, request_id, metadata, additional_info, created_at)
    VALUES
        (now() - interval '5 days', 'frontend', 'CheckoutWidget', 'submitPayment', 'DemoPaymentError', 'Demo failed card payment for admin error dashboard.', '', v_owner_id::text, 'pvseed-demo-error-001', '{"business":"core"}', '{"severity":"medium"}', now()),
        (now() - interval '4 days', 'backend', 'WebhookHandler', 'HandleWebhook', 'DemoWebhookRetry', 'Demo failed webhook retry item.', '', v_owner_id::text, 'pvseed-demo-error-002', '{"provider":"mercadopago"}', '{"severity":"high"}', now()),
        (now() - interval '3 days', 'integration', 'DeliveryQuote', 'QuoteDelivery', 'DemoOutOfZone', 'Demo out-of-zone quote validation.', '', v_owner_id::text, 'pvseed-demo-error-003', '{"business":"ai"}', '{"severity":"low"}', now()),
        (now() - interval '2 days', 'ai', 'AIWaiter', 'HandleAIWaiter', 'DemoAITimeout', 'Demo AI provider timeout.', '', v_owner_id::text, 'pvseed-demo-error-004', '{"business":"ai"}', '{"severity":"medium"}', now());

    FOR v_i IN 1..16 LOOP
        INSERT INTO ai_waiter_conversations (session_id, business_id, table_code, language, mode, status, is_paused, cart_items_added, created_at, updated_at)
        VALUES (
            CASE WHEN v_i <= 3 THEN 'whatsapp:demo:+155501010' || v_i ELSE 'ai-demo-session-' || lpad(v_i::text,3,'0') END,
            v_ai_id,
            CASE
                WHEN v_i = 2 THEN upper(substr(md5('payverge-fixture-ai-pro-table-2'), 1, 10))
                WHEN v_i <= 10 THEN upper(substr(md5('payverge-fixture-ai-pro-table-' || (((v_i - 1) % 16) + 1)::text), 1, 10))
                ELSE ''
            END,
            (ARRAY['en','ar','es','fr','zh'])[((v_i - 1) % 5) + 1],
            CASE WHEN v_i % 4 = 0 THEN 'concierge' ELSE 'ordering' END,
            CASE WHEN v_i BETWEEN 11 AND 14 THEN 'closed' ELSE 'active' END,
            v_i IN (15,16),
            CASE WHEN v_i % 3 = 0 THEN 2 ELSE 0 END,
            now() - ((v_i % 12) || ' hours')::interval,
            now()
        )
        RETURNING id INTO v_customer_business_id;

        INSERT INTO ai_waiter_messages (conversation_id, role, content, tool_calls, created_at) VALUES
            (v_customer_business_id, 'system', 'Demo AI waiter conversation seeded for Sofia.', '', now() - interval '30 minutes'),
            (v_customer_business_id, 'user', CASE WHEN v_i % 5 = 0 THEN 'I have a nut allergy. What should I order?' ELSE 'Can you recommend a premium dinner option?' END, '', now() - interval '20 minutes'),
            (v_customer_business_id, 'assistant', CASE WHEN v_i % 5 = 0 THEN 'I recommend the Saffron Sea Bass and will flag your nut allergy for staff.' ELSE 'The Wagyu Striploin pairs well with the Smoked Date Tonic.' END,
             '[{"name":"menu_search","arguments":{"dietary":"allergy_safe"}},{"name":"recommend_item","arguments":{"item_id":"ai-main-001"}}]', now() - interval '19 minutes');
    END LOOP;

    INSERT INTO menu_extraction_jobs (business_id, status, error_message, extracted_menu, image_count, created_at, updated_at)
    VALUES
        (v_ai_id, 'uploading', '', '{}', 2, now() - interval '4 days', now() - interval '4 days'),
        (v_ai_id, 'processing', '', '{}', 3, now() - interval '3 days', now() - interval '3 days'),
        (v_ai_id, 'completed', '', '{"categories":[{"name":"Imported Chef Menu"}]}', 4, now() - interval '2 days', now() - interval '2 days'),
        (v_ai_id, 'failed', 'Demo OCR quality failure', '{}', 1, now() - interval '1 day', now() - interval '1 day');

    FOR v_job_id IN SELECT id FROM menu_extraction_jobs WHERE business_id = v_ai_id ORDER BY id LOOP
        INSERT INTO menu_extraction_images (job_id, file_path, page_order, created_at) VALUES
            (v_job_id, 's3://demo-payverge/ai-menu/job-' || v_job_id || '-page-1.png', 1, now()),
            (v_job_id, 's3://demo-payverge/ai-menu/job-' || v_job_id || '-page-2.png', 2, now());
    END LOOP;

    INSERT INTO menu_wizard_sessions (business_id, status, generated_menu, config, created_at, updated_at)
    VALUES
        (v_ai_id, 'in_progress', '{}', '{"concept":"premium lounge","languages":["en","ar"]}', now() - interval '3 days', now() - interval '3 days')
    RETURNING id INTO v_wizard_id;
    INSERT INTO menu_wizard_messages (session_id, role, content, created_at) VALUES
        (v_wizard_id, 'user', 'Create a multilingual premium lounge menu with allergy-safe suggestions.', now() - interval '3 days'),
        (v_wizard_id, 'assistant', 'I will structure the menu around tasting, mains, mocktails, and late-night bites.', now() - interval '3 days' + interval '1 minute');

    INSERT INTO menu_wizard_sessions (business_id, status, generated_menu, config, created_at, updated_at)
    VALUES
        (v_ai_id, 'completed', '{"categories":[{"name":"AI Suggested Premium Plates"}]}', '{"concept":"AI menu import complete"}', now() - interval '8 days', now() - interval '8 days'),
        (v_ai_id, 'abandoned', '{}', '{"concept":"abandoned test"}', now() - interval '12 days', now() - interval '12 days');

    FOREACH v_status IN ARRAY ARRAY['Revenue opportunities this week','Low-stock risk before weekend service','VIP guest retention plan','Delivery margin review','Staffing and reservation load'] LOOP
        INSERT INTO director_console_threads (business_id, title, locale, last_message_at, created_at, updated_at)
        VALUES (v_ai_id, v_status, 'en', now() - interval '30 minutes', now() - interval '3 days', now())
        RETURNING id INTO v_thread_id;

        INSERT INTO director_console_messages (thread_id, business_id, role, locale, content, structured_response, model_name, latency_ms, feedback_vote, feedback_at, created_at, updated_at)
        VALUES
            (v_thread_id, v_ai_id, 'user', 'en', 'What should I focus on for this area?', '{}', '', 0, NULL, NULL, now() - interval '35 minutes', now()),
            (v_thread_id, v_ai_id, 'assistant', 'en', 'Focus on high-margin mocktails, VIP reservations, low-stock premium ingredients, and delivery cancellation patterns.',
             '{"cards":[{"title":"Review high-margin bundles","href":"/dashboard/inventory"},{"title":"Check VIP guests","href":"/dashboard/crm"}],"seed":"payverge-demo-seed-fixture"}',
             'gpt-demo-director', 1240,
             CASE WHEN v_status IN ('Revenue opportunities this week','VIP guest retention plan') THEN 'up' ELSE NULL END,
             CASE WHEN v_status IN ('Revenue opportunities this week','VIP guest retention plan') THEN now() - interval '20 minutes' ELSE NULL END,
             now() - interval '30 minutes', now());
    END LOOP;

    IF to_regclass('public.user_sessions') IS NOT NULL THEN
        INSERT INTO user_sessions (user_id, address, session_token, provider, ip_address, user_agent, revoked, expires_at, created_at, last_used_at, refresh_token, refresh_expires_at)
        VALUES
            (v_owner_id, v_wallet, 'pvseed-demo-revoked-core-session', 'google', '127.0.0.1', 'Payverge demo seed', true, now() - interval '1 day', now() - interval '8 days', now() - interval '2 days', 'pvseed-demo-refresh-core', now() - interval '1 day'),
            (v_owner_id, v_wallet, 'pvseed-demo-revoked-ai-session', 'wallet', '127.0.0.1', 'Payverge demo seed', true, now() - interval '2 days', now() - interval '10 days', now() - interval '3 days', 'pvseed-demo-refresh-ai', now() - interval '2 days')
        ON CONFLICT (session_token) DO UPDATE SET revoked = true, expires_at = EXCLUDED.expires_at;
    END IF;

    IF to_regclass('public.scheduler_states') IS NOT NULL THEN
        INSERT INTO scheduler_states ("key", created_at) VALUES
            ('pvseed-demo-daily-report-core-' || to_char(now(), 'YYYY-MM-DD'), now()),
            ('pvseed-demo-weekly-report-ai-' || to_char(now(), 'YYYY-MM-DD'), now())
        ON CONFLICT ("key") DO NOTHING;
    END IF;

    IF to_regclass('public.file_access') IS NOT NULL THEN
        INSERT INTO file_access (access_key, file_name, folder, expires_at, created_at) VALUES
            ('pvseed-demo-contract-core', 'core-demo-contract.pdf', 'demo-contracts', now() + interval '30 days', now()),
            ('pvseed-demo-contract-ai', 'ai-demo-contract.pdf', 'demo-contracts', now() + interval '30 days', now())
        ON CONFLICT (access_key) DO UPDATE SET expires_at = EXCLUDED.expires_at;
    END IF;

    SELECT id INTO v_admin_user_id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1;
    IF v_admin_user_id IS NOT NULL THEN
        INSERT INTO admin_actions (admin_user_id, target_user_id, action_type, details, created_at) VALUES
            (v_admin_user_id, v_owner_id, 'suspend_business', '{"seed":"payverge-demo-seed-fixture","business_id":"demo-ai-pro-business","reason":"demo admin lifecycle history"}'::jsonb, now() - interval '7 days'),
            (v_admin_user_id, v_owner_id, 'reactivate_business', '{"seed":"payverge-demo-seed-fixture","business_id":"demo-ai-pro-business"}'::jsonb, now() - interval '7 days' + interval '1 hour'),
            (v_admin_user_id, v_owner_id, 'reset_password', '{"seed":"payverge-demo-seed-fixture","reason":"demo admin user detail history"}'::jsonb, now() - interval '2 days');
    END IF;

    -- Keep the demo operational data fresh on every run. This makes financial
    -- transactions, guest activity, and testing workflows land on today's date
    -- while preserving useful ordering inside the day.
    WITH ranked AS (
        SELECT id, row_number() OVER (PARTITION BY business_id ORDER BY id) AS rn
        FROM bills
        WHERE business_id IN (v_core_id, v_ai_id)
    )
    UPDATE bills b
    SET created_at = v_demo_day + interval '8 hours' + (((ranked.rn - 1) % 12) * interval '1 hour') + ((ranked.rn % 6) * interval '7 minutes'),
        updated_at = v_demo_day + interval '8 hours' + (((ranked.rn - 1) % 12) * interval '1 hour') + ((ranked.rn % 6) * interval '7 minutes') + interval '20 minutes',
        closed_at = CASE
            WHEN b.status IN ('paid', 'closed') THEN v_demo_day + interval '8 hours' + (((ranked.rn - 1) % 12) * interval '1 hour') + ((ranked.rn % 6) * interval '7 minutes') + interval '55 minutes'
            ELSE NULL
        END
    FROM ranked
    WHERE b.id = ranked.id;

    UPDATE bill_items bi
    SET created_at = b.created_at + interval '3 minutes'
    FROM bills b
    WHERE bi.bill_id = b.id
      AND b.business_id IN (v_core_id, v_ai_id);

    WITH ranked AS (
        SELECT e.id, b.created_at, row_number() OVER (PARTITION BY e.bill_id ORDER BY e.id) AS rn
        FROM bill_history_events e
        JOIN bills b ON b.id = e.bill_id
        WHERE b.business_id IN (v_core_id, v_ai_id)
    )
    UPDATE bill_history_events e
    SET created_at = ranked.created_at + ((ranked.rn - 1) * interval '5 minutes')
    FROM ranked
    WHERE e.id = ranked.id;

    UPDATE orders o
    SET created_at = b.created_at + interval '8 minutes',
        updated_at = b.created_at + interval '16 minutes',
        approved_at = CASE WHEN o.status IN ('approved', 'in_kitchen', 'ready', 'delivered') THEN b.created_at + interval '12 minutes' ELSE NULL END,
        cancelled_at = CASE WHEN o.status = 'cancelled' THEN b.created_at + interval '15 minutes' ELSE NULL END
    FROM bills b
    WHERE o.bill_id = b.id
      AND b.business_id IN (v_core_id, v_ai_id);

    UPDATE payments p
    SET created_at = COALESCE(b.closed_at, b.created_at + interval '25 minutes'),
        updated_at = COALESCE(b.closed_at, b.created_at + interval '25 minutes')
    FROM bills b
    WHERE p.bill_id = b.id
      AND b.business_id IN (v_core_id, v_ai_id);

    UPDATE alternative_payments ap
    SET created_at = COALESCE(b.closed_at, b.created_at + interval '25 minutes'),
        updated_at = COALESCE(b.closed_at, b.created_at + interval '25 minutes'),
        confirmed_at = CASE WHEN ap.status = 'confirmed' THEN COALESCE(b.closed_at, b.created_at + interval '25 minutes') + interval '3 minutes' ELSE NULL END
    FROM bills b
    WHERE ap.bill_id = b.id
      AND b.business_id IN (v_core_id, v_ai_id);

    WITH ranked AS (
        SELECT id, status, row_number() OVER (PARTITION BY business_id ORDER BY id) AS rn
        FROM table_reservations
        WHERE business_id IN (v_core_id, v_ai_id)
    )
    UPDATE table_reservations r
    SET created_at = v_demo_day + interval '9 hours' + (((ranked.rn - 1) % 10) * interval '35 minutes'),
        updated_at = v_demo_day + interval '9 hours' + (((ranked.rn - 1) % 10) * interval '35 minutes') + interval '15 minutes',
        reservation_time = CASE
            WHEN ranked.status IN ('completed', 'cancelled', 'no_show', 'seated') THEN v_demo_day + interval '12 hours' + (((ranked.rn - 1) % 10) * interval '35 minutes')
            ELSE v_demo_day + interval '18 hours' + (((ranked.rn - 1) % 8) * interval '30 minutes')
        END,
        confirmed_at = CASE WHEN ranked.status IN ('confirmed', 'seated', 'completed') THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 10) * interval '35 minutes') ELSE NULL END,
        assigned_at = CASE WHEN ranked.status IN ('confirmed', 'seated', 'completed') THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 10) * interval '35 minutes') + interval '5 minutes' ELSE NULL END,
        seated_at = CASE WHEN ranked.status IN ('seated', 'completed') THEN v_demo_day + interval '12 hours' + (((ranked.rn - 1) % 10) * interval '35 minutes') ELSE NULL END,
        completed_at = CASE WHEN ranked.status = 'completed' THEN v_demo_day + interval '13 hours' + (((ranked.rn - 1) % 10) * interval '35 minutes') ELSE NULL END,
        cancelled_at = CASE WHEN ranked.status IN ('cancelled', 'no_show') THEN v_demo_day + interval '11 hours' + (((ranked.rn - 1) % 10) * interval '35 minutes') ELSE NULL END
    FROM ranked
    WHERE r.id = ranked.id;

    WITH ranked AS (
        SELECT h.id, r.created_at, row_number() OVER (PARTITION BY h.reservation_id ORDER BY h.id) AS rn
        FROM reservation_status_histories h
        JOIN table_reservations r ON r.id = h.reservation_id
        WHERE r.business_id IN (v_core_id, v_ai_id)
    )
    UPDATE reservation_status_histories h
    SET created_at = ranked.created_at + (ranked.rn * interval '10 minutes')
    FROM ranked
    WHERE h.id = ranked.id;

    WITH ranked AS (
        SELECT id, row_number() OVER (PARTITION BY business_id ORDER BY id) AS rn
        FROM customer_businesses
        WHERE business_id IN (v_core_id, v_ai_id)
    )
    UPDATE customer_businesses cb
    SET first_visit_at = v_demo_day + interval '9 hours' + (((ranked.rn - 1) % 8) * interval '30 minutes'),
        last_visit_at = v_demo_day + interval '13 hours' + (((ranked.rn - 1) % 8) * interval '30 minutes'),
        created_at = v_demo_day + interval '8 hours' + (((ranked.rn - 1) % 8) * interval '20 minutes'),
        updated_at = now()
    FROM ranked
    WHERE cb.id = ranked.id;

    UPDATE customer_addresses ca
    SET last_used_at = v_demo_day + interval '12 hours',
        created_at = v_demo_day + interval '8 hours',
        updated_at = now()
    WHERE business_id IN (v_core_id, v_ai_id);

    WITH ranked AS (
        SELECT id, status, row_number() OVER (PARTITION BY business_id ORDER BY id) AS rn
        FROM customer_communications
        WHERE business_id IN (v_core_id, v_ai_id)
    )
    UPDATE customer_communications cc
    SET sent_at = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 8) * interval '25 minutes'),
        delivered_at = CASE WHEN ranked.status <> 'failed' THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 8) * interval '25 minutes') + interval '10 minutes' ELSE NULL END,
        opened_at = CASE WHEN ranked.rn % 2 = 0 THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 8) * interval '25 minutes') + interval '22 minutes' ELSE NULL END,
        clicked_at = CASE WHEN ranked.rn % 5 = 0 THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 8) * interval '25 minutes') + interval '31 minutes' ELSE NULL END,
        created_at = v_demo_day + interval '9 hours' + (((ranked.rn - 1) % 8) * interval '25 minutes'),
        updated_at = now()
    FROM ranked
    WHERE cc.id = ranked.id;

    WITH ranked AS (
        SELECT id, row_number() OVER (PARTITION BY business_id ORDER BY id) AS rn
        FROM manual_ledger_entries
        WHERE business_id IN (v_core_id, v_ai_id)
    )
    UPDATE manual_ledger_entries le
    SET occurred_at = v_demo_day + interval '7 hours' + (((ranked.rn - 1) % 12) * interval '35 minutes'),
        voided_at = CASE WHEN le.voided_by_user_id IS NOT NULL THEN v_demo_day + interval '8 hours' + (((ranked.rn - 1) % 12) * interval '35 minutes') ELSE NULL END,
        created_at = now(),
        updated_at = now()
    FROM ranked
    WHERE le.id = ranked.id;

    UPDATE payroll_runs
    SET period_start = v_demo_day::date,
        period_end = v_demo_day::date,
        paid_at = CASE WHEN status = 'paid' THEN v_demo_day + interval '10 hours' ELSE NULL END,
        created_at = v_demo_day + interval '8 hours',
        updated_at = now()
    WHERE business_id IN (v_core_id, v_ai_id);

    UPDATE payroll_line_items
    SET created_at = v_demo_day + interval '8 hours 15 minutes',
        updated_at = now()
    WHERE business_id IN (v_core_id, v_ai_id);

    WITH ranked AS (
        SELECT id, status, row_number() OVER (PARTITION BY business_id ORDER BY id) AS rn
        FROM delivery_orders
        WHERE business_id IN (v_core_id, v_ai_id)
    )
    UPDATE delivery_orders d
    SET created_at = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes'),
        updated_at = now(),
        assigned_at = CASE WHEN ranked.status IN ('assigned', 'picked_up', 'in_transit', 'nearby', 'delivered') THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '10 minutes' ELSE NULL END,
        pickup_timestamp = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes'),
        dropoff_timestamp = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '35 minutes',
        "current_timestamp" = CASE WHEN ranked.status IN ('picked_up', 'in_transit', 'nearby') THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '30 minutes' ELSE NULL END,
        estimated_pickup_time = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '20 minutes',
        actual_pickup_time = CASE WHEN ranked.status IN ('picked_up', 'in_transit', 'nearby', 'delivered') THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '25 minutes' ELSE NULL END,
        estimated_delivery_time = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '55 minutes',
        actual_delivery_time = CASE WHEN ranked.status = 'delivered' THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '50 minutes' ELSE NULL END,
        cutoff_at = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '15 minutes',
        cancelled_at = CASE WHEN ranked.status = 'cancelled' THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 9) * interval '45 minutes') + interval '20 minutes' ELSE NULL END
    FROM ranked
    WHERE d.id = ranked.id;

    WITH ranked AS (
        SELECT h.id, d.created_at, row_number() OVER (PARTITION BY h.delivery_order_id ORDER BY h.id) AS rn
        FROM delivery_status_history h
        JOIN delivery_orders d ON d.id = h.delivery_order_id
        WHERE d.business_id IN (v_core_id, v_ai_id)
    )
    UPDATE delivery_status_history h
    SET created_at = ranked.created_at + ((ranked.rn - 1) * interval '10 minutes')
    FROM ranked
    WHERE h.id = ranked.id;

    UPDATE report_schedules
    SET last_sent_at = v_demo_day + interval '8 hours',
        next_send_at = v_demo_day + interval '1 day 8 hours',
        created_at = now(),
        updated_at = now()
    WHERE business_id IN (v_core_id, v_ai_id);

    WITH ranked AS (
        SELECT id, status, row_number() OVER (PARTITION BY business_id ORDER BY id) AS rn
        FROM withdrawal_histories
        WHERE business_id IN (v_core_id, v_ai_id)
    )
    UPDATE withdrawal_histories wh
    SET created_at = v_demo_day + interval '11 hours' + (((ranked.rn - 1) % 6) * interval '30 minutes'),
        updated_at = now(),
        confirmed_at = CASE WHEN ranked.status = 'confirmed' THEN v_demo_day + interval '12 hours' + (((ranked.rn - 1) % 6) * interval '30 minutes') ELSE NULL END
    FROM ranked
    WHERE wh.id = ranked.id;

    WITH ranked AS (
        SELECT id, status, row_number() OVER (ORDER BY id) AS rn
        FROM webhook_events
        WHERE webhook_id LIKE 'demo_seed_%'
    )
    UPDATE webhook_events we
    SET received_at = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 6) * interval '20 minutes'),
        processed_at = CASE WHEN ranked.status = 'processed' THEN v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 6) * interval '20 minutes') + interval '2 minutes' ELSE NULL END,
        created_at = now(),
        updated_at = now()
    FROM ranked
    WHERE we.id = ranked.id;

    WITH ranked AS (
        SELECT id, row_number() OVER (ORDER BY id) AS rn
        FROM page_views
        WHERE session_id LIKE 'pvseed-demo-session-%'
    )
    UPDATE page_views pv
    SET timestamp = v_demo_day + interval '8 hours' + (((ranked.rn - 1) % 12) * interval '20 minutes')
    FROM ranked
    WHERE pv.id = ranked.id;

    WITH ranked AS (
        SELECT id, row_number() OVER (ORDER BY id) AS rn
        FROM user_interactions
        WHERE session_id LIKE 'pvseed-demo-session-%'
    )
    UPDATE user_interactions ui
    SET timestamp = v_demo_day + interval '9 hours' + (((ranked.rn - 1) % 12) * interval '20 minutes')
    FROM ranked
    WHERE ui.id = ranked.id;

    WITH ranked AS (
        SELECT id, row_number() OVER (ORDER BY id) AS rn
        FROM conversion_events
        WHERE session_id LIKE 'pvseed-demo-session-%'
    )
    UPDATE conversion_events ce
    SET timestamp = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 8) * interval '25 minutes')
    FROM ranked
    WHERE ce.id = ranked.id;

    WITH ranked AS (
        SELECT session_id, row_number() OVER (ORDER BY session_id) AS rn
        FROM session_summaries
        WHERE session_id LIKE 'pvseed-demo-summary-%'
    )
    UPDATE session_summaries ss
    SET first_seen = v_demo_day + interval '8 hours' + (((ranked.rn - 1) % 8) * interval '25 minutes'),
        last_seen = v_demo_day + interval '8 hours' + (((ranked.rn - 1) % 8) * interval '25 minutes') + interval '12 minutes'
    FROM ranked
    WHERE ss.session_id = ranked.session_id;

    WITH ranked AS (
        SELECT id, row_number() OVER (ORDER BY id) AS rn
        FROM error_logs
        WHERE request_id LIKE 'pvseed-demo-error-%'
    )
    UPDATE error_logs el
    SET timestamp = v_demo_day + interval '11 hours' + (((ranked.rn - 1) % 6) * interval '30 minutes'),
        created_at = v_demo_day + interval '11 hours' + (((ranked.rn - 1) % 6) * interval '30 minutes')
    FROM ranked
    WHERE el.id = ranked.id;

    WITH ranked AS (
        SELECT id, row_number() OVER (ORDER BY id) AS rn
        FROM ai_waiter_conversations
        WHERE business_id = v_ai_id
    )
    UPDATE ai_waiter_conversations c
    SET created_at = v_demo_day + interval '12 hours' + (((ranked.rn - 1) % 8) * interval '20 minutes'),
        updated_at = now()
    FROM ranked
    WHERE c.id = ranked.id;

    WITH ranked AS (
        SELECT m.id, c.created_at, row_number() OVER (PARTITION BY m.conversation_id ORDER BY m.id) AS rn
        FROM ai_waiter_messages m
        JOIN ai_waiter_conversations c ON c.id = m.conversation_id
        WHERE c.business_id = v_ai_id
    )
    UPDATE ai_waiter_messages m
    SET created_at = ranked.created_at + ((ranked.rn - 1) * interval '5 minutes')
    FROM ranked
    WHERE m.id = ranked.id;

    WITH ranked AS (
        SELECT id, row_number() OVER (ORDER BY id) AS rn
        FROM menu_extraction_jobs
        WHERE business_id = v_ai_id
    )
    UPDATE menu_extraction_jobs j
    SET created_at = v_demo_day + interval '9 hours' + (((ranked.rn - 1) % 4) * interval '45 minutes'),
        updated_at = v_demo_day + interval '9 hours' + (((ranked.rn - 1) % 4) * interval '45 minutes') + interval '20 minutes'
    FROM ranked
    WHERE j.id = ranked.id;

    UPDATE menu_extraction_images i
    SET created_at = j.created_at + interval '2 minutes'
    FROM menu_extraction_jobs j
    WHERE i.job_id = j.id
      AND j.business_id = v_ai_id;

    WITH ranked AS (
        SELECT id, row_number() OVER (ORDER BY id) AS rn
        FROM menu_wizard_sessions
        WHERE business_id = v_ai_id
    )
    UPDATE menu_wizard_sessions s
    SET created_at = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 4) * interval '50 minutes'),
        updated_at = v_demo_day + interval '10 hours' + (((ranked.rn - 1) % 4) * interval '50 minutes') + interval '15 minutes'
    FROM ranked
    WHERE s.id = ranked.id;

    WITH ranked AS (
        SELECT m.id, s.created_at, row_number() OVER (PARTITION BY m.session_id ORDER BY m.id) AS rn
        FROM menu_wizard_messages m
        JOIN menu_wizard_sessions s ON s.id = m.session_id
        WHERE s.business_id = v_ai_id
    )
    UPDATE menu_wizard_messages m
    SET created_at = ranked.created_at + ((ranked.rn - 1) * interval '5 minutes')
    FROM ranked
    WHERE m.id = ranked.id;

    WITH ranked AS (
        SELECT id, row_number() OVER (ORDER BY id) AS rn
        FROM director_console_threads
        WHERE business_id = v_ai_id
    )
    UPDATE director_console_threads t
    SET created_at = v_demo_day + interval '13 hours' + (((ranked.rn - 1) % 5) * interval '20 minutes'),
        updated_at = now(),
        last_message_at = v_demo_day + interval '13 hours' + (((ranked.rn - 1) % 5) * interval '20 minutes') + interval '10 minutes'
    FROM ranked
    WHERE t.id = ranked.id;

    WITH ranked AS (
        SELECT m.id, t.created_at, row_number() OVER (PARTITION BY m.thread_id ORDER BY m.id) AS rn
        FROM director_console_messages m
        JOIN director_console_threads t ON t.id = m.thread_id
        WHERE t.business_id = v_ai_id
    )
    UPDATE director_console_messages m
    SET created_at = ranked.created_at + ((ranked.rn - 1) * interval '5 minutes'),
        updated_at = now(),
        feedback_at = CASE WHEN m.feedback_vote IS NOT NULL THEN ranked.created_at + interval '15 minutes' ELSE NULL END
    FROM ranked
    WHERE m.id = ranked.id;

    IF to_regclass('public.user_sessions') IS NOT NULL THEN
        UPDATE user_sessions
        SET expires_at = v_demo_day + interval '23 hours',
            created_at = v_demo_day + interval '8 hours',
            last_used_at = v_demo_day + interval '9 hours',
            refresh_expires_at = v_demo_day + interval '23 hours'
        WHERE session_token IN ('pvseed-demo-revoked-core-session', 'pvseed-demo-revoked-ai-session');
    END IF;

    IF to_regclass('public.admin_actions') IS NOT NULL THEN
        UPDATE admin_actions
        SET created_at = v_demo_day + interval '10 hours'
        WHERE target_user_id = v_owner_id
          AND details::text LIKE '%payverge-demo-seed-fixture%';
    END IF;

    RAISE NOTICE 'Payverge demo seed complete. Core business id %, AI Pro business id %', v_core_id, v_ai_id;
END $$ LANGUAGE plpgsql;

COMMIT;

SELECT 'owner' AS check_name, id, email, address
FROM users
WHERE email = 'demo-owner@example.com';

SELECT 'businesses' AS check_name, business_id, custom_url, is_active, is_demo
FROM businesses
WHERE business_id IN ('demo-core-business', 'demo-ai-pro-business')
ORDER BY business_id;

SELECT 'menu_items' AS check_name, b.business_id, jsonb_array_length(m.categories::jsonb) AS categories
FROM businesses b
JOIN menus m ON m.business_id = b.id
WHERE b.business_id IN ('demo-core-business', 'demo-ai-pro-business')
ORDER BY b.business_id;

SELECT 'bills' AS check_name, b.business_id, count(*) AS bill_count, count(*) FILTER (WHERE bills.status = 'open') AS open_bills
FROM businesses b
JOIN bills ON bills.business_id = b.id
WHERE b.business_id IN ('demo-core-business', 'demo-ai-pro-business')
GROUP BY b.business_id
ORDER BY b.business_id;

SELECT 'ai_waiter' AS check_name, b.business_id, count(c.id) AS conversations
FROM businesses b
LEFT JOIN ai_waiter_conversations c ON c.business_id = b.id
WHERE b.business_id IN ('demo-core-business', 'demo-ai-pro-business')
GROUP BY b.business_id
ORDER BY b.business_id;
