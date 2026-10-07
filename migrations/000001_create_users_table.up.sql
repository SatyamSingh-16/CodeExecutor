-- 000001_create_users_table.up.sql
-- Creates the users table with UUID primary key, unique email, password hash, and timestamps.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_users_email UNIQUE (email),
    CONSTRAINT chk_users_email_not_empty CHECK (char_length(trim(email)) > 0),
    CONSTRAINT chk_users_password_hash_not_empty CHECK (char_length(trim(password_hash)) > 0)
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
