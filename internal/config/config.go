package config

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Config struct {
	Port          string `hcl:"port,optional"`
	ServeFolder   string `hcl:"serve_folder,optional"`
	TemplateDir   string `hcl:"template_dir,optional"`
	SecretsDB     string `hcl:"secrets_db,optional"`
	SecretKey     string `hcl:"secret_key,optional"`
	Verbose       bool   `hcl:"verbose,optional"`
	AutoSelectTb0 bool   `hcl:"auto_select_tb0,optional"`
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

	err := hclsimple.DecodeFile(filename, nil, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// Double check empty strings if decoded from file (though hclsimple shouldn't overwrite if absent)
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
