-- Database indexes for performance optimization
-- Analytics and reporting query optimization

-- Bills table indexes for analytics queries
CREATE INDEX IF NOT EXISTS idx_bills_business_id ON bills(business_id);
CREATE INDEX IF NOT EXISTS idx_bills_created_at ON bills(created_at);
CREATE INDEX IF NOT EXISTS idx_bills_status ON bills(status);
CREATE INDEX IF NOT EXISTS idx_bills_business_created ON bills(business_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bills_business_status ON bills(business_id, status);

-- Payments table indexes for transaction analytics
CREATE INDEX IF NOT EXISTS idx_payments_bill_id ON payments(bill_id);
CREATE INDEX IF NOT EXISTS idx_payments_created_at ON payments(created_at);
CREATE INDEX IF NOT EXISTS idx_payments_payer_addr ON payments(payer_addr);
CREATE INDEX IF NOT EXISTS idx_payments_bill_created ON payments(bill_id, created_at);

-- Business table indexes for lookups
CREATE INDEX IF NOT EXISTS idx_businesses_owner_address ON businesses(owner_address);
CREATE INDEX IF NOT EXISTS idx_businesses_created_at ON businesses(created_at);

-- Tables table indexes for bill associations
CREATE INDEX IF NOT EXISTS idx_tables_business_id ON tables(business_id);
CREATE INDEX IF NOT EXISTS idx_tables_code ON tables(code);

-- Composite indexes for common analytics queries
CREATE INDEX IF NOT EXISTS idx_bills_analytics ON bills(business_id, created_at, status, total_amount);
CREATE INDEX IF NOT EXISTS idx_payments_analytics ON payments(bill_id, created_at, amount, tip_amount);

-- Alternative payments table indexes
CREATE INDEX IF NOT EXISTS idx_alternative_payments_bill_id ON alternative_payments(bill_id);
CREATE INDEX IF NOT EXISTS idx_alternative_payments_status ON alternative_payments(status);
CREATE INDEX IF NOT EXISTS idx_alternative_payments_participant ON alternative_payments(participant_addr);
CREATE INDEX IF NOT EXISTS idx_alternative_payments_created_at ON alternative_payments(created_at);
CREATE INDEX IF NOT EXISTS idx_alternative_payments_bill_status ON alternative_payments(bill_id, status);

-- Indexes for date range queries (common in analytics)
CREATE INDEX IF NOT EXISTS idx_bills_date_range ON bills(business_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payments_date_range ON payments(created_at DESC, amount);
CREATE INDEX IF NOT EXISTS idx_alternative_payments_date_range ON alternative_payments(created_at DESC, amount);
