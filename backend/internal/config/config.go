package config

import (
	"context"
	"log"
	"os"

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
}

type Config struct {
	AWS AWSConfig
}

// Load reads environment variables (optionally from a .env file), validates them,
// and calls STS GetCallerIdentity to log the active caller.
func Load() *Config {
	_ = godotenv.Load()
	awsCfg := AWSConfig{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Region:          os.Getenv("AWS_REGION"),
		Bucket:          os.Getenv("AWS_BUCKET"),
	}
	if awsCfg.AccessKeyID == "" || awsCfg.SecretAccessKey == "" || awsCfg.Region == "" || awsCfg.Bucket == "" {
		log.Fatal("Missing AWS credentials in environment variables")
	}

	cfg, err := v2config.LoadDefaultConfig(context.TODO(),
		v2config.WithRegion(awsCfg.Region),
		v2config.WithCredentialsProvider(v2cred.NewStaticCredentialsProvider(awsCfg.AccessKeyID, awsCfg.SecretAccessKey, "")),
	)
	if err != nil {
		log.Printf("warning: failed to load AWS SDK config: %v", err)
		return &Config{AWS: awsCfg}
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

	return &Config{AWS: awsCfg}
}
