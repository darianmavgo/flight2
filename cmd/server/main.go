package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"flight2/internal/config"
	"flight2/internal/dataset"
	"flight2/internal/dataset_source"
	"flight2/internal/secrets"
	"flight2/internal/server"

	"io"

	"github.com/darianmavgo/banquet"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "config.hcl", "Path to configuration file")
	flag.Parse()

	// Load Config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Fatal Error: Could not load %s: %v", *configPath, err)
	}

	// Setup logging
	os.MkdirAll("logs", 0755)
	logFile, err := os.OpenFile("logs/app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		mw := io.MultiWriter(os.Stderr, logFile)
		log.SetOutput(mw)
		log.Printf("Logging to logs/app.log")
	} else {
		log.Printf("Warning: Failed to open log file: %v", err)
	}

	if cfg.Verbose {
		banquet.SetVerbose(true)
		log.Printf("Verbose mode enabled across repositories.")
	}

	// Env vars override
	if p := os.Getenv("PORT"); p != "" {
		cfg.Port = p
	}
	if sf := os.Getenv("SERVE_FOLDER"); sf != "" {
		cfg.ServeFolder = sf
	}

	// Initialize Secrets Manager
	secretsService, err := secrets.NewService(cfg.UserSecretsDB, cfg.SecretKey)
	if err != nil {
		log.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	// Initialize Source/Rclone VFS Cache
	dataset_source.Init(cfg.CacheDir)

	// Initialize Data Manager (BigCache + MkSQLite)
	dataManager, err := dataset.NewManager(cfg.Verbose, cfg.CacheDir)
	if err != nil {
		log.Fatalf("Failed to initialize data manager: %v", err)
	}

	// Initialize Server
	srv := server.NewServer(dataManager, secretsService, cfg.ServeFolder, cfg.Verbose, cfg.AutoSelectTb0, cfg.LocalOnly, cfg.DefaultDB)

	startPort, _ := strconv.Atoi(cfg.Port)
	if startPort == 0 {
		startPort = 8080
	}

	var finalErr error
	for i := 0; i < 3; i++ {
		currentPort := strconv.Itoa(startPort + i)
		ln, err := net.Listen("tcp", ":"+currentPort)
		if err != nil {
			log.Printf("Port %s is busy, trying next...", currentPort)
			finalErr = err
			continue
		}

		log.Printf("Starting server on port %s", currentPort)
		// We use http.Serve with the listener
		finalErr = http.Serve(ln, srv.Router())
		if finalErr != nil {
			log.Fatalf("Server failed: %v", finalErr)
		}
		return
	}

	if finalErr != nil {
		log.Fatalf("Failed to start server after 3 attempts: %v", finalErr)
	}
}
