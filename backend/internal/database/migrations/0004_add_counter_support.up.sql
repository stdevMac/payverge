-- Add counter support to businesses table
ALTER TABLE businesses ADD COLUMN counter_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE businesses ADD COLUMN counter_count INTEGER DEFAULT 3;
ALTER TABLE businesses ADD COLUMN counter_prefix TEXT DEFAULT 'C';

-- Create counters table for tracking counter states
CREATE TABLE IF NOT EXISTS counters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    business_id INTEGER NOT NULL,
    counter_number INTEGER NOT NULL,
    name TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    current_bill_id INTEGER NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE CASCADE,
    FOREIGN KEY (current_bill_id) REFERENCES bills(id) ON DELETE SET NULL,
    UNIQUE(business_id, counter_number)
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_counters_business_id ON counters(business_id);
CREATE INDEX IF NOT EXISTS idx_counters_active ON counters(business_id, is_active);
CREATE INDEX IF NOT EXISTS idx_counters_current_bill ON counters(current_bill_id);

-- Add counter_id to bills table to track which counter a bill belongs to
ALTER TABLE bills ADD COLUMN counter_id INTEGER NULL;
CREATE INDEX IF NOT EXISTS idx_bills_counter_id ON bills(counter_id);

-- Update existing businesses to have default counter configuration
UPDATE businesses SET 
    counter_enabled = FALSE,
    counter_count = 3,
    counter_prefix = 'C'
WHERE counter_enabled IS NULL;
