package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"layoff-tracker/internal/database"
	"layoff-tracker/internal/models"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type FreeDataService struct {
	db *database.DB
}

func NewFreeDataService(db *database.DB) *FreeDataService {
	return &FreeDataService{db: db}
}

// Import data from GitHub CSV

func (s *FreeDataService) GetImportStats() (*ImportStats, error) {
	stats := &ImportStats{}

	// Get counts from database
	err := s.db.QueryRow("SELECT COUNT(*) FROM companies").Scan(&stats.TotalCompanies)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRow("SELECT COUNT(*) FROM layoffs").Scan(&stats.TotalLayoffs)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRow("SELECT COALESCE(SUM(employees_affected), 0) FROM layoffs").Scan(&stats.TotalEmployees)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

type ImportStats struct {
	TotalCompanies int
	TotalLayoffs   int
	TotalEmployees int
}

// Free data source handler for Echo
// Import from WARN Database (official government data via layoffdata.com)
func (s *FreeDataService) ImportFromWARNDatabase() error {
	// WARN Database CSV export URLs - both current and historical for comprehensive data
	urls := []string{
		"https://docs.google.com/spreadsheets/d/1ayO8dl7sXaIYBAwkBGRUjbDms6MAbZFvvxxRp8IyxvY/export?format=csv", // Historical data first
		"https://docs.google.com/spreadsheets/d/1Qx6lv3zAL9YTsKJQNALa2GqBLXq0RER2lHvzyx32pRs/export?format=csv", // 2025 data
	}

	totalImported := 0

	for _, url := range urls {
		log.Printf("Importing from WARN Database URL: %s", url)

		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error downloading WARN Database CSV from %s: %v", url, err)
			continue // Continue with other sources instead of failing completely
		}
		defer resp.Body.Close()

		reader := csv.NewReader(resp.Body)
		records, err := reader.ReadAll()
		if err != nil {
			log.Printf("Error reading WARN Database CSV from %s: %v", url, err)
			continue
		}

		if len(records) < 2 {
			log.Printf("WARN Database CSV from %s has no data", url)
			continue
		}

		imported := 0
		for _, record := range records[1:] { // Skip header row
			if len(record) < 7 {
				continue
			}

			// Parse WARN Database format:
			// State,Company,City,Number of Workers,WARN Received Date,Effective Date,Closure / Layoff,Temporary/Permanent,Union,Region,County,Industry,Notes
			state := strings.TrimSpace(record[0])
			companyName := strings.TrimSpace(record[1])
			city := strings.TrimSpace(record[2])
			workersStr := strings.TrimSpace(record[3])
			warnDateStr := strings.TrimSpace(record[4])
			effectiveDateStr := strings.TrimSpace(record[5])
			layoffType := strings.TrimSpace(record[6])
			warnIndustry := strings.TrimSpace(record[11]) // Extract industry from CSV

			if companyName == "" || workersStr == "" {
				continue // Skip records with missing company or worker count
			}

			// Clean workers string (remove commas)
			workersStr = strings.ReplaceAll(workersStr, ",", "")
			workers, err := strconv.Atoi(workersStr)
			if err != nil || workers <= 0 {
				continue // Skip invalid worker counts
			}

			// Parse effective date (layoff date) - handle various formats and ranges
			var layoffDate time.Time
			effectiveDateStr = strings.TrimSpace(effectiveDateStr)

			// Handle date ranges by taking the start date
			if strings.Contains(effectiveDateStr, "-") {
				parts := strings.Split(effectiveDateStr, "-")
				effectiveDateStr = strings.TrimSpace(parts[0])
			}

			// Try multiple date formats
			dateFormats := []string{"1/2/2006", "2006-01-02", "1/2/06", "01/02/2006", "1/02/2006"}
			parsed := false
			for _, format := range dateFormats {
				layoffDate, err = time.Parse(format, effectiveDateStr)
				if err == nil {
					parsed = true
					break
				}
			}
			if !parsed {
				continue // Skip invalid dates
			}

			// Skip future dates and dates that are too old (more than 1 year for historical coverage)
			now := time.Now()
			oneYearAgo := now.AddDate(-1, 0, 0)
			if layoffDate.After(now.AddDate(0, 6, 0)) { // Allow planned layoffs up to 6 months future
				continue
			}
			if layoffDate.Before(oneYearAgo) {
				continue
			}

			// Clean workers string (remove commas)
			workersStr = strings.ReplaceAll(workersStr, ",", "")
			workers, err = strconv.Atoi(workersStr)
			if err != nil || workers <= 0 {
				continue // Skip invalid worker counts
			}

			// Parse effective date (layoff date) - handle various formats and ranges
			effectiveDateStr = strings.TrimSpace(effectiveDateStr)

			// Handle date ranges by taking the start date
			if strings.Contains(effectiveDateStr, "-") {
				parts := strings.Split(effectiveDateStr, "-")
				effectiveDateStr = strings.TrimSpace(parts[0])
			}

			// Estimate company size based on known data
			companySize := EstimateCompanySize(companyName)

			// Determine status based on layoff date
			status := "completed"
			if layoffDate.After(time.Now()) {
				status = "planned"
			}

			// Create notes with additional info
			notes := fmt.Sprintf("Source: Official WARN Database (layoffdata.com) | State: %s | City: %s | Type: %s | WARN Date: %s",
				state, city, layoffType, warnDateStr)

			// Determine industry: use CSV industry first, then fall back to name inference
			industryID := MapWARNIndustryToID(warnIndustry)
			if !industryID.Valid {
				industryID = InferIndustryID(companyName)
			}

			// Create company if not exists, then get the ID
			_, err = s.db.Exec("INSERT OR IGNORE INTO companies (name, employee_count, industry_id) VALUES (?, ?, ?)",
				companyName, sql.NullInt64{Int64: int64(companySize), Valid: companySize > 0}, industryID)
			if err != nil {
				log.Printf("Error inserting company %s: %v", companyName, err)
				continue
			}

			// Get the company ID (whether it was just inserted or already existed)
			var companyID int
			err = s.db.QueryRow("SELECT id FROM companies WHERE name = ?", companyName).Scan(&companyID)
			if err != nil {
				log.Printf("Error getting company ID for %s: %v", companyName, err)
				continue
			}

			if companySize > 0 {
				_, err = s.db.Exec("UPDATE companies SET employee_count = ? WHERE id = ? AND (employee_count IS NULL OR employee_count = 0)", companySize, companyID)
				if err != nil {
					log.Printf("Error updating company size for %s: %v", companyName, err)
				}
			}

			// Update industry if not set and we inferred one
			if industryID.Valid {
				_, err = s.db.Exec("UPDATE companies SET industry_id = ? WHERE id = ? AND industry_id IS NULL", industryID.Int64, companyID)
				if err != nil {
					log.Printf("Error updating industry for %s: %v", companyName, err)
				}
			}

			// Insert layoff
			sourceURL := "https://edd.ca.gov/en/jobs_and_training/Layoff_Services_WARN/" // Default WARN source
			_, err = s.db.Exec(`
			INSERT OR IGNORE INTO layoffs (company_id, employees_affected, layoff_date, source_url, notes, status)
			VALUES (?, ?, ?, ?, ?, ?)`,
				companyID, workers, layoffDate.Format("2006-01-02"), sourceURL, notes, status)
			if err != nil {
				log.Printf("Error creating layoff for %s: %v", companyName, err)
				continue
			}

			imported++
		}

		log.Printf("Successfully imported %d layoff records from %s", imported, url)
		totalImported += imported
	}

	if totalImported == 0 {
		return fmt.Errorf("no layoff records imported from any WARN Database source")
	}

	log.Printf("Successfully imported %d total layoff records from WARN Database", totalImported)
	return nil
}

// estimateCompanySize provides rough estimates for major tech companies based on public data
func EstimateCompanySize(companyName string) int {
	estimates := map[string]int{
		"Apple":      147000,
		"Microsoft":  221000,
		"Google":     190000,
		"Amazon":     1500000,
		"Meta":       67300,
		"NVIDIA":     29600,
		"Tesla":      140000,
		"IBM":        270000,
		"Oracle":     164000,
		"Salesforce": 79000,
		"Adobe":      29900,
		"Cisco":      84900,
		"Intel":      124800,
		"Spotify":    8200,
	}

	// Try exact match first
	if size, exists := estimates[companyName]; exists {
		return size
	}

	// Try case-insensitive match
	for name, size := range estimates {
		if strings.EqualFold(name, companyName) {
			return size
		}
	}

	return 0 // Unknown size
}

func InferIndustryID(companyName string) sql.NullInt64 {
	companyName = strings.ToLower(strings.TrimSpace(companyName))

	// Check keyword mappings first (specific company matches take precedence)
	industryMappings := map[string]int{
		// Technology (1) - Major tech companies
		"google": 1, "microsoft": 1, "meta": 1, "apple": 1, "amazon": 1,
		"facebook": 1, "twitter": 1, "instagram": 1, "linkedin": 1, "netflix": 1,
		"slack": 1, "zoom": 1, "atlassian": 1, "salesforce": 1, "zendesk": 1,
		"twilio": 1, "stripe": 1, "dropbox": 1, "box": 1, "paypal": 1,
		"square": 1, "coinbase": 2, "robinhood": 2, "nvidia": 1, "intel": 1,
		"amd": 1, "qualcomm": 1, "ibm": 1, "oracle": 1, "adobe": 1,

		// Healthcare (2) - Major healthcare companies
		"johnson & johnson": 2, "pfizer": 2, "merck": 2, "abbott": 2,
		"medtronic": 2, "baxter": 2, "stryker": 2, "zimmer": 2,

		// Retail (3) - Major retailers
		"walmart": 3, "target": 3, "costco": 3, "home depot": 3, "lowes": 3,
		"kroger": 3, "macy's": 3, "nordstrom": 3, "bed bath & beyond": 3,

		// Manufacturing (4) - Major manufacturers
		"general motors": 4, "ford": 4, "toyota": 4, "volkswagen": 4,
		"boeing": 4, "lockheed martin": 4, "raytheon": 4, "general electric": 4,
		"caterpillar": 4, "john deere": 4,

		// Finance (5) - Major financial institutions
		"jpmorgan": 5, "bank of america": 5, "wells fargo": 5, "citigroup": 5,
		"goldman sachs": 5, "morgan stanley": 5, "fidelity": 5, "blackrock": 5,

		// Education (6) - Major education companies
		"pearson": 6, "mckinsey": 6, "deloitte": 6, "accenture": 6,

		// Hospitality (7) - Major hospitality companies
		"marriott": 7, "hilton": 7, "hyatt": 7, "starwood": 7,
		"mcdonald's": 7, "starbucks": 7, "chipotle": 7, "yum brands": 7,

		// Transportation (8) - Major transportation companies
		"uber": 8, "lyft": 8, "fedex": 8, "ups": 8, "dhl": 8,
		"delta": 8, "american airlines": 8, "united": 8, "tesla": 8,

		// Construction (9) - Major construction companies
		"bechtel": 9, "fluor": 9, "kbr": 9,

		// Energy (10) - Major energy companies
		"exxon": 10, "chevron": 10, "shell": 10, "bp": 10,
		"conocophillips": 10, "schlumberger": 10,

		// Entertainment (11) - Major entertainment companies
		"disney": 11, "comcast": 11, "viacom": 11, "news corp": 11,

		// Government (12) - Government entities
		"department of": 12, "united states": 12, "state of": 12,

		// Non-Profit (13) - Major non-profits
		"red cross": 13, "unicef": 13, "world health organization": 13,

		// Agriculture (14) - Major agriculture companies
		"monsanto": 14, "cargill": 14, "archer daniels midland": 14,

		// Real Estate (15) - Major real estate companies
		"cbre": 15, "jll": 15, "colliers": 15,
	}

	// Check for keyword matches first (specific company matches)
	for keyword, industryID := range industryMappings {
		if strings.Contains(companyName, keyword) {
			return sql.NullInt64{Int64: int64(industryID), Valid: true}
		}
	}

	// Then check regex patterns for general categories
	patterns := []struct {
		regex string
		id    int
	}{
		// Healthcare (2)
		{`(?i)\b(hospital|clinic|medical center|healthcare|pharma|pharmaceutical|biotech|laboratory)\b`, 2},
		{`(?i)\b(health|medical|clinic|hospital|pharma)\b.*\b(system|services|center|group|corp)\b`, 2},

		// Finance (5)
		{`(?i)\b(bank|credit union|financial|insurance|investment|banking|capital|securities)\b`, 5},
		{`(?i)\b(finance|bank|insurance|investment|capital|securities)\b.*\b(services|group|corp|inc)\b`, 5},

		// Retail (3)
		{`(?i)\b(retail|store|mall|supermarket|department|grocery|convenience|wholesale)\b`, 3},
		{`(?i)\b(store|retail|mall)\b.*\b(chain|corp|inc|llc)\b`, 3},

		// Manufacturing (4)
		{`(?i)\b(manufacturing|factory|production|chemical|pharmaceutical|electronics)\b`, 4},
		{`(?i)\b(manufacturing|factory|production)\b.*\b(inc|corp|llc|co)\b`, 4},

		// Hospitality (7)
		{`(?i)\b(restaurant|hotel|motel|casino|resort|hospitality|catering|food service)\b`, 7},
		{`(?i)\b(hotel|restaurant|casino)\b.*\b(chain|group|corp|inc)\b`, 7},

		// Education (6)
		{`(?i)\b(university|college|school|academy|education|educational|learning)\b`, 6},
		{`(?i)\b(education|school|college)\b.*\b(system|services|group)\b`, 6},

		// Transportation (8)
		{`(?i)\b(transportation|shipping|logistics|trucking|rail|airline|aviation|delivery)\b`, 8},
		{`(?i)\b(transport|shipping|logistics)\b.*\b(services|corp|inc)\b`, 8},

		// Construction (9)
		{`(?i)\b(construction|building|contractor|engineering|architecture)\b`, 9},
		{`(?i)\b(construction|building)\b.*\b(company|corp|inc|llc)\b`, 9},

		// Energy (10)
		{`(?i)\b(energy|oil|gas|electric|utility|power|mining|petroleum)\b`, 10},
		{`(?i)\b(energy|utility|power)\b.*\b(company|corp|inc)\b`, 10},

		// Government (12)
		{`(?i)\b(government|state|county|city|municipal|federal|public|county|city)\b`, 12},
		{`(?i)\b(county|city|state)\b.*\b(of|department|office)\b`, 12},

		// Non-Profit (13)
		{`(?i)\b(foundation|charity|non.?profit|association|organization|society)\b`, 13},

		// Agriculture (14)
		{`(?i)\b(agriculture|farming|farm|crop|livestock|dairy|poultry)\b`, 14},
		{`(?i)\b(farm|farming|agriculture)\b.*\b(inc|corp|llc|co)\b`, 14},

		// Real Estate (15)
		{`(?i)\b(real estate|property|housing|rental|leasing|development)\b`, 15},
		{`(?i)\b(real estate|property)\b.*\b(services|group|corp)\b`, 15},
	}

	// Check regex patterns for general categories
	for _, pattern := range patterns {
		matched, err := regexp.MatchString(pattern.regex, companyName)
		if err == nil && matched {
			return sql.NullInt64{Int64: int64(pattern.id), Valid: true}
		}
	}

	// Check for keyword matches
	for keyword, industryID := range industryMappings {
		if strings.Contains(companyName, keyword) {
			return sql.NullInt64{Int64: int64(industryID), Valid: true}
		}
	}

	return sql.NullInt64{Valid: false} // No match found
}

// MapWARNIndustryToID maps WARN Database industry descriptions to our industry IDs
func MapWARNIndustryToID(warnIndustry string) sql.NullInt64 {
	if warnIndustry == "" {
		return sql.NullInt64{Valid: false}
	}

	warnIndustry = strings.ToLower(strings.TrimSpace(warnIndustry))

	// Map WARN industry descriptions to our industry IDs (based on NAICS categories)
	warnMappings := map[string]int{
		// Technology (1) - Professional, Scientific, and Technical Services
		"professional, scientific, and technical services": 1,
		"information": 1,
		"computer systems design and related services":   1,
		"software publishers":                            1,
		"data processing, hosting, and related services": 1,

		// Healthcare (2) - Health Care and Social Assistance
		"health care and social assistance":       2,
		"offices of physicians":                   2,
		"hospitals":                               2,
		"nursing and residential care facilities": 2,
		"medical and diagnostic laboratories":     2,
		"home health care services":               2,

		// Retail (3) - Retail Trade
		"retail trade":                                                3,
		"motor vehicle and parts dealers":                             3,
		"furniture and home furnishings stores":                       3,
		"electronics and appliance stores":                            3,
		"building material and garden equipment and supplies dealers": 3,
		"food and beverage stores":                                    3,
		"health and personal care stores":                             3,
		"gasoline stations":                                           3,
		"clothing and clothing accessories stores":                    3,
		"sporting goods, hobby, musical instrument, and book stores":  3,
		"general merchandise stores":                                  3,

		// Manufacturing (4) - Manufacturing
		"manufacturing":      4,
		"food manufacturing": 4,
		"beverage and tobacco product manufacturing": 4,
		"textile mills":                                                4,
		"apparel manufacturing":                                        4,
		"wood product manufacturing":                                   4,
		"chemical manufacturing":                                       4,
		"plastics and rubber products manufacturing":                   4,
		"nonmetallic mineral product manufacturing":                    4,
		"primary metal manufacturing":                                  4,
		"fabricated metal product manufacturing":                       4,
		"machinery manufacturing":                                      4,
		"computer and electronic product manufacturing":                4,
		"electrical equipment, appliance, and component manufacturing": 4,
		"transportation equipment manufacturing":                       4,

		// Finance (5) - Finance and Insurance
		"finance and insurance":                        5,
		"credit intermediation and related activities": 5,
		"securities, commodity contracts, and other financial investments and related activities": 5,
		"insurance carriers and related activities":                                               5,

		// Education (6) - Educational Services
		"educational services":                             6,
		"elementary and secondary schools":                 6,
		"colleges, universities, and professional schools": 6,

		// Hospitality (7) - Accommodation and Food Services
		"accommodation and food services":   7,
		"traveler accommodation":            7,
		"food services and drinking places": 7,

		// Transportation (8) - Transportation and Warehousing
		"transportation and warehousing":        8,
		"air transportation":                    8,
		"rail transportation":                   8,
		"water transportation":                  8,
		"truck transportation":                  8,
		"support activities for transportation": 8,
		"postal service":                        8,

		// Construction (9) - Construction
		"construction":                             9,
		"construction of buildings":                9,
		"heavy and civil engineering construction": 9,
		"specialty trade contractors":              9,

		// Energy (10) - Utilities
		"utilities": 10,
		"electric power generation, transmission and distribution": 10,
		"natural gas distribution":                                 10,
		"water, sewage and other systems":                          10,

		// Entertainment (11) - Arts, Entertainment, and Recreation
		"arts, entertainment, and recreation":                       11,
		"performing arts, spectator sports, and related industries": 11,
		"museums, historical sites, and similar institutions":       11,
		"amusement, gambling, and recreation industries":            11,

		// Government (12) - Public Administration
		"public administration": 12,
		"executive, legislative, and other general government support": 12,
		"justice, public order, and safety activities":                 12,
		"administration of human resource programs":                    12,
		"administration of environmental quality programs":             12,
		"administration of economic programs":                          12,

		// Non-Profit (13) - Other Services (except Public Administration)
		"religious, grantmaking, civic, professional, and similar organizations": 13,

		// Agriculture (14) - Agriculture, Forestry, Fishing and Hunting
		"agriculture, forestry, fishing and hunting": 14,
		"crop production":                   14,
		"animal production and aquaculture": 14,

		// Real Estate (15) - Real Estate and Rental and Leasing
		"real estate and rental and leasing": 15,
		"real estate":                        15,
	}

	// Check for exact matches first
	if industryID, exists := warnMappings[warnIndustry]; exists {
		return sql.NullInt64{Int64: int64(industryID), Valid: true}
	}

	// Check for partial matches
	for warnDesc, industryID := range warnMappings {
		if strings.Contains(warnIndustry, warnDesc) || strings.Contains(warnDesc, warnIndustry) {
			return sql.NullInt64{Int64: int64(industryID), Valid: true}
		}
	}

	return sql.NullInt64{Valid: false} // No match found
}

// EnrichCompanyIndustries runs post-import enrichment using Clearbit API for companies without industries
func (s *FreeDataService) EnrichCompanyIndustries(clearbitAPIKey string) error {
	if clearbitAPIKey == "" {
		log.Println("Clearbit API key not provided, skipping enrichment")
		return nil
	}

	// Get companies without industries
	rows, err := s.db.Query(`
		SELECT id, name, website
		FROM companies
		WHERE industry_id IS NULL AND website IS NOT NULL
		LIMIT 100`) // Limit to avoid rate limits
	if err != nil {
		return fmt.Errorf("error querying companies without industries: %w", err)
	}
	defer rows.Close()

	enriched := 0
	for rows.Next() {
		var id int
		var name string
		var website sql.NullString

		if err := rows.Scan(&id, &name, &website); err != nil {
			log.Printf("Error scanning company: %v", err)
			continue
		}

		if !website.Valid || website.String == "" {
			continue
		}

		// Extract domain from website
		domain := extractDomain(website.String)
		if domain == "" {
			continue
		}

		// Query Clearbit API
		industryID, err := s.queryClearbitIndustry(domain, clearbitAPIKey)
		if err != nil {
			log.Printf("Error querying Clearbit for %s: %v", domain, err)
			continue
		}

		if industryID.Valid {
			// Update company with industry
			_, err = s.db.Exec("UPDATE companies SET industry_id = ? WHERE id = ?", industryID.Int64, id)
			if err != nil {
				log.Printf("Error updating industry for company %d: %v", id, err)
			} else {
				enriched++
				log.Printf("Enriched company %s (%s) with industry %d", name, domain, industryID.Int64)
			}
		}

		// Rate limit: Clearbit free tier allows 500 requests/month, so limit to ~15/day
		time.Sleep(2 * time.Second)
	}

	log.Printf("Enriched %d companies with Clearbit data", enriched)
	return nil
}

// queryClearbitIndustry queries Clearbit API for company industry
func (s *FreeDataService) queryClearbitIndustry(domain, apiKey string) (sql.NullInt64, error) {
	url := fmt.Sprintf("https://company.clearbit.com/v2/companies/find?domain=%s", domain)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return sql.NullInt64{Valid: false}, err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return sql.NullInt64{Valid: false}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return sql.NullInt64{Valid: false}, fmt.Errorf("Clearbit API returned status %d", resp.StatusCode)
	}

	var result struct {
		Category struct {
			Industry string `json:"industry"`
		} `json:"category"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return sql.NullInt64{Valid: false}, err
	}

	if result.Category.Industry == "" {
		return sql.NullInt64{Valid: false}, nil
	}

	// Map Clearbit industry to our industry IDs
	industryID := mapClearbitIndustryToID(result.Category.Industry)
	return industryID, nil
}

// mapClearbitIndustryToID maps Clearbit industry names to our industry IDs
func mapClearbitIndustryToID(clearbitIndustry string) sql.NullInt64 {
	clearbitIndustry = strings.ToLower(strings.TrimSpace(clearbitIndustry))

	clearbitMappings := map[string]int{
		"technology": 1,
		"software":   1,
		"internet":   1,
		"mobile":     1,
		"hardware":   1,

		"healthcare":     2,
		"medical":        2,
		"pharmaceutical": 2,
		"biotechnology":  2,

		"retail":         3,
		"e-commerce":     3,
		"consumer goods": 3,

		"manufacturing": 4,
		"industrial":    4,

		"finance":            5,
		"financial services": 5,
		"banking":            5,
		"insurance":          5,

		"education": 6,
		"edtech":    6,

		"hospitality": 7,
		"food":        7,
		"restaurants": 7,

		"transportation": 8,
		"logistics":      8,
		"automotive":     8,

		"construction": 9,
		"real estate":  15,
		"property":     15,

		"energy":    10,
		"utilities": 10,

		"entertainment": 11,
		"media":         11,
		"gaming":        11,

		"government":  12,
		"non-profit":  13,
		"agriculture": 14,
	}

	if id, exists := clearbitMappings[clearbitIndustry]; exists {
		return sql.NullInt64{Int64: int64(id), Valid: true}
	}

	// Partial matches
	for clearbit, id := range clearbitMappings {
		if strings.Contains(clearbitIndustry, clearbit) || strings.Contains(clearbit, clearbitIndustry) {
			return sql.NullInt64{Int64: int64(id), Valid: true}
		}
	}

	return sql.NullInt64{Valid: false}
}

// extractDomain extracts domain from URL
func extractDomain(url string) string {
	// Simple domain extraction - remove protocol and path
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "www.")

	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}

	return strings.ToLower(url)
}

// Import from Revelio Labs Public Labor Statistics (aggregated data)
func (s *FreeDataService) ImportFromRevelioLabs() error {
	// Try to import from the total layoffs CSV for summary data
	url := "https://info0.s3.us-east-2.amazonaws.com/rpls/latest/layoffs/total_layoffs.csv"

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error downloading Revelio Labs CSV: %w", err)
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("error reading Revelio Labs CSV: %w", err)
	}

	if len(records) < 2 {
		return fmt.Errorf("Revelio Labs CSV has no data")
	}

	log.Printf("Revelio Labs data contains %d monthly records (aggregated data for reference)", len(records)-1)
	log.Printf("Note: This data is aggregated by month and cannot be directly imported as individual layoff records")
	log.Printf("Use /import/warn endpoint for individual company layoff data")

	return nil
}

// NotificationService handles email notifications
type NotificationService struct {
	smtpServer string
	smtpPort   int
	fromEmail  string
	toEmails   []string
	username   string
	password   string
}

func NewNotificationService(smtpServer string, smtpPort int, fromEmail string, toEmails []string, username, password string) *NotificationService {
	return &NotificationService{
		smtpServer: smtpServer,
		smtpPort:   smtpPort,
		fromEmail:  fromEmail,
		toEmails:   toEmails,
		username:   username,
		password:   password,
	}
}

func (n *NotificationService) SendImportReport(result *models.ImportResult, history *models.ImportHistory) error {
	if n.smtpServer == "" {
		log.Printf("SMTP not configured, skipping email notification")
		return nil
	}

	subject := "Layoff Tracker - Nightly Import Report"
	body := fmt.Sprintf(`Nightly Import Report
Status: %s
Records Added: %d
Source: %s
Duration: %v
Timestamp: %s

%s`,
		result.Status,
		result.RecordsAdded,
		history.SourceURL,
		result.Duration,
		history.ImportedAt.Format(time.RFC3339),
		func() string {
			if result.Error != nil {
				return fmt.Sprintf("Error: %v", result.Error)
			}
			return ""
		}())

	return n.sendEmail(subject, body)
}

func (n *NotificationService) sendEmail(subject, body string) error {
	auth := smtp.PlainAuth("", n.username, n.password, n.smtpServer)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		n.fromEmail,
		strings.Join(n.toEmails, ","),
		subject,
		body)

	addr := fmt.Sprintf("%s:%d", n.smtpServer, n.smtpPort)
	return smtp.SendMail(addr, auth, n.fromEmail, n.toEmails, []byte(msg))
}

// Automated import functionality
func (s *FreeDataService) ImportWithChangeDetection() (*models.ImportResult, error) {
	startTime := time.Now()
	log.Printf("Starting import change detection at %v", startTime)

	// Get content hashes for all sources
	sourceHashes := make(map[string]string)
	totalRecords := 0

	urls := []string{
		"https://docs.google.com/spreadsheets/d/1ayO8dl7sXaIYBAwkBGRUjbDms6MAbZFvvxxRp8IyxvY/export?format=csv",
		"https://docs.google.com/spreadsheets/d/1Qx6lv3zAL9YTsKJQNALa2GqBLXq0RER2lHvzyx32pRs/export?format=csv",
	}

	log.Printf("Checking %d source URLs for changes", len(urls))

	for _, url := range urls {
		log.Printf("Getting content hash for %s", url)
		hash, recordCount, err := s.getContentHash(url)
		if err != nil {
			log.Printf("Error getting content hash for %s: %v", url, err)
			continue
		}
		hashPreview := hash
		if len(hash) > 16 {
			hashPreview = hash[:16]
		}
		log.Printf("Got hash for %s: %s (records: %d)", url, hashPreview, recordCount)
		sourceHashes[url] = hash
		totalRecords += recordCount
	}

	log.Printf("Total records across all sources: %d", totalRecords)

	// Check if any source has changed
	hasChanges := false
	for url, newHash := range sourceHashes {
		lastHash, err := s.getLastContentHash(url)
		if err != nil {
			log.Printf("Error getting last hash for %s: %v", url, err)
			hasChanges = true // Assume changed if we can't check
			continue
		}
		if lastHash != newHash {
			lastHashPreview := lastHash
			if len(lastHash) > 8 {
				lastHashPreview = lastHash[:8]
			}
			newHashPreview := newHash
			if len(newHash) > 8 {
				newHashPreview = newHash[:8]
			}
			log.Printf("Content changed for %s: %s -> %s", url, lastHashPreview, newHashPreview)
			hasChanges = true
		}
	}

	if !hasChanges {
		return &models.ImportResult{Status: "no_changes", RecordsAdded: 0, Duration: time.Since(startTime)}, nil
	}

	// Perform import with retry logic
	var lastErr error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("Starting import attempt %d/%d", attempt, maxRetries)

		err := s.ImportFromWARNDatabase()
		if err == nil {
			// Save import history
			for url, hash := range sourceHashes {
				s.saveImportHistory(url, totalRecords, hash, "completed", "", time.Since(startTime))
			}

			return &models.ImportResult{
				Status:       "updated",
				RecordsAdded: totalRecords, // Approximate based on source data
				Duration:     time.Since(startTime),
			}, nil
		}

		lastErr = err
		log.Printf("Import attempt %d failed: %v", attempt, err)

		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * time.Minute) // Exponential backoff
		}
	}

	// Save failed import history
	for url, hash := range sourceHashes {
		s.saveImportHistory(url, 0, hash, "failed", lastErr.Error(), time.Since(startTime))
	}

	return &models.ImportResult{
		Status:   "failed",
		Duration: time.Since(startTime),
		Error:    lastErr,
	}, lastErr
}

func (s *FreeDataService) getContentHash(url string) (string, int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	records, err := reader.ReadAll()
	if err != nil {
		return "", 0, err
	}

	if len(records) < 2 {
		return "", 0, fmt.Errorf("no data in CSV")
	}

	// Create content hash from all records
	h := sha256.New()
	for _, record := range records {
		h.Write([]byte(strings.Join(record, ",")))
		h.Write([]byte("\n"))
	}

	hash := fmt.Sprintf("%x", h.Sum(nil))
	if hash == "" {
		return "", 0, fmt.Errorf("generated empty hash")
	}
	return hash, len(records) - 1, nil // -1 for header
}

func (s *FreeDataService) getLastContentHash(url string) (string, error) {
	var hash string
	err := s.db.QueryRow("SELECT content_hash FROM import_history WHERE source_url = ? ORDER BY imported_at DESC LIMIT 1", url).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil // No previous import
	}
	return hash, err
}

func (s *FreeDataService) saveImportHistory(url string, recordCount int, contentHash, status, errorMsg string, duration time.Duration) {
	_, err := s.db.Exec(`
		INSERT INTO import_history (source_url, record_count, content_hash, status, error_message, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?)`,
		url, recordCount, contentHash, status, errorMsg, duration.Milliseconds())

	if err != nil {
		log.Printf("Error saving import history: %v", err)
	}
}

func SetupFreeDataRoutes(e *echo.Echo, db *database.DB) {
	freeDataService := NewFreeDataService(db)

	// Import WARN Database data
	e.POST("/import/warn", func(c echo.Context) error {
		err := freeDataService.ImportFromWARNDatabase()
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, map[string]string{"message": "WARN Database import completed"})
	})

	// Import Revelio Labs data (aggregated)
	e.POST("/import/revelio", func(c echo.Context) error {
		err := freeDataService.ImportFromRevelioLabs()
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, map[string]string{"message": "Revelio Labs data checked (aggregated data)"})
	})

	// Enrich company industries with Clearbit
	e.POST("/import/enrich", func(c echo.Context) error {
		clearbitKey := os.Getenv("CLEARBIT_API_KEY")
		err := freeDataService.EnrichCompanyIndustries(clearbitKey)
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, map[string]string{"message": "Company industry enrichment completed"})
	})

	// Import stats
	e.GET("/import/stats", func(c echo.Context) error {
		stats, err := freeDataService.GetImportStats()
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, stats)
	})

}
