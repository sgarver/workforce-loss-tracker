package services

import (
	"database/sql"
	"fmt"
	"layoff-tracker/internal/database"
	"layoff-tracker/internal/models"
	"strconv"
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
	if params.IndustryID > 0 {
		whereClauses = append(whereClauses, "c.industry_id = ?")
		args = append(args, params.IndustryID)
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
			orderBy = "i.name " + direction
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
		LEFT JOIN industries i ON c.industry_id = i.id
		WHERE %s`, whereSQL)

	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("error getting total count: %w", err)
	}

	// Get paginated results
	query := fmt.Sprintf(`
		SELECT
			l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_url, l.notes, l.status, l.created_at,
			c.id, c.name, c.employee_count, c.website, c.logo_url, c.created_at, c.updated_at,
			i.id, i.name, i.slug
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		LEFT JOIN industries i ON c.industry_id = i.id
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
			Company: &models.Company{
				Industry: &models.Industry{},
			},
		}

		var employeesAffected sql.NullInt64
		var layoffDate sql.NullTime
		var sourceURL sql.NullString
		var notes sql.NullString
		var status sql.NullString
		var createdAt sql.NullTime
		var layoffCompanyID sql.NullInt64
		var companyID sql.NullInt64
		var companyName sql.NullString
		var employeeCount sql.NullInt64
		var website sql.NullString
		var logoURL sql.NullString
		var companyCreatedAt sql.NullTime
		var companyUpdatedAt sql.NullTime
		var industryID sql.NullInt64
		var industryName sql.NullString
		var industrySlug sql.NullString

		err := rows.Scan(
			&layoff.ID, &layoffCompanyID, &employeesAffected, &layoffDate,
			&sourceURL, &notes, &status, &createdAt,
			&companyID, &companyName, &employeeCount,
			&website, &logoURL, &companyCreatedAt, &companyUpdatedAt,
			&industryID, &industryName, &industrySlug,
		)

		layoff.CompanyID = int(layoffCompanyID.Int64)
		layoff.EmployeesAffected = int(employeesAffected.Int64)
		if layoffDate.Valid {
			layoff.LayoffDate = layoffDate.Time
		} else {
			layoff.LayoffDate = time.Now()
		}
		layoff.SourceURL = sourceURL
		layoff.Notes = notes.String
		layoff.Status = status.String
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
		if industryID.Valid {
			layoff.Company.Industry.ID = int(industryID.Int64)
		}
		layoff.Company.Industry.Name = industryName.String
		layoff.Company.Industry.Slug = industrySlug.String
		layoff.Company.Industry.CreatedAt = time.Now()

		if employeeCount.Valid && employeeCount.Int64 > 0 {
			val := int(employeeCount.Int64)
			layoff.Company.EmployeeCount = &val
		}
		if website.Valid {
			layoff.Company.Website = &website.String
		}
		if logoURL.Valid {
			layoff.Company.LogoURL = &logoURL.String
		}
		if industryID.Valid {
			layoff.Company.Industry.ID = int(industryID.Int64)
		}
		if industryName.Valid {
			layoff.Company.Industry.Name = industryName.String
		}
		if industrySlug.Valid {
			layoff.Company.Industry.Slug = industrySlug.String
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

func (s *LayoffService) GetLayoff(id int) (*models.Layoff, error) {
	query := `
		SELECT
			l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_url, l.notes, l.status, l.created_at,
			c.id, c.name, c.employee_count, c.website, c.logo_url, c.created_at, c.updated_at,
			i.id, i.name, i.slug
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		LEFT JOIN industries i ON c.industry_id = i.id
		WHERE l.id = $1`

	layoff := &models.Layoff{
		Company: &models.Company{
			Industry: &models.Industry{},
		},
	}

	var logoURL sql.NullString
	var website sql.NullString
	var industryID sql.NullInt64
	var industryName sql.NullString
	var industrySlug sql.NullString
	var employeeCount sql.NullInt64

	rows, err := s.db.Query(query, id)
	if err != nil {
		return nil, fmt.Errorf("error querying layoff: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("layoff not found")
	}

	err = rows.Scan(
		&layoff.ID, &layoff.CompanyID, &layoff.EmployeesAffected, &layoff.LayoffDate,
		&layoff.SourceURL, &layoff.Notes, &layoff.Status, &layoff.CreatedAt,
		&layoff.Company.ID, &layoff.Company.Name, &employeeCount,
		&website, &logoURL, &layoff.Company.CreatedAt, &layoff.Company.UpdatedAt,
		&industryID, &industryName, &industrySlug,
	)

	if website.Valid {
		layoff.Company.Website = &website.String
	}
	if logoURL.Valid {
		layoff.Company.LogoURL = &logoURL.String
	}
	if employeeCount.Valid && employeeCount.Int64 > 0 {
		val := int(employeeCount.Int64)
		layoff.Company.EmployeeCount = &val
	}
	if industryID.Valid {
		layoff.Company.Industry.ID = int(industryID.Int64)
	}
	if industryName.Valid {
		layoff.Company.Industry.Name = industryName.String
	}
	if industrySlug.Valid {
		layoff.Company.Industry.Slug = industrySlug.String
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
	status := layoff.Status
	if status == "" {
		status = "completed"
		if layoff.LayoffDate.After(time.Now()) {
			status = "planned"
		}
	}

	query := `
		INSERT INTO layoffs (company_id, employees_affected, layoff_date, source_url, notes, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	err := s.db.QueryRow(query,
		layoff.CompanyID,
		layoff.EmployeesAffected,
		layoff.LayoffDate,
		layoff.SourceURL,
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

	// Industry breakdown
	industryQuery := `
		SELECT
			i.name,
			COUNT(*) as count,
			COALESCE(SUM(l.employees_affected), 0) as employees
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		JOIN industries i ON c.industry_id = i.id
		GROUP BY i.name
		ORDER BY employees DESC`

	rows, err = s.db.Query(industryQuery)
	if err != nil {
		return nil, fmt.Errorf("error getting industry breakdown: %w", err)
	}
	defer rows.Close()

	var industryBreakdown []models.IndustryBreakdown
	for rows.Next() {
		var breakdown models.IndustryBreakdown
		err := rows.Scan(&breakdown.Industry, &breakdown.Count, &breakdown.Employees)
		if err != nil {
			return nil, fmt.Errorf("error scanning industry breakdown: %w", err)
		}
		industryBreakdown = append(industryBreakdown, breakdown)
	}
	stats.IndustryBreakdown = industryBreakdown

	return stats, nil
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
			l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_url, l.notes, l.status, l.created_at,
			c.id, c.name, c.employee_count, c.website, c.logo_url, c.created_at, c.updated_at,
			i.id, i.name, i.slug
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		LEFT JOIN industries i ON c.industry_id = i.id
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
			Company: &models.Company{
				Industry: &models.Industry{},
			},
		}

		var logoURL sql.NullString
		var website sql.NullString
		var industryID sql.NullInt64
		var industryName sql.NullString
		var industrySlug sql.NullString
		var employeeCount sql.NullInt64

		err := rows.Scan(
			&layoff.ID, &layoff.CompanyID, &layoff.EmployeesAffected, &layoff.LayoffDate,
			&layoff.SourceURL, &layoff.Notes, &layoff.Status, &layoff.CreatedAt,
			&layoff.Company.ID, &layoff.Company.Name, &employeeCount,
			&website, &logoURL, &layoff.Company.CreatedAt, &layoff.Company.UpdatedAt,
			&industryID, &industryName, &industrySlug,
		)

		if employeeCount.Valid && employeeCount.Int64 > 0 {
			val := int(employeeCount.Int64)
			layoff.Company.EmployeeCount = &val
		}
		if website.Valid {
			layoff.Company.Website = &website.String
		}
		if logoURL.Valid {
			layoff.Company.LogoURL = &logoURL.String
		}
		if industryID.Valid {
			layoff.Company.Industry.ID = int(industryID.Int64)
		}
		if industryName.Valid {
			layoff.Company.Industry.Name = industryName.String
		}
		if industrySlug.Valid {
			layoff.Company.Industry.Slug = industrySlug.String
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

func (s *LayoffService) GetOrCreateCompany(name, industryIDStr string) (int, error) {
	// Try to find existing company
	var companyID int
	query := `SELECT id FROM companies WHERE name = ?`
	err := s.db.QueryRow(query, name).Scan(&companyID)
	if err == nil {
		// Company exists
		return companyID, nil
	}

	// Create new company
	var industryID interface{} = nil
	if industryIDStr != "" {
		if id, err := strconv.Atoi(industryIDStr); err == nil {
			industryID = id
		}
	}

	query = `INSERT INTO companies (name, industry_id) VALUES (?, ?)`
	result, err := s.db.Exec(query, name, industryID)
	if err != nil {
		return 0, fmt.Errorf("failed to create company: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get company ID: %w", err)
	}
	return int(id), nil
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
		SELECT l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_url, l.notes, l.status, l.created_at,
		       c.name, c.website, c.employee_count, c.industry_id
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
		err := rows.Scan(
			&layoff.ID, &layoff.Company.ID, &layoff.EmployeesAffected, &layoff.LayoffDate, &layoff.SourceURL, &layoff.Notes, &layoff.Status, &layoff.CreatedAt,
			&layoff.Company.Name, &layoff.Company.Website, &layoff.Company.EmployeeCount, &layoff.Company.IndustryID,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning layoff: %w", err)
		}
		layoffs = append(layoffs, layoff)
	}
	fmt.Printf("GetPendingLayoffs: found %d layoffs\n", len(layoffs))
	return layoffs, nil
}

func (s *LayoffService) GetAllLayoffs() ([]*models.Layoff, error) {
	rows, err := s.db.Query(`
		SELECT l.id, l.company_id, l.employees_affected, l.layoff_date, l.source_url, l.notes, l.status, l.created_at,
		       c.name, c.website, c.employee_count, c.industry_id
		FROM layoffs l
		JOIN companies c ON l.company_id = c.id
		ORDER BY l.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying all layoffs: %w", err)
	}
	defer rows.Close()

	var layoffs []*models.Layoff
	for rows.Next() {
		layoff := &models.Layoff{Company: &models.Company{}}
		err := rows.Scan(
			&layoff.ID, &layoff.Company.ID, &layoff.EmployeesAffected, &layoff.LayoffDate, &layoff.SourceURL, &layoff.Notes, &layoff.Status, &layoff.CreatedAt,
			&layoff.Company.Name, &layoff.Company.Website, &layoff.Company.EmployeeCount, &layoff.Company.IndustryID,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning layoff: %w", err)
		}
		layoffs = append(layoffs, layoff)
	}
	return layoffs, nil
}
