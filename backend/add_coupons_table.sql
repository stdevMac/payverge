-- Add coupons table migration
CREATE TABLE IF NOT EXISTS `coupons` (
  `id` integer PRIMARY KEY AUTOINCREMENT,
  `created_at` datetime,
  `updated_at` datetime,
  `deleted_at` datetime,
  `code` text NOT NULL UNIQUE,
  `hash` text NOT NULL UNIQUE,
  `discount_amount` text NOT NULL,
  `expiry_time` datetime,
  `is_active` numeric DEFAULT 1,
  `is_used` numeric DEFAULT 0,
  `used_by` text,
  `used_at` datetime,
  `created_by` text,
  `blockchain_tx_hash` text,
  `blockchain_block_number` integer,
  `notes` text
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS `idx_coupons_deleted_at` ON `coupons`(`deleted_at`);
CREATE INDEX IF NOT EXISTS `idx_coupons_code` ON `coupons`(`code`);
CREATE INDEX IF NOT EXISTS `idx_coupons_hash` ON `coupons`(`hash`);
CREATE INDEX IF NOT EXISTS `idx_coupons_is_active` ON `coupons`(`is_active`);
CREATE INDEX IF NOT EXISTS `idx_coupons_is_used` ON `coupons`(`is_used`);
CREATE INDEX IF NOT EXISTS `idx_coupons_used_by` ON `coupons`(`used_by`);
CREATE INDEX IF NOT EXISTS `idx_coupons_expiry_time` ON `coupons`(`expiry_time`);
