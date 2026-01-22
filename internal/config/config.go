package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Port          string `json:"port"`
	ServeFolder   string `json:"serve_folder"`
	TemplateDir   string `json:"template_dir"`
	SecretsDB     string `json:"secrets_db"`
	SecretKey     string `json:"secret_key"`
	Verbose       bool   `json:"verbose"`
	AutoSelectTb0 bool   `json:"auto_select_tb0"`
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

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	err = json.NewDecoder(f).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
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
