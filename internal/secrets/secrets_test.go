package secrets

import (
	"os"
	"testing"
)

func TestSecrets(t *testing.T) {
	dbPath := "test_secrets.db"
	keyPath := "test.key"
	defer os.Remove(dbPath)
	defer os.Remove(keyPath)

	svc, err := NewService(dbPath, keyPath)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Close()

	creds := map[string]interface{}{
		"type": "s3",
		"key":  "value",
	}

	alias, err := svc.StoreCredentials(creds)
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	if alias == "" {
		t.Fatal("Alias should not be empty")
	}

	fetchedCreds, err := svc.GetCredentials(alias)
	if err != nil {
		t.Fatalf("Failed to get credentials: %v", err)
	}

	if fetchedCreds["type"] != "s3" || fetchedCreds["key"] != "value" {
		t.Fatalf("Fetched credentials do not match: %v", fetchedCreds)
	}

	_, err = svc.GetCredentials("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent alias")
	}
}
