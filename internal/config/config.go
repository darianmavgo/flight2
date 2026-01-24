package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Config struct {
	Port          string `hcl:"port,optional"`
	ServeFolder   string `hcl:"serve_folder,optional"`
	UserSecretsDB string `hcl:"user_secrets_db,optional"`
	SecretKey     string `hcl:"secret_key,optional"`
	Verbose       bool   `hcl:"verbose,optional"`
	AutoSelectTb0 bool   `hcl:"auto_select_tb0,optional"`
	LocalOnly     bool   `hcl:"local_only,optional"`
	DefaultDB     string `hcl:"default_db,optional"`
	CacheDir      string `hcl:"cache_dir,optional"`
}

func LoadConfig(filename string) (*Config, error) {
	config := Config{
		Port:          "8080",
		UserSecretsDB: "user_secrets.db",
		SecretKey:     ".secret.key",
		AutoSelectTb0: true,
		LocalOnly:     true,
		DefaultDB:     "app.sqlite",
		CacheDir:      "cache",
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return &config, nil
	}

	ext := filepath.Ext(filename)
	if ext != ".hcl" {
		return nil, fmt.Errorf("config file must have .hcl extension: %s", filename)
	}

	err := hclsimple.DecodeFile(filename, nil, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to load hcl config file: %w", err)
	}

	// Double check defaults if empty
	if config.Port == "" {
		config.Port = "8080"
	}
	if config.UserSecretsDB == "" {
		config.UserSecretsDB = "user_secrets.db"
	}
	if config.SecretKey == "" {
		config.SecretKey = ".secret.key"
	}

	return &config, nil
}
