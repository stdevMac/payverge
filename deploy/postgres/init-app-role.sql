-- Runs once, when the postgres service initialises an empty data directory
-- (mounted into /docker-entrypoint-initdb.d by deploy/docker-compose.yml; the
-- image's entrypoint feeds it to psql as the superuser, with ON_ERROR_STOP).
--
-- The entrypoint creates POSTGRES_USER (postgres) as the superuser and
-- POSTGRES_DB. This file then:
--
--   1. creates PAYVERGE_DB_USER, the role the backend, backup and restore log
--      in as, with DB_PASSWORD: LOGIN, NOSUPERUSER, NOCREATEROLE,
--      NOREPLICATION, NOBYPASSRLS. It keeps CREATEDB because restore.sh
--      restores into a temporary database and swaps it in;
--   2. hands POSTGRES_DB and its public schema to it, so it owns the schema
--      and runs the migrations (pg_trgm, the one extension the schema needs,
--      is a trusted extension a database owner may create);
--   3. removes the superuser's password. The superuser can then only connect
--      over the container's local socket (docker compose exec postgres psql
--      -U postgres), never over the network, so a leaked DB_PASSWORD gives
--      the database, not the server.
--
-- SQL rather than a shell script: the entrypoint runs a .sh only when the
-- mounted file is executable, which a download or a bind mount may not keep.
-- \getenv keeps the password off any command line.
\getenv app_user PAYVERGE_DB_USER
\getenv app_password POSTGRES_PASSWORD
\getenv dbname POSTGRES_DB

SELECT set_config('payverge.app_user', :'app_user', false);
DO $$
BEGIN
	IF current_setting('payverge.app_user') = '' THEN
		RAISE EXCEPTION 'init-app-role: PAYVERGE_DB_USER (DB_USER) is empty';
	END IF;
	IF current_setting('payverge.app_user') = current_user THEN
		RAISE EXCEPTION 'init-app-role: DB_USER must not be ''%'' (the superuser); pick another name in .env', current_user;
	END IF;
END
$$;

CREATE ROLE :"app_user" LOGIN NOSUPERUSER NOCREATEROLE CREATEDB NOREPLICATION NOBYPASSRLS
	PASSWORD :'app_password';
ALTER DATABASE :"dbname" OWNER TO :"app_user";
ALTER SCHEMA public OWNER TO :"app_user";
SELECT format('ALTER ROLE %I PASSWORD NULL', current_user) \gexec
\echo init-app-role: role :app_user owns database :dbname (not a superuser)
