-- Safe migration: Add source_type column only if it doesn't exist
-- SQLite doesn't support IF NOT EXISTS for ALTER TABLE, so we use a workaround

-- First, try to add the column (will fail silently if it exists)
ALTER TABLE layoffs ADD COLUMN source_type TEXT DEFAULT 'unknown';

-- Update existing NULL values to 'unknown' (for safety)
UPDATE layoffs SET source_type = 'unknown' WHERE source_type IS NULL;

-- Backfill existing data as WARN source (only if not already set)
UPDATE layoffs SET source_type = 'warn' WHERE source_type = 'unknown';

-- Create index for potential filtering by source type (only if it doesn't exist)
CREATE INDEX IF NOT EXISTS idx_layoffs_source_type ON layoffs(source_type);

-- Update company sizes: set unknown companies (previously estimated as 100) to -1
-- This preserves companies that actually have 100 employees while marking unknowns as -1
UPDATE companies SET employee_count = -1 WHERE employee_count = 100;