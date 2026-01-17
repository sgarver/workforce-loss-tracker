-- Industries table
CREATE TABLE industries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Companies table
CREATE TABLE companies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    employee_count INTEGER,
    industry_id INTEGER,
    website TEXT,
    logo_url TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (industry_id) REFERENCES industries(id)
);

-- Layoffs table
CREATE TABLE layoffs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company_id INTEGER NOT NULL,
    employees_affected INTEGER NOT NULL,
    layoff_date DATE NOT NULL,
    source_url TEXT,
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
);

-- Sponsored listings table
CREATE TABLE sponsored_listings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company_id INTEGER NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    message TEXT,
    status TEXT DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
);

-- Indexes for better performance
CREATE INDEX idx_companies_industry ON companies(industry_id);
CREATE INDEX idx_layoffs_company ON layoffs(company_id);
CREATE INDEX idx_layoffs_date ON layoffs(layoff_date);
CREATE INDEX idx_sponsored_company ON sponsored_listings(company_id);
CREATE INDEX idx_sponsored_dates ON sponsored_listings(start_date, end_date);