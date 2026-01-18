package models

import (
	"database/sql"
	"time"
)

type Industry struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// Import-related structs
type ImportResult struct {
	Status       string        `json:"status"`
	RecordsAdded int           `json:"records_added"`
	Duration     time.Duration `json:"duration"`
	Error        error         `json:"-"`
}

type ImportHistory struct {
	ID           int       `json:"id"`
	SourceURL    string    `json:"source_url"`
	ImportedAt   time.Time `json:"imported_at"`
	RecordCount  int       `json:"record_count"`
	ContentHash  string    `json:"content_hash"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message,omitempty"`
	DurationMs   int       `json:"duration_ms"`
}

type Company struct {
	ID            int       `json:"id"`
	Name          string    `json:"name"`
	EmployeeCount *int      `json:"employee_count"`
	IndustryID    *int      `json:"industry_id"`
	Industry      *Industry `json:"industry,omitempty"`
	Website       *string   `json:"website"`
	LogoURL       *string   `json:"logo_url"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Layoff struct {
	ID                int            `json:"id"`
	CompanyID         int            `json:"company_id"`
	Company           *Company       `json:"company,omitempty"`
	EmployeesAffected int            `json:"employees_affected"`
	LayoffDate        time.Time      `json:"layoff_date"`
	SourceURL         sql.NullString `json:"source_url"`
	Notes             string         `json:"notes"`
	Status            string         `json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
}

type SponsoredListing struct {
	ID        int       `json:"id"`
	CompanyID int       `json:"company_id"`
	Company   *Company  `json:"company,omitempty"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	Message   string    `json:"message"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type FilterParams struct {
	IndustryID    int    `json:"industry_id"`
	MinEmployees  int    `json:"min_employees"`
	MaxEmployees  int    `json:"max_employees"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	Search        string `json:"search"`
	SortBy        string `json:"sort_by"`
	SortDirection string `json:"sort_direction"`
	Page          int    `json:"page"`
	Limit         int    `json:"limit"`
}

type Comment struct {
	ID          int       `json:"id"`
	LayoffID    int       `json:"layoff_id"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PaginatedResult struct {
	Data       interface{} `json:"data"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalPages int         `json:"total_pages"`
}

type User struct {
	ID         int       `json:"id" db:"id"`
	Provider   string    `json:"provider" db:"provider"`
	ProviderID string    `json:"provider_id" db:"provider_id"`
	Email      string    `json:"email" db:"email"`
	Name       string    `json:"name" db:"name"`
	AvatarURL  string    `json:"avatar_url" db:"avatar_url"`
	IsAdmin    bool      `json:"is_admin" db:"is_admin"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

type Stats struct {
	TotalLayoffs           int `json:"total_layoffs"`
	TotalCompanies         int `json:"total_companies"`
	TotalEmployeesAffected int `json:"total_employees_affected"`
	RecentLayoffs          int `json:"recent_layoffs"`
	RecentEmployees        int `json:"recent_employees"`
	// Formatted versions for display
	TotalLayoffsFormatted   string              `json:"-"`
	TotalCompaniesFormatted string              `json:"-"`
	TotalEmployeesFormatted string              `json:"-"`
	RecentLayoffsFormatted  string              `json:"-"`
	MonthlyTrend            []MonthlyTrend      `json:"monthly_trend"`
	IndustryBreakdown       []IndustryBreakdown `json:"industry_breakdown"`
}

type MonthlyTrend struct {
	Month       string `json:"month"`
	Count       int    `json:"count"`
	Employees   int    `json:"employees"`
	PeriodLabel string `json:"period_label,omitempty"`
}

type IndustryBreakdown struct {
	Industry  string `json:"industry"`
	Count     int    `json:"count"`
	Employees int    `json:"employees"`
}
