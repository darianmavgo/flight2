package dataset

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flight2/internal/dataset_source"

	"github.com/darianmavgo/mksqlite/converters/common"

	"github.com/allegro/bigcache/v3"
	"github.com/darianmavgo/mksqlite/converters"

	_ "github.com/darianmavgo/mksqlite/converters/all"
)

var extensionMap = map[string]string{
	".csv":      "csv",
	".xlsx":     "excel",
	".xls":      "excel",
	".zip":      "zip",
	".html":     "html",
	".htm":      "html",
	".json":     "json",
	".txt":      "txt",
	".md":       "markdown",
	".markdown": "markdown",
	".db":       "sqlite",
	".sqlite":   "sqlite",
	".sqlite3":  "sqlite",
}

type Manager struct {
	cache    *bigcache.BigCache
	verbose  bool
	cacheDir string
}

func NewManager(verbose bool, cacheDir string) (*Manager, error) {
	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	// Configure cache to hold gigabytes.
	// Max size in MB. 2GB = 2048.
	config := bigcache.DefaultConfig(10 * time.Minute)
	config.HardMaxCacheSize = 2048
	config.Shards = 32 // Fewer shards means larger shard size (2048 / 32 = 64MB)
	config.CleanWindow = 5 * time.Minute

	cache, err := bigcache.New(context.Background(), config)
	if err != nil {
		return nil, err
	}
	return &Manager{cache: cache, verbose: verbose, cacheDir: cacheDir}, nil
}

// GetSQLiteDB returns a path to a SQLite database for the given source.
func (m *Manager) GetSQLiteDB(ctx context.Context, sourcePath string, creds map[string]interface{}, alias string) (string, error) {
	// If type is local, try to resolve extension if file not found
	if t, ok := creds["type"].(string); ok && t == "local" {
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			for ext := range extensionMap {
				p := sourcePath + ext
				if info, err := os.Stat(p); err == nil && !info.IsDir() {
					sourcePath = p
					break
				}
			}
		}
	}

	// Include alias in cache key to prevent cross-user leaks
	if m.verbose {
		log.Printf("🔹 [CACHE KEY] Generating key from: Alias=[%s] SourcePath=[%s]", alias, sourcePath)
	}
	key := fmt.Sprintf("%s:%s", alias, sourcePath)

	// 1. Check Memory Cache (BigCache)
	entry, err := m.cache.Get(key)
	if err == nil {
		if m.verbose {
			fmt.Println("🟢 [CACHE HIT] (Memory) Serving from RAM")
		}
		return m.writeTempFile(entry)
	}

	// 2. Check Disk Cache
	hash := md5.Sum([]byte(key))
	hashStr := hex.EncodeToString(hash[:])
	diskPath := filepath.Join(m.cacheDir, hashStr+".sqlite")

	if info, err := os.Stat(diskPath); err == nil && !info.IsDir() {
		data, err := os.ReadFile(diskPath)
		if err == nil {
			if m.verbose {
				fmt.Println("🟢 [CACHE HIT] (Disk) Loaded from " + diskPath)
			}
			// Update memory cache
			m.cache.Set(key, data)
			return m.writeTempFile(data)
		}
	}

	// 3. Cache Miss - Fetch and Convert
	if m.verbose {
		fmt.Println("🟠 [CACHE MISS] Fetching and converting...")
	}

	// Prepare output file
	tmpOut, err := os.CreateTemp(m.cacheDir, "flight2_db_*.sqlite")
	if err != nil {
		return "", err
	}
	tmpOutName := tmpOut.Name()

	// Check if sourcePath is a local directory, but only if type is local
	isLocal := false
	if t, ok := creds["type"].(string); ok && t == "local" {
		isLocal = true
	}

	isDir := false
	if isLocal {
		if info, err := os.Stat(sourcePath); err == nil && info.IsDir() {
			isDir = true
		}
	}

	if isDir {
		f, err := os.Open(sourcePath)
		if err != nil {
			tmpOut.Close()
			os.Remove(tmpOutName)
			return "", err
		}

		conv, err := converters.Open("filesystem", f, &common.ConversionConfig{Verbose: m.verbose})
		if err != nil {
			f.Close()
			tmpOut.Close()
			os.Remove(tmpOutName)
			return "", fmt.Errorf("failed to open filesystem converter: %w", err)
		}

		err = converters.ImportToSQLite(conv, tmpOut, &converters.ImportOptions{Verbose: m.verbose})
		f.Close()
		if err != nil {
			tmpOut.Close()
			os.Remove(tmpOutName)
			return "", fmt.Errorf("conversion failed: %w", err)
		}
	} else {
		// Fetch source stream
		rc, err := dataset_source.GetFileStream(ctx, sourcePath, creds)
		if err != nil {
			tmpOut.Close()
			os.Remove(tmpOutName)
			return "", fmt.Errorf("fetch error: %w", err)
		}
		defer rc.Close()

		// Determine driver and check if it's SQLite
		ext := strings.ToLower(filepath.Ext(sourcePath))
		driver := extensionMap[ext]
		if driver == "" {
			// Fallback: try using the extension themselves as the driver name
			driver = strings.TrimPrefix(ext, ".")
		}

		tmpSource, err := os.CreateTemp(m.cacheDir, "flight2_source_*"+ext)
		if err != nil {
			tmpOut.Close()
			os.Remove(tmpOutName)
			return "", err
		}
		tmpSourceName := tmpSource.Name()
		defer os.Remove(tmpSourceName)

		_, err = io.Copy(tmpSource, rc)
		tmpSource.Close() // Close source file so we can open it for read or it's flushed
		if err != nil {
			tmpOut.Close()
			os.Remove(tmpOutName)
			return "", fmt.Errorf("failed to write source temp file: %w", err)
		}

		if driver == "sqlite" {
			// Just copy source to output
			srcF, err := os.Open(tmpSourceName)
			if err != nil {
				tmpOut.Close()
				os.Remove(tmpOutName)
				return "", err
			}

			_, err = io.Copy(tmpOut, srcF)
			srcF.Close()
			if err != nil {
				tmpOut.Close()
				os.Remove(tmpOutName)
				return "", err
			}
		} else {
			// Convert
			srcF, err := os.Open(tmpSourceName)
			if err != nil {
				tmpOut.Close()
				os.Remove(tmpOutName)
				return "", err
			}

			conv, err := converters.Open(driver, srcF, &common.ConversionConfig{Verbose: m.verbose})
			if err != nil {
				srcF.Close()
				tmpOut.Close()
				os.Remove(tmpOutName)
				return "", fmt.Errorf("failed to open converter for %s (ext: %s): %w", driver, ext, err)
			}

			// Handle Closer interface for converter
			if c, ok := conv.(io.Closer); ok {
				defer c.Close()
			}

			err = converters.ImportToSQLite(conv, tmpOut, &converters.ImportOptions{Verbose: m.verbose})
			srcF.Close()
			if err != nil {
				tmpOut.Close()
				os.Remove(tmpOutName)
				return "", fmt.Errorf("conversion failed for %s: %w", driver, err)
			}
		}
	}

	tmpOut.Close()

	// Read the result back to memory to store in cache
	data, err := os.ReadFile(tmpOutName)
	if err != nil {
		return "", fmt.Errorf("failed to read converted db: %w", err)
	}

	// Update Caches
	// 1. Memory
	err = m.cache.Set(key, data)
	if err != nil {
		fmt.Printf("Warning: failed to set cache: %v\n", err)
	}
	// 2. Disk
	if err := os.WriteFile(diskPath, data, 0644); err != nil {
		fmt.Printf("Warning: failed to write disk cache: %v\n", err)
	} else {
		if m.verbose {
			fmt.Printf("💾 [CACHE SAVED] Written to disk: %s\n", diskPath)
		}
	}

	return tmpOutName, nil
}

func (m *Manager) writeTempFile(data []byte) (string, error) {
	f, err := os.CreateTemp(m.cacheDir, "flight2_cache_*.sqlite")
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return "", err
	}
	return f.Name(), nil
}
