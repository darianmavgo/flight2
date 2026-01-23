package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Type: Unit Test
func TestLoadConfig(t *testing.T) {
	// Create a temp config file
	content := `
port = "9090"
serve_folder = "/tmp/data"
`
	// Use test_output directory
	testOutputDir := "../../test_output"
	if err := os.MkdirAll(testOutputDir, 0755); err != nil {
		t.Fatalf("Failed to create test_output dir: %v", err)
	}
	configPath := filepath.Join(testOutputDir, "config_test.hcl")

	// Clean up before test (optional)
	os.Remove(configPath)
	defer os.Remove(configPath)

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("Expected Port 9090, got %s", cfg.Port)
	}
	if cfg.ServeFolder != "/tmp/data" {
		t.Errorf("Expected ServeFolder /tmp/data, got %s", cfg.ServeFolder)
	}
	// Check defaults
	if cfg.TemplateDir != "templates" {
		t.Errorf("Expected default TemplateDir templates, got %s", cfg.TemplateDir)
	}
}

// Type: Unit Test
func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfig("non_existent_file.hcl")
	if err != nil {
		t.Fatalf("LoadConfig failed for missing file: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Expected default Port 8080, got %s", cfg.Port)
	}
}
