package dataset

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Type: Integration Test
func TestManager_GetSQLiteDB_ExtensionResolution(t *testing.T) {
	// Create a temp directory to simulate data folder
	// Use test_output for temp dir
	testOutputDir, _ := filepath.Abs("../../test_output")
	tempDir := filepath.Join(testOutputDir, "tmp_ext_test")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a csv file: testfile.csv
	csvPath := filepath.Join(tempDir, "testfile.csv")
	err := os.WriteFile(csvPath, []byte("id,name\n1,Test"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := NewManager(true, filepath.Join(testOutputDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}

	creds := map[string]interface{}{
		"type": "local",
	}

	// Request "testfile" (without extension)
	reqPath := filepath.Join(tempDir, "testfile")

	dbPath, err := mgr.GetSQLiteDB(context.Background(), reqPath, creds, "test-alias")
	if err != nil {
		t.Fatalf("Failed to resolve extension: %v", err)
	}
	defer os.Remove(dbPath)

	// Ensure DB exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("DB file was not created")
	}

	// Verify that requesting with extension also works and might cache separately
	// (logic says cache key includes sourcePath. If sourcePath was updated in GetSQLiteDB,
	// the cache key uses the UPDATED path or the ORIGINAL one?
	// Let's check logic:
	// func (m *Manager) GetSQLiteDB(..., sourcePath, ...) {
	//    if ... { sourcePath = p }
	//    key := ... sourcePath
	// }
	// So the cache key uses the RESOLVED path.
	// So if I request `testfile`, it resolves to `testfile.csv`. Key is `...:testfile.csv`.
	// If I request `testfile.csv`, key is `...:testfile.csv`.
	// So they should share cache! This is great.

	dbPath2, err := mgr.GetSQLiteDB(context.Background(), csvPath, creds, "test-alias")
	if err != nil {
		t.Fatalf("Failed to get DB with explicit extension: %v", err)
	}
	defer os.Remove(dbPath2)

	// We can't easily verify cache hit via public API without logs or timing,
	// but the functionality is what matters here.
}
