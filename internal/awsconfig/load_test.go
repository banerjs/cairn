package awsconfig

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
)

func TestLoad_EmptyRegion(t *testing.T) {
	if _, err := Load(context.Background(), ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoad_SuccessStubbed(t *testing.T) {
	prev := loadDefaultConfig
	defer func() { loadDefaultConfig = prev }()
	loadDefaultConfig = func(ctx context.Context, optFns ...func(*awscfg.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	cfg, err := Load(context.Background(), "eu-west-1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Region != "us-east-1" {
		t.Fatalf("region %q", cfg.Region)
	}
}
