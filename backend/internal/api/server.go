package api

import (
	"context"
	"encoding/json"
	"file-server/internal/aws"
	"file-server/internal/config"
	"file-server/internal/database"
	"file-server/internal/middleware"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	uuidv7 "github.com/samborkent/uuidv7"

	"path/filepath"

	v2aws "github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func StartServer(cfg *config.Config) {
	s3Client := aws.NewS3(&cfg.AWS)
	const maxUploadBytes = 1 * 1024 * 1024 * 1024 // 1GB

	// Initialize database connection
	ctx := context.Background()
	db, err := database.NewDatabase(ctx, cfg.Database.URL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// CORS is handled by internal/middleware.ApplyCORS

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if middleware.ApplyCORS(w, r) {
			return
		}
		fmt.Fprintln(w, "OK")
	})

	http.HandleFunc("/initiate-multipart", func(w http.ResponseWriter, r *http.Request) {
		if middleware.ApplyCORS(w, r) {
			return
		}
		var req struct {
			Key  string `json:"key"`  // original filename or path
			Size int64  `json:"size"` // file size in bytes
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.Size > maxUploadBytes {
			http.Error(w, "File exceeds maximum allowed size (1GB)", http.StatusBadRequest)
			return
		}
		// generate uuidv7 per file and place under configured prefix/<uuidv7>/<basename>
		u := uuidv7.New()
		uidStr := u.String()
		filename := filepath.Base(req.Key)
		targetKey := filepath.Join(cfg.AWS.S3Prefix, uidStr, filename)
		info, err := aws.InitiateMultipartUpload(s3Client, cfg.AWS.Bucket, targetKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// also return a publicly addressable URL for convenience
		fileURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.AWS.Bucket, cfg.AWS.Region, targetKey)

		// Get client IP address
		ip := getClientIP(r)

		// Log to database
		record := &database.UploadRecord{
			UploadID:    info.UploadID,
			S3Key:       info.Key,
			Filename:    filename,
			SizeMB:      float64(req.Size) / (1024 * 1024), // Convert bytes to MB
			UploaderIP:  ip,
			PublicURL:   fileURL,
			DownloadURL: "",
			CompletedAt: nil,
		}

		if err := db.CreateUploadRecord(ctx, record); err != nil {
			fmt.Printf("Warning: Failed to log upload to database: %v\n", err)
			// Continue anyway - don't fail the upload if logging fails
		}

		resp := map[string]interface{}{"uploadId": info.UploadID, "key": info.Key, "url": fileURL}
		json.NewEncoder(w).Encode(resp)
	})

	http.HandleFunc("/presign-part", func(w http.ResponseWriter, r *http.Request) {
		if middleware.ApplyCORS(w, r) {
			return
		}
		var req struct {
			Key        string `json:"key"`
			UploadID   string `json:"uploadId"`
			PartNumber int32  `json:"partNumber"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" || req.UploadID == "" || req.PartNumber == 0 {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		url, err := aws.GetPresignedURLForPart(s3Client, cfg.AWS.Bucket, req.Key, req.UploadID, req.PartNumber)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"url": url})
	})

	http.HandleFunc("/complete-multipart", func(w http.ResponseWriter, r *http.Request) {
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
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		var completedParts []s3types.CompletedPart
		for _, p := range req.Parts {
			completedParts = append(completedParts, s3types.CompletedPart{
				ETag:       &p.ETag,
				PartNumber: v2aws.Int32(int32(p.PartNumber)),
			})
		}
		err := aws.CompleteMultipartUpload(s3Client, cfg.AWS.Bucket, req.Key, req.UploadID, completedParts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Generate a presigned URL for download with configurable expiry
		downloadURL, err := aws.GetPresignedDownloadURL(s3Client, cfg.AWS.Bucket, req.Key, time.Duration(cfg.Server.DownloadURLExpiryDays)*24*time.Hour)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		publicURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.AWS.Bucket, cfg.AWS.Region, req.Key)

		// Update database with completion info
		if err := db.UpdateUploadCompletion(ctx, req.UploadID, publicURL, downloadURL); err != nil {
			fmt.Printf("Warning: Failed to update upload completion in database: %v\n", err)
			// Continue anyway - don't fail the completion if logging fails
		}

		resp := map[string]interface{}{
			"status":      "completed",
			"downloadUrl": downloadURL,
			"publicUrl":   publicURL,
		}
		json.NewEncoder(w).Encode(resp)
	})

	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", nil)
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header (common with proxies)
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check for X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}
	return ip
}
