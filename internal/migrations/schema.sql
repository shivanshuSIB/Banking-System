-- Banking System Schema
-- Money is stored in INR (int64) to avoid floating-point issues.
-- Double-entry: every movement creates a debit and a credit journal entry.
-- This file is idempotent: safe to re-run.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Accounts hold a current balance (cached for fast reads).
CREATE TABLE IF NOT EXISTS accounts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    currency    CHAR(3)      NOT NULL DEFAULT 'INR',
    balance     BIGINT       NOT NULL DEFAULT 0,   -- in rupees, always >= 0
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT balance_non_negative CHECK (balance >= 0)
);

-- A transaction groups one or more journal entries.
-- type:   DEPOSIT | WITHDRAWAL | TRANSFER | REVERSAL
-- status: PENDING | SUCCESS | FAILED
CREATE TABLE IF NOT EXISTS transactions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type                VARCHAR(20)  NOT NULL,
    status              VARCHAR(10)  NOT NULL DEFAULT 'PENDING',
    amount              BIGINT       NOT NULL,
    from_account_id     UUID         REFERENCES accounts(id),
    to_account_id       UUID         REFERENCES accounts(id),
    reversal_of         UUID         REFERENCES transactions(id),
    idempotency_key     VARCHAR(255) UNIQUE,
    note                TEXT,
    error_message       TEXT,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT amount_positive CHECK (amount > 0)
);

-- Journal entries (double-entry ledger).
-- amount is signed: positive = credit to account, negative = debit from account.
CREATE TABLE IF NOT EXISTS journal_entries (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID        NOT NULL REFERENCES transactions(id),
    account_id     UUID        NOT NULL REFERENCES accounts(id),
    amount         BIGINT      NOT NULL,
    direction      VARCHAR(6)  NOT NULL,   -- 'DEBIT' or 'CREDIT'
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_journal_entries_transaction ON journal_entries(transaction_id);
CREATE INDEX IF NOT EXISTS idx_journal_entries_account     ON journal_entries(account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_from_account   ON transactions(from_account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_to_account     ON transactions(to_account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_reversal_of    ON transactions(reversal_of);

-- Seed demo accounts (ON CONFLICT DO NOTHING makes this idempotent).
INSERT INTO accounts (id, name, currency, balance) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Alice',   'INR', 1000),
    ('22222222-2222-2222-2222-222222222222', 'Bob',     'INR',  500),
    ('33333333-3333-3333-3333-333333333333', 'Charlie', 'INR',  250)
ON CONFLICT (id) DO NOTHING;
