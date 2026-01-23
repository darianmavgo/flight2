package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// 1. Read URLs from docs/TEST_BANQUET.md
	urls, err := readURLs("docs/TEST_BANQUET.md")
	if err != nil {
		log.Fatalf("Failed to read URLs: %v", err)
	}
	fmt.Printf("Found %d URLs\n", len(urls))

	// 2. Create/Open SQLite DB
	if err := os.MkdirAll("sample_data", 0755); err != nil {
		log.Fatalf("Failed to create sample_data directory: %v", err)
	}
	dbPath := "sample_data/test_links.db"
	os.Remove(dbPath) // Start fresh
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// 3. Create Tables
	createTablesSQL := `
	CREATE TABLE test_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL
	);
	CREATE TABLE test_run_timestamp (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		test_link_id INTEGER,
		parsed_result TEXT,
		error TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(test_link_id) REFERENCES test_links(id)
	);
	`
	_, err = db.Exec(createTablesSQL)
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	// 4. Insert URLs
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}
	stmt, err := tx.Prepare("INSERT INTO test_links (url) VALUES (?)")
	if err != nil {
		log.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	for _, u := range urls {
		_, err = stmt.Exec(u)
		if err != nil {
			log.Printf("Failed to insert URL %s: %v", u, err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}

	fmt.Println("Successfully populated test_links.db")
}

func readURLs(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			urls = append(urls, line)
		}
	}
	return urls, scanner.Err()
}
