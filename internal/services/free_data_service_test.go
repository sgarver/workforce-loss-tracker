package services

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"layoff-tracker/internal/database"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Run migrations
	if err := db.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Clear seed data for clean test environment
	db.Exec("DELETE FROM layoffs")
	db.Exec("DELETE FROM companies")
	db.Exec("DELETE FROM sponsored_listings")

	return db.DB
}

func TestInferIndustryID(t *testing.T) {
	tests := []struct {
		companyName string
		expectedID  int
		shouldFind  bool
	}{
		{"slack", 1, true},    // SaaS
		{"coinbase", 2, true}, // FinTech
		{"unknown", 0, false},
		{"google", 1, true}, // Technology
	}

	for _, tt := range tests {
		t.Run(tt.companyName, func(t *testing.T) {
			result := InferIndustryID(tt.companyName)
			if tt.shouldFind {
				if !result.Valid || int(result.Int64) != tt.expectedID {
					t.Errorf("Expected industry ID %d for %s, got %v", tt.expectedID, tt.companyName, result)
				}
			} else {
				if result.Valid {
					t.Errorf("Expected no industry for %s, got %v", tt.companyName, result)
				}
			}
		})
	}
}

func TestEstimateCompanySize(t *testing.T) {
	tests := []struct {
		companyName string
		expected    int
	}{
		{"Apple", 147000},
		{"Unknown Startup", -1}, // Unknown companies return -1 (unknown indicator)
		{"Microsoft", 221000},
	}

	for _, tt := range tests {
		t.Run(tt.companyName, func(t *testing.T) {
			result := EstimateCompanySize(tt.companyName)
			if result != tt.expected {
				t.Errorf("Expected size %d for %s, got %d", tt.expected, tt.companyName, result)
			}
		})
	}
}
