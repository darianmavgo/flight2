package main

import (
	"flight2/internal/config"
	"flight2/internal/secrets"
	"log"
	"os"
)

func main() {
	// Load Config to get paths
	cfg, err := config.LoadConfig("config.hcl")
	if err != nil {
		log.Fatalf("Fatal Error: Could not load config.hcl: %v", err)
	}

	// Initialize Secrets Manager
	secretsService, err := secrets.NewService(cfg.UserSecretsDB, cfg.SecretKey)
	if err != nil {
		log.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	// Credentials from environment variables
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	endpoint := os.Getenv("R2_ENDPOINT")

	if accessKey == "" || secretKey == "" || endpoint == "" {
		log.Fatal("R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, or R2_ENDPOINT not set")
	}

	creds := map[string]interface{}{
		"provider":          "Cloudflare",
		"access_key_id":     accessKey,
		"secret_access_key": secretKey,
		"endpoint":          endpoint,
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
