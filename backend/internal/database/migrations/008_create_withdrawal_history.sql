-- Create withdrawal_history table for tracking business withdrawals
CREATE TABLE IF NOT EXISTS withdrawal_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    business_id INTEGER NOT NULL,
    transaction_hash TEXT UNIQUE NOT NULL,
    payment_amount REAL DEFAULT 0,
    tip_amount REAL DEFAULT 0,
    total_amount REAL NOT NULL,
    withdrawal_address TEXT NOT NULL,
    blockchain_network TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    confirmed_at DATETIME,
    FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE CASCADE
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_withdrawal_history_business_id ON withdrawal_history(business_id);
CREATE INDEX IF NOT EXISTS idx_withdrawal_history_status ON withdrawal_history(status);
CREATE INDEX IF NOT EXISTS idx_withdrawal_history_created_at ON withdrawal_history(created_at);
CREATE INDEX IF NOT EXISTS idx_withdrawal_history_tx_hash ON withdrawal_history(transaction_hash);
