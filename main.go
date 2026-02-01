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
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/robfig/cron/v3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/time/rate"
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

	// Log current working directory
	if wd, err := os.Getwd(); err == nil {
		log.Printf("Current working directory: %s", wd)
	} else {
		log.Printf("Error getting working directory: %v", err)
	}

	// Initialize database
	databasePath := strings.TrimSpace(os.Getenv("DATABASE_PATH"))
	if databasePath == "" {
		databasePath = "/tmp/layoff_tracker.db"
	}
	if strings.EqualFold(os.Getenv("GO_ENV"), "production") && strings.HasPrefix(databasePath, "/tmp/") {
		log.Fatalf("Refusing to use /tmp database path in production: %s", databasePath)
	}
	log.Printf("Using database path: %s", databasePath)
	db, err := database.NewDB(databasePath)
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
	authMailer := services.NewAuthMailerFromEnv()

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
		"timeAgo": func(value time.Time) string {
			if value.IsZero() {
				return ""
			}
			duration := time.Since(value)
			if duration < time.Minute {
				return "now"
			}
			if duration < time.Hour {
				return fmt.Sprintf("%dm", int(duration.Minutes()))
			}
			if duration < 24*time.Hour {
				return fmt.Sprintf("%dh", int(duration.Hours()))
			}
			if value.Year() == time.Now().Year() {
				return value.Format("Jan 2")
			}
			return value.Format("Jan 2, 2006")
		},
	}).ParseFiles(
		"templates/dashboard.html",
		"templates/tracker.html",
		"templates/layoff_detail.html",
		"templates/new_layoff.html",
		"templates/industries.html",
		"templates/faq.html",
		"templates/privacy.html",
		"templates/terms.html",
		"templates/contact.html",
		"templates/layout.html",
		"templates/profile.html",
		"templates/admin.html",
		"templates/login.html",
		"templates/register.html",
		"templates/verify_email.html",
		"templates/forgot_password.html",
		"templates/reset_password.html",
	))

	// Initialize free data service
	freeDataService := services.NewFreeDataService(db, layoffService)

	// Initialize handlers
	handler := handlers.NewHandler(layoffService, userService, freeDataService, authMailer, templates)

	// Initialize Echo
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		log.Printf("SESSION_SECRET not set, using insecure default")
		sessionSecret = "change-me-in-production"
	}
	store := sessions.NewCookieStore([]byte(sessionSecret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	e.Use(session.Middleware(store))
	// Static files
	e.Static("/static", "static")

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
	e.POST("/comments/:id/like", handler.LikeComment)
	e.POST("/comments/:id/flag", handler.FlagComment)
	e.GET("/layoffs/new", handler.NewLayoff)
	e.GET("/layoffs/:id", handler.LayoffDetail)
	e.POST("/layoffs", handler.CreateLayoff)
	e.GET("/industries", handler.Industries)
	e.GET("/faq", handler.FAQ)
	e.GET("/privacy", handler.Privacy)
	e.GET("/terms", handler.Terms)
	e.GET("/contact", handler.Contact)
	e.GET("/export/csv", handler.ExportCSV)
	e.POST("/admin/classify-companies", handler.ClassifyCompanies)
	e.POST("/admin/reclassify-companies", handler.ReclassifyAllCompanies)
	e.POST("/admin/update-company-sizes", handler.UpdateCompanySizes)
	e.POST("/admin/clear-seed-data", handler.ClearSeedData)

	// Setup free data import routes
	services.SetupFreeDataRoutes(e, db, layoffService)

	// DEBUG: Auto-run import on startup for testing
	go func() {
		time.Sleep(2 * time.Second) // Wait for server to start
		log.Println("Auto-running WARN import for testing...")
		err := freeDataService.ImportFromWARNDatabase()
		if err != nil {
			log.Printf("Auto-import failed: %v", err)
		} else {
			log.Println("Auto-import completed successfully")

			// Update company sizes for companies without employee counts
			log.Println("Updating company sizes...")
			err = layoffService.UpdateCompanySizes()
			if err != nil {
				log.Printf("Company size update failed: %v", err)
				log.Println("Continuing without company sizes - tracker page will show '-' for company sizes")
			} else {
				log.Println("Company size update completed")
			}

			// Classify companies that don't have industry data
			log.Println("Running industry classification...")
			err = freeDataService.ClassifyExistingCompanies()
			if err != nil {
				log.Printf("Industry classification failed: %v", err)
				log.Println("Continuing without industry classification - dashboard will work with limited industry data")
			} else {
				log.Println("Industry classification completed")
			}
		}
	}()

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
	authRateLimiter := middleware.NewRateLimiterMemoryStore(rate.Limit(5))
	// Email/password auth
	e.GET("/auth/register", handler.RegisterForm)
	e.POST("/auth/register", handler.Register, middleware.RateLimiter(authRateLimiter))
	e.GET("/auth/login", handler.LoginForm)
	e.POST("/auth/login", handler.Login, middleware.RateLimiter(authRateLimiter))
	e.GET("/auth/verify", handler.VerifyEmail)
	e.POST("/auth/resend-verification", handler.ResendVerification, middleware.RateLimiter(authRateLimiter))
	e.GET("/auth/forgot", handler.ForgotPasswordForm)
	e.POST("/auth/forgot", handler.ForgotPassword, middleware.RateLimiter(authRateLimiter))
	e.GET("/auth/reset", handler.ResetPasswordForm)
	e.POST("/auth/reset", handler.ResetPassword, middleware.RateLimiter(authRateLimiter))

	// OAuth routes
	e.GET("/auth/google", func(c echo.Context) error {
		url := googleOAuthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline)
		return c.Redirect(http.StatusTemporaryRedirect, url)
	})

	// Dev debug
	e.GET("/debug/session", handler.DebugSession)
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
		userService.UpdateLastLogin(user.ID)

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
	e.POST("/admin/flags/resolve", handler.ResolveCommentFlag)
	e.POST("/admin/flags/delete", handler.DeleteFlaggedComment)
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

	log.Printf("Server starting on port %s - SDLC test update - build system test - production check", port)
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
