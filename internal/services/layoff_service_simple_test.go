package services

import (
	"testing"

	"layoff-tracker/internal/database"
	"layoff-tracker/internal/models"
)

func TestGetLayoffs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewLayoffService(&database.DB{DB: db})

	// Test empty result
	params := models.FilterParams{}
	result, err := service.GetLayoffs(params)
	if err != nil {
		t.Fatalf("GetLayoffs failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Expected 0 total layoffs, got %d", result.Total)
	}

	layoffs, ok := result.Data.([]*models.Layoff)
	if !ok {
		t.Fatalf("Expected data to be []*models.Layoff")
	}

	if len(layoffs) != 0 {
		t.Errorf("Expected 0 items, got %d", len(layoffs))
	}
}
