package s3adapter

import (
	"context"
	"testing"
)

func TestNewClientWithEndpoint(t *testing.T) {
	t.Parallel()

	client, err := NewClient(context.Background(), ClientConfig{
		Region:          "us-east-1",
		Endpoint:        "https://s3.example.com",
		AccessKeyID:     "key-id",
		SecretAccessKey: "secret-key",
	})
	if err != nil {
		t.Fatalf("new s3 client: %v", err)
	}
	if client == nil {
		t.Fatalf("expected non-nil s3 client")
	}
}
