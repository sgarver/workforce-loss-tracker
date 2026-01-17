-- Insert sample industries
INSERT INTO industries (name, slug) VALUES 
('SaaS', 'saas'),
('FinTech', 'fintech'),
('HealthTech', 'healthtech'),
('E-commerce', 'ecommerce'),
('AI/ML', 'ai-ml'),
('Gaming', 'gaming'),
('Social Media', 'social-media'),
('Cloud Computing', 'cloud-computing'),
('Cybersecurity', 'cybersecurity'),
('EdTech', 'edtech'),
('Transportation', 'transportation'),
('Real Estate Tech', 'realestate-tech'),
('HR Tech', 'hr-tech'),
('Marketing Tech', 'marketing-tech'),
('Hardware', 'hardware');

-- Insert sample companies
INSERT INTO companies (name, employee_count, industry_id, website) VALUES 
('TechCorp Inc', 5000, 1, 'https://techcorp.com'),
('DataFlow Systems', 1200, 5, 'https://dataflow.io'),
('PaySecure', 800, 2, 'https://paysecure.com'),
('CloudNet', 3500, 8, 'https://cloudnet.net'),
('GameStudio Pro', 300, 6, 'https://gamestudio.com'),
('HealthPlus', 1500, 3, 'https://healthplus.com'),
('ShopNow', 2000, 4, 'https://shopnow.com'),
('SocialHub', 1800, 7, 'https://socialhub.com'),
('SecureIT', 450, 9, 'https://secureit.com'),
('EduLearn', 250, 10, 'https://edulearn.com'),
('TranspoGo', 900, 11, 'https://transpogo.com'),
('PropTech', 180, 12, 'https://proptech.com'),
('HRTech', 320, 13, 'https://hrtech.com'),
('MarketAI', 600, 14, 'https://marketai.com'),
('DeviceCo', 4200, 15, 'https://deviceco.com');

-- Insert sample layoff data
INSERT INTO layoffs (company_id, employees_affected, layoff_date, source_url, notes) VALUES 
(1, 500, '2024-01-15', 'https://techcrunch.com/techcorp-layoffs', 'Restructuring due to market conditions'),
(2, 150, '2024-01-20', 'https://news.com/dataflow-layoffs', 'AI integration reducing workforce'),
(3, 80, '2024-02-01', 'https://fintechnews.com/paysecure-cuts', 'Funding round delays'),
(4, 300, '2024-02-10', 'https://cloudnews.com/cloudnet-layoffs', 'Infrastructure optimization'),
(5, 50, '2024-02-15', 'https://gamingnews.com/gamestudio-cuts', 'Project cancellation'),
(6, 120, '2024-03-01', 'https://healthnews.com/healthplus-layoffs', 'Pilot program ended'),
(7, 200, '2024-03-05', 'https://ecomnews.com/shopnow-layoffs', 'Competitive pressure'),
(8, 180, '2024-03-10', 'https://socialnews.com/socialhub-cuts', 'Revenue shortfall'),
(2, 75, '2024-03-15', 'https://techcrunch.com/dataflow-more-cuts', 'Further restructuring'),
(1, 200, '2024-03-20', 'https://news.com/techcorp-more-layoffs', 'Q1 adjustments'),
(9, 40, '2024-04-01', 'https://secnews.com/secureit-layoffs', 'Contract losses'),
(10, 25, '2024-04-05', 'https://edunews.com/edulearn-cuts', 'Funding reduction'),
(11, 60, '2024-04-10', 'https://transportnews.com/transpogo-layoffs', 'Fleet automation'),
(12, 30, '2024-04-15', 'https://renews.com/proptech-cuts', 'Market slowdown'),
(13, 45, '2024-04-20', 'https://hrnews.com/hrtech-layoffs', 'Product pivot'),
(14, 70, '2024-05-01', 'https://marketingnews.com/marketai-cuts', 'AI efficiency gains'),
(15, 400, '2024-05-05', 'https://hardnews.com/deviceco-layoffs', 'Supply chain issues');

-- Insert sample sponsored listings
INSERT INTO sponsored_listings (company_id, start_date, end_date, message, status) VALUES 
(10, '2024-01-01', '2024-12-31', 'We are hiring! Check out our open positions.', 'active'),
(12, '2024-02-01', '2024-11-30', 'Now hiring engineers and product managers.', 'active'),
(13, '2024-03-01', '2024-12-31', 'Join our growing team!', 'active');