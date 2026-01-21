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
	AutoSelectTb0 bool   `hcl:"auto_select_tb0,optional,default:true"`
}

func LoadConfig(filename string) (*Config, error) {
	var config Config

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return &Config{
			Port:          "8080",
			TemplateDir:   "templates",
			SecretsDB:     "secrets.db",
			SecretKey:     ".secret.key",
			AutoSelectTb0: true,
		}, nil
	}

	err := hclsimple.DecodeFile(filename, nil, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// Set defaults if empty
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
