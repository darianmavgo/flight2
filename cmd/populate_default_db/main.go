package main

import (
	"bufio"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"

	"flight2/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// 0. Load Config
	cfg, err := config.LoadConfig("config.hcl")
	if err != nil {
		log.Fatalf("Fatal Error: Could not load config.hcl: %v", err)
	}

	// 1. Read TEST_BANQUET.md
	file, err := os.Open("TEST_BANQUET.md")
	if err != nil {
		log.Fatalf("Failed to open TEST_BANQUET.md: %v", err)
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			urls = append(urls, line)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading TEST_BANQUET.md: %v", err)
	}

	// 2. Create/Open app.sqlite from config
	dbPath := cfg.DefaultDB
	os.Remove(dbPath) // Start fresh

	// Ensure directory exists
	if dir := filepath.Dir(dbPath); dir != "." {
		os.MkdirAll(dir, 0755)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database %s: %v", dbPath, err)
	}
	defer db.Close()

	// 3. Create table test_links
	_, err = db.Exec("CREATE TABLE test_links (id INTEGER PRIMARY KEY AUTOINCREMENT, url TEXT)")
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	// 4. Insert data
	stmt, err := db.Prepare("INSERT INTO test_links (url) VALUES (?)")
	if err != nil {
		log.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	for _, url := range urls {
		_, err = stmt.Exec(url)
		if err != nil {
			log.Printf("Failed to insert URL %s: %v", url, err)
		}
	}

	log.Printf("Successfully populated app.sqlite with %d links.", len(urls))
}
