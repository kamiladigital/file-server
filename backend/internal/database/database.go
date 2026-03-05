package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
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
	FileboxName string
}

type UploadMetadata struct {
	UploadID    string
	FileSizeMB  float64
	UploaderIP  string
	CreatedAt   time.Time
	S3Key       string
	Filename    string
	FileboxName string
}

type Database struct {
	pool *pgxpool.Pool
}

func NewDatabase(ctx context.Context, connString string) (*Database, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Disable prepared statement caching to avoid "cached plan must not change result type" errors
	// when schema changes (e.g., uploader_ip column type changed from INET to TEXT)
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

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
			public_url, download_url, completed_at, filebox_name
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (upload_id) DO UPDATE SET
			s3_key = EXCLUDED.s3_key,
			filename = EXCLUDED.filename,
			size_mb = EXCLUDED.size_mb,
			uploader_ip = EXCLUDED.uploader_ip,
			public_url = EXCLUDED.public_url,
			download_url = EXCLUDED.download_url,
			completed_at = EXCLUDED.completed_at,
			filebox_name = EXCLUDED.filebox_name
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
		record.FileboxName,
	)
	return err
}

func (db *Database) UpdateUploadCompletion(ctx context.Context, uploadID, publicURL, downloadURL string) error {
	query := `
		UPDATE uploads 
		SET public_url = $1, download_url = $2, completed_at = CURRENT_TIMESTAMP
		WHERE upload_id = $3
	`

	tag, err := db.pool.Exec(ctx, query, publicURL, downloadURL, uploadID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no upload found for upload_id %q", uploadID)
	}
	return nil
}

func (db *Database) GetUploadByID(ctx context.Context, uploadID string) (*UploadRecord, error) {
	query := `
		SELECT 
			id, upload_id, s3_key, filename, size_mb, uploader_ip,
			created_at, public_url, download_url, completed_at, filebox_name
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
		&record.FileboxName,
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
			created_at, public_url, download_url, completed_at, filebox_name
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
			&record.FileboxName,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func (db *Database) GetRecentUploads(ctx context.Context, limit int) ([]UploadRecord, error) {
	query := `
		SELECT 
			id, upload_id, s3_key, filename, size_mb, uploader_ip,
			created_at, public_url, download_url, completed_at, filebox_name
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
			&record.FileboxName,
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

// ========== Filebox Operations ==========

// GetUploadsByFilebox retrieves all completed uploads for a specific filebox
func (db *Database) GetUploadsByFilebox(ctx context.Context, fileboxName string, limit int) ([]UploadRecord, error) {
	query := `
		SELECT 
			id, upload_id, s3_key, filename, size_mb, uploader_ip,
			created_at, public_url, download_url, completed_at, filebox_name
		FROM uploads 
		WHERE filebox_name = $1 AND completed_at IS NOT NULL
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := db.pool.Query(ctx, query, fileboxName, limit)
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
			&record.FileboxName,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

// GetTotalUploadSizeByFilebox returns the total size in MB of uploads in a specific filebox
func (db *Database) GetTotalUploadSizeByFilebox(ctx context.Context, fileboxName string) (float64, error) {
	query := `
		SELECT COALESCE(SUM(size_mb), 0) FROM uploads 
		WHERE filebox_name = $1 AND completed_at IS NOT NULL
	`

	var totalSize float64
	err := db.pool.QueryRow(ctx, query, fileboxName).Scan(&totalSize)
	if err != nil {
		return 0, err
	}

	return totalSize, nil
}

// ========== Upload Metadata Management ==========

// CreateUploadMetadata stores metadata for an upload in progress
func (db *Database) CreateUploadMetadata(ctx context.Context, metadata *UploadMetadata) error {
	query := `
		INSERT INTO upload_metadata (upload_id, file_size_mb, uploader_ip, s3_key, filename, created_at, filebox_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := db.pool.Exec(ctx, query,
		metadata.UploadID,
		metadata.FileSizeMB,
		metadata.UploaderIP,
		metadata.S3Key,
		metadata.Filename,
		metadata.CreatedAt,
		metadata.FileboxName,
	)
	return err
}

// GetUploadMetadata retrieves metadata for an upload in progress
func (db *Database) GetUploadMetadata(ctx context.Context, uploadID string) (*UploadMetadata, error) {
	query := `
		SELECT upload_id, file_size_mb, uploader_ip, created_at, s3_key, filename, filebox_name
		FROM upload_metadata
		WHERE upload_id = $1
	`

	var metadata UploadMetadata
	err := db.pool.QueryRow(ctx, query, uploadID).Scan(
		&metadata.UploadID,
		&metadata.FileSizeMB,
		&metadata.UploaderIP,
		&metadata.CreatedAt,
		&metadata.S3Key,
		&metadata.Filename,
		&metadata.FileboxName,
	)
	if err != nil {
		return nil, err
	}

	return &metadata, nil
}

// DeleteUploadMetadata removes metadata for an upload (called after completion)
func (db *Database) DeleteUploadMetadata(ctx context.Context, uploadID string) error {
	query := `DELETE FROM upload_metadata WHERE upload_id = $1`

	_, err := db.pool.Exec(ctx, query, uploadID)
	return err
}

// CleanupExpiredMetadata removes metadata older than the specified duration
func (db *Database) CleanupExpiredMetadata(ctx context.Context, maxAge time.Duration) (int64, error) {
	query := `
		DELETE FROM upload_metadata
		WHERE created_at < $1
	`

	cutoffTime := time.Now().Add(-maxAge)
	tag, err := db.pool.Exec(ctx, query, cutoffTime)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}

// IsUploadActive checks if an upload is currently in progress
func (db *Database) IsUploadActive(ctx context.Context, uploadID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM upload_metadata WHERE upload_id = $1)`

	var exists bool
	err := db.pool.QueryRow(ctx, query, uploadID).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// ========== Processed Parts Management ==========

// MarkPartAsProcessed records that a part has been uploaded
func (db *Database) MarkPartAsProcessed(ctx context.Context, uploadID string, partNumber int32) error {
	query := `
		INSERT INTO processed_parts (upload_id, part_number)
		VALUES ($1, $2)
		ON CONFLICT (upload_id, part_number) DO NOTHING
	`

	_, err := db.pool.Exec(ctx, query, uploadID, partNumber)
	return err
}

// IsPartProcessed checks if a part has already been uploaded
func (db *Database) IsPartProcessed(ctx context.Context, uploadID string, partNumber int32) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM processed_parts 
			WHERE upload_id = $1 AND part_number = $2
		)
	`

	var exists bool
	err := db.pool.QueryRow(ctx, query, uploadID, partNumber).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// DeleteProcessedParts removes all processed parts for an upload (called after completion)
func (db *Database) DeleteProcessedParts(ctx context.Context, uploadID string) error {
	query := `DELETE FROM processed_parts WHERE upload_id = $1`

	_, err := db.pool.Exec(ctx, query, uploadID)
	return err
}

// CleanupExpiredProcessedParts removes processed parts older than the specified duration
func (db *Database) CleanupExpiredProcessedParts(ctx context.Context, maxAge time.Duration) (int64, error) {
	query := `
		DELETE FROM processed_parts
		WHERE processed_at < $1
	`

	cutoffTime := time.Now().Add(-maxAge)
	tag, err := db.pool.Exec(ctx, query, cutoffTime)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}
