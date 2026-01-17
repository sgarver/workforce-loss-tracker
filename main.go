package main

import (
	"fmt"
	"html/template"
	"layoff-tracker/internal/database"
	"layoff-tracker/internal/handlers"
	"layoff-tracker/internal/models"
	"layoff-tracker/internal/services"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func performNightlyImport(freeDataService *services.FreeDataService, notificationService *services.NotificationService) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in nightly import: %v", r)
			// Could send admin notification here
		}
	}()

	log.Println("Starting nightly automated import...")

	result, err := freeDataService.ImportWithChangeDetection()
	if err != nil {
		log.Printf("Nightly import failed: %v", err)
		// Send notification about failure
		failedResult := &models.ImportResult{
			Status:   "failed",
			Duration: result.Duration,
			Error:    err,
		}
		failedHistory := &models.ImportHistory{
			SourceURL:    "automated-import",
			ImportedAt:   time.Now(),
			RecordCount:  0,
			ContentHash:  "",
			Status:       "failed",
			ErrorMessage: err.Error(),
			DurationMs:   int(result.Duration.Milliseconds()),
		}
		notificationService.SendImportReport(failedResult, failedHistory)
		return
	}

	log.Printf("Nightly import completed: %s (%d records added, duration: %v)",
		result.Status, result.RecordsAdded, result.Duration)

	// Send success notification if records were added
	if result.Status == "updated" && result.RecordsAdded > 0 {
		// Create a mock history entry for notification
		history := &models.ImportHistory{
			SourceURL:   "automated-import",
			ImportedAt:  time.Now(),
			RecordCount: result.RecordsAdded,
			Status:      "completed",
			DurationMs:  int(result.Duration.Milliseconds()),
		}
		notificationService.SendImportReport(result, history)
	}
}

func main() {
	// Initialize database
	db, err := database.NewDB("layoff_tracker.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.RunMigrations("migrations"); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Printf("Working directory: %s", os.Getenv("PWD"))

	// Initialize services
	layoffService := services.NewLayoffService(db)

	// Load templates
	templates := template.Must(template.New("").Funcs(template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
		"multiply": func(a, b int) int { return a + b },
		"timeAgo": func(t time.Time) string {
			duration := time.Since(t)
			isFuture := duration < 0
			duration = duration.Abs()
			days := int(duration.Hours() / 24)
			suffix := "ago"
			if isFuture {
				suffix = "from now"
			}
			if days == 0 {
				hours := int(duration.Hours())
				if hours == 0 {
					minutes := int(duration.Minutes())
					if minutes == 0 {
						return "now"
					}
					return fmt.Sprintf("%d minutes %s", minutes, suffix)
				}
				return fmt.Sprintf("%d hours %s", hours, suffix)
			} else if days == 1 {
				return fmt.Sprintf("1 day %s", suffix)
			} else if days < 30 {
				return fmt.Sprintf("%d days %s", days, suffix)
			} else if days < 365 {
				months := days / 30
				return fmt.Sprintf("%d months %s", months, suffix)
			} else {
				years := days / 365
				return fmt.Sprintf("%d years %s", years, suffix)
			}
		},
	}).ParseFiles(
		"templates/dashboard.html",
		"templates/tracker.html",
		"templates/layoff_detail.html",
		"templates/new_layoff.html",
		"templates/industries.html",
		"templates/faq.html",
		"templates/layout.html",
	))

	// Initialize handlers
	handler := handlers.NewHandler(layoffService, templates)

	// Initialize Echo
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		Root:   "static",
		Browse: false,
	}))

	// Template renderer
	renderer := &handlers.TemplateRenderer{
		Templates: templates,
	}
	e.Renderer = renderer

	// Routes
	e.GET("/", handler.Dashboard)
	e.GET("/dashboard", handler.Dashboard)
	e.GET("/tracker", handler.Tracker)
	e.GET("/layoffs/:id", handler.LayoffDetail)
	e.GET("/layoffs/new", handler.NewLayoff)
	e.POST("/layoffs", handler.CreateLayoff)
	e.GET("/industries", handler.Industries)
	e.GET("/faq", handler.FAQ)
	e.GET("/export/csv", handler.ExportCSV)

	// Setup free data import routes
	freeDataService := services.NewFreeDataService(db)
	services.SetupFreeDataRoutes(e, db)

	// Initialize notification service (configure as needed)
	notificationService := services.NewNotificationService(
		"", 587, "noreply@layofftracker.com",
		[]string{"admin@example.com"}, "", "")

	// Start automated nightly import scheduler
	go func() {
		log.Println("Starting automated nightly import scheduler...")
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			performNightlyImport(freeDataService, notificationService)
		}
	}()

	// API routes
	api := e.Group("/api")
	api.GET("/stats", func(c echo.Context) error {
		monthsStr := c.QueryParam("months")
		months := 6 // default to 6 months
		if monthsStr != "" {
			if parsed, err := strconv.Atoi(monthsStr); err == nil && parsed > 0 {
				months = parsed
			}
		}

		stats, err := layoffService.GetStatsWithMonths(months)
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, stats)
	})
	api.GET("/layoffs", func(c echo.Context) error {
		params := handler.ParseFilterParams(c)
		layoffs, err := layoffService.GetLayoffs(params)
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, layoffs)
	})
	api.GET("/industries", func(c echo.Context) error {
		industries, err := layoffService.GetIndustries()
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, industries)
	})
	api.GET("/sponsored", func(c echo.Context) error {
		listings, err := layoffService.GetSponsoredListings()
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, listings)
	})
	api.GET("/current-layoffs", func(c echo.Context) error {
		layoffs, err := layoffService.GetCurrentLayoffs()
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, layoffs)
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(e.Start(":" + port))
}
