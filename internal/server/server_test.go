package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/darianmavgo/sqliter/sqliter"
)

// Type: Unit Test
func TestHandleDebugEnv(t *testing.T) {
	// Set a custom env var to verify it appears
	key := "FLIGHT2_TEST_ENV"
	val := "some_value"
	os.Setenv(key, val)
	defer os.Unsetenv(key)

	// Create a server instance with nil dependencies as they are not used by handleDebugEnv
	s := &Server{}
	router := s.Router()

	req, err := http.NewRequest("GET", "/app/debug/env", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check if the output contains our env var
	expected := key + "=" + val
	if !strings.Contains(rr.Body.String(), expected) {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

// Mock or create a real helper to setup DB
func setupTestDB(t *testing.T, tableNames []string) *sql.DB {
	// Use test_output for DB
	testOutputDir := "../../test_output"
	if err := os.MkdirAll(testOutputDir, 0755); err != nil {
		t.Fatalf("Failed to create test_output dir: %v", err)
	}
	f, err := os.CreateTemp(testOutputDir, "testdb_*.sqlite")
	if err != nil {
		t.Fatalf("Failed to create temp db: %v", err)
	}
	f.Close()
	dbPath := f.Name()
	t.Cleanup(func() { os.Remove(dbPath) })

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}

	for _, name := range tableNames {
		_, err := db.Exec("CREATE TABLE " + name + " (id INTEGER)")
		if err != nil {
			t.Fatalf("Failed to create table %s: %v", name, err)
		}
	}
	return db
}

// Type: Unit Test
func TestListTables_AutoSelectTb0(t *testing.T) {
	tests := []struct {
		name           string
		tables         []string
		autoSelect     bool
		expectRedirect bool
		target         string
	}{
		{
			name:           "AutoSelect True, Only tb0",
			tables:         []string{"tb0"},
			autoSelect:     true,
			expectRedirect: true,
			target:         "/testdb/tb0",
		},
		{
			name:           "AutoSelect True, Only other",
			tables:         []string{"other"},
			autoSelect:     true,
			expectRedirect: false,
		},
		{
			name:           "AutoSelect True, Multiple tables including tb0",
			tables:         []string{"tb0", "other"},
			autoSelect:     true,
			expectRedirect: false,
		},
		{
			name:           "AutoSelect False, Only tb0",
			tables:         []string{"tb0"},
			autoSelect:     false,
			expectRedirect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t, tt.tables)
			defer db.Close()

			// Create Server with desired config
			s := &Server{
				verbose:       true,
				autoSelectTb0: tt.autoSelect,
				tableWriter:   sqliter.NewTableWriter(nil, nil),
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "http://example.com/testdb", nil)

			s.listTables(w, r, db, "/testdb")

			resp := w.Result()
			if tt.expectRedirect {
				if resp.StatusCode != http.StatusFound {
					t.Errorf("Expected status 302 Found, got %v", resp.Status)
				}
				loc, err := resp.Location()
				if err != nil {
					t.Fatalf("Failed to get location: %v", err)
				}
				if loc.Path != tt.target {
					t.Errorf("Expected redirect to %s, got %s", tt.target, loc.Path)
				}
			} else {
				if resp.StatusCode == http.StatusFound {
					t.Errorf("Expected no redirect, got 302 Found")
				}
			}
		})
	}
}
