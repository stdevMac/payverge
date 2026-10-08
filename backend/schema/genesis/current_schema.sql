--
-- PostgreSQL database dump
--



SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pg_trgm; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;


--
-- Name: EXTENSION pg_trgm; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pg_trgm IS 'text similarity measurement and index searching based on trigrams';


--
-- Name: capture_business_activation_insert(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_business_activation_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    PERFORM record_business_activation_first(NEW.id, 'registration_completed', NEW.created_at, 'business:' || NEW.id);
    PERFORM record_business_activation_first(NEW.id, 'workspace_created', NEW.created_at, 'business:' || NEW.id);
    RETURN NEW;
END;
$$;


--
-- Name: capture_business_activation_update(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_business_activation_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF OLD.qr_previewed_at IS NULL AND NEW.qr_previewed_at IS NOT NULL THEN
        PERFORM record_business_activation_first(NEW.id, 'qr_previewed', NEW.qr_previewed_at, 'business:' || NEW.id);
        PERFORM record_activation_achieved_if_ready(NEW.id, 'business:' || NEW.id);
    END IF;
    IF OLD.onboarding_completed_at IS NULL AND NEW.onboarding_completed_at IS NOT NULL THEN
        PERFORM record_business_activation_first(NEW.id, 'setup_completed', NEW.onboarding_completed_at, 'business:' || NEW.id);
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: capture_menu_activation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_menu_activation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NULLIF(BTRIM(COALESCE(NEW.categories, '')), '') IS NOT NULL AND BTRIM(NEW.categories) NOT IN ('[]', 'null') THEN
        PERFORM record_business_activation_first(NEW.business_id, 'menu_item_created', COALESCE(NEW.updated_at, NEW.created_at, NOW()), 'menu:' || NEW.id);
        PERFORM record_activation_achieved_if_ready(NEW.business_id, 'menu:' || NEW.id);
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: capture_order_activation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_order_activation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.status = 'completed'
       AND (TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM 'completed') THEN
        PERFORM record_business_activation_first(NEW.business_id, 'test_order_completed', COALESCE(NEW.approved_at, NEW.created_at, NOW()), 'order:' || NEW.id);
        PERFORM record_activation_achieved_if_ready(NEW.business_id, 'order:' || NEW.id);
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: capture_paid_bill_activation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_paid_bill_activation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.status IN ('paid', 'closed')
       AND (TG_OP = 'INSERT' OR OLD.status NOT IN ('paid', 'closed')) THEN
        PERFORM record_business_activation_first(NEW.business_id, 'first_paid_bill', COALESCE(NEW.settled_at, NEW.closed_at, NEW.updated_at, NEW.created_at, NOW()), 'bill:' || NEW.id);
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: capture_payment_config_activation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_payment_config_activation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.is_enabled = TRUE
       AND (TG_OP = 'INSERT' OR OLD.is_enabled IS DISTINCT FROM TRUE)
       AND EXISTS (SELECT 1 FROM plugins WHERE plugins.id = NEW.plugin_id AND plugins.category = 'payment') THEN
        PERFORM record_business_activation_first(NEW.business_id, 'payment_configured', COALESCE(NEW.created_at, NOW()), 'business_plugin:' || NEW.id);
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: capture_staff_invitation_activation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_staff_invitation_activation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    PERFORM record_business_activation_first(NEW.business_id, 'staff_invited', COALESCE(NEW.created_at, NOW()), 'staff_invitation:' || NEW.id);
    RETURN NEW;
END;
$$;


--
-- Name: capture_table_activation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_table_activation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    PERFORM record_business_activation_first(NEW.business_id, 'table_created', COALESCE(NEW.created_at, NOW()), 'table:' || NEW.id);
    PERFORM record_activation_achieved_if_ready(NEW.business_id, 'table:' || NEW.id);
    RETURN NEW;
END;
$$;


--
-- Name: capture_user_session_refresh_history(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.capture_user_session_refresh_history() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.previous_refresh_token IS NOT NULL
       AND NEW.previous_refresh_token <> ''
       AND NEW.previous_rotated_at IS NOT NULL
       AND (
           OLD.previous_refresh_token IS DISTINCT FROM NEW.previous_refresh_token OR
           OLD.previous_rotated_at IS DISTINCT FROM NEW.previous_rotated_at
       ) THEN
        INSERT INTO user_session_refresh_history (session_id, token_hash, rotated_at, expires_at)
        VALUES (
            NEW.id,
            NEW.previous_refresh_token,
            NEW.previous_rotated_at,
            COALESCE(NEW.refresh_expires_at, NEW.previous_rotated_at + INTERVAL '24 hours')
        )
        ON CONFLICT (token_hash) DO UPDATE
        SET expires_at = EXCLUDED.expires_at
        WHERE user_session_refresh_history.session_id = EXCLUDED.session_id;

        IF NOT FOUND THEN
            RAISE EXCEPTION 'refresh-history token collision for session %', NEW.id
                USING ERRCODE = 'unique_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: default_user_session_refresh_history_expiry(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.default_user_session_refresh_history_expiry() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.expires_at IS NULL THEN
        SELECT COALESCE(sessions.refresh_expires_at, NEW.rotated_at + INTERVAL '24 hours')
        INTO NEW.expires_at
        FROM user_sessions AS sessions
        WHERE sessions.id = NEW.session_id;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: record_activation_achieved_if_ready(bigint, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.record_activation_achieved_if_ready(p_business_id bigint, p_source_key text) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
    achieved_at TIMESTAMPTZ;
BEGIN
    SELECT GREATEST(
        menu_item_created_at, table_created_at, qr_previewed_at,
        test_order_completed_at
    )
    INTO achieved_at
    FROM business_activation_states
    WHERE business_id = p_business_id
      AND menu_item_created_at IS NOT NULL
      AND table_created_at IS NOT NULL
      AND qr_previewed_at IS NOT NULL
      AND test_order_completed_at IS NOT NULL;

    IF achieved_at IS NOT NULL THEN
        PERFORM record_business_activation_first(
            p_business_id, 'activation_achieved', achieved_at, p_source_key
        );
    END IF;
END;
$$;


--
-- Name: record_business_activation_first(bigint, text, timestamp with time zone, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.record_business_activation_first(p_business_id bigint, p_event_name text, p_occurred_at timestamp with time zone, p_source_key text) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
    changed_count BIGINT := 0;
    business_locale TEXT;
    business_created_at TIMESTAMPTZ;
    acquisition_source TEXT;
BEGIN
    INSERT INTO business_activation_states (business_id)
    VALUES (p_business_id)
    ON CONFLICT (business_id) DO NOTHING;

    IF p_event_name = 'registration_completed' THEN
        UPDATE business_activation_states SET registration_completed_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND registration_completed_at IS NULL;
    ELSIF p_event_name = 'workspace_created' THEN
        UPDATE business_activation_states SET workspace_created_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND workspace_created_at IS NULL;
    ELSIF p_event_name = 'menu_item_created' THEN
        UPDATE business_activation_states SET menu_item_created_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND menu_item_created_at IS NULL;
    ELSIF p_event_name = 'table_created' THEN
        UPDATE business_activation_states SET table_created_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND table_created_at IS NULL;
    ELSIF p_event_name = 'qr_previewed' THEN
        UPDATE business_activation_states SET qr_previewed_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND qr_previewed_at IS NULL;
    ELSIF p_event_name = 'payment_configured' THEN
        UPDATE business_activation_states SET payment_configured_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND payment_configured_at IS NULL;
    ELSIF p_event_name = 'staff_invited' THEN
        UPDATE business_activation_states SET staff_invited_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND staff_invited_at IS NULL;
    ELSIF p_event_name = 'setup_completed' THEN
        UPDATE business_activation_states SET setup_completed_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND setup_completed_at IS NULL;
    ELSIF p_event_name = 'test_order_completed' THEN
        UPDATE business_activation_states SET test_order_completed_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND test_order_completed_at IS NULL;
    ELSIF p_event_name = 'activation_achieved' THEN
        UPDATE business_activation_states SET activation_achieved_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND activation_achieved_at IS NULL;
    ELSIF p_event_name = 'first_paid_bill' THEN
        UPDATE business_activation_states SET first_paid_bill_at = p_occurred_at, updated_at = p_occurred_at
        WHERE business_id = p_business_id AND first_paid_bill_at IS NULL;
    ELSE
        RAISE EXCEPTION 'unsupported server activation event: %', p_event_name;
    END IF;
    GET DIAGNOSTICS changed_count = ROW_COUNT;
    IF changed_count = 0 THEN
        RETURN;
    END IF;

    SELECT
        COALESCE(NULLIF(BTRIM(default_language), ''), 'unknown'),
        created_at,
        'direct'
    INTO business_locale, business_created_at, acquisition_source
    FROM businesses WHERE id = p_business_id;

    INSERT INTO activation_event_outbox (
        event_name, schema_version, origin, business_id, idempotency_key,
        dimensions, occurred_at
    ) VALUES (
        p_event_name, 1, 'server', p_business_id,
        'server:' || p_business_id || ':' || p_event_name,
        jsonb_build_object(
            'business_id', p_business_id::TEXT,
            'locale', business_locale,
            'device_class', 'server',
            'acquisition_source', acquisition_source,
            'acquisition_campaign', 'unknown',
            'elapsed_ms', GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (p_occurred_at - business_created_at)) * 1000)::BIGINT)
        ),
        p_occurred_at
    );
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: accounting_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.accounting_categories (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    key character varying(64) NOT NULL,
    label character varying(128) NOT NULL,
    entry_type character varying(16) NOT NULL,
    active boolean DEFAULT true NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: accounting_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.accounting_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: accounting_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.accounting_categories_id_seq OWNED BY public.accounting_categories.id;


--
-- Name: accounting_period_locks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.accounting_period_locks (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    locked_through date,
    locked_by_user_id bigint,
    locked_by_staff_id bigint,
    note text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: accounting_period_locks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.accounting_period_locks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: accounting_period_locks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.accounting_period_locks_id_seq OWNED BY public.accounting_period_locks.id;


--
-- Name: activation_event_outbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activation_event_outbox (
    id bigint NOT NULL,
    event_name character varying(64) NOT NULL,
    schema_version integer DEFAULT 1 NOT NULL,
    origin character varying(16) NOT NULL,
    business_id bigint,
    funnel_id uuid,
    idempotency_key character varying(255) NOT NULL,
    dimensions jsonb DEFAULT '{}'::jsonb NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    delivery_status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    delivery_attempts integer DEFAULT 0 NOT NULL,
    delivered_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT activation_event_delivery_check CHECK (((delivery_status)::text = ANY (ARRAY[('pending'::character varying)::text, ('processing'::character varying)::text, ('delivered'::character varying)::text, ('failed'::character varying)::text]))),
    CONSTRAINT activation_event_dimensions_shape_check CHECK (((jsonb_typeof(dimensions) = 'object'::text) AND (dimensions ? 'locale'::text) AND (dimensions ? 'device_class'::text) AND (dimensions ? 'elapsed_ms'::text) AND ((dimensions - ARRAY['business_id'::text, 'locale'::text, 'device_class'::text, 'acquisition_source'::text, 'acquisition_campaign'::text, 'onboarding_step'::text, 'elapsed_ms'::text]) = '{}'::jsonb))),
    CONSTRAINT activation_event_name_check CHECK (((event_name)::text = ANY (ARRAY[('registration_started'::character varying)::text, ('registration_completed'::character varying)::text, ('workspace_created'::character varying)::text, ('onboarding_step_viewed'::character varying)::text, ('onboarding_step_clicked'::character varying)::text, ('menu_item_created'::character varying)::text, ('table_created'::character varying)::text, ('qr_previewed'::character varying)::text, ('payment_configured'::character varying)::text, ('staff_invited'::character varying)::text, ('setup_completed'::character varying)::text, ('test_order_completed'::character varying)::text, ('activation_achieved'::character varying)::text, ('first_paid_bill'::character varying)::text]))),
    CONSTRAINT activation_event_origin_check CHECK (((((origin)::text = 'server'::text) AND (business_id IS NOT NULL) AND (funnel_id IS NULL)) OR (((origin)::text = 'client'::text) AND (funnel_id IS NOT NULL)))),
    CONSTRAINT activation_event_schema_check CHECK ((schema_version = 1))
);


--
-- Name: activation_event_outbox_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.activation_event_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: activation_event_outbox_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.activation_event_outbox_id_seq OWNED BY public.activation_event_outbox.id;


--
-- Name: admin_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_actions (
    id bigint NOT NULL,
    admin_user_id bigint NOT NULL,
    target_user_id bigint NOT NULL,
    action_type text NOT NULL,
    details jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone
);


--
-- Name: admin_actions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_actions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_actions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_actions_id_seq OWNED BY public.admin_actions.id;


--
-- Name: ai_budget_controls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_budget_controls (
    id smallint NOT NULL,
    unpriced_model_shutdown boolean DEFAULT false NOT NULL,
    shutdown_reason text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_budget_controls_singleton CHECK ((id = 1))
);


--
-- Name: ai_daily_spend; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_daily_spend (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    feature_scope text DEFAULT ''::text NOT NULL,
    usage_date date NOT NULL,
    finalized_micro_usd bigint DEFAULT 0 NOT NULL,
    reserved_micro_usd bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_daily_spend_finalized_nonneg CHECK ((finalized_micro_usd >= 0)),
    CONSTRAINT ai_daily_spend_reserved_nonneg CHECK ((reserved_micro_usd >= 0))
);


--
-- Name: ai_daily_spend_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_daily_spend_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_daily_spend_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_daily_spend_id_seq OWNED BY public.ai_daily_spend.id;


--
-- Name: ai_generated_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_generated_images (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    s3_key text NOT NULL,
    source character varying(32) NOT NULL,
    model character varying(128),
    created_at timestamp with time zone
);


--
-- Name: ai_generated_images_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_generated_images_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_generated_images_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_generated_images_id_seq OWNED BY public.ai_generated_images.id;


--
-- Name: ai_image_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_image_usage (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    daily_used bigint DEFAULT 0 NOT NULL,
    daily_period_start timestamp with time zone DEFAULT now() NOT NULL,
    monthly_used bigint DEFAULT 0 NOT NULL,
    monthly_anchor_day bigint DEFAULT 1 NOT NULL,
    monthly_period_start timestamp with time zone DEFAULT now() NOT NULL,
    monthly_alert_sent_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: ai_image_usage_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_image_usage_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_image_usage_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_image_usage_id_seq OWNED BY public.ai_image_usage.id;


--
-- Name: ai_spend_reservations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_spend_reservations (
    id uuid NOT NULL,
    business_id bigint NOT NULL,
    feature_scope text DEFAULT ''::text NOT NULL,
    usage_date date NOT NULL,
    conservative_micro_usd bigint NOT NULL,
    actual_micro_usd bigint,
    status text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    input_tokens integer DEFAULT 0 NOT NULL,
    output_tokens integer DEFAULT 0 NOT NULL,
    max_output_tokens integer DEFAULT 0 NOT NULL,
    served_model text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    parent_reservation_id uuid,
    CONSTRAINT ai_spend_reservations_conservative_nonneg CHECK ((conservative_micro_usd >= 0)),
    CONSTRAINT ai_spend_reservations_status_check CHECK ((status = ANY (ARRAY['reserved'::text, 'consumed'::text, 'finalized'::text, 'released'::text, 'expired'::text])))
);


--
-- Name: ai_waiter_conversations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_waiter_conversations (
    id bigint NOT NULL,
    session_id text NOT NULL,
    business_id bigint NOT NULL,
    table_code text,
    language text,
    mode text DEFAULT 'ordering'::text,
    status text DEFAULT 'active'::text,
    is_paused boolean DEFAULT false,
    cart_items_added bigint DEFAULT 0,
    claimed_by_staff_id bigint,
    claimed_by_name text,
    claimed_by_role text,
    claimed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: ai_waiter_conversations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_waiter_conversations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_waiter_conversations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_waiter_conversations_id_seq OWNED BY public.ai_waiter_conversations.id;


--
-- Name: ai_waiter_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_waiter_messages (
    id bigint NOT NULL,
    conversation_id bigint NOT NULL,
    role text NOT NULL,
    content text,
    tool_calls text,
    author_staff_id bigint,
    author_name text,
    author_role text,
    created_at timestamp with time zone,
    structured_response jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: ai_waiter_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_waiter_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_waiter_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_waiter_messages_id_seq OWNED BY public.ai_waiter_messages.id;


--
-- Name: alternative_payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.alternative_payments (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    participant_addr text NOT NULL,
    participant_name text,
    amount bigint NOT NULL,
    bill_amount_cents bigint DEFAULT 0 NOT NULL,
    tip_amount_cents bigint DEFAULT 0 NOT NULL,
    payment_method text NOT NULL,
    status text DEFAULT 'pending'::text,
    idempotency_key text,
    confirmed_by text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    confirmed_at timestamp with time zone,
    idempotency_key_hash character varying(64),
    payload_hash character varying(64),
    expires_at timestamp with time zone,
    resolved_by character varying(255) DEFAULT ''::character varying NOT NULL,
    resolution_reason character varying(500) DEFAULT ''::character varying NOT NULL,
    resolved_at timestamp with time zone,
    payer_guest_session character varying(64),
    CONSTRAINT alternative_payments_request_hashes_check CHECK (((((COALESCE(idempotency_key_hash, ''::character varying))::text = ''::text) AND ((COALESCE(payload_hash, ''::character varying))::text = ''::text)) OR ((length((idempotency_key_hash)::text) = 64) AND (length((payload_hash)::text) = 64))))
);


--
-- Name: alternative_payments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.alternative_payments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: alternative_payments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.alternative_payments_id_seq OWNED BY public.alternative_payments.id;


--
-- Name: announcement_acks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.announcement_acks (
    announcement_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    business_id bigint NOT NULL,
    acknowledged_at timestamp with time zone
);


--
-- Name: announcements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.announcements (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    author_staff_id bigint NOT NULL,
    title text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    require_ack boolean DEFAULT false NOT NULL,
    audience_filter text DEFAULT 'all'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: announcements_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.announcements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: announcements_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.announcements_id_seq OWNED BY public.announcements.id;


--
-- Name: auth_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auth_attempts (
    id bigint NOT NULL,
    principal text NOT NULL,
    kind text NOT NULL,
    count bigint DEFAULT 0 NOT NULL,
    window_started_at timestamp with time zone NOT NULL,
    locked_until timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: auth_attempts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.auth_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: auth_attempts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.auth_attempts_id_seq OWNED BY public.auth_attempts.id;


--
-- Name: bill_history_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bill_history_events (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    business_id bigint NOT NULL,
    event_type text NOT NULL,
    actor text,
    reason text,
    order_id bigint,
    order_number text,
    bill_item_id text,
    item_name text,
    details text,
    created_at timestamp with time zone
);


--
-- Name: bill_history_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bill_history_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bill_history_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bill_history_events_id_seq OWNED BY public.bill_history_events.id;


--
-- Name: bill_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bill_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    bill_id integer NOT NULL,
    menu_item_id character varying(255) DEFAULT ''::character varying NOT NULL,
    name character varying(255) NOT NULL,
    price numeric(10,2) NOT NULL,
    quantity integer NOT NULL,
    options jsonb,
    subtotal numeric(10,2) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    item_type character varying(32) DEFAULT 'menu_item'::character varying,
    bundle_id bigint,
    parent_bundle_id bigint,
    source_offer_id bigint,
    order_id bigint
);


--
-- Name: bill_split_shares; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bill_split_shares (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    guest_session_id character varying(128) NOT NULL,
    display_name character varying(120),
    mode character varying(16) NOT NULL,
    claimed_item_ids text,
    claimed_fractions text,
    amount_cents bigint NOT NULL,
    tip_cents bigint DEFAULT 0,
    status character varying(16) DEFAULT 'held'::character varying NOT NULL,
    hold_expires_at timestamp with time zone,
    idempotency_key character varying(160),
    settlement_idempotency_key character varying(160),
    tender character varying(32),
    payment_id bigint,
    alternative_payment_id bigint,
    settled_at timestamp with time zone,
    released_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    CONSTRAINT bill_split_shares_amount_cents_positive CHECK ((amount_cents > 0))
);


--
-- Name: bill_split_shares_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bill_split_shares_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bill_split_shares_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bill_split_shares_id_seq OWNED BY public.bill_split_shares.id;


--
-- Name: bills; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bills (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    table_id bigint,
    counter_id bigint,
    bill_number text NOT NULL,
    public_token text DEFAULT (replace((gen_random_uuid())::text, '-'::text, ''::text) || replace((gen_random_uuid())::text, '-'::text, ''::text)) NOT NULL,
    notes text,
    items text,
    subtotal bigint DEFAULT 0,
    tax_amount bigint DEFAULT 0,
    service_fee_amount bigint DEFAULT 0,
    total_amount bigint DEFAULT 0,
    paid_amount bigint DEFAULT 0,
    tip_amount bigint DEFAULT 0,
    loyalty_discount_cents bigint DEFAULT 0,
    loyalty_points_redeemed bigint DEFAULT 0,
    loyalty_redeemed_by_customer_id bigint,
    status text DEFAULT 'open'::text,
    settlement_addr text NOT NULL,
    tipping_addr text NOT NULL,
    created_by_staff_id bigint,
    closed_by_staff_id bigint,
    crm_customer_id bigint,
    fiscal_customer_doc_type text,
    fiscal_customer_doc_number text,
    fiscal_customer_tax_condition text,
    fiscal_customer_name text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    closed_at timestamp with time zone,
    feedback_email_sent_at timestamp with time zone,
    abandoned_at timestamp with time zone,
    settled_at timestamp with time zone,
    fiscal_customer_email text,
    fiscal_customer_guest_session character varying(64),
    CONSTRAINT bills_public_token_not_blank CHECK ((btrim(public_token) <> ''::text))
);


--
-- Name: bills_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bills_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bills_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bills_id_seq OWNED BY public.bills.id;


--
-- Name: bundles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bundles (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    name text NOT NULL,
    description text,
    price numeric NOT NULL,
    currency text,
    image text,
    items text,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: bundles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bundles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bundles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bundles_id_seq OWNED BY public.bundles.id;


--
-- Name: business_activation_states; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_activation_states (
    business_id bigint NOT NULL,
    registration_completed_at timestamp with time zone,
    workspace_created_at timestamp with time zone,
    menu_item_created_at timestamp with time zone,
    table_created_at timestamp with time zone,
    qr_previewed_at timestamp with time zone,
    payment_configured_at timestamp with time zone,
    staff_invited_at timestamp with time zone,
    setup_completed_at timestamp with time zone,
    test_order_completed_at timestamp with time zone,
    activation_achieved_at timestamp with time zone,
    first_paid_bill_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: business_alert_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_alert_settings (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    browser_notifications_enabled boolean DEFAULT true NOT NULL,
    sound_enabled boolean DEFAULT false NOT NULL,
    volume numeric DEFAULT 0.8 NOT NULL,
    repeat_interval_seconds bigint DEFAULT 30 NOT NULL,
    event_settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    CONSTRAINT business_alert_settings_repeat_interval_seconds_check CHECK (((repeat_interval_seconds >= 30) AND (repeat_interval_seconds <= 60)))
);


--
-- Name: business_alert_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_alert_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_alert_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_alert_settings_id_seq OWNED BY public.business_alert_settings.id;


--
-- Name: business_creation_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_creation_requests (
    id bigint NOT NULL,
    owner_scope_hash character varying(64) NOT NULL,
    idempotency_key_hash character varying(64) NOT NULL,
    payload_hash character varying(64) NOT NULL,
    business_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT business_creation_request_hash_lengths CHECK (((length((owner_scope_hash)::text) = 64) AND (length((idempotency_key_hash)::text) = 64) AND (length((payload_hash)::text) = 64)))
);


--
-- Name: business_creation_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_creation_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_creation_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_creation_requests_id_seq OWNED BY public.business_creation_requests.id;


--
-- Name: business_currencies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_currencies (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    currency_code character varying(3) NOT NULL,
    is_preferred boolean DEFAULT false,
    display_order bigint DEFAULT 0,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: business_currencies_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_currencies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_currencies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_currencies_id_seq OWNED BY public.business_currencies.id;


--
-- Name: business_fiscal_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_fiscal_settings (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    country character varying(2) NOT NULL,
    provider character varying(64) NOT NULL,
    mode character varying(32) DEFAULT 'off'::character varying NOT NULL,
    environment character varying(16) DEFAULT 'sandbox'::character varying NOT NULL,
    tax_id character varying(64),
    tax_condition character varying(64),
    point_of_sale bigint,
    credentials_encrypted bytea,
    credentials_fingerprint character varying(128),
    credentials_expires_at timestamp with time zone,
    provider_config jsonb,
    setup_status character varying(32) DEFAULT 'draft'::character varying NOT NULL,
    last_validated_at timestamp with time zone,
    last_validation_error text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: business_fiscal_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_fiscal_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_fiscal_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_fiscal_settings_id_seq OWNED BY public.business_fiscal_settings.id;


--
-- Name: business_gallery_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_gallery_images (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    image_url text NOT NULL,
    caption text,
    display_order bigint DEFAULT 0,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: business_gallery_images_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_gallery_images_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_gallery_images_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_gallery_images_id_seq OWNED BY public.business_gallery_images.id;


--
-- Name: business_languages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_languages (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    language_code character varying(16) NOT NULL,
    is_default boolean DEFAULT false,
    display_order bigint DEFAULT 0,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: business_languages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_languages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_languages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_languages_id_seq OWNED BY public.business_languages.id;


--
-- Name: business_milestone_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_milestone_events (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    milestone_type character varying(64) NOT NULL,
    threshold_cents bigint DEFAULT 0 NOT NULL,
    bill_id bigint,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    processing_token character varying(64),
    payload text,
    last_error text,
    sent_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: business_milestone_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_milestone_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_milestone_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_milestone_events_id_seq OWNED BY public.business_milestone_events.id;


--
-- Name: business_operating_exceptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_operating_exceptions (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    exception_date date NOT NULL,
    open_time text,
    close_time text,
    kitchen_close_time text,
    is_closed boolean DEFAULT true NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: business_operating_exceptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_operating_exceptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_operating_exceptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_operating_exceptions_id_seq OWNED BY public.business_operating_exceptions.id;


--
-- Name: business_operating_hours; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_operating_hours (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    day_of_week bigint NOT NULL,
    open_time text,
    close_time text,
    is_closed boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    kitchen_close_time text
);


--
-- Name: business_operating_hours_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_operating_hours_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_operating_hours_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_operating_hours_id_seq OWNED BY public.business_operating_hours.id;


--
-- Name: business_plugins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_plugins (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    plugin_id bigint NOT NULL,
    is_enabled boolean DEFAULT false,
    config text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    last_status text DEFAULT ''::text NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    last_error_at timestamp with time zone,
    last_success_at timestamp with time zone
);


--
-- Name: business_plugins_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_plugins_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_plugins_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_plugins_id_seq OWNED BY public.business_plugins.id;


--
-- Name: business_revenue_aggregates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_revenue_aggregates (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    net_revenue_cents bigint DEFAULT 0 NOT NULL,
    net_tip_cents bigint DEFAULT 0 NOT NULL,
    gross_revenue_cents bigint DEFAULT 0 NOT NULL,
    gross_tip_cents bigint DEFAULT 0 NOT NULL,
    positive_event_count bigint DEFAULT 0 NOT NULL,
    recognized_bill_count bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: business_revenue_aggregates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_revenue_aggregates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_revenue_aggregates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_revenue_aggregates_id_seq OWNED BY public.business_revenue_aggregates.id;


--
-- Name: business_schedule_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_schedule_settings (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    week_start_day bigint DEFAULT 1 NOT NULL,
    default_shift_minutes bigint DEFAULT 480 NOT NULL,
    reminder_lead_hours bigint DEFAULT 3 NOT NULL,
    overtime_weekly_minutes bigint DEFAULT 2400 NOT NULL,
    posted_lead_days bigint DEFAULT 7 NOT NULL,
    minor_cutoff_min bigint,
    quiet_hours_start_min bigint,
    quiet_hours_end_min bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: business_schedule_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_schedule_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_schedule_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_schedule_settings_id_seq OWNED BY public.business_schedule_settings.id;


--
-- Name: business_special_features; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_special_features (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    title text NOT NULL,
    description text,
    icon text,
    display_order bigint DEFAULT 0,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: business_special_features_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_special_features_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_special_features_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_special_features_id_seq OWNED BY public.business_special_features.id;


--
-- Name: businesses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.businesses (
    id bigint NOT NULL,
    business_id text NOT NULL,
    owner_address text NOT NULL,
    user_id bigint,
    owner_name text,
    name text NOT NULL,
    logo text,
    street text,
    city text,
    state text,
    postal_code text,
    country text,
    settlement_addr text NOT NULL,
    tipping_addr text NOT NULL,
    tax_rate numeric,
    service_fee_rate numeric,
    tax_inclusive boolean,
    service_inclusive boolean,
    is_active boolean DEFAULT true,
    description text,
    custom_url text,
    phone text,
    email text,
    website text,
    social_media text,
    banner_images text,
    business_page_enabled boolean,
    show_reviews boolean,
    google_reviews_enabled boolean,
    google_place_id text,
    google_business_name text,
    google_review_link text,
    google_business_url text,
    timezone text DEFAULT 'UTC'::text,
    business_type character varying(32),
    counter_enabled boolean,
    counter_count bigint,
    counter_prefix text,
    kitchen_enabled boolean DEFAULT true,
    orders_enabled boolean DEFAULT true,
    crm_enabled boolean DEFAULT false,
    default_currency text,
    display_currency text,
    default_language text,
    source_language text,
    design_primary_color text DEFAULT '#1f2937'::text,
    design_secondary_color text DEFAULT '#3b82f6'::text,
    design_font_family text DEFAULT 'Inter'::text,
    design_theme text DEFAULT 'light'::text,
    design_menu_layout text DEFAULT 'grid'::text,
    design_show_images boolean DEFAULT true,
    design_show_descriptions boolean DEFAULT true,
    design_header_style text DEFAULT 'banner'::text,
    design_corner_radius text DEFAULT 'medium'::text,
    design_shadow_intensity text DEFAULT 'subtle'::text,
    design_background_pattern text DEFAULT 'none'::text,
    design_pattern_opacity numeric DEFAULT 0.1,
    design_hero_layout text DEFAULT 'centered'::text,
    design_section_density text DEFAULT 'comfortable'::text,
    ai_ai_enabled boolean DEFAULT true,
    ai_ai_name text DEFAULT 'Sage'::text,
    ai_ai_priority text DEFAULT 'balanced'::text,
    ai_special_instructions text,
    ai_business_page_ai_enabled boolean DEFAULT false,
    default_qr_logo_url text,
    default_qr_foreground_color text DEFAULT '#000000'::text,
    default_qr_background_color text DEFAULT '#FFFFFF'::text,
    default_qr_logo_size bigint DEFAULT 20,
    default_qr_show_business_name boolean DEFAULT false,
    default_qr_show_table_name boolean DEFAULT false,
    default_qr_text_font text DEFAULT 'Verdana'::text,
    latitude numeric(10,8),
    longitude numeric(11,8),
    welcome_message text,
    about_story text,
    show_welcome_message boolean,
    show_about_story boolean,
    show_gallery boolean,
    show_operating_hours boolean,
    show_special_features boolean,
    closed_at timestamp with time zone,
    closed_reason text,
    onboarding_state jsonb DEFAULT '{}'::jsonb NOT NULL,
    onboarding_completed_at timestamp with time zone,
    is_demo boolean DEFAULT false NOT NULL,
    demo_owner_user_id bigint,
    director_digest_last_sent_at timestamp with time zone,
    getting_started_email_sent_at timestamp with time zone,
    setup_nudge_email_sent_at timestamp with time zone,
    first_order_milestone_sent_at timestamp with time zone,
    revenue_milestone_sent_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    marketing_settings jsonb DEFAULT '{"enabled": true, "disabled_plays": []}'::jsonb NOT NULL,
    kind text DEFAULT 'real'::text NOT NULL,
    qr_previewed_at timestamp with time zone,
    service_day_start_minute integer DEFAULT 0 NOT NULL,
    CONSTRAINT businesses_kind_check CHECK ((kind = ANY (ARRAY['real'::text, 'demo'::text, 'test'::text]))),
    CONSTRAINT businesses_service_day_start_minute_range CHECK (((service_day_start_minute >= 0) AND (service_day_start_minute < 1440)))
);


--
-- Name: businesses_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.businesses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: businesses_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.businesses_id_seq OWNED BY public.businesses.id;


--
-- Name: cash_register_movements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cash_register_movements (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    session_id bigint NOT NULL,
    movement_type character varying(32) NOT NULL,
    amount_cents bigint NOT NULL,
    reason character varying(80) DEFAULT ''::character varying NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    alternative_payment_id bigint,
    bill_id bigint,
    actor_user_id bigint,
    actor_staff_id bigint,
    actor_label character varying(255) DEFAULT ''::character varying NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone,
    CONSTRAINT cash_register_movements_amount_check CHECK ((amount_cents > 0)),
    CONSTRAINT cash_register_movements_manual_reason_check CHECK ((((movement_type)::text <> ALL (ARRAY[('cash_in'::character varying)::text, ('cash_out'::character varying)::text])) OR (length(TRIM(BOTH FROM reason)) > 0))),
    CONSTRAINT cash_register_movements_type_check CHECK (((movement_type)::text = ANY (ARRAY[('cash_sale'::character varying)::text, ('cash_refund'::character varying)::text, ('cash_in'::character varying)::text, ('cash_out'::character varying)::text])))
);


--
-- Name: cash_register_movements_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cash_register_movements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cash_register_movements_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cash_register_movements_id_seq OWNED BY public.cash_register_movements.id;


--
-- Name: cash_register_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cash_register_sessions (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    status character varying(16) DEFAULT 'open'::character varying NOT NULL,
    opening_float_cents bigint DEFAULT 0 NOT NULL,
    opening_note text DEFAULT ''::text NOT NULL,
    opened_by_user_id bigint,
    opened_by_staff_id bigint,
    opened_by_label character varying(255) DEFAULT ''::character varying NOT NULL,
    opened_at timestamp with time zone NOT NULL,
    cash_sales_cents bigint DEFAULT 0 NOT NULL,
    cash_refunds_cents bigint DEFAULT 0 NOT NULL,
    cash_in_cents bigint DEFAULT 0 NOT NULL,
    cash_out_cents bigint DEFAULT 0 NOT NULL,
    expected_cash_cents bigint DEFAULT 0 NOT NULL,
    counted_cash_cents bigint DEFAULT 0 NOT NULL,
    variance_cents bigint DEFAULT 0 NOT NULL,
    closing_note text DEFAULT ''::text NOT NULL,
    closed_by_user_id bigint,
    closed_by_staff_id bigint,
    closed_by_label character varying(255) DEFAULT ''::character varying NOT NULL,
    closed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    CONSTRAINT cash_register_sessions_counted_cash_check CHECK ((counted_cash_cents >= 0)),
    CONSTRAINT cash_register_sessions_opening_float_check CHECK ((opening_float_cents >= 0)),
    CONSTRAINT cash_register_sessions_status_check CHECK (((status)::text = ANY (ARRAY[('open'::character varying)::text, ('closed'::character varying)::text])))
);


--
-- Name: cash_register_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cash_register_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cash_register_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cash_register_sessions_id_seq OWNED BY public.cash_register_sessions.id;


--
-- Name: chat_channel_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_channel_members (
    channel_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    business_id bigint NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    muted_until timestamp with time zone,
    joined_at timestamp with time zone
);


--
-- Name: chat_channels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_channels (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    type text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    ref_key text DEFAULT ''::text NOT NULL,
    is_archived boolean DEFAULT false NOT NULL,
    created_by_staff_id bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: chat_channels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chat_channels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chat_channels_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.chat_channels_id_seq OWNED BY public.chat_channels.id;


--
-- Name: chat_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_messages (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    channel_id bigint NOT NULL,
    sender_staff_id bigint NOT NULL,
    sender_name text DEFAULT ''::text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    parent_id bigint,
    attachment_url text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);


--
-- Name: chat_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chat_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chat_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.chat_messages_id_seq OWNED BY public.chat_messages.id;


--
-- Name: chat_reads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_reads (
    channel_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    business_id bigint NOT NULL,
    last_read_message_id bigint DEFAULT 0 NOT NULL,
    last_read_at timestamp with time zone
);


--
-- Name: checklist_item_completions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_item_completions (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    run_id bigint NOT NULL,
    item_id bigint NOT NULL,
    staff_id bigint DEFAULT 0 NOT NULL,
    done boolean DEFAULT false NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: checklist_item_completions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.checklist_item_completions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: checklist_item_completions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.checklist_item_completions_id_seq OWNED BY public.checklist_item_completions.id;


--
-- Name: checklist_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_items (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    template_id bigint NOT NULL,
    label character varying(500) NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    is_required boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: checklist_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.checklist_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: checklist_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.checklist_items_id_seq OWNED BY public.checklist_items.id;


--
-- Name: checklist_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_runs (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    template_id bigint NOT NULL,
    assigned_staff_id bigint,
    shift_id bigint,
    for_date timestamp with time zone NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: checklist_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.checklist_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: checklist_runs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.checklist_runs_id_seq OWNED BY public.checklist_runs.id;


--
-- Name: checklist_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_templates (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    kind character varying(16) DEFAULT 'custom'::character varying NOT NULL,
    position_id bigint,
    is_active boolean DEFAULT true NOT NULL,
    created_by_staff_id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: checklist_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.checklist_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: checklist_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.checklist_templates_id_seq OWNED BY public.checklist_templates.id;


--
-- Name: comp_void_audit; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comp_void_audit (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    staff_id bigint,
    target_type character varying(32) NOT NULL,
    target_id character varying(64) NOT NULL,
    action character varying(16) NOT NULL,
    reason text,
    amount_cents bigint,
    pin_present boolean DEFAULT false,
    ip_address character varying(64),
    user_agent text,
    created_at timestamp with time zone
);


--
-- Name: comp_void_audit_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.comp_void_audit_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: comp_void_audit_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.comp_void_audit_id_seq OWNED BY public.comp_void_audit.id;


--
-- Name: conversion_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversion_events (
    id bigint NOT NULL,
    session_id character varying(255) NOT NULL,
    conversion_type character varying(100) NOT NULL,
    value numeric(10,2) DEFAULT 0,
    metadata text,
    "timestamp" timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: conversion_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.conversion_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: conversion_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.conversion_events_id_seq OWNED BY public.conversion_events.id;


--
-- Name: counters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.counters (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    counter_number bigint NOT NULL,
    name text NOT NULL,
    is_active boolean DEFAULT true,
    current_bill_id bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: counters_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.counters_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: counters_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.counters_id_seq OWNED BY public.counters.id;


--
-- Name: crypto_payment_quotes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.crypto_payment_quotes (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    business_id bigint NOT NULL,
    chain_id bigint NOT NULL,
    settlement_address character varying(42) NOT NULL,
    exact_microunits bigint NOT NULL,
    payment_method character varying(32) NOT NULL,
    base_microunits bigint NOT NULL,
    offset_microunits bigint NOT NULL,
    status character varying(16) DEFAULT 'active'::character varying NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    consumed_tx_hash character varying(66),
    payment_id bigint,
    client_key character varying(64),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT crypto_payment_quotes_address_chk CHECK (((settlement_address)::text ~ '^0x[0-9a-f]{40}$'::text)),
    CONSTRAINT crypto_payment_quotes_amount_chk CHECK (((base_microunits > 0) AND (exact_microunits = (base_microunits + offset_microunits)))),
    CONSTRAINT crypto_payment_quotes_client_key_chk CHECK (((client_key IS NULL) OR ((client_key)::text ~ '^[0-9a-f]{32}$'::text))),
    CONSTRAINT crypto_payment_quotes_consumed_chk CHECK ((((status)::text <> 'consumed'::text) OR ((consumed_at IS NOT NULL) AND (consumed_tx_hash IS NOT NULL)))),
    CONSTRAINT crypto_payment_quotes_offset_chk CHECK (((offset_microunits >= 1) AND (offset_microunits <= 9999))),
    CONSTRAINT crypto_payment_quotes_status_chk CHECK (((status)::text = ANY (ARRAY[('active'::character varying)::text, ('consumed'::character varying)::text, ('expired'::character varying)::text]))),
    CONSTRAINT crypto_payment_quotes_tx_hash_chk CHECK (((consumed_tx_hash IS NULL) OR ((consumed_tx_hash)::text ~ '^0x[0-9a-f]{64}$'::text))),
    CONSTRAINT crypto_payment_quotes_window_chk CHECK ((expires_at > issued_at))
);


--
-- Name: crypto_payment_quotes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.crypto_payment_quotes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: crypto_payment_quotes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.crypto_payment_quotes_id_seq OWNED BY public.crypto_payment_quotes.id;


--
-- Name: customer_addresses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customer_addresses (
    id bigint NOT NULL,
    customer_id bigint NOT NULL,
    business_id bigint,
    label character varying(100),
    street character varying(255) NOT NULL,
    apartment character varying(100),
    city character varying(100) NOT NULL,
    state character varying(100),
    postal_code character varying(20),
    country character varying(100) NOT NULL,
    formatted_address text,
    latitude numeric(10,8),
    longitude numeric(11,8),
    delivery_instructions text,
    contactless_delivery boolean DEFAULT false,
    leave_at_door boolean DEFAULT false,
    contact_name character varying(255),
    contact_phone character varying(50),
    is_default boolean DEFAULT false,
    is_active boolean DEFAULT true,
    last_used_at timestamp with time zone,
    usage_count bigint DEFAULT 0,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: customer_addresses_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.customer_addresses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: customer_addresses_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.customer_addresses_id_seq OWNED BY public.customer_addresses.id;


--
-- Name: customer_businesses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customer_businesses (
    id bigint NOT NULL,
    customer_id bigint NOT NULL,
    business_id bigint NOT NULL,
    loyalty_points bigint DEFAULT 0,
    loyalty_tier text,
    total_spent numeric DEFAULT 0,
    visit_count bigint DEFAULT 0,
    last_visit_at timestamp with time zone,
    first_visit_at timestamp with time zone,
    opt_in_marketing boolean DEFAULT false,
    opt_in_sms boolean DEFAULT false,
    opt_in_email boolean DEFAULT true,
    favorite_items text,
    dietary_preferences text,
    allergies text,
    notes text,
    tags text,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: customer_businesses_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.customer_businesses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: customer_businesses_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.customer_businesses_id_seq OWNED BY public.customer_businesses.id;


--
-- Name: customer_communications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customer_communications (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    customer_business_id bigint NOT NULL,
    type text NOT NULL,
    status text DEFAULT 'pending'::text,
    subject text,
    content text,
    scheduled_for timestamp with time zone,
    sent_at timestamp with time zone,
    delivered_at timestamp with time zone,
    opened_at timestamp with time zone,
    clicked_at timestamp with time zone,
    error_message text,
    campaign_id text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: customer_communications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.customer_communications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: customer_communications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.customer_communications_id_seq OWNED BY public.customer_communications.id;


--
-- Name: customer_preferences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customer_preferences (
    id bigint NOT NULL,
    customer_id bigint NOT NULL,
    preferred_language text DEFAULT 'en'::text,
    preferred_currency text DEFAULT 'USD'::text,
    receive_promotions boolean DEFAULT true,
    receive_newsletters boolean DEFAULT true,
    receive_birthday_offers boolean DEFAULT true,
    share_data_with_businesses boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: customer_preferences_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.customer_preferences_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: customer_preferences_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.customer_preferences_id_seq OWNED BY public.customer_preferences.id;


--
-- Name: customer_visits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customer_visits (
    id bigint NOT NULL,
    customer_business_id bigint NOT NULL,
    bill_id bigint,
    table_id bigint,
    amount_spent numeric,
    points_earned bigint,
    items_purchased text,
    visit_date timestamp with time zone,
    visit_duration bigint,
    rating bigint,
    feedback text,
    created_at timestamp with time zone
);


--
-- Name: customer_visits_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.customer_visits_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: customer_visits_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.customer_visits_id_seq OWNED BY public.customer_visits.id;


--
-- Name: customers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customers (
    id bigint NOT NULL,
    email text NOT NULL,
    password_hash text,
    name text,
    phone text,
    wallet_address text,
    birthday timestamp with time zone,
    profile_image_url text,
    is_active boolean DEFAULT true,
    email_verified boolean DEFAULT false,
    verification_token text,
    password_reset_token text,
    password_reset_expiry timestamp with time zone,
    last_login_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: customers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.customers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: customers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.customers_id_seq OWNED BY public.customers.id;


--
-- Name: delivery_drivers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.delivery_drivers (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    staff_id bigint,
    name text NOT NULL,
    phone text NOT NULL,
    email text,
    license_number text,
    vehicle_type text,
    vehicle_plate text,
    status text DEFAULT 'offline'::text,
    is_available boolean DEFAULT true,
    current_latitude numeric,
    current_longitude numeric,
    "current_timestamp" timestamp with time zone,
    total_deliveries bigint DEFAULT 0,
    completed_deliveries bigint DEFAULT 0,
    average_rating numeric DEFAULT 0,
    total_earnings numeric DEFAULT 0,
    current_delivery_id bigint,
    is_active boolean DEFAULT true,
    last_active_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: delivery_drivers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.delivery_drivers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: delivery_drivers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.delivery_drivers_id_seq OWNED BY public.delivery_drivers.id;


--
-- Name: delivery_orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.delivery_orders (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    bill_id bigint NOT NULL,
    order_id bigint,
    customer_id bigint,
    zone_id bigint,
    delivery_number text NOT NULL,
    delivery_type text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    priority text,
    fulfillment_mode text DEFAULT 'in_house'::text,
    driver_id bigint,
    assigned_at timestamp with time zone,
    customer_name text NOT NULL,
    customer_phone text NOT NULL,
    customer_email text,
    customer_locale text DEFAULT ''::text,
    delivery_street text,
    delivery_apartment text,
    delivery_city text,
    delivery_state text,
    delivery_postal_code text,
    delivery_country text,
    delivery_formatted_address text,
    pickup_latitude numeric,
    pickup_longitude numeric,
    pickup_timestamp timestamp with time zone,
    dropoff_latitude numeric,
    dropoff_longitude numeric,
    dropoff_timestamp timestamp with time zone,
    current_latitude numeric,
    current_longitude numeric,
    "current_timestamp" timestamp with time zone,
    estimated_pickup_time timestamp with time zone,
    actual_pickup_time timestamp with time zone,
    estimated_delivery_time timestamp with time zone,
    actual_delivery_time timestamp with time zone,
    delivery_fee bigint,
    driver_tip bigint DEFAULT 0,
    platform_fee bigint,
    delivery_instructions text,
    contactless_delivery boolean DEFAULT false,
    leave_at_door boolean DEFAULT false,
    signature_url text,
    photo_url text,
    delivery_code text,
    quote_metadata jsonb DEFAULT '{}'::jsonb,
    cutoff_at timestamp with time zone,
    payment_expires_at timestamp with time zone,
    payment_mode_stored text DEFAULT ''::text,
    external_order_id text,
    external_tracking_url text,
    cancelled_at timestamp with time zone,
    cancelled_by text,
    cancellation_reason text,
    customer_rating bigint,
    customer_feedback text,
    driver_rating bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    claimed_by_staff_id bigint,
    claimed_by_name text DEFAULT ''::text NOT NULL,
    claimed_by_role text DEFAULT ''::text NOT NULL,
    claimed_at timestamp with time zone
);


--
-- Name: delivery_orders_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.delivery_orders_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: delivery_orders_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.delivery_orders_id_seq OWNED BY public.delivery_orders.id;


--
-- Name: delivery_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.delivery_settings (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    delivery_enabled boolean DEFAULT false,
    in_house_delivery_enabled boolean DEFAULT false,
    third_party_enabled boolean DEFAULT false,
    uber_eats_enabled boolean DEFAULT false,
    doordash_enabled boolean DEFAULT false,
    grubhub_enabled boolean DEFAULT false,
    uber_eats_api_key text,
    doordash_api_key text,
    grubhub_api_key text,
    payment_mode text DEFAULT ''::text,
    default_delivery_fee bigint,
    free_delivery_threshold bigint,
    minimum_order_amount bigint,
    max_delivery_radius numeric,
    estimated_prep_time bigint,
    max_concurrent_deliveries bigint DEFAULT 5,
    delivery_hours_same_as_business boolean DEFAULT true,
    delivery_start_time text,
    delivery_end_time text,
    delivery_instructions text,
    delivery_zones jsonb,
    external_partner_links jsonb DEFAULT '[]'::jsonb,
    auto_assign_drivers boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: delivery_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.delivery_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: delivery_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.delivery_settings_id_seq OWNED BY public.delivery_settings.id;


--
-- Name: delivery_status_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.delivery_status_history (
    id bigint NOT NULL,
    delivery_order_id bigint NOT NULL,
    status text NOT NULL,
    latitude numeric,
    longitude numeric,
    "timestamp" timestamp with time zone,
    notes text,
    changed_by text,
    created_at timestamp with time zone
);


--
-- Name: delivery_status_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.delivery_status_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: delivery_status_history_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.delivery_status_history_id_seq OWNED BY public.delivery_status_history.id;


--
-- Name: delivery_zones; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.delivery_zones (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    name text NOT NULL,
    description text,
    boundaries text,
    delivery_fee bigint,
    minimum_order_amount bigint,
    estimated_time bigint,
    priority bigint DEFAULT 0,
    cutoff_buffer_minutes bigint DEFAULT 0,
    is_active boolean DEFAULT true,
    operating_hours text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: delivery_zones_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.delivery_zones_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: delivery_zones_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.delivery_zones_id_seq OWNED BY public.delivery_zones.id;


--
-- Name: demo_instances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.demo_instances (
    id bigint NOT NULL,
    admin_user_id bigint NOT NULL,
    primary_business_id bigint,
    secondary_business_id bigint,
    status text DEFAULT 'creating'::text NOT NULL,
    seed_version text NOT NULL,
    baseline_start_date date NOT NULL,
    last_simulated_business_date date,
    timezone text DEFAULT 'America/New_York'::text NOT NULL,
    last_error text,
    last_verified_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: demo_instances_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.demo_instances_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: demo_instances_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.demo_instances_id_seq OWNED BY public.demo_instances.id;


--
-- Name: demo_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.demo_runs (
    id bigint NOT NULL,
    demo_instance_id bigint NOT NULL,
    admin_user_id bigint NOT NULL,
    run_type text NOT NULL,
    status text NOT NULL,
    seed_version text NOT NULL,
    started_at timestamp with time zone NOT NULL,
    finished_at timestamp with time zone,
    business_date_from date,
    business_date_to date,
    records_created jsonb DEFAULT '{}'::jsonb NOT NULL,
    verification jsonb DEFAULT '{}'::jsonb NOT NULL,
    error text
);


--
-- Name: demo_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.demo_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: demo_runs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.demo_runs_id_seq OWNED BY public.demo_runs.id;


--
-- Name: director_action_audits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.director_action_audits (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    thread_id bigint NOT NULL,
    proposed_action_id bigint NOT NULL,
    actor_user_id bigint NOT NULL,
    kind character varying(32) NOT NULL,
    params_json text DEFAULT '{}'::text NOT NULL,
    before_json text DEFAULT '[]'::text NOT NULL,
    after_json text DEFAULT '[]'::text NOT NULL,
    applied_at timestamp with time zone,
    undone_at timestamp with time zone,
    undone_by bigint
);


--
-- Name: director_action_audits_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.director_action_audits_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: director_action_audits_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.director_action_audits_id_seq OWNED BY public.director_action_audits.id;


--
-- Name: director_console_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.director_console_messages (
    id bigint NOT NULL,
    thread_id bigint NOT NULL,
    business_id bigint NOT NULL,
    role character varying(16) NOT NULL,
    locale character varying(8) DEFAULT 'en'::character varying,
    content text,
    structured_response text,
    model_name character varying(64),
    latency_ms bigint,
    feedback_vote character varying(8),
    feedback_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: director_console_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.director_console_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: director_console_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.director_console_messages_id_seq OWNED BY public.director_console_messages.id;


--
-- Name: director_console_threads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.director_console_threads (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    title text NOT NULL,
    locale character varying(8) DEFAULT 'en'::character varying,
    last_message_at timestamp with time zone,
    archived_at timestamp with time zone,
    pinned boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: director_console_threads_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.director_console_threads_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: director_console_threads_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.director_console_threads_id_seq OWNED BY public.director_console_threads.id;


--
-- Name: director_proposed_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.director_proposed_actions (
    id bigint NOT NULL,
    public_id character varying(40) NOT NULL,
    business_id bigint NOT NULL,
    thread_id bigint NOT NULL,
    message_id bigint,
    kind character varying(32) NOT NULL,
    params_json text DEFAULT '{}'::text NOT NULL,
    preview_json text DEFAULT '{}'::text NOT NULL,
    menu_version bigint DEFAULT 0 NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    created_at timestamp with time zone,
    expires_at timestamp with time zone
);


--
-- Name: director_proposed_actions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.director_proposed_actions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: director_proposed_actions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.director_proposed_actions_id_seq OWNED BY public.director_proposed_actions.id;


--
-- Name: director_tool_calls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.director_tool_calls (
    id bigint NOT NULL,
    thread_id bigint NOT NULL,
    message_id bigint,
    business_id bigint NOT NULL,
    tool_name character varying(128) NOT NULL,
    args_json text DEFAULT '{}'::text NOT NULL,
    summary text DEFAULT ''::text NOT NULL,
    duration_ms bigint DEFAULT 0 NOT NULL,
    success boolean DEFAULT true NOT NULL,
    error text,
    created_at timestamp with time zone,
    deleted_at timestamp with time zone
);


--
-- Name: director_tool_calls_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.director_tool_calls_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: director_tool_calls_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.director_tool_calls_id_seq OWNED BY public.director_tool_calls.id;


--
-- Name: document_acks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_acks (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    document_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    version bigint NOT NULL,
    acknowledged_at timestamp with time zone,
    created_at timestamp with time zone
);


--
-- Name: document_acks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.document_acks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: document_acks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.document_acks_id_seq OWNED BY public.document_acks.id;


--
-- Name: documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.documents (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    created_by_staff_id bigint NOT NULL,
    title character varying(255) NOT NULL,
    url character varying(1024) DEFAULT ''::character varying NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    require_ack boolean DEFAULT false NOT NULL,
    audience_filter character varying(64) DEFAULT 'all'::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: documents_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.documents_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: documents_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.documents_id_seq OWNED BY public.documents.id;


--
-- Name: email_delivery_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_delivery_events (
    id bigint NOT NULL,
    provider character varying(32) NOT NULL,
    webhook_id character varying(255) NOT NULL,
    event_type character varying(64) NOT NULL,
    provider_email_id character varying(255) DEFAULT ''::character varying NOT NULL,
    recipient_email character varying(320) DEFAULT ''::character varying NOT NULL,
    occurred_at timestamp with time zone,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT email_delivery_events_recipient_canonical_check CHECK ((((recipient_email)::text = ''::text) OR ((recipient_email)::text = lower(btrim((recipient_email)::text)))))
);


--
-- Name: email_delivery_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.email_delivery_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: email_delivery_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.email_delivery_events_id_seq OWNED BY public.email_delivery_events.id;


--
-- Name: email_outbound_sends; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_outbound_sends (
    id bigint NOT NULL,
    provider character varying(32) NOT NULL,
    provider_message_id character varying(255) NOT NULL,
    recipient_redacted character varying(320) DEFAULT ''::character varying NOT NULL,
    template_name character varying(128) DEFAULT ''::character varying NOT NULL,
    tag character varying(64) DEFAULT ''::character varying NOT NULL,
    message_type character varying(32) DEFAULT ''::character varying NOT NULL,
    status character varying(32) DEFAULT 'sent'::character varying NOT NULL,
    last_event_type character varying(64) DEFAULT ''::character varying NOT NULL,
    sent_at timestamp with time zone NOT NULL,
    last_event_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: email_outbound_sends_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.email_outbound_sends_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: email_outbound_sends_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.email_outbound_sends_id_seq OWNED BY public.email_outbound_sends.id;


--
-- Name: email_outbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_outbox (
    id bigint NOT NULL,
    idempotency_key character varying(255) NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    provider character varying(32) DEFAULT ''::character varying NOT NULL,
    provider_message_id character varying(255) DEFAULT ''::character varying NOT NULL,
    template_name character varying(128) DEFAULT ''::character varying NOT NULL,
    tag character varying(64) DEFAULT ''::character varying NOT NULL,
    message_type character varying(32) DEFAULT ''::character varying NOT NULL,
    recipient_redacted character varying(320) DEFAULT ''::character varying NOT NULL,
    payload text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone NOT NULL,
    last_attempt_at timestamp with time zone,
    sent_at timestamp with time zone,
    failed_at timestamp with time zone,
    last_error_code character varying(64) DEFAULT ''::character varying NOT NULL,
    last_error_message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    business_id bigint,
    reservation_id bigint,
    bill_id bigint,
    delivery_order_id bigint,
    customer_communication_id bigint,
    delivery_status character varying(24) DEFAULT ''::character varying NOT NULL,
    delivery_detail character varying(255) DEFAULT ''::character varying NOT NULL,
    delivery_event_at timestamp with time zone
);


--
-- Name: email_outbox_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.email_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: email_outbox_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.email_outbox_id_seq OWNED BY public.email_outbox.id;


--
-- Name: email_suppressions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_suppressions (
    email character varying(320) NOT NULL,
    reason character varying(32) NOT NULL,
    provider character varying(32) NOT NULL,
    provider_event_id character varying(255) DEFAULT ''::character varying NOT NULL,
    provider_email_id character varying(255) DEFAULT ''::character varying NOT NULL,
    detail text DEFAULT ''::text NOT NULL,
    suppressed_at timestamp with time zone NOT NULL,
    last_event_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT email_suppressions_email_canonical_check CHECK (((email)::text = lower(btrim((email)::text)))),
    CONSTRAINT email_suppressions_reason_check CHECK (((reason)::text = ANY (ARRAY[('bounce'::character varying)::text, ('complaint'::character varying)::text, ('provider_suppressed'::character varying)::text, ('manual'::character varying)::text])))
);


--
-- Name: error_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.error_logs (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    "timestamp" timestamp with time zone,
    source text,
    component text,
    function text,
    error text,
    message text,
    stack text,
    user_id text,
    request_id text,
    metadata jsonb,
    additional_info text
);


--
-- Name: error_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.error_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: error_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.error_logs_id_seq OWNED BY public.error_logs.id;


--
-- Name: escalations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.escalations (
    id bigint NOT NULL,
    source character varying(16) NOT NULL,
    business_id bigint,
    session_ref character varying(80),
    issue text,
    transcript_summary text,
    contact_email character varying(320),
    status character varying(16) DEFAULT 'open'::character varying NOT NULL,
    admin_notes text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    request_id character varying(64)
);


--
-- Name: escalations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.escalations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: escalations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.escalations_id_seq OWNED BY public.escalations.id;


--
-- Name: exchange_rates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.exchange_rates (
    id bigint NOT NULL,
    from_currency character varying(10) NOT NULL,
    to_currency character varying(10) NOT NULL,
    rate numeric NOT NULL,
    source character varying(50) DEFAULT 'coinbase'::character varying,
    fetched_at timestamp with time zone NOT NULL,
    last_seen_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: exchange_rates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.exchange_rates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: exchange_rates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.exchange_rates_id_seq OWNED BY public.exchange_rates.id;


--
-- Name: fiscal_audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fiscal_audit_events (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    receipt_id bigint,
    job_id bigint,
    actor character varying(64) DEFAULT 'system'::character varying NOT NULL,
    event_type character varying(64) NOT NULL,
    message text NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone
);


--
-- Name: fiscal_audit_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.fiscal_audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: fiscal_audit_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.fiscal_audit_events_id_seq OWNED BY public.fiscal_audit_events.id;


--
-- Name: fiscal_delivery_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fiscal_delivery_tasks (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    receipt_id bigint NOT NULL,
    channel character varying(16) NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 8 NOT NULL,
    next_attempt_at timestamp with time zone,
    lease_owner character varying(64),
    lease_expires_at timestamp with time zone,
    last_error text,
    idempotency_key character varying(200) NOT NULL,
    provider_message_id character varying(128),
    locale character varying(16),
    succeeded_at timestamp with time zone,
    dead_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT fiscal_delivery_tasks_channel_check CHECK (((channel)::text = ANY (ARRAY[('artifact'::character varying)::text, ('email'::character varying)::text, ('print'::character varying)::text]))),
    CONSTRAINT fiscal_delivery_tasks_status_check CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('leased'::character varying)::text, ('succeeded'::character varying)::text, ('dead'::character varying)::text])))
);


--
-- Name: TABLE fiscal_delivery_tasks; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.fiscal_delivery_tasks IS 'Per-channel durable delivery of authorized fiscal receipts (artifact/email/print)';


--
-- Name: fiscal_delivery_tasks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.fiscal_delivery_tasks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: fiscal_delivery_tasks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.fiscal_delivery_tasks_id_seq OWNED BY public.fiscal_delivery_tasks.id;


--
-- Name: fiscal_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fiscal_jobs (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    settings_id bigint NOT NULL,
    receipt_id bigint,
    produced_receipt_id bigint,
    bill_id bigint NOT NULL,
    payment_id bigint,
    alternative_payment_id bigint,
    action character varying(32) NOT NULL,
    idempotency_key character varying(160) NOT NULL,
    credit_amount_cents bigint,
    attempted_provider_receipt_id character varying(64),
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    attempts bigint DEFAULT 0 NOT NULL,
    max_attempts bigint DEFAULT 5 NOT NULL,
    next_attempt_at timestamp with time zone,
    last_error_code character varying(64),
    last_error_message text,
    locked_at timestamp with time zone,
    locked_by character varying(64),
    created_by character varying(64) DEFAULT 'system'::character varying NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: fiscal_jobs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.fiscal_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: fiscal_jobs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.fiscal_jobs_id_seq OWNED BY public.fiscal_jobs.id;


--
-- Name: fiscal_receipts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fiscal_receipts (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    settings_id bigint NOT NULL,
    bill_id bigint NOT NULL,
    payment_id bigint,
    alternative_payment_id bigint,
    country character varying(2) NOT NULL,
    provider character varying(64) NOT NULL,
    action character varying(32) NOT NULL,
    receipt_type character varying(32) NOT NULL,
    receipt_number character varying(64),
    provider_receipt_id character varying(128),
    auth_code character varying(128),
    auth_expires_at timestamp with time zone,
    qr_payload text,
    qr_image_path character varying(256),
    pdf_path character varying(256),
    customer_doc_type character varying(32),
    customer_doc_number character varying(64),
    total_amount_cents bigint DEFAULT 0 NOT NULL,
    tip_amount_cents bigint DEFAULT 0 NOT NULL,
    currency character varying(8) DEFAULT 'USD'::character varying NOT NULL,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    error_code character varying(64),
    error_message text,
    raw_request jsonb,
    raw_response jsonb,
    issued_at timestamp with time zone,
    delivered_at timestamp with time zone,
    delivery_locked_at timestamp with time zone,
    delivery_locked_by text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    customer_name text,
    customer_tax_condition character varying(32)
);


--
-- Name: fiscal_receipts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.fiscal_receipts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: fiscal_receipts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.fiscal_receipts_id_seq OWNED BY public.fiscal_receipts.id;


--
-- Name: guest_receipt_sends; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_receipt_sends (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    recipient_redacted text NOT NULL
);


--
-- Name: guest_receipt_sends_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.guest_receipt_sends_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: guest_receipt_sends_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.guest_receipt_sends_id_seq OWNED BY public.guest_receipt_sends.id;


--
-- Name: idempotency_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.idempotency_keys (
    id bigint NOT NULL,
    key text NOT NULL,
    endpoint text NOT NULL,
    business_id bigint,
    request_hash text NOT NULL,
    response_status integer DEFAULT 0 NOT NULL,
    response_body bytea,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    ttl_at timestamp with time zone DEFAULT (now() + '24:00:00'::interval) NOT NULL
);


--
-- Name: idempotency_keys_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.idempotency_keys_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: idempotency_keys_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.idempotency_keys_id_seq OWNED BY public.idempotency_keys.id;


--
-- Name: inventory_alert_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_alert_logs (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    inventory_item_id bigint NOT NULL,
    alert_type text NOT NULL,
    alerted_at timestamp with time zone NOT NULL,
    alert_day date GENERATED ALWAYS AS (((alerted_at AT TIME ZONE 'UTC'::text))::date) STORED
);


--
-- Name: inventory_alert_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.inventory_alert_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: inventory_alert_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.inventory_alert_logs_id_seq OWNED BY public.inventory_alert_logs.id;


--
-- Name: inventory_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_items (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    name text NOT NULL,
    sku text,
    category text,
    unit text DEFAULT 'unit'::text NOT NULL,
    current_quantity numeric DEFAULT 0,
    reorder_threshold numeric DEFAULT 0,
    cost_per_unit numeric DEFAULT 0,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: inventory_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.inventory_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: inventory_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.inventory_items_id_seq OWNED BY public.inventory_items.id;


--
-- Name: inventory_movements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_movements (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    inventory_item_id bigint NOT NULL,
    movement_type text NOT NULL,
    quantity_delta numeric NOT NULL,
    quantity_before numeric NOT NULL,
    quantity_after numeric NOT NULL,
    reason text,
    actor text,
    menu_item_id text,
    menu_item_name text,
    reference_order_id bigint,
    reference_bill_id bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: inventory_movements_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.inventory_movements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: inventory_movements_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.inventory_movements_id_seq OWNED BY public.inventory_movements.id;


--
-- Name: inventory_recipes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_recipes (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    menu_item_id text NOT NULL,
    menu_item_name text,
    inventory_item_id bigint NOT NULL,
    quantity_required numeric NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: inventory_recipes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.inventory_recipes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: inventory_recipes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.inventory_recipes_id_seq OWNED BY public.inventory_recipes.id;


--
-- Name: inventory_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_settings (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    inventory_enabled boolean DEFAULT false,
    auto_deduct_on_order_approval boolean DEFAULT true,
    low_stock_warnings_enabled boolean DEFAULT true,
    availability_sync_mode text DEFAULT 'warn'::text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: inventory_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.inventory_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: inventory_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.inventory_settings_id_seq OWNED BY public.inventory_settings.id;


--
-- Name: ledger_entry_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ledger_entry_attachments (
    id bigint NOT NULL,
    entry_id bigint NOT NULL,
    business_id bigint NOT NULL,
    s3_key text NOT NULL,
    file_name character varying(255) NOT NULL,
    content_type character varying(128) DEFAULT ''::character varying NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    uploaded_by_user_id bigint,
    uploaded_by_staff_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ledger_entry_attachments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ledger_entry_attachments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ledger_entry_attachments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ledger_entry_attachments_id_seq OWNED BY public.ledger_entry_attachments.id;


--
-- Name: loyalty_programs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.loyalty_programs (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    points_per_dollar double precision DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    redemption_points_per_dollar numeric DEFAULT 100 NOT NULL
);


--
-- Name: loyalty_programs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.loyalty_programs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: loyalty_programs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.loyalty_programs_id_seq OWNED BY public.loyalty_programs.id;


--
-- Name: loyalty_tiers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.loyalty_tiers (
    id bigint NOT NULL,
    loyalty_program_id bigint NOT NULL,
    name text NOT NULL,
    min_lifetime_spent_cents bigint NOT NULL,
    sort_order bigint NOT NULL,
    color text
);


--
-- Name: loyalty_tiers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.loyalty_tiers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: loyalty_tiers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.loyalty_tiers_id_seq OWNED BY public.loyalty_tiers.id;


--
-- Name: manual_ledger_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.manual_ledger_entries (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    entry_type text NOT NULL,
    category text NOT NULL,
    amount bigint NOT NULL,
    currency text NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    description text NOT NULL,
    notes text,
    reference text,
    created_by_user_id bigint,
    created_by_staff_id bigint,
    voided_at timestamp with time zone,
    voided_by_user_id bigint,
    voided_by_staff_id bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: manual_ledger_entries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.manual_ledger_entries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: manual_ledger_entries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.manual_ledger_entries_id_seq OWNED BY public.manual_ledger_entries.id;


--
-- Name: marketing_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.marketing_activities (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    suggestion_id character varying(128) NOT NULL,
    play character varying(32) NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    target_name text DEFAULT ''::text NOT NULL,
    status character varying(16) NOT NULL,
    image_url text DEFAULT ''::text NOT NULL,
    caption text DEFAULT ''::text NOT NULL,
    posted_at timestamp with time zone,
    dismissed_at timestamp with time zone,
    created_by character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    creative_snapshot jsonb,
    CONSTRAINT marketing_activities_status_check CHECK (((status)::text = ANY (ARRAY[('posted'::character varying)::text, ('dismissed'::character varying)::text])))
);


--
-- Name: TABLE marketing_activities; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.marketing_activities IS 'Durable per-suggestion marketing activity (posted/dismissed); restore deletes a dismissed row, posted rows are permanent';


--
-- Name: COLUMN marketing_activities.creative_snapshot; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.marketing_activities.creative_snapshot IS 'Exact approved marketing creative: caption, image provenance, template, aspect, editable slots, and crop';


--
-- Name: marketing_activities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.marketing_activities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: marketing_activities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.marketing_activities_id_seq OWNED BY public.marketing_activities.id;


--
-- Name: menu_extraction_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menu_extraction_images (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    file_path text NOT NULL,
    page_order bigint,
    created_at timestamp with time zone,
    mime_type character varying(32) DEFAULT 'image/jpeg'::character varying NOT NULL,
    storage_key text
);


--
-- Name: COLUMN menu_extraction_images.mime_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.menu_extraction_images.mime_type IS 'Verified content type of the stored page (image/jpeg|image/png|image/webp)';


--
-- Name: COLUMN menu_extraction_images.storage_key; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.menu_extraction_images.storage_key IS 'Protected-object storage key for restart-safe provider input; FilePath is legacy';


--
-- Name: menu_extraction_images_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.menu_extraction_images_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: menu_extraction_images_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.menu_extraction_images_id_seq OWNED BY public.menu_extraction_images.id;


--
-- Name: menu_extraction_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menu_extraction_jobs (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    status text DEFAULT 'pending'::text,
    error_message text,
    extracted_menu text,
    image_count bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    claimed_at timestamp with time zone,
    claim_token character varying(64),
    attempt_count bigint DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone
);


--
-- Name: COLUMN menu_extraction_jobs.claim_token; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.menu_extraction_jobs.claim_token IS 'Opaque token held by the worker that currently owns the claim; not exposed on the wire';


--
-- Name: menu_extraction_jobs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.menu_extraction_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: menu_extraction_jobs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.menu_extraction_jobs_id_seq OWNED BY public.menu_extraction_jobs.id;


--
-- Name: menu_wizard_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menu_wizard_messages (
    id bigint NOT NULL,
    session_id bigint NOT NULL,
    role text NOT NULL,
    content text NOT NULL,
    created_at timestamp with time zone
);


--
-- Name: menu_wizard_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.menu_wizard_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: menu_wizard_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.menu_wizard_messages_id_seq OWNED BY public.menu_wizard_messages.id;


--
-- Name: menu_wizard_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menu_wizard_sessions (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    status text DEFAULT 'in_progress'::text,
    generated_menu text,
    config text,
    language character varying(16),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: menu_wizard_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.menu_wizard_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: menu_wizard_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.menu_wizard_sessions_id_seq OWNED BY public.menu_wizard_sessions.id;


--
-- Name: menus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menus (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    categories text,
    is_active boolean DEFAULT true,
    version bigint DEFAULT 1,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: menus_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.menus_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: menus_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.menus_id_seq OWNED BY public.menus.id;


--
-- Name: missing_translations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.missing_translations (
    id bigint NOT NULL,
    locale character varying(32) NOT NULL,
    key_path character varying(255) NOT NULL,
    page character varying(500) DEFAULT ''::character varying NOT NULL,
    fallback_used character varying(16) DEFAULT 'leaf'::character varying NOT NULL,
    occurrence_count bigint DEFAULT 1 NOT NULL,
    first_seen_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_seen_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    status character varying(16) DEFAULT 'open'::character varying NOT NULL,
    status_updated_at timestamp with time zone,
    CONSTRAINT missing_translations_status_check CHECK (((status)::text = ANY (ARRAY[('open'::character varying)::text, ('resolved'::character varying)::text, ('ignored'::character varying)::text])))
);


--
-- Name: missing_translations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.missing_translations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: missing_translations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.missing_translations_id_seq OWNED BY public.missing_translations.id;


--
-- Name: offers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.offers (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    name text NOT NULL,
    description text,
    image text,
    discount_type text NOT NULL,
    discount_value numeric NOT NULL,
    start_date timestamp with time zone,
    end_date timestamp with time zone,
    is_active boolean,
    applicable_to text DEFAULT 'all'::text,
    target_id text,
    code character varying(50),
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    weekday_mask smallint DEFAULT 127 NOT NULL,
    start_minute integer,
    end_minute integer,
    CONSTRAINT offers_end_minute_check CHECK (((end_minute IS NULL) OR ((end_minute >= 0) AND (end_minute <= 1439)))),
    CONSTRAINT offers_start_minute_check CHECK (((start_minute IS NULL) OR ((start_minute >= 0) AND (start_minute <= 1439)))),
    CONSTRAINT offers_time_window_pair_check CHECK (((start_minute IS NULL) = (end_minute IS NULL))),
    CONSTRAINT offers_weekday_mask_check CHECK (((weekday_mask >= 1) AND (weekday_mask <= 127)))
);


--
-- Name: offers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.offers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: offers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.offers_id_seq OWNED BY public.offers.id;


--
-- Name: open_shift_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.open_shift_claims (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    shift_id bigint NOT NULL,
    claiming_staff_id bigint NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    decided_by_staff_id bigint,
    decided_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: open_shift_claims_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.open_shift_claims_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: open_shift_claims_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.open_shift_claims_id_seq OWNED BY public.open_shift_claims.id;


--
-- Name: operational_alert_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operational_alert_events (
    id bigint NOT NULL,
    alert_id bigint NOT NULL,
    business_id bigint NOT NULL,
    event_type text NOT NULL,
    actor_staff_id bigint,
    actor_user_id bigint,
    actor_name text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: operational_alert_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.operational_alert_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: operational_alert_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.operational_alert_events_id_seq OWNED BY public.operational_alert_events.id;


--
-- Name: operational_alerts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operational_alerts (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    alert_type text NOT NULL,
    resource_type text NOT NULL,
    resource_id bigint NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    priority text DEFAULT 'normal'::text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    claimed_by_staff_id bigint,
    claimed_by_user_id bigint,
    claimed_by_name text DEFAULT ''::text NOT NULL,
    claimed_at timestamp with time zone,
    resolved_at timestamp with time zone,
    snoozed_until timestamp with time zone,
    last_event_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: operational_alerts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.operational_alerts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: operational_alerts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.operational_alerts_id_seq OWNED BY public.operational_alerts.id;


--
-- Name: ops_assistant_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ops_assistant_messages (
    id bigint NOT NULL,
    thread_id bigint NOT NULL,
    business_id bigint NOT NULL,
    role character varying(16) NOT NULL,
    locale character varying(16),
    content text,
    structured_response jsonb,
    model_name character varying(64),
    latency_ms bigint,
    feedback character varying(8),
    created_at timestamp with time zone,
    request_id bigint
);


--
-- Name: ops_assistant_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ops_assistant_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ops_assistant_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ops_assistant_messages_id_seq OWNED BY public.ops_assistant_messages.id;


--
-- Name: ops_assistant_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ops_assistant_requests (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    thread_id bigint,
    client_request_id character varying(64) NOT NULL,
    status character varying(24) DEFAULT 'pending'::character varying NOT NULL,
    claim_token character varying(64),
    claimed_at timestamp with time zone,
    error_code character varying(64),
    user_message_id bigint,
    assistant_message_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_ops_assistant_requests_status CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('processing'::character varying)::text, ('completed'::character varying)::text, ('failed'::character varying)::text])))
);


--
-- Name: ops_assistant_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ops_assistant_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ops_assistant_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ops_assistant_requests_id_seq OWNED BY public.ops_assistant_requests.id;


--
-- Name: ops_assistant_threads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ops_assistant_threads (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    title character varying(200),
    locale character varying(16),
    last_message_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: ops_assistant_threads_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ops_assistant_threads_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ops_assistant_threads_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ops_assistant_threads_id_seq OWNED BY public.ops_assistant_threads.id;


--
-- Name: ops_assistant_tool_calls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ops_assistant_tool_calls (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    thread_id bigint NOT NULL,
    message_id bigint,
    tool_name character varying(64) NOT NULL,
    arguments_json jsonb,
    result_summary text,
    status character varying(16),
    created_at timestamp with time zone
);


--
-- Name: ops_assistant_tool_calls_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ops_assistant_tool_calls_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ops_assistant_tool_calls_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ops_assistant_tool_calls_id_seq OWNED BY public.ops_assistant_tool_calls.id;


--
-- Name: orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orders (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    business_id bigint NOT NULL,
    order_number text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_by text,
    client_request_id character varying(64),
    approved_by text,
    cancelled_by text,
    notes text,
    items text,
    cancel_reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    approved_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    kitchen_acked_at timestamp with time zone,
    quote_snapshot jsonb
);


--
-- Name: COLUMN orders.quote_snapshot; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.orders.quote_snapshot IS 'Immutable versioned cent-exact checkout quote used for idempotent guest-order replay; NULL on legacy rows.';


--
-- Name: orders_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.orders_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: orders_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.orders_id_seq OWNED BY public.orders.id;


--
-- Name: page_views; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_views (
    id bigint NOT NULL,
    session_id character varying(255) NOT NULL,
    page character varying(500) NOT NULL,
    referrer character varying(500),
    user_agent text,
    ip_address character varying(45),
    country character varying(100),
    city character varying(100),
    device_type character varying(50),
    browser character varying(50),
    os character varying(50),
    screen_width integer,
    screen_height integer,
    locale character varying(10),
    "timestamp" timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    duration integer DEFAULT 0
);


--
-- Name: page_views_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.page_views_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: page_views_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.page_views_id_seq OWNED BY public.page_views.id;


--
-- Name: payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payments (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    payer_addr text NOT NULL,
    amount bigint NOT NULL,
    tip_amount bigint DEFAULT 0,
    tx_hash text,
    status text DEFAULT 'pending'::text,
    payment_method text DEFAULT 'crypto'::text,
    source_chain text,
    source_token text,
    settlement_chain text DEFAULT 'base'::text,
    lifi_route_id text,
    confirmed_at timestamp with time zone,
    reversed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    block_number bigint,
    block_hash character varying(66),
    reorg_checked_at timestamp with time zone,
    reorg_suspected_at timestamp with time zone,
    payer_guest_session character varying(64),
    provider_refunded_cents bigint DEFAULT 0 NOT NULL,
    provider_disputed_cents bigint DEFAULT 0 NOT NULL,
    provider_disputed_tip_cents bigint DEFAULT 0 NOT NULL,
    settlement_addr text,
    CONSTRAINT payments_tx_hash_evm_canonical_chk CHECK (((tx_hash !~ '^(0[xX])?[0-9a-fA-F]{64}$'::text) OR (tx_hash ~ '^0x[0-9a-f]{64}$'::text)))
);


--
-- Name: payment_events; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.payment_events AS
 SELECT p.id,
    'payments'::text AS source_table,
    p.bill_id,
    b.business_id,
    COALESCE(p.payment_method, ''::text) AS method,
    COALESCE(p.status, ''::text) AS status,
    COALESCE(p.amount, (0)::bigint) AS amount_cents,
    COALESCE(p.tip_amount, (0)::bigint) AS tip_cents,
    ''::text AS currency,
    COALESCE(p.payer_addr, ''::text) AS payer_ref,
    COALESCE(p.tx_hash, ''::text) AS external_ref,
    p.created_at,
    p.updated_at
   FROM (public.payments p
     JOIN public.bills b ON ((b.id = p.bill_id)))
UNION ALL
 SELECT ap.id,
    'alternative_payments'::text AS source_table,
    ap.bill_id,
    b.business_id,
    COALESCE(ap.payment_method, ''::text) AS method,
    COALESCE(ap.status, ''::text) AS status,
    COALESCE(ap.amount, (0)::bigint) AS amount_cents,
    COALESCE(ap.tip_amount_cents, (0)::bigint) AS tip_cents,
    ''::text AS currency,
    COALESCE(NULLIF(ap.participant_name, ''::text), ap.participant_addr, ''::text) AS payer_ref,
    COALESCE(ap.idempotency_key, ''::text) AS external_ref,
    ap.created_at,
    ap.updated_at
   FROM (public.alternative_payments ap
     JOIN public.bills b ON ((b.id = ap.bill_id)));


--
-- Name: payment_refund_destinations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_refund_destinations (
    id bigint NOT NULL,
    payment_id bigint NOT NULL,
    chain_id bigint NOT NULL,
    token character varying(32) DEFAULT 'USDC'::character varying NOT NULL,
    amount_base_units bigint NOT NULL,
    refund_address character varying(64) NOT NULL,
    evidence_type character varying(32) NOT NULL,
    signature_ref text,
    log_ref character varying(128),
    verified_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone
);


--
-- Name: payment_refund_destinations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.payment_refund_destinations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: payment_refund_destinations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.payment_refund_destinations_id_seq OWNED BY public.payment_refund_destinations.id;


--
-- Name: payment_refunds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_refunds (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    bill_id bigint NOT NULL,
    payment_id bigint NOT NULL,
    chain_id bigint NOT NULL,
    token character varying(32) DEFAULT 'USDC'::character varying NOT NULL,
    amount_base_units bigint NOT NULL,
    verified_recipient character varying(64) NOT NULL,
    recipient_override boolean DEFAULT false NOT NULL,
    override_reason text,
    reason text NOT NULL,
    requested_by character varying(255) NOT NULL,
    approved_by character varying(255),
    rejected_by character varying(255),
    status character varying(32) DEFAULT 'requested'::character varying NOT NULL,
    idempotency_key character varying(128) NOT NULL,
    submitted_tx_hash character varying(128),
    confirmations bigint DEFAULT 0 NOT NULL,
    last_error text,
    locked_at timestamp with time zone,
    locked_by character varying(128),
    ledger_applied_at timestamp with time zone,
    requested_at timestamp with time zone,
    approved_at timestamp with time zone,
    submitted_at timestamp with time zone,
    confirming_at timestamp with time zone,
    confirmed_at timestamp with time zone,
    failed_at timestamp with time zone,
    rejected_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    block_number bigint,
    block_hash character varying(66),
    reorg_checked_at timestamp with time zone,
    reorg_suspected_at timestamp with time zone,
    CONSTRAINT payment_refunds_submitted_tx_hash_evm_canonical_chk CHECK ((((submitted_tx_hash)::text !~ '^(0[xX])?[0-9a-fA-F]{64}$'::text) OR ((submitted_tx_hash)::text ~ '^0x[0-9a-f]{64}$'::text)))
);


--
-- Name: payment_refunds_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.payment_refunds_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: payment_refunds_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.payment_refunds_id_seq OWNED BY public.payment_refunds.id;


--
-- Name: payments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.payments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: payments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.payments_id_seq OWNED BY public.payments.id;


--
-- Name: payroll_line_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payroll_line_items (
    id bigint NOT NULL,
    payroll_run_id bigint NOT NULL,
    business_id bigint NOT NULL,
    payee_type text NOT NULL,
    staff_id bigint,
    payee_name text NOT NULL,
    gross_amount bigint DEFAULT 0,
    bonus_amount bigint DEFAULT 0,
    deduction_amount bigint DEFAULT 0,
    net_amount bigint DEFAULT 0,
    notes text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: payroll_line_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.payroll_line_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: payroll_line_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.payroll_line_items_id_seq OWNED BY public.payroll_line_items.id;


--
-- Name: payroll_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payroll_runs (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    period_start timestamp with time zone NOT NULL,
    period_end timestamp with time zone NOT NULL,
    status text DEFAULT 'draft'::text,
    currency character varying(8),
    paid_at timestamp with time zone,
    paid_by_user_id bigint,
    paid_by_staff_id bigint,
    voided_at timestamp with time zone,
    voided_by_user_id bigint,
    voided_by_staff_id bigint,
    notes text,
    gross_total bigint DEFAULT 0,
    bonus_total bigint DEFAULT 0,
    deduction_total bigint DEFAULT 0,
    net_total bigint DEFAULT 0,
    created_by_user_id bigint,
    created_by_staff_id bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: payroll_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.payroll_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: payroll_runs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.payroll_runs_id_seq OWNED BY public.payroll_runs.id;


--
-- Name: platform_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_settings (
    id bigint NOT NULL,
    key text NOT NULL,
    value text,
    category text,
    is_secret boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: platform_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.platform_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: platform_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.platform_settings_id_seq OWNED BY public.platform_settings.id;


--
-- Name: plugin_notification_deliveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plugin_notification_deliveries (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    plugin_name text NOT NULL,
    event_type text NOT NULL,
    event_id text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    payload text,
    attempt_count bigint DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone NOT NULL,
    last_attempt_at timestamp with time zone,
    delivered_at timestamp with time zone,
    failed_at timestamp with time zone,
    last_error_code text,
    last_error_message text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: plugin_notification_deliveries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.plugin_notification_deliveries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: plugin_notification_deliveries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.plugin_notification_deliveries_id_seq OWNED BY public.plugin_notification_deliveries.id;


--
-- Name: plugin_notification_delivery_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plugin_notification_delivery_attempts (
    id bigint NOT NULL,
    delivery_id bigint NOT NULL,
    attempt_number bigint NOT NULL,
    status text NOT NULL,
    provider_message_id text,
    error_code text,
    error_message text,
    attempted_at timestamp with time zone NOT NULL
);


--
-- Name: plugin_notification_delivery_attempts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.plugin_notification_delivery_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: plugin_notification_delivery_attempts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.plugin_notification_delivery_attempts_id_seq OWNED BY public.plugin_notification_delivery_attempts.id;


--
-- Name: plugin_translations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plugin_translations (
    id bigint NOT NULL,
    plugin_id bigint NOT NULL,
    language_code character varying(16) NOT NULL,
    field_name character varying(50) NOT NULL,
    content text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: plugin_translations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.plugin_translations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: plugin_translations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.plugin_translations_id_seq OWNED BY public.plugin_translations.id;


--
-- Name: plugins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plugins (
    id bigint NOT NULL,
    name text NOT NULL,
    display_name text NOT NULL,
    description text,
    message text,
    image text,
    is_active boolean DEFAULT true,
    admin_active_override boolean DEFAULT false,
    coming_soon boolean DEFAULT false,
    category text NOT NULL,
    version text DEFAULT '1.0.0'::text,
    features text,
    config_schema text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: plugins_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.plugins_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: plugins_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.plugins_id_seq OWNED BY public.plugins.id;


--
-- Name: poll_options; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.poll_options (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    poll_id bigint NOT NULL,
    label character varying(255) NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone
);


--
-- Name: poll_options_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.poll_options_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: poll_options_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.poll_options_id_seq OWNED BY public.poll_options.id;


--
-- Name: poll_votes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.poll_votes (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    poll_id bigint NOT NULL,
    option_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    created_at timestamp with time zone
);


--
-- Name: poll_votes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.poll_votes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: poll_votes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.poll_votes_id_seq OWNED BY public.poll_votes.id;


--
-- Name: polls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.polls (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    author_staff_id bigint NOT NULL,
    question character varying(500) NOT NULL,
    is_anonymous boolean DEFAULT false NOT NULL,
    audience_filter character varying(64) DEFAULT 'all'::character varying NOT NULL,
    status character varying(16) DEFAULT 'open'::character varying NOT NULL,
    closes_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: polls_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.polls_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: polls_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.polls_id_seq OWNED BY public.polls.id;


--
-- Name: positions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.positions (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    color_hex character varying(9) DEFAULT ''::character varying NOT NULL,
    department character varying(16) DEFAULT ''::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: positions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.positions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: positions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.positions_id_seq OWNED BY public.positions.id;


--
-- Name: print_audit_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.print_audit_log (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    actor text NOT NULL,
    action text NOT NULL,
    print_job_id bigint,
    printer_id bigint,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: print_audit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.print_audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: print_audit_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.print_audit_log_id_seq OWNED BY public.print_audit_log.id;


--
-- Name: print_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.print_jobs (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    location_id bigint,
    printer_id bigint,
    kind text NOT NULL,
    source_type text NOT NULL,
    source_id bigint NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    payload_html text,
    payload_escpos bytea,
    retries integer DEFAULT 0 NOT NULL,
    last_error text,
    language text DEFAULT 'en'::text NOT NULL,
    created_by text DEFAULT 'system'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    printed_at timestamp with time zone,
    next_attempt_at timestamp with time zone,
    attempt_count integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 6 NOT NULL,
    order_id bigint,
    kitchen_acked_at timestamp with time zone,
    claimed_by character varying(64),
    lease_expires_at timestamp with time zone,
    presented_at timestamp with time zone
);


--
-- Name: print_jobs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.print_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: print_jobs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.print_jobs_id_seq OWNED BY public.print_jobs.id;


--
-- Name: printers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.printers (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    location_id bigint,
    name text NOT NULL,
    role text NOT NULL,
    transport text NOT NULL,
    paper_width_mm integer DEFAULT 80 NOT NULL,
    code_page text DEFAULT 'CP858'::text NOT NULL,
    cloudprnt_token text,
    cloudprnt_last_seen_at timestamp with time zone,
    enabled boolean DEFAULT true NOT NULL,
    fallback_printer_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: printers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.printers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: printers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.printers_id_seq OWNED BY public.printers.id;


--
-- Name: push_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.push_subscriptions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    business_id bigint NOT NULL,
    principal_type character varying(16) DEFAULT 'owner_user'::character varying NOT NULL,
    principal_id bigint NOT NULL,
    endpoint text NOT NULL,
    p256dh_key text NOT NULL,
    auth_key text NOT NULL,
    user_agent text,
    created_at timestamp with time zone,
    last_used_at timestamp with time zone
);


--
-- Name: push_subscriptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.push_subscriptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: push_subscriptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.push_subscriptions_id_seq OWNED BY public.push_subscriptions.id;


--
-- Name: rbac_audit_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rbac_audit_logs (
    id bigint NOT NULL,
    staff_id bigint NOT NULL,
    business_id bigint NOT NULL,
    action text NOT NULL,
    old_role text,
    new_role text,
    old_permissions text,
    new_permissions text,
    changed_by text NOT NULL,
    reason text,
    ip_address text,
    user_agent text,
    created_at timestamp with time zone
);


--
-- Name: rbac_audit_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.rbac_audit_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: rbac_audit_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.rbac_audit_logs_id_seq OWNED BY public.rbac_audit_logs.id;


--
-- Name: recurring_entry_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.recurring_entry_templates (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    entry_type character varying(16) NOT NULL,
    category character varying(64) NOT NULL,
    amount_cents bigint NOT NULL,
    currency character varying(8) DEFAULT 'USD'::character varying NOT NULL,
    description text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    reference character varying(160) DEFAULT ''::character varying NOT NULL,
    cadence character varying(16) NOT NULL,
    anchor_day integer NOT NULL,
    next_run_on date NOT NULL,
    active boolean DEFAULT true NOT NULL,
    needs_attention boolean DEFAULT false NOT NULL,
    created_by_user_id bigint,
    created_by_staff_id bigint,
    last_generated_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: recurring_entry_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.recurring_entry_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: recurring_entry_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.recurring_entry_templates_id_seq OWNED BY public.recurring_entry_templates.id;


--
-- Name: report_deliveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.report_deliveries (
    id bigint NOT NULL,
    schedule_id bigint NOT NULL,
    business_id bigint NOT NULL,
    scheduled_for timestamp with time zone NOT NULL,
    window_start timestamp with time zone NOT NULL,
    window_end timestamp with time zone NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    lease_token text DEFAULT ''::text NOT NULL,
    lease_expires_at timestamp with time zone,
    attempt_count integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    sent_at timestamp with time zone,
    idempotency_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT report_deliveries_attempt_count_check CHECK ((attempt_count >= 0)),
    CONSTRAINT report_deliveries_check CHECK ((window_end > window_start)),
    CONSTRAINT report_deliveries_check1 CHECK ((((state = 'sent'::text) AND (sent_at IS NOT NULL)) OR ((state <> 'sent'::text) AND (sent_at IS NULL)))),
    CONSTRAINT report_deliveries_check2 CHECK ((((state = 'leased'::text) AND (lease_token <> ''::text) AND (lease_expires_at IS NOT NULL)) OR ((state <> 'leased'::text) AND (lease_token = ''::text) AND (lease_expires_at IS NULL)))),
    CONSTRAINT report_deliveries_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'leased'::text, 'retry_wait'::text, 'sent'::text, 'dead_letter'::text])))
);


--
-- Name: report_deliveries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.report_deliveries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: report_deliveries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.report_deliveries_id_seq OWNED BY public.report_deliveries.id;


--
-- Name: report_schedules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.report_schedules (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    frequency text NOT NULL,
    day_of_week bigint,
    hour bigint NOT NULL,
    minute bigint DEFAULT 0,
    timezone text DEFAULT 'UTC'::text,
    is_active boolean DEFAULT true,
    last_sent_at timestamp with time zone,
    next_send_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: report_schedules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.report_schedules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: report_schedules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.report_schedules_id_seq OWNED BY public.report_schedules.id;


--
-- Name: reservation_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reservation_settings (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    enabled boolean DEFAULT false,
    max_advance_days bigint DEFAULT 30,
    min_advance_minutes bigint DEFAULT 30,
    min_party_size bigint DEFAULT 1,
    max_party_size bigint DEFAULT 20,
    default_duration bigint DEFAULT 120,
    slot_interval_minutes bigint DEFAULT 30,
    service_buffer_minutes bigint DEFAULT 15,
    max_covers_per_slot bigint DEFAULT 0,
    auto_assign_tables boolean DEFAULT true,
    approval_mode character varying(16) DEFAULT 'auto'::character varying,
    allow_waitlist boolean DEFAULT true,
    hold_duration_minutes bigint DEFAULT 15,
    allow_cancellation boolean DEFAULT true,
    cancellation_deadline bigint DEFAULT 24,
    no_show_grace_minutes bigint DEFAULT 15,
    send_confirmation_email boolean DEFAULT true,
    send_reminder_email boolean DEFAULT true,
    reminder_hours_before bigint DEFAULT 24,
    external_partner_links jsonb DEFAULT '[]'::jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: reservation_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.reservation_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reservation_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.reservation_settings_id_seq OWNED BY public.reservation_settings.id;


--
-- Name: reservation_status_histories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reservation_status_histories (
    id bigint NOT NULL,
    reservation_id bigint NOT NULL,
    status text NOT NULL,
    table_id bigint,
    notes text,
    changed_by text,
    created_at timestamp with time zone
);


--
-- Name: reservation_status_histories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.reservation_status_histories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reservation_status_histories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.reservation_status_histories_id_seq OWNED BY public.reservation_status_histories.id;


--
-- Name: restaurant_spaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.restaurant_spaces (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    name text NOT NULL,
    space_type text DEFAULT 'indoor'::text NOT NULL,
    floor_level integer DEFAULT 0 NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    measurement_unit text DEFAULT 'm'::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    width_mm integer,
    height_mm integer,
    boundary_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    draft_layout_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    published_layout_json jsonb,
    layout_schema_version integer DEFAULT 1 NOT NULL,
    draft_revision bigint DEFAULT 1 NOT NULL,
    published_revision bigint DEFAULT 0 NOT NULL,
    has_unpublished_changes boolean DEFAULT false NOT NULL,
    scan_source text,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT restaurant_spaces_measurement_unit_check CHECK ((measurement_unit = ANY (ARRAY['m'::text, 'ft'::text]))),
    CONSTRAINT restaurant_spaces_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'archived'::text])))
);


--
-- Name: restaurant_spaces_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.restaurant_spaces_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: restaurant_spaces_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.restaurant_spaces_id_seq OWNED BY public.restaurant_spaces.id;


--
-- Name: runtime_control_audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.runtime_control_audit_events (
    id bigint NOT NULL,
    control_key text NOT NULL,
    event_type text NOT NULL,
    old_enabled boolean,
    new_enabled boolean,
    invite_batch_id bigint,
    owner text NOT NULL,
    reason text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    actor text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: runtime_control_audit_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.runtime_control_audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: runtime_control_audit_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.runtime_control_audit_events_id_seq OWNED BY public.runtime_control_audit_events.id;


--
-- Name: runtime_controls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.runtime_controls (
    key text NOT NULL,
    enabled boolean NOT NULL,
    owner text NOT NULL,
    reason text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    updated_by text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT runtime_controls_key_check CHECK ((key = ANY (ARRAY['maintenance_mode'::text, 'read_only_mode'::text, 'payments_enabled'::text, 'fiscal_enabled'::text, 'ai_enabled'::text, 'uploads_enabled'::text, 'guest_orders_enabled'::text]))),
    CONSTRAINT runtime_controls_owner_check CHECK ((btrim(owner) <> ''::text)),
    CONSTRAINT runtime_controls_reason_check CHECK ((btrim(reason) <> ''::text)),
    CONSTRAINT runtime_controls_updated_by_check CHECK ((btrim(updated_by) <> ''::text))
);


--
-- Name: runtime_invite_batches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.runtime_invite_batches (
    id bigint NOT NULL,
    name text NOT NULL,
    code_digest character(64) NOT NULL,
    cohort_cap integer NOT NULL,
    claimed_count integer DEFAULT 0 NOT NULL,
    active boolean DEFAULT true NOT NULL,
    owner text NOT NULL,
    reason text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_by text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT runtime_invite_batches_check CHECK (((claimed_count >= 0) AND (claimed_count <= cohort_cap))),
    CONSTRAINT runtime_invite_batches_cohort_cap_check CHECK ((cohort_cap > 0)),
    CONSTRAINT runtime_invite_batches_created_by_check CHECK ((btrim(created_by) <> ''::text)),
    CONSTRAINT runtime_invite_batches_name_check CHECK ((btrim(name) <> ''::text)),
    CONSTRAINT runtime_invite_batches_owner_check CHECK ((btrim(owner) <> ''::text)),
    CONSTRAINT runtime_invite_batches_reason_check CHECK ((btrim(reason) <> ''::text))
);


--
-- Name: runtime_invite_batches_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.runtime_invite_batches_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: runtime_invite_batches_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.runtime_invite_batches_id_seq OWNED BY public.runtime_invite_batches.id;


--
-- Name: runtime_invite_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.runtime_invite_claims (
    id bigint NOT NULL,
    invite_batch_id bigint NOT NULL,
    normalized_email text NOT NULL,
    claimed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT runtime_invite_claims_normalized_email_check CHECK ((normalized_email = lower(btrim(normalized_email))))
);


--
-- Name: runtime_invite_claims_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.runtime_invite_claims_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: runtime_invite_claims_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.runtime_invite_claims_id_seq OWNED BY public.runtime_invite_claims.id;


--
-- Name: scheduler_states; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scheduler_states (
    id bigint NOT NULL,
    key text NOT NULL,
    created_at timestamp with time zone
);


--
-- Name: scheduler_states_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.scheduler_states_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: scheduler_states_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.scheduler_states_id_seq OWNED BY public.scheduler_states.id;


--
-- Name: schedules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schedules (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    week_start timestamp with time zone NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    published_at timestamp with time zone,
    published_by_staff_id bigint,
    notes text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: schedules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.schedules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: schedules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.schedules_id_seq OWNED BY public.schedules.id;


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version bigint NOT NULL,
    dirty boolean NOT NULL
);


--
-- Name: session_summaries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.session_summaries (
    session_id character varying(255) NOT NULL,
    first_seen timestamp without time zone NOT NULL,
    last_seen timestamp without time zone NOT NULL,
    total_page_views integer DEFAULT 0,
    total_interactions integer DEFAULT 0,
    total_duration integer DEFAULT 0,
    pages_visited text,
    converted boolean DEFAULT false,
    conversion_type character varying(100),
    device_type character varying(50),
    country character varying(100)
);


--
-- Name: shift_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shift_notes (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    shift_id bigint,
    for_date timestamp with time zone NOT NULL,
    author_staff_id bigint NOT NULL,
    category character varying(16) NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone
);


--
-- Name: shift_notes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.shift_notes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: shift_notes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.shift_notes_id_seq OWNED BY public.shift_notes.id;


--
-- Name: shift_swap_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shift_swap_requests (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    shift_id bigint NOT NULL,
    requesting_staff_id bigint NOT NULL,
    kind text NOT NULL,
    target text DEFAULT 'all_in_role'::text NOT NULL,
    target_staff_id bigint,
    status text DEFAULT 'open'::text NOT NULL,
    accepting_staff_id bigint,
    approved_by_staff_id bigint,
    created_at timestamp with time zone,
    resolved_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: shift_swap_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.shift_swap_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: shift_swap_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.shift_swap_requests_id_seq OWNED BY public.shift_swap_requests.id;


--
-- Name: shifts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shifts (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    schedule_id bigint NOT NULL,
    staff_id bigint,
    position_id bigint NOT NULL,
    starts_at timestamp with time zone NOT NULL,
    ends_at timestamp with time zone NOT NULL,
    break_minutes bigint DEFAULT 0 NOT NULL,
    status text DEFAULT 'private_draft'::text NOT NULL,
    published boolean DEFAULT false NOT NULL,
    notes text,
    created_by_staff_id bigint NOT NULL,
    reminded_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: shifts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.shifts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: shifts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.shifts_id_seq OWNED BY public.shifts.id;


--
-- Name: shoutouts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shoutouts (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    from_staff_id bigint NOT NULL,
    to_staff_id bigint NOT NULL,
    message character varying(500) NOT NULL,
    emoji character varying(16) DEFAULT ''::character varying NOT NULL,
    visibility character varying(16) DEFAULT 'team'::character varying NOT NULL,
    created_at timestamp with time zone
);


--
-- Name: shoutouts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.shoutouts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: shoutouts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.shoutouts_id_seq OWNED BY public.shoutouts.id;


--
-- Name: space_layout_audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_layout_audit_events (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    space_id bigint,
    actor_user_id bigint,
    actor_staff_id bigint,
    action text NOT NULL,
    detail_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: space_layout_audit_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.space_layout_audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: space_layout_audit_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.space_layout_audit_events_id_seq OWNED BY public.space_layout_audit_events.id;


--
-- Name: space_layout_elements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_layout_elements (
    id bigint NOT NULL,
    space_id bigint NOT NULL,
    business_id bigint NOT NULL,
    element_type text NOT NULL,
    name text,
    geometry_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    x_mm integer,
    y_mm integer,
    width_mm integer,
    height_mm integer,
    rotation_deg double precision DEFAULT 0 NOT NULL,
    z_index integer DEFAULT 0 NOT NULL,
    is_draft boolean DEFAULT true NOT NULL,
    meta_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT space_layout_elements_type_check CHECK ((element_type = ANY (ARRAY['wall'::text, 'door'::text, 'window'::text, 'column'::text, 'bar'::text, 'counter'::text, 'entrance'::text, 'stairs'::text, 'service_station'::text, 'restroom'::text, 'divider'::text, 'obstacle'::text, 'label'::text])))
);


--
-- Name: space_layout_elements_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.space_layout_elements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: space_layout_elements_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.space_layout_elements_id_seq OWNED BY public.space_layout_elements.id;


--
-- Name: space_regions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_regions (
    id bigint NOT NULL,
    space_id bigint NOT NULL,
    business_id bigint NOT NULL,
    name text NOT NULL,
    color character varying(7),
    description text,
    purpose text,
    polygon_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: space_regions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.space_regions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: space_regions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.space_regions_id_seq OWNED BY public.space_regions.id;


--
-- Name: space_scan_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_scan_sessions (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    space_id bigint,
    created_by_user_id bigint,
    created_by_staff_id bigint,
    token_hash text NOT NULL,
    token_prefix text NOT NULL,
    status text DEFAULT 'waiting_for_phone'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    connected_at timestamp with time zone,
    completed_at timestamp with time zone,
    progress_pct integer DEFAULT 0 NOT NULL,
    progress_message text,
    error_code text,
    error_message text,
    device_meta_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    calibration_mm integer,
    idempotency_key text,
    result_layout_json jsonb,
    retain_raw_until timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT space_scan_sessions_status_check CHECK ((status = ANY (ARRAY['waiting_for_phone'::text, 'phone_connected'::text, 'scanning'::text, 'uploading'::text, 'processing'::text, 'review_ready'::text, 'failed'::text, 'expired'::text, 'cancelled'::text, 'completed'::text])))
);


--
-- Name: space_scan_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.space_scan_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: space_scan_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.space_scan_sessions_id_seq OWNED BY public.space_scan_sessions.id;


--
-- Name: space_scan_uploads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.space_scan_uploads (
    id bigint NOT NULL,
    session_id bigint NOT NULL,
    business_id bigint NOT NULL,
    upload_kind text NOT NULL,
    content_type text,
    byte_size bigint,
    checksum_sha256 text,
    s3_key text,
    format_version integer DEFAULT 1 NOT NULL,
    part_index integer DEFAULT 0 NOT NULL,
    is_complete boolean DEFAULT false NOT NULL,
    idempotency_key text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT space_scan_uploads_kind_check CHECK ((upload_kind = ANY (ARRAY['roomplan_json'::text, 'keyframes'::text, 'video'::text, 'depth'::text, 'metadata'::text])))
);


--
-- Name: space_scan_uploads_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.space_scan_uploads_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: space_scan_uploads_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.space_scan_uploads_id_seq OWNED BY public.space_scan_uploads.id;


--
-- Name: staff; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    email text NOT NULL,
    name text NOT NULL,
    role text NOT NULL,
    is_active boolean DEFAULT true,
    last_login_at timestamp with time zone,
    invited_by text NOT NULL,
    custom_permissions text,
    role_level bigint DEFAULT 0,
    permissions_updated_at timestamp with time zone,
    permissions_updated_by text,
    authz_version bigint DEFAULT 1 NOT NULL,
    employment_type character varying(16) DEFAULT ''::character varying,
    hourly_rate_cents bigint DEFAULT 0,
    annual_salary_cents bigint DEFAULT 0,
    pin_hash text,
    pin_set_at timestamp with time zone,
    login_code_failed_attempts bigint DEFAULT 0,
    login_code_locked_until timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: staff_availabilities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_availabilities (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    weekday bigint NOT NULL,
    start_min bigint NOT NULL,
    end_min bigint NOT NULL,
    kind text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: staff_availabilities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_availabilities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_availabilities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_availabilities_id_seq OWNED BY public.staff_availabilities.id;


--
-- Name: staff_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_id_seq OWNED BY public.staff.id;


--
-- Name: staff_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_identities (
    id bigint NOT NULL,
    normalized_email character varying(320) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: staff_identities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_identities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_identities_id_seq OWNED BY public.staff_identities.id;


--
-- Name: staff_invitations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_invitations (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    email text NOT NULL,
    name text NOT NULL,
    role text NOT NULL,
    token text NOT NULL,
    status text DEFAULT 'pending'::text,
    invited_by text NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: staff_invitations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_invitations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_invitations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_invitations_id_seq OWNED BY public.staff_invitations.id;


--
-- Name: staff_login_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_login_codes (
    id bigint NOT NULL,
    staff_id bigint NOT NULL,
    code text NOT NULL,
    expires_at timestamp with time zone,
    used boolean DEFAULT false,
    created_at timestamp with time zone
);


--
-- Name: staff_login_codes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_login_codes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_login_codes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_login_codes_id_seq OWNED BY public.staff_login_codes.id;


--
-- Name: staff_membership_selection_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_membership_selection_tokens (
    jti_hash character varying(64) NOT NULL,
    expires_at timestamp with time zone NOT NULL
);


--
-- Name: staff_memberships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_memberships (
    id bigint NOT NULL,
    identity_id bigint NOT NULL,
    business_id bigint NOT NULL,
    legacy_staff_id bigint NOT NULL,
    name text NOT NULL,
    role text NOT NULL,
    custom_permissions text DEFAULT ''::text NOT NULL,
    role_level bigint DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    authz_version bigint DEFAULT 1 NOT NULL,
    invited_by text DEFAULT ''::text NOT NULL,
    permissions_updated_at timestamp with time zone,
    permissions_updated_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: staff_memberships_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_memberships_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_memberships_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_memberships_id_seq OWNED BY public.staff_memberships.id;


--
-- Name: staff_notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_notifications (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    kind character varying(32) NOT NULL,
    title character varying(255) NOT NULL,
    body character varying(500) DEFAULT ''::character varying NOT NULL,
    url character varying(512) DEFAULT ''::character varying NOT NULL,
    read_at timestamp with time zone,
    created_at timestamp with time zone
);


--
-- Name: staff_notifications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_notifications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_notifications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_notifications_id_seq OWNED BY public.staff_notifications.id;


--
-- Name: staff_permission_denies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_permission_denies (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    permission character varying(128) NOT NULL,
    created_by character varying(255) DEFAULT ''::character varying NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE staff_permission_denies; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.staff_permission_denies IS 'Explicit permission deny overrides for staff; subtracted from role∪grant effective set';


--
-- Name: staff_permission_denies_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_permission_denies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_permission_denies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_permission_denies_id_seq OWNED BY public.staff_permission_denies.id;


--
-- Name: staff_positions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.staff_positions (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    position_id bigint NOT NULL,
    pay_rate_cents bigint DEFAULT 0 NOT NULL,
    is_primary boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: staff_positions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.staff_positions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: staff_positions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.staff_positions_id_seq OWNED BY public.staff_positions.id;


--
-- Name: supported_currencies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.supported_currencies (
    id bigint NOT NULL,
    code character varying(3) NOT NULL,
    name character varying(100) NOT NULL,
    symbol character varying(10) NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: supported_currencies_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.supported_currencies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: supported_currencies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.supported_currencies_id_seq OWNED BY public.supported_currencies.id;


--
-- Name: supported_languages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.supported_languages (
    id bigint NOT NULL,
    code character varying(16) NOT NULL,
    name character varying(100) NOT NULL,
    native_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: supported_languages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.supported_languages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: supported_languages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.supported_languages_id_seq OWNED BY public.supported_languages.id;


--
-- Name: table_combination_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.table_combination_members (
    id bigint NOT NULL,
    combination_id bigint NOT NULL,
    business_id bigint NOT NULL,
    table_id bigint NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: table_combination_members_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.table_combination_members_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: table_combination_members_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.table_combination_members_id_seq OWNED BY public.table_combination_members.id;


--
-- Name: table_combinations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.table_combinations (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    name text NOT NULL,
    primary_table_id bigint NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: table_combinations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.table_combinations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: table_combinations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.table_combinations_id_seq OWNED BY public.table_combinations.id;


--
-- Name: table_reservations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.table_reservations (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    table_id bigint,
    customer_name text NOT NULL,
    customer_phone text,
    customer_email text,
    party_size bigint NOT NULL,
    reservation_time timestamp with time zone NOT NULL,
    duration bigint DEFAULT 120,
    status text DEFAULT 'pending'::text,
    source text DEFAULT 'customer'::text,
    confirmation_code character varying(32),
    language character varying(16) DEFAULT 'en'::character varying NOT NULL,
    special_requests text,
    notes text,
    reminder_sent boolean DEFAULT false,
    approval_reminder_sent_at timestamp with time zone,
    created_by text,
    confirmed_at timestamp with time zone,
    assigned_at timestamp with time zone,
    seated_at timestamp with time zone,
    completed_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    cancelled_by text,
    cancellation_reason text,
    waitlist_position bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    claimed_by_staff_id bigint,
    claimed_by_name text DEFAULT ''::text NOT NULL,
    claimed_by_role text DEFAULT ''::text NOT NULL,
    claimed_at timestamp with time zone
);


--
-- Name: table_reservations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.table_reservations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: table_reservations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.table_reservations_id_seq OWNED BY public.table_reservations.id;


--
-- Name: tables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tables (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    table_code text NOT NULL,
    name text NOT NULL,
    capacity bigint DEFAULT 4,
    qr_code text,
    is_active boolean DEFAULT true,
    qr_logo_url text,
    qr_foreground_color text DEFAULT '#000000'::text,
    qr_background_color text DEFAULT '#FFFFFF'::text,
    qr_logo_size bigint DEFAULT 20,
    qr_show_business_name boolean DEFAULT false,
    qr_show_table_name boolean DEFAULT false,
    qr_text_font text DEFAULT 'Verdana'::text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    space_id bigint,
    region_id bigint,
    pos_x_mm integer,
    pos_y_mm integer,
    width_mm integer,
    height_mm integer,
    rotation_deg double precision DEFAULT 0,
    shape text DEFAULT 'rectangle'::text,
    min_capacity integer,
    max_capacity integer,
    visible_seat_count integer,
    is_reservable boolean DEFAULT true,
    is_combinable boolean DEFAULT false,
    is_accessible boolean DEFAULT false,
    layout_in_draft boolean DEFAULT false,
    layout_published boolean DEFAULT false,
    CONSTRAINT tables_shape_check CHECK (((shape IS NULL) OR (shape = ANY (ARRAY['round'::text, 'square'::text, 'rectangle'::text, 'oval'::text, 'bar'::text, 'custom'::text]))))
);


--
-- Name: tables_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tables_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tables_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tables_id_seq OWNED BY public.tables.id;


--
-- Name: telegram_connection_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telegram_connection_tokens (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    used_by_chat_id bigint,
    used_by_username text,
    created_by_user_id bigint,
    created_by_staff_id bigint,
    created_ip text,
    created_at timestamp with time zone
);


--
-- Name: telegram_connection_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.telegram_connection_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: telegram_connection_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.telegram_connection_tokens_id_seq OWNED BY public.telegram_connection_tokens.id;


--
-- Name: telegram_update_receipts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telegram_update_receipts (
    update_id bigint NOT NULL,
    business_id bigint,
    chat_id bigint,
    status text NOT NULL,
    error_code text,
    processed_at timestamp with time zone NOT NULL
);


--
-- Name: telegram_update_receipts_update_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.telegram_update_receipts_update_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: telegram_update_receipts_update_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.telegram_update_receipts_update_id_seq OWNED BY public.telegram_update_receipts.update_id;


--
-- Name: time_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.time_entries (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    shift_id bigint,
    clock_in_at timestamp with time zone NOT NULL,
    clock_out_at timestamp with time zone,
    break_minutes bigint DEFAULT 0 NOT NULL,
    source text DEFAULT 'staff_punch'::text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    approved_by_staff_id bigint,
    note text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: time_entries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.time_entries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: time_entries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.time_entries_id_seq OWNED BY public.time_entries.id;


--
-- Name: time_off_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.time_off_requests (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    starts_at timestamp with time zone NOT NULL,
    ends_at timestamp with time zone NOT NULL,
    reason text,
    status text DEFAULT 'pending'::text NOT NULL,
    decided_by_staff_id bigint,
    decided_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: time_off_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.time_off_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: time_off_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.time_off_requests_id_seq OWNED BY public.time_off_requests.id;


--
-- Name: translations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.translations (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    entity_type character varying(50) NOT NULL,
    entity_id bigint NOT NULL,
    field_name character varying(50) NOT NULL,
    language_code character varying(16) NOT NULL,
    original_text text,
    translated_text text NOT NULL,
    is_auto_translated boolean DEFAULT true,
    translation_source character varying(50) DEFAULT 'google'::character varying,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: translations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.translations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: translations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.translations_id_seq OWNED BY public.translations.id;


--
-- Name: user_auths; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_auths (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    provider text NOT NULL,
    password_hash text,
    provider_user_id text,
    wallet_address text,
    email_verified boolean DEFAULT false,
    verification_token text,
    verification_expiry timestamp with time zone,
    reset_token text,
    reset_expiry timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: user_auths_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_auths_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_auths_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_auths_id_seq OWNED BY public.user_auths.id;


--
-- Name: user_interactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_interactions (
    id bigint NOT NULL,
    session_id character varying(255) NOT NULL,
    page character varying(500) NOT NULL,
    event_type character varying(100) NOT NULL,
    event_category character varying(100),
    event_label character varying(255),
    event_value text,
    x_position integer,
    y_position integer,
    "timestamp" timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: user_interactions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_interactions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_interactions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_interactions_id_seq OWNED BY public.user_interactions.id;


--
-- Name: user_session_refresh_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_session_refresh_history (
    session_id bigint NOT NULL,
    token_hash text NOT NULL,
    rotated_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL
);


--
-- Name: user_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_sessions (
    id bigint NOT NULL,
    user_id bigint,
    address text,
    session_token text NOT NULL,
    provider text,
    ip_address text,
    user_agent text,
    revoked boolean DEFAULT false,
    expires_at timestamp with time zone,
    created_at timestamp with time zone,
    last_used_at timestamp with time zone,
    refresh_token text,
    refresh_expires_at timestamp with time zone,
    previous_refresh_token text,
    previous_rotated_at timestamp with time zone,
    revoked_at timestamp with time zone,
    revocation_reason text
);


--
-- Name: user_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_sessions_id_seq OWNED BY public.user_sessions.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    email text,
    address text,
    name text,
    username text,
    picture text,
    role text DEFAULT 'user'::text,
    auth_method text,
    google_id text,
    email_verified boolean DEFAULT false,
    email_enabled boolean,
    news_enabled boolean,
    updates_enabled boolean,
    transactional_enabled boolean,
    security_enabled boolean,
    reports_enabled boolean,
    statistics_enabled boolean,
    language_selected text,
    deleted_at timestamp with time zone,
    deletion_scheduled_at timestamp with time zone,
    deletion_reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    signup_source text DEFAULT ''::text NOT NULL,
    activated_at timestamp with time zone
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: webhook_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_events (
    id bigint NOT NULL,
    provider text NOT NULL,
    webhook_id text NOT NULL,
    event_type text NOT NULL,
    status text DEFAULT 'processing'::text,
    payload text,
    error text,
    received_at timestamp with time zone NOT NULL,
    processed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: webhook_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.webhook_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: webhook_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.webhook_events_id_seq OWNED BY public.webhook_events.id;


--
-- Name: whatsapp_business_devices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.whatsapp_business_devices (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    device_jid text NOT NULL,
    status character varying(24) DEFAULT 'disconnected'::character varying NOT NULL,
    last_error_code character varying(64),
    last_connected_at timestamp with time zone,
    reconnect_attempts bigint DEFAULT 0 NOT NULL,
    next_retry_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_whatsapp_business_device_status CHECK (((status)::text = ANY (ARRAY[('pairing'::character varying)::text, ('connecting'::character varying)::text, ('connected'::character varying)::text, ('degraded'::character varying)::text, ('disconnected'::character varying)::text])))
);


--
-- Name: TABLE whatsapp_business_devices; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.whatsapp_business_devices IS 'Explicit business↔device mapping for restart-safe WhatsApp AI waiter sessions';


--
-- Name: whatsapp_business_devices_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.whatsapp_business_devices_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: whatsapp_business_devices_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.whatsapp_business_devices_id_seq OWNED BY public.whatsapp_business_devices.id;


--
-- Name: withdrawal_histories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.withdrawal_histories (
    id bigint NOT NULL,
    business_id bigint NOT NULL,
    transaction_hash text NOT NULL,
    payment_amount bigint DEFAULT 0,
    tip_amount bigint DEFAULT 0,
    total_amount bigint NOT NULL,
    withdrawal_address text NOT NULL,
    blockchain_network text NOT NULL,
    status text DEFAULT 'pending'::text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    confirmed_at timestamp with time zone
);


--
-- Name: withdrawal_histories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.withdrawal_histories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: withdrawal_histories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.withdrawal_histories_id_seq OWNED BY public.withdrawal_histories.id;


--
-- Name: accounting_categories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounting_categories ALTER COLUMN id SET DEFAULT nextval('public.accounting_categories_id_seq'::regclass);


--
-- Name: accounting_period_locks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounting_period_locks ALTER COLUMN id SET DEFAULT nextval('public.accounting_period_locks_id_seq'::regclass);


--
-- Name: activation_event_outbox id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activation_event_outbox ALTER COLUMN id SET DEFAULT nextval('public.activation_event_outbox_id_seq'::regclass);


--
-- Name: admin_actions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_actions ALTER COLUMN id SET DEFAULT nextval('public.admin_actions_id_seq'::regclass);


--
-- Name: ai_daily_spend id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_daily_spend ALTER COLUMN id SET DEFAULT nextval('public.ai_daily_spend_id_seq'::regclass);


--
-- Name: ai_generated_images id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_generated_images ALTER COLUMN id SET DEFAULT nextval('public.ai_generated_images_id_seq'::regclass);


--
-- Name: ai_image_usage id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_image_usage ALTER COLUMN id SET DEFAULT nextval('public.ai_image_usage_id_seq'::regclass);


--
-- Name: ai_waiter_conversations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_waiter_conversations ALTER COLUMN id SET DEFAULT nextval('public.ai_waiter_conversations_id_seq'::regclass);


--
-- Name: ai_waiter_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_waiter_messages ALTER COLUMN id SET DEFAULT nextval('public.ai_waiter_messages_id_seq'::regclass);


--
-- Name: alternative_payments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alternative_payments ALTER COLUMN id SET DEFAULT nextval('public.alternative_payments_id_seq'::regclass);


--
-- Name: announcements id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.announcements ALTER COLUMN id SET DEFAULT nextval('public.announcements_id_seq'::regclass);


--
-- Name: auth_attempts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_attempts ALTER COLUMN id SET DEFAULT nextval('public.auth_attempts_id_seq'::regclass);


--
-- Name: bill_history_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_history_events ALTER COLUMN id SET DEFAULT nextval('public.bill_history_events_id_seq'::regclass);


--
-- Name: bill_split_shares id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_split_shares ALTER COLUMN id SET DEFAULT nextval('public.bill_split_shares_id_seq'::regclass);


--
-- Name: bills id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bills ALTER COLUMN id SET DEFAULT nextval('public.bills_id_seq'::regclass);


--
-- Name: bundles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bundles ALTER COLUMN id SET DEFAULT nextval('public.bundles_id_seq'::regclass);


--
-- Name: business_alert_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_alert_settings ALTER COLUMN id SET DEFAULT nextval('public.business_alert_settings_id_seq'::regclass);


--
-- Name: business_creation_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_creation_requests ALTER COLUMN id SET DEFAULT nextval('public.business_creation_requests_id_seq'::regclass);


--
-- Name: business_currencies id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_currencies ALTER COLUMN id SET DEFAULT nextval('public.business_currencies_id_seq'::regclass);


--
-- Name: business_fiscal_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_fiscal_settings ALTER COLUMN id SET DEFAULT nextval('public.business_fiscal_settings_id_seq'::regclass);


--
-- Name: business_gallery_images id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_gallery_images ALTER COLUMN id SET DEFAULT nextval('public.business_gallery_images_id_seq'::regclass);


--
-- Name: business_languages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_languages ALTER COLUMN id SET DEFAULT nextval('public.business_languages_id_seq'::regclass);


--
-- Name: business_milestone_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_milestone_events ALTER COLUMN id SET DEFAULT nextval('public.business_milestone_events_id_seq'::regclass);


--
-- Name: business_operating_exceptions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_operating_exceptions ALTER COLUMN id SET DEFAULT nextval('public.business_operating_exceptions_id_seq'::regclass);


--
-- Name: business_operating_hours id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_operating_hours ALTER COLUMN id SET DEFAULT nextval('public.business_operating_hours_id_seq'::regclass);


--
-- Name: business_plugins id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_plugins ALTER COLUMN id SET DEFAULT nextval('public.business_plugins_id_seq'::regclass);


--
-- Name: business_revenue_aggregates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_revenue_aggregates ALTER COLUMN id SET DEFAULT nextval('public.business_revenue_aggregates_id_seq'::regclass);


--
-- Name: business_schedule_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_schedule_settings ALTER COLUMN id SET DEFAULT nextval('public.business_schedule_settings_id_seq'::regclass);


--
-- Name: business_special_features id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_special_features ALTER COLUMN id SET DEFAULT nextval('public.business_special_features_id_seq'::regclass);


--
-- Name: businesses id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.businesses ALTER COLUMN id SET DEFAULT nextval('public.businesses_id_seq'::regclass);


--
-- Name: cash_register_movements id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_movements ALTER COLUMN id SET DEFAULT nextval('public.cash_register_movements_id_seq'::regclass);


--
-- Name: cash_register_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_sessions ALTER COLUMN id SET DEFAULT nextval('public.cash_register_sessions_id_seq'::regclass);


--
-- Name: chat_channels id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_channels ALTER COLUMN id SET DEFAULT nextval('public.chat_channels_id_seq'::regclass);


--
-- Name: chat_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages ALTER COLUMN id SET DEFAULT nextval('public.chat_messages_id_seq'::regclass);


--
-- Name: checklist_item_completions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_item_completions ALTER COLUMN id SET DEFAULT nextval('public.checklist_item_completions_id_seq'::regclass);


--
-- Name: checklist_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_items ALTER COLUMN id SET DEFAULT nextval('public.checklist_items_id_seq'::regclass);


--
-- Name: checklist_runs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_runs ALTER COLUMN id SET DEFAULT nextval('public.checklist_runs_id_seq'::regclass);


--
-- Name: checklist_templates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_templates ALTER COLUMN id SET DEFAULT nextval('public.checklist_templates_id_seq'::regclass);


--
-- Name: comp_void_audit id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comp_void_audit ALTER COLUMN id SET DEFAULT nextval('public.comp_void_audit_id_seq'::regclass);


--
-- Name: conversion_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversion_events ALTER COLUMN id SET DEFAULT nextval('public.conversion_events_id_seq'::regclass);


--
-- Name: counters id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.counters ALTER COLUMN id SET DEFAULT nextval('public.counters_id_seq'::regclass);


--
-- Name: crypto_payment_quotes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crypto_payment_quotes ALTER COLUMN id SET DEFAULT nextval('public.crypto_payment_quotes_id_seq'::regclass);


--
-- Name: customer_addresses id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_addresses ALTER COLUMN id SET DEFAULT nextval('public.customer_addresses_id_seq'::regclass);


--
-- Name: customer_businesses id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_businesses ALTER COLUMN id SET DEFAULT nextval('public.customer_businesses_id_seq'::regclass);


--
-- Name: customer_communications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_communications ALTER COLUMN id SET DEFAULT nextval('public.customer_communications_id_seq'::regclass);


--
-- Name: customer_preferences id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_preferences ALTER COLUMN id SET DEFAULT nextval('public.customer_preferences_id_seq'::regclass);


--
-- Name: customer_visits id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_visits ALTER COLUMN id SET DEFAULT nextval('public.customer_visits_id_seq'::regclass);


--
-- Name: customers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customers ALTER COLUMN id SET DEFAULT nextval('public.customers_id_seq'::regclass);


--
-- Name: delivery_drivers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_drivers ALTER COLUMN id SET DEFAULT nextval('public.delivery_drivers_id_seq'::regclass);


--
-- Name: delivery_orders id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_orders ALTER COLUMN id SET DEFAULT nextval('public.delivery_orders_id_seq'::regclass);


--
-- Name: delivery_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_settings ALTER COLUMN id SET DEFAULT nextval('public.delivery_settings_id_seq'::regclass);


--
-- Name: delivery_status_history id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_status_history ALTER COLUMN id SET DEFAULT nextval('public.delivery_status_history_id_seq'::regclass);


--
-- Name: delivery_zones id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_zones ALTER COLUMN id SET DEFAULT nextval('public.delivery_zones_id_seq'::regclass);


--
-- Name: demo_instances id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.demo_instances ALTER COLUMN id SET DEFAULT nextval('public.demo_instances_id_seq'::regclass);


--
-- Name: demo_runs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.demo_runs ALTER COLUMN id SET DEFAULT nextval('public.demo_runs_id_seq'::regclass);


--
-- Name: director_action_audits id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_action_audits ALTER COLUMN id SET DEFAULT nextval('public.director_action_audits_id_seq'::regclass);


--
-- Name: director_console_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_console_messages ALTER COLUMN id SET DEFAULT nextval('public.director_console_messages_id_seq'::regclass);


--
-- Name: director_console_threads id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_console_threads ALTER COLUMN id SET DEFAULT nextval('public.director_console_threads_id_seq'::regclass);


--
-- Name: director_proposed_actions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_proposed_actions ALTER COLUMN id SET DEFAULT nextval('public.director_proposed_actions_id_seq'::regclass);


--
-- Name: director_tool_calls id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_tool_calls ALTER COLUMN id SET DEFAULT nextval('public.director_tool_calls_id_seq'::regclass);


--
-- Name: document_acks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_acks ALTER COLUMN id SET DEFAULT nextval('public.document_acks_id_seq'::regclass);


--
-- Name: documents id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents ALTER COLUMN id SET DEFAULT nextval('public.documents_id_seq'::regclass);


--
-- Name: email_delivery_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_delivery_events ALTER COLUMN id SET DEFAULT nextval('public.email_delivery_events_id_seq'::regclass);


--
-- Name: email_outbound_sends id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_outbound_sends ALTER COLUMN id SET DEFAULT nextval('public.email_outbound_sends_id_seq'::regclass);


--
-- Name: email_outbox id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_outbox ALTER COLUMN id SET DEFAULT nextval('public.email_outbox_id_seq'::regclass);


--
-- Name: error_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.error_logs ALTER COLUMN id SET DEFAULT nextval('public.error_logs_id_seq'::regclass);


--
-- Name: escalations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.escalations ALTER COLUMN id SET DEFAULT nextval('public.escalations_id_seq'::regclass);


--
-- Name: exchange_rates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exchange_rates ALTER COLUMN id SET DEFAULT nextval('public.exchange_rates_id_seq'::regclass);


--
-- Name: fiscal_audit_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_audit_events ALTER COLUMN id SET DEFAULT nextval('public.fiscal_audit_events_id_seq'::regclass);


--
-- Name: fiscal_delivery_tasks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_delivery_tasks ALTER COLUMN id SET DEFAULT nextval('public.fiscal_delivery_tasks_id_seq'::regclass);


--
-- Name: fiscal_jobs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_jobs ALTER COLUMN id SET DEFAULT nextval('public.fiscal_jobs_id_seq'::regclass);


--
-- Name: fiscal_receipts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_receipts ALTER COLUMN id SET DEFAULT nextval('public.fiscal_receipts_id_seq'::regclass);


--
-- Name: guest_receipt_sends id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_receipt_sends ALTER COLUMN id SET DEFAULT nextval('public.guest_receipt_sends_id_seq'::regclass);


--
-- Name: idempotency_keys id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.idempotency_keys ALTER COLUMN id SET DEFAULT nextval('public.idempotency_keys_id_seq'::regclass);


--
-- Name: inventory_alert_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_alert_logs ALTER COLUMN id SET DEFAULT nextval('public.inventory_alert_logs_id_seq'::regclass);


--
-- Name: inventory_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_items ALTER COLUMN id SET DEFAULT nextval('public.inventory_items_id_seq'::regclass);


--
-- Name: inventory_movements id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_movements ALTER COLUMN id SET DEFAULT nextval('public.inventory_movements_id_seq'::regclass);


--
-- Name: inventory_recipes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_recipes ALTER COLUMN id SET DEFAULT nextval('public.inventory_recipes_id_seq'::regclass);


--
-- Name: inventory_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_settings ALTER COLUMN id SET DEFAULT nextval('public.inventory_settings_id_seq'::regclass);


--
-- Name: ledger_entry_attachments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ledger_entry_attachments ALTER COLUMN id SET DEFAULT nextval('public.ledger_entry_attachments_id_seq'::regclass);


--
-- Name: loyalty_programs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loyalty_programs ALTER COLUMN id SET DEFAULT nextval('public.loyalty_programs_id_seq'::regclass);


--
-- Name: loyalty_tiers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loyalty_tiers ALTER COLUMN id SET DEFAULT nextval('public.loyalty_tiers_id_seq'::regclass);


--
-- Name: manual_ledger_entries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manual_ledger_entries ALTER COLUMN id SET DEFAULT nextval('public.manual_ledger_entries_id_seq'::regclass);


--
-- Name: marketing_activities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.marketing_activities ALTER COLUMN id SET DEFAULT nextval('public.marketing_activities_id_seq'::regclass);


--
-- Name: menu_extraction_images id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_extraction_images ALTER COLUMN id SET DEFAULT nextval('public.menu_extraction_images_id_seq'::regclass);


--
-- Name: menu_extraction_jobs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_extraction_jobs ALTER COLUMN id SET DEFAULT nextval('public.menu_extraction_jobs_id_seq'::regclass);


--
-- Name: menu_wizard_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_wizard_messages ALTER COLUMN id SET DEFAULT nextval('public.menu_wizard_messages_id_seq'::regclass);


--
-- Name: menu_wizard_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_wizard_sessions ALTER COLUMN id SET DEFAULT nextval('public.menu_wizard_sessions_id_seq'::regclass);


--
-- Name: menus id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus ALTER COLUMN id SET DEFAULT nextval('public.menus_id_seq'::regclass);


--
-- Name: missing_translations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.missing_translations ALTER COLUMN id SET DEFAULT nextval('public.missing_translations_id_seq'::regclass);


--
-- Name: offers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers ALTER COLUMN id SET DEFAULT nextval('public.offers_id_seq'::regclass);


--
-- Name: open_shift_claims id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.open_shift_claims ALTER COLUMN id SET DEFAULT nextval('public.open_shift_claims_id_seq'::regclass);


--
-- Name: operational_alert_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alert_events ALTER COLUMN id SET DEFAULT nextval('public.operational_alert_events_id_seq'::regclass);


--
-- Name: operational_alerts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alerts ALTER COLUMN id SET DEFAULT nextval('public.operational_alerts_id_seq'::regclass);


--
-- Name: ops_assistant_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_messages ALTER COLUMN id SET DEFAULT nextval('public.ops_assistant_messages_id_seq'::regclass);


--
-- Name: ops_assistant_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_requests ALTER COLUMN id SET DEFAULT nextval('public.ops_assistant_requests_id_seq'::regclass);


--
-- Name: ops_assistant_threads id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_threads ALTER COLUMN id SET DEFAULT nextval('public.ops_assistant_threads_id_seq'::regclass);


--
-- Name: ops_assistant_tool_calls id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_tool_calls ALTER COLUMN id SET DEFAULT nextval('public.ops_assistant_tool_calls_id_seq'::regclass);


--
-- Name: orders id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders ALTER COLUMN id SET DEFAULT nextval('public.orders_id_seq'::regclass);


--
-- Name: page_views id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_views ALTER COLUMN id SET DEFAULT nextval('public.page_views_id_seq'::regclass);


--
-- Name: payment_refund_destinations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_refund_destinations ALTER COLUMN id SET DEFAULT nextval('public.payment_refund_destinations_id_seq'::regclass);


--
-- Name: payment_refunds id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_refunds ALTER COLUMN id SET DEFAULT nextval('public.payment_refunds_id_seq'::regclass);


--
-- Name: payments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments ALTER COLUMN id SET DEFAULT nextval('public.payments_id_seq'::regclass);


--
-- Name: payroll_line_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_line_items ALTER COLUMN id SET DEFAULT nextval('public.payroll_line_items_id_seq'::regclass);


--
-- Name: payroll_runs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_runs ALTER COLUMN id SET DEFAULT nextval('public.payroll_runs_id_seq'::regclass);


--
-- Name: platform_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_settings ALTER COLUMN id SET DEFAULT nextval('public.platform_settings_id_seq'::regclass);


--
-- Name: plugin_notification_deliveries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_notification_deliveries ALTER COLUMN id SET DEFAULT nextval('public.plugin_notification_deliveries_id_seq'::regclass);


--
-- Name: plugin_notification_delivery_attempts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_notification_delivery_attempts ALTER COLUMN id SET DEFAULT nextval('public.plugin_notification_delivery_attempts_id_seq'::regclass);


--
-- Name: plugin_translations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_translations ALTER COLUMN id SET DEFAULT nextval('public.plugin_translations_id_seq'::regclass);


--
-- Name: plugins id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugins ALTER COLUMN id SET DEFAULT nextval('public.plugins_id_seq'::regclass);


--
-- Name: poll_options id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.poll_options ALTER COLUMN id SET DEFAULT nextval('public.poll_options_id_seq'::regclass);


--
-- Name: poll_votes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.poll_votes ALTER COLUMN id SET DEFAULT nextval('public.poll_votes_id_seq'::regclass);


--
-- Name: polls id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.polls ALTER COLUMN id SET DEFAULT nextval('public.polls_id_seq'::regclass);


--
-- Name: positions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.positions ALTER COLUMN id SET DEFAULT nextval('public.positions_id_seq'::regclass);


--
-- Name: print_audit_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_audit_log ALTER COLUMN id SET DEFAULT nextval('public.print_audit_log_id_seq'::regclass);


--
-- Name: print_jobs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_jobs ALTER COLUMN id SET DEFAULT nextval('public.print_jobs_id_seq'::regclass);


--
-- Name: printers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.printers ALTER COLUMN id SET DEFAULT nextval('public.printers_id_seq'::regclass);


--
-- Name: push_subscriptions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.push_subscriptions ALTER COLUMN id SET DEFAULT nextval('public.push_subscriptions_id_seq'::regclass);


--
-- Name: rbac_audit_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rbac_audit_logs ALTER COLUMN id SET DEFAULT nextval('public.rbac_audit_logs_id_seq'::regclass);


--
-- Name: recurring_entry_templates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recurring_entry_templates ALTER COLUMN id SET DEFAULT nextval('public.recurring_entry_templates_id_seq'::regclass);


--
-- Name: report_deliveries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_deliveries ALTER COLUMN id SET DEFAULT nextval('public.report_deliveries_id_seq'::regclass);


--
-- Name: report_schedules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_schedules ALTER COLUMN id SET DEFAULT nextval('public.report_schedules_id_seq'::regclass);


--
-- Name: reservation_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_settings ALTER COLUMN id SET DEFAULT nextval('public.reservation_settings_id_seq'::regclass);


--
-- Name: reservation_status_histories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_status_histories ALTER COLUMN id SET DEFAULT nextval('public.reservation_status_histories_id_seq'::regclass);


--
-- Name: restaurant_spaces id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.restaurant_spaces ALTER COLUMN id SET DEFAULT nextval('public.restaurant_spaces_id_seq'::regclass);


--
-- Name: runtime_control_audit_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_control_audit_events ALTER COLUMN id SET DEFAULT nextval('public.runtime_control_audit_events_id_seq'::regclass);


--
-- Name: runtime_invite_batches id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_invite_batches ALTER COLUMN id SET DEFAULT nextval('public.runtime_invite_batches_id_seq'::regclass);


--
-- Name: runtime_invite_claims id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_invite_claims ALTER COLUMN id SET DEFAULT nextval('public.runtime_invite_claims_id_seq'::regclass);


--
-- Name: scheduler_states id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scheduler_states ALTER COLUMN id SET DEFAULT nextval('public.scheduler_states_id_seq'::regclass);


--
-- Name: schedules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedules ALTER COLUMN id SET DEFAULT nextval('public.schedules_id_seq'::regclass);


--
-- Name: shift_notes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shift_notes ALTER COLUMN id SET DEFAULT nextval('public.shift_notes_id_seq'::regclass);


--
-- Name: shift_swap_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shift_swap_requests ALTER COLUMN id SET DEFAULT nextval('public.shift_swap_requests_id_seq'::regclass);


--
-- Name: shifts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shifts ALTER COLUMN id SET DEFAULT nextval('public.shifts_id_seq'::regclass);


--
-- Name: shoutouts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shoutouts ALTER COLUMN id SET DEFAULT nextval('public.shoutouts_id_seq'::regclass);


--
-- Name: space_layout_audit_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_layout_audit_events ALTER COLUMN id SET DEFAULT nextval('public.space_layout_audit_events_id_seq'::regclass);


--
-- Name: space_layout_elements id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_layout_elements ALTER COLUMN id SET DEFAULT nextval('public.space_layout_elements_id_seq'::regclass);


--
-- Name: space_regions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_regions ALTER COLUMN id SET DEFAULT nextval('public.space_regions_id_seq'::regclass);


--
-- Name: space_scan_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_sessions ALTER COLUMN id SET DEFAULT nextval('public.space_scan_sessions_id_seq'::regclass);


--
-- Name: space_scan_uploads id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_uploads ALTER COLUMN id SET DEFAULT nextval('public.space_scan_uploads_id_seq'::regclass);


--
-- Name: staff id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff ALTER COLUMN id SET DEFAULT nextval('public.staff_id_seq'::regclass);


--
-- Name: staff_availabilities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_availabilities ALTER COLUMN id SET DEFAULT nextval('public.staff_availabilities_id_seq'::regclass);


--
-- Name: staff_identities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_identities ALTER COLUMN id SET DEFAULT nextval('public.staff_identities_id_seq'::regclass);


--
-- Name: staff_invitations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_invitations ALTER COLUMN id SET DEFAULT nextval('public.staff_invitations_id_seq'::regclass);


--
-- Name: staff_login_codes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_login_codes ALTER COLUMN id SET DEFAULT nextval('public.staff_login_codes_id_seq'::regclass);


--
-- Name: staff_memberships id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_memberships ALTER COLUMN id SET DEFAULT nextval('public.staff_memberships_id_seq'::regclass);


--
-- Name: staff_notifications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_notifications ALTER COLUMN id SET DEFAULT nextval('public.staff_notifications_id_seq'::regclass);


--
-- Name: staff_permission_denies id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_permission_denies ALTER COLUMN id SET DEFAULT nextval('public.staff_permission_denies_id_seq'::regclass);


--
-- Name: staff_positions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_positions ALTER COLUMN id SET DEFAULT nextval('public.staff_positions_id_seq'::regclass);


--
-- Name: supported_currencies id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supported_currencies ALTER COLUMN id SET DEFAULT nextval('public.supported_currencies_id_seq'::regclass);


--
-- Name: supported_languages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supported_languages ALTER COLUMN id SET DEFAULT nextval('public.supported_languages_id_seq'::regclass);


--
-- Name: table_combination_members id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combination_members ALTER COLUMN id SET DEFAULT nextval('public.table_combination_members_id_seq'::regclass);


--
-- Name: table_combinations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combinations ALTER COLUMN id SET DEFAULT nextval('public.table_combinations_id_seq'::regclass);


--
-- Name: table_reservations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_reservations ALTER COLUMN id SET DEFAULT nextval('public.table_reservations_id_seq'::regclass);


--
-- Name: tables id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tables ALTER COLUMN id SET DEFAULT nextval('public.tables_id_seq'::regclass);


--
-- Name: telegram_connection_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telegram_connection_tokens ALTER COLUMN id SET DEFAULT nextval('public.telegram_connection_tokens_id_seq'::regclass);


--
-- Name: telegram_update_receipts update_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telegram_update_receipts ALTER COLUMN update_id SET DEFAULT nextval('public.telegram_update_receipts_update_id_seq'::regclass);


--
-- Name: time_entries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.time_entries ALTER COLUMN id SET DEFAULT nextval('public.time_entries_id_seq'::regclass);


--
-- Name: time_off_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.time_off_requests ALTER COLUMN id SET DEFAULT nextval('public.time_off_requests_id_seq'::regclass);


--
-- Name: translations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.translations ALTER COLUMN id SET DEFAULT nextval('public.translations_id_seq'::regclass);


--
-- Name: user_auths id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_auths ALTER COLUMN id SET DEFAULT nextval('public.user_auths_id_seq'::regclass);


--
-- Name: user_interactions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_interactions ALTER COLUMN id SET DEFAULT nextval('public.user_interactions_id_seq'::regclass);


--
-- Name: user_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_sessions ALTER COLUMN id SET DEFAULT nextval('public.user_sessions_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: webhook_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_events ALTER COLUMN id SET DEFAULT nextval('public.webhook_events_id_seq'::regclass);


--
-- Name: whatsapp_business_devices id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.whatsapp_business_devices ALTER COLUMN id SET DEFAULT nextval('public.whatsapp_business_devices_id_seq'::regclass);


--
-- Name: withdrawal_histories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.withdrawal_histories ALTER COLUMN id SET DEFAULT nextval('public.withdrawal_histories_id_seq'::regclass);


--
-- Name: accounting_categories accounting_categories_business_id_key_entry_type_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounting_categories
    ADD CONSTRAINT accounting_categories_business_id_key_entry_type_key UNIQUE (business_id, key, entry_type);


--
-- Name: accounting_categories accounting_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounting_categories
    ADD CONSTRAINT accounting_categories_pkey PRIMARY KEY (id);


--
-- Name: accounting_period_locks accounting_period_locks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounting_period_locks
    ADD CONSTRAINT accounting_period_locks_pkey PRIMARY KEY (id);


--
-- Name: activation_event_outbox activation_event_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activation_event_outbox
    ADD CONSTRAINT activation_event_outbox_pkey PRIMARY KEY (id);


--
-- Name: admin_actions admin_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_actions
    ADD CONSTRAINT admin_actions_pkey PRIMARY KEY (id);


--
-- Name: ai_budget_controls ai_budget_controls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_budget_controls
    ADD CONSTRAINT ai_budget_controls_pkey PRIMARY KEY (id);


--
-- Name: ai_daily_spend ai_daily_spend_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_daily_spend
    ADD CONSTRAINT ai_daily_spend_pkey PRIMARY KEY (id);


--
-- Name: ai_generated_images ai_generated_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_generated_images
    ADD CONSTRAINT ai_generated_images_pkey PRIMARY KEY (id);


--
-- Name: ai_image_usage ai_image_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_image_usage
    ADD CONSTRAINT ai_image_usage_pkey PRIMARY KEY (id);


--
-- Name: ai_spend_reservations ai_spend_reservations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_spend_reservations
    ADD CONSTRAINT ai_spend_reservations_pkey PRIMARY KEY (id);


--
-- Name: ai_waiter_conversations ai_waiter_conversations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_waiter_conversations
    ADD CONSTRAINT ai_waiter_conversations_pkey PRIMARY KEY (id);


--
-- Name: ai_waiter_messages ai_waiter_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_waiter_messages
    ADD CONSTRAINT ai_waiter_messages_pkey PRIMARY KEY (id);


--
-- Name: alternative_payments alternative_payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alternative_payments
    ADD CONSTRAINT alternative_payments_pkey PRIMARY KEY (id);


--
-- Name: announcement_acks announcement_acks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.announcement_acks
    ADD CONSTRAINT announcement_acks_pkey PRIMARY KEY (announcement_id, staff_id);


--
-- Name: announcements announcements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.announcements
    ADD CONSTRAINT announcements_pkey PRIMARY KEY (id);


--
-- Name: auth_attempts auth_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_attempts
    ADD CONSTRAINT auth_attempts_pkey PRIMARY KEY (id);


--
-- Name: bill_history_events bill_history_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_history_events
    ADD CONSTRAINT bill_history_events_pkey PRIMARY KEY (id);


--
-- Name: bill_items bill_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_items
    ADD CONSTRAINT bill_items_pkey PRIMARY KEY (id);


--
-- Name: bill_split_shares bill_split_shares_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_split_shares
    ADD CONSTRAINT bill_split_shares_pkey PRIMARY KEY (id);


--
-- Name: bills bills_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bills
    ADD CONSTRAINT bills_pkey PRIMARY KEY (id);


--
-- Name: bundles bundles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bundles
    ADD CONSTRAINT bundles_pkey PRIMARY KEY (id);


--
-- Name: business_activation_states business_activation_states_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_activation_states
    ADD CONSTRAINT business_activation_states_pkey PRIMARY KEY (business_id);


--
-- Name: business_alert_settings business_alert_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_alert_settings
    ADD CONSTRAINT business_alert_settings_pkey PRIMARY KEY (id);


--
-- Name: business_creation_requests business_creation_owner_key_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_creation_requests
    ADD CONSTRAINT business_creation_owner_key_unique UNIQUE (owner_scope_hash, idempotency_key_hash);


--
-- Name: business_creation_requests business_creation_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_creation_requests
    ADD CONSTRAINT business_creation_requests_pkey PRIMARY KEY (id);


--
-- Name: business_currencies business_currencies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_currencies
    ADD CONSTRAINT business_currencies_pkey PRIMARY KEY (id);


--
-- Name: business_fiscal_settings business_fiscal_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_fiscal_settings
    ADD CONSTRAINT business_fiscal_settings_pkey PRIMARY KEY (id);


--
-- Name: business_gallery_images business_gallery_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_gallery_images
    ADD CONSTRAINT business_gallery_images_pkey PRIMARY KEY (id);


--
-- Name: business_languages business_languages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_languages
    ADD CONSTRAINT business_languages_pkey PRIMARY KEY (id);


--
-- Name: business_milestone_events business_milestone_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_milestone_events
    ADD CONSTRAINT business_milestone_events_pkey PRIMARY KEY (id);


--
-- Name: business_operating_exceptions business_operating_exceptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_operating_exceptions
    ADD CONSTRAINT business_operating_exceptions_pkey PRIMARY KEY (id);


--
-- Name: business_operating_hours business_operating_hours_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_operating_hours
    ADD CONSTRAINT business_operating_hours_pkey PRIMARY KEY (id);


--
-- Name: business_plugins business_plugins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_plugins
    ADD CONSTRAINT business_plugins_pkey PRIMARY KEY (id);


--
-- Name: business_revenue_aggregates business_revenue_aggregates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_revenue_aggregates
    ADD CONSTRAINT business_revenue_aggregates_pkey PRIMARY KEY (id);


--
-- Name: business_schedule_settings business_schedule_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_schedule_settings
    ADD CONSTRAINT business_schedule_settings_pkey PRIMARY KEY (id);


--
-- Name: business_special_features business_special_features_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_special_features
    ADD CONSTRAINT business_special_features_pkey PRIMARY KEY (id);


--
-- Name: businesses businesses_has_canonical_owner; Type: CHECK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE public.businesses
    ADD CONSTRAINT businesses_has_canonical_owner CHECK (((owner_address ~ '[^[:space:]]'::text) OR (user_id IS NOT NULL))) NOT VALID;


--
-- Name: businesses businesses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.businesses
    ADD CONSTRAINT businesses_pkey PRIMARY KEY (id);


--
-- Name: cash_register_movements cash_register_movements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_movements
    ADD CONSTRAINT cash_register_movements_pkey PRIMARY KEY (id);


--
-- Name: cash_register_sessions cash_register_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_sessions
    ADD CONSTRAINT cash_register_sessions_pkey PRIMARY KEY (id);


--
-- Name: chat_channel_members chat_channel_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_channel_members
    ADD CONSTRAINT chat_channel_members_pkey PRIMARY KEY (channel_id, staff_id);


--
-- Name: chat_channels chat_channels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_channels
    ADD CONSTRAINT chat_channels_pkey PRIMARY KEY (id);


--
-- Name: chat_messages chat_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages
    ADD CONSTRAINT chat_messages_pkey PRIMARY KEY (id);


--
-- Name: chat_reads chat_reads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_reads
    ADD CONSTRAINT chat_reads_pkey PRIMARY KEY (channel_id, staff_id);


--
-- Name: checklist_item_completions checklist_item_completions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_item_completions
    ADD CONSTRAINT checklist_item_completions_pkey PRIMARY KEY (id);


--
-- Name: checklist_items checklist_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_items
    ADD CONSTRAINT checklist_items_pkey PRIMARY KEY (id);


--
-- Name: checklist_runs checklist_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_runs
    ADD CONSTRAINT checklist_runs_pkey PRIMARY KEY (id);


--
-- Name: checklist_templates checklist_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_templates
    ADD CONSTRAINT checklist_templates_pkey PRIMARY KEY (id);


--
-- Name: comp_void_audit comp_void_audit_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comp_void_audit
    ADD CONSTRAINT comp_void_audit_pkey PRIMARY KEY (id);


--
-- Name: conversion_events conversion_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversion_events
    ADD CONSTRAINT conversion_events_pkey PRIMARY KEY (id);


--
-- Name: counters counters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.counters
    ADD CONSTRAINT counters_pkey PRIMARY KEY (id);


--
-- Name: crypto_payment_quotes crypto_payment_quotes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crypto_payment_quotes
    ADD CONSTRAINT crypto_payment_quotes_pkey PRIMARY KEY (id);


--
-- Name: customer_addresses customer_addresses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_addresses
    ADD CONSTRAINT customer_addresses_pkey PRIMARY KEY (id);


--
-- Name: customer_businesses customer_businesses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_businesses
    ADD CONSTRAINT customer_businesses_pkey PRIMARY KEY (id);


--
-- Name: customer_communications customer_communications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_communications
    ADD CONSTRAINT customer_communications_pkey PRIMARY KEY (id);


--
-- Name: customer_preferences customer_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_preferences
    ADD CONSTRAINT customer_preferences_pkey PRIMARY KEY (id);


--
-- Name: customer_visits customer_visits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_visits
    ADD CONSTRAINT customer_visits_pkey PRIMARY KEY (id);


--
-- Name: customers customers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_pkey PRIMARY KEY (id);


--
-- Name: delivery_drivers delivery_drivers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_drivers
    ADD CONSTRAINT delivery_drivers_pkey PRIMARY KEY (id);


--
-- Name: delivery_orders delivery_orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_orders
    ADD CONSTRAINT delivery_orders_pkey PRIMARY KEY (id);


--
-- Name: delivery_settings delivery_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_settings
    ADD CONSTRAINT delivery_settings_pkey PRIMARY KEY (id);


--
-- Name: delivery_status_history delivery_status_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_status_history
    ADD CONSTRAINT delivery_status_history_pkey PRIMARY KEY (id);


--
-- Name: delivery_zones delivery_zones_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_zones
    ADD CONSTRAINT delivery_zones_pkey PRIMARY KEY (id);


--
-- Name: demo_instances demo_instances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.demo_instances
    ADD CONSTRAINT demo_instances_pkey PRIMARY KEY (id);


--
-- Name: demo_runs demo_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.demo_runs
    ADD CONSTRAINT demo_runs_pkey PRIMARY KEY (id);


--
-- Name: director_action_audits director_action_audits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_action_audits
    ADD CONSTRAINT director_action_audits_pkey PRIMARY KEY (id);


--
-- Name: director_console_messages director_console_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_console_messages
    ADD CONSTRAINT director_console_messages_pkey PRIMARY KEY (id);


--
-- Name: director_console_threads director_console_threads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_console_threads
    ADD CONSTRAINT director_console_threads_pkey PRIMARY KEY (id);


--
-- Name: director_proposed_actions director_proposed_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_proposed_actions
    ADD CONSTRAINT director_proposed_actions_pkey PRIMARY KEY (id);


--
-- Name: director_tool_calls director_tool_calls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_tool_calls
    ADD CONSTRAINT director_tool_calls_pkey PRIMARY KEY (id);


--
-- Name: document_acks document_acks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_acks
    ADD CONSTRAINT document_acks_pkey PRIMARY KEY (id);


--
-- Name: documents documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_pkey PRIMARY KEY (id);


--
-- Name: email_delivery_events email_delivery_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_delivery_events
    ADD CONSTRAINT email_delivery_events_pkey PRIMARY KEY (id);


--
-- Name: email_delivery_events email_delivery_events_provider_webhook_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_delivery_events
    ADD CONSTRAINT email_delivery_events_provider_webhook_unique UNIQUE (provider, webhook_id);


--
-- Name: email_outbound_sends email_outbound_sends_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_outbound_sends
    ADD CONSTRAINT email_outbound_sends_pkey PRIMARY KEY (id);


--
-- Name: email_outbound_sends email_outbound_sends_provider_msg_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_outbound_sends
    ADD CONSTRAINT email_outbound_sends_provider_msg_unique UNIQUE (provider, provider_message_id);


--
-- Name: email_outbox email_outbox_idempotency_key_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_outbox
    ADD CONSTRAINT email_outbox_idempotency_key_unique UNIQUE (idempotency_key);


--
-- Name: email_outbox email_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_outbox
    ADD CONSTRAINT email_outbox_pkey PRIMARY KEY (id);


--
-- Name: email_suppressions email_suppressions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_suppressions
    ADD CONSTRAINT email_suppressions_pkey PRIMARY KEY (email);


--
-- Name: error_logs error_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.error_logs
    ADD CONSTRAINT error_logs_pkey PRIMARY KEY (id);


--
-- Name: escalations escalations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.escalations
    ADD CONSTRAINT escalations_pkey PRIMARY KEY (id);


--
-- Name: exchange_rates exchange_rates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exchange_rates
    ADD CONSTRAINT exchange_rates_pkey PRIMARY KEY (id);


--
-- Name: fiscal_audit_events fiscal_audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_audit_events
    ADD CONSTRAINT fiscal_audit_events_pkey PRIMARY KEY (id);


--
-- Name: fiscal_delivery_tasks fiscal_delivery_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_delivery_tasks
    ADD CONSTRAINT fiscal_delivery_tasks_pkey PRIMARY KEY (id);


--
-- Name: fiscal_jobs fiscal_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_jobs
    ADD CONSTRAINT fiscal_jobs_pkey PRIMARY KEY (id);


--
-- Name: fiscal_receipts fiscal_receipts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_receipts
    ADD CONSTRAINT fiscal_receipts_pkey PRIMARY KEY (id);


--
-- Name: guest_receipt_sends guest_receipt_sends_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_receipt_sends
    ADD CONSTRAINT guest_receipt_sends_pkey PRIMARY KEY (id);


--
-- Name: idempotency_keys idempotency_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.idempotency_keys
    ADD CONSTRAINT idempotency_keys_pkey PRIMARY KEY (id);


--
-- Name: inventory_alert_logs inventory_alert_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_alert_logs
    ADD CONSTRAINT inventory_alert_logs_pkey PRIMARY KEY (id);


--
-- Name: inventory_items inventory_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_items
    ADD CONSTRAINT inventory_items_pkey PRIMARY KEY (id);


--
-- Name: inventory_movements inventory_movements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_movements
    ADD CONSTRAINT inventory_movements_pkey PRIMARY KEY (id);


--
-- Name: inventory_recipes inventory_recipes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_recipes
    ADD CONSTRAINT inventory_recipes_pkey PRIMARY KEY (id);


--
-- Name: inventory_settings inventory_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_settings
    ADD CONSTRAINT inventory_settings_pkey PRIMARY KEY (id);


--
-- Name: ledger_entry_attachments ledger_entry_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ledger_entry_attachments
    ADD CONSTRAINT ledger_entry_attachments_pkey PRIMARY KEY (id);


--
-- Name: loyalty_programs loyalty_programs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loyalty_programs
    ADD CONSTRAINT loyalty_programs_pkey PRIMARY KEY (id);


--
-- Name: loyalty_tiers loyalty_tiers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loyalty_tiers
    ADD CONSTRAINT loyalty_tiers_pkey PRIMARY KEY (id);


--
-- Name: manual_ledger_entries manual_ledger_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manual_ledger_entries
    ADD CONSTRAINT manual_ledger_entries_pkey PRIMARY KEY (id);


--
-- Name: marketing_activities marketing_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.marketing_activities
    ADD CONSTRAINT marketing_activities_pkey PRIMARY KEY (id);


--
-- Name: menu_extraction_images menu_extraction_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_extraction_images
    ADD CONSTRAINT menu_extraction_images_pkey PRIMARY KEY (id);


--
-- Name: menu_extraction_jobs menu_extraction_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_extraction_jobs
    ADD CONSTRAINT menu_extraction_jobs_pkey PRIMARY KEY (id);


--
-- Name: menu_wizard_messages menu_wizard_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_wizard_messages
    ADD CONSTRAINT menu_wizard_messages_pkey PRIMARY KEY (id);


--
-- Name: menu_wizard_sessions menu_wizard_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_wizard_sessions
    ADD CONSTRAINT menu_wizard_sessions_pkey PRIMARY KEY (id);


--
-- Name: menus menus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_pkey PRIMARY KEY (id);


--
-- Name: missing_translations missing_translations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.missing_translations
    ADD CONSTRAINT missing_translations_pkey PRIMARY KEY (id);


--
-- Name: offers offers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT offers_pkey PRIMARY KEY (id);


--
-- Name: open_shift_claims open_shift_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.open_shift_claims
    ADD CONSTRAINT open_shift_claims_pkey PRIMARY KEY (id);


--
-- Name: operational_alert_events operational_alert_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alert_events
    ADD CONSTRAINT operational_alert_events_pkey PRIMARY KEY (id);


--
-- Name: operational_alerts operational_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alerts
    ADD CONSTRAINT operational_alerts_pkey PRIMARY KEY (id);


--
-- Name: ops_assistant_messages ops_assistant_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_messages
    ADD CONSTRAINT ops_assistant_messages_pkey PRIMARY KEY (id);


--
-- Name: ops_assistant_requests ops_assistant_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_requests
    ADD CONSTRAINT ops_assistant_requests_pkey PRIMARY KEY (id);


--
-- Name: ops_assistant_threads ops_assistant_threads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_threads
    ADD CONSTRAINT ops_assistant_threads_pkey PRIMARY KEY (id);


--
-- Name: ops_assistant_tool_calls ops_assistant_tool_calls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_tool_calls
    ADD CONSTRAINT ops_assistant_tool_calls_pkey PRIMARY KEY (id);


--
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (id);


--
-- Name: page_views page_views_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_views
    ADD CONSTRAINT page_views_pkey PRIMARY KEY (id);


--
-- Name: payment_refund_destinations payment_refund_destinations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_refund_destinations
    ADD CONSTRAINT payment_refund_destinations_pkey PRIMARY KEY (id);


--
-- Name: payment_refunds payment_refunds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_refunds
    ADD CONSTRAINT payment_refunds_pkey PRIMARY KEY (id);


--
-- Name: payments payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_pkey PRIMARY KEY (id);


--
-- Name: payroll_line_items payroll_line_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_line_items
    ADD CONSTRAINT payroll_line_items_pkey PRIMARY KEY (id);


--
-- Name: payroll_runs payroll_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_runs
    ADD CONSTRAINT payroll_runs_pkey PRIMARY KEY (id);


--
-- Name: platform_settings platform_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_settings
    ADD CONSTRAINT platform_settings_pkey PRIMARY KEY (id);


--
-- Name: plugin_notification_deliveries plugin_notification_deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_notification_deliveries
    ADD CONSTRAINT plugin_notification_deliveries_pkey PRIMARY KEY (id);


--
-- Name: plugin_notification_delivery_attempts plugin_notification_delivery_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_notification_delivery_attempts
    ADD CONSTRAINT plugin_notification_delivery_attempts_pkey PRIMARY KEY (id);


--
-- Name: plugin_translations plugin_translations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_translations
    ADD CONSTRAINT plugin_translations_pkey PRIMARY KEY (id);


--
-- Name: plugins plugins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugins
    ADD CONSTRAINT plugins_pkey PRIMARY KEY (id);


--
-- Name: poll_options poll_options_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.poll_options
    ADD CONSTRAINT poll_options_pkey PRIMARY KEY (id);


--
-- Name: poll_votes poll_votes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.poll_votes
    ADD CONSTRAINT poll_votes_pkey PRIMARY KEY (id);


--
-- Name: polls polls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.polls
    ADD CONSTRAINT polls_pkey PRIMARY KEY (id);


--
-- Name: positions positions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.positions
    ADD CONSTRAINT positions_pkey PRIMARY KEY (id);


--
-- Name: print_audit_log print_audit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_audit_log
    ADD CONSTRAINT print_audit_log_pkey PRIMARY KEY (id);


--
-- Name: print_jobs print_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_jobs
    ADD CONSTRAINT print_jobs_pkey PRIMARY KEY (id);


--
-- Name: printers printers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.printers
    ADD CONSTRAINT printers_pkey PRIMARY KEY (id);


--
-- Name: push_subscriptions push_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.push_subscriptions
    ADD CONSTRAINT push_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: rbac_audit_logs rbac_audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rbac_audit_logs
    ADD CONSTRAINT rbac_audit_logs_pkey PRIMARY KEY (id);


--
-- Name: recurring_entry_templates recurring_entry_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recurring_entry_templates
    ADD CONSTRAINT recurring_entry_templates_pkey PRIMARY KEY (id);


--
-- Name: report_deliveries report_deliveries_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_deliveries
    ADD CONSTRAINT report_deliveries_idempotency_key_key UNIQUE (idempotency_key);


--
-- Name: report_deliveries report_deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_deliveries
    ADD CONSTRAINT report_deliveries_pkey PRIMARY KEY (id);


--
-- Name: report_deliveries report_deliveries_schedule_id_window_start_window_end_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_deliveries
    ADD CONSTRAINT report_deliveries_schedule_id_window_start_window_end_key UNIQUE (schedule_id, window_start, window_end);


--
-- Name: report_schedules report_schedules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_schedules
    ADD CONSTRAINT report_schedules_pkey PRIMARY KEY (id);


--
-- Name: reservation_settings reservation_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_settings
    ADD CONSTRAINT reservation_settings_pkey PRIMARY KEY (id);


--
-- Name: reservation_status_histories reservation_status_histories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_status_histories
    ADD CONSTRAINT reservation_status_histories_pkey PRIMARY KEY (id);


--
-- Name: restaurant_spaces restaurant_spaces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.restaurant_spaces
    ADD CONSTRAINT restaurant_spaces_pkey PRIMARY KEY (id);


--
-- Name: runtime_control_audit_events runtime_control_audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_control_audit_events
    ADD CONSTRAINT runtime_control_audit_events_pkey PRIMARY KEY (id);


--
-- Name: runtime_controls runtime_controls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_controls
    ADD CONSTRAINT runtime_controls_pkey PRIMARY KEY (key);


--
-- Name: runtime_invite_batches runtime_invite_batches_code_digest_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_invite_batches
    ADD CONSTRAINT runtime_invite_batches_code_digest_key UNIQUE (code_digest);


--
-- Name: runtime_invite_batches runtime_invite_batches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_invite_batches
    ADD CONSTRAINT runtime_invite_batches_pkey PRIMARY KEY (id);


--
-- Name: runtime_invite_claims runtime_invite_claims_normalized_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_invite_claims
    ADD CONSTRAINT runtime_invite_claims_normalized_email_key UNIQUE (normalized_email);


--
-- Name: runtime_invite_claims runtime_invite_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_invite_claims
    ADD CONSTRAINT runtime_invite_claims_pkey PRIMARY KEY (id);


--
-- Name: scheduler_states scheduler_states_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scheduler_states
    ADD CONSTRAINT scheduler_states_pkey PRIMARY KEY (id);


--
-- Name: schedules schedules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedules
    ADD CONSTRAINT schedules_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: session_summaries session_summaries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.session_summaries
    ADD CONSTRAINT session_summaries_pkey PRIMARY KEY (session_id);


--
-- Name: shift_notes shift_notes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shift_notes
    ADD CONSTRAINT shift_notes_pkey PRIMARY KEY (id);


--
-- Name: shift_swap_requests shift_swap_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shift_swap_requests
    ADD CONSTRAINT shift_swap_requests_pkey PRIMARY KEY (id);


--
-- Name: shifts shifts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shifts
    ADD CONSTRAINT shifts_pkey PRIMARY KEY (id);


--
-- Name: shoutouts shoutouts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shoutouts
    ADD CONSTRAINT shoutouts_pkey PRIMARY KEY (id);


--
-- Name: space_layout_audit_events space_layout_audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_layout_audit_events
    ADD CONSTRAINT space_layout_audit_events_pkey PRIMARY KEY (id);


--
-- Name: space_layout_elements space_layout_elements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_layout_elements
    ADD CONSTRAINT space_layout_elements_pkey PRIMARY KEY (id);


--
-- Name: space_regions space_regions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_regions
    ADD CONSTRAINT space_regions_pkey PRIMARY KEY (id);


--
-- Name: space_scan_sessions space_scan_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_sessions
    ADD CONSTRAINT space_scan_sessions_pkey PRIMARY KEY (id);


--
-- Name: space_scan_sessions space_scan_sessions_token_hash_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_sessions
    ADD CONSTRAINT space_scan_sessions_token_hash_unique UNIQUE (token_hash);


--
-- Name: space_scan_uploads space_scan_uploads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_uploads
    ADD CONSTRAINT space_scan_uploads_pkey PRIMARY KEY (id);


--
-- Name: staff_availabilities staff_availabilities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_availabilities
    ADD CONSTRAINT staff_availabilities_pkey PRIMARY KEY (id);


--
-- Name: staff_identities staff_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_identities
    ADD CONSTRAINT staff_identities_pkey PRIMARY KEY (id);


--
-- Name: staff_invitations staff_invitations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_invitations
    ADD CONSTRAINT staff_invitations_pkey PRIMARY KEY (id);


--
-- Name: staff_login_codes staff_login_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_login_codes
    ADD CONSTRAINT staff_login_codes_pkey PRIMARY KEY (id);


--
-- Name: staff_membership_selection_tokens staff_membership_selection_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_membership_selection_tokens
    ADD CONSTRAINT staff_membership_selection_tokens_pkey PRIMARY KEY (jti_hash);


--
-- Name: staff_memberships staff_memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_memberships
    ADD CONSTRAINT staff_memberships_pkey PRIMARY KEY (id);


--
-- Name: staff_notifications staff_notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_notifications
    ADD CONSTRAINT staff_notifications_pkey PRIMARY KEY (id);


--
-- Name: staff_permission_denies staff_permission_denies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_permission_denies
    ADD CONSTRAINT staff_permission_denies_pkey PRIMARY KEY (id);


--
-- Name: staff staff_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff
    ADD CONSTRAINT staff_pkey PRIMARY KEY (id);


--
-- Name: staff_positions staff_positions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_positions
    ADD CONSTRAINT staff_positions_pkey PRIMARY KEY (id);


--
-- Name: supported_currencies supported_currencies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supported_currencies
    ADD CONSTRAINT supported_currencies_pkey PRIMARY KEY (id);


--
-- Name: supported_languages supported_languages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supported_languages
    ADD CONSTRAINT supported_languages_pkey PRIMARY KEY (id);


--
-- Name: table_combination_members table_combination_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combination_members
    ADD CONSTRAINT table_combination_members_pkey PRIMARY KEY (id);


--
-- Name: table_combination_members table_combination_members_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combination_members
    ADD CONSTRAINT table_combination_members_unique UNIQUE (combination_id, table_id);


--
-- Name: table_combinations table_combinations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combinations
    ADD CONSTRAINT table_combinations_pkey PRIMARY KEY (id);


--
-- Name: table_reservations table_reservations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_reservations
    ADD CONSTRAINT table_reservations_pkey PRIMARY KEY (id);


--
-- Name: tables tables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tables
    ADD CONSTRAINT tables_pkey PRIMARY KEY (id);


--
-- Name: telegram_connection_tokens telegram_connection_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telegram_connection_tokens
    ADD CONSTRAINT telegram_connection_tokens_pkey PRIMARY KEY (id);


--
-- Name: telegram_update_receipts telegram_update_receipts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telegram_update_receipts
    ADD CONSTRAINT telegram_update_receipts_pkey PRIMARY KEY (update_id);


--
-- Name: time_entries time_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.time_entries
    ADD CONSTRAINT time_entries_pkey PRIMARY KEY (id);


--
-- Name: time_off_requests time_off_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.time_off_requests
    ADD CONSTRAINT time_off_requests_pkey PRIMARY KEY (id);


--
-- Name: translations translations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.translations
    ADD CONSTRAINT translations_pkey PRIMARY KEY (id);


--
-- Name: delivery_settings uni_delivery_settings_business_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_settings
    ADD CONSTRAINT uni_delivery_settings_business_id UNIQUE (business_id);


--
-- Name: reservation_settings uni_reservation_settings_business_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_settings
    ADD CONSTRAINT uni_reservation_settings_business_id UNIQUE (business_id);


--
-- Name: business_operating_exceptions uq_business_operating_exception_date; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_operating_exceptions
    ADD CONSTRAINT uq_business_operating_exception_date UNIQUE (business_id, exception_date);


--
-- Name: ops_assistant_requests uq_ops_assistant_requests_biz_client; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_requests
    ADD CONSTRAINT uq_ops_assistant_requests_biz_client UNIQUE (business_id, client_request_id);


--
-- Name: whatsapp_business_devices uq_whatsapp_business_device_business; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.whatsapp_business_devices
    ADD CONSTRAINT uq_whatsapp_business_device_business UNIQUE (business_id);


--
-- Name: whatsapp_business_devices uq_whatsapp_business_device_jid; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.whatsapp_business_devices
    ADD CONSTRAINT uq_whatsapp_business_device_jid UNIQUE (device_jid);


--
-- Name: user_auths user_auths_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_auths
    ADD CONSTRAINT user_auths_pkey PRIMARY KEY (id);


--
-- Name: user_interactions user_interactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_interactions
    ADD CONSTRAINT user_interactions_pkey PRIMARY KEY (id);


--
-- Name: user_session_refresh_history user_session_refresh_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_session_refresh_history
    ADD CONSTRAINT user_session_refresh_history_pkey PRIMARY KEY (session_id, token_hash);


--
-- Name: user_session_refresh_history user_session_refresh_history_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_session_refresh_history
    ADD CONSTRAINT user_session_refresh_history_token_hash_key UNIQUE (token_hash);


--
-- Name: user_sessions user_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: webhook_events webhook_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_events
    ADD CONSTRAINT webhook_events_pkey PRIMARY KEY (id);


--
-- Name: whatsapp_business_devices whatsapp_business_devices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.whatsapp_business_devices
    ADD CONSTRAINT whatsapp_business_devices_pkey PRIMARY KEY (id);


--
-- Name: withdrawal_histories withdrawal_histories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.withdrawal_histories
    ADD CONSTRAINT withdrawal_histories_pkey PRIMARY KEY (id);


--
-- Name: idempotency_keys_key_endpoint; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idempotency_keys_key_endpoint ON public.idempotency_keys USING btree (key, endpoint);


--
-- Name: idempotency_keys_ttl; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idempotency_keys_ttl ON public.idempotency_keys USING btree (ttl_at);


--
-- Name: idx_accounting_categories_business_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_accounting_categories_business_active ON public.accounting_categories USING btree (business_id, entry_type, active, "position");


--
-- Name: idx_accounting_period_locks_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_accounting_period_locks_business_created ON public.accounting_period_locks USING btree (business_id, created_at DESC);


--
-- Name: idx_activation_event_outbox_delivery; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activation_event_outbox_delivery ON public.activation_event_outbox USING btree (delivery_status, occurred_at, id);


--
-- Name: idx_activation_event_outbox_funnel; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activation_event_outbox_funnel ON public.activation_event_outbox USING btree (funnel_id, occurred_at, id) WHERE (funnel_id IS NOT NULL);


--
-- Name: idx_activation_event_outbox_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_activation_event_outbox_idempotency ON public.activation_event_outbox USING btree (idempotency_key);


--
-- Name: idx_activation_event_outbox_server_first; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_activation_event_outbox_server_first ON public.activation_event_outbox USING btree (business_id, event_name) WHERE ((origin)::text = 'server'::text);


--
-- Name: idx_admin_actions_admin_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_admin_actions_admin_user_id ON public.admin_actions USING btree (admin_user_id);


--
-- Name: idx_admin_actions_target_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_admin_actions_target_user_id ON public.admin_actions USING btree (target_user_id);


--
-- Name: idx_ai_daily_spend_biz_scope_date; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_ai_daily_spend_biz_scope_date ON public.ai_daily_spend USING btree (business_id, feature_scope, usage_date);


--
-- Name: idx_ai_generated_images_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_generated_images_business ON public.ai_generated_images USING btree (business_id);


--
-- Name: idx_ai_generated_images_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_generated_images_business_id ON public.ai_generated_images USING btree (business_id);


--
-- Name: idx_ai_generated_images_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_generated_images_created_at ON public.ai_generated_images USING btree (created_at);


--
-- Name: idx_ai_image_usage_business; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_ai_image_usage_business ON public.ai_image_usage USING btree (business_id);


--
-- Name: idx_ai_spend_reservations_biz_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_spend_reservations_biz_date ON public.ai_spend_reservations USING btree (business_id, feature_scope, usage_date);


--
-- Name: idx_ai_spend_reservations_expire; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_spend_reservations_expire ON public.ai_spend_reservations USING btree (expires_at) WHERE (status = ANY (ARRAY['reserved'::text, 'consumed'::text]));


--
-- Name: idx_ai_spend_reservations_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_spend_reservations_parent ON public.ai_spend_reservations USING btree (parent_reservation_id) WHERE (parent_reservation_id IS NOT NULL);


--
-- Name: idx_ai_waiter_conversations_business_claimed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_waiter_conversations_business_claimed_at ON public.ai_waiter_conversations USING btree (business_id, claimed_at) WHERE (claimed_at IS NOT NULL);


--
-- Name: idx_ai_waiter_conversations_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_waiter_conversations_business_id ON public.ai_waiter_conversations USING btree (business_id);


--
-- Name: idx_ai_waiter_conversations_business_status_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_waiter_conversations_business_status_updated_at ON public.ai_waiter_conversations USING btree (business_id, status, updated_at DESC);


--
-- Name: idx_ai_waiter_conversations_claimed_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_waiter_conversations_claimed_by_staff_id ON public.ai_waiter_conversations USING btree (claimed_by_staff_id);


--
-- Name: idx_ai_waiter_conversations_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_waiter_conversations_created_at ON public.ai_waiter_conversations USING btree (created_at);


--
-- Name: idx_ai_waiter_messages_conversation_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_waiter_messages_conversation_created_at ON public.ai_waiter_messages USING btree (conversation_id, created_at);


--
-- Name: idx_ai_waiter_messages_conversation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_waiter_messages_conversation_id ON public.ai_waiter_messages USING btree (conversation_id);


--
-- Name: idx_ai_waiter_messages_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_waiter_messages_created_at ON public.ai_waiter_messages USING btree (created_at);


--
-- Name: idx_ai_waiter_session_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_ai_waiter_session_scope ON public.ai_waiter_conversations USING btree (session_id, business_id, table_code, mode);


--
-- Name: idx_alt_payments_bill_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alt_payments_bill_created_at ON public.alternative_payments USING btree (bill_id, created_at);


--
-- Name: idx_alt_payments_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alt_payments_bill_id ON public.alternative_payments USING btree (bill_id);


--
-- Name: idx_alt_payments_bill_idem; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_alt_payments_bill_idem ON public.alternative_payments USING btree (bill_id, idempotency_key) WHERE (idempotency_key <> ''::text);


--
-- Name: idx_alt_payments_participant_addr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alt_payments_participant_addr ON public.alternative_payments USING btree (participant_addr) WHERE (participant_addr <> ''::text);


--
-- Name: idx_alt_payments_status_method_confirmed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alt_payments_status_method_confirmed_at ON public.alternative_payments USING btree (status, payment_method, confirmed_at DESC);


--
-- Name: idx_alt_payments_status_method_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alt_payments_status_method_created_at ON public.alternative_payments USING btree (status, payment_method, created_at DESC);


--
-- Name: idx_alternative_payments_bill_idempotency_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_alternative_payments_bill_idempotency_hash ON public.alternative_payments USING btree (bill_id, idempotency_key_hash) WHERE ((idempotency_key_hash IS NOT NULL) AND ((idempotency_key_hash)::text <> ''::text));


--
-- Name: idx_alternative_payments_bill_payer_guest_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alternative_payments_bill_payer_guest_session ON public.alternative_payments USING btree (bill_id, payer_guest_session) WHERE (payer_guest_session IS NOT NULL);


--
-- Name: idx_alternative_payments_idempotency_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alternative_payments_idempotency_key ON public.alternative_payments USING btree (idempotency_key);


--
-- Name: idx_alternative_payments_pending_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alternative_payments_pending_expiry ON public.alternative_payments USING btree (bill_id, expires_at) WHERE (status = 'pending'::text);


--
-- Name: idx_alternative_payments_resolved_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alternative_payments_resolved_at ON public.alternative_payments USING btree (resolved_at) WHERE (resolved_at IS NOT NULL);


--
-- Name: idx_ann_acks_biz; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ann_acks_biz ON public.announcement_acks USING btree (business_id);


--
-- Name: idx_announcements_biz_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_announcements_biz_created ON public.announcements USING btree (business_id, created_at);


--
-- Name: idx_bill_events_bill_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_events_bill_created ON public.bill_history_events USING btree (bill_id, created_at);


--
-- Name: idx_bill_history_events_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_history_events_bill_id ON public.bill_history_events USING btree (bill_id);


--
-- Name: idx_bill_history_events_bill_item_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_history_events_bill_item_id ON public.bill_history_events USING btree (bill_item_id);


--
-- Name: idx_bill_history_events_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_history_events_business_id ON public.bill_history_events USING btree (business_id);


--
-- Name: idx_bill_history_events_event_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_history_events_event_type ON public.bill_history_events USING btree (event_type);


--
-- Name: idx_bill_history_events_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_history_events_order_id ON public.bill_history_events USING btree (order_id);


--
-- Name: idx_bill_items_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_items_bill_id ON public.bill_items USING btree (bill_id);


--
-- Name: idx_bill_items_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_items_name ON public.bill_items USING btree (name);


--
-- Name: idx_bill_items_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_items_order_id ON public.bill_items USING btree (order_id);


--
-- Name: idx_bill_split_shares_alternative_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_alternative_payment_id ON public.bill_split_shares USING btree (alternative_payment_id);


--
-- Name: idx_bill_split_shares_bill_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_bill_expires ON public.bill_split_shares USING btree (bill_id, hold_expires_at);


--
-- Name: idx_bill_split_shares_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_bill_id ON public.bill_split_shares USING btree (bill_id);


--
-- Name: idx_bill_split_shares_bill_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_bill_status ON public.bill_split_shares USING btree (bill_id, status);


--
-- Name: idx_bill_split_shares_guest_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_guest_session ON public.bill_split_shares USING btree (guest_session_id);


--
-- Name: idx_bill_split_shares_guest_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_guest_session_id ON public.bill_split_shares USING btree (guest_session_id);


--
-- Name: idx_bill_split_shares_hold_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_hold_idempotency ON public.bill_split_shares USING btree (idempotency_key);


--
-- Name: idx_bill_split_shares_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_payment_id ON public.bill_split_shares USING btree (payment_id);


--
-- Name: idx_bill_split_shares_payment_settled; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_bill_split_shares_payment_settled ON public.bill_split_shares USING btree (bill_id, payment_id) WHERE (((status)::text = 'settled'::text) AND (payment_id IS NOT NULL));


--
-- Name: idx_bill_split_shares_settle_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_split_shares_settle_idempotency ON public.bill_split_shares USING btree (settlement_idempotency_key);


--
-- Name: idx_bills_active_business_created_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_active_business_created_id ON public.bills USING btree (business_id, created_at DESC, id DESC) WHERE (status = ANY (ARRAY['open'::text, 'partial'::text]));


--
-- Name: idx_bills_active_per_counter; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_bills_active_per_counter ON public.bills USING btree (counter_id) WHERE ((counter_id IS NOT NULL) AND (status = ANY (ARRAY['open'::text, 'partial'::text])));


--
-- Name: idx_bills_active_per_table; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_bills_active_per_table ON public.bills USING btree (table_id) WHERE ((table_id IS NOT NULL) AND (table_id <> 0) AND (status = ANY (ARRAY['open'::text, 'partial'::text])));


--
-- Name: idx_bills_active_table_created_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_active_table_created_id ON public.bills USING btree (table_id, created_at DESC, id DESC) WHERE (status = ANY (ARRAY['open'::text, 'partial'::text]));


--
-- Name: idx_bills_bill_number; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_bills_bill_number ON public.bills USING btree (bill_number);


--
-- Name: idx_bills_bill_number_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_bill_number_trgm ON public.bills USING gin (lower(COALESCE(bill_number, ''::text)) public.gin_trgm_ops);


--
-- Name: idx_bills_business_closed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_closed_at ON public.bills USING btree (business_id, closed_at DESC);


--
-- Name: idx_bills_business_closed_staff_closed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_closed_staff_closed_at ON public.bills USING btree (business_id, closed_by_staff_id, closed_at DESC);


--
-- Name: idx_bills_business_counter_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_counter_created_at ON public.bills USING btree (business_id, counter_id, created_at DESC);


--
-- Name: idx_bills_business_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_created_at ON public.bills USING btree (business_id, created_at DESC);


--
-- Name: idx_bills_business_created_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_created_id ON public.bills USING btree (business_id, created_at DESC, id DESC);


--
-- Name: idx_bills_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_id ON public.bills USING btree (business_id);


--
-- Name: idx_bills_business_settled_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_settled_at ON public.bills USING btree (business_id, settled_at DESC) WHERE (settled_at IS NOT NULL);


--
-- Name: idx_bills_business_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_status_created_at ON public.bills USING btree (business_id, status, created_at DESC);


--
-- Name: idx_bills_business_status_created_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_status_created_id ON public.bills USING btree (business_id, status, created_at DESC, id DESC);


--
-- Name: idx_bills_business_table_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_table_created_at ON public.bills USING btree (business_id, table_id, created_at DESC);


--
-- Name: idx_bills_business_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_business_updated_at ON public.bills USING btree (business_id, updated_at DESC);


--
-- Name: idx_bills_closed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_closed_at ON public.bills USING btree (closed_at DESC);


--
-- Name: idx_bills_closed_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_closed_by_staff_id ON public.bills USING btree (closed_by_staff_id);


--
-- Name: idx_bills_counter_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_counter_id ON public.bills USING btree (counter_id);


--
-- Name: idx_bills_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_created_at ON public.bills USING btree (created_at DESC);


--
-- Name: idx_bills_created_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_created_by_staff_id ON public.bills USING btree (created_by_staff_id);


--
-- Name: idx_bills_crm_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_crm_customer_id ON public.bills USING btree (crm_customer_id);


--
-- Name: idx_bills_feedback_email_sent_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_feedback_email_sent_at ON public.bills USING btree (feedback_email_sent_at);


--
-- Name: idx_bills_loyalty_redeemed_by_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_loyalty_redeemed_by_customer_id ON public.bills USING btree (loyalty_redeemed_by_customer_id);


--
-- Name: idx_bills_outstanding_as_of; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_outstanding_as_of ON public.bills USING btree (business_id, created_at DESC) WHERE ((status <> 'voided'::text) AND (total_amount > paid_amount));


--
-- Name: idx_bills_public_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_bills_public_token ON public.bills USING btree (public_token);


--
-- Name: idx_bills_stale_sweep_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_stale_sweep_created_at ON public.bills USING btree (created_at) WHERE ((status = ANY (ARRAY['open'::text, 'partial'::text])) AND (abandoned_at IS NULL));


--
-- Name: idx_bills_table_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_table_id ON public.bills USING btree (table_id);


--
-- Name: idx_bills_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bills_updated_at ON public.bills USING btree (updated_at DESC);


--
-- Name: idx_bundles_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bundles_business_id ON public.bundles USING btree (business_id);


--
-- Name: idx_business_alert_settings_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_business_alert_settings_business_id ON public.business_alert_settings USING btree (business_id);


--
-- Name: idx_business_creation_requests_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_creation_requests_business_id ON public.business_creation_requests USING btree (business_id);


--
-- Name: idx_business_currencies_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_currencies_business_id ON public.business_currencies USING btree (business_id);


--
-- Name: idx_business_fiscal_settings_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_fiscal_settings_business_id ON public.business_fiscal_settings USING btree (business_id);


--
-- Name: idx_business_fiscal_settings_credentials_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_fiscal_settings_credentials_expiry ON public.business_fiscal_settings USING btree (credentials_expires_at) WHERE (credentials_expires_at IS NOT NULL);


--
-- Name: idx_business_fiscal_settings_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_fiscal_settings_status ON public.business_fiscal_settings USING btree (business_id, setup_status);


--
-- Name: idx_business_fiscal_settings_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_business_fiscal_settings_unique ON public.business_fiscal_settings USING btree (business_id, country, provider);


--
-- Name: idx_business_gallery_images_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_gallery_images_business_id ON public.business_gallery_images USING btree (business_id);


--
-- Name: idx_business_languages_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_languages_business_id ON public.business_languages USING btree (business_id);


--
-- Name: idx_business_milestone_events_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_milestone_events_bill_id ON public.business_milestone_events USING btree (bill_id);


--
-- Name: idx_business_milestone_events_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_milestone_events_business_id ON public.business_milestone_events USING btree (business_id);


--
-- Name: idx_business_milestone_events_processing_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_milestone_events_processing_token ON public.business_milestone_events USING btree (processing_token);


--
-- Name: idx_business_milestone_events_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_milestone_events_status ON public.business_milestone_events USING btree (status);


--
-- Name: idx_business_milestone_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_milestone_pending ON public.business_milestone_events USING btree (business_id, status, created_at);


--
-- Name: idx_business_milestone_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_milestone_status_created_at ON public.business_milestone_events USING btree (status, created_at);


--
-- Name: idx_business_milestone_status_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_milestone_status_updated_at ON public.business_milestone_events USING btree (status, updated_at);


--
-- Name: idx_business_milestone_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_business_milestone_unique ON public.business_milestone_events USING btree (business_id, milestone_type, threshold_cents);


--
-- Name: idx_business_operating_exceptions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_operating_exceptions_business_id ON public.business_operating_exceptions USING btree (business_id);


--
-- Name: idx_business_operating_exceptions_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_operating_exceptions_date ON public.business_operating_exceptions USING btree (business_id, exception_date);


--
-- Name: idx_business_operating_hours_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_operating_hours_business_id ON public.business_operating_hours USING btree (business_id);


--
-- Name: idx_business_plugins_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_plugins_business_id ON public.business_plugins USING btree (business_id);


--
-- Name: idx_business_plugins_plugin_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_plugins_plugin_id ON public.business_plugins USING btree (plugin_id);


--
-- Name: idx_business_revenue_aggregates_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_business_revenue_aggregates_business_id ON public.business_revenue_aggregates USING btree (business_id);


--
-- Name: idx_business_schedule_settings_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_business_schedule_settings_business_id ON public.business_schedule_settings USING btree (business_id);


--
-- Name: idx_business_special_features_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_special_features_business_id ON public.business_special_features USING btree (business_id);


--
-- Name: idx_businesses_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_businesses_business_id ON public.businesses USING btree (business_id);


--
-- Name: idx_businesses_business_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_business_type ON public.businesses USING btree (business_type);


--
-- Name: idx_businesses_custom_url_lower; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_businesses_custom_url_lower ON public.businesses USING btree (lower(custom_url)) WHERE ((custom_url IS NOT NULL) AND (custom_url <> ''::text));


--
-- Name: idx_businesses_demo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_demo ON public.businesses USING btree (is_demo);


--
-- Name: idx_businesses_demo_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_demo_owner ON public.businesses USING btree (demo_owner_user_id);


--
-- Name: idx_businesses_kind; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_kind ON public.businesses USING btree (kind);


--
-- Name: idx_businesses_onboarding_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_onboarding_pending ON public.businesses USING btree (id) WHERE (onboarding_completed_at IS NULL);


--
-- Name: idx_businesses_owner_address; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_owner_address ON public.businesses USING btree (owner_address);


--
-- Name: idx_businesses_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_user_id ON public.businesses USING btree (user_id);


--
-- Name: idx_cash_register_movements_actor_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_movements_actor_staff_id ON public.cash_register_movements USING btree (actor_staff_id);


--
-- Name: idx_cash_register_movements_actor_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_movements_actor_user_id ON public.cash_register_movements USING btree (actor_user_id);


--
-- Name: idx_cash_register_movements_alt_payment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_movements_alt_payment ON public.cash_register_movements USING btree (alternative_payment_id) WHERE (alternative_payment_id IS NOT NULL);


--
-- Name: idx_cash_register_movements_alt_payment_type_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_cash_register_movements_alt_payment_type_unique ON public.cash_register_movements USING btree (movement_type, alternative_payment_id) WHERE (alternative_payment_id IS NOT NULL);


--
-- Name: idx_cash_register_movements_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_movements_bill_id ON public.cash_register_movements USING btree (bill_id);


--
-- Name: idx_cash_register_movements_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_movements_business_id ON public.cash_register_movements USING btree (business_id);


--
-- Name: idx_cash_register_movements_business_occurred; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_movements_business_occurred ON public.cash_register_movements USING btree (business_id, occurred_at DESC);


--
-- Name: idx_cash_register_movements_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_movements_session_id ON public.cash_register_movements USING btree (session_id);


--
-- Name: idx_cash_register_movements_session_occurred; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_movements_session_occurred ON public.cash_register_movements USING btree (session_id, occurred_at DESC);


--
-- Name: idx_cash_register_sessions_business_closed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_sessions_business_closed_at ON public.cash_register_sessions USING btree (business_id, closed_at DESC);


--
-- Name: idx_cash_register_sessions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_sessions_business_id ON public.cash_register_sessions USING btree (business_id);


--
-- Name: idx_cash_register_sessions_business_opened_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_sessions_business_opened_at ON public.cash_register_sessions USING btree (business_id, opened_at DESC);


--
-- Name: idx_cash_register_sessions_closed_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_sessions_closed_by_staff_id ON public.cash_register_sessions USING btree (closed_by_staff_id);


--
-- Name: idx_cash_register_sessions_closed_by_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_sessions_closed_by_user_id ON public.cash_register_sessions USING btree (closed_by_user_id);


--
-- Name: idx_cash_register_sessions_one_open_per_business; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_cash_register_sessions_one_open_per_business ON public.cash_register_sessions USING btree (business_id) WHERE ((status)::text = 'open'::text);


--
-- Name: idx_cash_register_sessions_opened_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_sessions_opened_by_staff_id ON public.cash_register_sessions USING btree (opened_by_staff_id);


--
-- Name: idx_cash_register_sessions_opened_by_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cash_register_sessions_opened_by_user_id ON public.cash_register_sessions USING btree (opened_by_user_id);


--
-- Name: idx_chat_channels_biz; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chat_channels_biz ON public.chat_channels USING btree (business_id);


--
-- Name: idx_chat_channels_refkey; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_chat_channels_refkey ON public.chat_channels USING btree (business_id, ref_key) WHERE (ref_key <> ''::text);


--
-- Name: idx_chat_members_biz; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chat_members_biz ON public.chat_channel_members USING btree (business_id);


--
-- Name: idx_chat_messages_cursor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chat_messages_cursor ON public.chat_messages USING btree (business_id, channel_id, created_at);


--
-- Name: idx_chat_messages_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chat_messages_deleted ON public.chat_messages USING btree (deleted_at);


--
-- Name: idx_chat_reads_biz; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chat_reads_biz ON public.chat_reads USING btree (business_id);


--
-- Name: idx_checklist_item_completions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_item_completions_business_id ON public.checklist_item_completions USING btree (business_id);


--
-- Name: idx_checklist_items_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_items_business_id ON public.checklist_items USING btree (business_id);


--
-- Name: idx_checklist_items_template_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_items_template_id ON public.checklist_items USING btree (template_id);


--
-- Name: idx_checklist_runs_assigned; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_runs_assigned ON public.checklist_runs USING btree (assigned_staff_id);


--
-- Name: idx_checklist_runs_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_runs_business_id ON public.checklist_runs USING btree (business_id);


--
-- Name: idx_checklist_runs_shift; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_runs_shift ON public.checklist_runs USING btree (shift_id);


--
-- Name: idx_checklist_runs_template_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_runs_template_id ON public.checklist_runs USING btree (template_id);


--
-- Name: idx_checklist_templates_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_templates_business_id ON public.checklist_templates USING btree (business_id);


--
-- Name: idx_comp_void_audit_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comp_void_audit_business_created ON public.comp_void_audit USING btree (business_id, created_at DESC);


--
-- Name: idx_comp_void_audit_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comp_void_audit_business_id ON public.comp_void_audit USING btree (business_id);


--
-- Name: idx_comp_void_audit_staff; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comp_void_audit_staff ON public.comp_void_audit USING btree (staff_id, created_at DESC);


--
-- Name: idx_comp_void_audit_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comp_void_audit_staff_id ON public.comp_void_audit USING btree (staff_id);


--
-- Name: idx_comp_void_audit_target; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comp_void_audit_target ON public.comp_void_audit USING btree (target_type, target_id);


--
-- Name: idx_conversion_events_conversion_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversion_events_conversion_type ON public.conversion_events USING btree (conversion_type);


--
-- Name: idx_conversion_events_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversion_events_session_id ON public.conversion_events USING btree (session_id);


--
-- Name: idx_conversion_events_timestamp; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversion_events_timestamp ON public.conversion_events USING btree ("timestamp");


--
-- Name: idx_counters_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_counters_active ON public.counters USING btree (business_id, is_active);


--
-- Name: idx_counters_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_counters_business_id ON public.counters USING btree (business_id);


--
-- Name: idx_counters_current_bill; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_counters_current_bill ON public.counters USING btree (current_bill_id);


--
-- Name: idx_crypto_payment_quotes_active_amount; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_crypto_payment_quotes_active_amount ON public.crypto_payment_quotes USING btree (chain_id, settlement_address, exact_microunits) WHERE ((status)::text = 'active'::text);


--
-- Name: idx_crypto_payment_quotes_bill_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_crypto_payment_quotes_bill_status ON public.crypto_payment_quotes USING btree (bill_id, status);


--
-- Name: idx_crypto_payment_quotes_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_crypto_payment_quotes_business_id ON public.crypto_payment_quotes USING btree (business_id);


--
-- Name: idx_crypto_payment_quotes_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_crypto_payment_quotes_payment_id ON public.crypto_payment_quotes USING btree (payment_id) WHERE (payment_id IS NOT NULL);


--
-- Name: idx_crypto_payment_quotes_wallet_amount; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_crypto_payment_quotes_wallet_amount ON public.crypto_payment_quotes USING btree (chain_id, settlement_address, exact_microunits, expires_at);


--
-- Name: idx_customer_addresses_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_addresses_business_id ON public.customer_addresses USING btree (business_id);


--
-- Name: idx_customer_addresses_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_addresses_customer_id ON public.customer_addresses USING btree (customer_id);


--
-- Name: idx_customer_addresses_is_default; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_addresses_is_default ON public.customer_addresses USING btree (is_default);


--
-- Name: idx_customer_business_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_customer_business_unique ON public.customer_businesses USING btree (customer_id, business_id);


--
-- Name: idx_customer_businesses_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_businesses_business_id ON public.customer_businesses USING btree (business_id);


--
-- Name: idx_customer_businesses_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_businesses_customer_id ON public.customer_businesses USING btree (customer_id);


--
-- Name: idx_customer_businesses_loyalty_points; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_businesses_loyalty_points ON public.customer_businesses USING btree (loyalty_points);


--
-- Name: idx_customer_businesses_total_spent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_businesses_total_spent ON public.customer_businesses USING btree (total_spent);


--
-- Name: idx_customer_communications_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_communications_business_id ON public.customer_communications USING btree (business_id);


--
-- Name: idx_customer_communications_campaign_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_communications_campaign_id ON public.customer_communications USING btree (campaign_id);


--
-- Name: idx_customer_communications_customer_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_communications_customer_business_id ON public.customer_communications USING btree (customer_business_id);


--
-- Name: idx_customer_communications_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_communications_status ON public.customer_communications USING btree (status);


--
-- Name: idx_customer_preferences_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_customer_preferences_customer_id ON public.customer_preferences USING btree (customer_id);


--
-- Name: idx_customer_preferences_customer_id_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_customer_preferences_customer_id_unique ON public.customer_preferences USING btree (customer_id);


--
-- Name: idx_customer_visits_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_visits_bill_id ON public.customer_visits USING btree (bill_id);


--
-- Name: idx_customer_visits_customer_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_visits_customer_business_id ON public.customer_visits USING btree (customer_business_id);


--
-- Name: idx_customer_visits_table_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_visits_table_id ON public.customer_visits USING btree (table_id);


--
-- Name: idx_customer_visits_unique_bill; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_customer_visits_unique_bill ON public.customer_visits USING btree (customer_business_id, bill_id) WHERE (bill_id IS NOT NULL);


--
-- Name: idx_customer_visits_visit_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_visits_visit_date ON public.customer_visits USING btree (visit_date);


--
-- Name: idx_customers_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_customers_email ON public.customers USING btree (email);


--
-- Name: idx_customers_wallet_address; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customers_wallet_address ON public.customers USING btree (wallet_address);


--
-- Name: idx_delivery_drivers_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_drivers_business_id ON public.delivery_drivers USING btree (business_id);


--
-- Name: idx_delivery_drivers_is_available; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_drivers_is_available ON public.delivery_drivers USING btree (is_available);


--
-- Name: idx_delivery_drivers_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_drivers_staff_id ON public.delivery_drivers USING btree (staff_id);


--
-- Name: idx_delivery_drivers_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_drivers_status ON public.delivery_drivers USING btree (status);


--
-- Name: idx_delivery_orders_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_bill_id ON public.delivery_orders USING btree (bill_id);


--
-- Name: idx_delivery_orders_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_business_id ON public.delivery_orders USING btree (business_id);


--
-- Name: idx_delivery_orders_business_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_business_status_created_at ON public.delivery_orders USING btree (business_id, status, created_at DESC);


--
-- Name: idx_delivery_orders_business_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_business_updated_at ON public.delivery_orders USING btree (business_id, updated_at DESC);


--
-- Name: idx_delivery_orders_claimed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_claimed_at ON public.delivery_orders USING btree (business_id, claimed_at) WHERE (claimed_at IS NOT NULL);


--
-- Name: idx_delivery_orders_claimed_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_claimed_by ON public.delivery_orders USING btree (claimed_by_staff_id);


--
-- Name: idx_delivery_orders_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_created_at ON public.delivery_orders USING btree (created_at);


--
-- Name: idx_delivery_orders_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_customer_id ON public.delivery_orders USING btree (customer_id);


--
-- Name: idx_delivery_orders_delivery_number; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_delivery_orders_delivery_number ON public.delivery_orders USING btree (delivery_number);


--
-- Name: idx_delivery_orders_driver_business_assigned_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_driver_business_assigned_created_at ON public.delivery_orders USING btree (driver_id, business_id, assigned_at DESC, created_at DESC);


--
-- Name: idx_delivery_orders_driver_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_driver_id ON public.delivery_orders USING btree (driver_id);


--
-- Name: idx_delivery_orders_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_order_id ON public.delivery_orders USING btree (order_id);


--
-- Name: idx_delivery_orders_payment_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_payment_expiry ON public.delivery_orders USING btree (status, payment_expires_at) WHERE (payment_expires_at IS NOT NULL);


--
-- Name: idx_delivery_orders_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_status ON public.delivery_orders USING btree (status);


--
-- Name: idx_delivery_orders_zone_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_orders_zone_id ON public.delivery_orders USING btree (zone_id);


--
-- Name: idx_delivery_settings_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_settings_business_id ON public.delivery_settings USING btree (business_id);


--
-- Name: idx_delivery_status_history_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_status_history_created_at ON public.delivery_status_history USING btree (created_at);


--
-- Name: idx_delivery_status_history_delivery_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_status_history_delivery_order_id ON public.delivery_status_history USING btree (delivery_order_id);


--
-- Name: idx_delivery_zones_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_zones_business_id ON public.delivery_zones USING btree (business_id);


--
-- Name: idx_delivery_zones_is_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_zones_is_active ON public.delivery_zones USING btree (is_active);


--
-- Name: idx_demo_instances_admin_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_demo_instances_admin_user_id ON public.demo_instances USING btree (admin_user_id);


--
-- Name: idx_demo_instances_primary_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_demo_instances_primary_business_id ON public.demo_instances USING btree (primary_business_id);


--
-- Name: idx_demo_instances_secondary_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_demo_instances_secondary_business_id ON public.demo_instances USING btree (secondary_business_id);


--
-- Name: idx_demo_runs_admin_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_demo_runs_admin_user_id ON public.demo_runs USING btree (admin_user_id);


--
-- Name: idx_demo_runs_instance_started; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_demo_runs_instance_started ON public.demo_runs USING btree (demo_instance_id, started_at DESC);


--
-- Name: idx_director_action_audits_actor_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_action_audits_actor_user_id ON public.director_action_audits USING btree (actor_user_id);


--
-- Name: idx_director_action_audits_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_action_audits_business ON public.director_action_audits USING btree (business_id);


--
-- Name: idx_director_action_audits_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_action_audits_business_id ON public.director_action_audits USING btree (business_id);


--
-- Name: idx_director_action_audits_proposal; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_action_audits_proposal ON public.director_action_audits USING btree (proposed_action_id);


--
-- Name: idx_director_action_audits_proposed_action_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_action_audits_proposed_action_id ON public.director_action_audits USING btree (proposed_action_id);


--
-- Name: idx_director_action_audits_thread_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_action_audits_thread_id ON public.director_action_audits USING btree (thread_id);


--
-- Name: idx_director_console_messages_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_console_messages_business_id ON public.director_console_messages USING btree (business_id);


--
-- Name: idx_director_console_messages_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_console_messages_created_at ON public.director_console_messages USING btree (created_at);


--
-- Name: idx_director_console_messages_thread_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_console_messages_thread_history ON public.director_console_messages USING btree (business_id, thread_id, created_at DESC);


--
-- Name: idx_director_console_messages_thread_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_console_messages_thread_id ON public.director_console_messages USING btree (thread_id);


--
-- Name: idx_director_console_threads_archived_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_console_threads_archived_at ON public.director_console_threads USING btree (archived_at);


--
-- Name: idx_director_console_threads_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_console_threads_business_id ON public.director_console_threads USING btree (business_id);


--
-- Name: idx_director_console_threads_last_message_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_console_threads_last_message_at ON public.director_console_threads USING btree (last_message_at);


--
-- Name: idx_director_console_threads_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_console_threads_updated_at ON public.director_console_threads USING btree (updated_at DESC);


--
-- Name: idx_director_proposed_actions_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_proposed_actions_business ON public.director_proposed_actions USING btree (business_id);


--
-- Name: idx_director_proposed_actions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_proposed_actions_business_id ON public.director_proposed_actions USING btree (business_id);


--
-- Name: idx_director_proposed_actions_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_proposed_actions_created_at ON public.director_proposed_actions USING btree (created_at);


--
-- Name: idx_director_proposed_actions_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_proposed_actions_expires_at ON public.director_proposed_actions USING btree (expires_at);


--
-- Name: idx_director_proposed_actions_message_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_proposed_actions_message_id ON public.director_proposed_actions USING btree (message_id);


--
-- Name: idx_director_proposed_actions_public_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_director_proposed_actions_public_id ON public.director_proposed_actions USING btree (public_id);


--
-- Name: idx_director_proposed_actions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_proposed_actions_status ON public.director_proposed_actions USING btree (status);


--
-- Name: idx_director_proposed_actions_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_proposed_actions_thread ON public.director_proposed_actions USING btree (thread_id);


--
-- Name: idx_director_proposed_actions_thread_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_proposed_actions_thread_id ON public.director_proposed_actions USING btree (thread_id);


--
-- Name: idx_director_threads_archived; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_threads_archived ON public.director_console_threads USING btree (business_id, archived_at);


--
-- Name: idx_director_tool_calls_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_tool_calls_business_created ON public.director_tool_calls USING btree (business_id, created_at DESC);


--
-- Name: idx_director_tool_calls_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_tool_calls_business_id ON public.director_tool_calls USING btree (business_id);


--
-- Name: idx_director_tool_calls_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_tool_calls_deleted_at ON public.director_tool_calls USING btree (deleted_at);


--
-- Name: idx_director_tool_calls_message_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_tool_calls_message_id ON public.director_tool_calls USING btree (message_id);


--
-- Name: idx_director_tool_calls_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_tool_calls_thread ON public.director_tool_calls USING btree (thread_id);


--
-- Name: idx_director_tool_calls_thread_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_tool_calls_thread_id ON public.director_tool_calls USING btree (thread_id);


--
-- Name: idx_director_tool_calls_tool_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_tool_calls_tool_created ON public.director_tool_calls USING btree (tool_name, created_at DESC);


--
-- Name: idx_director_tool_calls_tool_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_director_tool_calls_tool_name ON public.director_tool_calls USING btree (tool_name);


--
-- Name: idx_doc_ack; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_doc_ack ON public.document_acks USING btree (document_id, staff_id, version);


--
-- Name: idx_document_acks_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_acks_business_id ON public.document_acks USING btree (business_id);


--
-- Name: idx_documents_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_documents_business_id ON public.documents USING btree (business_id);


--
-- Name: idx_email_delivery_events_provider_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_delivery_events_provider_email ON public.email_delivery_events USING btree (provider, provider_email_id) WHERE ((provider_email_id)::text <> ''::text);


--
-- Name: idx_email_delivery_events_type_received; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_delivery_events_type_received ON public.email_delivery_events USING btree (event_type, received_at DESC);


--
-- Name: idx_email_outbound_sends_status_sent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbound_sends_status_sent ON public.email_outbound_sends USING btree (status, sent_at DESC);


--
-- Name: idx_email_outbound_sends_template_sent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbound_sends_template_sent ON public.email_outbound_sends USING btree (template_name, sent_at DESC);


--
-- Name: idx_email_outbox_bill; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbox_bill ON public.email_outbox USING btree (bill_id) WHERE (bill_id IS NOT NULL);


--
-- Name: idx_email_outbox_business_delivery; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbox_business_delivery ON public.email_outbox USING btree (business_id, delivery_status) WHERE (business_id IS NOT NULL);


--
-- Name: idx_email_outbox_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbox_created ON public.email_outbox USING btree (created_at);


--
-- Name: idx_email_outbox_delivery_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbox_delivery_order ON public.email_outbox USING btree (delivery_order_id) WHERE (delivery_order_id IS NOT NULL);


--
-- Name: idx_email_outbox_due; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbox_due ON public.email_outbox USING btree (status, next_attempt_at);


--
-- Name: idx_email_outbox_processing; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbox_processing ON public.email_outbox USING btree (status, last_attempt_at);


--
-- Name: idx_email_outbox_provider_msg; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbox_provider_msg ON public.email_outbox USING btree (provider, provider_message_id);


--
-- Name: idx_email_outbox_reservation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_outbox_reservation ON public.email_outbox USING btree (reservation_id) WHERE (reservation_id IS NOT NULL);


--
-- Name: idx_email_suppressions_reason_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_suppressions_reason_time ON public.email_suppressions USING btree (reason, suppressed_at DESC);


--
-- Name: idx_error_logs_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_error_logs_created_at ON public.error_logs USING btree (created_at);


--
-- Name: idx_error_logs_request_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_error_logs_request_id ON public.error_logs USING btree (request_id);


--
-- Name: idx_error_logs_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_error_logs_source ON public.error_logs USING btree (source);


--
-- Name: idx_error_logs_timestamp; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_error_logs_timestamp ON public.error_logs USING btree ("timestamp");


--
-- Name: idx_error_logs_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_error_logs_user_id ON public.error_logs USING btree (user_id);


--
-- Name: idx_escalations_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_escalations_business ON public.escalations USING btree (business_id);


--
-- Name: idx_escalations_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_escalations_business_id ON public.escalations USING btree (business_id);


--
-- Name: idx_escalations_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_escalations_created_at ON public.escalations USING btree (created_at);


--
-- Name: idx_escalations_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_escalations_source ON public.escalations USING btree (source);


--
-- Name: idx_escalations_source_biz_request; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_escalations_source_biz_request ON public.escalations USING btree (source, business_id, request_id) WHERE ((request_id IS NOT NULL) AND ((request_id)::text <> ''::text));


--
-- Name: idx_escalations_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_escalations_status ON public.escalations USING btree (status);


--
-- Name: idx_exchange_rates_last_seen_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_exchange_rates_last_seen_at ON public.exchange_rates USING btree (last_seen_at);


--
-- Name: idx_exchange_rates_pair_fetched; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_exchange_rates_pair_fetched ON public.exchange_rates USING btree (from_currency, to_currency, fetched_at DESC);


--
-- Name: idx_fiscal_audit_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_audit_business ON public.fiscal_audit_events USING btree (business_id, created_at DESC);


--
-- Name: idx_fiscal_audit_events_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_audit_events_business_id ON public.fiscal_audit_events USING btree (business_id);


--
-- Name: idx_fiscal_audit_events_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_audit_events_job_id ON public.fiscal_audit_events USING btree (job_id);


--
-- Name: idx_fiscal_audit_events_receipt_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_audit_events_receipt_id ON public.fiscal_audit_events USING btree (receipt_id);


--
-- Name: idx_fiscal_delivery_tasks_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_delivery_tasks_business_id ON public.fiscal_delivery_tasks USING btree (business_id);


--
-- Name: idx_fiscal_delivery_tasks_claim; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_delivery_tasks_claim ON public.fiscal_delivery_tasks USING btree (status, next_attempt_at);


--
-- Name: idx_fiscal_delivery_tasks_next_attempt; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_delivery_tasks_next_attempt ON public.fiscal_delivery_tasks USING btree (next_attempt_at) WHERE (next_attempt_at IS NOT NULL);


--
-- Name: idx_fiscal_delivery_tasks_receipt_channel; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_fiscal_delivery_tasks_receipt_channel ON public.fiscal_delivery_tasks USING btree (receipt_id, channel);


--
-- Name: idx_fiscal_delivery_tasks_receipt_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_delivery_tasks_receipt_id ON public.fiscal_delivery_tasks USING btree (receipt_id);


--
-- Name: idx_fiscal_jobs_alternative_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_alternative_payment_id ON public.fiscal_jobs USING btree (alternative_payment_id);


--
-- Name: idx_fiscal_jobs_bill; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_bill ON public.fiscal_jobs USING btree (bill_id);


--
-- Name: idx_fiscal_jobs_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_bill_id ON public.fiscal_jobs USING btree (bill_id);


--
-- Name: idx_fiscal_jobs_bill_issue_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_fiscal_jobs_bill_issue_unique ON public.fiscal_jobs USING btree (bill_id, action) WHERE ((action)::text = 'issue_receipt'::text);


--
-- Name: idx_fiscal_jobs_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_business_id ON public.fiscal_jobs USING btree (business_id);


--
-- Name: idx_fiscal_jobs_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_business_status ON public.fiscal_jobs USING btree (business_id, status);


--
-- Name: idx_fiscal_jobs_claim; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_claim ON public.fiscal_jobs USING btree (status, next_attempt_at);


--
-- Name: idx_fiscal_jobs_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_fiscal_jobs_idempotency ON public.fiscal_jobs USING btree (idempotency_key);


--
-- Name: idx_fiscal_jobs_next_attempt; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_next_attempt ON public.fiscal_jobs USING btree (next_attempt_at) WHERE (next_attempt_at IS NOT NULL);


--
-- Name: idx_fiscal_jobs_next_attempt_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_next_attempt_at ON public.fiscal_jobs USING btree (next_attempt_at);


--
-- Name: idx_fiscal_jobs_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_payment_id ON public.fiscal_jobs USING btree (payment_id);


--
-- Name: idx_fiscal_jobs_produced_receipt_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_produced_receipt_id ON public.fiscal_jobs USING btree (produced_receipt_id);


--
-- Name: idx_fiscal_jobs_receipt_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_receipt_id ON public.fiscal_jobs USING btree (receipt_id);


--
-- Name: idx_fiscal_jobs_settings_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_jobs_settings_id ON public.fiscal_jobs USING btree (settings_id);


--
-- Name: idx_fiscal_receipts_alternative_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_alternative_payment_id ON public.fiscal_receipts USING btree (alternative_payment_id);


--
-- Name: idx_fiscal_receipts_bill; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_bill ON public.fiscal_receipts USING btree (bill_id);


--
-- Name: idx_fiscal_receipts_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_bill_id ON public.fiscal_receipts USING btree (bill_id);


--
-- Name: idx_fiscal_receipts_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_business_created ON public.fiscal_receipts USING btree (business_id, created_at DESC);


--
-- Name: idx_fiscal_receipts_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_business_id ON public.fiscal_receipts USING btree (business_id);


--
-- Name: idx_fiscal_receipts_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_business_status ON public.fiscal_receipts USING btree (business_id, status);


--
-- Name: idx_fiscal_receipts_business_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_business_status_created_at ON public.fiscal_receipts USING btree (business_id, status, created_at DESC);


--
-- Name: idx_fiscal_receipts_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_payment_id ON public.fiscal_receipts USING btree (payment_id);


--
-- Name: idx_fiscal_receipts_provider_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_provider_id ON public.fiscal_receipts USING btree (provider, provider_receipt_id) WHERE (provider_receipt_id IS NOT NULL);


--
-- Name: idx_fiscal_receipts_settings_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_settings_id ON public.fiscal_receipts USING btree (settings_id);


--
-- Name: idx_fiscal_receipts_undelivered; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscal_receipts_undelivered ON public.fiscal_receipts USING btree (issued_at) WHERE (((status)::text = 'authorized'::text) AND (delivered_at IS NULL));


--
-- Name: idx_fiscal_receipts_unique_number; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_fiscal_receipts_unique_number ON public.fiscal_receipts USING btree (business_id, country, provider, receipt_type, receipt_number) WHERE (receipt_number IS NOT NULL);


--
-- Name: idx_gallery_images_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gallery_images_active ON public.business_gallery_images USING btree (business_id, is_active);


--
-- Name: idx_gallery_images_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gallery_images_business_id ON public.business_gallery_images USING btree (business_id);


--
-- Name: idx_gallery_images_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gallery_images_order ON public.business_gallery_images USING btree (business_id, display_order);


--
-- Name: idx_guest_receipt_sends_bill_sent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_guest_receipt_sends_bill_sent ON public.guest_receipt_sends USING btree (bill_id, sent_at);


--
-- Name: idx_inventory_alert_logs_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_alert_logs_business_id ON public.inventory_alert_logs USING btree (business_id);


--
-- Name: idx_inventory_alert_logs_dedup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_alert_logs_dedup ON public.inventory_alert_logs USING btree (business_id, inventory_item_id, alert_type, alerted_at);


--
-- Name: idx_inventory_alert_logs_inventory_item_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_alert_logs_inventory_item_id ON public.inventory_alert_logs USING btree (inventory_item_id);


--
-- Name: idx_inventory_items_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_items_active ON public.inventory_items USING btree (business_id, is_active);


--
-- Name: idx_inventory_items_biz_sku; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_inventory_items_biz_sku ON public.inventory_items USING btree (business_id, sku) WHERE ((sku IS NOT NULL) AND (sku <> ''::text));


--
-- Name: idx_inventory_items_business_active_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_items_business_active_name ON public.inventory_items USING btree (business_id, is_active, name);


--
-- Name: idx_inventory_items_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_items_business_id ON public.inventory_items USING btree (business_id);


--
-- Name: idx_inventory_items_sku; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_items_sku ON public.inventory_items USING btree (sku);


--
-- Name: idx_inventory_movements_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_bill_id ON public.inventory_movements USING btree (reference_bill_id);


--
-- Name: idx_inventory_movements_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_business_created ON public.inventory_movements USING btree (business_id, created_at DESC);


--
-- Name: idx_inventory_movements_business_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_business_created_at ON public.inventory_movements USING btree (business_id, created_at DESC);


--
-- Name: idx_inventory_movements_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_business_id ON public.inventory_movements USING btree (business_id);


--
-- Name: idx_inventory_movements_business_item_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_business_item_created ON public.inventory_movements USING btree (business_id, inventory_item_id, created_at DESC);


--
-- Name: idx_inventory_movements_business_order_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_business_order_type ON public.inventory_movements USING btree (business_id, reference_order_id, movement_type);


--
-- Name: idx_inventory_movements_inventory_item_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_inventory_item_id ON public.inventory_movements USING btree (inventory_item_id);


--
-- Name: idx_inventory_movements_movement_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_movement_type ON public.inventory_movements USING btree (movement_type);


--
-- Name: idx_inventory_movements_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_order_id ON public.inventory_movements USING btree (reference_order_id);


--
-- Name: idx_inventory_movements_reference_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_reference_bill_id ON public.inventory_movements USING btree (reference_bill_id);


--
-- Name: idx_inventory_movements_reference_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_reference_order_id ON public.inventory_movements USING btree (reference_order_id);


--
-- Name: idx_inventory_recipes_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_recipes_business_id ON public.inventory_recipes USING btree (business_id);


--
-- Name: idx_inventory_recipes_business_inventory_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_recipes_business_inventory_item ON public.inventory_recipes USING btree (business_id, inventory_item_id);


--
-- Name: idx_inventory_recipes_business_menu_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_recipes_business_menu_item ON public.inventory_recipes USING btree (business_id, menu_item_id);


--
-- Name: idx_inventory_recipes_inventory_item_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_recipes_inventory_item_id ON public.inventory_recipes USING btree (inventory_item_id);


--
-- Name: idx_inventory_recipes_menu_item_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_recipes_menu_item_id ON public.inventory_recipes USING btree (menu_item_id);


--
-- Name: idx_inventory_settings_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_inventory_settings_business_id ON public.inventory_settings USING btree (business_id);


--
-- Name: idx_ledger_entry_attachments_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ledger_entry_attachments_business ON public.ledger_entry_attachments USING btree (business_id, created_at DESC);


--
-- Name: idx_ledger_entry_attachments_entry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ledger_entry_attachments_entry ON public.ledger_entry_attachments USING btree (entry_id);


--
-- Name: idx_loyalty_programs_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_loyalty_programs_business_id ON public.loyalty_programs USING btree (business_id);


--
-- Name: idx_loyalty_tiers_loyalty_program_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loyalty_tiers_loyalty_program_id ON public.loyalty_tiers USING btree (loyalty_program_id);


--
-- Name: idx_manual_ledger_entries_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_business_id ON public.manual_ledger_entries USING btree (business_id);


--
-- Name: idx_manual_ledger_entries_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_category ON public.manual_ledger_entries USING btree (category);


--
-- Name: idx_manual_ledger_entries_created_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_created_by_staff_id ON public.manual_ledger_entries USING btree (created_by_staff_id);


--
-- Name: idx_manual_ledger_entries_created_by_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_created_by_user_id ON public.manual_ledger_entries USING btree (created_by_user_id);


--
-- Name: idx_manual_ledger_entries_entry_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_entry_type ON public.manual_ledger_entries USING btree (entry_type);


--
-- Name: idx_manual_ledger_entries_occurred_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_occurred_at ON public.manual_ledger_entries USING btree (occurred_at);


--
-- Name: idx_manual_ledger_entries_recurring_ref; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_manual_ledger_entries_recurring_ref ON public.manual_ledger_entries USING btree (business_id, reference) WHERE (reference ~~ 'recurring:%'::text);


--
-- Name: idx_manual_ledger_entries_voided_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_voided_at ON public.manual_ledger_entries USING btree (voided_at);


--
-- Name: idx_manual_ledger_entries_voided_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_voided_by_staff_id ON public.manual_ledger_entries USING btree (voided_by_staff_id);


--
-- Name: idx_manual_ledger_entries_voided_by_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_manual_ledger_entries_voided_by_user_id ON public.manual_ledger_entries USING btree (voided_by_user_id);


--
-- Name: idx_marketing_activities_biz_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_marketing_activities_biz_created ON public.marketing_activities USING btree (business_id, created_at DESC, id DESC);


--
-- Name: idx_marketing_activities_biz_dismissed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_marketing_activities_biz_dismissed ON public.marketing_activities USING btree (business_id, suggestion_id) WHERE ((status)::text = 'dismissed'::text);


--
-- Name: idx_marketing_activities_biz_suggestion; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_marketing_activities_biz_suggestion ON public.marketing_activities USING btree (business_id, suggestion_id);


--
-- Name: idx_menu_extraction_images_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menu_extraction_images_job_id ON public.menu_extraction_images USING btree (job_id);


--
-- Name: idx_menu_extraction_jobs_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menu_extraction_jobs_business_id ON public.menu_extraction_jobs USING btree (business_id);


--
-- Name: idx_menu_extraction_jobs_claimable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menu_extraction_jobs_claimable ON public.menu_extraction_jobs USING btree (status, next_attempt_at, id);


--
-- Name: idx_menu_wizard_messages_session_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menu_wizard_messages_session_history ON public.menu_wizard_messages USING btree (session_id, created_at DESC);


--
-- Name: idx_menu_wizard_messages_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menu_wizard_messages_session_id ON public.menu_wizard_messages USING btree (session_id);


--
-- Name: idx_menu_wizard_sessions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menu_wizard_sessions_business_id ON public.menu_wizard_sessions USING btree (business_id);


--
-- Name: idx_menus_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_menus_business_id ON public.menus USING btree (business_id);


--
-- Name: idx_missing_translations_last_seen; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_missing_translations_last_seen ON public.missing_translations USING btree (last_seen_at DESC);


--
-- Name: idx_missing_translations_locale_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_missing_translations_locale_key ON public.missing_translations USING btree (locale, key_path);


--
-- Name: idx_missing_translations_status_last_seen; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_missing_translations_status_last_seen ON public.missing_translations USING btree (status, last_seen_at DESC);


--
-- Name: idx_missing_translations_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_missing_translations_unique ON public.missing_translations USING btree (locale, key_path, page, fallback_used);


--
-- Name: idx_offers_business_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_offers_business_code ON public.offers USING btree (business_id, code) WHERE (code IS NOT NULL);


--
-- Name: idx_offers_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offers_business_id ON public.offers USING btree (business_id);


--
-- Name: idx_open_claims_business_shift; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_open_claims_business_shift ON public.open_shift_claims USING btree (business_id, shift_id);


--
-- Name: idx_open_claims_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_open_claims_business_status ON public.open_shift_claims USING btree (business_id, status);


--
-- Name: idx_open_claims_one_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_open_claims_one_pending ON public.open_shift_claims USING btree (business_id, shift_id, claiming_staff_id) WHERE (status = 'pending'::text);


--
-- Name: idx_operating_hours_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operating_hours_business_id ON public.business_operating_hours USING btree (business_id);


--
-- Name: idx_operating_hours_day; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operating_hours_day ON public.business_operating_hours USING btree (business_id, day_of_week);


--
-- Name: idx_operational_alert_events_actor_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alert_events_actor_staff_id ON public.operational_alert_events USING btree (actor_staff_id);


--
-- Name: idx_operational_alert_events_actor_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alert_events_actor_user_id ON public.operational_alert_events USING btree (actor_user_id);


--
-- Name: idx_operational_alert_events_alert_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alert_events_alert_created ON public.operational_alert_events USING btree (alert_id, created_at);


--
-- Name: idx_operational_alert_events_alert_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alert_events_alert_id ON public.operational_alert_events USING btree (alert_id);


--
-- Name: idx_operational_alert_events_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alert_events_business_created ON public.operational_alert_events USING btree (business_id, created_at DESC);


--
-- Name: idx_operational_alert_events_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alert_events_business_id ON public.operational_alert_events USING btree (business_id);


--
-- Name: idx_operational_alerts_active_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_operational_alerts_active_unique ON public.operational_alerts USING btree (business_id, alert_type, resource_type, resource_id) WHERE (status = ANY (ARRAY['open'::text, 'claimed'::text]));


--
-- Name: idx_operational_alerts_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alerts_business_id ON public.operational_alerts USING btree (business_id);


--
-- Name: idx_operational_alerts_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alerts_business_status ON public.operational_alerts USING btree (business_id, status);


--
-- Name: idx_operational_alerts_claimed_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alerts_claimed_by_staff_id ON public.operational_alerts USING btree (claimed_by_staff_id);


--
-- Name: idx_operational_alerts_claimed_by_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alerts_claimed_by_user_id ON public.operational_alerts USING btree (claimed_by_user_id);


--
-- Name: idx_operational_alerts_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alerts_resource ON public.operational_alerts USING btree (resource_type, resource_id);


--
-- Name: idx_operational_alerts_snoozed_until; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alerts_snoozed_until ON public.operational_alerts USING btree (snoozed_until);


--
-- Name: idx_operational_alerts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operational_alerts_status ON public.operational_alerts USING btree (status);


--
-- Name: idx_ops_assistant_messages_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_messages_business_id ON public.ops_assistant_messages USING btree (business_id);


--
-- Name: idx_ops_assistant_messages_request_role; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_ops_assistant_messages_request_role ON public.ops_assistant_messages USING btree (request_id, role) WHERE (request_id IS NOT NULL);


--
-- Name: idx_ops_assistant_messages_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_messages_thread ON public.ops_assistant_messages USING btree (thread_id, id);


--
-- Name: idx_ops_assistant_messages_thread_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_messages_thread_id ON public.ops_assistant_messages USING btree (thread_id);


--
-- Name: idx_ops_assistant_requests_claimable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_requests_claimable ON public.ops_assistant_requests USING btree (status, claimed_at, id);


--
-- Name: idx_ops_assistant_requests_thread_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_requests_thread_id ON public.ops_assistant_requests USING btree (thread_id);


--
-- Name: idx_ops_assistant_threads_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_threads_business ON public.ops_assistant_threads USING btree (business_id, last_message_at DESC);


--
-- Name: idx_ops_assistant_threads_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_threads_business_id ON public.ops_assistant_threads USING btree (business_id);


--
-- Name: idx_ops_assistant_threads_last_message_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_threads_last_message_at ON public.ops_assistant_threads USING btree (last_message_at);


--
-- Name: idx_ops_assistant_tool_calls_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_tool_calls_business_id ON public.ops_assistant_tool_calls USING btree (business_id);


--
-- Name: idx_ops_assistant_tool_calls_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_tool_calls_thread ON public.ops_assistant_tool_calls USING btree (thread_id, id);


--
-- Name: idx_ops_assistant_tool_calls_thread_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ops_assistant_tool_calls_thread_id ON public.ops_assistant_tool_calls USING btree (thread_id);


--
-- Name: idx_orders_bill_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_bill_created_at ON public.orders USING btree (bill_id, created_at DESC);


--
-- Name: idx_orders_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_bill_id ON public.orders USING btree (bill_id);


--
-- Name: idx_orders_business_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_business_created_at ON public.orders USING btree (business_id, created_at DESC);


--
-- Name: idx_orders_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_business_id ON public.orders USING btree (business_id);


--
-- Name: idx_orders_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_business_status ON public.orders USING btree (business_id, status);


--
-- Name: idx_orders_business_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_business_status_created_at ON public.orders USING btree (business_id, status, created_at DESC);


--
-- Name: idx_orders_delivery_request_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_orders_delivery_request_identity ON public.orders USING btree (business_id, client_request_id) WHERE ((created_by = 'guest_delivery'::text) AND (client_request_id IS NOT NULL));


--
-- Name: idx_orders_kitchen_acked_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_kitchen_acked_at ON public.orders USING btree (kitchen_acked_at);


--
-- Name: idx_orders_kitchen_unacked; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_kitchen_unacked ON public.orders USING btree (business_id, created_at) WHERE (kitchen_acked_at IS NULL);


--
-- Name: idx_orders_request_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_orders_request_identity ON public.orders USING btree (bill_id, created_by, client_request_id);


--
-- Name: idx_page_views_country; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_page_views_country ON public.page_views USING btree (country);


--
-- Name: idx_page_views_page; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_page_views_page ON public.page_views USING btree (page);


--
-- Name: idx_page_views_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_page_views_session_id ON public.page_views USING btree (session_id);


--
-- Name: idx_page_views_timestamp; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_page_views_timestamp ON public.page_views USING btree ("timestamp");


--
-- Name: idx_payment_refund_destinations_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payment_refund_destinations_payment_id ON public.payment_refund_destinations USING btree (payment_id);


--
-- Name: idx_payment_refunds_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_bill_id ON public.payment_refunds USING btree (bill_id);


--
-- Name: idx_payment_refunds_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_business_id ON public.payment_refunds USING btree (business_id);


--
-- Name: idx_payment_refunds_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_business_status ON public.payment_refunds USING btree (business_id, status, created_at DESC);


--
-- Name: idx_payment_refunds_claim; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_claim ON public.payment_refunds USING btree (status, locked_at, updated_at) WHERE ((status)::text = ANY (ARRAY[('submitted'::character varying)::text, ('confirming'::character varying)::text]));


--
-- Name: idx_payment_refunds_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payment_refunds_idempotency ON public.payment_refunds USING btree (business_id, idempotency_key);


--
-- Name: idx_payment_refunds_one_active_per_payment; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payment_refunds_one_active_per_payment ON public.payment_refunds USING btree (payment_id) WHERE ((status)::text <> ALL (ARRAY[('confirmed'::character varying)::text, ('failed'::character varying)::text, ('rejected'::character varying)::text, ('cancelled'::character varying)::text]));


--
-- Name: idx_payment_refunds_payment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_payment_id ON public.payment_refunds USING btree (payment_id);


--
-- Name: idx_payment_refunds_reorg_watch; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_reorg_watch ON public.payment_refunds USING btree (confirmed_at) WHERE (((status)::text = 'confirmed'::text) AND (block_hash IS NOT NULL));


--
-- Name: idx_payment_refunds_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_status ON public.payment_refunds USING btree (status);


--
-- Name: idx_payment_refunds_submitted_tx_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payment_refunds_submitted_tx_hash ON public.payment_refunds USING btree (submitted_tx_hash) WHERE ((submitted_tx_hash IS NOT NULL) AND ((submitted_tx_hash)::text <> ''::text));


--
-- Name: idx_payment_refunds_submitted_tx_hash_evm_canonical; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payment_refunds_submitted_tx_hash_evm_canonical ON public.payment_refunds USING btree (lower("right"((submitted_tx_hash)::text, 64))) WHERE ((submitted_tx_hash)::text ~ '^(0[xX])?[0-9a-fA-F]{64}$'::text);


--
-- Name: idx_payments_bill_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_bill_id ON public.payments USING btree (bill_id);


--
-- Name: idx_payments_bill_payer_guest_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_bill_payer_guest_session ON public.payments USING btree (bill_id, payer_guest_session) WHERE (payer_guest_session IS NOT NULL);


--
-- Name: idx_payments_payer_addr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_payer_addr ON public.payments USING btree (payer_addr);


--
-- Name: idx_payments_reorg_watch; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_reorg_watch ON public.payments USING btree (confirmed_at) WHERE ((status = 'confirmed'::text) AND (block_hash IS NOT NULL));


--
-- Name: idx_payments_status_confirmed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_status_confirmed_at ON public.payments USING btree (status, confirmed_at DESC);


--
-- Name: idx_payments_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_status_created_at ON public.payments USING btree (status, created_at DESC);


--
-- Name: idx_payments_status_reversed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_status_reversed_at ON public.payments USING btree (status, reversed_at DESC);


--
-- Name: idx_payments_status_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payments_status_updated_at ON public.payments USING btree (status, updated_at DESC);


--
-- Name: idx_payments_tx_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payments_tx_hash ON public.payments USING btree (tx_hash);


--
-- Name: idx_payments_tx_hash_evm_canonical; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payments_tx_hash_evm_canonical ON public.payments USING btree (lower("right"(tx_hash, 64))) WHERE (tx_hash ~ '^(0[xX])?[0-9a-fA-F]{64}$'::text);


--
-- Name: idx_payroll_line_items_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_line_items_business_id ON public.payroll_line_items USING btree (business_id);


--
-- Name: idx_payroll_line_items_payee_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_line_items_payee_type ON public.payroll_line_items USING btree (payee_type);


--
-- Name: idx_payroll_line_items_payroll_run_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_line_items_payroll_run_id ON public.payroll_line_items USING btree (payroll_run_id);


--
-- Name: idx_payroll_line_items_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_line_items_staff_id ON public.payroll_line_items USING btree (staff_id);


--
-- Name: idx_payroll_runs_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_business_id ON public.payroll_runs USING btree (business_id);


--
-- Name: idx_payroll_runs_created_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_created_by_staff_id ON public.payroll_runs USING btree (created_by_staff_id);


--
-- Name: idx_payroll_runs_created_by_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_created_by_user_id ON public.payroll_runs USING btree (created_by_user_id);


--
-- Name: idx_payroll_runs_paid_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_paid_at ON public.payroll_runs USING btree (paid_at);


--
-- Name: idx_payroll_runs_paid_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_paid_by_staff_id ON public.payroll_runs USING btree (paid_by_staff_id);


--
-- Name: idx_payroll_runs_paid_by_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_paid_by_user_id ON public.payroll_runs USING btree (paid_by_user_id);


--
-- Name: idx_payroll_runs_period_end; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_period_end ON public.payroll_runs USING btree (period_end);


--
-- Name: idx_payroll_runs_period_start; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_period_start ON public.payroll_runs USING btree (period_start);


--
-- Name: idx_payroll_runs_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_status ON public.payroll_runs USING btree (status);


--
-- Name: idx_payroll_runs_voided_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_voided_at ON public.payroll_runs USING btree (voided_at);


--
-- Name: idx_payroll_runs_voided_by_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_voided_by_staff_id ON public.payroll_runs USING btree (voided_by_staff_id);


--
-- Name: idx_payroll_runs_voided_by_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payroll_runs_voided_by_user_id ON public.payroll_runs USING btree (voided_by_user_id);


--
-- Name: idx_platform_settings_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_platform_settings_category ON public.platform_settings USING btree (category);


--
-- Name: idx_platform_settings_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_platform_settings_key ON public.platform_settings USING btree (key);


--
-- Name: idx_plugin_notification_deliveries_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_deliveries_business ON public.plugin_notification_deliveries USING btree (business_id, plugin_name, created_at DESC);


--
-- Name: idx_plugin_notification_deliveries_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_deliveries_business_id ON public.plugin_notification_deliveries USING btree (business_id);


--
-- Name: idx_plugin_notification_deliveries_claim; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_deliveries_claim ON public.plugin_notification_deliveries USING btree (plugin_name, status, next_attempt_at, id) WHERE (status = ANY (ARRAY['pending'::text, 'retry'::text]));


--
-- Name: idx_plugin_notification_deliveries_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_deliveries_created_at ON public.plugin_notification_deliveries USING btree (created_at);


--
-- Name: idx_plugin_notification_deliveries_next_attempt_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_deliveries_next_attempt_at ON public.plugin_notification_deliveries USING btree (next_attempt_at);


--
-- Name: idx_plugin_notification_deliveries_plugin_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_deliveries_plugin_name ON public.plugin_notification_deliveries USING btree (plugin_name);


--
-- Name: idx_plugin_notification_deliveries_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_deliveries_status ON public.plugin_notification_deliveries USING btree (status);


--
-- Name: idx_plugin_notification_delivery_attempts_attempted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_delivery_attempts_attempted_at ON public.plugin_notification_delivery_attempts USING btree (attempted_at);


--
-- Name: idx_plugin_notification_delivery_attempts_delivery; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_delivery_attempts_delivery ON public.plugin_notification_delivery_attempts USING btree (delivery_id, attempted_at DESC);


--
-- Name: idx_plugin_notification_delivery_attempts_delivery_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_notification_delivery_attempts_delivery_id ON public.plugin_notification_delivery_attempts USING btree (delivery_id);


--
-- Name: idx_plugin_notification_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_plugin_notification_unique ON public.plugin_notification_deliveries USING btree (business_id, plugin_name, event_type, event_id);


--
-- Name: idx_plugin_translations_plugin_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_translations_plugin_id ON public.plugin_translations USING btree (plugin_id);


--
-- Name: idx_plugins_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_plugins_name ON public.plugins USING btree (name);


--
-- Name: idx_poll_one_vote; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_poll_one_vote ON public.poll_votes USING btree (poll_id, staff_id);


--
-- Name: idx_poll_options_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_poll_options_business_id ON public.poll_options USING btree (business_id);


--
-- Name: idx_poll_options_poll_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_poll_options_poll_id ON public.poll_options USING btree (poll_id);


--
-- Name: idx_poll_votes_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_poll_votes_business_id ON public.poll_votes USING btree (business_id);


--
-- Name: idx_poll_votes_option; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_poll_votes_option ON public.poll_votes USING btree (option_id);


--
-- Name: idx_polls_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_polls_business_id ON public.polls USING btree (business_id);


--
-- Name: idx_positions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_positions_business_id ON public.positions USING btree (business_id);


--
-- Name: idx_positions_business_name_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_positions_business_name_active ON public.positions USING btree (business_id, lower(btrim((name)::text))) WHERE is_active;


--
-- Name: idx_print_audit_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_print_audit_business ON public.print_audit_log USING btree (business_id, created_at DESC);


--
-- Name: idx_print_jobs_browser_lease; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_print_jobs_browser_lease ON public.print_jobs USING btree (business_id, status, lease_expires_at) WHERE (claimed_by IS NOT NULL);


--
-- Name: idx_print_jobs_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_print_jobs_business_status ON public.print_jobs USING btree (business_id, status);


--
-- Name: idx_print_jobs_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_print_jobs_order ON public.print_jobs USING btree (order_id) WHERE (order_id IS NOT NULL);


--
-- Name: idx_print_jobs_printer_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_print_jobs_printer_status ON public.print_jobs USING btree (printer_id, status);


--
-- Name: idx_print_jobs_retry_ready; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_print_jobs_retry_ready ON public.print_jobs USING btree (status, next_attempt_at) WHERE (status = ANY (ARRAY['pending'::text, 'routed'::text, 'failed_retryable'::text]));


--
-- Name: idx_print_jobs_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_print_jobs_source ON public.print_jobs USING btree (source_type, source_id);


--
-- Name: idx_print_jobs_status_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_print_jobs_status_created ON public.print_jobs USING btree (status, created_at);


--
-- Name: idx_printers_business_location_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_printers_business_location_role ON public.printers USING btree (business_id, location_id, role);


--
-- Name: idx_printers_cloudprnt_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_printers_cloudprnt_token ON public.printers USING btree (cloudprnt_token) WHERE (cloudprnt_token IS NOT NULL);


--
-- Name: idx_provider_event; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_provider_event ON public.webhook_events USING btree (provider, webhook_id);


--
-- Name: idx_push_subscriptions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_push_subscriptions_business_id ON public.push_subscriptions USING btree (business_id);


--
-- Name: idx_push_subscriptions_principal_fanout; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_push_subscriptions_principal_fanout ON public.push_subscriptions USING btree (business_id, principal_type, principal_id);


--
-- Name: idx_push_subscriptions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_push_subscriptions_user_id ON public.push_subscriptions USING btree (user_id);


--
-- Name: idx_rbac_audit_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rbac_audit_action ON public.rbac_audit_logs USING btree (action);


--
-- Name: idx_rbac_audit_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rbac_audit_business_id ON public.rbac_audit_logs USING btree (business_id);


--
-- Name: idx_rbac_audit_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rbac_audit_created_at ON public.rbac_audit_logs USING btree (created_at);


--
-- Name: idx_rbac_audit_logs_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rbac_audit_logs_business_id ON public.rbac_audit_logs USING btree (business_id);


--
-- Name: idx_rbac_audit_logs_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rbac_audit_logs_staff_id ON public.rbac_audit_logs USING btree (staff_id);


--
-- Name: idx_rbac_audit_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rbac_audit_staff_id ON public.rbac_audit_logs USING btree (staff_id);


--
-- Name: idx_recurring_entry_templates_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recurring_entry_templates_business ON public.recurring_entry_templates USING btree (business_id, active);


--
-- Name: idx_recurring_entry_templates_due; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recurring_entry_templates_due ON public.recurring_entry_templates USING btree (active, next_run_on) WHERE (active = true);


--
-- Name: idx_report_deliveries_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_report_deliveries_business_id ON public.report_deliveries USING btree (business_id);


--
-- Name: idx_report_deliveries_ready; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_report_deliveries_ready ON public.report_deliveries USING btree (next_attempt_at, lease_expires_at) WHERE (state = ANY (ARRAY['pending'::text, 'retry_wait'::text, 'leased'::text]));


--
-- Name: idx_report_deliveries_scheduled_for; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_report_deliveries_scheduled_for ON public.report_deliveries USING btree (scheduled_for);


--
-- Name: idx_report_schedules_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_report_schedules_business_id ON public.report_schedules USING btree (business_id);


--
-- Name: idx_report_schedules_next_send_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_report_schedules_next_send_at ON public.report_schedules USING btree (next_send_at);


--
-- Name: idx_reservation_settings_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reservation_settings_business_id ON public.reservation_settings USING btree (business_id);


--
-- Name: idx_reservation_status_histories_reservation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reservation_status_histories_reservation_id ON public.reservation_status_histories USING btree (reservation_id);


--
-- Name: idx_reservation_status_histories_table_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reservation_status_histories_table_id ON public.reservation_status_histories USING btree (table_id);


--
-- Name: idx_reservations_biz_status_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reservations_biz_status_time ON public.table_reservations USING btree (business_id, status, reservation_time);


--
-- Name: idx_reservations_reminder; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reservations_reminder ON public.table_reservations USING btree (business_id, reservation_time, reminder_sent, status);


--
-- Name: idx_restaurant_spaces_business_sort; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_restaurant_spaces_business_sort ON public.restaurant_spaces USING btree (business_id, sort_order);


--
-- Name: idx_restaurant_spaces_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_restaurant_spaces_business_status ON public.restaurant_spaces USING btree (business_id, status);


--
-- Name: idx_restaurant_spaces_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_restaurant_spaces_deleted_at ON public.restaurant_spaces USING btree (deleted_at) WHERE (deleted_at IS NOT NULL);


--
-- Name: idx_run_item; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_run_item ON public.checklist_item_completions USING btree (run_id, item_id);


--
-- Name: idx_runtime_control_audit_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_runtime_control_audit_created ON public.runtime_control_audit_events USING btree (created_at DESC);


--
-- Name: idx_runtime_invite_batches_active_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_runtime_invite_batches_active_expiry ON public.runtime_invite_batches USING btree (active, expires_at);


--
-- Name: idx_scheduler_states_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_scheduler_states_key ON public.scheduler_states USING btree (key);


--
-- Name: idx_schedules_biz_week; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_schedules_biz_week ON public.schedules USING btree (business_id, week_start);


--
-- Name: idx_session_summaries_converted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_session_summaries_converted ON public.session_summaries USING btree (converted);


--
-- Name: idx_session_summaries_country; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_session_summaries_country ON public.session_summaries USING btree (country);


--
-- Name: idx_session_summaries_first_seen; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_session_summaries_first_seen ON public.session_summaries USING btree (first_seen);


--
-- Name: idx_shift_notes_business_for_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shift_notes_business_for_date ON public.shift_notes USING btree (business_id, for_date);


--
-- Name: idx_shifts_biz_schedule; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shifts_biz_schedule ON public.shifts USING btree (business_id, schedule_id);


--
-- Name: idx_shifts_biz_staff_starts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shifts_biz_staff_starts ON public.shifts USING btree (business_id, staff_id, starts_at);


--
-- Name: idx_shifts_biz_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shifts_biz_status ON public.shifts USING btree (business_id, status);


--
-- Name: idx_shoutouts_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shoutouts_business_id ON public.shoutouts USING btree (business_id);


--
-- Name: idx_shoutouts_to_staff; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shoutouts_to_staff ON public.shoutouts USING btree (to_staff_id);


--
-- Name: idx_space_layout_audit_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_layout_audit_business_created ON public.space_layout_audit_events USING btree (business_id, created_at DESC);


--
-- Name: idx_space_layout_audit_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_layout_audit_space ON public.space_layout_audit_events USING btree (space_id);


--
-- Name: idx_space_layout_elements_business_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_layout_elements_business_space ON public.space_layout_elements USING btree (business_id, space_id);


--
-- Name: idx_space_layout_elements_draft; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_layout_elements_draft ON public.space_layout_elements USING btree (space_id, is_draft);


--
-- Name: idx_space_layout_elements_space_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_layout_elements_space_id ON public.space_layout_elements USING btree (space_id);


--
-- Name: idx_space_regions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_regions_business_id ON public.space_regions USING btree (business_id);


--
-- Name: idx_space_regions_space_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_regions_space_id ON public.space_regions USING btree (space_id);


--
-- Name: idx_space_scan_sessions_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_scan_sessions_business_status ON public.space_scan_sessions USING btree (business_id, status);


--
-- Name: idx_space_scan_sessions_expires_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_scan_sessions_expires_active ON public.space_scan_sessions USING btree (expires_at) WHERE (status = ANY (ARRAY['waiting_for_phone'::text, 'phone_connected'::text, 'scanning'::text, 'uploading'::text, 'processing'::text, 'review_ready'::text]));


--
-- Name: idx_space_scan_sessions_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_space_scan_sessions_idempotency ON public.space_scan_sessions USING btree (business_id, idempotency_key) WHERE (idempotency_key IS NOT NULL);


--
-- Name: idx_space_scan_uploads_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_scan_uploads_business ON public.space_scan_uploads USING btree (business_id);


--
-- Name: idx_space_scan_uploads_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_space_scan_uploads_idempotency ON public.space_scan_uploads USING btree (idempotency_key) WHERE (idempotency_key IS NOT NULL);


--
-- Name: idx_space_scan_uploads_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_space_scan_uploads_session ON public.space_scan_uploads USING btree (session_id);


--
-- Name: idx_special_features_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_special_features_active ON public.business_special_features USING btree (business_id, is_active);


--
-- Name: idx_special_features_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_special_features_business_id ON public.business_special_features USING btree (business_id);


--
-- Name: idx_special_features_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_special_features_order ON public.business_special_features USING btree (business_id, display_order);


--
-- Name: idx_staff_availabilities_biz_staff_weekday; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_availabilities_biz_staff_weekday ON public.staff_availabilities USING btree (business_id, staff_id, weekday);


--
-- Name: idx_staff_business_email_lower; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_business_email_lower ON public.staff USING btree (business_id, lower(btrim(email)));


--
-- Name: idx_staff_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_business_id ON public.staff USING btree (business_id);


--
-- Name: idx_staff_identities_normalized_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_identities_normalized_email ON public.staff_identities USING btree (normalized_email);


--
-- Name: idx_staff_invitations_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_invitations_business_id ON public.staff_invitations USING btree (business_id);


--
-- Name: idx_staff_invitations_pending_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_invitations_pending_email ON public.staff_invitations USING btree (business_id, lower(btrim(email))) WHERE (status = 'pending'::text);


--
-- Name: idx_staff_invitations_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_invitations_token ON public.staff_invitations USING btree (token);


--
-- Name: idx_staff_login_codes_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_login_codes_code ON public.staff_login_codes USING btree (code);


--
-- Name: idx_staff_login_codes_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_login_codes_expires_at ON public.staff_login_codes USING btree (expires_at);


--
-- Name: idx_staff_login_codes_staff_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_login_codes_staff_code ON public.staff_login_codes USING btree (staff_id, code);


--
-- Name: idx_staff_membership_selection_tokens_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_membership_selection_tokens_expires_at ON public.staff_membership_selection_tokens USING btree (expires_at);


--
-- Name: idx_staff_memberships_business_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_memberships_business_active ON public.staff_memberships USING btree (business_id, is_active);


--
-- Name: idx_staff_memberships_identity_business; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_memberships_identity_business ON public.staff_memberships USING btree (identity_id, business_id);


--
-- Name: idx_staff_memberships_legacy_staff; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_memberships_legacy_staff ON public.staff_memberships USING btree (legacy_staff_id);


--
-- Name: idx_staff_notifications_inbox; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_notifications_inbox ON public.staff_notifications USING btree (business_id, staff_id, id);


--
-- Name: idx_staff_one_primary; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_one_primary ON public.staff_positions USING btree (business_id, staff_id) WHERE is_primary;


--
-- Name: idx_staff_permission_denies_staff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_permission_denies_staff_id ON public.staff_permission_denies USING btree (staff_id);


--
-- Name: idx_staff_permission_denies_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_permission_denies_unique ON public.staff_permission_denies USING btree (business_id, staff_id, permission);


--
-- Name: idx_staff_position; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_staff_position ON public.staff_positions USING btree (staff_id, position_id);


--
-- Name: idx_staff_positions_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_positions_business_id ON public.staff_positions USING btree (business_id);


--
-- Name: idx_staff_role_level; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_staff_role_level ON public.staff USING btree (role_level);


--
-- Name: idx_supported_currencies_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_supported_currencies_code ON public.supported_currencies USING btree (code);


--
-- Name: idx_supported_languages_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_supported_languages_code ON public.supported_languages USING btree (code);


--
-- Name: idx_swaps_business_shift; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_swaps_business_shift ON public.shift_swap_requests USING btree (business_id, shift_id);


--
-- Name: idx_swaps_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_swaps_business_status ON public.shift_swap_requests USING btree (business_id, status);


--
-- Name: idx_table_combination_members_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_combination_members_business ON public.table_combination_members USING btree (business_id);


--
-- Name: idx_table_combination_members_table; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_combination_members_table ON public.table_combination_members USING btree (table_id);


--
-- Name: idx_table_combinations_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_combinations_business ON public.table_combinations USING btree (business_id);


--
-- Name: idx_table_combinations_primary; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_combinations_primary ON public.table_combinations USING btree (primary_table_id);


--
-- Name: idx_table_reservations_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_business_id ON public.table_reservations USING btree (business_id);


--
-- Name: idx_table_reservations_business_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_business_time ON public.table_reservations USING btree (business_id, reservation_time);


--
-- Name: idx_table_reservations_claimed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_claimed_at ON public.table_reservations USING btree (business_id, claimed_at) WHERE (claimed_at IS NOT NULL);


--
-- Name: idx_table_reservations_claimed_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_claimed_by ON public.table_reservations USING btree (claimed_by_staff_id);


--
-- Name: idx_table_reservations_confirmation_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_confirmation_code ON public.table_reservations USING btree (confirmation_code);


--
-- Name: idx_table_reservations_language; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_language ON public.table_reservations USING btree (language);


--
-- Name: idx_table_reservations_reminder_scan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_reminder_scan ON public.table_reservations USING btree (business_id, reminder_sent, status, reservation_time);


--
-- Name: idx_table_reservations_reservation_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_reservation_time ON public.table_reservations USING btree (reservation_time);


--
-- Name: idx_table_reservations_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_status ON public.table_reservations USING btree (status);


--
-- Name: idx_table_reservations_table_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_table_id ON public.table_reservations USING btree (table_id);


--
-- Name: idx_table_reservations_table_status_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_table_status_time ON public.table_reservations USING btree (table_id, status, reservation_time);


--
-- Name: idx_table_reservations_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_reservations_time ON public.table_reservations USING btree (reservation_time);


--
-- Name: idx_tables_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tables_business_id ON public.tables USING btree (business_id);


--
-- Name: idx_tables_business_space; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tables_business_space ON public.tables USING btree (business_id, space_id);


--
-- Name: idx_tables_space_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tables_space_id ON public.tables USING btree (space_id);


--
-- Name: idx_tables_table_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_tables_table_code ON public.tables USING btree (table_code);


--
-- Name: idx_telegram_connection_tokens_business_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telegram_connection_tokens_business_pending ON public.telegram_connection_tokens USING btree (business_id, expires_at);


--
-- Name: idx_telegram_connection_tokens_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telegram_connection_tokens_expires_at ON public.telegram_connection_tokens USING btree (expires_at);


--
-- Name: idx_telegram_connection_tokens_token_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_telegram_connection_tokens_token_hash ON public.telegram_connection_tokens USING btree (token_hash);


--
-- Name: idx_telegram_update_receipts_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telegram_update_receipts_business_id ON public.telegram_update_receipts USING btree (business_id);


--
-- Name: idx_telegram_update_receipts_processed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telegram_update_receipts_processed_at ON public.telegram_update_receipts USING btree (processed_at);


--
-- Name: idx_time_entries_biz_staff_clockin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_time_entries_biz_staff_clockin ON public.time_entries USING btree (business_id, staff_id, clock_in_at);


--
-- Name: idx_time_entries_biz_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_time_entries_biz_status ON public.time_entries USING btree (business_id, status);


--
-- Name: idx_time_entries_one_open; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_time_entries_one_open ON public.time_entries USING btree (business_id, staff_id) WHERE (status = 'open'::text);


--
-- Name: idx_time_off_requests_biz_staff; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_time_off_requests_biz_staff ON public.time_off_requests USING btree (business_id, staff_id);


--
-- Name: idx_time_off_requests_biz_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_time_off_requests_biz_status ON public.time_off_requests USING btree (business_id, status);


--
-- Name: idx_translations_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_translations_business_id ON public.translations USING btree (business_id);


--
-- Name: idx_translations_entity_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_translations_entity_id ON public.translations USING btree (entity_id);


--
-- Name: idx_translations_entity_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_translations_entity_type ON public.translations USING btree (entity_type);


--
-- Name: idx_translations_guest_lookup_effective; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_translations_guest_lookup_effective ON public.translations USING btree (business_id, language_code, entity_type, entity_id, field_name, id DESC);


--
-- Name: idx_user_auths_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_auths_user_id ON public.user_auths USING btree (user_id);


--
-- Name: idx_user_auths_wallet_address; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_auths_wallet_address ON public.user_auths USING btree (wallet_address);


--
-- Name: idx_user_interactions_event_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_interactions_event_category ON public.user_interactions USING btree (event_category);


--
-- Name: idx_user_interactions_event_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_interactions_event_type ON public.user_interactions USING btree (event_type);


--
-- Name: idx_user_interactions_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_interactions_session_id ON public.user_interactions USING btree (session_id);


--
-- Name: idx_user_interactions_timestamp; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_interactions_timestamp ON public.user_interactions USING btree ("timestamp");


--
-- Name: idx_user_session_refresh_history_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_session_refresh_history_expires_at ON public.user_session_refresh_history USING btree (expires_at, session_id);


--
-- Name: idx_user_session_refresh_history_session_rotated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_session_refresh_history_session_rotated_at ON public.user_session_refresh_history USING btree (session_id, rotated_at);


--
-- Name: idx_user_sessions_address; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_sessions_address ON public.user_sessions USING btree (address);


--
-- Name: idx_user_sessions_cleanup_active_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_sessions_cleanup_active_expiry ON public.user_sessions USING btree (expires_at, refresh_expires_at) WHERE (revoked = false);


--
-- Name: idx_user_sessions_cleanup_revoked_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_sessions_cleanup_revoked_at ON public.user_sessions USING btree (revoked_at) WHERE ((revoked = true) AND (revoked_at IS NOT NULL));


--
-- Name: idx_user_sessions_cleanup_revoked_legacy_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_sessions_cleanup_revoked_legacy_expiry ON public.user_sessions USING btree (expires_at, refresh_expires_at) WHERE ((revoked = true) AND (revoked_at IS NULL));


--
-- Name: idx_user_sessions_previous_refresh_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_sessions_previous_refresh_token ON public.user_sessions USING btree (previous_refresh_token);


--
-- Name: idx_user_sessions_refresh_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_sessions_refresh_token ON public.user_sessions USING btree (refresh_token);


--
-- Name: idx_user_sessions_session_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_user_sessions_session_token ON public.user_sessions USING btree (session_token);


--
-- Name: idx_user_sessions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_sessions_user_id ON public.user_sessions USING btree (user_id);


--
-- Name: idx_users_address; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_address ON public.users USING btree (address);


--
-- Name: idx_users_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_deleted_at ON public.users USING btree (deleted_at);


--
-- Name: idx_users_deletion_scheduled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_deletion_scheduled ON public.users USING btree (deletion_scheduled_at) WHERE (deletion_scheduled_at IS NOT NULL);


--
-- Name: idx_users_deletion_scheduled_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_deletion_scheduled_at ON public.users USING btree (deletion_scheduled_at);


--
-- Name: idx_users_email_lower_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_email_lower_active ON public.users USING btree (lower(TRIM(BOTH FROM email))) WHERE ((email IS NOT NULL) AND (btrim(email) <> ''::text) AND (deleted_at IS NULL));


--
-- Name: idx_users_google_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_google_id ON public.users USING btree (google_id);


--
-- Name: idx_webhook_events_event_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_events_event_type ON public.webhook_events USING btree (event_type);


--
-- Name: idx_webhook_events_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_events_status ON public.webhook_events USING btree (status);


--
-- Name: idx_webhook_events_status_received; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_events_status_received ON public.webhook_events USING btree (status, received_at);


--
-- Name: idx_whatsapp_business_devices_retry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_whatsapp_business_devices_retry ON public.whatsapp_business_devices USING btree (status, next_retry_at, business_id);


--
-- Name: idx_withdrawal_histories_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_withdrawal_histories_business_id ON public.withdrawal_histories USING btree (business_id);


--
-- Name: idx_withdrawal_histories_transaction_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_withdrawal_histories_transaction_hash ON public.withdrawal_histories USING btree (transaction_hash);


--
-- Name: orders_guest_request_identity_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX orders_guest_request_identity_uq ON public.orders USING btree (business_id, created_by, client_request_id) WHERE ((created_by = 'guest'::text) AND (client_request_id IS NOT NULL));


--
-- Name: uni_auth_attempts_principal_kind; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uni_auth_attempts_principal_kind ON public.auth_attempts USING btree (principal, kind);


--
-- Name: uq_inventory_alert_logs_daily; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_inventory_alert_logs_daily ON public.inventory_alert_logs USING btree (business_id, inventory_item_id, alert_type, alert_day);


--
-- Name: businesses trg_business_activation_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_business_activation_insert AFTER INSERT ON public.businesses FOR EACH ROW EXECUTE FUNCTION public.capture_business_activation_insert();


--
-- Name: businesses trg_business_activation_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_business_activation_update AFTER UPDATE OF qr_previewed_at, onboarding_completed_at ON public.businesses FOR EACH ROW EXECUTE FUNCTION public.capture_business_activation_update();


--
-- Name: user_sessions trg_capture_user_session_refresh_history; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER trg_capture_user_session_refresh_history AFTER UPDATE OF previous_refresh_token, previous_rotated_at ON public.user_sessions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.capture_user_session_refresh_history();


--
-- Name: user_session_refresh_history trg_default_user_session_refresh_history_expiry; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_default_user_session_refresh_history_expiry BEFORE INSERT ON public.user_session_refresh_history FOR EACH ROW EXECUTE FUNCTION public.default_user_session_refresh_history_expiry();


--
-- Name: menus trg_menu_activation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_menu_activation AFTER INSERT OR UPDATE OF categories ON public.menus FOR EACH ROW EXECUTE FUNCTION public.capture_menu_activation();


--
-- Name: orders trg_order_activation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_order_activation AFTER INSERT OR UPDATE OF status ON public.orders FOR EACH ROW EXECUTE FUNCTION public.capture_order_activation();


--
-- Name: bills trg_paid_bill_activation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_paid_bill_activation AFTER INSERT OR UPDATE OF status ON public.bills FOR EACH ROW EXECUTE FUNCTION public.capture_paid_bill_activation();


--
-- Name: business_plugins trg_payment_config_activation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_payment_config_activation AFTER INSERT OR UPDATE OF is_enabled, plugin_id ON public.business_plugins FOR EACH ROW EXECUTE FUNCTION public.capture_payment_config_activation();


--
-- Name: staff_invitations trg_staff_invitation_activation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_staff_invitation_activation AFTER INSERT ON public.staff_invitations FOR EACH ROW EXECUTE FUNCTION public.capture_staff_invitation_activation();


--
-- Name: tables trg_table_activation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_table_activation AFTER INSERT ON public.tables FOR EACH ROW EXECUTE FUNCTION public.capture_table_activation();


--
-- Name: activation_event_outbox activation_event_outbox_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activation_event_outbox
    ADD CONSTRAINT activation_event_outbox_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: bill_items bill_items_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_items
    ADD CONSTRAINT bill_items_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.bills(id) ON DELETE CASCADE;


--
-- Name: business_activation_states business_activation_states_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_activation_states
    ADD CONSTRAINT business_activation_states_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: business_creation_requests business_creation_requests_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_creation_requests
    ADD CONSTRAINT business_creation_requests_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: business_operating_exceptions business_operating_exceptions_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_operating_exceptions
    ADD CONSTRAINT business_operating_exceptions_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: crypto_payment_quotes crypto_payment_quotes_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crypto_payment_quotes
    ADD CONSTRAINT crypto_payment_quotes_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.bills(id) ON DELETE CASCADE;


--
-- Name: crypto_payment_quotes crypto_payment_quotes_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crypto_payment_quotes
    ADD CONSTRAINT crypto_payment_quotes_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: crypto_payment_quotes crypto_payment_quotes_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crypto_payment_quotes
    ADD CONSTRAINT crypto_payment_quotes_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.payments(id) ON DELETE SET NULL;


--
-- Name: fiscal_delivery_tasks fiscal_delivery_tasks_receipt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscal_delivery_tasks
    ADD CONSTRAINT fiscal_delivery_tasks_receipt_id_fkey FOREIGN KEY (receipt_id) REFERENCES public.fiscal_receipts(id) ON DELETE CASCADE;


--
-- Name: ai_waiter_conversations fk_ai_waiter_conversations_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_waiter_conversations
    ADD CONSTRAINT fk_ai_waiter_conversations_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: bill_history_events fk_bill_history_events_bill; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_history_events
    ADD CONSTRAINT fk_bill_history_events_bill FOREIGN KEY (bill_id) REFERENCES public.bills(id);


--
-- Name: bill_split_shares fk_bill_split_shares_alternative_payment; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_split_shares
    ADD CONSTRAINT fk_bill_split_shares_alternative_payment FOREIGN KEY (alternative_payment_id) REFERENCES public.alternative_payments(id);


--
-- Name: bill_split_shares fk_bill_split_shares_bill; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_split_shares
    ADD CONSTRAINT fk_bill_split_shares_bill FOREIGN KEY (bill_id) REFERENCES public.bills(id);


--
-- Name: bill_split_shares fk_bill_split_shares_payment; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_split_shares
    ADD CONSTRAINT fk_bill_split_shares_payment FOREIGN KEY (payment_id) REFERENCES public.payments(id);


--
-- Name: alternative_payments fk_bills_alternative_payments; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alternative_payments
    ADD CONSTRAINT fk_bills_alternative_payments FOREIGN KEY (bill_id) REFERENCES public.bills(id);


--
-- Name: bills fk_bills_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bills
    ADD CONSTRAINT fk_bills_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: bills fk_bills_closed_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bills
    ADD CONSTRAINT fk_bills_closed_by_staff FOREIGN KEY (closed_by_staff_id) REFERENCES public.staff(id);


--
-- Name: bills fk_bills_created_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bills
    ADD CONSTRAINT fk_bills_created_by_staff FOREIGN KEY (created_by_staff_id) REFERENCES public.staff(id);


--
-- Name: bills fk_bills_crm_customer; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bills
    ADD CONSTRAINT fk_bills_crm_customer FOREIGN KEY (crm_customer_id) REFERENCES public.customers(id);


--
-- Name: payments fk_bills_payments; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT fk_bills_payments FOREIGN KEY (bill_id) REFERENCES public.bills(id);


--
-- Name: bundles fk_bundles_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bundles
    ADD CONSTRAINT fk_bundles_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: business_alert_settings fk_business_alert_settings_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_alert_settings
    ADD CONSTRAINT fk_business_alert_settings_business FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: business_currencies fk_business_currencies_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_currencies
    ADD CONSTRAINT fk_business_currencies_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: business_gallery_images fk_business_gallery_images_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_gallery_images
    ADD CONSTRAINT fk_business_gallery_images_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: business_languages fk_business_languages_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_languages
    ADD CONSTRAINT fk_business_languages_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: business_milestone_events fk_business_milestone_events_bill; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_milestone_events
    ADD CONSTRAINT fk_business_milestone_events_bill FOREIGN KEY (bill_id) REFERENCES public.bills(id);


--
-- Name: business_milestone_events fk_business_milestone_events_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_milestone_events
    ADD CONSTRAINT fk_business_milestone_events_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: business_operating_hours fk_business_operating_hours_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_operating_hours
    ADD CONSTRAINT fk_business_operating_hours_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: business_plugins fk_business_plugins_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_plugins
    ADD CONSTRAINT fk_business_plugins_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: business_plugins fk_business_plugins_plugin; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_plugins
    ADD CONSTRAINT fk_business_plugins_plugin FOREIGN KEY (plugin_id) REFERENCES public.plugins(id);


--
-- Name: business_revenue_aggregates fk_business_revenue_aggregates_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_revenue_aggregates
    ADD CONSTRAINT fk_business_revenue_aggregates_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: business_special_features fk_business_special_features_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_special_features
    ADD CONSTRAINT fk_business_special_features_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: cash_register_movements fk_cash_register_movements_actor_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_movements
    ADD CONSTRAINT fk_cash_register_movements_actor_staff FOREIGN KEY (actor_staff_id) REFERENCES public.staff(id) ON DELETE SET NULL;


--
-- Name: cash_register_movements fk_cash_register_movements_actor_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_movements
    ADD CONSTRAINT fk_cash_register_movements_actor_user FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: cash_register_movements fk_cash_register_movements_alternative_payment; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_movements
    ADD CONSTRAINT fk_cash_register_movements_alternative_payment FOREIGN KEY (alternative_payment_id) REFERENCES public.alternative_payments(id) ON DELETE SET NULL;


--
-- Name: cash_register_movements fk_cash_register_movements_bill; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_movements
    ADD CONSTRAINT fk_cash_register_movements_bill FOREIGN KEY (bill_id) REFERENCES public.bills(id) ON DELETE SET NULL;


--
-- Name: cash_register_sessions fk_cash_register_sessions_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_sessions
    ADD CONSTRAINT fk_cash_register_sessions_business FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: cash_register_sessions fk_cash_register_sessions_closed_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_sessions
    ADD CONSTRAINT fk_cash_register_sessions_closed_by FOREIGN KEY (closed_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: cash_register_sessions fk_cash_register_sessions_closed_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_sessions
    ADD CONSTRAINT fk_cash_register_sessions_closed_by_staff FOREIGN KEY (closed_by_staff_id) REFERENCES public.staff(id) ON DELETE SET NULL;


--
-- Name: cash_register_movements fk_cash_register_sessions_movements; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_movements
    ADD CONSTRAINT fk_cash_register_sessions_movements FOREIGN KEY (session_id) REFERENCES public.cash_register_sessions(id) ON DELETE CASCADE;


--
-- Name: cash_register_sessions fk_cash_register_sessions_opened_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_sessions
    ADD CONSTRAINT fk_cash_register_sessions_opened_by FOREIGN KEY (opened_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: cash_register_sessions fk_cash_register_sessions_opened_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cash_register_sessions
    ADD CONSTRAINT fk_cash_register_sessions_opened_by_staff FOREIGN KEY (opened_by_staff_id) REFERENCES public.staff(id) ON DELETE SET NULL;


--
-- Name: counters fk_counters_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.counters
    ADD CONSTRAINT fk_counters_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: counters fk_counters_current_bill; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.counters
    ADD CONSTRAINT fk_counters_current_bill FOREIGN KEY (current_bill_id) REFERENCES public.bills(id);


--
-- Name: customer_addresses fk_customer_addresses_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_addresses
    ADD CONSTRAINT fk_customer_addresses_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: customer_addresses fk_customer_addresses_customer; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_addresses
    ADD CONSTRAINT fk_customer_addresses_customer FOREIGN KEY (customer_id) REFERENCES public.customers(id);


--
-- Name: customer_businesses fk_customer_businesses_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_businesses
    ADD CONSTRAINT fk_customer_businesses_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: customer_visits fk_customer_businesses_visits; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_visits
    ADD CONSTRAINT fk_customer_businesses_visits FOREIGN KEY (customer_business_id) REFERENCES public.customer_businesses(id);


--
-- Name: customer_communications fk_customer_communications_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_communications
    ADD CONSTRAINT fk_customer_communications_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: customer_communications fk_customer_communications_customer_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_communications
    ADD CONSTRAINT fk_customer_communications_customer_business FOREIGN KEY (customer_business_id) REFERENCES public.customer_businesses(id);


--
-- Name: customer_visits fk_customer_visits_bill; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_visits
    ADD CONSTRAINT fk_customer_visits_bill FOREIGN KEY (bill_id) REFERENCES public.bills(id);


--
-- Name: customer_visits fk_customer_visits_table; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_visits
    ADD CONSTRAINT fk_customer_visits_table FOREIGN KEY (table_id) REFERENCES public.tables(id);


--
-- Name: customer_businesses fk_customers_business_connections; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_businesses
    ADD CONSTRAINT fk_customers_business_connections FOREIGN KEY (customer_id) REFERENCES public.customers(id);


--
-- Name: customer_preferences fk_customers_preferences; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer_preferences
    ADD CONSTRAINT fk_customers_preferences FOREIGN KEY (customer_id) REFERENCES public.customers(id);


--
-- Name: delivery_drivers fk_delivery_drivers_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_drivers
    ADD CONSTRAINT fk_delivery_drivers_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: delivery_orders fk_delivery_drivers_deliveries; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_orders
    ADD CONSTRAINT fk_delivery_drivers_deliveries FOREIGN KEY (driver_id) REFERENCES public.delivery_drivers(id);


--
-- Name: delivery_drivers fk_delivery_drivers_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_drivers
    ADD CONSTRAINT fk_delivery_drivers_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id);


--
-- Name: delivery_orders fk_delivery_orders_bill; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_orders
    ADD CONSTRAINT fk_delivery_orders_bill FOREIGN KEY (bill_id) REFERENCES public.bills(id);


--
-- Name: delivery_orders fk_delivery_orders_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_orders
    ADD CONSTRAINT fk_delivery_orders_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: delivery_orders fk_delivery_orders_customer; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_orders
    ADD CONSTRAINT fk_delivery_orders_customer FOREIGN KEY (customer_id) REFERENCES public.customers(id);


--
-- Name: delivery_orders fk_delivery_orders_order; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_orders
    ADD CONSTRAINT fk_delivery_orders_order FOREIGN KEY (order_id) REFERENCES public.orders(id);


--
-- Name: delivery_status_history fk_delivery_orders_status_history; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_status_history
    ADD CONSTRAINT fk_delivery_orders_status_history FOREIGN KEY (delivery_order_id) REFERENCES public.delivery_orders(id);


--
-- Name: delivery_orders fk_delivery_orders_zone; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_orders
    ADD CONSTRAINT fk_delivery_orders_zone FOREIGN KEY (zone_id) REFERENCES public.delivery_zones(id);


--
-- Name: delivery_settings fk_delivery_settings_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_settings
    ADD CONSTRAINT fk_delivery_settings_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: delivery_zones fk_delivery_zones_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_zones
    ADD CONSTRAINT fk_delivery_zones_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: demo_instances fk_demo_instances_admin_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.demo_instances
    ADD CONSTRAINT fk_demo_instances_admin_user FOREIGN KEY (admin_user_id) REFERENCES public.users(id);


--
-- Name: demo_instances fk_demo_instances_primary_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.demo_instances
    ADD CONSTRAINT fk_demo_instances_primary_business FOREIGN KEY (primary_business_id) REFERENCES public.businesses(id);


--
-- Name: demo_instances fk_demo_instances_secondary_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.demo_instances
    ADD CONSTRAINT fk_demo_instances_secondary_business FOREIGN KEY (secondary_business_id) REFERENCES public.businesses(id);


--
-- Name: demo_runs fk_demo_runs_demo_instance; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.demo_runs
    ADD CONSTRAINT fk_demo_runs_demo_instance FOREIGN KEY (demo_instance_id) REFERENCES public.demo_instances(id);


--
-- Name: director_console_messages fk_director_console_messages_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_console_messages
    ADD CONSTRAINT fk_director_console_messages_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: director_console_messages fk_director_console_messages_thread; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_console_messages
    ADD CONSTRAINT fk_director_console_messages_thread FOREIGN KEY (thread_id) REFERENCES public.director_console_threads(id);


--
-- Name: director_console_threads fk_director_console_threads_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.director_console_threads
    ADD CONSTRAINT fk_director_console_threads_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: inventory_items fk_inventory_items_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_items
    ADD CONSTRAINT fk_inventory_items_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: inventory_movements fk_inventory_movements_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_movements
    ADD CONSTRAINT fk_inventory_movements_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: inventory_movements fk_inventory_movements_inventory_item; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_movements
    ADD CONSTRAINT fk_inventory_movements_inventory_item FOREIGN KEY (inventory_item_id) REFERENCES public.inventory_items(id);


--
-- Name: inventory_recipes fk_inventory_recipes_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_recipes
    ADD CONSTRAINT fk_inventory_recipes_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: inventory_recipes fk_inventory_recipes_inventory_item; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_recipes
    ADD CONSTRAINT fk_inventory_recipes_inventory_item FOREIGN KEY (inventory_item_id) REFERENCES public.inventory_items(id);


--
-- Name: inventory_settings fk_inventory_settings_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_settings
    ADD CONSTRAINT fk_inventory_settings_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: loyalty_programs fk_loyalty_programs_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loyalty_programs
    ADD CONSTRAINT fk_loyalty_programs_business FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: loyalty_tiers fk_loyalty_programs_tiers; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loyalty_tiers
    ADD CONSTRAINT fk_loyalty_programs_tiers FOREIGN KEY (loyalty_program_id) REFERENCES public.loyalty_programs(id) ON DELETE CASCADE;


--
-- Name: manual_ledger_entries fk_manual_ledger_entries_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manual_ledger_entries
    ADD CONSTRAINT fk_manual_ledger_entries_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: manual_ledger_entries fk_manual_ledger_entries_created_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manual_ledger_entries
    ADD CONSTRAINT fk_manual_ledger_entries_created_by_staff FOREIGN KEY (created_by_staff_id) REFERENCES public.staff(id);


--
-- Name: manual_ledger_entries fk_manual_ledger_entries_voided_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manual_ledger_entries
    ADD CONSTRAINT fk_manual_ledger_entries_voided_by_staff FOREIGN KEY (voided_by_staff_id) REFERENCES public.staff(id);


--
-- Name: menu_extraction_jobs fk_menu_extraction_jobs_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_extraction_jobs
    ADD CONSTRAINT fk_menu_extraction_jobs_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: menu_extraction_images fk_menu_extraction_jobs_images; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_extraction_images
    ADD CONSTRAINT fk_menu_extraction_jobs_images FOREIGN KEY (job_id) REFERENCES public.menu_extraction_jobs(id);


--
-- Name: menu_wizard_sessions fk_menu_wizard_sessions_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_wizard_sessions
    ADD CONSTRAINT fk_menu_wizard_sessions_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: menu_wizard_messages fk_menu_wizard_sessions_messages; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menu_wizard_messages
    ADD CONSTRAINT fk_menu_wizard_sessions_messages FOREIGN KEY (session_id) REFERENCES public.menu_wizard_sessions(id);


--
-- Name: menus fk_menus_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT fk_menus_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: offers fk_offers_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT fk_offers_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: operational_alert_events fk_operational_alert_events_actor_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alert_events
    ADD CONSTRAINT fk_operational_alert_events_actor_staff FOREIGN KEY (actor_staff_id) REFERENCES public.staff(id) ON DELETE SET NULL;


--
-- Name: operational_alert_events fk_operational_alert_events_actor_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alert_events
    ADD CONSTRAINT fk_operational_alert_events_actor_user FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: operational_alert_events fk_operational_alert_events_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alert_events
    ADD CONSTRAINT fk_operational_alert_events_business FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: operational_alerts fk_operational_alerts_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alerts
    ADD CONSTRAINT fk_operational_alerts_business FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: operational_alerts fk_operational_alerts_claimed_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alerts
    ADD CONSTRAINT fk_operational_alerts_claimed_by_staff FOREIGN KEY (claimed_by_staff_id) REFERENCES public.staff(id) ON DELETE SET NULL;


--
-- Name: operational_alerts fk_operational_alerts_claimed_by_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alerts
    ADD CONSTRAINT fk_operational_alerts_claimed_by_user FOREIGN KEY (claimed_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: operational_alert_events fk_operational_alerts_events; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_alert_events
    ADD CONSTRAINT fk_operational_alerts_events FOREIGN KEY (alert_id) REFERENCES public.operational_alerts(id) ON DELETE CASCADE;


--
-- Name: orders fk_orders_bill; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT fk_orders_bill FOREIGN KEY (bill_id) REFERENCES public.bills(id);


--
-- Name: orders fk_orders_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT fk_orders_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: payment_refund_destinations fk_payment_refund_destinations_payment; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_refund_destinations
    ADD CONSTRAINT fk_payment_refund_destinations_payment FOREIGN KEY (payment_id) REFERENCES public.payments(id);


--
-- Name: payroll_line_items fk_payroll_line_items_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_line_items
    ADD CONSTRAINT fk_payroll_line_items_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: payroll_line_items fk_payroll_line_items_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_line_items
    ADD CONSTRAINT fk_payroll_line_items_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id);


--
-- Name: payroll_runs fk_payroll_runs_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_runs
    ADD CONSTRAINT fk_payroll_runs_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: payroll_runs fk_payroll_runs_created_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_runs
    ADD CONSTRAINT fk_payroll_runs_created_by_staff FOREIGN KEY (created_by_staff_id) REFERENCES public.staff(id);


--
-- Name: payroll_line_items fk_payroll_runs_line_items; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_line_items
    ADD CONSTRAINT fk_payroll_runs_line_items FOREIGN KEY (payroll_run_id) REFERENCES public.payroll_runs(id);


--
-- Name: payroll_runs fk_payroll_runs_paid_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_runs
    ADD CONSTRAINT fk_payroll_runs_paid_by_staff FOREIGN KEY (paid_by_staff_id) REFERENCES public.staff(id);


--
-- Name: payroll_runs fk_payroll_runs_voided_by_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payroll_runs
    ADD CONSTRAINT fk_payroll_runs_voided_by_staff FOREIGN KEY (voided_by_staff_id) REFERENCES public.staff(id);


--
-- Name: plugin_notification_deliveries fk_plugin_notification_deliveries_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_notification_deliveries
    ADD CONSTRAINT fk_plugin_notification_deliveries_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: plugin_notification_delivery_attempts fk_plugin_notification_delivery_attempts_delivery; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_notification_delivery_attempts
    ADD CONSTRAINT fk_plugin_notification_delivery_attempts_delivery FOREIGN KEY (delivery_id) REFERENCES public.plugin_notification_deliveries(id);


--
-- Name: plugin_translations fk_plugins_translations; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_translations
    ADD CONSTRAINT fk_plugins_translations FOREIGN KEY (plugin_id) REFERENCES public.plugins(id);


--
-- Name: rbac_audit_logs fk_rbac_audit_logs_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rbac_audit_logs
    ADD CONSTRAINT fk_rbac_audit_logs_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: rbac_audit_logs fk_rbac_audit_logs_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rbac_audit_logs
    ADD CONSTRAINT fk_rbac_audit_logs_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id);


--
-- Name: report_schedules fk_report_schedules_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_schedules
    ADD CONSTRAINT fk_report_schedules_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: reservation_settings fk_reservation_settings_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_settings
    ADD CONSTRAINT fk_reservation_settings_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: reservation_status_histories fk_reservation_status_histories_table; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_status_histories
    ADD CONSTRAINT fk_reservation_status_histories_table FOREIGN KEY (table_id) REFERENCES public.tables(id);


--
-- Name: staff fk_staff_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff
    ADD CONSTRAINT fk_staff_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: staff_invitations fk_staff_invitations_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_invitations
    ADD CONSTRAINT fk_staff_invitations_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: staff_login_codes fk_staff_login_codes_staff; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_login_codes
    ADD CONSTRAINT fk_staff_login_codes_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id);


--
-- Name: table_reservations fk_table_reservations_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_reservations
    ADD CONSTRAINT fk_table_reservations_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: reservation_status_histories fk_table_reservations_status_history; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_status_histories
    ADD CONSTRAINT fk_table_reservations_status_history FOREIGN KEY (reservation_id) REFERENCES public.table_reservations(id);


--
-- Name: table_reservations fk_table_reservations_table; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_reservations
    ADD CONSTRAINT fk_table_reservations_table FOREIGN KEY (table_id) REFERENCES public.tables(id);


--
-- Name: tables fk_tables_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tables
    ADD CONSTRAINT fk_tables_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: telegram_connection_tokens fk_telegram_connection_tokens_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telegram_connection_tokens
    ADD CONSTRAINT fk_telegram_connection_tokens_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: telegram_update_receipts fk_telegram_update_receipts_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telegram_update_receipts
    ADD CONSTRAINT fk_telegram_update_receipts_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: user_session_refresh_history fk_user_session_refresh_history_session; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_session_refresh_history
    ADD CONSTRAINT fk_user_session_refresh_history_session FOREIGN KEY (session_id) REFERENCES public.user_sessions(id) ON DELETE CASCADE;


--
-- Name: withdrawal_histories fk_withdrawal_histories_business; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.withdrawal_histories
    ADD CONSTRAINT fk_withdrawal_histories_business FOREIGN KEY (business_id) REFERENCES public.businesses(id);


--
-- Name: guest_receipt_sends guest_receipt_sends_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_receipt_sends
    ADD CONSTRAINT guest_receipt_sends_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.bills(id) ON DELETE CASCADE;


--
-- Name: ledger_entry_attachments ledger_entry_attachments_entry_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ledger_entry_attachments
    ADD CONSTRAINT ledger_entry_attachments_entry_id_fkey FOREIGN KEY (entry_id) REFERENCES public.manual_ledger_entries(id) ON DELETE CASCADE;


--
-- Name: ops_assistant_messages ops_assistant_messages_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_messages
    ADD CONSTRAINT ops_assistant_messages_request_id_fkey FOREIGN KEY (request_id) REFERENCES public.ops_assistant_requests(id) ON DELETE SET NULL;


--
-- Name: ops_assistant_requests ops_assistant_requests_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_requests
    ADD CONSTRAINT ops_assistant_requests_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: ops_assistant_requests ops_assistant_requests_thread_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ops_assistant_requests
    ADD CONSTRAINT ops_assistant_requests_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES public.ops_assistant_threads(id) ON DELETE SET NULL;


--
-- Name: print_audit_log print_audit_log_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_audit_log
    ADD CONSTRAINT print_audit_log_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: print_audit_log print_audit_log_print_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_audit_log
    ADD CONSTRAINT print_audit_log_print_job_id_fkey FOREIGN KEY (print_job_id) REFERENCES public.print_jobs(id) ON DELETE SET NULL;


--
-- Name: print_audit_log print_audit_log_printer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_audit_log
    ADD CONSTRAINT print_audit_log_printer_id_fkey FOREIGN KEY (printer_id) REFERENCES public.printers(id) ON DELETE SET NULL;


--
-- Name: print_jobs print_jobs_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_jobs
    ADD CONSTRAINT print_jobs_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: print_jobs print_jobs_order_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_jobs
    ADD CONSTRAINT print_jobs_order_id_fkey FOREIGN KEY (order_id) REFERENCES public.orders(id) ON DELETE SET NULL;


--
-- Name: print_jobs print_jobs_printer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.print_jobs
    ADD CONSTRAINT print_jobs_printer_id_fkey FOREIGN KEY (printer_id) REFERENCES public.printers(id) ON DELETE SET NULL;


--
-- Name: printers printers_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.printers
    ADD CONSTRAINT printers_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: printers printers_fallback_printer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.printers
    ADD CONSTRAINT printers_fallback_printer_id_fkey FOREIGN KEY (fallback_printer_id) REFERENCES public.printers(id) ON DELETE SET NULL;


--
-- Name: recurring_entry_templates recurring_entry_templates_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recurring_entry_templates
    ADD CONSTRAINT recurring_entry_templates_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: report_deliveries report_deliveries_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_deliveries
    ADD CONSTRAINT report_deliveries_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: report_deliveries report_deliveries_schedule_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_deliveries
    ADD CONSTRAINT report_deliveries_schedule_id_fkey FOREIGN KEY (schedule_id) REFERENCES public.report_schedules(id) ON DELETE CASCADE;


--
-- Name: restaurant_spaces restaurant_spaces_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.restaurant_spaces
    ADD CONSTRAINT restaurant_spaces_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: runtime_control_audit_events runtime_control_audit_events_invite_batch_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_control_audit_events
    ADD CONSTRAINT runtime_control_audit_events_invite_batch_id_fkey FOREIGN KEY (invite_batch_id) REFERENCES public.runtime_invite_batches(id) ON DELETE RESTRICT;


--
-- Name: runtime_invite_claims runtime_invite_claims_invite_batch_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_invite_claims
    ADD CONSTRAINT runtime_invite_claims_invite_batch_id_fkey FOREIGN KEY (invite_batch_id) REFERENCES public.runtime_invite_batches(id) ON DELETE RESTRICT;


--
-- Name: space_layout_audit_events space_layout_audit_events_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_layout_audit_events
    ADD CONSTRAINT space_layout_audit_events_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: space_layout_audit_events space_layout_audit_events_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_layout_audit_events
    ADD CONSTRAINT space_layout_audit_events_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.restaurant_spaces(id) ON DELETE SET NULL;


--
-- Name: space_layout_elements space_layout_elements_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_layout_elements
    ADD CONSTRAINT space_layout_elements_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: space_layout_elements space_layout_elements_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_layout_elements
    ADD CONSTRAINT space_layout_elements_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.restaurant_spaces(id) ON DELETE CASCADE;


--
-- Name: space_regions space_regions_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_regions
    ADD CONSTRAINT space_regions_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: space_regions space_regions_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_regions
    ADD CONSTRAINT space_regions_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.restaurant_spaces(id) ON DELETE CASCADE;


--
-- Name: space_scan_sessions space_scan_sessions_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_sessions
    ADD CONSTRAINT space_scan_sessions_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: space_scan_sessions space_scan_sessions_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_sessions
    ADD CONSTRAINT space_scan_sessions_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.restaurant_spaces(id) ON DELETE SET NULL;


--
-- Name: space_scan_uploads space_scan_uploads_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_uploads
    ADD CONSTRAINT space_scan_uploads_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: space_scan_uploads space_scan_uploads_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.space_scan_uploads
    ADD CONSTRAINT space_scan_uploads_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.space_scan_sessions(id) ON DELETE CASCADE;


--
-- Name: staff_memberships staff_memberships_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_memberships
    ADD CONSTRAINT staff_memberships_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: staff_memberships staff_memberships_identity_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_memberships
    ADD CONSTRAINT staff_memberships_identity_id_fkey FOREIGN KEY (identity_id) REFERENCES public.staff_identities(id) ON DELETE RESTRICT;


--
-- Name: staff_memberships staff_memberships_legacy_staff_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.staff_memberships
    ADD CONSTRAINT staff_memberships_legacy_staff_id_fkey FOREIGN KEY (legacy_staff_id) REFERENCES public.staff(id) ON DELETE CASCADE;


--
-- Name: table_combination_members table_combination_members_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combination_members
    ADD CONSTRAINT table_combination_members_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: table_combination_members table_combination_members_combination_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combination_members
    ADD CONSTRAINT table_combination_members_combination_id_fkey FOREIGN KEY (combination_id) REFERENCES public.table_combinations(id) ON DELETE CASCADE;


--
-- Name: table_combination_members table_combination_members_table_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combination_members
    ADD CONSTRAINT table_combination_members_table_id_fkey FOREIGN KEY (table_id) REFERENCES public.tables(id) ON DELETE CASCADE;


--
-- Name: table_combinations table_combinations_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combinations
    ADD CONSTRAINT table_combinations_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- Name: table_combinations table_combinations_primary_table_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.table_combinations
    ADD CONSTRAINT table_combinations_primary_table_id_fkey FOREIGN KEY (primary_table_id) REFERENCES public.tables(id) ON DELETE CASCADE;


--
-- Name: tables tables_region_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tables
    ADD CONSTRAINT tables_region_id_fkey FOREIGN KEY (region_id) REFERENCES public.space_regions(id) ON DELETE SET NULL;


--
-- Name: tables tables_space_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tables
    ADD CONSTRAINT tables_space_id_fkey FOREIGN KEY (space_id) REFERENCES public.restaurant_spaces(id) ON DELETE SET NULL;


--
-- Name: whatsapp_business_devices whatsapp_business_devices_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.whatsapp_business_devices
    ADD CONSTRAINT whatsapp_business_devices_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--


