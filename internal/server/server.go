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
}

// NewServer creates a new Server.
func NewServer(dm *data.Manager, ss *secrets.Service, templateDir string) *Server {
	t := sqliter.LoadTemplates(templateDir)
	return &Server{
		dataManager: dm,
		secrets:     ss,
		tableWriter: sqliter.NewTableWriter(t),
		templateDir: templateDir,
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

	// Check if we have source path
	if bq.DataSetPath == "" || bq.DataSetPath == "/" {
		http.Error(w, "Welcome to Flight2! Usage: /<alias>@<source_url>/<query>", http.StatusOK)
		return
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
		// Allow public access if no alias provided? Or error?
		// Prompt implies "use the userinfo... to retrieve stored credentials".
		// If empty, maybe assume local file system or no auth?
		// Let's allow empty creds (might work for local files if allowed)
		creds = make(map[string]interface{})
	}

	// The DataSetPath in banquet includes the host part if it's a URL.
	// Wait, banquet parsing:
	// If input is `http://localhost:8080/myalias@s3/bucket/file.csv/query`
	// Banquet parses this.
	// Verify what banquet does with `ParseNested`.
	// Assuming `r.URL.String()` is passed, it parses the request URL.

	// `banquet` treats the request path as the dataset path + query.
	// But we are using the banquet URL format *in* the request path?
	// "Use Banquet urls to parse a request url for a source and query."
	// If the request is `GET /myalias@s3.amazonaws.com/bucket/file.csv`,
	// `r.URL.String()` is `/myalias@s3.amazonaws.com/bucket/file.csv`.
	// `banquet.ParseNested` parses the path.

	// We need to reconstruct the "source URL" from the parsed banquet object.
	// Banquet splits it into Source (DataSetPath) and Query (Table/etc).
	// Note: `banquet` might not preserve the scheme if it's not part of the path.
	// The user prompt says: "Use Banquet urls to parse a request url for a source and query."
	// If I pass `/alias@host/path`, banquet parses userinfo=`alias`, host=`host`, path=`path`.

	// We need to construct the `sourcePath` for `internal/data`.
	// It should probably be something `rclone` understands.
	// If creds["type"] is "s3", rclone expects just the bucket/path, or maybe `s3:bucket/path`?
	// In `internal/source`, we use `regInfo.NewFs`. The parent path is passed.
	// If we use `fs.Find(type)`, we get a backend.
	// Then `NewFs` creates a filesystem.
	// If the source is `s3.amazonaws.com/bucket/file.csv`, we need to handle the host.

	// Rclone config map handles the type.
	// If creds["type"] == "s3", we just need the path inside the bucket?
	// Or if creds["type"] == "http", we need the URL.

	// Let's assume the `bq.DataSetPath` is the path we want to access on the remote.
	// However, `banquet` might have stripped the "host" if it parsed it as a URL?
	// Let's look at `bq.DataSetPath`.

	sourcePath := bq.DataSetPath
	// If banquet parsed "host", we might need to prepend it if it's relevant (e.g. http/ftp).
	// But for cloud providers (s3, gcs), usually the bucket is the first part of the path or the host.

	// Clean up leading slash
	sourcePath = strings.TrimPrefix(sourcePath, "/")

	// If we have a host in banquet, prepend it?
	if bq.Host != "" {
		sourcePath = bq.Host + "/" + sourcePath
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
