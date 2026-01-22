package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temp config file
	content := `
{
  "port": "9090",
  "serve_folder": "/tmp/data"
}
`
	tmpFile, err := os.CreateTemp("", "config_test_*.hcl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
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

func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfig("non_existent_file.hcl")
	if err != nil {
		t.Fatalf("LoadConfig failed for missing file: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Expected default Port 8080, got %s", cfg.Port)
	}
}
