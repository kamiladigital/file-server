package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	awsint "file-server/internal/aws"
	cfgpkg "file-server/internal/config"

	v2aws "github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func main() {
	cfg := cfgpkg.Load()
	s3Client := awsint.NewS3(&cfg.AWS)

	// Initiate multipart under fileserver/ prefix
	key := fmt.Sprintf("fileserver/hello-%d.txt", time.Now().Unix())
	info, err := awsint.InitiateMultipartUpload(s3Client, cfg.AWS.Bucket, key)
	if err != nil {
		log.Fatalf("initiate failed: %v", err)
	}
	fmt.Printf("Initiated upload: uploadId=%s key=%s\n", info.UploadID, info.Key)

	// Presign part 1
	url, err := awsint.GetPresignedURLForPart(s3Client, cfg.AWS.Bucket, key, info.UploadID, 1)
	if err != nil {
		log.Fatalf("presign failed: %v", err)
	}
	fmt.Printf("Presigned URL for part 1: %s\n", url)

	// Upload 'Hello World' as part 1 using HTTP PUT
	body := []byte("Hello World")
	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	if err != nil {
		log.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len(body))
	client := &http.Client{}
	resp, err := client.Do(req)
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

	// Complete multipart
	parts := []s3types.CompletedPart{{ETag: &etag, PartNumber: v2aws.Int32(1)}}
	if err := awsint.CompleteMultipartUpload(s3Client, cfg.AWS.Bucket, key, info.UploadID, parts); err != nil {
		log.Fatalf("complete failed: %v", err)
	}
	fmt.Printf("Completed multipart upload for key=%s\n", key)
}
