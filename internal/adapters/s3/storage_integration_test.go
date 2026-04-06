//go:build integration

package s3adapter

import (
	"context"
	"os"
	"testing"
)

func TestStorageIntegrationRequiresEnvironment(t *testing.T) {
	bucket := os.Getenv("INTEGRATION_S3_BUCKET")
	region := os.Getenv("INTEGRATION_AWS_REGION")
	if bucket == "" || region == "" {
		t.Skip("set INTEGRATION_S3_BUCKET and INTEGRATION_AWS_REGION to run integration tests")
	}

	client, err := NewClient(context.Background(), ClientConfig{
		Region:   region,
		Endpoint: os.Getenv("INTEGRATION_S3_ENDPOINT"),
	})
	if err != nil {
		t.Fatalf("create s3 client: %v", err)
	}
	if client == nil {
		t.Fatalf("expected non-nil client")
	}
}
