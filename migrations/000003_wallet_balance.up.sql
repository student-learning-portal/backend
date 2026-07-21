-- ============================================================
-- Virtual wallet balance (sandbox money, no real funds)
-- ============================================================
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS wallet_balance NUMERIC(12,2) NOT NULL DEFAULT 1000.00;
