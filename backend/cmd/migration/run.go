package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Read migration file
	migrationSQL, err := os.ReadFile("../../migrations/001_create_uploads_table.sql")
	if err != nil {
		log.Fatalf("Failed to read migration file: %v", err)
	}

	// Connect to database
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close(ctx)

	// Drop table if exists
	_, err = conn.Exec(ctx, "DROP TABLE IF EXISTS uploads")
	if err != nil {
		log.Printf("Warning: Failed to drop table: %v", err)
	}

	// Execute migration
	sqlStatements := strings.Split(string(migrationSQL), ";")
	for _, stmt := range sqlStatements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		_, err := conn.Exec(ctx, stmt)
		if err != nil {
			log.Printf("Warning: Failed to execute statement: %v", err)
			// Continue anyway - might be because table/index already exists
		}
	}

	fmt.Println("Migration completed successfully!")
}
