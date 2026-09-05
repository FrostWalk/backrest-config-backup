package s3adapter

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestStorageFullPrefix(t *testing.T) {
	t.Parallel()

	storage := NewStorage(nil, "bucket", "/backrest/config/")
	if got, want := storage.fullPrefix(), "backrest/config/"; got != want {
		t.Fatalf("fullPrefix mismatch: got %q want %q", got, want)
	}
}

func TestStorageFullPrefixEmpty(t *testing.T) {
	t.Parallel()

	storage := NewStorage(nil, "bucket", "")
	if got := storage.fullPrefix(); got != "" {
		t.Fatalf("expected empty prefix, got %q", got)
	}
}

func TestStorageGetLatestBackupAcrossPages(t *testing.T) {
	t.Parallel()

	storage := newTestStorage(t, []s3Response{
		{
			method: http.MethodGet,
			target: "/bucket?list-type=2&prefix=backrest%2F",
			body: `<ListBucketResult>
<IsTruncated>true</IsTruncated><NextContinuationToken>page2</NextContinuationToken>
<Contents><Key>backrest/undated.json.age</Key></Contents>
<Contents><Key>backrest/older.json.age</Key><LastModified>2026-09-01T00:00:00Z</LastModified></Contents>
<Contents><Key>backrest/readme.txt</Key><LastModified>2026-09-06T00:00:00Z</LastModified></Contents>
</ListBucketResult>`,
		},
		{
			method: http.MethodGet,
			target: "/bucket?list-type=2&prefix=backrest%2F&continuation-token=page2",
			body: `<ListBucketResult><IsTruncated>false</IsTruncated>
<Contents><Key>backrest/latest.json.age</Key><LastModified>2026-09-05T00:00:00Z</LastModified></Contents>
<Contents><Key>backrest/also-undated.json.age</Key></Contents>
</ListBucketResult>`,
		},
		{
			method:  http.MethodHead,
			target:  "/bucket/backrest/latest.json.age",
			headers: http.Header{"X-Amz-Meta-Config-Sha512": {"config-hash"}},
		},
	})

	latest, err := storage.GetLatestBackup(t.Context())
	if err != nil {
		t.Fatalf("get latest backup: %v", err)
	}
	if latest == nil || latest.ObjectKey != "backrest/latest.json.age" || latest.Hash != "config-hash" {
		t.Fatalf("unexpected latest backup: %+v", latest)
	}
}

func TestStorageGetLatestBackupWithoutBackups(t *testing.T) {
	t.Parallel()

	storage := newTestStorage(t, []s3Response{{
		method: http.MethodGet,
		target: "/bucket?list-type=2&prefix=backrest%2F",
		body: `<ListBucketResult><IsTruncated>false</IsTruncated>
<Contents><Key>backrest/readme.txt</Key></Contents></ListBucketResult>`,
	}})
	latest, err := storage.GetLatestBackup(t.Context())
	if err != nil || latest != nil {
		t.Fatalf("expected no backup, got %+v, %v", latest, err)
	}
}

func TestStorageCleanupVersionsAcrossPages(t *testing.T) {
	t.Parallel()

	storage := newTestStorage(t, []s3Response{
		{
			method: http.MethodGet,
			target: "/bucket?versions=&prefix=backrest%2F",
			body: `<ListVersionsResult><IsTruncated>true</IsTruncated>
<NextKeyMarker>backrest/old.json.age</NextKeyMarker><NextVersionIdMarker>v2</NextVersionIdMarker>
<Version><Key>backrest/keep.json.age</Key><VersionId>keep-v1</VersionId></Version>
<Version><Key>backrest/readme.txt</Key><VersionId>text-v1</VersionId></Version>
<Version><Key>backrest/old.json.age</Key><VersionId>v2</VersionId></Version>
</ListVersionsResult>`,
		},
		{
			method: http.MethodGet,
			target: "/bucket?versions=&prefix=backrest%2F&key-marker=backrest%2Fold.json.age&version-id-marker=v2",
			body: `<ListVersionsResult><IsTruncated>false</IsTruncated>
<Version><Key>backrest/old.json.age</Key><VersionId>v1</VersionId></Version>
<DeleteMarker><Key>backrest/deleted.json.age</Key><VersionId>marker</VersionId></DeleteMarker>
<DeleteMarker><Key>backrest/keep.json.age</Key><VersionId>keep-marker</VersionId></DeleteMarker>
<DeleteMarker><Key>backrest/readme.txt</Key><VersionId>text-marker</VersionId></DeleteMarker>
</ListVersionsResult>`,
		},
		{method: http.MethodDelete, target: "/bucket/backrest/old.json.age?versionId=v2", status: http.StatusNoContent},
		{method: http.MethodDelete, target: "/bucket/backrest/old.json.age?versionId=v1", status: http.StatusNoContent},
		{method: http.MethodDelete, target: "/bucket/backrest/deleted.json.age?versionId=marker", status: http.StatusNoContent},
	})
	deleted, err := storage.CleanupBackups(t.Context(), "backrest/keep.json.age")
	if err != nil || deleted != 3 {
		t.Fatalf("expected three versions deleted, got %d, %v", deleted, err)
	}
}

func TestStorageCleanupFallsBackForUnsupportedVersionListing(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusNotImplemented, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			storage := newTestStorage(t, []s3Response{
				{method: http.MethodGet, target: "/bucket?versions=&prefix=backrest%2F", status: status},
				{
					method: http.MethodGet,
					target: "/bucket?list-type=2&prefix=backrest%2F",
					body: `<ListBucketResult><IsTruncated>false</IsTruncated>
<Contents><Key>backrest/keep.json.age</Key></Contents>
<Contents><Key>backrest/old.json.age</Key></Contents>
<Contents><Key>backrest/readme.txt</Key></Contents></ListBucketResult>`,
				},
				{method: http.MethodDelete, target: "/bucket/backrest/old.json.age", status: http.StatusNoContent},
				{method: http.MethodHead, target: "/bucket/backrest/old.json.age", status: http.StatusNotFound},
			})
			deleted, err := storage.CleanupBackups(t.Context(), "backrest/keep.json.age")
			if err != nil || deleted != 1 {
				t.Fatalf("expected one object deleted, got %d, %v", deleted, err)
			}
		})
	}
}

func TestStorageCleanupReturnsListingErrors(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			storage := newTestStorage(t, []s3Response{{
				method: http.MethodGet,
				target: "/bucket?versions=&prefix=backrest%2F",
				status: status,
			}})
			deleted, err := storage.CleanupBackups(t.Context(), "backrest/keep.json.age")
			if err == nil || deleted != 0 {
				t.Fatalf("expected listing error and no deletions, got %d, %v", deleted, err)
			}
		})
	}
}

func TestStorageCleanupReturnsPartialDeletionErrors(t *testing.T) {
	t.Parallel()

	storage := newTestStorage(t, []s3Response{
		{
			method: http.MethodGet,
			target: "/bucket?versions=&prefix=backrest%2F",
			body: `<ListVersionsResult><IsTruncated>false</IsTruncated>
<Version><Key>backrest/old.json.age</Key><VersionId>v2</VersionId></Version>
<Version><Key>backrest/old.json.age</Key><VersionId>v1</VersionId></Version>
</ListVersionsResult>`,
		},
		{method: http.MethodDelete, target: "/bucket/backrest/old.json.age?versionId=v2", status: http.StatusNoContent},
		{
			method: http.MethodDelete,
			target: "/bucket/backrest/old.json.age?versionId=v1",
			status: http.StatusNotImplemented,
			body:   `<Error><Code>NotImplemented</Code><Message>version deletion unsupported</Message></Error>`,
		},
	})
	deleted, err := storage.CleanupBackups(t.Context(), "backrest/keep.json.age")
	if err == nil || deleted != 1 {
		t.Fatalf("expected deletion error after one deletion, got %d, %v", deleted, err)
	}
}

type s3Response struct {
	method  string
	target  string
	status  int
	headers http.Header
	body    string
}

func newTestStorage(t *testing.T, responses []s3Response) *Storage {
	t.Helper()

	var mu sync.Mutex
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if requests >= len(responses) {
			t.Errorf("unexpected S3 request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		response := responses[requests]
		requests++
		target, err := url.Parse(response.target)
		if err != nil {
			t.Errorf("parse response target: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		query := r.URL.Query()
		query.Del("x-id")
		if r.Method != response.method || r.URL.Path != target.Path || query.Encode() != target.Query().Encode() {
			t.Errorf("request %d: got %s %s, want %s %s", requests, r.Method, r.URL, response.method, response.target)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		maps.Copy(w.Header(), response.headers)
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response.body)
	}))
	t.Cleanup(func() {
		server.Close()
		mu.Lock()
		defer mu.Unlock()
		if requests != len(responses) {
			t.Errorf("got %d S3 requests, want %d", requests, len(responses))
		}
	})

	client := s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(server.URL),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider("test-key", "test-secret", ""),
		Retryer:      aws.NopRetryer{},
		HTTPClient:   server.Client(),
	})
	return NewStorage(client, "bucket", "backrest")
}
