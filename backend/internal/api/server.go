package api

import (
	"encoding/json"
	"file-server/internal/aws"
	"file-server/internal/config"
	"file-server/internal/middleware"
	"fmt"
	"net/http"

	uuidv7 "github.com/samborkent/uuidv7"

	"path/filepath"

	v2aws "github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func StartServer(cfg *config.Config) {
	s3Client := aws.NewS3(&cfg.AWS)
	const maxUploadBytes = 10 * 1024 * 1024 * 1024 // 10GB

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
			http.Error(w, "File exceeds maximum allowed size (10GB)", http.StatusBadRequest)
			return
		}
		// generate uuidv7 per file and place under fileserver/<uuidv7>/<basename>
		u := uuidv7.New()
		uidStr := u.String()
		filename := filepath.Base(req.Key)
		targetKey := filepath.Join("fileserver", uidStr, filename)
		info, err := aws.InitiateMultipartUpload(s3Client, cfg.AWS.Bucket, targetKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// also return a publicly addressable URL for convenience
		fileURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.AWS.Bucket, cfg.AWS.Region, targetKey)
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
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"completed"}`))
	})

	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", nil)
}
