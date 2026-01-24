package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"flight2/internal/dataset"
	"flight2/internal/secrets"
	"flight2/internal/server"
)

// TestLocalOnlyRestriction verifies the local-only middleware logic.
// Type: Unit Test
func TestLocalOnlyRestriction(t *testing.T) {
	// Setup mock services

	// Use test_output/cache
	cacheDir := filepath.Join("..", "test_output", "cache")
	os.MkdirAll(cacheDir, 0755)
	dm, _ := dataset.NewManager(false, cacheDir)
	// We don't need a real secrets service for this test as we only care about the middleware
	ss, _ := secrets.NewService(":memory:", "test-key")
	defer ss.Close()

	tests := []struct {
		name       string
		localOnly  bool
		remoteAddr string
		path       string
		wantCode   int
	}{
		{
			name:       "LocalOnly Enabled - Local IPv4 Allowed on /app",
			localOnly:  true,
			remoteAddr: "127.0.0.1:12345",
			path:       "/app/",
			wantCode:   http.StatusOK,
		},
		{
			name:       "LocalOnly Enabled - Local IPv6 Allowed on /app",
			localOnly:  true,
			remoteAddr: "[::1]:12345",
			path:       "/app/",
			wantCode:   http.StatusOK,
		},
		{
			name:       "LocalOnly Enabled - Remote Blocked on /app",
			localOnly:  true,
			remoteAddr: "192.168.1.1:12345",
			path:       "/app/",
			wantCode:   http.StatusForbidden,
		},
		{
			name:       "LocalOnly Disabled - Remote Allowed on /app",
			localOnly:  false,
			remoteAddr: "192.168.1.1:12345",
			path:       "/app/",
			wantCode:   http.StatusOK,
		},
		{
			name:       "LocalOnly Enabled - Remote Blocked on /app/credentials",
			localOnly:  true,
			remoteAddr: "192.168.1.1:12345",
			path:       "/app/credentials",
			wantCode:   http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := server.NewServer(dm, ss, "", false, true, tt.localOnly, "app.sqlite")
			handler := srv.Router()

			req := httptest.NewRequest("GET", tt.path, nil)
			req.RemoteAddr = tt.remoteAddr
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("%s: got code %d, want %d", tt.name, w.Code, tt.wantCode)
			}
		})
	}
}
