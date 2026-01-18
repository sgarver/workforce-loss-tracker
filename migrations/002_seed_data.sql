-- Insert sample industries
INSERT INTO industries (name, slug) VALUES
('Technology', 'technology'),
('Healthcare', 'healthcare'),
('Retail', 'retail'),
('Manufacturing', 'manufacturing'),
('Finance', 'finance'),
('Education', 'education'),
('Hospitality', 'hospitality'),
('Transportation', 'transportation'),
('Construction', 'construction'),
('Energy', 'energy'),
('Entertainment', 'entertainment'),
('Government', 'government'),
('Non-Profit', 'non-profit'),
('Agriculture', 'agriculture'),
('Real Estate', 'real-estate');

-- Insert sample companies
INSERT INTO companies (name, employee_count, industry_id, website) VALUES
('TechCorp Inc', 5000, 1, 'https://techcorp.com'),
('MedCare Solutions', 1500, 2, 'https://medcare.com'),
('RetailMart', 2000, 3, 'https://retailmart.com'),
('AutoParts Inc', 1200, 4, 'https://autoparts.com'),
('FinanceFirst', 800, 5, 'https://financefirst.com'),
('EduLearn', 250, 6, 'https://edulearn.com'),
('HotelChain', 900, 7, 'https://hotelchain.com'),
('TranspoGo', 900, 8, 'https://transpogo.com'),
('BuildCorp', 600, 9, 'https://buildcorp.com'),
('EnergyCorp', 1800, 10, 'https://energycorp.com'),
('MovieStudio', 450, 11, 'https://moviestudio.com'),
('CityServices', 320, 12, 'https://cityservices.gov'),
('AidFoundation', 180, 13, 'https://aidfoundation.org'),
('FarmCo', 4200, 14, 'https://farmco.com'),
('PropManage', 350, 15, 'https://propmanage.com');

-- Insert sample layoff data
INSERT INTO layoffs (company_id, employees_affected, layoff_date, source_url, notes) VALUES
(1, 500, '2024-01-15', 'https://technews.com/techcorp-layoffs', 'Restructuring due to market conditions'),
(2, 120, '2024-01-20', 'https://healthnews.com/medcare-layoffs', 'Healthcare cost reductions'),
(3, 200, '2024-02-01', 'https://retailnews.com/retailmart-layoffs', 'Store closures and online shift'),
(4, 150, '2024-02-10', 'https://manufacturingnews.com/autoparts-layoffs', 'Supply chain disruptions'),
(5, 80, '2024-02-15', 'https://financenews.com/financefirst-layoffs', 'Banking consolidation'),
(6, 25, '2024-03-01', 'https://edunews.com/edulearn-cuts', 'Budget cuts in education'),
(7, 90, '2024-03-05', 'https://hospitalitynews.com/hotelchain-layoffs', 'Post-pandemic adjustments'),
(8, 60, '2024-03-10', 'https://transportnews.com/transpogo-layoffs', 'Fleet automation'),
(9, 70, '2024-03-15', 'https://constructionnews.com/buildcorp-layoffs', 'Housing market slowdown'),
(10, 180, '2024-03-20', 'https://energynews.com/energycorp-layoffs', 'Energy transition'),
(11, 40, '2024-04-01', 'https://entertainmentnews.com/moviestudio-cuts', 'Streaming competition'),
(12, 45, '2024-04-05', 'https://govnews.com/cityservices-layoffs', 'Government budget constraints'),
(13, 30, '2024-04-10', 'https://nonprofitnews.com/aidfoundation-cuts', 'Funding reduction'),
(14, 400, '2024-04-15', 'https://agnews.com/farmco-layoffs', 'Agricultural market changes'),
(15, 35, '2024-04-20', 'https://realestatenews.com/propmanage-layoffs', 'Commercial real estate slowdown');

-- Insert sample sponsored listings
INSERT INTO sponsored_listings (company_id, start_date, end_date, message, status) VALUES
(6, '2024-01-01', '2024-12-31', 'We are hiring teachers! Check out our open positions.', 'active'),
(15, '2024-02-01', '2024-11-30', 'Now hiring property managers.', 'active'),
(1, '2024-03-01', '2024-12-31', 'Join our growing tech team!', 'active');