package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	dataManager *data.Manager
	secrets     *secrets.Service
	tableWriter *sqliter.TableWriter
	templateDir string
	serveFolder string
}

// NewServer creates a new Server.
func NewServer(dm *data.Manager, ss *secrets.Service, templateDir string, serveFolder string) *Server {
	t := sqliter.LoadTemplates(templateDir)
	return &Server{
		dataManager: dm,
		secrets:     ss,
		tableWriter: sqliter.NewTableWriter(t),
		templateDir: templateDir,
		serveFolder: serveFolder,
	}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/credentials", s.handleCredentials)
	mux.HandleFunc("/credentials/delete", s.handleDeleteCredential)
	mux.HandleFunc("/", s.handleBanquet)
	return mux
}

// handleCredentials stores cloud credentials and returns an alias.
func (s *Server) handleCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.renderCredentials(w)
		return
	}

	if r.Method == http.MethodPost {
		// Handle JSON input (legacy/API)
		if r.Header.Get("Content-Type") == "application/json" {
			var creds map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
			// Check if alias is provided in JSON (optional extension)
			alias := ""
			if a, ok := creds["alias"].(string); ok {
				alias = a
				delete(creds, "alias") // Remove alias from stored creds
			}

			alias, err := s.secrets.StoreCredentials(alias, creds)
			if err != nil {
				log.Printf("Error storing credentials: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"alias": alias})
			return
		}

		// Handle Form input
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		alias := r.FormValue("alias")
		keys := r.Form["keys[]"]
		values := r.Form["values[]"]

		creds := make(map[string]interface{})
		for i, k := range keys {
			if i < len(values) && k != "" {
				creds[k] = values[i]
			}
		}

		if len(creds) == 0 {
			http.Error(w, "No credentials provided", http.StatusBadRequest)
			return
		}

		_, err := s.secrets.StoreCredentials(alias, creds)
		if err != nil {
			log.Printf("Error storing credentials: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/credentials", http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	alias := r.FormValue("alias")
	if alias == "" {
		http.Error(w, "Alias required", http.StatusBadRequest)
		return
	}

	if err := s.secrets.DeleteCredentials(alias); err != nil {
		log.Printf("Error deleting credentials: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/credentials", http.StatusSeeOther)
}

func (s *Server) renderCredentials(w http.ResponseWriter) {
	aliases, err := s.secrets.ListAliases()
	if err != nil {
		log.Printf("Error listing aliases: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles(
		filepath.Join(s.templateDir, "credentials.html"),
		filepath.Join(s.templateDir, "head.html"),
		filepath.Join(s.templateDir, "foot.html"),
	)
	if err != nil {
		log.Printf("Error parsing templates: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Aliases []string
	}{
		Aliases: aliases,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
	}
}

// handleBanquet handles the banquet URL requests.
func (s *Server) handleBanquet(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		http.NotFound(w, r)
		return
	}

	bq, err := banquet.ParseNested(r.URL.String())
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing URL: %v", err), http.StatusBadRequest)
		return
	}

	sourcePath := bq.DataSetPath

	// Handle ServeFolder if configured and path is root
	if (sourcePath == "" || sourcePath == "/") && s.serveFolder != "" {
		sourcePath = s.serveFolder
	} else if sourcePath == "" || sourcePath == "/" {
		http.Error(w, "Welcome to Flight2! Usage: /<alias>@<source_url>/<query>", http.StatusOK)
		return
	} else {
		// Existing logic for cleaning sourcePath
		sourcePath = strings.TrimPrefix(sourcePath, "/")
		if bq.Host != "" {
			sourcePath = bq.Host + "/" + sourcePath
		}

		// Workaround for banquet stripping scheme from path-based URLs
		// If request path contains "http:" or "https:" and sourcePath doesn't contain it.
		// And we don't have an alias (which would handle authentication).
		if (strings.Contains(r.URL.Path, "https:/") || strings.Contains(r.URL.Path, "http:/")) && !strings.Contains(sourcePath, "http") {
			if bq.User == nil {
				// Use the raw request path (trimmed) as the source path
				// This might include the table, but since banquet failed to preserve scheme,
				// we prioritize getting the source right.
				// We assume the whole path is the source URL.
				sourcePath = strings.TrimPrefix(r.URL.Path, "/")
			}
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
		c, err := s.secrets.GetCredentials(alias)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error retrieving credentials for alias %s: %v", alias, err), http.StatusForbidden)
			return
		}
		creds = c
	} else {
		creds = make(map[string]interface{})
		// If we are serving from local folder, inject local type
		if s.serveFolder != "" && strings.HasPrefix(sourcePath, s.serveFolder) {
			creds["type"] = "local"
		}
	}

	// Fetch and convert
	dbPath, err := s.dataManager.GetSQLiteDB(r.Context(), sourcePath, creds, alias)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error processing data: %v", err), http.StatusInternalServerError)
		return
	}
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
		s.listTables(w, db, bq.DataSetPath)
	} else {
		s.queryTable(w, db, bq)
	}
}

func (s *Server) listTables(w http.ResponseWriter, db *sql.DB, dbUrlPath string) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Ensure absolute path
	if !strings.HasPrefix(dbUrlPath, "/") {
		dbUrlPath = "/" + dbUrlPath
	}

	sqliter.StartTableList(w)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		// Link format needs to append the table name to the current URL.
		// If dbUrlPath is the current URL, we just append /tablename?
		// But we need to be careful about existing query params?
		// Banquet handles path/table.
		// If current url is /path, new url is /path/table

		sqliter.WriteTableLink(w, name, strings.TrimSuffix(dbUrlPath, "/")+"/"+name)
	}
	sqliter.EndTableList(w)
}

func (s *Server) queryTable(w http.ResponseWriter, db *sql.DB, bq *banquet.Banquet) {
	query := common.ConstructSQL(bq)
	log.Printf("Executing query: %s", query)

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

	s.tableWriter.StartHTMLTable(w, columns)

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
}
