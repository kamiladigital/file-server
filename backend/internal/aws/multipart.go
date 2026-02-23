package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type MultipartUploadInfo struct {
	UploadID string `json:"uploadId"`
	Key      string `json:"key"`
}

// InitiateMultipartUpload starts a multipart upload and returns the upload ID and key.
func InitiateMultipartUpload(s3Client *s3.Client, bucket, key string) (*MultipartUploadInfo, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	result, err := s3Client.CreateMultipartUpload(context.TODO(), input)
	if err != nil {
		return nil, err
	}
	uploadID := ""
	if result.UploadId != nil {
		uploadID = *result.UploadId
	}
	return &MultipartUploadInfo{
		UploadID: uploadID,
		Key:      key,
	}, nil
}

// GetPresignedURLForPart generates a presigned URL for uploading a part.
func GetPresignedURLForPart(s3Client *s3.Client, bucket, key, uploadID string, partNumber int32) (string, error) {
	presigner := s3.NewPresignClient(s3Client)
	resp, err := presigner.PresignUploadPart(context.TODO(), &s3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(partNumber),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return "", err
	}
	return resp.URL, nil
}

// CompleteMultipartUpload completes the multipart upload with the given parts.
func CompleteMultipartUpload(s3Client *s3.Client, bucket, key, uploadID string, completedParts []s3types.CompletedPart) error {
	input := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}
	_, err := s3Client.CompleteMultipartUpload(context.TODO(), input)
	return err
}
