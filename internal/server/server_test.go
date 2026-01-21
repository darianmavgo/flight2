package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHandleDebugEnv(t *testing.T) {
	// Set a custom env var to verify it appears
	key := "FLIGHT2_TEST_ENV"
	val := "some_value"
	os.Setenv(key, val)
	defer os.Unsetenv(key)

	// Create a server instance with nil dependencies as they are not used by handleDebugEnv
	s := &Server{}
	router := s.Router()

	req, err := http.NewRequest("GET", "/debug/env", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check if the output contains our env var
	expected := key + "=" + val
	if !strings.Contains(rr.Body.String(), expected) {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}
