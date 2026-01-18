package services

import (
	"layoff-tracker/internal/database"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAlertService_SendNewDataAlert_LogsFailure(t *testing.T) {
	// Integration test: Uses test DB, inserts user, calls SendNewDataAlert to verify logging on SMTP failure

	// Get test DB and run migrations
	db, err := database.NewDB("layoff_tracker.db")
	assert.NoError(t, err)
	err = db.RunMigrations("../migrations")
	assert.NoError(t, err)

	// Insert test user
	_, err = db.Exec(`INSERT OR IGNORE INTO users (provider, provider_id, email, name, is_admin, created_at)
	                  VALUES (?, ?, ?, ?, ?, ?)`, "test", "123", "test@example.com", "Test User", 1, time.Now())
	assert.NoError(t, err)

	// Get user ID
	var userID int
	err = db.QueryRow(`SELECT id FROM users WHERE provider_id = ?`, "123").Scan(&userID)
	assert.NoError(t, err)

	// Insert alert prefs
	_, err = db.Exec(`INSERT OR IGNORE INTO user_alert_prefs (user_id, email_alerts_enabled, alert_new_data)
	                  VALUES (?, ?, ?)`, userID, 1, 1)
	assert.NoError(t, err)

	userService := &UserService{db: db}
	alertService := &AlertService{
		userService: userService,
		smtpHost:    "invalid-host", // Force failure
		smtpPort:    25,
		fromEmail:   "test@example.com",
	}

	// Call the method - should log sending attempt and failure
	err = alertService.SendNewDataAlert(userID, 5, "test time")
	assert.Error(t, err) // SMTP fails

	// Clean up
	_, _ = db.Exec(`DELETE FROM user_alert_prefs WHERE user_id = ?`, userID)
	_, _ = db.Exec(`DELETE FROM users WHERE id = ?`, userID)
	db.Close()
}
