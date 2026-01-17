package handlers

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"layoff-tracker/internal/models"
	"layoff-tracker/internal/services"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type TemplateRenderer struct {
	Templates *template.Template
}

func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.Templates.ExecuteTemplate(w, name, data)
}

type Handler struct {
	layoffService *services.LayoffService
	templates     *template.Template
}

func NewHandler(layoffService *services.LayoffService, templates *template.Template) *Handler {
	return &Handler{
		layoffService: layoffService,
		templates:     templates,
	}
}

func (h *Handler) Dashboard(c echo.Context) error {
	stats, err := h.layoffService.GetStats()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Format numbers for display (fallback if service didn't set them)
	if stats.TotalLayoffsFormatted == "" {
		formatNumber := func(n int) string {
			if n >= 1000000 {
				return fmt.Sprintf("%.1fM", float64(n)/1000000)
			} else if n >= 1000 {
				return fmt.Sprintf("%.1fK", float64(n)/1000)
			}
			return fmt.Sprintf("%d", n)
		}
		stats.TotalLayoffsFormatted = formatNumber(stats.TotalLayoffs)
		stats.TotalCompaniesFormatted = formatNumber(stats.TotalCompanies)
		stats.TotalEmployeesFormatted = formatNumber(stats.TotalEmployeesAffected)
		stats.RecentLayoffsFormatted = formatNumber(stats.RecentLayoffs)
	}

	// Calculate current quarter information
	now := time.Now()
	year := now.Year()
	month := int(now.Month())
	quarter := (month-1)/3 + 1

	var quarterMonths string
	switch quarter {
	case 1:
		quarterMonths = "Jan-Mar"
	case 2:
		quarterMonths = "Apr-Jun"
	case 3:
		quarterMonths = "Jul-Sep"
	case 4:
		quarterMonths = "Oct-Dec"
	default:
		quarterMonths = "Unknown"
	}

	// Get last import time
	lastImportTime, err := h.layoffService.GetLastImportTime()
	if err != nil {
		log.Printf("Error getting last import time: %v", err)
		lastImportTime = "Unknown"
	}

	// Render dashboard content
	var contentBuf bytes.Buffer
	err = h.templates.ExecuteTemplate(&contentBuf, "dashboard.html", map[string]interface{}{
		"Stats":             stats,
		"CurrentQuarter":    fmt.Sprintf("Q%d %d (%s)", quarter, year, quarterMonths),
		"LastImportTime":    lastImportTime,
		"SponsoredListings": []interface{}{}, // Empty for now
	})

	layoutData := map[string]interface{}{
		"Title":      "Tech Layoff Tracker - Dashboard",
		"ActivePage": "dashboard",
		"Content":    template.HTML(contentBuf.String()),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) Tracker(c echo.Context) error {
	// Parse query parameters
	params := h.ParseFilterParams(c)

	layoffs, err := h.layoffService.GetLayoffs(params)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	industries, err := h.layoffService.GetIndustries()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Render tracker content
	var contentBuf bytes.Buffer
	data := map[string]interface{}{
		"Layoffs":    layoffs.Data,
		"Pagination": layoffs,
		"Industries": industries,
		"Filters":    params,
	}
	err = h.templates.ExecuteTemplate(&contentBuf, "tracker.html", data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Tech Layoff Tracker - Browse Layoffs",
		"ActivePage": "tracker",
		"Content":    contentBuf.String(),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) LayoffDetail(c echo.Context) error {
	layoffID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid layoff ID"})
	}

	layoff, err := h.layoffService.GetLayoff(layoffID)
	if err != nil {
		if err.Error() == "layoff not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Layoff not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	data := map[string]interface{}{
		"Layoff": layoff,
	}

	return c.Render(http.StatusOK, "layoff_detail", data)
}

func (h *Handler) CreateLayoff(c echo.Context) error {
	return c.String(http.StatusOK, "Create layoff coming soon.")
}

// Comment handlers
func (h *Handler) GetComments(c echo.Context) error {
	layoffID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid layoff ID"})
	}

	comments, err := h.layoffService.GetComments(layoffID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, comments)
}

func (h *Handler) CreateComment(c echo.Context) error {
	layoffID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid layoff ID"})
	}

	authorName := c.FormValue("author_name")
	content := c.FormValue("content")
	authorEmail := c.FormValue("author_email")

	if authorName == "" || content == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Name and content are required"})
	}

	comment := &models.Comment{
		LayoffID:    layoffID,
		AuthorName:  authorName,
		AuthorEmail: authorEmail,
		Content:     content,
	}

	if err := h.layoffService.CreateComment(comment); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, comment)
}

func (h *Handler) Industries(c echo.Context) error {
	industries, err := h.layoffService.GetIndustries()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Render industries content
	var contentBuf bytes.Buffer
	data := map[string]interface{}{
		"Industries": industries,
	}
	err = h.templates.ExecuteTemplate(&contentBuf, "industries.html", data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Tech Layoff Tracker - Industries",
		"ActivePage": "industries",
		"Content":    template.HTML(contentBuf.String()),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) FAQ(c echo.Context) error {
	// Render faq content
	var contentBuf bytes.Buffer
	err := h.templates.ExecuteTemplate(&contentBuf, "faq.html", map[string]interface{}{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Tech Layoff Tracker - FAQ",
		"ActivePage": "faq",
		"Content":    template.HTML(contentBuf.String()),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) ExportCSV(c echo.Context) error {
	params := h.ParseFilterParams(c)
	params.Limit = 10000 // Export up to 10,000 records

	layoffs, err := h.layoffService.GetLayoffs(params)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Generate CSV
	var csvLines []string
	csvLines = append(csvLines, "Company,Industry,Employees Affected,Layoff Date,Source URL,Notes")

	for _, item := range layoffs.Data.([]*models.Layoff) {
		sourceURL := ""
		if item.SourceURL.Valid {
			sourceURL = item.SourceURL.String
		}
		line := fmt.Sprintf(`"%s","%s",%d,"%s","%s","%s"`,
			item.Company.Name,
			item.Company.Industry.Name,
			item.EmployeesAffected,
			item.LayoffDate.Format("2006-01-02"),
			sourceURL,
			strings.ReplaceAll(item.Notes, `"`, `""`))
		csvLines = append(csvLines, line)
	}

	csvContent := strings.Join(csvLines, "\n")

	c.Response().Header().Set("Content-Type", "text/csv")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=layoffs.csv")
	return c.String(http.StatusOK, csvContent)
}

func (h *Handler) ParseFilterParams(c echo.Context) models.FilterParams {
	params := models.FilterParams{}

	// Parse pagination
	if page, err := strconv.Atoi(c.QueryParam("page")); err == nil {
		params.Page = page
	}
	if limit, err := strconv.Atoi(c.QueryParam("limit")); err == nil {
		params.Limit = limit
	}

	// Parse filters
	if industryID, err := strconv.Atoi(c.QueryParam("industry_id")); err == nil {
		params.IndustryID = industryID
	}
	if minEmployees, err := strconv.Atoi(c.QueryParam("min_employees")); err == nil {
		params.MinEmployees = minEmployees
	}
	if maxEmployees, err := strconv.Atoi(c.QueryParam("max_employees")); err == nil {
		params.MaxEmployees = maxEmployees
	}

	params.StartDate = c.QueryParam("start_date")
	params.EndDate = c.QueryParam("end_date")
	params.Search = c.QueryParam("search")
	params.SortBy = c.QueryParam("sort_by")
	params.SortDirection = c.QueryParam("sort_direction")

	return params
}

func isHTMXRequest(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}
