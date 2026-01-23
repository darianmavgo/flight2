package dataset

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Mocking source fetch or just testing logic?
// Since `internal/dataset` depends on `internal/source` which depends on `rclone`,
// a true unit test is hard without mocking the source.
// However, we can test with a local file using rclone "local" backend.

// Type: Integration Test
func TestManager_GetSQLiteDB_LocalFile(t *testing.T) {
	// Create a dummy CSV file
	// Ensure test_output exists
	testOutputDir, _ := filepath.Abs("../../test_output")
	if err := os.MkdirAll(testOutputDir, 0755); err != nil {
		t.Fatalf("Failed to create test_output: %v", err)
	}

	f, err := os.CreateTemp(testOutputDir, "test*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("id,name\n1,Alice\n2,Bob")
	f.Close()

	mgr, err := NewManager(true, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Credentials for local file access (rclone uses local backend if no config? Or we need explicit local config)
	// Rclone local backend is usually implicit for paths, but we use `regInfo.NewFs`.
	// The "local" backend needs to be specified.

	creds := map[string]interface{}{
		"type": "local",
	}

	// The source path needs to be absolute for local backend to work reliably in test
	absPath := f.Name()

	dbPath, err := mgr.GetSQLiteDB(context.Background(), absPath, creds, "test-alias")
	if err != nil {
		t.Fatalf("GetSQLiteDB failed: %v", err)
	}
	defer os.Remove(dbPath)

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("DB file not created at %s", dbPath)
	}

	// Test Cache
	// If we call again, it should come from cache (check logs if we could, but here we just check it works)
	dbPath2, err := mgr.GetSQLiteDB(context.Background(), absPath, creds, "test-alias")
	if err != nil {
		t.Fatalf("GetSQLiteDB cached failed: %v", err)
	}
	defer os.Remove(dbPath2)

	// Check content?
	// We assume mksqlite works if the file exists.
}
