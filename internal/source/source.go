package source

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/rclone/rclone/backend/all"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/vfs"
	"github.com/rclone/rclone/vfs/vfscommon"
)

var (
	vfsCache = make(map[string]*vfs.VFS)
	vfsMu    sync.Mutex
	cacheDir = filepath.Join(os.TempDir(), "flight2-vfs-cache")
)

// Init sets the cache directory for rclone VFS.
func Init(cd string) {
	if cd != "" {
		cacheDir = cd
		config.SetCacheDir(cd) // Set global rclone cache dir
		os.MkdirAll(cacheDir, 0755)
	}
}

// getVFS returns a cached or new VFS instance.
func getVFS(ctx context.Context, sourcePath string, creds map[string]interface{}) (*vfs.VFS, string, error) {
	fsType, ok := creds["type"].(string)
	if !ok {
		if strings.HasPrefix(sourcePath, "http:") || strings.HasPrefix(sourcePath, "https:") {
			fsType = "http"
		} else {
			return nil, "", fmt.Errorf("credentials missing 'type' field")
		}
	}

	// Determine FS Root and Relative Path based on type
	var fsRoot string
	var relPath string

	switch fsType {
	case "local":
		// For local, we map the VFS to the system root /
		fsRoot = "/"
		if abs, err := filepath.Abs(sourcePath); err == nil {
			relPath = abs
		} else {
			relPath = sourcePath
		}
		// Provide cleaner relative path for VFS: remove leading slash
		relPath = strings.TrimPrefix(relPath, "/")

	case "http", "https":
		// For HTTP, we try to root at the domain
		u, err := url.Parse(sourcePath)
		if err == nil {
			fsRoot = u.Scheme + "://" + u.Host
			relPath = strings.TrimPrefix(u.Path, "/")
		} else {
			// Fallback
			fsRoot = path.Dir(sourcePath)
			relPath = path.Base(sourcePath)
		}
	default:
		// Cloud providers (S3, Drive, etc)
		// We root at "" (backend root)
		fsRoot = ""
		relPath = sourcePath
		// Fix S3 path: if it starts with /, trim it
		relPath = strings.TrimPrefix(relPath, "/")
	}

	// Generate Hash Key depending on Creds + FsRoot
	// Sort keys
	keys := make([]string, 0, len(creds))
	for k := range creds {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := md5.New()
	io.WriteString(h, fsRoot) // Include root in hash
	for _, k := range keys {
		io.WriteString(h, k)
		io.WriteString(h, fmt.Sprint(creds[k]))
	}
	hash := hex.EncodeToString(h.Sum(nil))

	vfsMu.Lock()
	defer vfsMu.Unlock()

	if v, ok := vfsCache[hash]; ok {
		return v, relPath, nil
	}

	// Create New
	conf := make(configmap.Simple)
	for k, v := range creds {
		if k != "type" {
			conf[k] = fmt.Sprint(v)
		}
	}

	regInfo, err := fs.Find(fsType)
	if err != nil {
		return nil, "", fmt.Errorf("backend type '%s' not found: %w", fsType, err)
	}

	remoteName := fmt.Sprintf("flight2_%s", hash)
	fsrc, err := regInfo.NewFs(ctx, remoteName, fsRoot, conf)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create fs: %w", err)
	}

	opt := &vfscommon.Options{
		CacheMode:         vfscommon.CacheModeFull,
		DirCacheTime:      fs.Duration(10 * time.Minute),
		CacheMaxAge:       fs.Duration(24 * time.Hour),
		CachePollInterval: fs.Duration(1 * time.Minute),
		ChunkSize:         fs.SizeSuffix(128 * 1024 * 1024),
	}

	v := vfs.New(fsrc, opt)
	vfsCache[hash] = v

	return v, relPath, nil
}

// GetFileStream returns a stream using VFS.
func GetFileStream(ctx context.Context, sourcePath string, creds map[string]interface{}) (io.ReadCloser, error) {
	v, relPath, err := getVFS(ctx, sourcePath, creds)
	if err != nil {
		return nil, err
	}

	f, err := v.OpenFile(relPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file '%s': %w", relPath, err)
	}
	return f, nil
}

// ListEntries returns a list of files as []os.FileInfo.
func ListEntries(ctx context.Context, sourcePath string, creds map[string]interface{}) ([]os.FileInfo, error) {
	v, relPath, err := getVFS(ctx, sourcePath, creds)
	if err != nil {
		return nil, err
	}

	if relPath == "" {
		relPath = "."
	}

	infos, err := v.ReadDir(relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory '%s': %w", relPath, err)
	}

	return infos, nil
}
