package awsconfig

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
)

// Load returns the default AWS config with region forced from the operator config.
func Load(ctx context.Context, region string) (aws.Config, error) {
	if region == "" {
		return aws.Config{}, fmt.Errorf("awsconfig: empty region")
	}
	return awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(region))
}
