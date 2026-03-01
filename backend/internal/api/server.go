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
	maxUploadBytes := int64(cfg.Server.MaxFileSizeMB) * 1024 * 1024 // Convert MB to bytes

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
		if req.Size <= 0 {
			http.Error(w, "Invalid file size: must be greater than 0", http.StatusBadRequest)
			return
		}
		if req.Size > maxUploadBytes {
			maxFileSizeGB := cfg.Server.MaxFileSizeMB / 1024
			http.Error(w, fmt.Sprintf("File exceeds maximum allowed size (%dGB)", maxFileSizeGB), http.StatusBadRequest)
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

		// Check total upload size limit (10GB = 10000 MB)
		totalSizeMB, err := db.GetTotalUploadSize(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("Database error checking total upload size: %v", err), http.StatusInternalServerError)
			return
		}

		fileSizeMB := float64(req.Size) / (1024 * 1024)
		maxTotalSizeMB := float64(cfg.Server.MaxTotalUploadMB)

		if totalSizeMB+fileSizeMB > maxTotalSizeMB {
			maxTotalSizeGB := cfg.Server.MaxTotalUploadMB / 1024
			http.Error(w, fmt.Sprintf("Total upload size limit exceeded (%dGB). Current total: %.2fMB, Requested: %.2fMB", maxTotalSizeGB, totalSizeMB, fileSizeMB), http.StatusBadRequest)
			return
		}

		// Store upload info in memory (not in database yet)
		// We'll create the database record only after upload completion
		// This avoids having null download_url in the database

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

		// Check total upload size limit (10GB = 10000 MB)
		totalSizeMB, err := db.GetTotalUploadSize(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("Database error checking total upload size: %v", err), http.StatusInternalServerError)
			return
		}

		maxTotalSizeMB := float64(cfg.Server.MaxTotalUploadMB)

		if totalSizeMB > maxTotalSizeMB {
			maxTotalSizeGB := cfg.Server.MaxTotalUploadMB / 1024
			http.Error(w, fmt.Sprintf("Total upload size limit exceeded (%dGB). Current total: %.2fMB", maxTotalSizeGB, totalSizeMB), http.StatusBadRequest)
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
			http.Error(w, fmt.Sprintf("Database error updating upload completion: %v", err), http.StatusInternalServerError)
			return
		}

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
