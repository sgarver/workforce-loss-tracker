-- Add email/password auth fields to users table
ALTER TABLE users ADD COLUMN password_hash TEXT;
ALTER TABLE users ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN verification_token TEXT;
ALTER TABLE users ADD COLUMN verification_expires_at DATETIME;
ALTER TABLE users ADD COLUMN reset_token TEXT;
ALTER TABLE users ADD COLUMN reset_expires_at DATETIME;
ALTER TABLE users ADD COLUMN last_login_at DATETIME;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique ON users(email);
