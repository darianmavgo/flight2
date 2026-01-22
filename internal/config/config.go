package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Config struct {
	Port          string `json:"port" hcl:"port,optional"`
	ServeFolder   string `json:"serve_folder" hcl:"serve_folder,optional"`
	TemplateDir   string `json:"template_dir" hcl:"template_dir,optional"`
	SecretsDB     string `json:"secrets_db" hcl:"secrets_db,optional"`
	SecretKey     string `json:"secret_key" hcl:"secret_key,optional"`
	Verbose       bool   `json:"verbose" hcl:"verbose,optional"`
	AutoSelectTb0 bool   `json:"auto_select_tb0" hcl:"auto_select_tb0,optional"`
}

func LoadConfig(filename string) (*Config, error) {
	config := Config{
		Port:          "8080",
		TemplateDir:   "templates",
		SecretsDB:     "secrets.db",
		SecretKey:     ".secret.key",
		AutoSelectTb0: true,
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return &config, nil
	}

	ext := filepath.Ext(filename)
	if ext == ".hcl" {
		err := hclsimple.DecodeFile(filename, nil, &config)
		if err != nil {
			return nil, fmt.Errorf("failed to load hcl config file: %w", err)
		}
	} else {
		f, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		err = json.NewDecoder(f).Decode(&config)
		if err != nil {
			return nil, fmt.Errorf("failed to load json config file: %w", err)
		}
	}

	// Double check defaults if empty
	if config.Port == "" {
		config.Port = "8080"
	}
	if config.TemplateDir == "" {
		config.TemplateDir = "templates"
	}
	if config.SecretsDB == "" {
		config.SecretsDB = "secrets.db"
	}
	if config.SecretKey == "" {
		config.SecretKey = ".secret.key"
	}

	return &config, nil
}
