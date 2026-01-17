-- Create comments table
CREATE TABLE comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    layoff_id INTEGER NOT NULL,
    author_name TEXT NOT NULL,
    author_email TEXT,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (layoff_id) REFERENCES layoffs(id) ON DELETE CASCADE
);

-- Create index for performance
CREATE INDEX idx_comments_layoff_id ON comments(layoff_id);
CREATE INDEX idx_comments_created_at ON comments(created_at);