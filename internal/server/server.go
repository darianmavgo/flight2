package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"flight2/internal/dataset"
	"flight2/internal/dataset_source"
	"flight2/internal/secrets"

	"github.com/darianmavgo/banquet"
	"github.com/darianmavgo/sqliter/common"
	"github.com/darianmavgo/sqliter/sqliter"
	_ "github.com/mattn/go-sqlite3"

	// Register all rclone backends
	_ "github.com/rclone/rclone/backend/all"
)

// Server handles serving data.
type Server struct {
	dataManager   *dataset.Manager
	secrets       *secrets.Service
	tableWriter   *sqliter.TableWriter
	serveFolder   string
	verbose       bool
	autoSelectTb0 bool
	localOnly     bool
	defaultDB     string
	history       *RequestHistory
}

type RequestHistory struct {
	mu    sync.Mutex
	items []string
	limit int
}

func NewRequestHistory(limit int) *RequestHistory {
	return &RequestHistory{
		items: make([]string, 0, limit),
		limit: limit,
	}
}

func (h *RequestHistory) Add(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Deduplicate: remove if exists
	for i, item := range h.items {
		if item == url {
			h.items = append(h.items[:i], h.items[i+1:]...)
			break
		}
	}
	h.items = append(h.items, url)
	if len(h.items) > h.limit {
		h.items = h.items[1:]
	}
}

func (h *RequestHistory) GetRecent() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Return copy in reverse order
	res := make([]string, len(h.items))
	for i, item := range h.items {
		res[len(h.items)-1-i] = item
	}
	return res
}

// NewServer creates a new Server.
func NewServer(dm *dataset.Manager, ss *secrets.Service, serveFolder string, verbose bool, autoSelectTb0 bool, localOnly bool, defaultDB string) *Server {
	t := sqliter.GetDefaultTemplates()
	sqliterCfg := sqliter.DefaultConfig()
	sqliterCfg.Verbose = verbose
	srv := &Server{
		dataManager:   dm,
		secrets:       ss,
		tableWriter:   sqliter.NewTableWriter(t, sqliterCfg),
		serveFolder:   serveFolder,
		verbose:       verbose,
		autoSelectTb0: autoSelectTb0,
		localOnly:     localOnly,
		defaultDB:     defaultDB,
		history:       NewRequestHistory(20),
	}
	// Log a warning if the configured serveFolder does not exist
	if serveFolder != "" {
		if _, err := os.Stat(serveFolder); err != nil {
			log.Printf("ServeFolder %s does not exist: %v", serveFolder, err)
		}
	}
	return srv
}

func (s *Server) log(format string, args ...interface{}) {
	if s.verbose {
		log.Printf(format, args...)
	}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /app/debug/env", s.handleDebugEnv)
	mux.HandleFunc("GET /app/credentials/manage", s.handleIndex)
	mux.HandleFunc("POST /app/credentials/manage", s.handleCreateCredential)
	mux.HandleFunc("POST /app/credentials/delete", s.handleDeleteCredential)
	mux.HandleFunc("GET /app/browse/{alias}/{path...}", s.handleBrowse)
	mux.HandleFunc("GET /app/view/{alias}/{path...}", s.handleView)
	mux.HandleFunc("GET /app/test/banquet/{path...}", s.handleBanquetTestDB)
	mux.HandleFunc("/app/credentials", s.handleCredentials)
	mux.HandleFunc("/app/", s.handleAppIndex)
	mux.HandleFunc("/", s.handleBanquet)

	if s.localOnly {
		return s.localOnlyMiddleware(mux)
	}
	return mux
}

func (s *Server) handleAppIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/app" && r.URL.Path != "/app/" {
		http.NotFound(w, r)
		return
	}
	//Make this view a sqlite db via sqliter
	// user_secrets.db
	// app.sqlite?
	// test
}

func (s *Server) localOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remote := r.RemoteAddr
		// r.RemoteAddr is usually host:port
		host, _, err := net.SplitHostPort(remote)
		if err != nil {
			host = remote
		}

		if host != "127.0.0.1" && host != "::1" && host != "localhost" {
			s.log("Blocking non-local request from: %s", host)
			http.Error(w, "Access denied: local only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleDebugEnv shows environment variables sorted.
// SECURITY WARNING: This endpoint exposes all environment variables, which may contain sensitive secrets.
// It should only be enabled in trusted environments or for debugging purposes.
func (s *Server) handleDebugEnv(w http.ResponseWriter, r *http.Request) {
	env := os.Environ()
	sort.Strings(env)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, e := range env {
		fmt.Fprintln(w, e)
	}
}

// handleCredentials stores cloud credentials and returns an alias.
func (s *Server) handleCredentials(w http.ResponseWriter, r *http.Request) {
	s.log("Incoming credentials request: %s %s from %s", r.Method, r.URL.String(), r.RemoteAddr)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var creds map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var alias string
	if a, ok := creds["alias"].(string); ok {
		alias = a
		delete(creds, "alias")
	}

	alias, err := s.secrets.StoreCredentials(alias, creds)
	if err != nil {
		log.Printf("Error storing credentials: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	s.log("Stored credentials with alias: %s", alias)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"alias": alias})
}

// handleBanquet handles the banquet URL requests.
func (s *Server) handleBanquet(w http.ResponseWriter, r *http.Request) {
	s.log("Incoming request: %s %s from %s", r.Method, r.URL.String(), r.RemoteAddr)

	if r.URL.Path == "/favicon.ico" {
		http.NotFound(w, r)
		return
	}

	bq, err := banquet.ParseNested(r.URL.String())
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing URL: %v", err), http.StatusBadRequest)
		return
	}
	s.log("BSCH:%s BDSP:%s TB:%s User:%v", bq.Scheme, bq.DataSetPath, bq.Table, bq.User)

	sourcePath := bq.DataSetPath

	// Workaround for banquet stripping scheme from path-based URLs
	// If request path contains "http:" or "https:" and sourcePath doesn't contain it.
	// And we don't have an alias (which would handle authentication).
	if bq.User == nil && (strings.Contains(r.URL.Path, "https:/") || strings.Contains(r.URL.Path, "http:/")) {
		// Use the raw request path (trimmed) as the source path
		rawPath := strings.TrimPrefix(r.URL.Path, "/")

		// Fix https:/ -> https:// if needed (browsers/proxies often merge slashes)
		if strings.Contains(rawPath, ":/") && !strings.Contains(rawPath, "://") {
			rawPath = strings.Replace(rawPath, ":/", "://", 1)
		}

		sourcePath = rawPath

		// Re-parse to extract user/alias from the corrected URL
		if newBq, err := banquet.ParseBanquet(sourcePath); err == nil {
			s.log("BURL re-parsed. User: %v", newBq.User)
			if newBq.User != nil {
				bq = newBq
			}
		} else {
			s.log("Failed to re-parse URL: %v", err)
		}
	} else if (sourcePath == "" || sourcePath == "/") && s.defaultDB != "" {
		// Serve DefaultDB at root
		sourcePath = s.defaultDB
	} else if (sourcePath == "" || sourcePath == "/") && s.serveFolder != "" {
		// Handle ServeFolder if configured and path is root
		sourcePath = s.serveFolder
	} else if sourcePath == "" || sourcePath == "/" {
		http.Error(w, "Welcome to Flight2! Usage: /<alias>@<source_url>/<query>", http.StatusOK)
		return
	} else {
		// Existing logic for cleaning sourcePath for non-URL paths
		sourcePath = strings.TrimPrefix(sourcePath, "/")
		if bq.Host != "" {
			sourcePath = bq.Host + "/" + sourcePath
		}

		// If serveFolder is set, and we don't have an alias or remote scheme, assume local path
		if s.serveFolder != "" && bq.User == nil && !strings.Contains(sourcePath, "://") && !strings.HasPrefix(sourcePath, "http") {
			joined := filepath.Join(s.serveFolder, sourcePath)
			// Prevent directory traversal
			rel, err := filepath.Rel(s.serveFolder, joined)
			if err != nil || strings.HasPrefix(rel, "..") {
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
			sourcePath = joined
		}

		// Fallback to DefaultDB if local file not found
		if s.defaultDB != "" && bq.User == nil && !strings.Contains(sourcePath, "://") && !strings.HasPrefix(sourcePath, "http") {
			if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
				// If it's not a file, it might be a table in the default DB
				if bq.Table == "" || bq.Table == "sqlite_master" {
					bq.Table = sourcePath
				}
				sourcePath = s.defaultDB
				s.log("Fallback: using table %q from default DB %s", bq.Table, sourcePath)
			}
		}
	}

	// Extract alias from userinfo
	alias := ""
	if bq.User != nil {
		alias = bq.User.Username()
	}

	var creds map[string]interface{}
	if alias != "" {
		s.log("Looking up credentials for alias: %s", alias)
		c, err := s.secrets.GetCredentials(alias)
		if err != nil {
			s.log("Error retrieving credentials for alias %s: %v", alias, err)
			http.Error(w, fmt.Sprintf("Error retrieving credentials for alias %s: %v", alias, err), http.StatusForbidden)
			return
		}
		creds = c
		s.log("Credentials found for alias: %s", alias)
	} else {
		creds = make(map[string]interface{})
		// Inject local type if it's a local path or DefaultDB
		isLocal := false
		if s.serveFolder != "" && strings.HasPrefix(sourcePath, s.serveFolder) {
			isLocal = true
		} else if s.defaultDB != "" && sourcePath == s.defaultDB {
			isLocal = true
		} else if !strings.Contains(sourcePath, "://") && !strings.HasPrefix(sourcePath, "http") {
			// Fallback for other local paths
			isLocal = true
		}

		if isLocal {
			creds["type"] = "local"
		}
	}

	// Fetch and convert
	s.log("Fetching and converting data from: %s", sourcePath)

	// S3 special handling: if using S3 credential and path is a URL, use only the path component.
	if t, ok := creds["type"].(string); ok && t == "s3" {
		if strings.HasPrefix(sourcePath, "http") {
			p := bq.Path
			p = strings.TrimPrefix(p, "/")
			s.log("Sanitized S3 path from URL: %s -> %s", sourcePath, p)
			sourcePath = p
		}
	}

	dbPath, err := s.dataManager.GetSQLiteDB(r.Context(), sourcePath, creds, alias)
	if err != nil {
		s.log("Error processing data: %v", err)

		// IMPROVED ERROR HANDLING
		// If fetch error and looking like a remote URL, suggest aliases
		if strings.Contains(err.Error(), "fetch error") || strings.Contains(err.Error(), "failed to create fs") {
			aliases, _ := s.secrets.ListAliases()
			recent := s.history.GetRecent()

			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Connection Error</title>
    <style>
        body { font-family: -apple-system, sans-serif; background: #0f172a; color: #f8fafc; padding: 2rem; max-width: 800px; margin: 0 auto; }
        .error { background: #fee2e2; color: #991b1b; padding: 1rem; border-radius: 8px; border: 1px solid #fca5a5; margin-bottom: 2rem; }
        .card { background: #1e293b; padding: 1.5rem; border-radius: 12px; border: 1px solid #334155; margin-bottom: 1.5rem; }
        h2 { margin-top: 0; color: #818cf8; font-size: 1.25rem; }
        ul { list-style: none; padding: 0; }
        li { margin-bottom: 0.5rem; }
        a { color: #38bdf8; text-decoration: none; word-break: break-all; }
        a:hover { text-decoration: underline; }
        code { background: #0f172a; padding: 0.2rem 0.4rem; border-radius: 4px; font-family: monospace; }
    </style>
</head>
<body>
    <h1>⚠️ Unable to Access Resource</h1>
    <div class="error">
        <strong>Error:</strong> %v
    </div>

    <div class="card">
        <h2>💡 Suggestion: Use a Credential Alias</h2>
        <p>This URL appears to be remote. Try prepending a configured alias to the URL:</p>
        <p>Format: <code>/&lt;alias&gt;@&lt;url&gt;</code></p>
        
        <h3>Available Aliases:</h3>
        <ul>`, err)
			for _, a := range aliases {
				// Construct a suggested link using the current path if it looks like a URL
				suggestion := fmt.Sprintf("/%s@%s", a, sourcePath)
				fmt.Fprintf(w, "<li><a href='%s'>%s</a> (e.g. <code>/%s@...</code>)</li>", suggestion, a, a)
			}
			fmt.Fprintf(w, `
        </ul>
    </div>

    <div class="card">
        <h2>🕒 Recent Successful Requests</h2>
        <ul>`)
			for _, url := range recent {
				fmt.Fprintf(w, "<li><a href='%s'>%s</a></li>", url, url)
			}
			fmt.Fprintf(w, `
        </ul>
    </div>
</body>
</html>`)
			return
		}

		http.Error(w, fmt.Sprintf("Error processing data: %v", err), http.StatusInternalServerError)
		return
	}
	dbPathLog := dbPath
	if s.defaultDB != "" && sourcePath == s.defaultDB {
		dbPathLog = "App.DB"
	}
	s.log("DB Ready: %s", dbPathLog)

	// Add to history if successful DB get (implies access worked)
	// We use the full original URL (or close to it)
	s.history.Add(r.URL.Path)
	// No need to defer remove dbPath here because it's cached.
	// But `writeTempFile` creates a temp file. The cache holds the bytes in memory (BigCache).
	// Wait, my `GetSQLiteDB` writes a temp file from cache every time.
	// So I SHOULD remove it after serving.
	defer os.Remove(dbPath)

	s.serveDatabase(w, r, bq, dbPath, bq.DataSetPath)
}

// handleBanquetTestDB serves the default database at /app/banquet/
func (s *Server) handleBanquetTestDB(w http.ResponseWriter, r *http.Request) {
	if s.defaultDB == "" {
		http.Error(w, "Default database not configured", http.StatusNotFound)
		return
	}

	bq, err := banquet.ParseNested(r.URL.String())
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing URL: %v", err), http.StatusBadRequest)
		return
	}

	table := r.PathValue("path")
	bq.DataSetPath = s.defaultDB
	bq.Table = table

	s.serveDatabase(w, r, bq, s.defaultDB, "/app/test/banquet")
}

func (s *Server) serveDatabase(w http.ResponseWriter, r *http.Request, bq *banquet.Banquet, dbPath string, dbUrlPath string) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error opening DB: %v", err), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if bq.Table == "sqlite_master" || bq.Table == "" {
		s.listTables(w, r, db, dbUrlPath)
	} else {
		s.queryTable(w, db, bq)
	}
}

func (s *Server) listTables(w http.ResponseWriter, r *http.Request, db *sql.DB, dbUrlPath string) {
	s.log("Listing tables for: %s", dbUrlPath)
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		tables = append(tables, name)
	}

	if s.autoSelectTb0 && len(tables) == 1 && tables[0] == "tb0" {
		target := strings.TrimSuffix(dbUrlPath, "/") + "/tb0"
		if !strings.HasPrefix(target, "/") {
			target = "/" + target
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	// Ensure absolute path
	if !strings.HasPrefix(dbUrlPath, "/") {
		dbUrlPath = "/" + dbUrlPath
	}

	// Use generic table for list of tables
	headers := []string{"Table Name", "Link"}
	s.tableWriter.StartHTMLTable(w, headers, "Flight2 Tables")

	for i, name := range tables {
		link := strings.TrimSuffix(dbUrlPath, "/") + "/" + name
		// Use raw HTML for link - requires row.html to use 'safe' filter
		linkHtml := fmt.Sprintf("<a href='%s'>%s</a>", link, name)
		s.tableWriter.WriteHTMLRow(w, i, []string{linkHtml, "Table"})
	}
	s.tableWriter.EndHTMLTable(w)
}

func (s *Server) queryTable(w http.ResponseWriter, db *sql.DB, bq *banquet.Banquet) {
	query := common.ConstructSQL(bq)
	s.log("Executing query: %s", query)

	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Query error: %v\nQuery: %s", err, query), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting columns: %v", err), http.StatusInternalServerError)
		return
	}

	s.tableWriter.StartHTMLTable(w, columns, bq.Table)

	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	rowCounter := 0

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			log.Println("Error scanning row:", err)
			continue
		}

		strValues := make([]string, len(columns))
		for i, val := range values {
			if val == nil {
				strValues[i] = "NULL"
			} else {
				strValues[i] = fmt.Sprintf("%v", val)
			}
		}

		s.tableWriter.WriteHTMLRow(w, rowCounter, strValues)
		rowCounter++
	}

	s.tableWriter.EndHTMLTable(w)
	s.log("Finished response")
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	aliases, err := s.secrets.ListAliases()
	if err != nil {
		http.Error(w, "Failed to list credentials", http.StatusInternalServerError)
		return
	}

	// Check for edit mode
	editAlias := r.URL.Query().Get("edit")
	var editType string
	var editConfig string
	if editAlias != "" {
		creds, err := s.secrets.GetCredentials(editAlias)
		if err == nil {
			if t, ok := creds["type"].(string); ok {
				editType = t
				delete(creds, "type")
			}
			b, _ := json.MarshalIndent(creds, "", "  ")
			editConfig = string(b)
		}
	}

	// Manual Page Header since StartTableList is gone
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Flight2 Remotes</title>
<link rel="stylesheet" href="/cssjs/default.css">
<style>
/* Add any page-specific overrides here */
</style>
</head>
<body>
`)
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
						<a href='/app/browse/%s/' class='btn btn-browse'>📂 Browse</a>
						<a href='/app/credentials/manage?edit=%s' class='btn btn-view'>✏️ Edit</a>
						<form action='/app/credentials/delete' method='POST' style='display:inline'>
							<input type='hidden' name='alias' value='%s'>
							<input type='submit' value='🗑️ Delete' class='btn btn-delete' onclick='return confirm("Are you sure?")'>
						</form>
					</td>
				</tr>`, alias, alias, alias, alias)
		}
	}

	formTitle := "➕ Add New Remote"
	submitText := "Add Remote"
	if editAlias != "" {
		formTitle = "✏️ Edit Remote: " + editAlias
		submitText = "Update Credential"
	}

	fmt.Fprintf(w, `
					</tbody>
				</table>
			</section>

			<hr class="separator">

			<section class="add-remote">
				<h2>%s</h2>
				<form action="/app/credentials/manage" method="POST" class="credential-form">
					<div class="form-group">
						<label>Alias Name</label>
						<input type="text" name="alias" required value="%s" placeholder="e.g., my-s3-bucket" %s>
						<input type="hidden" name="original_alias" value="%s">
						%s
					</div>
					<div class="form-group">
						<label>Provider Type</label>
						<select name="type" required id="provider-select">
							<option value="s3" %s>Cloudflare R2 / AWS S3</option>
							<option value="drive" %s>Google Drive</option>
							<option value="dropbox" %s>Dropbox</option>
							<option value="sftp" %s>SFTP</option>
							<option value="azureblob" %s>Azure Blob Storage</option>
							<option value="b2" %s>Backblaze B2</option>
							<option value="box" %s>Box</option>
							<option value="http" %s>HTTP/HTTPS</option>
							<option value="local" %s>Local Filesystem</option>
						</select>
						<div id="cloudflare-link" style="display: none; margin-top: 0.5rem; font-size: 0.85rem;">
							<a href="https://dash.cloudflare.com/?to=/:account/r2/api-tokens" target="_blank" style="color: #f6821f; font-weight: 600;">
								☁️ Click here to get your Cloudflare R2 API Tokens
							</a>
						</div>
					</div>
					<div class="form-group">
						<label>Configuration (JSON Key-Value Pairs)</label>
						<textarea name="config" rows="8" placeholder='{"access_key_id": "...", "secret_access_key": "...", "region": "us-east-1"}'>%s</textarea>
						<small>Refer to rclone documentation for each provider's required fields.</small>
					</div>
					<div style="display:flex; gap:1rem;">
						<button type="submit" class="btn btn-primary">%s</button>
						%s
					</div>
				</form>
				<script>
					const select = document.getElementById('provider-select');
					const div = document.getElementById('cloudflare-link');
					function updateLink() {
						div.style.display = select.value === 's3' ? 'block' : 'none';
					}
					select.onchange = updateLink;
					updateLink();
				</script>
			</section>
		</div>
	`, formTitle, editAlias,
		func() string {
			// Alias is now editable!
			return ""
		}(),
		editAlias, // For original_alias hidden input
		func() string {
			if editAlias != "" {
				return "<small style='color:#94a3b8'>You can rename this alias.</small>"
			}
			return ""
		}(),
		func() string {
			if editType == "s3" {
				return "selected"
			}
			return ""
		}(),
		func() string {
			if editType == "drive" {
				return "selected"
			}
			return ""
		}(),
		func() string {
			if editType == "dropbox" {
				return "selected"
			}
			return ""
		}(),
		func() string {
			if editType == "sftp" {
				return "selected"
			}
			return ""
		}(),
		func() string {
			if editType == "azureblob" {
				return "selected"
			}
			return ""
		}(),
		func() string {
			if editType == "b2" {
				return "selected"
			}
			return ""
		}(),
		func() string {
			if editType == "box" {
				return "selected"
			}
			return ""
		}(),
		func() string {
			if editType == "http" {
				return "selected"
			}
			return ""
		}(),
		func() string {
			if editType == "local" {
				return "selected"
			}
			return ""
		}(),
		editConfig, submitText,
		func() string {
			if editAlias != "" {
				return "<a href='/app/credentials/manage' class='btn' style='background:#334155; color:white;'>Cancel</a>"
			}
			return ""
		}())
	fmt.Fprintf(w, "</body></html>")
}

func (s *Server) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	alias := r.FormValue("alias")
	originalAlias := r.FormValue("original_alias")
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

	// Rename: if originalAlias is set and different, delete the old one
	if originalAlias != "" && originalAlias != alias {
		s.log("Renaming credential: %s -> %s", originalAlias, alias)
		if err := s.secrets.DeleteCredentials(originalAlias); err != nil {
			s.log("Warning: failed to delete old alias %s during rename: %v", originalAlias, err)
			// Don't fail the request, the new one is saved. just log it.
		}
	}

	http.Redirect(w, r, "/app/credentials/manage", http.StatusSeeOther)

	// Test auth in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s.log("🔍 [AUTH TEST] Verifying remote '%s'...", alias)
		_, err := dataset_source.ListEntries(ctx, "", creds)
		if err != nil {
			s.log("❌ [AUTH TEST] Remote '%s' FAILED: %v", alias, err)
		} else {
			s.log("✅ [AUTH TEST] Remote '%s' is AUTHENTICATED", alias)
		}
	}()
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

	http.Redirect(w, r, "/app/credentials/manage", http.StatusSeeOther)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")
	relPath := r.PathValue("path")

	creds, err := s.secrets.GetCredentials(alias)
	if err != nil {
		http.Error(w, "Remote not found", http.StatusNotFound)
		return
	}

	s.listingLogic(w, r, alias, relPath, creds)
}

func (s *Server) listingLogic(w http.ResponseWriter, r *http.Request, alias string, relPath string, creds map[string]interface{}) {
	entries, err := dataset_source.ListEntries(r.Context(), relPath, creds)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list entries: %v", err), http.StatusInternalServerError)
		return
	}

	// Sort entries: directories first, then files
	sort.Slice(entries, func(i, j int) bool {
		iIsDir := entries[i].IsDir()
		jIsDir := entries[j].IsDir()
		if iIsDir && !jIsDir {
			return true
		}
		if !iIsDir && jIsDir {
			return false
		}
		return entries[i].Name() < entries[j].Name()
	})

	// Manual Page Header
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Browse - %s</title>
<link rel="stylesheet" href="/cssjs/default.css">
</head>
<body>
`, alias)
	fmt.Fprintf(w, "<div class='container'>")
	fmt.Fprintf(w, "<h2>📂 Browsing: %s <span style='color:var(--text-muted); font-size: 0.9rem; margin-left: 0.5rem;'>/%s</span></h2>", alias, relPath)

	// Determine base path for links
	basePath := "/app/browse/" + alias
	viewPath := "/app/view/" + alias

	cols := []string{"Type", "Name", "Size", "Modified", "Actions"}
	s.tableWriter.StartHTMLTable(w, cols, "")

	// Add ".." link if not at root
	if relPath != "" && relPath != "." {
		parent := path.Dir(strings.TrimSuffix(relPath, "/"))
		if parent == "." {
			parent = ""
		}
		fmt.Fprintf(w, "<tr><td><span class='badge badge-folder'>📁</span></td><td><a href='%s/%s' style='font-weight:600;'>.. [ Parent Directory ]</a></td><td>-</td><td>-</td><td>-</td></tr>", basePath, parent)
	}

	for _, entry := range entries {
		name := entry.Name()
		fullPath := path.Join(relPath, name)

		var icon, sizeStr, modified, actions string
		if entry.IsDir() {
			icon = "<span class='badge badge-folder'>📁</span>"
			sizeStr = "-"
			modified = entry.ModTime().Format("2006-01-02 15:04:05")
			actions = fmt.Sprintf("<a href='%s/%s' class='btn btn-browse'>📂 Open</a>", basePath, fullPath)
		} else {
			icon = "<span class='badge badge-file'>📄</span>"
			sizeStr = formatSize(entry.Size())
			modified = entry.ModTime().Format("2006-01-02 15:04:05")
			// For files, we offer "View" and if it's a known database type, "Query"
			ext := strings.ToLower(path.Ext(fullPath))
			queryAction := ""
			if ext == ".db" || ext == ".sqlite" || ext == ".sqlite3" || ext == ".csv" || ext == ".xlsx" || ext == ".json" {
				queryAction = fmt.Sprintf("<a href='/%s@%s/' class='btn btn-primary'>📊 Query</a>", alias, fullPath)
			}
			actions = fmt.Sprintf("%s <a href='%s/%s' target='_blank' class='btn btn-view'>👁️ View</a>", queryAction, viewPath, fullPath)
		}

		fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>", icon, name, sizeStr, modified, actions)
	}

	s.tableWriter.EndHTMLTable(w)
	fmt.Fprintf(w, "</div>")
	fmt.Fprintf(w, "</body></html>")
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")
	relPath := r.PathValue("path")

	creds, err := s.secrets.GetCredentials(alias)
	if err != nil {
		http.Error(w, "Remote not found", http.StatusNotFound)
		return
	}

	rc, err := dataset_source.GetFileStream(r.Context(), relPath, creds)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open file: %v", err), http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", path.Base(relPath)))
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
