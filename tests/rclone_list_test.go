package tests

import (
	"context"
	"flight2/internal/dataset_source"
	"flight2/internal/secrets"
	"flight2/internal/server"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
)

// TestRcloneListing verifies that dataset_source.ListEntries works correctly
// using the Cloudflare R2 bucket.
// It lists the contents of the 'test-mksqlite/sample_data/' directory.
func TestRcloneListing(t *testing.T) {
	// 1. Setup Config & Secrets
	cfg, cleanup := getTestConfig(t)
	defer cleanup()

	secretsService, err := secrets.NewService(cfg.UserSecretsDB, cfg.SecretKey)
	if err != nil {
		t.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	// 2. Setup Credentials
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	endpoint := os.Getenv("R2_ENDPOINT")

	if accessKey == "" || secretKey == "" || endpoint == "" {
		t.Skip("R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, or R2_ENDPOINT not set. Skipping R2 listing test.")
	}

	creds := map[string]interface{}{
		"provider":          "Cloudflare",
		"access_key_id":     accessKey,
		"secret_access_key": secretKey,
		"endpoint":          endpoint,
		"region":            "auto",
		"chunk_size":        "5Mi",
		"copy_cutoff":       "5Mi",
		"type":              "s3",
	}

	// 3. Init Rclone VFS in correct cache dir
	dataset_source.Init(cfg.CacheDir)

	// 4. Test Listing
	// The bucket path we want to list is 'test-mksqlite/sample_data'
	// Note: For S3, the "bucket" is usually part of the root.
	// In dataset_source.go logic for cloud providers, we use "" as fsRoot, and path is absolute from there.
	targetPath := "test-mksqlite/sample_data"

	t.Logf("Listing entries in: %s", targetPath)
	entries, err := dataset_source.ListEntries(context.Background(), targetPath, creds)
	if err != nil {
		t.Fatalf("Failed to list entries: %v", err)
	}

	if len(entries) == 0 {
		t.Fatalf("Expected entries in %s, got none", targetPath)
	}

	foundCSV := false
	t.Logf("Found %d entries:", len(entries))
	for _, entry := range entries {
		name := entry.Name()
		isDir := entry.IsDir()
		size := entry.Size()
		t.Logf("- %s (Dir: %v, Size: %d)", name, isDir, size)

		if strings.Contains(name, "21mb.csv") && !isDir {
			foundCSV = true
		}
	}

	if !foundCSV {
		t.Errorf("Expected to find '21mb.csv' in listing, but did not.")
	}
}

// TestAppEndpoint verifies that the /app endpoint returns the expected HTML content.
// This is a basic integration test for the server handler.
// TestAppEndpoint verifies that the /app endpoint returns the expected HTML content.
// This is a basic integration test for the server handler.
func TestAppEndpoint(t *testing.T) {
	// 1. Setup Config
	cfg, cleanup := getTestConfig(t)
	defer cleanup()

	// Create templates
	tmpDir := path.Join("..", "test_output", "test_templates_app")
	os.MkdirAll(tmpDir, 0755)
	createTestTemplates(tmpDir)

	// 2. Setup Dependencies
	ss, err := secrets.NewService(cfg.UserSecretsDB, cfg.SecretKey)
	if err != nil {
		t.Fatalf("Failed to init secrets: %v", err)
	}
	defer ss.Close()

	// 3. Initialize Server
	// We pass nil for DataManager as /app index doesn't use it.
	srv := server.NewServer(nil, ss, cfg.ServeFolder, true, true, false, cfg.DefaultDB)

	// 4. Test /app request
	req, err := http.NewRequest("GET", "/app/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	// 5. Verify Response
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		t.Logf("Response body: %s", rr.Body.String())
	}

	expected := "Flight2 Management"
	if !strings.Contains(rr.Body.String(), expected) {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
	}
}
