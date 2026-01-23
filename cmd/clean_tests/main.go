package main

import (
	"flag"
	"flight2/internal/config"
	"flight2/tests"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	configPath := flag.String("config", "config.hcl", "Path to config file")
	dryRun := flag.Bool("dry-run", false, "Preview changes without deleting")
	rootPath := flag.String("root", ".", "Root directory to clean")
	flag.Parse()

	// Load Config to check for protected paths
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("Warning: Could not load config %s: %v. Proceeding with caution.", *configPath, err)
		cfg = &config.Config{} // Use empty config to avoid nil pointer
	}

	absRoot, err := filepath.Abs(*rootPath)
	if err != nil {
		log.Fatalf("Failed to resolve root path: %v", err)
	}

	log.Printf("Starting cleanup in: %s", absRoot)

	// Collect protected paths from config
	protected := make(map[string]string)
	if cfg.UserSecretsDB != "" {
		protected[resolve(cfg.UserSecretsDB)] = "UserSecretsDB"
	}
	if cfg.TemplateDir != "" {
		protected[resolve(cfg.TemplateDir)] = "TemplateDir"
	}
	if cfg.ServeFolder != "" {
		protected[resolve(cfg.ServeFolder)] = "ServeFolder"
	}
	if cfg.CacheDir != "" {
		protected[resolve(cfg.CacheDir)] = "CacheDir"
	}
	if cfg.DefaultDB != "" {
		protected[resolve(cfg.DefaultDB)] = "DefaultDB"
	}

	// Walk and check before cleaning?
	// The CleanTestArtifacts function walks and deletes.
	// We should probably modify CleanTestArtifacts or wrap it to check protected paths.
	// But `tests` package doesn't know about config.
	// So we will implement a custom walker here or pass a filter?
	// The user asked to "create a cmd... that warns me if any folder matches a setting in config.hcl".

	// Since tests/util.go `CleanTestArtifacts` is simple, maybe we can't use it directly if we need complex filtering?
	// Or we use it but we pre-scan?
	// Or we just implement the logic here calling `tests.CleanTestArtifacts` ?
	// Wait, `tests.CleanTestArtifacts` does `filepath.Walk`. I can't inject middleware easily unless I change it.
	// I'll update `tests/util.go` to accept a callback or blacklist?
	// Or I can just copy the logic since it's short, but reusing is better.
	// Let's update `tests/util.go` to accept a generic `ShouldSkip(path string) bool`.

	// Actually, let's just do a pre-scan here for safety, then call the cleaner.
	// But cleaner doesn't know about protected stuff.
	// It deletes "test_*".
	// If `config.hcl` has `cache_dir = "test_cache"`, it would be deleted!
	// This is the risk.

	// So we MUST check if any config path starts with "test_" and exists.
	for path, name := range protected {
		base := filepath.Base(path)
		if strings.HasPrefix(base, "test_") {
			log.Printf("WARNING: Configured %s (%s) matches deletion pattern (starts with 'test_').", name, path)
			log.Printf("Please rename your production resources to not start with 'test_'.")
			if !*dryRun {
				fmt.Print("Are you sure you want to proceed? (y/N): ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" {
					log.Println("Aborting.")
					os.Exit(1)
				}
			}
		}
	}

	if err := tests.CleanTestArtifacts(absRoot, *dryRun); err != nil {
		log.Fatalf("Cleanup failed: %v", err)
	}

	log.Println("Cleanup complete.")
}

func resolve(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
