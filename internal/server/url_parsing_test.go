package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/darianmavgo/banquet"
	_ "github.com/mattn/go-sqlite3"
)

// Type: Regression Test
func TestURLParsing(t *testing.T) {
	// Locate the sample_data/test_links.db file relative to this test file
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "../..")
	dbPath := filepath.Join(projectRoot, "sample_data", "test_links.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test DB at %s: %v", dbPath, err)
	}
	defer db.Close()

	// Check if tables exist (sanity check)
	_, err = db.Exec("SELECT 1 FROM test_links LIMIT 1")
	if err != nil {
		t.Fatalf("test_links table does not exist or DB is invalid: %v", err)
	}

	// Read URLs
	rows, err := db.Query("SELECT id, url FROM test_links")
	if err != nil {
		t.Fatalf("Failed to query test_links: %v", err)
	}
	defer rows.Close()

	type Link struct {
		ID  int
		URL string
	}
	var links []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.URL); err != nil {
			t.Fatalf("Failed to scan row: %v", err)
		}
		links = append(links, l)
	}

	// Prepare insertion into test_run_timestamp
	stmt, err := db.Prepare("INSERT INTO test_run_timestamp (test_link_id, parsed_result, error, timestamp) VALUES (?, ?, ?, ?)")
	if err != nil {
		t.Fatalf("Failed to prepare insert statement: %v", err)
	}
	defer stmt.Close()

	for _, link := range links {
		t.Logf("Testing URL: %s", link.URL)

		bq, parseErr := banquet.ParseNested(link.URL)

		var resultJSON []byte
		var errorStr sql.NullString

		if parseErr != nil {
			errorStr = sql.NullString{String: parseErr.Error(), Valid: true}
		} else {
			// Serialize the result to JSON
			var jsonErr error
			resultJSON, jsonErr = json.Marshal(bq)
			if jsonErr != nil {
				errorStr = sql.NullString{String: fmt.Sprintf("JSON marshal error: %v", jsonErr), Valid: true}
			}
		}

		// Insert result
		// We use sql.NullString for resultJSON because it might be empty if parse failed
		var resultStr sql.NullString
		if len(resultJSON) > 0 {
			resultStr = sql.NullString{String: string(resultJSON), Valid: true}
		}

		_, execErr := stmt.Exec(link.ID, resultStr, errorStr, time.Now())
		if execErr != nil {
			t.Errorf("Failed to save result for URL %s: %v", link.URL, execErr)
		}
	}
}
