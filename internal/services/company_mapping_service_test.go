package services

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"layoff-tracker/internal/database"
)

func TestCompanyMappingService(t *testing.T) {
	// Create in-memory database for testing
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE company_mappings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			original_name TEXT UNIQUE NOT NULL,
			canonical_name TEXT NOT NULL,
			mapping_type TEXT DEFAULT 'auto',
			confidence_score INTEGER DEFAULT 100,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE companies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			employee_count INTEGER,
			industry TEXT,
			canonical_name TEXT,
			mapping_id INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create test tables: %v", err)
	}

	service := NewCompanyMappingService(&database.DB{DB: db})

	t.Run("ExactMapping", func(t *testing.T) {
		// Add a mapping
		err := service.createMapping("Test Corp", "Test Company", "manual", 100)
		if err != nil {
			t.Errorf("Failed to create mapping: %v", err)
		}

		// Test exact match
		canonical, err := service.NormalizeCompany("Test Corp")
		if err != nil {
			t.Errorf("NormalizeCompany failed: %v", err)
		}
		if canonical != "Test Company" {
			t.Errorf("Expected 'Test Company', got '%s'", canonical)
		}
	})

	t.Run("FuzzyMatching", func(t *testing.T) {
		// Add base mapping
		err := service.createMapping("Microsoft Corporation", "Microsoft", "manual", 100)
		if err != nil {
			t.Errorf("Failed to create mapping: %v", err)
		}

		// Test fuzzy match
		canonical, err := service.NormalizeCompany("Microsoft Corp")
		if err != nil {
			t.Errorf("NormalizeCompany failed: %v", err)
		}
		if canonical != "Microsoft" {
			t.Errorf("Expected 'Microsoft', got '%s'", canonical)
		}
	})

	t.Run("SimilarityScoring", func(t *testing.T) {
		tests := []struct {
			name1    string
			name2    string
			minScore int
		}{
			{"Apple Inc", "Apple", 80},
			{"Google LLC", "Google", 80},
			{"Microsoft Corporation", "Microsoft", 80},
			{"IBM Corp", "IBM", 80},
			{"Tesla Motors", "Tesla", 70},
			{"Amazon.com Inc", "Amazon", 60},
		}

		for _, tt := range tests {
			t.Run(tt.name1+"_vs_"+tt.name2, func(t *testing.T) {
				score := service.calculateSimilarityScore(tt.name1, tt.name2)
				if score < tt.minScore {
					t.Errorf("Similarity score too low: %s vs %s = %d (expected >= %d)",
						tt.name1, tt.name2, score, tt.minScore)
				}
			})
		}
	})

	t.Run("CompanyNameCleaning", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"Apple Inc.", "apple inc"},
			{"Google LLC", "google llc"},
			{"Microsoft Corporation", "microsoft corporation"},
			{"IBM Corp.", "ibm corp"},
			{"Tesla, Inc.", "tesla inc"},
		}

		for _, tt := range tests {
			t.Run(tt.input, func(t *testing.T) {
				result := service.cleanCompanyName(tt.input)
				if result != tt.expected {
					t.Errorf("cleanCompanyName(%s) = %s, expected %s",
						tt.input, result, tt.expected)
				}
			})
		}
	})

	t.Run("LevenshteinDistance", func(t *testing.T) {
		tests := []struct {
			s1       string
			s2       string
			expected int
		}{
			{"abc", "abc", 0},
			{"abc", "abd", 1},
			{"abc", "xyz", 3},
			{"", "a", 1},
			{"apple", "apply", 1},
		}

		for _, tt := range tests {
			t.Run(tt.s1+"_vs_"+tt.s2, func(t *testing.T) {
				result := service.levenshteinDistance(tt.s1, tt.s2)
				if result != tt.expected {
					t.Errorf("levenshteinDistance(%s, %s) = %d, expected %d",
						tt.s1, tt.s2, result, tt.expected)
				}
			})
		}
	})

	t.Run("AggressiveFuzzyMatching", func(t *testing.T) {
		// Test cases that should match aggressively
		aggressiveTests := []struct {
			input    string
			expected string
			reason   string
		}{
			{"Apple Computer Inc", "Apple", "contains core name"},
			{"Microsoft Corp", "Microsoft", "common abbreviation"},
			{"Google LLC", "Google", "standard suffix"},
			{"Amazon.com Inc", "Amazon", "domain in name"},
			{"Facebook Inc", "Meta", "brand evolution"},
			{"Tesla Motors Inc", "Tesla", "product line name"},
		}

		// First add some base mappings
		baseMappings := []struct{ original, canonical string }{
			{"Apple Inc", "Apple"},
			{"Microsoft Corporation", "Microsoft"},
			{"Google LLC", "Google"},
			{"Amazon.com", "Amazon"},
			{"Facebook Inc", "Meta"},
			{"Tesla Inc", "Tesla"},
		}

		for _, mapping := range baseMappings {
			err := service.createMapping(mapping.original, mapping.canonical, "manual", 100)
			if err != nil {
				t.Errorf("Failed to create base mapping %s -> %s: %v", mapping.original, mapping.canonical, err)
			}
		}

		// Test aggressive matching
		for _, tt := range aggressiveTests {
			t.Run(tt.input, func(t *testing.T) {
				canonical, err := service.NormalizeCompany(tt.input)
				if err != nil {
					t.Errorf("NormalizeCompany failed for %s: %v", tt.input, err)
					return
				}

				if canonical != tt.expected {
					t.Errorf("Aggressive matching failed for %s: got %s, expected %s (%s)",
						tt.input, canonical, tt.expected, tt.reason)
				}
			})
		}
	})
}
