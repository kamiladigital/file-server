package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Get database URL from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Connected to database successfully")

	// Create migrations tracking table
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatalf("Failed to create schema_migrations table: %v", err)
	}

	// Get list of applied migrations
	appliedMigrations := make(map[string]bool)
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		log.Fatalf("Failed to query applied migrations: %v", err)
	}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			log.Fatalf("Failed to scan migration version: %v", err)
		}
		appliedMigrations[version] = true
	}
	rows.Close()

	// Get list of migration files
	migrationsDir := "migrations"
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		log.Fatalf("Failed to read migrations directory: %v", err)
	}

	// Sort files by name to ensure order
	var migrationFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") {
			migrationFiles = append(migrationFiles, file.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Apply pending migrations
	for _, filename := range migrationFiles {
		version := strings.TrimSuffix(filename, ".sql")

		if appliedMigrations[version] {
			log.Printf("Migration %s already applied, skipping", version)
			continue
		}

		log.Printf("Applying migration %s...", version)

		// Read migration file
		content, err := os.ReadFile(filepath.Join(migrationsDir, filename))
		if err != nil {
			log.Fatalf("Failed to read migration file %s: %v", filename, err)
		}

		// Execute migration
		_, err = pool.Exec(ctx, string(content))
		if err != nil {
			log.Fatalf("Failed to execute migration %s: %v", version, err)
		}

		// Record migration as applied
		_, err = pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
		if err != nil {
			log.Fatalf("Failed to record migration %s: %v", version, err)
		}

		log.Printf("Migration %s applied successfully", version)
	}

	log.Println("All migrations applied successfully!")
}
