-- Fix token_id unique constraint issue
-- SQLite doesn't support dropping constraints directly, so we need to recreate the table

-- Create new users table without token_id unique constraint
CREATE TABLE users_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    address TEXT NOT NULL UNIQUE,
    referral_code TEXT UNIQUE,
    token_id TEXT,
    email_enabled BOOLEAN DEFAULT TRUE,
    news_enabled BOOLEAN DEFAULT TRUE,
    updates_enabled BOOLEAN DEFAULT TRUE,
    transactional_enabled BOOLEAN DEFAULT TRUE,
    security_enabled BOOLEAN DEFAULT TRUE,
    reports_enabled BOOLEAN DEFAULT TRUE,
    statistics_enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME,
    updated_at DATETIME
);

-- Copy data from old table to new table
INSERT INTO users_new (id, address, referral_code, token_id, email_enabled, news_enabled, updates_enabled, transactional_enabled, security_enabled, reports_enabled, statistics_enabled, created_at, updated_at)
SELECT id, address, referral_code, CASE WHEN token_id = '' THEN NULL ELSE token_id END, email_enabled, news_enabled, updates_enabled, transactional_enabled, security_enabled, reports_enabled, statistics_enabled, created_at, updated_at
FROM users;

-- Drop old table
DROP TABLE users;

-- Rename new table to users
ALTER TABLE users_new RENAME TO users;

-- Recreate indexes
CREATE UNIQUE INDEX idx_users_address ON users(address);
CREATE UNIQUE INDEX idx_users_referral_code ON users(referral_code);
