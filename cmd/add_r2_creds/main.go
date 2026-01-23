package main

import (
	"flight2/internal/secrets"
	"log"
)

func main() {
	// Paths from config.hcl
	dbPath := "secrets.db"
	keyPath := "key" // explicitly using "key" as per config.hcl

	// Initialize Secrets Manager
	secretsService, err := secrets.NewService(dbPath, keyPath)
	if err != nil {
		log.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	// Credentials from tests/cf_r2_test.go
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

	alias := "r2-auth"

	_, err = secretsService.StoreCredentials(alias, creds)
	if err != nil {
		log.Fatalf("Failed to store credentials: %v", err)
	}

	log.Printf("Successfully added credentials for alias: %s", alias)
}
