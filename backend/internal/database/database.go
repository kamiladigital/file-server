package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UploadRecord struct {
	ID          string
	UploadID    string
	S3Key       string
	Filename    string
	SizeMB      float64
	UploaderIP  string
	CreatedAt   time.Time
	PublicURL   string
	DownloadURL string
	CompletedAt *time.Time
}

type Database struct {
	pool *pgxpool.Pool
}

func NewDatabase(ctx context.Context, connString string) (*Database, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connection established successfully")
	return &Database{pool: pool}, nil
}

func (db *Database) Close() {
	db.pool.Close()
}

func (db *Database) CreateUploadRecord(ctx context.Context, record *UploadRecord) error {
	query := `
		INSERT INTO uploads (
			upload_id, s3_key, filename, size_mb, uploader_ip, 
			public_url, download_url, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (upload_id) DO UPDATE SET
			s3_key = EXCLUDED.s3_key,
			filename = EXCLUDED.filename,
			size_mb = EXCLUDED.size_mb,
			uploader_ip = EXCLUDED.uploader_ip,
			public_url = EXCLUDED.public_url,
			download_url = EXCLUDED.download_url,
			completed_at = EXCLUDED.completed_at
	`

	_, err := db.pool.Exec(ctx, query,
		record.UploadID,
		record.S3Key,
		record.Filename,
		record.SizeMB,
		record.UploaderIP,
		record.PublicURL,
		record.DownloadURL,
		record.CompletedAt,
	)
	return err
}

func (db *Database) UpdateUploadCompletion(ctx context.Context, uploadID, publicURL, downloadURL string) error {
	query := `
		UPDATE uploads 
		SET public_url = $1, download_url = $2, completed_at = CURRENT_TIMESTAMP
		WHERE upload_id = $3
	`

	_, err := db.pool.Exec(ctx, query, publicURL, downloadURL, uploadID)
	return err
}

func (db *Database) GetUploadByID(ctx context.Context, uploadID string) (*UploadRecord, error) {
	query := `
		SELECT 
			id, upload_id, s3_key, filename, size_mb, uploader_ip,
			created_at, public_url, download_url, completed_at
		FROM uploads 
		WHERE upload_id = $1
	`

	var record UploadRecord
	err := db.pool.QueryRow(ctx, query, uploadID).Scan(
		&record.ID,
		&record.UploadID,
		&record.S3Key,
		&record.Filename,
		&record.SizeMB,
		&record.UploaderIP,
		&record.CreatedAt,
		&record.PublicURL,
		&record.DownloadURL,
		&record.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

func (db *Database) GetUploadsByIP(ctx context.Context, ip string, limit int) ([]UploadRecord, error) {
	query := `
		SELECT 
			id, upload_id, s3_key, filename, size_mb, uploader_ip,
			created_at, public_url, download_url, completed_at
		FROM uploads 
		WHERE uploader_ip = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := db.pool.Query(ctx, query, ip, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []UploadRecord
	for rows.Next() {
		var record UploadRecord
		err := rows.Scan(
			&record.ID,
			&record.UploadID,
			&record.S3Key,
			&record.Filename,
			&record.SizeMB,
			&record.UploaderIP,
			&record.CreatedAt,
			&record.PublicURL,
			&record.DownloadURL,
			&record.CompletedAt,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, nil
}

func (db *Database) GetRecentUploads(ctx context.Context, limit int) ([]UploadRecord, error) {
	query := `
		SELECT 
			id, upload_id, s3_key, filename, size_mb, uploader_ip,
			created_at, public_url, download_url, completed_at
		FROM uploads 
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := db.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []UploadRecord
	for rows.Next() {
		var record UploadRecord
		err := rows.Scan(
			&record.ID,
			&record.UploadID,
			&record.S3Key,
			&record.Filename,
			&record.SizeMB,
			&record.UploaderIP,
			&record.CreatedAt,
			&record.PublicURL,
			&record.DownloadURL,
			&record.CompletedAt,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, nil
}

// GetTotalUploadSize returns the total size in MB of all uploads in the database
func (db *Database) GetTotalUploadSize(ctx context.Context) (float64, error) {
	query := `SELECT COALESCE(SUM(size_mb), 0) FROM uploads`

	var totalSize float64
	err := db.pool.QueryRow(ctx, query).Scan(&totalSize)
	if err != nil {
		return 0, err
	}

	return totalSize, nil
}

// GetTotalUploadSizeByIP returns the total size in MB of uploads from a specific IP address
func (db *Database) GetTotalUploadSizeByIP(ctx context.Context, ip string) (float64, error) {
	query := `SELECT COALESCE(SUM(size_mb), 0) FROM uploads WHERE uploader_ip = $1`

	var totalSize float64
	err := db.pool.QueryRow(ctx, query, ip).Scan(&totalSize)
	if err != nil {
		return 0, err
	}

	return totalSize, nil
}
