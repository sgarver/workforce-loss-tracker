-- Add source_type column to differentiate data sources
ALTER TABLE layoffs ADD COLUMN source_type TEXT NOT NULL DEFAULT 'unknown';

-- Add check constraint for valid enum values
-- Note: Some SQLite versions don't support CHECK constraints, so we'll handle validation in application code

-- Backfill existing data as WARN source
UPDATE layoffs SET source_type = 'warn' WHERE source_type = 'unknown';

-- Create index for potential filtering by source type
CREATE INDEX idx_layoffs_source_type ON layoffs(source_type);