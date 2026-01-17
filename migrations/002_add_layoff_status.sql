-- Add status column to layoffs table
ALTER TABLE layoffs ADD COLUMN status TEXT DEFAULT 'completed';