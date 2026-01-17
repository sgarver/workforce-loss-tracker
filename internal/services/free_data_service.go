package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"fmt"
	"layoff-tracker/internal/database"
	"layoff-tracker/internal/models"
	"log"
	"net/http"
	"net/smtp"
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

			// Infer industry for the company
			industryID := InferIndustryID(companyName)

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
	companyName = strings.ToLower(companyName)

	// Industry mappings based on keywords in company names
	industryMappings := map[string]int{
		// SaaS (1)
		"slack": 1, "zoom": 1, "atlassian": 1, "salesforce": 1, "zendesk": 1,
		"twilio": 1, "stripe": 1, "dropbox": 1, "box": 1,

		// FinTech (2)
		"paypal": 2, "square": 2, "coinbase": 2, "robinhood": 2, "affirm": 2, "sofi": 2,
		"chime": 2, "brex": 2, "plaid": 2, "wise": 2, "revolut": 2, "monzo": 2,

		// HealthTech (3)
		"roche": 3, "pfizer": 3, "moderna": 3, "theranos": 3, "23andme": 3, "goodrx": 3,

		// E-commerce (4)
		"amazon": 4, "ebay": 4, "etsy": 4, "shopify": 4, "walmart": 4, "target": 4,

		// AI/ML (5)
		"openai": 5, "anthropic": 5, "hugging face": 5, "cortex": 5, "scale ai": 5, "databricks": 5,
		"cerebras": 5, "graphcore": 5, "nvidia": 5, "google": 5, "microsoft": 5, "meta": 5,

		// Gaming (6)
		"riot": 6, "epic": 6, "activision": 6, "ea": 6, "ubisoft": 6, "take-two": 6,
		"zynga": 6, "unity": 6, "roblox": 6, "steam": 6,

		// Social Media (7)
		"facebook": 7, "twitter": 7, "instagram": 7, "tiktok": 7, "snapchat": 7, "linkedin": 7,
		"pinterest": 7, "reddit": 7, "discord": 7,

		// Cloud Computing (8)
		"aws": 8, "azure": 8, "gcp": 8, "digitalocean": 8, "linode": 8, "heroku": 8,
		"vercel": 8, "netlify": 8, "cloudflare": 8,

		// Cybersecurity (9)
		"crowdstrike": 9, "palo alto": 9, "fortinet": 9, "checkpoint": 9, "zscaler": 9,
		"okta": 9, "duo": 9, "auth0": 9,

		// EdTech (10)
		"coursera": 10, "udacity": 10, "khan academy": 10, "duolingo": 10, "outschool": 10,
		"masterclass": 10, "codecademy": 10,

		// Transportation (11)
		"uber": 11, "lyft": 11, "doordash": 11, "instacart": 11, "bird": 11, "lime": 11,
		"waymo": 11, "cruise": 11,

		// Real Estate Tech (12)
		"zillow": 12, "redfin": 12, "compass": 12, "opendoor": 12, "offerpad": 12,

		// HR Tech (13)
		"greenhouse": 13, "lever": 13, "workday": 13, "bamboohr": 13, "gusto": 13,

		// Marketing Tech (14)
		"hubspot": 14, "mailchimp": 14, "constant contact": 14, "hootsuite": 14,

		// Hardware (15)
		"apple": 15, "dell": 15, "hp": 15, "lenovo": 15, "asus": 15, "samsung": 15,
		"intel": 15, "amd": 15, "qualcomm": 15,
	}

	// Check for keyword matches
	for keyword, industryID := range industryMappings {
		if strings.Contains(companyName, keyword) {
			return sql.NullInt64{Int64: int64(industryID), Valid: true}
		}
	}

	return sql.NullInt64{Valid: false} // No match found
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

	// Import stats
	e.GET("/import/stats", func(c echo.Context) error {
		stats, err := freeDataService.GetImportStats()
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, stats)
	})

}
