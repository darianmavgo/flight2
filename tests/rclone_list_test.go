package tests

import (
	"context"
	"flight2/internal/secrets"
	"flight2/internal/server"
	"flight2/internal/source"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
)

// TestRcloneListing verifies that source.ListEntries works correctly
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
	creds := map[string]interface{}{
		"provider":          "Cloudflare",
		"access_key_id":     "0d5aacd854377d79f3c83caa688effbe",
		"secret_access_key": "986a762b395b7b9ebc6c08a62a64cbd8a872654ce7c927270e46cab19c9b0af5",
		"endpoint":          "https://d8dc30936fb37cbd74552d31a709f6cf.r2.cloudflarestorage.com",
		"region":            "auto",
		"chunk_size":        "5Mi",
		"copy_cutoff":       "5Mi",
		"type":              "s3",
	}

	// 3. Init Rclone VFS in correct cache dir
	source.Init(cfg.CacheDir)

	// 4. Test Listing
	// The bucket path we want to list is 'test-mksqlite/sample_data'
	// Note: For S3, the "bucket" is usually part of the root.
	// In source.go logic for cloud providers, we use "" as fsRoot, and path is absolute from there.
	targetPath := "test-mksqlite/sample_data"

	t.Logf("Listing entries in: %s", targetPath)
	entries, err := source.ListEntries(context.Background(), targetPath, creds)
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
	srv := server.NewServer(nil, ss, tmpDir, cfg.ServeFolder, true, true, false, cfg.DefaultDB)

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
