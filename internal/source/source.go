package source

import (
	"context"
	"fmt"
	"io"
	"path"

	_ "github.com/rclone/rclone/backend/all"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
)

// GetFileStream returns a stream of the file at the given sourceURL using the credentials.
func GetFileStream(ctx context.Context, sourcePath string, creds map[string]interface{}) (io.ReadCloser, error) {
	fsType, ok := creds["type"].(string)
	if !ok {
		return nil, fmt.Errorf("credentials missing 'type' field")
	}

	// Prepare config map
	config := make(configmap.Simple)
	for k, v := range creds {
		if k != "type" {
			config[k] = fmt.Sprint(v)
		}
	}

	// Find the backend
	regInfo, err := fs.Find(fsType)
	if err != nil {
		return nil, fmt.Errorf("backend type '%s' not found: %w", fsType, err)
	}

	// We split the sourcePath into parent and leaf.
	// Rclone paths use forward slashes.
	parent := path.Dir(sourcePath)
	leaf := path.Base(sourcePath)

	if parent == "." {
		parent = ""
	}

	// Name for the remote (arbitrary, internal use)
	remoteName := "flight2_temp"

	// Create Fs
	fsrc, err := regInfo.NewFs(ctx, remoteName, parent, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create fs for path '%s': %w", parent, err)
	}

	// Get the object
	obj, err := fsrc.NewObject(ctx, leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to find object '%s': %w", leaf, err)
	}

	// Open the object
	rc, err := obj.Open(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open object: %w", err)
	}

	return rc, nil
}
