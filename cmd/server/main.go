package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"flight2/internal/config"
	"flight2/internal/data"
	"flight2/internal/secrets"
	"flight2/internal/server"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "config.hcl", "Path to configuration file")
	flag.Parse()

	// Load Config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("Warning: Could not load %s: %v", *configPath, err)
		// Fallback to defaults provided by LoadConfig internally if file missing,
		// but here err means file existed but bad parse or other error.
		// If file missing, LoadConfig currently returns defaults.
		// Let's assume we proceed if possible, but LoadConfig returns defaults on NotExist.
		// If it's a parse error, we might want to exit?
		// For now, let's respect env vars override or just fail if critical.
		// Actually, let's respect env vars as overrides to config.
	}

	// Env vars override
	if p := os.Getenv("PORT"); p != "" {
		cfg.Port = p
	}
	if sf := os.Getenv("SERVE_FOLDER"); sf != "" {
		cfg.ServeFolder = sf
	}

	// Initialize Secrets Manager
	secretsService, err := secrets.NewService(cfg.SecretsDB, cfg.SecretKey)
	if err != nil {
		log.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	// Initialize Data Manager (BigCache + MkSQLite)
	dataManager, err := data.NewManager(cfg.Verbose)
	if err != nil {
		log.Fatalf("Failed to initialize data manager: %v", err)
	}

	// Check if templates exist, if not create them.
	if _, err := os.Stat(cfg.TemplateDir); os.IsNotExist(err) {
		createDefaultTemplates(cfg.TemplateDir)
	}

	// Initialize Server
	srv := server.NewServer(dataManager, secretsService, cfg.TemplateDir, cfg.ServeFolder, cfg.Verbose, cfg.AutoSelectTb0)

	log.Printf("Loaded configuration. SecretKey: %s", cfg.SecretKey)

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

func createDefaultTemplates(dir string) {
	os.MkdirAll(dir, 0755)

	os.WriteFile(dir+"/head.html", []byte(`
<!DOCTYPE html>
<html>
<head>
    <title>Flight2 Data Browser</title>
    <style>
        table { border-collapse: collapse; width: 100%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        tr:nth-child(even) { background-color: #f9f9f9; }
    </style>
</head>
<body>
    <h1>Data Browser</h1>
`), 0644)

	os.WriteFile(dir+"/foot.html", []byte(`
</body>
</html>
`), 0644)

	os.WriteFile(dir+"/row.html", []byte(`
<tr>
    {{range .}}<td>{{.}}</td>{{end}}
</tr>
`), 0644)
}
