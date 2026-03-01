package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"file-server/internal/aws"
	"file-server/internal/config"
	"file-server/internal/database"
	"file-server/internal/middleware"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	uuidv7 "github.com/samborkent/uuidv7"

	"path/filepath"

	v2aws "github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ProcessedPart tracks which parts have been uploaded to prevent reprocessing
type ProcessedPart struct {
	UploadID   string
	PartNumber int32
}

var (
	// ETag validation regex (S3 ETags are quoted hex strings)
	etagRegex = regexp.MustCompile(`^"[a-f0-9]{32}(-\d+)?"$`)

	// Timeout for presign requests
	presignTimeout = 30 * time.Second

	// Metadata cleanup interval
	cleanupInterval = 5 * time.Minute
	metadataMaxAge  = 30 * time.Minute
)

func StartServer(cfg *config.Config) {
	s3Client := aws.NewS3(&cfg.AWS)
	maxUploadBytes := int64(cfg.Server.MaxFileSizeMB) * 1024 * 1024 // Convert MB to bytes

	// Initialize database connection
	ctx := context.Background()
	db, err := database.NewDatabase(ctx, cfg.Database.URL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Start cleanup goroutine for expired metadata and processed parts
	go cleanupExpiredData(db)

	// CORS is handled by internal/middleware.ApplyCORS

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		reqID := generateRequestID()
		if middleware.ApplyCORS(w, r) {
			return
		}
		log.Printf("[%s] GET /health", reqID)
		fmt.Fprintln(w, "OK")
	})

	http.HandleFunc("/initiate-multipart", func(w http.ResponseWriter, r *http.Request) {
		reqID := generateRequestID()
		if middleware.ApplyCORS(w, r) {
			return
		}
		var req struct {
			Key  string `json:"key"`  // original filename or path
			Size int64  `json:"size"` // file size in bytes
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
			log.Printf("[%s] Invalid request body: %v", reqID, err)
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.Size <= 0 {
			log.Printf("[%s] Invalid file size: %d", reqID, req.Size)
			http.Error(w, "Invalid file size: must be greater than 0", http.StatusBadRequest)
			return
		}
		if req.Size > maxUploadBytes {
			maxFileSizeGB := cfg.Server.MaxFileSizeMB / 1024
			log.Printf("[%s] File too large: %d bytes (max %d)", reqID, req.Size, maxUploadBytes)
			http.Error(w, fmt.Sprintf("File exceeds maximum allowed size (%dGB)", maxFileSizeGB), http.StatusBadRequest)
			return
		}
		// generate uuidv7 per file and place under configured prefix/<uuidv7>/<basename>
		u := uuidv7.New()
		uidStr := u.String()
		filename := filepath.Base(req.Key)
		// Use forward slashes for S3 keys (not filepath.Join which uses backslashes on Windows)
		targetKey := cfg.AWS.S3Prefix + uidStr + "/" + filename
		info, err := aws.InitiateMultipartUpload(s3Client, cfg.AWS.Bucket, targetKey)
		if err != nil {
			log.Printf("[%s] S3 initiate failed for key %s: %v", reqID, targetKey, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// also return a publicly addressable URL for convenience
		fileURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.AWS.Bucket, cfg.AWS.Region, targetKey)

		// Get client IP address
		ip := getClientIP(r)

		// Check total upload size limit (10GB = 10000 MB)
		totalSizeMB, err := db.GetTotalUploadSize(ctx)
		if err != nil {
			log.Printf("[%s] Database error checking total size: %v", reqID, err)
			http.Error(w, fmt.Sprintf("Database error checking total upload size: %v", err), http.StatusInternalServerError)
			return
		}

		fileSizeMB := float64(req.Size) / (1024 * 1024)
		maxTotalSizeMB := float64(cfg.Server.MaxTotalUploadMB)

		if totalSizeMB+fileSizeMB > maxTotalSizeMB {
			maxTotalSizeGB := cfg.Server.MaxTotalUploadMB / 1024
			log.Printf("[%s] Total size limit exceeded. Current: %.2fMB, Requested: %.2fMB, Limit: %.2fMB", reqID, totalSizeMB, fileSizeMB, maxTotalSizeMB)
			http.Error(w, fmt.Sprintf("Total upload size limit exceeded (%dGB). Current total: %.2fMB, Requested: %.2fMB", maxTotalSizeGB, totalSizeMB, fileSizeMB), http.StatusBadRequest)
			return
		}

		// Store upload metadata in database
		uploadMetadata := &database.UploadMetadata{
			UploadID:   info.UploadID,
			FileSizeMB: fileSizeMB,
			UploaderIP: ip,
			CreatedAt:  time.Now(),
			S3Key:      targetKey,
			Filename:   filename,
		}

		if err := db.CreateUploadMetadata(ctx, uploadMetadata); err != nil {
			log.Printf("[%s] Database error storing upload metadata: %v", reqID, err)
			http.Error(w, fmt.Sprintf("Database error storing upload metadata: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("[%s] Upload initiated for %s (%.2fMB) from %s", reqID, filename, fileSizeMB, ip)

		resp := map[string]interface{}{
			"uploadId":   info.UploadID,
			"key":        info.Key,
			"url":        fileURL,
			"fileSizeMB": fileSizeMB,
			"uploaderIP": ip,
			"filename":   filename,
		}
		json.NewEncoder(w).Encode(resp)
	})

	http.HandleFunc("/presign-part", func(w http.ResponseWriter, r *http.Request) {
		reqID := generateRequestID()
		if middleware.ApplyCORS(w, r) {
			return
		}
		var req struct {
			Key        string `json:"key"`
			UploadID   string `json:"uploadId"`
			PartNumber int32  `json:"partNumber"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" || req.UploadID == "" || req.PartNumber == 0 {
			log.Printf("[%s] Invalid presign request: %v", reqID, err)
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Check for duplicate part upload (race condition prevention)
		partExists, err := db.IsPartProcessed(ctx, req.UploadID, req.PartNumber)
		if err != nil {
			log.Printf("[%s] Database error checking processed part: %v", reqID, err)
			http.Error(w, "Database error checking part status", http.StatusInternalServerError)
			return
		}

		if partExists {
			log.Printf("[%s] Duplicate part request detected: %s part %d", reqID, req.UploadID, req.PartNumber)
			// Return 409 Conflict to indicate duplicate, client should retry
			http.Error(w, "Part already being processed", http.StatusConflict)
			return
		}

		// Mark part as being processed
		if err := db.MarkPartAsProcessed(ctx, req.UploadID, req.PartNumber); err != nil {
			log.Printf("[%s] Database error marking part as processed: %v", reqID, err)
			http.Error(w, "Database error marking part as processed", http.StatusInternalServerError)
			return
		}

		// Create a context with timeout for S3 presign operation
		presignCtx, cancel := context.WithTimeout(ctx, presignTimeout)
		defer cancel()

		// Check total upload size limit (10GB = 10000 MB)
		totalSizeMB, err := db.GetTotalUploadSize(presignCtx)
		if err != nil {
			log.Printf("[%s] Database error checking total size: %v", reqID, err)
			http.Error(w, fmt.Sprintf("Database error checking total upload size: %v", err), http.StatusInternalServerError)
			return
		}

		maxTotalSizeMB := float64(cfg.Server.MaxTotalUploadMB)

		if totalSizeMB > maxTotalSizeMB {
			maxTotalSizeGB := cfg.Server.MaxTotalUploadMB / 1024
			log.Printf("[%s] Total size limit exceeded during presign: %.2fMB", reqID, totalSizeMB)
			http.Error(w, fmt.Sprintf("Total upload size limit exceeded (%dGB). Current total: %.2fMB", maxTotalSizeGB, totalSizeMB), http.StatusBadRequest)
			return
		}

		url, err := aws.GetPresignedURLForPart(s3Client, cfg.AWS.Bucket, req.Key, req.UploadID, req.PartNumber)
		if err != nil {
			log.Printf("[%s] S3 presign failed for part %d: %v", reqID, req.PartNumber, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("[%s] Presigned URL generated for part %d", reqID, req.PartNumber)
		json.NewEncoder(w).Encode(map[string]string{"url": url})
	})

	http.HandleFunc("/complete-multipart", func(w http.ResponseWriter, r *http.Request) {
		reqID := generateRequestID()
		if middleware.ApplyCORS(w, r) {
			return
		}
		var req struct {
			Key      string `json:"key"`
			UploadID string `json:"uploadId"`
			Parts    []struct {
				ETag       string `json:"etag"`
				PartNumber int64  `json:"partNumber"`
			} `json:"parts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" || req.UploadID == "" || len(req.Parts) == 0 {
			log.Printf("[%s] Invalid complete-multipart request: %v", reqID, err)
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Validate ETags format
		var completedParts []s3types.CompletedPart
		for _, p := range req.Parts {
			// Validate ETag format (should be a quoted hex string)
			if !etagRegex.MatchString(strings.ToLower(p.ETag)) {
				log.Printf("[%s] Invalid ETag format for part %d: %s", reqID, p.PartNumber, p.ETag)
				http.Error(w, fmt.Sprintf("Invalid ETag format for part %d", p.PartNumber), http.StatusBadRequest)
				return
			}
			completedParts = append(completedParts, s3types.CompletedPart{
				ETag:       &p.ETag,
				PartNumber: v2aws.Int32(int32(p.PartNumber)),
			})
		}

		log.Printf("[%s] Completing multipart upload with %d parts", reqID, len(completedParts))
		err := aws.CompleteMultipartUpload(s3Client, cfg.AWS.Bucket, req.Key, req.UploadID, completedParts)
		if err != nil {
			log.Printf("[%s] S3 complete multipart failed: %v", reqID, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Generate a presigned URL for download with configurable expiry
		downloadURL, err := aws.GetPresignedDownloadURL(s3Client, cfg.AWS.Bucket, req.Key, time.Duration(cfg.Server.DownloadURLExpiryDays)*24*time.Hour)
		if err != nil {
			log.Printf("[%s] Failed to generate download URL: %v", reqID, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		publicURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.AWS.Bucket, cfg.AWS.Region, req.Key)

		// Extract filename from key (format: prefix/uuid/filename)
		parts := strings.Split(req.Key, "/")
		filename := parts[len(parts)-1]

		// Retrieve stored metadata from database
		metadata, err := db.GetUploadMetadata(ctx, req.UploadID)

		// Default values if metadata not found (shouldn't happen in normal flow)
		sizeMB := float64(0)
		uploaderIP := ""
		if err != nil {
			log.Printf("[%s] Warning: Unable to retrieve upload metadata: %v", reqID, err)
		} else {
			sizeMB = metadata.FileSizeMB
			uploaderIP = metadata.UploaderIP
		}

		// Clean up upload metadata from database
		if err := db.DeleteUploadMetadata(ctx, req.UploadID); err != nil {
			log.Printf("[%s] Warning: Failed to delete upload metadata: %v", reqID, err)
		}

		// Clean up processed parts from database
		if err := db.DeleteProcessedParts(ctx, req.UploadID); err != nil {
			log.Printf("[%s] Warning: Failed to delete processed parts: %v", reqID, err)
		}

		// Create/insert upload record with completion info
		completedTime := time.Now()
		uploadRecord := &database.UploadRecord{
			UploadID:    req.UploadID,
			S3Key:       req.Key,
			Filename:    filename,
			SizeMB:      sizeMB,
			UploaderIP:  uploaderIP,
			PublicURL:   publicURL,
			DownloadURL: downloadURL,
			CompletedAt: &completedTime,
		}
		if err := db.CreateUploadRecord(ctx, uploadRecord); err != nil {
			log.Printf("[%s] Database error inserting upload: %v", reqID, err)
			http.Error(w, fmt.Sprintf("Database error inserting upload completion: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("[%s] Upload completed successfully for %s (%.2fMB)", reqID, filename, sizeMB)

		resp := map[string]interface{}{
			"status":      "completed",
			"downloadUrl": downloadURL,
			"publicUrl":   publicURL,
		}
		json.NewEncoder(w).Encode(resp)
	})

	fmt.Println("Server running on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header (common with proxies)
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			// Remove brackets from IPv6 addresses
			if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
				ip = ip[1 : len(ip)-1]
			}
			return ip
		}
	}

	// Check for X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		// Remove brackets from IPv6 addresses
		if strings.HasPrefix(realIP, "[") && strings.HasSuffix(realIP, "]") {
			realIP = realIP[1 : len(realIP)-1]
		}
		return realIP
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}
	// Remove brackets from IPv6 addresses
	if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
		ip = ip[1 : len(ip)-1]
	}
	return ip
}

// generateRequestID creates a unique request ID for logging and tracing
func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// cleanupExpiredData removes expired metadata and processed parts from database
func cleanupExpiredData(db *database.Database) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()

		// Cleanup expired metadata (uploads that never completed)
		deletedMetadata, err := db.CleanupExpiredMetadata(ctx, metadataMaxAge)
		if err != nil {
			log.Printf("Error cleaning up expired metadata: %v", err)
		} else if deletedMetadata > 0 {
			log.Printf("Cleaned up %d expired upload metadata record(s)", deletedMetadata)
		}

		// Cleanup expired processed parts (should be cleaned on completion, but just in case)
		deletedParts, err := db.CleanupExpiredProcessedParts(ctx, metadataMaxAge)
		if err != nil {
			log.Printf("Error cleaning up expired processed parts: %v", err)
		} else if deletedParts > 0 {
			log.Printf("Cleaned up %d stale processed part(s)", deletedParts)
		}
	}
}
