package database

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

func NewDB(dataSourceName string) (*DB, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	// Enable foreign key constraints
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("error enabling foreign keys: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err = db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("error enabling WAL mode: %w", err)
	}

	return &DB{db}, nil
}

func (db *DB) RunMigrations(migrationsDir string) error {
	// Create migrations table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			filename TEXT UNIQUE NOT NULL,
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("error creating migrations table: %w", err)
	}

	// Get list of migration files
	files, err := ioutil.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("error reading migrations directory: %w", err)
	}

	// Get executed migrations
	var executedMigrations map[string]bool
	executedMigrations = make(map[string]bool)

	rows, err := db.Query("SELECT filename FROM migrations")
	if err != nil {
		return fmt.Errorf("error querying executed migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return fmt.Errorf("error scanning migration filename: %w", err)
		}
		executedMigrations[filename] = true
	}

	// Run pending migrations
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()
		if executedMigrations[filename] {
			continue // Already executed
		}

		log.Printf("Running migration: %s", filename)

		migrationPath := filepath.Join(migrationsDir, filename)
		content, err := ioutil.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("error reading migration file %s: %w", filename, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("error beginning transaction for migration %s: %w", filename, err)
		}

		// Execute migration
		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("error executing migration %s: %w", filename, err)
		}

		// Record migration
		if _, err := tx.Exec("INSERT INTO migrations (filename) VALUES (?)", filename); err != nil {
			tx.Rollback()
			return fmt.Errorf("error recording migration %s: %w", filename, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("error committing migration %s: %w", filename, err)
		}

		log.Printf("Migration completed: %s", filename)
	}

	return nil
}
