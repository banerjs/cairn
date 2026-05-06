package awsconfig

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
)

// loadDefaultConfig is swapped in tests (avoid live AWS metadata calls).
var loadDefaultConfig = awscfg.LoadDefaultConfig

// Load returns the default AWS config with region forced from the operator config.
func Load(ctx context.Context, region string) (aws.Config, error) {
	if region == "" {
		return aws.Config{}, fmt.Errorf("awsconfig: empty region")
	}
	return loadDefaultConfig(ctx, awscfg.WithRegion(region))
}
