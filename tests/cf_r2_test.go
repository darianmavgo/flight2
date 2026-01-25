package tests

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flight2/internal/dataset"
	"flight2/internal/secrets"

	"flag"
	"flight2/internal/config"

	"github.com/darianmavgo/banquet"
	"github.com/darianmavgo/sqliter/sqliter"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/rclone/rclone/backend/all"
)

var useTempPaths = flag.Bool("use-temp-paths", false, "Use temporary paths for tests instead of config.hcl")

func getTestConfig(t *testing.T) (*config.Config, func()) {
	if *useTempPaths {
		// Use test_output subdirectory for temp paths
		tmpDir := filepath.Join("..", "test_output", "tmp_run_"+time.Now().Format("20060102150405"))
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			t.Fatalf("Failed to create temp dir in test_output: %v", err)
		}

		// Copy real credentials to temp dir for reading
		testOutputDir, _ := filepath.Abs("../test_output")
		realSecretsDB := filepath.Join("..", "user_secrets.db")
		realSecretKey := filepath.Join("..", ".secret.key")
		testSecretsDB := filepath.Join(testOutputDir, "user_settings.db")
		testSecretKey := filepath.Join(testOutputDir, "secret.key")

		if data, err := os.ReadFile(realSecretsDB); err == nil {
			os.WriteFile(testSecretsDB, data, 0644)
		}
		if data, err := os.ReadFile(realSecretKey); err == nil {
			os.WriteFile(testSecretKey, data, 0644)
		}

		return &config.Config{
				Port:          "0",
				UserSecretsDB: testSecretsDB,
				SecretKey:     testSecretKey,
				ServeFolder:   tmpDir,
				CacheDir:      filepath.Join(tmpDir, "cache"),
				Verbose:       true,
				DefaultDB:     filepath.Join(tmpDir, "app.sqlite"),
				AutoSelectTb0: true,
				LocalOnly:     true,
			}, func() {
				os.RemoveAll(tmpDir) // Clean up this specific run folder
			}
	}

	// Load real config
	cfg, err := config.LoadConfig("../config.hcl")
	if err != nil {
		// Fallback to default mostly
		t.Logf("Failed to load ../config.hcl: %v. Using defaults with relative paths.", err)
		cfg = &config.Config{
			Port:          "0",
			UserSecretsDB: "user_secrets.db",
			SecretKey:     ".secret.key",
			CacheDir:      "cache",
			Verbose:       true,
			DefaultDB:     "app.sqlite",
		}
	}

	// Redirect artifacts to ../test_output
	testOutputDir, _ := filepath.Abs("../test_output")
	if err := os.MkdirAll(testOutputDir, 0755); err != nil {
		t.Logf("Failed to create test_output dir: %v", err)
	}

	// Force rclone to use test_output
	os.Setenv("RCLONE_CACHE_DIR", filepath.Join(testOutputDir, "test_rclone_cache"))

	// Copy real credentials to test_output for reading, then write to user_settings.db
	realSecretsDB := filepath.Join("..", "user_secrets.db")
	realSecretKey := filepath.Join("..", ".secret.key")
	testSecretsDB := filepath.Join(testOutputDir, "user_settings.db")
	testSecretKey := filepath.Join(testOutputDir, "secret.key")

	if data, err := os.ReadFile(realSecretsDB); err == nil {
		os.WriteFile(testSecretsDB, data, 0644)
	} else {
		t.Logf("Warning: Real user_secrets.db not found at %s: %v", realSecretsDB, err)
	}

	if data, err := os.ReadFile(realSecretKey); err == nil {
		os.WriteFile(testSecretKey, data, 0644)
	}

	cfg.UserSecretsDB = testSecretsDB
	cfg.SecretKey = testSecretKey
	cfg.CacheDir = filepath.Join(testOutputDir, "cache")
	cfg.DefaultDB = filepath.Join(testOutputDir, "app.sqlite")

	return cfg, func() {}
}

// TestCloudflareR2EndToEnd performs an end-to-end test of the Cloudflare R2 integration.
// It verifies that we can:
// 1. Configure Cloudflare R2 credentials (mocked/aliased).
// 2. Parse a banquet URL pointing to R2.
// 3. Automatically fetch and convert the remote CSV to local SQLite.
// 4. Query the resulting SQLite database.
//
// URL: https://r2-auth@d8dc30936fb37cbd74552d31a709f6cf.r2.cloudflarestorage.com/test-mksqlite/sample_data/21mb.csv
// Credential Alias: r2-auth
// Type: Integration Test
func TestCloudflareR2EndToEnd(t *testing.T) {
	// This test requires internet access and valid credentials if we weren't mocking.
	// However, since we are testing the logic assuming Rclone works, we can try to rely on real Rclone if configured,
	// or skip if no credentials.
	// For this specific test URL, the bucket is likely private.
	// Use the hardcoded credentials from the prompt for the "r2-auth" alias.

	// 1. Setup Credentials
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	endpoint := os.Getenv("R2_ENDPOINT")

	if accessKey == "" || secretKey == "" || endpoint == "" {
		t.Skip("R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, or R2_ENDPOINT not set. Skipping R2 end-to-end test.")
	}

	creds := map[string]interface{}{
		"provider":          "Cloudflare",
		"access_key_id":     accessKey,
		"secret_access_key": secretKey,
		"endpoint":          endpoint,
		"region":            "auto",
		"chunk_size":        "5Mi",
		"copy_cutoff":       "5Mi",
		"type":              "s3",
	}

	// 1. Setup Configuration
	cfg, _ := getTestConfig(t)

	// Ensure cache dir exists for real config
	if cfg.CacheDir != "" {
		if !strings.HasPrefix(cfg.CacheDir, "/") && !strings.HasPrefix(cfg.CacheDir, ".") {
			// It was resolved?
		} else {
			// Ensure it exists
			os.MkdirAll(cfg.CacheDir, 0755)
		}
	}

	secretsService, err := secrets.NewService(cfg.UserSecretsDB, cfg.SecretKey)
	if err != nil {
		t.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	alias := "r2-auth"
	_, err = secretsService.StoreCredentials(alias, creds)
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	// 2. Parse Banquet URL
	targetURL := "https://r2-auth@d8dc30936fb37cbd74552d31a709f6cf.r2.cloudflarestorage.com/test-mksqlite/sample_data/21mb.csv"

	bq, err := banquet.ParseBanquet(targetURL)
	if err != nil {
		t.Fatalf("Failed to parse banquet URL: %v", err)
	}

	if bq.User == nil || bq.User.Username() != alias {
		t.Fatalf("Expected alias '%s', got '%v'", alias, bq.User)
	}

	log.Printf("Parsed URL: Scheme=%s, Alias=%s, Host=%s, Path=%s", bq.Scheme, bq.User.Username(), bq.Host, bq.Path)

	sourcePath := bq.Path
	if len(sourcePath) > 0 && sourcePath[0] == '/' {
		sourcePath = sourcePath[1:]
	}

	// 3. Convert to SQLite
	// 3. Convert to SQLite
	dm, err := dataset.NewManager(cfg.Verbose, cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create data manager: %v", err)
	}

	ctx := context.Background()

	storedCreds, err := secretsService.GetCredentials(alias)
	if err != nil {
		t.Fatalf("Failed to retrieve credentials: %v", err)
	}

	if _, ok := storedCreds["type"]; !ok {
		storedCreds["type"] = "s3"
	}

	log.Printf("Fetching and converting: %s", sourcePath)
	dbPath, err := dm.GetSQLiteDB(ctx, sourcePath, storedCreds, alias)
	if err != nil {
		t.Fatalf("Failed to convert to SQLite: %v", err)
	}

	log.Printf("Conversion successful, DB at: %s", dbPath)

	// 4. Query resulting SQLite file
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite DB: %v", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT * FROM tb0")
	if err != nil {
		t.Fatalf("Failed to query tb0: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("Failed to get columns: %v", err)
	}
	log.Printf("Columns: %v", cols)

	rowCount := 0
	for rows.Next() {
		rowCount++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Error iterating rows: %v", err)
	}

	log.Printf("Successfully queried %d rows from tb0", rowCount)

	if rowCount == 0 {
		t.Log("Warning: Table tb0 is empty")
	}
}

func StartTestServer(t *testing.T, secretsService *secrets.Service) (string, func()) {
	// Use cfg for templates if not temp
	cfg, _ := getTestConfig(t)
	// We handle cleanup here because this function returns a cleanup func too.
	// But `getTestConfig` cleanup does nothing for real config (or closes stuff?)
	// Actually it returns func(){}.
	// If it returned cleanup for temp paths, we should incorporate it.

	// Use test_output/test_templates
	tmpDir := filepath.Join("..", "test_output", "test_templates")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create test templates dir: %v", err)
	}
	createTestTemplates(tmpDir)

	if !*useTempPaths {
		// Log usage
	}
	tpl := sqliter.LoadTemplates(tmpDir)

	dm, err := dataset.NewManager(cfg.Verbose, cfg.CacheDir)
	if err != nil {
		t.Fatalf("Failed to create data manager: %v", err)
	}

	server := &TestServerWrapper{
		dm: dm,
		ss: secretsService,
		tw: sqliter.NewTableWriter(tpl, sqliter.DefaultConfig()),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	baseUrl := fmt.Sprintf("http://127.0.0.1:%d", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handleRequest)

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	return baseUrl, func() {
		t.Logf("Leaving server running on %s", baseUrl)
	}
}

type TestServerWrapper struct {
	dm *dataset.Manager
	ss *secrets.Service
	tw *sqliter.TableWriter
}

func (s *TestServerWrapper) handleRequest(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimPrefix(r.URL.Path, "/")
	if strings.Contains(rawPath, ":/") && !strings.Contains(rawPath, "://") {
		rawPath = strings.Replace(rawPath, ":/", "://", 1)
	}

	bq, err := banquet.ParseBanquet(rawPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	alias := bq.User.Username()
	srcPath := bq.Path
	if len(srcPath) > 0 && srcPath[0] == '/' {
		srcPath = srcPath[1:]
	}

	creds, err := s.ss.GetCredentials(alias)
	if err != nil {
		http.Error(w, "Creds not found", 500)
		return
	}
	if _, ok := creds["type"]; !ok {
		creds["type"] = "s3"
	}

	dbPath, err := s.dm.GetSQLiteDB(r.Context(), srcPath, creds, alias)
	if err != nil {
		http.Error(w, "Conversion failed: "+err.Error(), 500)
		return
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT * FROM tb0 LIMIT 50")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()

	s.tw.StartHTMLTable(w, cols, "Test Table")

	rowData := make([]interface{}, len(cols))
	rowPtrs := make([]interface{}, len(cols))
	for i := range rowData {
		rowPtrs[i] = &rowData[i]
	}

	rowCounter := 0
	for rows.Next() {
		rows.Scan(rowPtrs...)
		strRow := make([]string, len(cols))
		for i, val := range rowData {
			if val == nil {
				strRow[i] = ""
			} else {
				strRow[i] = fmt.Sprintf("%v", val)
			}
		}
		s.tw.WriteHTMLRow(w, rowCounter, strRow)
		rowCounter++
	}
	s.tw.EndHTMLTable(w)
}

func createTestTemplates(dir string) {
	os.WriteFile(filepath.Join(dir, "head.html"), []byte(`<html><head><title>{{.Title}}</title></head><body><table><thead><tr>{{range .Headers}}<th>{{.}}</th>{{end}}</tr></thead><tbody>`), 0644)
	os.WriteFile(filepath.Join(dir, "foot.html"), []byte(`</table></body></html>`), 0644)
	os.WriteFile(filepath.Join(dir, "row.html"), []byte(`<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>`), 0644)
	os.WriteFile(filepath.Join(dir, "list_head.html"), []byte(`<html><body><ul>`), 0644)
	os.WriteFile(filepath.Join(dir, "list_foot.html"), []byte(`</ul></body></html>`), 0644)
	os.WriteFile(filepath.Join(dir, "list_item.html"), []byte(`<li><a href="{{.URL}}">{{.Name}}</a></li>`), 0644)
}

// TestCloudflareR2Browser performs a browser-based E2E test.
// Type: E2E Test (Browser)
func TestCloudflareR2Browser(t *testing.T) {
	// 1. Setup Credentials
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	endpoint := os.Getenv("R2_ENDPOINT")

	if accessKey == "" || secretKey == "" || endpoint == "" {
		t.Skip("R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, or R2_ENDPOINT not set. Skipping R2 browser test.")
	}

	creds := map[string]interface{}{
		"provider":          "Cloudflare",
		"access_key_id":     accessKey,
		"secret_access_key": secretKey,
		"endpoint":          endpoint,
		"region":            "auto",
		"chunk_size":        "5Mi",
		"copy_cutoff":       "5Mi",
		"type":              "s3",
	}

	cfg, _ := getTestConfig(t)

	secretsService, err := secrets.NewService(cfg.UserSecretsDB, cfg.SecretKey)
	if err != nil {
		t.Fatalf("Failed to initialize secrets service: %v", err)
	}
	defer secretsService.Close()

	alias := "r2-auth"
	_, err = secretsService.StoreCredentials(alias, creds)
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	serverURL, _ := StartTestServer(t, secretsService)

	l := launcher.New().Headless(true)
	u, err := l.Launch()
	if err != nil {
		t.Logf("Failed to launch browser: %v. Attempting to use system browser...", err)
		u = launcher.NewUserMode().MustLaunch()
	}

	browser := rod.New().ControlURL(u).MustConnect()

	start := time.Now()

	banquetURL := "https://r2-auth@d8dc30936fb37cbd74552d31a709f6cf.r2.cloudflarestorage.com/test-mksqlite/sample_data/21mb.csv"
	visitURL := fmt.Sprintf("%s/%s", serverURL, banquetURL)

	t.Logf("Navigating to %s", visitURL)

	page := browser.MustPage(visitURL)

	err = page.Timeout(10*time.Second).WaitElementsMoreThan("tr", 10)
	if err != nil {
		t.Fatalf("Failed to load 10 rows within timeout: %v", err)
	}

	elapsed := time.Since(start)
	t.Logf("Page loaded 10+ rows in %v", elapsed)

	if elapsed > 3*time.Second {
		t.Fatalf("Performance Test Failed: Took %v to load 10 rows (limit 3s)", elapsed)
	}

	content, err := page.HTML()
	if err != nil {
		t.Fatalf("Failed to get page HTML: %v", err)
	}
	if !strings.Contains(content, "id") || !strings.Contains(content, "email") {
		t.Errorf("Page content missing expected headers")
	}
}

// TestCloudflareR2IntegrationBinary performs an integration test using the actual compiled executable associated with flight2.
