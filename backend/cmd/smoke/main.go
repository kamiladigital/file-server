package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func main() {
	// Test the API endpoints
	baseURL := "http://localhost:8080"
	filename := fmt.Sprintf("test-%d.txt", time.Now().Unix())
	fileSize := int64(len("Hello World"))

	// 1. Initiate multipart upload
	initReq := map[string]interface{}{
		"key":  filename,
		"size": fileSize,
	}
	initJSON, _ := json.Marshal(initReq)

	resp, err := http.Post(baseURL+"/initiate-multipart", "application/json", bytes.NewReader(initJSON))
	if err != nil {
		log.Fatalf("Failed to initiate multipart upload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Initiate failed with status %d: %s", resp.StatusCode, string(body))
	}

	var initResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&initResult); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
	}

	uploadID := initResult["uploadId"].(string)
	key := initResult["key"].(string)
	fmt.Printf("Initiated upload: uploadId=%s key=%s\n", uploadID, key)

	// 2. Get presigned URL for part 1
	presignReq := map[string]interface{}{
		"key":        key,
		"uploadId":   uploadID,
		"partNumber": 1,
	}
	presignJSON, _ := json.Marshal(presignReq)

	resp, err = http.Post(baseURL+"/presign-part", "application/json", bytes.NewReader(presignJSON))
	if err != nil {
		log.Fatalf("Failed to get presigned URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Presign failed with status %d: %s", resp.StatusCode, string(body))
	}

	var presignResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&presignResult); err != nil {
		log.Fatalf("Failed to decode presign response: %v", err)
	}

	presignedURL := presignResult["url"].(string)
	fmt.Printf("Presigned URL for part 1: %s\n", presignedURL)

	// 3. Upload part using presigned URL
	body := []byte("Hello World")
	req, err := http.NewRequest("PUT", presignedURL, bytes.NewReader(body))
	if err != nil {
		log.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len(body))
	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		log.Fatalf("upload part failed: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Fatalf("upload returned status %d", resp.StatusCode)
	}
	etag := resp.Header.Get("ETag")
	fmt.Printf("Uploaded part1, ETag=%s\n", etag)

	// 4. Complete multipart upload
	completeReq := map[string]interface{}{
		"key":      key,
		"uploadId": uploadID,
		"parts": []map[string]interface{}{
			{
				"etag":       etag,
				"partNumber": 1,
			},
		},
	}
	completeJSON, _ := json.Marshal(completeReq)

	resp, err = http.Post(baseURL+"/complete-multipart", "application/json", bytes.NewReader(completeJSON))
	if err != nil {
		log.Fatalf("Failed to complete multipart upload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Complete failed with status %d: %s", resp.StatusCode, string(body))
	}

	var completeResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&completeResult); err != nil {
		log.Fatalf("Failed to decode complete response: %v", err)
	}

	fmt.Printf("Completed multipart upload: status=%s\n", completeResult["status"])
	fmt.Printf("Public URL: %s\n", completeResult["publicUrl"])
	fmt.Printf("Download URL: %s\n", completeResult["downloadUrl"])
}
