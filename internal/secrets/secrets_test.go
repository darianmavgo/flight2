package secrets

import (
	"os"
	"testing"
)

func TestAddCompellingCredentials(t *testing.T) {
	// These values should match config.hcl if we want securebrowse to see them,
	// BUT typically tests use separate files.
	// The user said: "add some fake but compelling credentials to secrets.db ... and then confirm that securebrowse can display those"
	// This implies we should hit the real secrets.db and .secret.key.

	dbPath := "/Users/darianhickman/Documents/flight2/secrets.db"
	keyPath := "/Users/darianhickman/Documents/flight2/.secret.key"

	svc, err := NewService(dbPath, keyPath)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Close()

	compellingCreds := []struct {
		Alias string
		Data  map[string]interface{}
	}{
		{
			Alias: "production-s3-backups",
			Data: map[string]interface{}{
				"type":              "s3",
				"provider":          "AWS",
				"access_key_id":     "AKIA-FAKE-1234-5678",
				"secret_access_key": "SECRET-FAKE-KEY-ABCD-EFGH",
				"region":            "us-east-1",
				"endpoint":          "https://s3.us-east-1.amazonaws.com",
			},
		},
		{
			Alias: "google-drive-shared",
			Data: map[string]interface{}{
				"type":          "drive",
				"client_id":     "fake-google-client-id.apps.googleusercontent.com",
				"client_secret": "GOCSPX-fake-secret",
				"scope":         "drive",
				"token":         `{"access_token":"ya29.fake","token_type":"Bearer","refresh_token":"1//fake","expiry":"2026-01-21T22:00:00Z"}`,
			},
		},
		{
			Alias: "marketing-sftp",
			Data: map[string]interface{}{
				"type": "sftp",
				"host": "sftp.marketing.example.com",
				"user": "transfer_user",
				"port": 22,
				"pass": "super-secret-password",
			},
		},
		{
			Alias: "local-workspace",
			Data: map[string]interface{}{
				"type": "local",
			},
		},
	}

	for _, c := range compellingCreds {
		_, err := svc.StoreCredentials(c.Alias, c.Data)
		if err != nil {
			t.Errorf("Failed to store compelling credential %s: %v", c.Alias, err)
		} else {
			t.Logf("Successfully stored credential: %s", c.Alias)
		}
	}
}

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

	// Test auto-generated alias
	alias, err := svc.StoreCredentials("", creds)
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

	// Test specific alias
	myAlias := "my-s3"
	_, err = svc.StoreCredentials(myAlias, creds)
	if err != nil {
		t.Fatalf("Failed to store credentials with specific alias: %v", err)
	}

	fetchedCreds, err = svc.GetCredentials(myAlias)
	if err != nil {
		t.Fatalf("Failed to get credentials for specific alias: %v", err)
	}
	if fetchedCreds["type"] != "s3" {
		t.Fatalf("Fetched credentials do not match")
	}

	// Test ListAliases
	aliases, err := svc.ListAliases()
	if err != nil {
		t.Fatalf("Failed to list aliases: %v", err)
	}
	// We expect 2 aliases: generated one and "my-s3"
	if len(aliases) != 2 {
		t.Fatalf("Expected 2 aliases, got %d: %v", len(aliases), aliases)
	}

	// Test DeleteCredentials
	err = svc.DeleteCredentials(myAlias)
	if err != nil {
		t.Fatalf("Failed to delete credentials: %v", err)
	}

	aliases, err = svc.ListAliases()
	if err != nil {
		t.Fatalf("Failed to list aliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("Expected 1 alias after delete, got %d", len(aliases))
	}

	_, err = svc.GetCredentials(myAlias)
	if err == nil {
		t.Fatal("Expected error for deleted alias")
	}

	_, err = svc.GetCredentials("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent alias")
	}
}
