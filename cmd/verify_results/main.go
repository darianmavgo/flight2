package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "sample_data/test_links.db")
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT count(*) FROM test_run_timestamp").Scan(&count)
	if err != nil {
		log.Fatalf("Failed to count rows: %v", err)
	}
	fmt.Printf("Total rows in test_run_timestamp: %d\n", count)

	rows, err := db.Query("SELECT id, parsed_result, error FROM test_run_timestamp LIMIT 3")
	if err != nil {
		log.Fatalf("Failed to query rows: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var res, errStr sql.NullString
		if err := rows.Scan(&id, &res, &errStr); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		r := res.String
		if len(r) > 50 {
			r = r[:50] + "..."
		}
		fmt.Printf("ID: %d, Result: %s, Error: %s\n", id, r, errStr.String)
	}
}
