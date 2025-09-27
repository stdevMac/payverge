-- Create referrers table
CREATE TABLE IF NOT EXISTS referrers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    wallet_address TEXT UNIQUE NOT NULL,
    referral_code TEXT UNIQUE NOT NULL,
    tier TEXT NOT NULL CHECK (tier IN ('basic', 'premium')),
    status TEXT DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'suspended')),
    total_referrals INTEGER DEFAULT 0,
    total_commissions TEXT DEFAULT '0',
    claimable_commissions TEXT DEFAULT '0',
    last_claimed_at DATETIME,
    registration_tx_hash TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create referral_records table
CREATE TABLE IF NOT EXISTS referral_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    referrer_id INTEGER NOT NULL,
    business_id INTEGER NOT NULL,
    registration_fee TEXT,
    discount TEXT,
    commission TEXT,
    commission_paid BOOLEAN DEFAULT FALSE,
    commission_tx_hash TEXT,
    processed_tx_hash TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (referrer_id) REFERENCES referrers(id) ON DELETE CASCADE,
    FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE CASCADE
);

-- Create referral_commission_claims table
CREATE TABLE IF NOT EXISTS referral_commission_claims (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    referrer_id INTEGER NOT NULL,
    amount TEXT,
    tx_hash TEXT,
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'failed')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (referrer_id) REFERENCES referrers(id) ON DELETE CASCADE
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_referrers_wallet_address ON referrers(wallet_address);
CREATE INDEX IF NOT EXISTS idx_referrers_referral_code ON referrers(referral_code);
CREATE INDEX IF NOT EXISTS idx_referrers_tier ON referrers(tier);
CREATE INDEX IF NOT EXISTS idx_referrers_status ON referrers(status);

CREATE INDEX IF NOT EXISTS idx_referral_records_referrer_id ON referral_records(referrer_id);
CREATE INDEX IF NOT EXISTS idx_referral_records_business_id ON referral_records(business_id);
CREATE INDEX IF NOT EXISTS idx_referral_records_commission_paid ON referral_records(commission_paid);

CREATE INDEX IF NOT EXISTS idx_referral_commission_claims_referrer_id ON referral_commission_claims(referrer_id);
CREATE INDEX IF NOT EXISTS idx_referral_commission_claims_status ON referral_commission_claims(status);

-- Add referral_code field to businesses table if it doesn't exist
-- This will track which referral code was used when registering the business
ALTER TABLE businesses ADD COLUMN referred_by_code TEXT;
CREATE INDEX IF NOT EXISTS idx_businesses_referred_by_code ON businesses(referred_by_code);
