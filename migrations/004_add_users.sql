-- Create users table for membership system
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL, -- e.g., 'google', 'github'
    provider_id TEXT NOT NULL UNIQUE, -- ID from the provider
    email TEXT,
    name TEXT,
    avatar_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, provider_id)
);