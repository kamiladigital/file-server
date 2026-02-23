package aws

import (
	"context"

	"file-server/internal/config"

	v2config "github.com/aws/aws-sdk-go-v2/config"
	v2cred "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// NewS3 creates an AWS SDK v2 S3 client using the provided config.AWSConfig.
func NewS3(cfg *config.AWSConfig) *s3.Client {
	vcfg, err := v2config.LoadDefaultConfig(context.TODO(),
		v2config.WithRegion(cfg.Region),
		v2config.WithCredentialsProvider(v2cred.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		// fallback: attempt to load defaults without static provider
		vcfg, _ = v2config.LoadDefaultConfig(context.TODO(), v2config.WithRegion(cfg.Region))
	}
	return s3.NewFromConfig(vcfg)
}
