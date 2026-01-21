package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Mock or create a real helper to setup DB
func setupTestDB(t *testing.T, tableNames []string) *sql.DB {
	// Use a temp file for DB
	f, err := os.CreateTemp("", "testdb_*.sqlite")
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
				// We don't need other fields for listTables
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
