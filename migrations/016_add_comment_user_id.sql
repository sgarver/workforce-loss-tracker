-- Add user_id to comments for authenticated authors
ALTER TABLE comments ADD COLUMN user_id INTEGER;

CREATE INDEX IF NOT EXISTS idx_comments_user_id ON comments(user_id);
