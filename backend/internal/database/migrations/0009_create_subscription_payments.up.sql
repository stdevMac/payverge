-- Create subscription_payments table for tracking subscription renewal payments
CREATE TABLE IF NOT EXISTS subscription_payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    business_id INTEGER NOT NULL,
    payment_amount TEXT NOT NULL,
    transaction_hash TEXT NOT NULL UNIQUE,
    block_number INTEGER NOT NULL,
    new_expiry_time INTEGER NOT NULL,
    payment_date DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE CASCADE
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_subscription_payments_business_id ON subscription_payments(business_id);
CREATE INDEX IF NOT EXISTS idx_subscription_payments_payment_date ON subscription_payments(payment_date DESC);
CREATE INDEX IF NOT EXISTS idx_subscription_payments_transaction_hash ON subscription_payments(transaction_hash);
CREATE INDEX IF NOT EXISTS idx_subscription_payments_block_number ON subscription_payments(block_number);

-- Create trigger to update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_subscription_payments_updated_at 
    AFTER UPDATE ON subscription_payments
    FOR EACH ROW
BEGIN
    UPDATE subscription_payments SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
