-- Create company mappings table for dynamic company name normalization
CREATE TABLE company_mappings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    original_name TEXT UNIQUE NOT NULL,
    canonical_name TEXT NOT NULL,
    mapping_type TEXT DEFAULT 'auto', -- 'auto', 'manual', 'regex'
    confidence_score INTEGER DEFAULT 100,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add canonical name reference to companies table
ALTER TABLE companies ADD COLUMN canonical_name TEXT;
ALTER TABLE companies ADD COLUMN mapping_id INTEGER REFERENCES company_mappings(id);

-- Create indexes for performance
CREATE INDEX idx_company_mappings_original ON company_mappings(original_name);
CREATE INDEX idx_company_mappings_canonical ON company_mappings(canonical_name);
CREATE INDEX idx_companies_canonical ON companies(canonical_name);