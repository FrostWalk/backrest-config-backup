package healthcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNotifierSuccessPing(t *testing.T) {
	t.Parallel()

	var method string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		method = request.Method
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(&http.Client{Timeout: time.Second}, server.URL)
	if err := notifier.PingSuccess(context.Background()); err != nil {
		t.Fatalf("success ping failed: %v", err)
	}
	if method != http.MethodGet {
		t.Fatalf("expected method GET, got %s", method)
	}
}

func TestNotifierFailurePingUsesFailEndpoint(t *testing.T) {
	t.Parallel()

	var path string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(&http.Client{Timeout: time.Second}, server.URL)
	if err := notifier.PingFailure(context.Background(), "error"); err != nil {
		t.Fatalf("failure ping failed: %v", err)
	}
	if path != "/fail" {
		t.Fatalf("expected path /fail, got %s", path)
	}
}
