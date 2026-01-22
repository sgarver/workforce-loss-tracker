-- Drop the old industry_id column and foreign key
-- Note: SQLite doesn't support DROP COLUMN, so we'll recreate the table
CREATE TABLE companies_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    employee_count INTEGER,
    industry TEXT,
    website TEXT,
    logo_url TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert companies with industry strings
INSERT INTO companies_new (id, name, employee_count, industry, website, logo_url, created_at, updated_at) VALUES
(1, 'TechCorp Inc', 5000, 'Technology', 'https://techcorp.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(2, 'MedCare Solutions', 1500, 'Healthcare', 'https://medcare.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(3, 'RetailMart', 2000, 'Retail', 'https://retailmart.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(4, 'AutoParts Inc', 1200, 'Manufacturing', 'https://autoparts.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(5, 'FinanceFirst', 800, 'Finance', 'https://financefirst.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(6, 'EduLearn', 250, 'Education', 'https://edulearn.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(7, 'HotelChain', 900, 'Hospitality', 'https://hotelchain.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(8, 'TranspoGo', 900, 'Transportation', 'https://transpogo.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(9, 'BuildCorp', 600, 'Construction', 'https://buildcorp.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(10, 'EnergyCorp', 1800, 'Energy', 'https://energycorp.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(11, 'MovieStudio', 450, 'Entertainment', 'https://moviestudio.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(12, 'CityServices', 320, 'Government', 'https://cityservices.gov', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(13, 'AidFoundation', 180, 'Non-Profit', 'https://aidfoundation.org', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(14, 'FarmCo', 4200, 'Agriculture', 'https://farmco.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
(15, 'PropManage', 350, 'Real Estate', 'https://propmanage.com', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- Re-insert layoffs with updated company references
INSERT INTO layoffs (company_id, employees_affected, layoff_date, source_url, notes, status, created_at) VALUES
(1, 500, '2024-01-15', 'https://technews.com/techcorp-layoffs', 'Restructuring due to market conditions', 'approved', CURRENT_TIMESTAMP),
(2, 120, '2024-01-20', 'https://healthnews.com/medcare-layoffs', 'Healthcare cost reductions', 'approved', CURRENT_TIMESTAMP),
(3, 200, '2024-02-01', 'https://retailnews.com/retailmart-layoffs', 'Store closures and online shift', 'approved', CURRENT_TIMESTAMP),
(4, 150, '2024-02-10', 'https://manufacturingnews.com/autoparts-layoffs', 'Supply chain disruptions', 'approved', CURRENT_TIMESTAMP),
(5, 80, '2024-02-15', 'https://financenews.com/financefirst-layoffs', 'Banking consolidation', 'approved', CURRENT_TIMESTAMP),
(6, 25, '2024-03-01', 'https://edunews.com/edulearn-cuts', 'Budget cuts in education', 'approved', CURRENT_TIMESTAMP),
(7, 90, '2024-03-05', 'https://hospitalitynews.com/hotelchain-layoffs', 'Post-pandemic adjustments', 'approved', CURRENT_TIMESTAMP),
(8, 60, '2024-03-10', 'https://transportnews.com/transpogo-layoffs', 'Fleet automation', 'approved', CURRENT_TIMESTAMP),
(9, 70, '2024-03-15', 'https://constructionnews.com/buildcorp-layoffs', 'Housing market slowdown', 'approved', CURRENT_TIMESTAMP),
(10, 180, '2024-03-20', 'https://energynews.com/energycorp-layoffs', 'Energy transition', 'approved', CURRENT_TIMESTAMP),
(11, 40, '2024-04-01', 'https://entertainmentnews.com/moviestudio-cuts', 'Streaming competition', 'approved', CURRENT_TIMESTAMP),
(12, 45, '2024-04-05', 'https://govnews.com/cityservices-layoffs', 'Government budget constraints', 'approved', CURRENT_TIMESTAMP),
(13, 30, '2024-04-10', 'https://nonprofitnews.com/aidfoundation-cuts', 'Funding reduction', 'approved', CURRENT_TIMESTAMP),
(14, 400, '2024-04-15', 'https://agnews.com/farmco-layoffs', 'Agricultural market changes', 'approved', CURRENT_TIMESTAMP),
(15, 35, '2024-04-20', 'https://realestatenews.com/propmanage-layoffs', 'Commercial real estate slowdown', 'approved', CURRENT_TIMESTAMP);

DROP TABLE companies;
ALTER TABLE companies_new RENAME TO companies;

-- Recreate indexes
CREATE INDEX idx_companies_industry ON companies(industry);