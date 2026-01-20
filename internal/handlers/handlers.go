package handlers

import (
	"bytes"
	"database/sql"
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

	"github.com/labstack/echo-contrib/session"
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
	userService   *services.UserService
	templates     *template.Template
}

func NewHandler(layoffService *services.LayoffService, userService *services.UserService, templates *template.Template) *Handler {
	return &Handler{
		layoffService: layoffService,
		userService:   userService,
		templates:     templates,
	}
}

func (h *Handler) getCurrentUser(c echo.Context) *models.User {
	sess, err := session.Get("session", c)
	if err != nil {
		log.Printf("Session error: %v", err)
		return nil
	}
	userID, ok := sess.Values["user_id"]
	if !ok {
		log.Printf("No user_id in session")
		return nil
	}
	userIDI, ok := userID.(int)
	if !ok {
		log.Printf("user_id not int: %v", userID)
		return nil
	}
	user, err := h.userService.GetUserByID(userIDI)
	if err != nil {
		log.Printf("Error getting user %d: %v", userIDI, err)
		return nil
	}
	log.Printf("Scanned user: ID=%d, IsAdmin=%v", user.ID, user.IsAdmin)
	log.Printf("Current user: %s (admin: %v)", user.Email, user.IsAdmin)
	return user
}

func (h *Handler) Profile(c echo.Context) error {
	user := h.getCurrentUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, "/auth/google")
	}

	prefs, err := h.userService.GetAlertPrefs(user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Render profile content
	var contentBuf bytes.Buffer
	data := map[string]interface{}{
		"User":  user,
		"Prefs": prefs,
	}
	err = h.templates.ExecuteTemplate(&contentBuf, "profile.html", data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Workforce Loss Tracker - Profile",
		"ActivePage": "",
		"Content":    template.HTML(contentBuf.String()),
		"User":       user,
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) UpdateProfile(c echo.Context) error {
	user := h.getCurrentUser(c)
	if user == nil {
		return c.Redirect(http.StatusSeeOther, "/auth/google")
	}

	emailEnabled := c.FormValue("email_alerts_enabled") == "on"
	alertNewData := c.FormValue("alert_new_data") == "on"

	err := h.userService.UpdateAlertPrefs(user.ID, emailEnabled, alertNewData)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.Redirect(http.StatusSeeOther, "/profile")
}

func (h *Handler) AdminDashboard(c echo.Context) error {
	user := h.getCurrentUser(c)
	log.Printf("Admin access attempt: user=%v, isAdmin=%v", user, user != nil && user.IsAdmin)
	if user == nil || !user.IsAdmin {
		// Log denied access
		if user != nil {
			h.userService.LogSessionEvent(user.ID, "admin_denied", c.RealIP(), c.Request().UserAgent())
		}
		return c.Redirect(http.StatusSeeOther, "/")
	}

	// Log successful admin access
	h.userService.LogSessionEvent(user.ID, "admin_access", c.RealIP(), c.Request().UserAgent())

	pending, err := h.layoffService.GetPendingLayoffs()
	if err != nil {
		log.Printf("Error getting pending layoffs: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	log.Printf("Found %d pending layoffs", len(pending))

	// Render admin dashboard
	var contentBuf bytes.Buffer
	data := map[string]interface{}{
		"PendingLayoffs": pending,
	}
	err = h.templates.ExecuteTemplate(&contentBuf, "admin.html", data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Workforce Loss Tracker - Admin",
		"ActivePage": "",
		"Content":    template.HTML(contentBuf.String()),
		"User":       user,
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) ApproveLayoff(c echo.Context) error {
	user := h.getCurrentUser(c)
	if user == nil || !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Access denied"})
	}

	idStr := c.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
	}

	err = h.layoffService.ApproveLayoff(id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Layoff approved"})
}

func (h *Handler) RejectLayoff(c echo.Context) error {
	user := h.getCurrentUser(c)
	if user == nil || !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Access denied"})
	}

	idStr := c.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
	}

	err = h.layoffService.RejectLayoff(id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Layoff rejected"})
}

func (h *Handler) DebugLayoffs(c echo.Context) error {
	user := h.getCurrentUser(c)
	if user == nil || !user.IsAdmin {
		return c.Redirect(http.StatusSeeOther, "/")
	}

	// Get all layoffs for debugging
	layoffs, err := h.layoffService.GetAllLayoffs()
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	result := "All layoffs:\n"
	for _, l := range layoffs {
		result += fmt.Sprintf("ID: %d, Company: %s, Status: %s, Created: %s\n", l.ID, l.Company.Name, l.Status, l.CreatedAt)
	}

	return c.String(http.StatusOK, result)
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
		"Title":      "Workforce Loss Tracker - Dashboard",
		"ActivePage": "dashboard",
		"Content":    template.HTML(contentBuf.String()),
		"User":       h.getCurrentUser(c),
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
		"Layoffs": layoffs.Data,
		"Filters": params,
		"Pagination": map[string]interface{}{
			"Page":       layoffs.Page,
			"TotalPages": layoffs.TotalPages,
			"Total":      layoffs.Total,
		},
		"Industries": industries,
	}
	err = h.templates.ExecuteTemplate(&contentBuf, "tracker.html", data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Workforce Loss Tracker - Browse Workforce Losses",
		"ActivePage": "tracker",
		"Content":    template.HTML(contentBuf.String()),
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

	// Render layoff detail content
	var contentBuf bytes.Buffer
	data := map[string]interface{}{
		"Layoff": layoff,
		"User":   h.getCurrentUser(c),
	}
	err = h.templates.ExecuteTemplate(&contentBuf, "layoff_detail.html", data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      fmt.Sprintf("%s Workforce Loss Details", layoff.Company.Name),
		"ActivePage": "",
		"Content":    template.HTML(contentBuf.String()),
		"User":       h.getCurrentUser(c),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) NewLayoff(c echo.Context) error {
	industries, err := h.layoffService.GetIndustries()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Render new layoff form
	var contentBuf bytes.Buffer
	data := map[string]interface{}{
		"Industries": industries,
	}
	err = h.templates.ExecuteTemplate(&contentBuf, "new_layoff.html", data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Workforce Loss Tracker - Report Workforce Loss",
		"ActivePage": "",
		"Content":    template.HTML(contentBuf.String()),
		"User":       h.getCurrentUser(c),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) CreateLayoff(c echo.Context) error {
	// Parse form data
	companyName := c.FormValue("company_name")
	employeesStr := c.FormValue("employees_affected")
	layoffDateStr := c.FormValue("layoff_date")
	sourceURL := c.FormValue("source_url")
	notes := c.FormValue("notes")
	industryIDStr := c.FormValue("industry_id")

	// Validate required fields
	if companyName == "" || employeesStr == "" || layoffDateStr == "" {
		return c.String(http.StatusBadRequest, "Company name, employees affected, and layoff date are required")
	}

	// Parse employees
	employees, err := strconv.Atoi(employeesStr)
	if err != nil || employees <= 0 {
		return c.String(http.StatusBadRequest, "Invalid number of employees")
	}

	// Parse date
	layoffDate, err := time.Parse("2006-01-02", layoffDateStr)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid layoff date format")
	}

	// Get or create company
	companyID, err := h.layoffService.GetOrCreateCompany(companyName, industryIDStr)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to create company")
	}

	// Create layoff
	layoff := &models.Layoff{
		CompanyID:         companyID,
		EmployeesAffected: employees,
		LayoffDate:        layoffDate,
		SourceURL:         sql.NullString{String: sourceURL, Valid: sourceURL != ""},
		Notes:             notes,
		Status:            "pending",
		CreatedAt:         time.Now(),
	}

	err = h.layoffService.CreateLayoff(layoff)
	if err != nil {
		log.Printf("Error creating layoff: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to create layoff")
	}

	log.Printf("Layoff created successfully: company %s, employees %d, status %s", companyName, employees, layoff.Status)

	// Redirect to success or home
	return c.Redirect(http.StatusSeeOther, "/?message=Layoff+reported+successfully")
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
		"Title":      "Workforce Loss Tracker - Industries",
		"ActivePage": "industries",
		"Content":    template.HTML(contentBuf.String()),
		"User":       h.getCurrentUser(c),
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
		"Title":      "Workforce Loss Tracker - Browse Workforce Losses",
		"ActivePage": "tracker",
		"Content":    template.HTML(contentBuf.String()),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) Privacy(c echo.Context) error {
	// Render privacy content
	var contentBuf bytes.Buffer
	err := h.templates.ExecuteTemplate(&contentBuf, "privacy.html", map[string]interface{}{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Privacy Policy - Workforce Loss Tracker",
		"ActivePage": "privacy",
		"Content":    template.HTML(contentBuf.String()),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) Terms(c echo.Context) error {
	// Render terms content
	var contentBuf bytes.Buffer
	err := h.templates.ExecuteTemplate(&contentBuf, "terms.html", map[string]interface{}{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Terms of Service - Workforce Loss Tracker",
		"ActivePage": "terms",
		"Content":    template.HTML(contentBuf.String()),
	}

	return c.Render(http.StatusOK, "layout.html", layoutData)
}

func (h *Handler) Contact(c echo.Context) error {
	// Render contact content
	var contentBuf bytes.Buffer
	err := h.templates.ExecuteTemplate(&contentBuf, "contact.html", map[string]interface{}{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	layoutData := map[string]interface{}{
		"Title":      "Contact Us - Workforce Loss Tracker",
		"ActivePage": "contact",
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
			item.Company.Industry,
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
