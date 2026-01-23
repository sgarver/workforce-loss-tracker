package services

import (
	"database/sql"
	"fmt"
	"layoff-tracker/internal/database"
	"layoff-tracker/internal/models"
	"log"
	"strings"
	"time"
)

// formatNumber formats large numbers with K/M suffixes for display
func formatNumber(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	} else if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

type LayoffService struct {
	db *database.DB
}

func NewLayoffService(db *database.DB) *LayoffService {
	return &LayoffService{db: db}
}

func (s *LayoffService) GetLayoffs(params models.FilterParams) (*models.PaginatedResult, error) {
	// Fix: Handle nullable industry fields from LEFT JOIN
	whereClauses := []string{}
	args := []interface{}{}

	// Add search filter
	if params.Search != "" {
		whereClauses = append(whereClauses, "(LOWER(c.name) LIKE LOWER(?) OR LOWER(l.notes) LIKE LOWER(?))")
		searchPattern := "%" + params.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// Add industry filter
	if params.Industry != "" {
		whereClauses = append(whereClauses, "c.industry = ?")
		args = append(args, params.Industry)
	}

	// Add employee count filters
	if params.MinEmployees > 0 {
		whereClauses = append(whereClauses, "l.employees_affected >= ?")
		args = append(args, params.MinEmployees)
	}
	if params.MaxEmployees > 0 {
		whereClauses = append(whereClauses, "l.employees_affected <= ?")
		args = append(args, params.MaxEmployees)
	}

	// Add date range filters
	if params.StartDate != "" {
		whereClauses = append(whereClauses, "l.layoff_date >= ?")
		args = append(args, params.StartDate)
	}
	if params.EndDate != "" {
		whereClauses = append(whereClauses, "l.layoff_date <= ?")
		args = append(args, params.EndDate)
	}

	// Add unknown dates filter
	if !params.IncludeUnknownDates {
		whereClauses = append(whereClauses, "l.layoff_date IS NOT NULL")
	}

	// Default to "1=1" if no filters
	if len(whereClauses) == 0 {
		whereClauses = append(whereClauses, "1=1")
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// Set defaults
	page := params.Page
	if page <= 0 {
		page = 1
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}

	offset := (page - 1) * limit

	// Build ORDER BY clause
	orderBy := "l.layoff_date DESC"
	if params.SortBy != "" {
		direction := "ASC"
		if params.SortDirection == "desc" {
			direction = "DESC"
		}

		switch params.SortBy {
		case "company":
			orderBy = "c.name " + direction
		case "industry":
			orderBy = "c.industry " + direction
		case "company_size":
			orderBy = "c.employee_count " + direction
		case "employees":
			orderBy = "l.employees_affected " + direction
		case "date":
			orderBy = "l.layoff_date " + direction
		default:
			orderBy = "l.layoff_date DESC"
		}
	}

	// Get total count
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		WHERE %s`, whereSQL)

	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("error getting total count: %w", err)
	}

	// Get paginated results
	query := fmt.Sprintf(`
		SELECT l.id, l.company_id, l.employees_affected, l.layoff_date,
			l.source_type, l.notes, l.status, l.created_at,
			c.id, c.name, c.employee_count, c.website, c.logo_url, c.created_at, c.updated_at,
			c.industry
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		WHERE %s
		ORDER BY %s
		LIMIT %d OFFSET %d`, whereSQL, orderBy, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying layoffs: %w", err)
	}
	defer rows.Close()

	var layoffs []*models.Layoff
	for rows.Next() {
		layoff := &models.Layoff{
			Company: &models.Company{},
		}

		defer rows.Close()

		if !rows.Next() {
			return nil, fmt.Errorf("layoff not found")
		}

		var employeesAffected sql.NullInt64
		var layoffDate sql.NullTime
		var createdAt sql.NullTime
		var layoffCompanyID sql.NullInt64
		var companyID sql.NullInt64
		var companyName sql.NullString
		var employeeCount sql.NullInt64
		var website sql.NullString
		var logoURL sql.NullString
		var companyCreatedAt sql.NullTime
		var companyUpdatedAt sql.NullTime
		var industry sql.NullString

		err = rows.Scan(
			&layoff.ID, &layoffCompanyID, &employeesAffected, &layoffDate,
			&layoff.SourceType, &layoff.Notes, &layoff.Status, &createdAt,
			&companyID, &companyName, &employeeCount,
			&website, &logoURL, &companyCreatedAt, &companyUpdatedAt,
			&industry,
		)

		layoff.CompanyID = int(layoffCompanyID.Int64)
		layoff.EmployeesAffected = int(employeesAffected.Int64)
		if layoffDate.Valid {
			layoff.LayoffDate = layoffDate.Time
			layoff.DisplayDate = layoffDate.Time.Format("2006-01-02")
		} else {
			layoff.LayoffDate = time.Time{}
			layoff.DisplayDate = "unknown"
		}
		if createdAt.Valid {
			layoff.CreatedAt = createdAt.Time
		} else {
			layoff.CreatedAt = time.Now()
		}
		layoff.Company.ID = int(companyID.Int64)
		if companyName.Valid {
			layoff.Company.Name = companyName.String
		} else {
			layoff.Company.Name = "Unknown Company"
		}
		// Normalize company name on-demand for display
		if companyName.Valid {
			mappingService := NewCompanyMappingService(s.db)
			if normalizedName, err := mappingService.NormalizeCompany(companyName.String); err == nil {
				layoff.Company.Name = normalizedName
			}
		}
		layoff.Company.EmployeeCount = employeeCount
		if website.Valid {
			layoff.Company.Website = website.String
		}
		if logoURL.Valid {
			layoff.Company.LogoURL = logoURL.String
		}
		if industry.Valid {
			layoff.Company.Industry = industry.String
		}
		if companyCreatedAt.Valid {
			layoff.Company.CreatedAt = companyCreatedAt.Time
		} else {
			layoff.Company.CreatedAt = time.Now()
		}
		if companyUpdatedAt.Valid {
			layoff.Company.UpdatedAt = companyUpdatedAt.Time
		} else {
			layoff.Company.UpdatedAt = time.Now()
		}
		if industry.Valid {
			layoff.Company.Industry = industry.String
		}

		layoff.Company.EmployeeCount = employeeCount
		if website.Valid {
			layoff.Company.Website = website.String
		}
		if logoURL.Valid {
			layoff.Company.LogoURL = logoURL.String
		}
		if err != nil {
			return nil, fmt.Errorf("error scanning layoff row: %w", err)
		}

		layoffs = append(layoffs, layoff)
	}

	totalPages := (total + limit - 1) / limit

	return &models.PaginatedResult{
		Data:       layoffs,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

func (s *LayoffService) GetLayoff(layoffID int) (*models.Layoff, error) {
	query := `
		SELECT
			l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_type, l.notes, l.status, l.created_at,
			c.id, c.name, c.employee_count, c.website, c.logo_url, c.created_at, c.updated_at,
			c.industry
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		WHERE l.id = ?`

	layoff := &models.Layoff{
		Company: &models.Company{},
	}

	var logoURL sql.NullString
	var website sql.NullString
	var industry sql.NullString
	var employeeCount sql.NullInt64
	var layoffDate sql.NullTime
	var sourceType string
	var notes sql.NullString
	var status sql.NullString
	var createdAt sql.NullTime
	var companyID sql.NullInt64
	var companyName sql.NullString
	var companyCreatedAt sql.NullTime
	var companyUpdatedAt sql.NullTime

	rows, err := s.db.Query(query, layoffID)
	if err != nil {
		return nil, fmt.Errorf("error querying layoff: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("layoff not found")
	}

	err = rows.Scan(
		&layoff.ID, &layoff.CompanyID, &layoff.EmployeesAffected, &layoffDate,
		&sourceType, &notes, &status, &createdAt,
		&companyID, &companyName, &employeeCount,
		&website, &logoURL, &companyCreatedAt, &companyUpdatedAt,
		&industry,
	)

	// Handle nullable fields
	layoff.LayoffDate = layoffDate.Time
	if layoffDate.Valid {
		layoff.DisplayDate = layoffDate.Time.Format("2006-01-02")
	} else {
		layoff.DisplayDate = "unknown"
	}
	if createdAt.Valid {
		layoff.CreatedAt = createdAt.Time
	} else {
		layoff.CreatedAt = time.Now()
	}

	if website.Valid {
		layoff.Company.Website = website.String
	}
	if logoURL.Valid {
		layoff.Company.LogoURL = logoURL.String
	}
	layoff.Company.EmployeeCount = employeeCount
	if industry.Valid {
		layoff.Company.Industry = industry.String
	}
	if companyID.Valid {
		layoff.Company.ID = int(companyID.Int64)
	}
	if companyName.Valid {
		layoff.Company.Name = companyName.String
	} else {
		layoff.Company.Name = "Unknown Company"
	}
	// Normalize company name on-demand for display
	if companyName.Valid {
		mappingService := NewCompanyMappingService(s.db)
		if normalizedName, err := mappingService.NormalizeCompany(companyName.String); err == nil {
			layoff.Company.Name = normalizedName
		}
	}
	if companyCreatedAt.Valid {
		layoff.Company.CreatedAt = companyCreatedAt.Time
	} else {
		layoff.Company.CreatedAt = time.Now()
	}
	if companyUpdatedAt.Valid {
		layoff.Company.UpdatedAt = companyUpdatedAt.Time
	} else {
		layoff.Company.UpdatedAt = time.Now()
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("layoff not found")
		}
		return nil, fmt.Errorf("error querying layoff: %w", err)
	}

	return layoff, nil
}

func (s *LayoffService) CreateLayoff(layoff *models.Layoff) error {
	// Use status from layoff if set, otherwise set based on date
	status := layoff.Status.String
	if !layoff.Status.Valid || status == "" {
		status = "completed"
		if layoff.LayoffDate.After(time.Now()) {
			status = "planned"
		}
	}

	query := `
		INSERT INTO layoffs (company_id, employees_affected, layoff_date, source_type, notes, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	err := s.db.QueryRow(query,
		layoff.CompanyID,
		layoff.EmployeesAffected,
		layoff.LayoffDate,
		layoff.SourceType,
		layoff.Notes,
		status,
	).Scan(&layoff.ID, &layoff.CreatedAt)

	if err != nil {
		return fmt.Errorf("error creating layoff: %w", err)
	}

	return nil
}

func (s *LayoffService) GetIndustries() ([]*models.Industry, error) {
	query := "SELECT id, name, slug, created_at FROM industries ORDER BY name"

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying industries: %w", err)
	}
	defer rows.Close()

	var industries []*models.Industry
	for rows.Next() {
		var industry models.Industry
		err := rows.Scan(&industry.ID, &industry.Name, &industry.Slug, &industry.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("error scanning industry: %w", err)
		}
		industries = append(industries, &industry)
	}

	return industries, nil
}

// Comment-related methods
func (s *LayoffService) GetComments(layoffID int) ([]*models.Comment, error) {
	query := `
		SELECT id, layoff_id, author_name, author_email, content, created_at, updated_at
		FROM comments
		WHERE layoff_id = $1
		ORDER BY created_at ASC`

	rows, err := s.db.Query(query, layoffID)
	if err != nil {
		return nil, fmt.Errorf("error querying comments: %w", err)
	}
	defer rows.Close()

	var comments []*models.Comment
	for rows.Next() {
		var comment models.Comment
		var email sql.NullString
		err := rows.Scan(&comment.ID, &comment.LayoffID, &comment.AuthorName, &email, &comment.Content, &comment.CreatedAt, &comment.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("error scanning comment: %w", err)
		}
		if email.Valid {
			comment.AuthorEmail = email.String
		}
		comments = append(comments, &comment)
	}

	return comments, nil
}

func (s *LayoffService) CreateComment(comment *models.Comment) error {
	query := `
		INSERT INTO comments (layoff_id, author_name, author_email, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	var email *string
	if comment.AuthorEmail != "" {
		email = &comment.AuthorEmail
	}

	err := s.db.QueryRow(query, comment.LayoffID, comment.AuthorName, email, comment.Content).Scan(&comment.ID, &comment.CreatedAt, &comment.UpdatedAt)
	if err != nil {
		return fmt.Errorf("error creating comment: %w", err)
	}

	return nil
}

func (s *LayoffService) GetStats() (*models.Stats, error) {
	stats, err := s.GetStatsWithMonths(6)
	if err != nil {
		return nil, err
	}

	// Ensure formatted fields are set (they should already be set in GetStatsWithMonths)
	if stats.TotalLayoffsFormatted == "" {
		stats.TotalLayoffsFormatted = formatNumber(stats.TotalLayoffs)
		stats.TotalCompaniesFormatted = formatNumber(stats.TotalCompanies)
		stats.TotalEmployeesFormatted = formatNumber(stats.TotalEmployeesAffected)
		stats.RecentLayoffsFormatted = formatNumber(stats.RecentLayoffs)
	}

	return stats, nil
}

func (s *LayoffService) GetStatsWithMonths(months int) (*models.Stats, error) {
	stats := &models.Stats{}

	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, -months, 0)
	cutoffStr := cutoffDate.Format("2006-01-02")

	// Query weekly trends for the specified months
	query := `
		SELECT strftime('%Y-%W', layoff_date) as period,
			   COUNT(*) as count,
			   COALESCE(SUM(employees_affected), 0) as employees
		FROM layoffs
		WHERE layoff_date >= ? AND layoff_date <= date('now')
		GROUP BY strftime('%Y-%W', layoff_date)
		ORDER BY period`

	rows, err := s.db.Query(query, cutoffStr)
	if err != nil {
		return nil, fmt.Errorf("error querying weekly trends for %d months: %w", months, err)
	}
	defer rows.Close()

	var monthlyTrend []models.MonthlyTrend
	for rows.Next() {
		var trend models.MonthlyTrend
		var periodValue string
		err := rows.Scan(&periodValue, &trend.Count, &trend.Employees)
		if err != nil {
			return nil, fmt.Errorf("error scanning weekly trend: %w", err)
		}
		// Store the period value (week)
		trend.Month = periodValue
		trend.PeriodLabel = "Week"
		monthlyTrend = append(monthlyTrend, trend)
	}
	stats.MonthlyTrend = monthlyTrend

	// Total layoffs and employees affected
	err = s.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(employees_affected), 0) FROM layoffs").Scan(&stats.TotalLayoffs, &stats.TotalEmployeesAffected)
	if err != nil {
		return nil, fmt.Errorf("error getting total layoffs: %w", err)
	}

	// Total companies
	err = s.db.QueryRow("SELECT COUNT(DISTINCT company_id) FROM layoffs").Scan(&stats.TotalCompanies)
	if err != nil {
		return nil, fmt.Errorf("error getting total companies: %w", err)
	}

	// Last 6 months layoffs
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	sixMonthsAgoStr := sixMonthsAgo.Format("2006-01-02")
	err = s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(employees_affected), 0)
		FROM layoffs
		WHERE layoff_date >= ?`,
		sixMonthsAgoStr).Scan(&stats.RecentLayoffs, &stats.RecentEmployees)

	// Format numbers for display
	stats.TotalLayoffsFormatted = formatNumber(stats.TotalLayoffs)
	stats.TotalCompaniesFormatted = formatNumber(stats.TotalCompanies)
	stats.TotalEmployeesFormatted = formatNumber(stats.TotalEmployeesAffected)
	stats.RecentLayoffsFormatted = formatNumber(stats.RecentLayoffs)

	// Industry breakdown - skip if industry column doesn't exist or has no data
	industryQuery := `
		SELECT
			c.industry,
			COUNT(*) as count,
			COALESCE(SUM(l.employees_affected), 0) as employees
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		WHERE c.industry IS NOT NULL AND c.industry != '' AND c.industry != 'Unknown'
		GROUP BY c.industry
		ORDER BY employees DESC
		LIMIT 12`

	rows, err = s.db.Query(industryQuery)
	if err != nil {
		// If industry column doesn't exist or query fails, skip industry breakdown
		log.Printf("Skipping industry breakdown: %v", err)
		stats.IndustryBreakdown = []models.IndustryBreakdown{}
	} else {
		defer rows.Close()

		var industryBreakdown []models.IndustryBreakdown
		for rows.Next() {
			var breakdown models.IndustryBreakdown
			err := rows.Scan(&breakdown.Industry, &breakdown.Count, &breakdown.Employees)
			if err != nil {
				log.Printf("Error scanning industry breakdown: %v", err)
				break
			}
			industryBreakdown = append(industryBreakdown, breakdown)
		}

		stats.IndustryBreakdown = industryBreakdown
	}

	// Company breakdown (top 10 by employee impact)
	mappingService := NewCompanyMappingService(s.db)
	companyQuery := `
		SELECT c.name, SUM(l.employees_affected) as total_employees, COUNT(l.id) as layoffs
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		GROUP BY c.id, c.name
		HAVING total_employees >= 1000
		ORDER BY total_employees DESC` // Get companies with significant layoffs for normalization

	rows, err = s.db.Query(companyQuery)
	if err != nil {
		return nil, fmt.Errorf("error getting company breakdown: %w", err)
	}
	defer rows.Close()

	// Group companies by normalized names
	companyMap := make(map[string]*models.CompanyBreakdown)
	for rows.Next() {
		var name string
		var employees int
		var layoffs int

		err := rows.Scan(&name, &employees, &layoffs)
		if err != nil {
			return nil, fmt.Errorf("error scanning company data: %w", err)
		}

		// Normalize company name on-demand
		normalizedName, err := mappingService.NormalizeCompany(name)
		if err != nil {
			normalizedName = name // fallback to original name if normalization fails
		}

		// Aggregate by normalized name
		if existing, exists := companyMap[normalizedName]; exists {
			existing.Employees += employees
			existing.Layoffs += layoffs
		} else {
			companyMap[normalizedName] = &models.CompanyBreakdown{
				Company:   normalizedName,
				Employees: employees,
				Layoffs:   layoffs,
			}
		}
	}

	// Convert map to slice and sort by employees
	var companyBreakdown []*models.CompanyBreakdown
	for _, company := range companyMap {
		companyBreakdown = append(companyBreakdown, company)
	}

	// Sort by employees descending
	for i := 0; i < len(companyBreakdown)-1; i++ {
		for j := i + 1; j < len(companyBreakdown); j++ {
			if companyBreakdown[i].Employees < companyBreakdown[j].Employees {
				companyBreakdown[i], companyBreakdown[j] = companyBreakdown[j], companyBreakdown[i]
			}
		}
	}

	// Take top 12
	if len(companyBreakdown) > 12 {
		companyBreakdown = companyBreakdown[:12]
	}

	stats.CompanyBreakdown = companyBreakdown

	// Layoff scale breakdown (dynamic ranges divided into 4 equal segments)
	scaleBreakdown, err := s.getLayoffScaleBreakdown(months)
	if err != nil {
		log.Printf("Error getting layoff scale breakdown: %v", err)
		// Continue without scale breakdown rather than failing
	} else {
		stats.LayoffScaleBreakdown = scaleBreakdown
	}

	return stats, nil
}

func (s *LayoffService) getLayoffScaleBreakdown(months int) ([]models.LayoffScaleBreakdown, error) {
	cutoffDate := time.Now().AddDate(0, -months, 0)
	cutoffStr := cutoffDate.Format("2006-01-02")

	// First get the min and max employees_affected for the time range
	minMaxQuery := `
		SELECT MIN(employees_affected), MAX(employees_affected), COUNT(*)
		FROM layoffs
		WHERE layoff_date >= ? AND employees_affected > 0`

	var minEmployees, maxEmployees, totalLayoffs int
	err := s.db.QueryRow(minMaxQuery, cutoffStr).Scan(&minEmployees, &maxEmployees, &totalLayoffs)
	if err != nil {
		return nil, fmt.Errorf("error getting min/max employees: %w", err)
	}

	if totalLayoffs == 0 {
		return []models.LayoffScaleBreakdown{}, nil
	}

	// If all layoffs are the same size, handle edge case
	if minEmployees == maxEmployees {
		return []models.LayoffScaleBreakdown{
			{
				Scale:     "All Layoffs",
				Range:     fmt.Sprintf("%d employees", minEmployees),
				Count:     totalLayoffs,
				Employees: minEmployees * totalLayoffs,
			},
		}, nil
	}

	// Calculate range and segment size
	totalRange := maxEmployees - minEmployees
	segmentSize := totalRange / 4

	// Create 4 segments
	breakdowns := make([]models.LayoffScaleBreakdown, 4)
	scaleNames := []string{"Micro Layoffs", "Small Layoffs", "Major Layoffs", "Mass Layoffs"}

	for i := 0; i < 4; i++ {
		var minRange, maxRange int
		if i == 0 {
			minRange = minEmployees
		} else {
			minRange = minEmployees + (segmentSize * i)
		}

		if i == 3 {
			maxRange = maxEmployees
		} else {
			maxRange = minEmployees + (segmentSize * (i + 1)) - 1
		}

		// Query layoffs in this range
		rangeQuery := `
			SELECT COUNT(*), COALESCE(SUM(employees_affected), 0)
			FROM layoffs
			WHERE layoff_date >= ? AND employees_affected >= ? AND employees_affected <= ?`

		var count, employees int
		err := s.db.QueryRow(rangeQuery, cutoffStr, minRange, maxRange).Scan(&count, &employees)
		if err != nil {
			return nil, fmt.Errorf("error querying range %d: %w", i, err)
		}

		breakdowns[i] = models.LayoffScaleBreakdown{
			Scale:     scaleNames[i],
			Range:     fmt.Sprintf("%d-%d employees", minRange, maxRange),
			Count:     count,
			Employees: employees,
		}
	}

	return breakdowns, nil
}

func (s *LayoffService) GetSponsoredListings() ([]*models.SponsoredListing, error) {
	query := `
		SELECT 
			sl.id, sl.company_id, sl.start_date, sl.end_date, sl.message, sl.status, sl.created_at,
			c.id, c.name, c.employee_count, c.website, c.logo_url, c.created_at, c.updated_at
		FROM sponsored_listings sl
		JOIN companies c ON sl.company_id = c.id
		WHERE sl.status = 'active' 
		AND sl.start_date <= CURRENT_DATE 
		AND sl.end_date >= CURRENT_DATE
		ORDER BY sl.created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying sponsored listings: %w", err)
	}
	defer rows.Close()

	var listings []*models.SponsoredListing
	for rows.Next() {
		listing := &models.SponsoredListing{
			Company: &models.Company{},
		}

		err := rows.Scan(
			&listing.ID, &listing.CompanyID, &listing.StartDate, &listing.EndDate,
			&listing.Message, &listing.Status, &listing.CreatedAt,
			&listing.Company.ID, &listing.Company.Name, &listing.Company.EmployeeCount,
			&listing.Company.Website, &listing.Company.LogoURL, &listing.Company.CreatedAt, &listing.Company.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning sponsored listing row: %w", err)
		}

		listings = append(listings, listing)
	}

	return listings, nil
}

func (s *LayoffService) GetCurrentLayoffs() (*models.PaginatedResult, error) {
	// Get layoffs from the last 30 days
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	query := `
		SELECT
			l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_type, l.notes, l.status, l.created_at,
			c.id, c.name, c.employee_count, c.website, c.logo_url, c.created_at, c.updated_at,
			c.industry
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		WHERE l.layoff_date >= ?
		ORDER BY l.layoff_date DESC
		LIMIT 50`

	rows, err := s.db.Query(query, thirtyDaysAgo.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("error querying current layoffs: %w", err)
	}
	defer rows.Close()

	var layoffs []*models.Layoff
	for rows.Next() {
		layoff := &models.Layoff{
			Company: &models.Company{},
		}

		var logoURL sql.NullString
		var website sql.NullString
		var industry sql.NullString
		var employeeCount sql.NullInt64
		var layoffDate sql.NullTime
		var sourceType string
		var notes sql.NullString
		var status sql.NullString
		var createdAt sql.NullTime
		var layoffCompanyID sql.NullInt64
		var companyID sql.NullInt64
		var companyName sql.NullString
		var companyCreatedAt sql.NullTime
		var companyUpdatedAt sql.NullTime

		err := rows.Scan(
			&layoff.ID, &layoffCompanyID, &employeeCount, &layoffDate,
			&sourceType, &notes, &status, &createdAt,
			&companyID, &companyName, &layoff.Company.EmployeeCount,
			&website, &logoURL, &companyCreatedAt, &companyUpdatedAt,
			&industry,
		)

		layoff.CompanyID = int(layoffCompanyID.Int64)
		layoff.EmployeesAffected = int(employeeCount.Int64)
		if layoffDate.Valid {
			layoff.LayoffDate = layoffDate.Time
			layoff.DisplayDate = layoffDate.Time.Format("2006-01-02")
		} else {
			layoff.LayoffDate = time.Time{}
			layoff.DisplayDate = "unknown"
		}
		if createdAt.Valid {
			layoff.CreatedAt = createdAt.Time
		} else {
			layoff.CreatedAt = time.Now()
		}
		layoff.SourceType = sourceType
		layoff.Company.ID = int(companyID.Int64)
		if companyName.Valid {
			layoff.Company.Name = companyName.String
		} else {
			layoff.Company.Name = "Unknown Company"
		}
		// Normalize company name on-demand for display
		if companyName.Valid {
			mappingService := NewCompanyMappingService(s.db)
			if normalizedName, err := mappingService.NormalizeCompany(companyName.String); err == nil {
				layoff.Company.Name = normalizedName
			}
		}
		if companyCreatedAt.Valid {
			layoff.Company.CreatedAt = companyCreatedAt.Time
		} else {
			layoff.Company.CreatedAt = time.Now()
		}
		if companyUpdatedAt.Valid {
			layoff.Company.UpdatedAt = companyUpdatedAt.Time
		} else {
			layoff.Company.UpdatedAt = time.Now()
		}
		if website.Valid {
			layoff.Company.Website = website.String
		}
		if logoURL.Valid {
			layoff.Company.LogoURL = logoURL.String
		}
		if industry.Valid {
			layoff.Company.Industry = industry.String
		}
		if err != nil {
			return nil, fmt.Errorf("error scanning layoff row: %w", err)
		}

		layoffs = append(layoffs, layoff)
	}

	return &models.PaginatedResult{
		Data:       layoffs,
		Total:      len(layoffs),
		Page:       1,
		Limit:      50,
		TotalPages: 1,
	}, nil
}

// GetLastImportTime returns the timestamp of the most recent successful import
func (s *LayoffService) GetLastImportTime() (string, error) {
	var lastImportTime string
	err := s.db.QueryRow("SELECT imported_at FROM import_history WHERE status = 'completed' ORDER BY imported_at DESC LIMIT 1").Scan(&lastImportTime)
	if err == sql.ErrNoRows {
		return "Never", nil
	}
	if err != nil {
		return "", err
	}
	return lastImportTime, nil
}

func (s *LayoffService) GetOrCreateCompany(name, industry string) (int, error) {
	// First try to find existing company
	query := `SELECT id FROM companies WHERE name = ?`
	var id int
	err := s.db.QueryRow(query, name).Scan(&id)
	if err == nil {
		return id, nil
	}

	// Company doesn't exist, create it
	// Estimate company size based on name
	estimatedSize := EstimateCompanySize(name)

	query = `INSERT INTO companies (name, industry, employee_count) VALUES (?, ?, ?)`
	result, err := s.db.Exec(query, name, industry, estimatedSize)
	if err != nil {
		return 0, fmt.Errorf("failed to create company: %w", err)
	}
	id64, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get company ID: %w", err)
	}
	return int(id64), nil
}

// UpdateCompanySizes updates existing companies that don't have employee counts
func (s *LayoffService) UpdateCompanySizes() error {
	rows, err := s.db.Query(`SELECT id, name FROM companies WHERE employee_count IS NULL OR employee_count = 0`)
	if err != nil {
		return fmt.Errorf("error querying companies without sizes: %w", err)
	}
	defer rows.Close()

	updated := 0
	count := 0
	for rows.Next() {
		count++
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}

		estimatedSize := EstimateCompanySize(name)
		if estimatedSize > 0 {
			_, err := s.db.Exec(`UPDATE companies SET employee_count = ? WHERE id = ?`, estimatedSize, id)
			if err != nil {
				log.Printf("Error updating company %d size: %v", id, err)
			} else {
				updated++
			}
		} else {
			// For unknown companies, set to 0 to indicate unknown size
			// This will be displayed as "Unknown" in the UI
			_, err := s.db.Exec(`UPDATE companies SET employee_count = 0 WHERE id = ?`, id)
			if err != nil {
				log.Printf("Error setting unknown company %d size: %v", id, err)
			} else {
				updated++
			}
		}
	}

	log.Printf("Processed %d companies, updated %d with sizes", count, updated)
	return nil
}

func (s *LayoffService) ApproveLayoff(id int) error {
	_, err := s.db.Exec(`UPDATE layoffs SET status = 'approved' WHERE id = ?`, id)
	return err
}

func (s *LayoffService) RejectLayoff(id int) error {
	_, err := s.db.Exec(`UPDATE layoffs SET status = 'rejected' WHERE id = ?`, id)
	return err
}

func (s *LayoffService) GetPendingLayoffs() ([]*models.Layoff, error) {
	rows, err := s.db.Query(`
		SELECT l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_type, l.notes, l.status, l.created_at,
		       c.name, c.canonical_name, c.website, c.employee_count, c.industry_id
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		WHERE l.status = 'pending'
		ORDER BY l.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying pending layoffs: %w", err)
	}
	defer rows.Close()

	var layoffs []*models.Layoff
	for rows.Next() {
		layoff := &models.Layoff{Company: &models.Company{}}
		var companyName sql.NullString
		var canonicalName sql.NullString
		err := rows.Scan(
			&layoff.ID, &layoff.Company.ID, &layoff.EmployeesAffected, &layoff.LayoffDate, &layoff.SourceType, &layoff.Notes, &layoff.Status, &layoff.CreatedAt,
			&companyName, &canonicalName, &layoff.Company.Website, &layoff.Company.EmployeeCount, &layoff.Company.IndustryID,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning layoff: %w", err)
		}
		if companyName.Valid {
			layoff.Company.Name = companyName.String
		}
		if canonicalName.Valid && canonicalName.String != "" {
			layoff.Company.Name = canonicalName.String // Use canonical name for display
		}
		layoffs = append(layoffs, layoff)
	}
	fmt.Printf("GetPendingLayoffs: found %d layoffs\n", len(layoffs))
	return layoffs, nil
}

func (s *LayoffService) GetAllLayoffs() ([]*models.Layoff, error) {
	rows, err := s.db.Query(`
		SELECT l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_type, l.notes, l.status, l.created_at
		FROM layoffs l
		ORDER BY l.layoff_date DESC`)
	if err != nil {
		return nil, fmt.Errorf("error querying all layoffs: %w", err)
	}
	defer rows.Close()

	var layoffs []*models.Layoff
	for rows.Next() {
		layoff := &models.Layoff{}
		err := rows.Scan(&layoff.ID, &layoff.CompanyID, &layoff.EmployeesAffected, &layoff.LayoffDate, &layoff.SourceType, &layoff.Notes, &layoff.Status, &layoff.CreatedAt)
		if err != nil {
			continue
		}
		layoffs = append(layoffs, layoff)
	}

	return layoffs, nil
}

func (s *LayoffService) ClearSeedData() error {
	// Delete seed layoffs based on their characteristic notes and source URLs
	_, err := s.db.Exec(`
		DELETE FROM layoffs
		WHERE notes LIKE '%Restructuring due to market conditions%'
		   OR notes LIKE '%Healthcare cost reductions%'
		   OR notes LIKE '%Store closures and online shift%'
		   OR notes LIKE '%Supply chain disruptions%'
		   OR notes LIKE '%Banking consolidation%'
		   OR notes LIKE '%Budget cuts in education%'
		   OR notes LIKE '%Post-pandemic adjustments%'
		   OR notes LIKE '%Fleet automation%'
		   OR notes LIKE '%Housing market slowdown%'
		   OR notes LIKE '%Energy transition%'
		   OR notes LIKE '%Streaming competition%'
		   OR notes LIKE '%Government budget constraints%'
		   OR notes LIKE '%Funding reduction%'
		   OR notes LIKE '%Agricultural market changes%'
		   OR notes LIKE '%Commercial real estate slowdown%'
		   OR source_url LIKE 'https://technews.com/%'
		   OR source_url LIKE 'https://healthnews.com/%'
		   OR source_url LIKE 'https://retailnews.com/%'
		   OR source_url LIKE 'https://manufacturingnews.com/%'
		   OR source_url LIKE 'https://financenews.com/%'
		   OR source_url LIKE 'https://edunews.com/%'
		   OR source_url LIKE 'https://hospitalitynews.com/%'
		   OR source_url LIKE 'https://transportnews.com/%'
		   OR source_url LIKE 'https://constructionnews.com/%'
		   OR source_url LIKE 'https://energynews.com/%'
		   OR source_url LIKE 'https://entertainmentnews.com/%'
		   OR source_url LIKE 'https://govnews.com/%'
		   OR source_url LIKE 'https://nonprofitnews.com/%'
		   OR source_url LIKE 'https://agnews.com/%'
		   OR source_url LIKE 'https://realestatenews.com/%'`)
	if err != nil {
		return fmt.Errorf("error clearing seed data: %w", err)
	}

	return nil
}
