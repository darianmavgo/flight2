package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"flight2/internal/config"
	"flight2/internal/secrets"
)

func main() {
	alias := flag.String("alias", "", "Generic alias for the credential")
	fsFile := flag.String("file", "", "Path to JSON file containing credentials")
	configPath := flag.String("config", "config.hcl", "Path to config file (to find secrets DB)")
	flag.Parse()

	if *alias == "" || *fsFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Load Config to find DB path
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Fatal Error: Could not load %s: %v", *configPath, err)
	}

	// Read JSON file
	data, err := os.ReadFile(*fsFile)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", *fsFile, err)
	}

	var creds map[string]interface{}
	if err := json.Unmarshal(data, &creds); err != nil {
		log.Fatalf("Invalid JSON in %s: %v", *fsFile, err)
	}

	// Initialize Secrets Service
	secretsService, err := secrets.NewService(cfg.SecretsDB, cfg.SecretKey)
	if err != nil {
		log.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	// Store
	actualAlias, err := secretsService.StoreCredentials(*alias, creds)
	if err != nil {
		log.Fatalf("Failed to store credentials: %v", err)
	}

	fmt.Printf("Successfully stored credentials for alias: %s\n", actualAlias)
}
