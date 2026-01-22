package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"flight2/internal/config"
	"flight2/internal/secrets"
	"flight2/internal/source"

	"github.com/darianmavgo/sqliter/sqliter"
	// Register all rclone backends
	_ "github.com/rclone/rclone/backend/all"
	"github.com/rclone/rclone/fs"
)

type Server struct {
	secrets     *secrets.Service
	tableWriter *sqliter.TableWriter
	cfg         *config.Config
}

func main() {
	cfg, err := config.LoadConfig("config.hcl")
	if err != nil {
		log.Printf("Warning: Could not load config.hcl: %v", err)
	}

	// Env overrides
	if p := os.Getenv("PORT"); p != "" {
		cfg.Port = p
	}

	secretsService, err := secrets.NewService(cfg.SecretsDB, cfg.SecretKey)
	if err != nil {
		log.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	// Ensure templates exist.
	createDefaultTemplates(cfg.TemplateDir)
	tpl := sqliter.LoadTemplates(cfg.TemplateDir)

	srv := &Server{
		secrets:     secretsService,
		tableWriter: sqliter.NewTableWriter(tpl, sqliter.DefaultConfig()),
		cfg:         cfg,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", srv.handleIndex)
	mux.HandleFunc("POST /credentials", srv.handleCreateCredential)
	mux.HandleFunc("POST /credentials/delete", srv.handleDeleteCredential)
	mux.HandleFunc("GET /browse/{alias}/{path...}", srv.handleBrowse)
	mux.HandleFunc("GET /view/{alias}/{path...}", srv.handleView)

	// Static assets if any
	// mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Try starting on 3 different ports if busy
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

		log.Printf("🚀 SecureBrowse starting on http://localhost:%s", currentPort)
		// We use s.srv = &http.Server{...} if we wanted to be fancy,
		// but simple Serve with the listener is fine.
		finalErr = http.Serve(ln, mux)
		if finalErr != nil {
			log.Fatalf("Server failed: %v", finalErr)
		}
		return
	}

	if finalErr != nil {
		log.Fatalf("Failed to start server after 3 attempts: %v", finalErr)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	aliases, err := s.secrets.ListAliases()
	if err != nil {
		http.Error(w, "Failed to list credentials", http.StatusInternalServerError)
		return
	}

	s.tableWriter.StartTableList(w, "SecureBrowse Remotes")
	fmt.Fprintf(w, `
		<div class="container">
			<section class="remotes">
				<h2>📡 Configured Remotes</h2>
				<table class="premium-table">
					<thead>
						<tr><th>Alias</th><th>Actions</th></tr>
					</thead>
					<tbody>`)

	if len(aliases) == 0 {
		fmt.Fprintf(w, "<tr><td colspan='2'>No remotes configured yet.</td></tr>")
	} else {
		for _, alias := range aliases {
			fmt.Fprintf(w, `
				<tr>
					<td><strong>%s</strong></td>
					<td>
						<a href='/browse/%s/' class='btn btn-browse'>📂 Browse</a>
						<form action='/credentials/delete' method='POST' style='display:inline'>
							<input type='hidden' name='alias' value='%s'>
							<input type='submit' value='🗑️ Delete' class='btn btn-delete' onclick='return confirm("Are you sure?")'>
						</form>
					</td>
				</tr>`, alias, alias, alias)
		}
	}

	fmt.Fprintf(w, `
					</tbody>
				</table>
			</section>

			<hr class="separator">

			<section class="add-remote">
				<h2>➕ Add New Remote</h2>
				<form action="/credentials" method="POST" class="credential-form">
					<div class="form-group">
						<label>Alias Name</label>
						<input type="text" name="alias" required placeholder="e.g., my-s3-bucket">
					</div>
					<div class="form-group">
						<label>Provider Type</label>
						<select name="type" required>
							<option value="s3">Amazon S3</option>
							<option value="drive">Google Drive</option>
							<option value="dropbox">Dropbox</option>
							<option value="sftp">SFTP</option>
							<option value="azureblob">Azure Blob Storage</option>
							<option value="b2">Backblaze B2</option>
							<option value="box">Box</option>
							<option value="http">HTTP/HTTPS</option>
							<option value="local">Local Filesystem</option>
						</select>
					</div>
					<div class="form-group">
						<label>Configuration (JSON Key-Value Pairs)</label>
						<textarea name="config" rows="6" placeholder='{"access_key_id": "...", "secret_access_key": "...", "region": "us-east-1"}'></textarea>
						<small>Refer to rclone documentation for each provider's required fields.</small>
					</div>
					<button type="submit" class="btn btn-primary">Add Remote</button>
				</form>
			</section>
		</div>
	`)
	s.tableWriter.EndTableList(w)
}

func (s *Server) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	alias := r.FormValue("alias")
	fsType := r.FormValue("type")
	configStr := r.FormValue("config")

	creds := make(map[string]interface{})
	if configStr != "" {
		if err := json.Unmarshal([]byte(configStr), &creds); err != nil {
			http.Error(w, "Invalid JSON in config: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	creds["type"] = fsType

	_, err := s.secrets.StoreCredentials(alias, creds)
	if err != nil {
		http.Error(w, "Failed to store credentials", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	alias := r.FormValue("alias")
	if alias == "" {
		http.Error(w, "Alias required", http.StatusBadRequest)
		return
	}

	if err := s.secrets.DeleteCredentials(alias); err != nil {
		http.Error(w, "Failed to delete credential", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")
	relPath := r.PathValue("path")

	creds, err := s.secrets.GetCredentials(alias)
	if err != nil {
		http.Error(w, "Remote not found", http.StatusNotFound)
		return
	}

	entries, err := source.ListEntries(r.Context(), relPath, creds)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list entries: %v", err), http.StatusInternalServerError)
		return
	}

	// Sort entries: directories first, then files
	sort.Slice(entries, func(i, j int) bool {
		_, iIsDir := entries[i].(fs.Directory)
		_, jIsDir := entries[j].(fs.Directory)
		if iIsDir && !jIsDir {
			return true
		}
		if !iIsDir && jIsDir {
			return false
		}
		return entries[i].Remote() < entries[j].Remote()
	})

	s.tableWriter.StartTableList(w, "Browse - "+alias)
	fmt.Fprintf(w, "<div class='container'>")
	fmt.Fprintf(w, "<h2>📂 Browsing: %s <span style='color:var(--text-muted); font-size: 0.9rem; margin-left: 0.5rem;'>/%s</span></h2>", alias, relPath)

	cols := []string{"Type", "Name", "Size", "Modified", "Actions"}
	s.tableWriter.StartHTMLTable(w, cols, "") // Empty title as we added H2 manually

	// Add ".." link if not at root
	if relPath != "" && relPath != "." {
		parent := path.Dir(strings.TrimSuffix(relPath, "/"))
		if parent == "." {
			parent = ""
		}
		s.tableWriter.WriteHTMLRow(w, []string{
			"<span class='badge badge-folder'>📁</span>",
			fmt.Sprintf("<a href='/browse/%s/%s' style='font-weight:600;'>.. [ Parent Directory ]</a>", alias, parent),
			"-",
			"-",
			"-",
		})
	}

	for _, entry := range entries {
		name := path.Base(entry.Remote())
		fullPath := entry.Remote()

		var icon, sizeStr, modified, actions string
		if _, ok := entry.(fs.Directory); ok {
			icon = "<span class='badge badge-folder'>📁</span>"
			sizeStr = "-"
			modified = entry.ModTime(r.Context()).Format("2006-01-02 15:04:05")
			actions = fmt.Sprintf("<a href='/browse/%s/%s' class='btn btn-browse'>📂 Open</a>", alias, fullPath)
		} else {
			icon = "<span class='badge badge-file'>📄</span>"
			sizeStr = formatSize(entry.Size())
			modified = entry.ModTime(r.Context()).Format("2006-01-02 15:04:05")
			actions = fmt.Sprintf("<a href='/view/%s/%s' target='_blank' class='btn btn-view'>👁️ View</a>", alias, fullPath)
		}

		fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>", icon, name, sizeStr, modified, actions)
	}

	s.tableWriter.EndHTMLTable(w)
	fmt.Fprintf(w, "</div>")
	s.tableWriter.EndTableList(w) // We need to write foot.html
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")
	relPath := r.PathValue("path")

	creds, err := s.secrets.GetCredentials(alias)
	if err != nil {
		http.Error(w, "Remote not found", http.StatusNotFound)
		return
	}

	rc, err := source.GetFileStream(r.Context(), relPath, creds)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open file: %v", err), http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", path.Base(relPath)))
	// Detect content type if possible, or just default
	io.Copy(w, rc)
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("% d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func createDefaultTemplates(dir string) {
	os.MkdirAll(dir, 0755)

	// We only write if not exists to avoid overwriting user changes,
	// but for row.html and foot.html which are critical and often missing, we ensure them.

	headPath := dir + "/head.html"
	if _, err := os.Stat(headPath); os.IsNotExist(err) {
		os.WriteFile(headPath, []byte(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SecureBrowse | Cloud File Manager</title>
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
        <h1>🔐 SecureBrowse</h1>
    </header>
    <main>
`), 0644)
	}

	footPath := dir + "/foot.html"
	if _, err := os.Stat(footPath); os.IsNotExist(err) {
		os.WriteFile(footPath, []byte(`
    </main>
    <footer style="text-align: center; padding: 3rem 1rem; color: var(--text-muted); font-size: 0.8125rem; border-top: 1px solid var(--border); margin-top: 4rem;">
        <p>&copy; 2026 Flight2 SecureBrowse Team &bull; Powered by Antigravity AI</p>
    </footer>
</body>
</html>
`), 0644)
	}

	rowPath := dir + "/row.html"
	if _, err := os.Stat(rowPath); os.IsNotExist(err) {
		os.WriteFile(rowPath, []byte(`
<tr>
    {{range .}}<td>{{.}}</td>{{end}}
</tr>
`), 0644)
	}
}
