package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flight2/internal/data"
	"flight2/internal/secrets"

	"github.com/darianmavgo/banquet"
	"github.com/darianmavgo/sqliter/pkg/common"
	"github.com/darianmavgo/sqliter/sqliter"
	_ "github.com/mattn/go-sqlite3"
)

// Server handles serving data.
type Server struct {
	dataManager   *data.Manager
	secrets       *secrets.Service
	tableWriter   *sqliter.TableWriter
	templateDir   string
	serveFolder   string
	verbose       bool
	autoSelectTb0 bool
}

// NewServer creates a new Server.
func NewServer(dm *data.Manager, ss *secrets.Service, templateDir string, serveFolder string, verbose bool, autoSelectTb0 bool) *Server {
	if _, err := os.Stat(templateDir); err != nil {
		log.Printf("TemplateDir %s does not exist: %v", templateDir, err)
	}
	t := sqliter.LoadTemplates(templateDir)
	srv := &Server{
		dataManager:   dm,
		secrets:       ss,
		tableWriter:   sqliter.NewTableWriter(t, sqliter.DefaultConfig()),
		templateDir:   templateDir,
		serveFolder:   serveFolder,
		verbose:       verbose,
		autoSelectTb0: autoSelectTb0,
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
	mux.HandleFunc("/debug/env", s.handleDebugEnv)
	mux.HandleFunc("/credentials", s.handleCredentials)
	mux.HandleFunc("/", s.handleBanquet)
	return mux
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
	s.log("Parsed URL: source=%s, table=%s, user=%v", bq.DataSetPath, bq.Table, bq.User)

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
			s.log("Re-parsed URL. User: %v", newBq.User)
			if newBq.User != nil {
				bq = newBq
			}
		} else {
			s.log("Failed to re-parse URL: %v", err)
		}
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
		// If we are serving from local folder, inject local type
		if s.serveFolder != "" && strings.HasPrefix(sourcePath, s.serveFolder) {
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
		http.Error(w, fmt.Sprintf("Error processing data: %v", err), http.StatusInternalServerError)
		return
	}
	s.log("Database ready at: %s", dbPath)
	// No need to defer remove dbPath here because it's cached.
	// But `writeTempFile` creates a temp file. The cache holds the bytes in memory (BigCache).
	// Wait, my `GetSQLiteDB` writes a temp file from cache every time.
	// So I SHOULD remove it after serving.
	defer os.Remove(dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error opening DB: %v", err), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if bq.Table == "sqlite_master" || bq.Table == "" {
		s.listTables(w, r, db, bq.DataSetPath)
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

	s.tableWriter.StartTableList(w, "Flight2 Tables")
	for _, name := range tables {
		sqliter.WriteTableLink(w, name, strings.TrimSuffix(dbUrlPath, "/")+"/"+name, "Table")
	}
	s.tableWriter.EndTableList(w)
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

		s.tableWriter.WriteHTMLRow(w, strValues)
	}

	s.tableWriter.EndHTMLTable(w)
	s.log("Finished response")
}
