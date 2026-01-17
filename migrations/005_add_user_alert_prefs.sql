-- Add user alert preferences table
CREATE TABLE IF NOT EXISTS user_alert_prefs (
    user_id INTEGER PRIMARY KEY,
    email_alerts_enabled BOOLEAN DEFAULT 1,
    alert_new_data BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);