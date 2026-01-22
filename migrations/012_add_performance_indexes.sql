-- Add performance indexes for stats queries
CREATE INDEX idx_layoffs_date_company ON layoffs(layoff_date, company_id);
CREATE INDEX idx_layoffs_company_date_employees ON layoffs(company_id, layoff_date, employees_affected);