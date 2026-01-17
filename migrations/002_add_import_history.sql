-- Import history table for tracking automated updates
CREATE TABLE IF NOT EXISTS import_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_url TEXT NOT NULL,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    record_count INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    status TEXT DEFAULT 'completed',
    error_message TEXT,
    duration_ms INTEGER
);

-- Indexes for import history (only create if they don't exist)
CREATE INDEX IF NOT EXISTS idx_import_history_url ON import_history(source_url);
CREATE INDEX IF NOT EXISTS idx_import_history_date ON import_history(imported_at);