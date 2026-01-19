package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"layoff-tracker/internal/database"
	"layoff-tracker/internal/handlers"
	"layoff-tracker/internal/services"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/robfig/cron/v3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type GoogleUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	VerifiedEmail bool   `json:"verified_email"`
}

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

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
	userService := services.NewUserService(db)
	alertService := services.NewAlertService(userService, "localhost", 25, "alerts@localhost")

	// Start daily data check alerts
	c := cron.New()
	c.AddFunc("@daily", func() {
		sendNewDataAlerts(layoffService, userService, alertService)
	})
	c.Start()

	// OAuth2 config
	googleOAuthConfig := &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("BASE_URL") + "/auth/google/callback",
		Scopes:       []string{"openid", "profile", "email"},
		Endpoint:     google.Endpoint,
	}

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
		"templates/profile.html",
		"templates/admin.html",
	))

	// Initialize handlers
	handler := handlers.NewHandler(layoffService, userService, templates)

	// Initialize Echo
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("your-secret-key")))) // Change for production
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
	e.GET("/layoffs/:id/comments", handler.GetComments)
	e.POST("/layoffs/:id/comments", handler.CreateComment)
	e.GET("/layoffs/new", handler.NewLayoff)
	e.GET("/layoffs/:id", handler.LayoffDetail)
	e.POST("/layoffs", handler.CreateLayoff)
	e.GET("/industries", handler.Industries)
	e.GET("/faq", handler.FAQ)
	e.GET("/privacy", handler.Privacy)
	e.GET("/export/csv", handler.ExportCSV)

	// Setup free data import routes
	services.SetupFreeDataRoutes(e, db)

	// Initialize notification service (configure as needed)
	// notificationService := services.NewNotificationService(
	// 	"", 587, "noreply@layofftracker.com",
	// 	[]string{"admin@example.com"}, "", "")

	// Start automated nightly import scheduler
	// go func() {
	// 	log.Println("Starting automated nightly import scheduler...")
	// 	ticker := time.NewTicker(24 * time.Hour)
	// 	defer ticker.Stop()

	// 	for range ticker.C {
	// 		performNightlyImport(freeDataService, notificationService)
	// 	}
	// }()

	// Auth routes
	e.GET("/auth/google", func(c echo.Context) error {
		url := googleOAuthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline)
		return c.Redirect(http.StatusTemporaryRedirect, url)
	})
	e.GET("/auth/google/callback", func(c echo.Context) error {
		code := c.QueryParam("code")
		if code == "" {
			return c.String(http.StatusBadRequest, "No code provided")
		}

		token, err := googleOAuthConfig.Exchange(c.Request().Context(), code)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to exchange token")
		}

		client := googleOAuthConfig.Client(c.Request().Context(), token)
		resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to get user info")
		}
		defer resp.Body.Close()

		var googleUser GoogleUser
		if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
			return c.String(http.StatusInternalServerError, "Failed to decode user info")
		}

		// Create or update user
		user, err := userService.CreateUser("google", googleUser.ID, googleUser.Email, googleUser.Name, googleUser.Picture)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to create user")
		}

		log.Printf("User logged in: %s (ID: %d, Admin: %v)", user.Email, user.ID, user.IsAdmin)

		// Store user ID in session
		sess, _ := session.Get("session", c)
		sess.Values["user_id"] = user.ID
		sess.Save(c.Request(), c.Response())

		log.Printf("Session set for user ID: %d", user.ID)

		// Log login event
		userService.LogSessionEvent(user.ID, "login", c.RealIP(), c.Request().UserAgent())

		return c.Redirect(http.StatusSeeOther, "/")
	})
	e.GET("/auth/logout", func(c echo.Context) error {
		sess, _ := session.Get("session", c)
		userID, ok := sess.Values["user_id"].(int)
		delete(sess.Values, "user_id")
		sess.Save(c.Request(), c.Response())

		// Log logout if user was logged in
		if ok {
			userService.LogSessionEvent(userID, "logout", c.RealIP(), c.Request().UserAgent())
		}

		return c.Redirect(http.StatusSeeOther, "/")
	})

	// Profile routes
	e.GET("/profile", handler.Profile)
	e.POST("/profile", handler.UpdateProfile)

	// Admin routes
	e.GET("/admin", handler.AdminDashboard)
	e.POST("/admin/approve", handler.ApproveLayoff)
	e.POST("/admin/reject", handler.RejectLayoff)
	e.GET("/debug/layoffs", handler.DebugLayoffs)

	// Health check
	e.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "pong")
	})

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
	log.Fatal(e.Start("0.0.0.0:" + port))
}

func sendNewDataAlerts(layoffService *services.LayoffService, userService *services.UserService, alertService *services.AlertService) {
	// Get current total layoffs
	stats, err := layoffService.GetStats()
	if err != nil {
		log.Printf("Error getting stats for alerts: %v", err)
		return
	}
	currentCount := stats.TotalLayoffs

	// Get last alerted count
	lastCountStr, err := userService.GetSystemSetting("last_alerted_layoff_count")
	if err != nil {
		log.Printf("Error getting last alerted count: %v", err)
		return
	}

	var lastCount int
	if lastCountStr != "" {
		lastCount, _ = strconv.Atoi(lastCountStr)
	}

	if currentCount <= lastCount {
		// No new data
		log.Printf("No new layoff data found (current: %d, last: %d)", currentCount, lastCount)
		return
	}

	newCount := currentCount - lastCount
	log.Printf("New layoff data detected: %d new records (total: %d)", newCount, currentCount)

	// Get all users who want alerts
	userIDs, err := userService.GetUsersForNewDataAlerts()
	if err != nil {
		log.Printf("Error getting users for alerts: %v", err)
		return
	}

	log.Printf("Sending alerts to %d users", len(userIDs))
	for _, userID := range userIDs {
		err := alertService.SendNewDataAlert(userID, newCount, time.Now().Format("January 2, 2006 at 3:04 PM UTC"))
		if err != nil {
			log.Printf("Error sending alert to user %d: %v", userID, err)
		} else {
			log.Printf("Alert sent successfully to user %d", userID)
		}
	}

	// Update last alerted count
	err = userService.SetSystemSetting("last_alerted_layoff_count", strconv.Itoa(currentCount))
	if err != nil {
		log.Printf("Error updating last alerted count: %v", err)
	}
}
