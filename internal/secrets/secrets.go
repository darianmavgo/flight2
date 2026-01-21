package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// Service handles secrets management backed by SQLite.
type Service struct {
	db  *sql.DB
	key []byte
}

// NewService creates a new Secrets service.
// dbPath: Path to the SQLite database file.
// keyPath: Path to the file containing the encryption key.
func NewService(dbPath, keyPath string) (*Service, error) {
	// Load or generate key
	key, err := loadOrGenerateKey(keyPath)
	if err != nil {
		return nil, err
	}

	// Open DB
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open secrets db: %w", err)
	}

	// Create table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS credentials (
			alias TEXT PRIMARY KEY,
			data TEXT NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return &Service{
		db:  db,
		key: key,
	}, nil
}

// Close closes the database connection.
func (s *Service) Close() error {
	return s.db.Close()
}

// StoreCredentials encrypts and stores the credentials.
// If alias is empty, a random one is generated.
// Returns the alias used.
func (s *Service) StoreCredentials(alias string, creds map[string]interface{}) (string, error) {
	if alias == "" {
		var err error
		alias, err = generateRandomString(12)
		if err != nil {
			return "", err
		}
	}

	jsonData, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("failed to marshal creds: %w", err)
	}

	encryptedData, err := encrypt(jsonData, s.key)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt data: %w", err)
	}

	_, err = s.db.Exec("INSERT OR REPLACE INTO credentials (alias, data) VALUES (?, ?)", alias, encryptedData)
	if err != nil {
		return "", fmt.Errorf("failed to insert into db: %w", err)
	}

	return alias, nil
}

// ListAliases returns a list of all stored aliases.
func (s *Service) ListAliases() ([]string, error) {
	rows, err := s.db.Query("SELECT alias FROM credentials ORDER BY alias")
	if err != nil {
		return nil, fmt.Errorf("failed to query aliases: %w", err)
	}
	defer rows.Close()

	var aliases []string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("failed to scan alias: %w", err)
		}
		aliases = append(aliases, alias)
	}
	return aliases, nil
}

// DeleteCredentials removes the credentials for the given alias.
func (s *Service) DeleteCredentials(alias string) error {
	_, err := s.db.Exec("DELETE FROM credentials WHERE alias = ?", alias)
	if err != nil {
		return fmt.Errorf("failed to delete credentials: %w", err)
	}
	return nil
}

// GetCredentials retrieves and decrypts credentials for the given alias.
func (s *Service) GetCredentials(alias string) (map[string]interface{}, error) {
	var encryptedData string
	err := s.db.QueryRow("SELECT data FROM credentials WHERE alias = ?", alias).Scan(&encryptedData)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("credentials not found for alias: %s", alias)
		}
		return nil, fmt.Errorf("db query error: %w", err)
	}

	jsonData, err := decrypt(encryptedData, s.key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	var creds map[string]interface{}
	err = json.Unmarshal(jsonData, &creds)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal creds: %w", err)
	}

	return creds, nil
}

func loadOrGenerateKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("invalid key length in %s", path)
		}
		return key, nil
	}

	if !os.IsNotExist(err) {
		return nil, err
	}

	// Generate new key
	key = make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, err
	}

	return key, nil
}

func encrypt(plaintext []byte, key []byte) (string, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func decrypt(ciphertextStr string, key []byte) ([]byte, error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(ciphertextStr)
	if err != nil {
		return nil, err
	}

	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:length], nil
}
