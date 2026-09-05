package app

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRunOnceVerification(t *testing.T) {
	for _, tt := range []struct {
		name    string
		enabled bool
		corrupt bool
	}{
		{name: "verified restore", enabled: true},
		{name: "corrupted restore preserves backups", enabled: true, corrupt: true},
		{name: "verification disabled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const original = `{"setting":"original"}`
			const changed = `{"setting":"changed during backup"}`
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.json")
			secretPath := filepath.Join(dir, "passphrase")
			for path, data := range map[string]string{configPath: original, secretPath: "test passphrase"} {
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
			}

			var mu sync.Mutex
			var uploaded []byte
			var uploadedPath string
			var downloads, deletions, successPings, failurePings int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch {
				case r.URL.Path == "/healthchecks":
					if r.Method != http.MethodGet {
						t.Errorf("unexpected success ping method: %s", r.Method)
					}
					successPings++
				case r.URL.Path == "/healthchecks/fail":
					failurePings++
					body, err := io.ReadAll(r.Body)
					if err != nil || r.Method != http.MethodPost || !strings.Contains(string(body), "decrypting backup") {
						t.Errorf("unexpected failure ping: %s %q, %v", r.Method, body, err)
					}
				case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
					if r.URL.Query().Get("prefix") != "backrest/" {
						t.Errorf("unexpected prefix: %s", r.URL)
					}
					_, _ = io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated>
<Contents><Key>backrest/previous.json.age</Key><LastModified>2026-01-01T00:00:00Z</LastModified></Contents>
</ListBucketResult>`)
				case r.Method == http.MethodHead && r.URL.Path == "/bucket/backrest/previous.json.age":
					w.Header().Set("X-Amz-Meta-Config-Sha512", "previous-hash")
				case r.Method == http.MethodPut:
					var err error
					uploaded, err = io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("read upload: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					uploadedPath = r.URL.Path
					if !strings.HasPrefix(uploadedPath, "/bucket/backrest/config-backup-") || bytes.Contains(uploaded, []byte(original)) {
						t.Error("expected an encrypted upload under a new backup key")
					}
					// Verification must use the original hash even if the live file changes.
					if err := os.WriteFile(configPath, []byte(changed), 0o600); err != nil {
						t.Errorf("change live configuration: %v", err)
					}
				case r.Method == http.MethodGet && uploadedPath != "" && r.URL.Path == uploadedPath:
					downloads++
					data := bytes.Clone(uploaded)
					if tt.corrupt && len(data) > 0 {
						data[len(data)-1] ^= 1
					}
					_, _ = w.Write(data)
				case r.Method == http.MethodGet && r.URL.Query().Has("versions"):
					if tt.enabled && downloads == 0 {
						t.Error("cleanup started before the uploaded object was downloaded")
					}
					if tt.corrupt {
						t.Error("cleanup started after corrupt restore")
					}
					type objectVersion struct {
						Key       string `xml:"Key"`
						VersionID string `xml:"VersionId"`
					}
					response := struct {
						XMLName  xml.Name        `xml:"ListVersionsResult"`
						Versions []objectVersion `xml:"Version"`
					}{}
					for _, key := range []string{"backrest/previous.json.age", strings.TrimPrefix(uploadedPath, "/bucket/")} {
						response.Versions = append(response.Versions, objectVersion{Key: key, VersionID: "v1"})
					}
					if err := xml.NewEncoder(w).Encode(response); err != nil {
						t.Errorf("encode versions: %v", err)
					}
				case r.Method == http.MethodDelete:
					if r.URL.Path != "/bucket/backrest/previous.json.age" || r.URL.Query().Get("versionId") != "v1" {
						t.Errorf("unexpected deletion: %s", r.URL)
					}
					deletions++
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			t.Cleanup(server.Close)

			verify := "false"
			if tt.enabled {
				verify = "true"
			}
			for key, value := range map[string]string{
				"CONFIG_PATH":          configPath,
				"AGE_PASSPHRASE_FILE":  secretPath,
				"S3_BUCKET":            "bucket",
				"S3_PREFIX":            "backrest",
				"S3_ENDPOINT":          server.URL,
				"S3_ACCESS_KEY_ID":     "test-key",
				"S3_SECRET_ACCESS_KEY": "test-secret",
				"S3_SESSION_TOKEN":     "",
				"AWS_REGION":           "us-east-1",
				"TZ":                   "UTC",
				"CRON_SCHEDULE":        "",
				"RUN_ONCE":             "true",
				"RUN_TIMEOUT":          "30s",
				"VERIFY_AFTER_UPLOAD":  verify,
				"HEALTHCHECKS_URL":     server.URL + "/healthchecks",
			} {
				t.Setenv(key, value)
			}

			err := Run()
			if tt.corrupt {
				if err == nil || !strings.Contains(err.Error(), "decrypting backup") {
					t.Fatalf("expected failed restore, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("run: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			wantDownloads := 0
			if tt.enabled {
				wantDownloads = 1
			}
			if downloads != wantDownloads || len(uploaded) == 0 {
				t.Errorf("unexpected backup operations: downloads=%d upload bytes=%d", downloads, len(uploaded))
			}
			if tt.corrupt {
				if deletions != 0 || successPings != 0 || failurePings != 1 {
					t.Errorf("unexpected failure outcome: deletes=%d success=%d failure=%d", deletions, successPings, failurePings)
				}
			} else if deletions != 1 || successPings != 1 || failurePings != 0 {
				t.Errorf("unexpected success outcome: deletes=%d success=%d failure=%d", deletions, successPings, failurePings)
			}
			content, err := os.ReadFile(configPath)
			if err != nil || string(content) != changed {
				t.Errorf("verification modified the live configuration: %v", err)
			}
		})
	}
}
