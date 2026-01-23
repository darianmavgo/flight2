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
	"flight2/internal/secrets"
	"flight2/internal/server"
	"flight2/internal/source"

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
	source.Init(cfg.CacheDir)

	// Initialize Data Manager (BigCache + MkSQLite)
	dataManager, err := dataset.NewManager(cfg.Verbose, cfg.CacheDir)
	if err != nil {
		log.Fatalf("Failed to initialize data manager: %v", err)
	}

	// Check if templates exist, if not create them.
	if _, err := os.Stat(cfg.TemplateDir); os.IsNotExist(err) {
		createDefaultTemplates(cfg.TemplateDir)
	}

	// Initialize Server
	srv := server.NewServer(dataManager, secretsService, cfg.TemplateDir, cfg.ServeFolder, cfg.Verbose, cfg.AutoSelectTb0, cfg.LocalOnly, cfg.DefaultDB)

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

	// Base table head for sqliter
	os.WriteFile(dir+"/head.html", []byte(`<html><head><title>{{.Title}}</title></head><body><table><thead><tr>{{range .Headers}}<th>{{.}}</th>{{end}}</tr></thead><tbody>`), 0644)

	headPath := dir + "/list_head.html"
	os.WriteFile(headPath, []byte(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.}} | Flight2</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono&display=swap" rel="stylesheet">
    <style>
        :root {
            --primary: #6366f1;
            --primary-hover: #4f46e5;
            --bg: #0f172a;
            --surface: #1e293b;
            --surface-hover: #334155;
            --text: #f8fafc;
            --text-muted: #94a3b8;
            --border: #334155;
            --danger: #ef4444;
            --success: #10b981;
            --accent: #8b5cf6;
        }

        body { 
            font-family: 'Inter', -apple-system, sans-serif; 
            margin: 0; 
            background-color: var(--bg);
            color: var(--text);
            line-height: 1.6;
        }

        header {
            background: rgba(30, 41, 59, 0.7);
            backdrop-filter: blur(12px);
            -webkit-backdrop-filter: blur(12px);
            border-bottom: 1px solid var(--border);
            color: white;
            padding: 1.25rem 2rem;
            position: sticky;
            top: 0;
            z-index: 100;
        }

        header h1 { 
            margin: 0; 
            font-size: 1.25rem; 
            font-weight: 700; 
            display: flex; 
            align-items: center; 
            gap: 0.75rem;
            background: linear-gradient(135deg, #818cf8 0%, #c084fc 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        
        main { max-width: 1100px; margin: 2.5rem auto; padding: 0 1.5rem; }

        .container { 
            background: var(--surface); 
            padding: 2.5rem; 
            border-radius: 16px; 
            box-shadow: 0 20px 25px -5px rgb(0 0 0 / 0.3), 0 8px 10px -6px rgb(0 0 0 / 0.3);
            border: 1px solid var(--border);
        }

        h2 { font-size: 1.5rem; margin-top: 0; margin-bottom: 2rem; display: flex; align-items: center; gap: 0.75rem; color: #fff; }

        .premium-table { width: 100%; border-collapse: separate; border-spacing: 0; margin: 1.5rem 0; border-radius: 12px; overflow: hidden; border: 1px solid var(--border); }
        .premium-table th { background: #1e293b; text-align: left; padding: 1rem 1.25rem; font-weight: 600; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-muted); border-bottom: 1px solid var(--border); }
        .premium-table td { padding: 1rem 1.25rem; border-bottom: 1px solid var(--border); font-size: 0.9375rem; }
        .premium-table tr:last-child td { border-bottom: none; }
        .premium-table tr:hover { background-color: var(--surface-hover); }

        .btn {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            padding: 0.625rem 1.25rem;
            border-radius: 8px;
            font-size: 0.875rem;
            font-weight: 600;
            text-decoration: none;
            transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
            cursor: pointer;
            border: none;
            gap: 0.5rem;
        }

        .btn-primary { background: var(--primary); color: white; box-shadow: 0 4px 6px -1px rgba(99, 102, 241, 0.2); }
        .btn-primary:hover { background: var(--primary-hover); transform: translateY(-1px); box-shadow: 0 10px 15px -3px rgba(99, 102, 241, 0.3); }
        
        .btn-browse { background: rgba(16, 185, 129, 0.1); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.2); }
        .btn-browse:hover { background: rgba(16, 185, 129, 0.2); }
        
        .btn-view { background: rgba(99, 102, 241, 0.1); color: #818cf8; border: 1px solid rgba(99, 102, 241, 0.2); }
        .btn-view:hover { background: rgba(99, 102, 241, 0.2); }
        
        .btn-delete { background: rgba(239, 68, 68, 0.1); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.2); }
        .btn-delete:hover { background: rgba(239, 68, 68, 0.2); }

        .separator { border: 0; border-top: 1px solid var(--border); margin: 4rem 0; opacity: 0.5; }

        .form-group { margin-bottom: 1.75rem; }
        .form-group label { display: block; font-weight: 600; margin-bottom: 0.75rem; font-size: 0.875rem; color: var(--text-muted); }
        .form-group input, .form-group select, .form-group textarea {
            width: 100%;
            padding: 0.75rem 1rem;
            background: #0f172a;
            border: 1px solid var(--border);
            border-radius: 10px;
            font-size: 0.9375rem;
            color: white;
            box-sizing: border-box;
            transition: border-color 0.2s, box-shadow 0.2s;
        }
        .form-group input:focus, .form-group select:focus, .form-group textarea:focus {
            outline: none;
            border-color: var(--primary);
            box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
        }
        .form-group textarea { font-family: 'JetBrains Mono', monospace; min-height: 120px; }
        .form-group small { color: var(--text-muted); display: block; margin-top: 0.5rem; font-size: 0.75rem; }

        .remotes-list { list-style: none; padding: 0; margin: 0; }
        .remote-item { display: flex; align-items: center; justify-content: space-between; padding: 1.25rem; border: 1px solid var(--border); border-radius: 12px; margin-bottom: 1rem; transition: background 0.2s; }
        .remote-item:hover { background: var(--surface-hover); }

        a { color: var(--primary); text-decoration: none; transition: color 0.2s; }
        a:hover { color: var(--accent); }

        .badge { display: inline-block; padding: 0.25rem 0.625rem; border-radius: 9999px; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; }
        .badge-folder { background: rgba(245, 158, 11, 0.1); color: #f59e0b; }
        .badge-file { background: rgba(99, 102, 241, 0.1); color: #818cf8; }
    </style>
</head>
<body>
    <header>
        <h1>🔐 Flight2</h1>
    </header>
    <main>
`), 0644)

	footPath := dir + "/list_foot.html"
	os.WriteFile(footPath, []byte(`
    </main>
    <footer style="text-align: center; padding: 3rem 1rem; color: var(--text-muted); font-size: 0.8125rem; border-top: 1px solid var(--border); margin-top: 4rem;">
        <p>&copy; 2026 Flight2 &bull; All Systems Optimal</p>
    </footer>
</body>
</html>
`), 0644)

	// sqliter footer
	os.WriteFile(dir+"/foot.html", []byte(`</tbody></table></body></html>`), 0644)

	os.WriteFile(dir+"/list_item.html", []byte(`<li><a href="{{.URL}}">{{.Name}}</a></li>`), 0644)

	rowPath := dir + "/row.html"
	os.WriteFile(rowPath, []byte(`
<tr>
    {{range .}}<td>{{.}}</td>{{end}}
</tr>
`), 0644)
}
