package main

import (
	"log"
	"net/http"
	"os"

	"flight2/internal/data"
	"flight2/internal/secrets"
	"flight2/internal/server"
)

func main() {
	// Initialize Secrets Manager
	secretsService, err := secrets.NewService("secrets.db", ".secret.key")
	if err != nil {
		log.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	// Initialize Data Manager (BigCache + MkSQLite)
	dataManager, err := data.NewManager()
	if err != nil {
		log.Fatalf("Failed to initialize data manager: %v", err)
	}

    // Template dir
    // We need to ensure we have templates for sqliter.
    // For now, we can use the ones in the module if we can locate them,
    // or we can expect them to be in a "templates" folder relative to CWD.
    // We should probably create some default templates if they don't exist.

    // Check if templates exist, if not create them.
    if _, err := os.Stat("templates"); os.IsNotExist(err) {
        createDefaultTemplates("templates")
    }

	// Initialize Server
	srv := server.NewServer(dataManager, secretsService, "templates")

	// Start HTTP Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	if err := http.ListenAndServe(":"+port, srv.Router()); err != nil {
		log.Fatalf("Server failed: %v", err)
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
