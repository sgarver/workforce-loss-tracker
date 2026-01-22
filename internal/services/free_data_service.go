package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"layoff-tracker/internal/classifier"
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
	db            *database.DB
	layoffService *LayoffService
}

func NewFreeDataService(db *database.DB, layoffService *LayoffService) *FreeDataService {
	return &FreeDataService{db: db, layoffService: layoffService}
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
	startTime := time.Now()
	log.Printf("Starting full WARN Database import from layoffdata.com")

	// WARN Database CSV export URLs - both current and historical for comprehensive data
	urls := []string{
		"https://docs.google.com/spreadsheets/d/1ayO8dl7sXaIYBAwkBGRUjbDms6MAbZFvvxxRp8IyxvY/export?format=csv", // Historical data
		"https://docs.google.com/spreadsheets/d/1Qx6lv3zAL9YTsKJQNALa2GqBLXq0RER2lHvzyx32pRs/export?format=csv", // 2025 data
	}

	totalImported := 0
	totalProcessed := 0
	totalSkipped := 0
	var errors []string

	for _, url := range urls {
		log.Printf("Importing from WARN Database: %s", url)

		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error downloading CSV from %s: %v", url, err)
			errors = append(errors, fmt.Sprintf("Download error for %s: %v", url, err))
			continue
		}
		defer resp.Body.Close()

		reader := csv.NewReader(resp.Body)
		// Handle quoted fields and special characters
		reader.LazyQuotes = true
		reader.TrimLeadingSpace = true
		reader.ReuseRecord = true // Reuse record slice to reduce allocations

		records, err := reader.ReadAll()
		if err != nil {
			log.Printf("Error reading CSV from %s: %v", url, err)
			errors = append(errors, fmt.Sprintf("CSV read error for %s: %v", url, err))
			continue
		}

		if len(records) < 2 {
			log.Printf("CSV from %s has insufficient data", url)
			continue
		}

		log.Printf("Processing %d records from %s", len(records)-1, url)

		imported := 0
		for _, record := range records[1:] { // Skip header
			totalProcessed++

			// Parse record with robust error handling
			company, workers, displayDate, industry, err := s.parseWARNRecord(record)
			if err != nil {
				// Log first few validation failures to understand patterns
				if totalSkipped < 10 {
					sampleLen := 5
					if len(record) < sampleLen {
						sampleLen = len(record)
					}
					log.Printf("Skipping record %d: %v (sample: %v)", totalProcessed, err, record[:sampleLen])
				} else if totalSkipped == 10 {
					log.Printf("Skipping further validation failures (too many to log)")
				}
				totalSkipped++
				continue
			}

			// Check if company with this exact name already exists (no normalization during import)
			var companyID int
			err = s.db.QueryRow("SELECT id FROM companies WHERE name = ?", company).Scan(&companyID)
			if err == nil {
				// Company already exists, use existing ID
			} else {
				// Company doesn't exist, insert new one with name and industry
				_, err = s.db.Exec("INSERT INTO companies (name, industry) VALUES (?, ?)",
					company, industry)
				if err != nil {
					// If industry column doesn't exist, try without it
					log.Printf("Insert with industry failed, trying without: %v", err)
					_, err = s.db.Exec("INSERT INTO companies (name) VALUES (?)", company)
				}
				if err != nil {
					log.Printf("Error inserting company %s: %v", company, err)
					totalSkipped++
					continue
				}

				// Get the new company ID
				err = s.db.QueryRow("SELECT id FROM companies WHERE name = ?", company).Scan(&companyID)
				if err != nil {
					log.Printf("Error getting company ID for %s: %v", company, err)
					totalSkipped++
					continue
				}
			}
			if err != nil {
				log.Printf("Error getting company ID for %s: %v", company, err)
				totalSkipped++
				continue
			}

			// Insert layoff
			var dbDate interface{}
			var layoffStatus string
			if displayDate == "unknown" {
				dbDate = nil
				layoffStatus = "pending" // Unknown dates should be reviewed
			} else {
				// Parse the display date back to time for database
				if parsed, err := time.Parse("2006-01-02", displayDate); err == nil {
					dbDate = parsed.Format("2006-01-02")
					// Future layoffs should be pending for verification
					if parsed.After(time.Now()) {
						layoffStatus = "pending"
					} else {
						layoffStatus = "completed"
					}
				} else {
					dbDate = nil
					displayDate = "unknown"
					layoffStatus = "pending" // Unknown dates should be reviewed
				}
			}

			_, err = s.db.Exec(`
				INSERT OR IGNORE INTO layoffs (company_id, employees_affected, layoff_date, source_url, status, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
				companyID, workers, dbDate, "https://layoffdata.com", layoffStatus, time.Now())
			if err != nil {
				log.Printf("Error inserting layoff for %s: %v", company, err)
				totalSkipped++
				continue
			}

			imported++
			if imported%1000 == 0 {
				log.Printf("Imported %d records so far from %s", imported, url)
			}
		}

		log.Printf("Completed %s: %d imported from %d records", url, imported, len(records)-1)
		totalImported += imported
	}

	log.Printf("WARN Database import completed: %d total imported, %d processed, %d skipped",
		totalImported, totalProcessed, totalSkipped)

	// Update company sizes for newly imported companies
	if s.layoffService != nil {
		log.Println("Updating company sizes for imported companies...")
		if updateErr := s.layoffService.UpdateCompanySizes(); updateErr != nil {
			log.Printf("Company size update failed: %v", updateErr)
		} else {
			log.Println("Company size update completed")
		}
	} else {
		log.Println("Warning: LayoffService not available, skipping company size update")
	}

	// Log import to history for monitoring
	contentHash := fmt.Sprintf("%d-%d-%d-%d", totalImported, totalProcessed, totalSkipped, startTime.Unix())
	errorMessage := ""
	if len(errors) > 0 {
		errorMessage = strings.Join(errors, "; ")
	}

	log.Printf("Logging import to history: records=%d, errors=%d, duration=%dms",
		totalImported, len(errors), time.Since(startTime).Milliseconds())

	_, err := s.db.Exec(`
		INSERT INTO import_history (source_url, record_count, content_hash, status, error_message, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"layoffdata.com (combined sheets)", totalImported, contentHash,
		"completed", errorMessage, time.Since(startTime).Milliseconds())

	if err != nil {
		log.Printf("Error logging import to history: %v", err)
	} else {
		log.Printf("Successfully logged import to history")
	}

	return nil
}

// parseWARNRecord parses a single WARN database CSV record
func (s *FreeDataService) parseWARNRecord(record []string) (company string, workers int, displayDate string, industry string, err error) {
	if len(record) < 4 {
		return "", 0, "", "", fmt.Errorf("record too short")
	}

	// Clean and validate company name
	company = strings.TrimSpace(record[1])
	if company == "" {
		return "", 0, "", "", fmt.Errorf("empty company name")
	}

	// Handle quoted company names and clean special characters
	company = strings.Trim(company, `"`)
	company = strings.ReplaceAll(company, "dba", "")
	company = strings.ReplaceAll(company, "DBA", "")
	company = regexp.MustCompile(`\s+`).ReplaceAllString(company, " ")
	company = strings.TrimSpace(company)

	// Parse workers count - handle corrupted data where addresses might be in this field
	workersStr := strings.TrimSpace(record[3])
	workersStr = strings.ReplaceAll(workersStr, ",", "")

	// If worker count looks like an address (contains street numbers, states, etc.), skip
	if strings.Contains(workersStr, " CA") || strings.Contains(workersStr, " NY") ||
		regexp.MustCompile(`^\d+\s+[A-Za-z]+\s+[A-Za-z]+`).MatchString(workersStr) ||
		len(strings.Fields(workersStr)) > 3 {
		return "", 0, "", "", fmt.Errorf("worker count appears to be address data: %s", workersStr)
	}

	workers, err = strconv.Atoi(workersStr)
	if err != nil || workers <= 0 {
		return "", 0, "", "", fmt.Errorf("invalid worker count: %s", workersStr)
	}

	// Parse dates with fallback
	displayDate = "unknown"
	if len(record) > 4 {
		warnDateStr := strings.TrimSpace(record[4])
		if warnDateStr != "" {
			if date, err := s.parseFlexibleDate(warnDateStr); err == nil {
				displayDate = date.Format("2006-01-02")
			}
		}
	}

	if displayDate == "unknown" && len(record) > 5 {
		effectiveDateStr := strings.TrimSpace(record[5])
		if effectiveDateStr != "" {
			if date, err := s.parseFlexibleDate(effectiveDateStr); err == nil {
				displayDate = date.Format("2006-01-02")
			}
		}
	}

	// Parse industry
	industry = "Unknown"
	if len(record) > 11 {
		warnIndustry := strings.TrimSpace(record[11])
		if warnIndustry != "" {
			industry = s.mapWARNIndustryToReadable(warnIndustry)
		}
	}

	return company, workers, displayDate, industry, nil
}

// parseFlexibleDate handles multiple date formats
func (s *FreeDataService) parseFlexibleDate(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)

	// Handle date ranges by taking the start date
	if strings.Contains(dateStr, "-") {
		parts := strings.Split(dateStr, "-")
		dateStr = strings.TrimSpace(parts[0])
	}

	formats := []string{
		"1/2/2006", "2006-01-02", "1/2/06", "01/02/2006", "1/02/2006",
		"2006/01/02", "02/01/2006", "Jan 2, 2006", "January 2, 2006",
		time.RFC3339, time.RFC822,
	}

	for _, format := range formats {
		if date, err := time.Parse(format, dateStr); err == nil {
			return date, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

// mapWARNIndustryToReadable converts NAICS codes to readable industry names
func (s *FreeDataService) mapWARNIndustryToReadable(naics string) string {
	// Clean the NAICS string: remove extra spaces, colons, and descriptions
	naics = strings.TrimSpace(naics)

	// Remove any text after colon (descriptions)
	if colonIndex := strings.Index(naics, ":"); colonIndex != -1 {
		naics = naics[:colonIndex]
	}

	// Take the first NAICS code if multiple are present (comma-separated)
	if commaIndex := strings.Index(naics, ","); commaIndex != -1 {
		naics = naics[:commaIndex]
	}

	// Remove any remaining spaces and clean
	naics = strings.TrimSpace(strings.Split(naics, " ")[0])

	mappings := map[string]string{
		// 2-digit NAICS codes (top level)
		"11": "Agriculture",
		"21": "Mining",
		"22": "Utilities",
		"23": "Construction",
		"31": "Manufacturing",
		"32": "Manufacturing",
		"33": "Manufacturing",
		"42": "Wholesale Trade",
		"44": "Retail",
		"45": "Retail",
		"48": "Transportation",
		"49": "Transportation",
		"51": "Information",
		"52": "Finance",
		"53": "Real Estate",
		"54": "Professional Services",
		"55": "Management",
		"56": "Administrative",
		"61": "Education",
		"62": "Healthcare",
		"71": "Arts & Entertainment",
		"72": "Hospitality",
		"81": "Other Services",
		"92": "Public Administration",

		// 3-digit NAICS codes (more specific)
		"111": "Agriculture",
		"112": "Agriculture",
		"113": "Agriculture",
		"114": "Agriculture",
		"115": "Agriculture",
		"211": "Mining",
		"212": "Mining",
		"213": "Mining",
		"221": "Utilities",
		"236": "Construction",
		"237": "Construction",
		"238": "Construction",
		"311": "Manufacturing",
		"312": "Manufacturing",
		"313": "Manufacturing",
		"314": "Manufacturing",
		"315": "Manufacturing",
		"316": "Manufacturing",
		"321": "Manufacturing",
		"322": "Manufacturing",
		"323": "Manufacturing",
		"324": "Manufacturing",
		"325": "Manufacturing",
		"326": "Manufacturing",
		"327": "Manufacturing",
		"331": "Manufacturing",
		"332": "Manufacturing",
		"333": "Manufacturing",
		"334": "Manufacturing",
		"335": "Manufacturing",
		"336": "Manufacturing",
		"337": "Manufacturing",
		"339": "Manufacturing",
		"423": "Wholesale Trade",
		"424": "Wholesale Trade",
		"425": "Wholesale Trade",
		"441": "Retail",
		"442": "Retail",
		"443": "Retail",
		"444": "Retail",
		"445": "Retail",
		"446": "Retail",
		"447": "Retail",
		"448": "Retail",
		"451": "Retail",
		"452": "Retail",
		"453": "Retail",
		"454": "Retail",
		"481": "Transportation",
		"482": "Transportation",
		"483": "Transportation",
		"484": "Transportation",
		"485": "Transportation",
		"486": "Transportation",
		"487": "Transportation",
		"488": "Transportation",
		"491": "Transportation",
		"492": "Transportation",
		"493": "Transportation",
		"511": "Information",
		"512": "Information",
		"515": "Information",
		"517": "Information",
		"518": "Information",
		"519": "Information",
		"521": "Finance",
		"522": "Finance",
		"523": "Finance",
		"524": "Finance",
		"525": "Finance",
		"531": "Real Estate",
		"532": "Real Estate",
		"533": "Real Estate",
		"541": "Professional Services",
		"551": "Management",
		"561": "Administrative",
		"562": "Administrative",
		"611": "Education",
		"621": "Healthcare",
		"622": "Healthcare",
		"623": "Healthcare",
		"624": "Healthcare",
		"711": "Arts & Entertainment",
		"712": "Arts & Entertainment",
		"713": "Arts & Entertainment",
		"721": "Hospitality",
		"722": "Hospitality",
		"811": "Other Services",
		"812": "Other Services",
		"813": "Other Services",
		"814": "Other Services",
		"921": "Public Administration",
		"922": "Public Administration",
		"923": "Public Administration",
		"924": "Public Administration",
		"925": "Public Administration",
		"926": "Public Administration",
		"927": "Public Administration",
		"928": "Public Administration",

		// 4-digit NAICS codes (most specific common ones)
		"5610": "Administrative",
		"5611": "Administrative",
		"5612": "Administrative",
		"5613": "Administrative",
		"5614": "Administrative",
		"5615": "Administrative",
		"5616": "Administrative",
		"5617": "Administrative",
		"5619": "Administrative",
		"5620": "Administrative",
		"5621": "Administrative",
		"5622": "Administrative",
		"5629": "Administrative",
		"4451": "Retail",
		"4452": "Retail",
		"4453": "Retail",
		"4461": "Retail",
		"4471": "Retail",
		"4481": "Retail",
		"4482": "Retail",
		"4511": "Retail",
		"4522": "Retail",
		"4523": "Retail",
		"4531": "Retail",
		"4532": "Retail",
		"4533": "Retail",
		"4539": "Retail",
		"7223": "Restaurant",
		"7224": "Restaurant",
		"7225": "Restaurant",
	}

	// Try increasingly specific NAICS code matches (longest first)
	for length := len(naics); length >= 2; length-- {
		code := naics[:length]
		if industry, exists := mappings[code]; exists {
			return industry
		}
	}

	// Special handling for common NAICS patterns that aren't in our mapping
	if len(naics) >= 3 {
		prefix3 := naics[:3]
		switch prefix3 {
		case "111", "112", "113", "114", "115":
			return "Agriculture"
		case "211", "212", "213":
			return "Mining"
		case "221":
			return "Utilities"
		case "236", "237", "238":
			return "Construction"
		case "311", "312", "313", "314", "315", "316", "321", "322", "323", "324", "325", "326", "327", "331", "332", "333", "334", "335", "336", "337", "339":
			return "Manufacturing"
		case "423", "424", "425":
			return "Wholesale Trade"
		case "441", "442", "443", "444", "445", "446", "447", "448", "451", "452", "453", "454":
			return "Retail"
		case "481", "482", "483", "484", "485", "486", "487", "488", "491", "492", "493":
			return "Transportation"
		case "511", "512", "515", "517", "518", "519":
			return "Information"
		case "521", "522", "523", "524", "525":
			return "Finance"
		case "531", "532", "533":
			return "Real Estate"
		case "541":
			return "Professional Services"
		case "551":
			return "Management"
		case "561", "562":
			return "Administrative"
		case "611":
			return "Education"
		case "621", "622", "623", "624":
			return "Healthcare"
		case "711", "712", "713":
			return "Arts & Entertainment"
		case "721", "722":
			return "Hospitality"
		case "811", "812", "813", "814":
			return "Other Services"
		case "921", "922", "923", "924", "925", "926", "927", "928":
			return "Public Administration"
		}
	}

	// If no mapping found, return the cleaned NAICS code if it looks valid
	// This preserves specific industry codes that don't have readable names
	if naics != "" && regexp.MustCompile(`^\d+$`).MatchString(naics) && len(naics) >= 2 {
		return naics
	}
	return "Unknown"
}

// EstimateCompanySize provides rough estimates for companies based on name patterns and known data
func EstimateCompanySize(companyName string) int {
	name := strings.ToLower(strings.TrimSpace(companyName))

	// Known major companies with specific sizes
	estimates := map[string]int{
		"apple":      147000,
		"microsoft":  221000,
		"google":     190000,
		"amazon":     1500000,
		"meta":       67300,
		"nvidia":     29600,
		"tesla":      140000,
		"ibm":        270000,
		"oracle":     164000,
		"salesforce": 79000,
		"adobe":      29900,
		"cisco":      84900,
		"intel":      124800,
		"spotify":    8200,
	}

	// Try exact match first
	if size, exists := estimates[name]; exists {
		return size
	}

	// Try partial matches for subsidiaries/brands
	for company, size := range estimates {
		if strings.Contains(name, company) {
			// Reduce size for subsidiaries (rough estimate)
			return size / 3
		}
	}

	// Estimate based on company name patterns and size indicators
	size := estimateSizeFromPatterns(name)
	if size > 0 {
		return size
	}

	// For unknown companies (typical small businesses in WARN data),
	// provide a reasonable default estimate of 50-200 employees
	// This is better than returning 0 which results in NULL in database
	return 100 // Default estimate for unknown companies
}

// estimateSizeFromPatterns estimates company size based on name patterns and keywords
func estimateSizeFromPatterns(name string) int {
	// Very large companies (100K+ employees)
	if strings.Contains(name, "walmart") || strings.Contains(name, "amazon") ||
		strings.Contains(name, "china") || strings.Contains(name, "alibaba") ||
		strings.Contains(name, "samsung") || strings.Contains(name, "toyota") {
		return 500000 // Rough estimate for mega-corps
	}

	// Large companies (50K-100K employees)
	if strings.Contains(name, "disney") || strings.Contains(name, "comcast") ||
		strings.Contains(name, "verizon") || strings.Contains(name, "att") ||
		strings.Contains(name, "fedex") || strings.Contains(name, "ups") ||
		strings.Contains(name, "walgreens") || strings.Contains(name, "cvs") ||
		strings.Contains(name, "target") || strings.Contains(name, "costco") ||
		strings.Contains(name, "lowe's") || strings.Contains(name, "home depot") {
		return 75000
	}

	// Medium-large companies (10K-50K employees)
	if strings.Contains(name, "starbucks") || strings.Contains(name, "mcdonald") ||
		strings.Contains(name, "chipotle") || strings.Contains(name, "dunkin") ||
		strings.Contains(name, "kroger") || strings.Contains(name, "safeway") ||
		strings.Contains(name, "dhl") || strings.Contains(name, "maersk") ||
		strings.Contains(name, "delta") || strings.Contains(name, "united airlines") ||
		strings.Contains(name, "american airlines") || strings.Contains(name, "southwest") {
		return 25000
	}

	// Medium companies (5K-10K employees)
	if strings.Contains(name, "autozone") || strings.Contains(name, "advance auto") ||
		strings.Contains(name, "tractor supply") || strings.Contains(name, "bath & body") ||
		strings.Contains(name, "ulta") || strings.Contains(name, "lululemon") ||
		strings.Contains(name, "lululemon") || strings.Contains(name, "peloton") ||
		strings.Contains(name, "square") || strings.Contains(name, "block") ||
		strings.Contains(name, "shopify") || strings.Contains(name, "square") {
		return 7500
	}

	// Small-medium companies (1K-5K employees)
	if strings.Contains(name, "github") || strings.Contains(name, "twilio") ||
		strings.Contains(name, "slack") || strings.Contains(name, "zoom") ||
		strings.Contains(name, "notion") || strings.Contains(name, "figma") ||
		strings.Contains(name, "stripe") || strings.Contains(name, "coinbase") ||
		strings.Contains(name, "robinhood") || strings.Contains(name, "affirm") {
		return 2500
	}

	// Based on company suffixes and keywords
	if strings.Contains(name, "university") || strings.Contains(name, "college") ||
		strings.Contains(name, "school") || strings.Contains(name, "academy") ||
		strings.Contains(name, "hospital") || strings.Contains(name, "medical center") {
		return 5000 // Educational/medical institutions tend to be larger
	}

	if strings.Contains(name, "bank") || strings.Contains(name, "credit union") ||
		strings.Contains(name, "insurance") || strings.Contains(name, "financial") {
		return 3000 // Financial institutions
	}

	if strings.Contains(name, "restaurant") || strings.Contains(name, "cafe") ||
		strings.Contains(name, "diner") || strings.Contains(name, "grill") {
		return 500 // Restaurants are typically smaller
	}

	if strings.Contains(name, "construction") || strings.Contains(name, "contracting") ||
		strings.Contains(name, "builders") || strings.Contains(name, "development") {
		return 2000 // Construction companies
	}

	if strings.Contains(name, "manufacturing") || strings.Contains(name, "mfg") ||
		strings.Contains(name, "corp") || strings.Contains(name, "incorporated") {
		return 1500 // Manufacturing tends to be medium-sized
	}

	// Unknown companies get no size estimate (return 0 to indicate unknown)
	return 0 // 0 means unknown size
}

// normalizeCompanyBasic applies basic normalization rules for well-known companies
func (s *FreeDataService) normalizeCompanyBasic(companyName string) string {
	name := strings.TrimSpace(strings.ToLower(companyName))

	// A&P variations - comprehensive matching for all A&P company names
	if strings.Contains(name, "a&p") ||
		(strings.Contains(name, "atlantic") && strings.Contains(name, "pacific")) ||
		strings.Contains(name, "great atlantic and pacific tea") {
		return "A&P"
	}

	// Boeing variations
	if strings.Contains(name, "boeing") {
		return "Boeing"
	}

	// Intel variations
	if strings.Contains(name, "intel") && !strings.Contains(name, "intelli") {
		return "Intel"
	}

	// Wells Fargo variations
	if strings.Contains(name, "wells fargo") {
		return "Wells Fargo"
	}

	// Bank of America variations
	if strings.Contains(name, "bank of america") {
		return "Bank of America"
	}

	// Walmart variations
	if strings.Contains(name, "walmart") {
		return "Walmart"
	}

	// Microsoft variations
	if strings.Contains(name, "microsoft") {
		return "Microsoft"
	}

	// Google variations
	if strings.Contains(name, "google") || strings.Contains(name, "alphabet") {
		return "Google"
	}

	// Amazon variations
	if strings.Contains(name, "amazon") {
		return "Amazon"
	}

	// Meta variations
	if strings.Contains(name, "meta") || strings.Contains(name, "facebook") {
		return "Meta"
	}

	// Apple variations
	if strings.Contains(name, "apple") && !strings.Contains(name, "pineapple") {
		return "Apple"
	}

	// Remove common suffixes to create cleaner names
	result := s.removeCommonSuffixes(name)

	return result
}

// removeCommonSuffixes removes common corporate suffixes to create cleaner company names
func (s *FreeDataService) removeCommonSuffixes(name string) string {
	// Common suffixes to remove (in order of specificity)
	suffixes := []string{
		" corporation", " incorporated", " inc.", " inc", " corp.", " corp", " co.", " co", " llc", " llp", " ltd.", " ltd",
		" company", " companies", " group", " holding", " holdings", " international", " global", " systems", " solutions",
		" technologies", " technology", " services", " service", " associates", " partners", " partner",
	}

	result := name

	// Remove suffixes from the end
	for _, suffix := range suffixes {
		if strings.HasSuffix(result, suffix) {
			result = strings.TrimSuffix(result, suffix)
			break // Only remove one suffix
		}
	}

	// Clean up extra spaces and return
	return strings.TrimSpace(result)
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

	// Then check regex patterns for general categories (expanded for better coverage)
	patterns := []struct {
		regex string
		id    int
	}{
		// Healthcare (2) - Expanded
		{`(?i)\b(hospital|clinic|medical center|healthcare|pharma|pharmaceutical|biotech|laboratory|dental|therapy|care|wellness)\b`, 2},
		{`(?i)\b(health|medical|clinic|hospital|pharma|pharmaceutical)\b.*\b(system|services|center|group|corp|inc)\b`, 2},
		{`(?i)\b(kaiser|mayo|cleve|mass general|johns hopkins|mount sinai|cedars|methodist|presbyterian)\b`, 2},

		// Finance (5) - Expanded
		{`(?i)\b(bank|credit union|financial|insurance|investment|banking|capital|securities|mortgage|lending|wealth|fiduciary)\b`, 5},
		{`(?i)\b(finance|bank|insurance|investment|capital|securities|mortgage)\b.*\b(services|group|corp|inc|llc)\b`, 5},
		{`(?i)\b(jpmorgan|goldman|citigroup|wells fargo|bank of america|morgan stanley|fidelity|blackrock|state street)\b`, 5},

		// Retail (3) - Expanded
		{`(?i)\b(retail|store|mall|supermarket|department|grocery|convenience|wholesale|market|shop|outlet)\b`, 3},
		{`(?i)\b(store|retail|mall|market)\b.*\b(chain|corp|inc|llc|co)\b`, 3},
		{`(?i)\b(walmart|target|costco|home depot|lowes|kroger|publix|meijer|trader joe|whole foods)\b`, 3},

		// Manufacturing (4) - Expanded
		{`(?i)\b(manufacturing|factory|production|chemical|pharmaceutical|electronics|automotive|machinery|equipment)\b`, 4},
		{`(?i)\b(manufacturing|factory|production|chemical)\b.*\b(inc|corp|llc|co|ltd)\b`, 4},
		{`(?i)\b(general motors|ford|toyota|honda|volkswagen|boeing|caterpillar|john deere|ge|siemens)\b`, 4},

		// Hospitality (7) - Expanded
		{`(?i)\b(restaurant|hotel|motel|casino|resort|hospitality|catering|food service|beverage|bar|pub)\b`, 7},
		{`(?i)\b(hotel|restaurant|casino|resort)\b.*\b(chain|group|corp|inc|llc)\b`, 7},
		{`(?i)\b(marriott|hilton|hyatt|ihg|choice|wyn|starwood|mcdonalds|chipotle|starbucks|dominos)\b`, 7},

		// Education (6) - Expanded
		{`(?i)\b(university|college|school|academy|education|educational|learning|training|institute)\b`, 6},
		{`(?i)\b(education|school|college|academy)\b.*\b(system|services|group|district)\b`, 6},
		{`(?i)\b(harvard|stanford|mit|yale|princeton|berkeley|usc|ucla|nyu|columbia)\b`, 6},

		// Transportation (8) - Expanded
		{`(?i)\b(transportation|shipping|logistics|trucking|rail|airline|aviation|delivery|courier|freight)\b`, 8},
		{`(?i)\b(transport|shipping|logistics|rail|airline)\b.*\b(services|corp|inc|llc)\b`, 8},
		{`(?i)\b(fedex|ups|dhl|usps|delta|american|united|southwest|amazon logistics)\b`, 8},

		// Construction (9) - Expanded
		{`(?i)\b(construction|building|contractor|engineering|architecture|development|builders|contracting)\b`, 9},
		{`(?i)\b(construction|building|contractor)\b.*\b(company|corp|inc|llc|ltd)\b`, 9},
		{`(?i)\b(bechtel|fluor|kbr|turner|pulte|kb home|lennar|dr horton)\b`, 9},

		// Energy (10) - Expanded
		{`(?i)\b(energy|oil|gas|electric|utility|power|mining|petroleum|coal|solar|wind|renewable)\b`, 10},
		{`(?i)\b(energy|utility|power|oil|gas)\b.*\b(company|corp|inc|ltd)\b`, 10},
		{`(?i)\b(exxon|chevron|shell|bp|conocophillips|schlumberger|halliburton|dukenergy|southern|dominion)\b`, 10},

		// Entertainment (11) - Expanded
		{`(?i)\b(entertainment|media|broadcast|publishing|film|television|music|streaming|production)\b`, 11},
		{`(?i)\b(entertainment|media|film|music)\b.*\b(studios|corp|inc|llc)\b`, 11},
		{`(?i)\b(disney|comcast|viacom|warner|universal|paramount|netflix|spotify|hulu)\b`, 11},

		// Government (12) - Expanded
		{`(?i)\b(government|state|county|city|municipal|federal|public|county|city|department)\b`, 12},
		{`(?i)\b(county|city|state|department)\b.*\b(of|office|services)\b`, 12},
		{`(?i)\b(united states|department of|bureau of|census|irs|ssa|fbi|cia)\b`, 12},

		// Non-Profit (13) - Expanded
		{`(?i)\b(foundation|charity|non.?profit|association|organization|society|fund|trust|alliance)\b`, 13},
		{`(?i)\b(red cross|unicef|world health|greenpeace|amnesty|salvation army)\b`, 13},

		// Agriculture (14) - Expanded
		{`(?i)\b(agriculture|farming|farm|crop|livestock|dairy|poultry|ranch|plantation)\b`, 14},
		{`(?i)\b(farm|farming|agriculture|ranch)\b.*\b(inc|corp|llc|co|ltd)\b`, 14},
		{`(?i)\b(monsanto|bayer|cargill|john deere|deere|archer daniels|cargill)\b`, 14},

		// Real Estate (15) - Expanded
		{`(?i)\b(real estate|property|housing|rental|leasing|development|realtor|brokerage)\b`, 15},
		{`(?i)\b(real estate|property|housing)\b.*\b(services|group|corp|inc)\b`, 15},
		{`(?i)\b(cbre|jll|colliers|cushman|newmark|eastdil|transwestern)\b`, 15},

		// Technology (1) - Additional patterns
		{`(?i)\b(tech|software|digital|cyber|cloud|ai|ml|data|analytics|platform)\b`, 1},
		{`(?i)\b(tech|software|digital)\b.*\b(solutions|systems|services|inc)\b`, 1},
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
	warnIndustry = strings.TrimSpace(warnIndustry)
	if warnIndustry == "" {
		return sql.NullInt64{Valid: false}
	}

	warnIndustry = strings.ToLower(warnIndustry)

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

// GetUniqueIndustries returns list of unique industry names
func (s *FreeDataService) GetUniqueIndustries() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT industry
		FROM companies
		WHERE industry IS NOT NULL AND industry != '' AND industry != 'Unknown'
		ORDER BY industry`)
	if err != nil {
		return nil, fmt.Errorf("error querying unique industries: %w", err)
	}
	defer rows.Close()

	var industries []string
	for rows.Next() {
		var industry string
		if err := rows.Scan(&industry); err != nil {
			continue
		}
		industries = append(industries, industry)
	}

	return industries, nil
}

// ClassifyExistingCompanies classifies companies that don't have industry set
func (s *FreeDataService) ClassifyExistingCompanies() error {
	classifier := classifier.NewIndustryClassifier()

	rows, err := s.db.Query(`
		SELECT id, name
		FROM companies
		WHERE industry IS NULL OR industry = ''`)
	if err != nil {
		return fmt.Errorf("error querying companies without industry: %w", err)
	}
	defer rows.Close()

	updated := 0
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}

		industry, confidence := classifier.ClassifyIndustry(name)
		if industry != "" {
			// Update the company
			_, err := s.db.Exec(`
				UPDATE companies
				SET industry = ?, industry_method = 'rule_based', industry_confidence = ?, industry_source = 'classifier_v1', updated_at = CURRENT_TIMESTAMP
				WHERE id = ?`, industry, confidence, id)
			if err != nil {
				log.Printf("Error updating company %d: %v", id, err)
			} else {
				updated++
			}
		}
	}

	log.Printf("Successfully classified %d companies using rule-based classifier", updated)
	return nil
}

// ReclassifyAllCompanies reclassifies ALL companies, including those with existing industry data
// This is useful for fixing poorly parsed NAICS codes from imports
func (s *FreeDataService) ReclassifyAllCompanies() error {
	rows, err := s.db.Query(`
		SELECT id, name, industry
		FROM companies
		WHERE industry IS NOT NULL AND industry != ''`)
	if err != nil {
		return fmt.Errorf("error querying companies with industry data: %w", err)
	}
	defer rows.Close()

	updated := 0
	for rows.Next() {
		var id int
		var name, currentIndustry string
		if err := rows.Scan(&id, &name, &currentIndustry); err != nil {
			continue
		}

		// Re-parse the current industry value using improved NAICS logic
		// This handles cases where NAICS codes were poorly parsed during import
		improvedIndustry := s.mapWARNIndustryToReadable(currentIndustry)

		// Only update if the industry classification improved
		if improvedIndustry != currentIndustry && improvedIndustry != "Unknown" {
			_, err := s.db.Exec(`
				UPDATE companies
				SET industry = ?, industry_method = 'reclassified', industry_confidence = 90, industry_source = 'naics_reparse_v2', updated_at = CURRENT_TIMESTAMP
				WHERE id = ?`, improvedIndustry, id)
			if err != nil {
				log.Printf("Error reclassifying company %d: %v", id, err)
			} else {
				log.Printf("Reclassified company %s: '%s' -> '%s'", name, currentIndustry, improvedIndustry)
				updated++
			}
		}
	}

	log.Printf("Successfully reclassified %d companies with improved NAICS parsing", updated)
	return nil
}

// ClassifyCompanyIndustries runs rule-based classification for companies without industries
func (s *FreeDataService) ClassifyCompanyIndustries() error {
	classifier := classifier.NewIndustryClassifier()

	// Get companies without industry classification or with "Unknown" industry
	rows, err := s.db.Query(`
		SELECT id, name
		FROM companies
		WHERE industry IS NULL OR industry = '' OR industry = 'Unknown'
		LIMIT 1000`) // Process in batches
	if err != nil {
		return fmt.Errorf("error querying companies without industries: %w", err)
	}
	defer rows.Close()

	updated := 0
	for rows.Next() {
		var id int
		var name string
		err := rows.Scan(&id, &name)
		if err != nil {
			log.Printf("Error scanning company: %v", err)
			continue
		}

		// Classify the company
		industry, confidence := classifier.ClassifyIndustry(name)

		// Debug logging
		if updated < 5 { // Log first few attempts
			log.Printf("Classifying company %d '%s': got '%s' with confidence %d", id, name, industry, confidence)
		}

		// Skip if classification is unknown or low confidence
		if industry == "Unknown" || confidence < 10 { // Lower threshold for testing
			continue
		}

		// Update the database
		_, err = s.db.Exec(`
			UPDATE companies
			SET industry = ?, industry_method = 'rule_based', industry_confidence = ?, industry_source = 'classifier_v1', updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`, industry, confidence, id)
		if err != nil {
			log.Printf("Error updating company %d: %v", id, err)
			continue
		}

		log.Printf("Updated company %d '%s' from 'Unknown' to '%s' (confidence %d)", id, name, industry, confidence)
		updated++
		if updated%100 == 0 {
			log.Printf("Classified %d companies so far", updated)
		}
	}

	log.Printf("Successfully classified %d companies using rule-based classifier", updated)
	return nil
}

// EnrichCompanyIndustries runs post-import enrichment using Clearbit API for companies with high-confidence rule-based classifications or no industry
func (s *FreeDataService) EnrichCompanyIndustries(clearbitAPIKey string) error {
	if clearbitAPIKey == "" {
		log.Println("Clearbit API key not provided, skipping enrichment")
		return nil
	}

	// Prioritize: high-confidence rule-based classifications, then companies without any industry
	rows, err := s.db.Query(`
		SELECT id, name, website
		FROM companies
		WHERE (industry_method = 'rule_based' AND industry_confidence > 80)
		   OR (industry IS NULL OR industry = '')
		ORDER BY CASE WHEN industry_method = 'rule_based' AND industry_confidence > 80 THEN 1 ELSE 2 END
		LIMIT 50`) // Smaller limit for Clearbit rate limits
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
		industry, confidence, err := s.queryClearbitIndustry(domain, clearbitAPIKey)
		if err != nil {
			log.Printf("Error querying Clearbit for %s: %v", domain, err)
			continue
		}

		if industry != "" {
			// Update company with industry
			_, err = s.db.Exec(`
				UPDATE companies
				SET industry = ?, industry_method = 'clearbit', industry_confidence = ?, industry_source = 'clearbit_api'
				WHERE id = ?`, industry, confidence, id)
			if err != nil {
				log.Printf("Error updating industry for company %d: %v", id, err)
			} else {
				enriched++
				log.Printf("Enriched company %s (%s) with industry %s (confidence %d)", name, domain, industry, confidence)
			}
		}

		// Rate limit: Clearbit free tier allows 500 requests/month, so limit to ~15/day
		time.Sleep(2 * time.Second)
	}

	log.Printf("Enriched %d companies with Clearbit data", enriched)
	return nil
}

// queryClearbitIndustry queries Clearbit API for company industry
func (s *FreeDataService) queryClearbitIndustry(domain, apiKey string) (string, int, error) {
	url := fmt.Sprintf("https://company.clearbit.com/v2/companies/find?domain=%s", domain)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", 0, err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("Clearbit API returned status %d", resp.StatusCode)
	}

	var result struct {
		Category struct {
			Industry string `json:"industry"`
		} `json:"category"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, err
	}

	if result.Category.Industry == "" {
		return "", 0, nil
	}

	// Return industry and high confidence for Clearbit
	return result.Category.Industry, 95, nil
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

func SetupFreeDataRoutes(e *echo.Echo, db *database.DB, layoffService *LayoffService) {
	freeDataService := NewFreeDataService(db, layoffService)

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

	e.POST("/import/classify", func(c echo.Context) error {
		err := freeDataService.ClassifyCompanyIndustries()
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, map[string]string{"message": "Company industry classification completed"})
	})

	// Get import history for monitoring
	e.GET("/api/import-history", func(c echo.Context) error {
		// TODO: Add admin authentication check here
		rows, err := freeDataService.db.Query(`
			SELECT id, source_url, imported_at, record_count, content_hash, status, error_message, duration_ms
			FROM import_history
			ORDER BY imported_at DESC
			LIMIT 50`)
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		defer rows.Close()

		var history []map[string]interface{}
		for rows.Next() {
			var id int
			var sourceURL string
			var importedAt time.Time
			var recordCount int
			var contentHash string
			var status string
			var errorMessage sql.NullString
			var durationMs sql.NullInt64

			err := rows.Scan(&id, &sourceURL, &importedAt, &recordCount, &contentHash, &status, &errorMessage, &durationMs)
			if err != nil {
				continue
			}

			history = append(history, map[string]interface{}{
				"id":            id,
				"source_url":    sourceURL,
				"imported_at":   importedAt,
				"record_count":  recordCount,
				"content_hash":  contentHash,
				"status":        status,
				"error_message": errorMessage.String,
				"duration_ms":   durationMs.Int64,
			})
		}

		return c.JSON(200, map[string]interface{}{
			"import_history": history,
			"total_imports":  len(history),
		})
	})

	e.POST("/companies/:id/verify-industry", func(c echo.Context) error {
		id := c.Param("id")
		companyID, err := strconv.Atoi(id)
		if err != nil {
			return c.JSON(400, map[string]string{"error": "Invalid company ID"})
		}

		var req struct {
			Verified   bool   `json:"verified"`
			VerifiedBy string `json:"verified_by"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(400, map[string]string{"error": "Invalid request body"})
		}

		// Update verification status
		_, err = freeDataService.db.Exec(`
			UPDATE companies
			SET industry_verified = ?, industry_verified_by = ?, industry_verified_at = CURRENT_TIMESTAMP
			WHERE id = ?`, req.Verified, req.VerifiedBy, companyID)
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}

		return c.JSON(200, map[string]string{"message": "Industry verification updated"})
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
