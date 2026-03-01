package config

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"

	v2config "github.com/aws/aws-sdk-go-v2/config"
	v2cred "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/joho/godotenv"
)

type AWSConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	Bucket          string
	S3Prefix        string
}

type DatabaseConfig struct {
	URL string
}

type ServerConfig struct {
	DownloadURLExpiryDays int
}

type Config struct {
	AWS      AWSConfig
	Database DatabaseConfig
	Server   ServerConfig
}

// Load reads environment variables (optionally from a .env file), validates them,
// and calls STS GetCallerIdentity to log the active caller.
func Load() *Config {
	_ = godotenv.Load()

	// Load AWS config
	awsCfg := AWSConfig{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Region:          os.Getenv("AWS_REGION"),
		Bucket:          os.Getenv("AWS_BUCKET"),
		S3Prefix:        os.Getenv("S3_PREFIX"),
	}
	if awsCfg.AccessKeyID == "" || awsCfg.SecretAccessKey == "" || awsCfg.Region == "" || awsCfg.Bucket == "" {
		log.Fatal("Missing AWS credentials in environment variables")
	}

	// Set default S3 prefix if not provided
	if awsCfg.S3Prefix == "" {
		awsCfg.S3Prefix = "fileserver/"
	}
	// Ensure prefix ends with slash
	if !strings.HasSuffix(awsCfg.S3Prefix, "/") {
		awsCfg.S3Prefix = awsCfg.S3Prefix + "/"
	}

	// Load database config
	dbCfg := DatabaseConfig{
		URL: os.Getenv("DATABASE_URL"),
	}
	if dbCfg.URL == "" {
		log.Fatal("Missing DATABASE_URL in environment variables")
	}

	// Load server config
	expiryDaysStr := os.Getenv("DOWNLOAD_URL_EXPIRY_DAYS")
	expiryDays := 4 // default
	if expiryDaysStr != "" {
		if days, err := strconv.Atoi(expiryDaysStr); err == nil && days > 0 {
			expiryDays = days
		} else {
			log.Printf("Warning: Invalid DOWNLOAD_URL_EXPIRY_DAYS value '%s', using default %d days", expiryDaysStr, expiryDays)
		}
	}

	serverCfg := ServerConfig{
		DownloadURLExpiryDays: expiryDays,
	}

	cfg, err := v2config.LoadDefaultConfig(context.TODO(),
		v2config.WithRegion(awsCfg.Region),
		v2config.WithCredentialsProvider(v2cred.NewStaticCredentialsProvider(awsCfg.AccessKeyID, awsCfg.SecretAccessKey, "")),
	)
	if err != nil {
		log.Printf("warning: failed to load AWS SDK config: %v", err)
		return &Config{AWS: awsCfg, Database: dbCfg, Server: serverCfg}
	}

	stsClient := sts.NewFromConfig(cfg)
	out, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		log.Printf("warning: failed to call STS GetCallerIdentity: %v", err)
	} else {
		if out.Arn != nil {
			log.Printf("AWS caller identity: %s", *out.Arn)
		} else if out.Account != nil {
			log.Printf("AWS account id: %s", *out.Account)
		}
	}

	return &Config{AWS: awsCfg, Database: dbCfg, Server: serverCfg}
}
