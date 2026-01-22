-- Add industry classification metadata columns to companies table
ALTER TABLE companies ADD COLUMN industry_method TEXT;
ALTER TABLE companies ADD COLUMN industry_confidence INTEGER;
ALTER TABLE companies ADD COLUMN industry_source TEXT;