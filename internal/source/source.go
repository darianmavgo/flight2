package source

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	_ "github.com/rclone/rclone/backend/all"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
)

// GetSourceFs creates an rclone Fs for the given path and credentials.
func GetSourceFs(ctx context.Context, sourcePath string, creds map[string]interface{}) (fs.Fs, string, error) {
	fsType, ok := creds["type"].(string)
	if !ok {
		if strings.HasPrefix(sourcePath, "http:") || strings.HasPrefix(sourcePath, "https:") {
			fsType = "http"
		} else {
			return nil, "", fmt.Errorf("credentials missing 'type' field")
		}
	}

	// Fix potentially malformed URL scheme from banquet/path parsing
	if fsType == "http" {
		if strings.HasPrefix(sourcePath, "https:/") && !strings.HasPrefix(sourcePath, "https://") {
			sourcePath = strings.Replace(sourcePath, "https:/", "https://", 1)
		} else if strings.HasPrefix(sourcePath, "http:/") && !strings.HasPrefix(sourcePath, "http://") {
			sourcePath = strings.Replace(sourcePath, "http:/", "http://", 1)
		}
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
		return nil, "", fmt.Errorf("backend type '%s' not found: %w", fsType, err)
	}

	// Rclone paths use forward slashes.
	var parent, leaf string
	if sourcePath == "" || sourcePath == "/" {
		parent = ""
		leaf = ""
	} else {
		parent = path.Dir(sourcePath)
		leaf = path.Base(sourcePath)
		if parent == "." {
			parent = ""
		}
	}

	// Name for the remote (arbitrary, internal use)
	remoteName := "flight2_temp"

	// Create Fs
	fsrc, err := regInfo.NewFs(ctx, remoteName, parent, config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create fs for path '%s': %w", parent, err)
	}

	return fsrc, leaf, nil
}

// GetFileStream returns a stream of the file at the given sourceURL using the credentials.
func GetFileStream(ctx context.Context, sourcePath string, creds map[string]interface{}) (io.ReadCloser, error) {
	fsrc, leaf, err := GetSourceFs(ctx, sourcePath, creds)
	if err != nil {
		return nil, err
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

// ListEntries returns a list of files and directories at the given path.
func ListEntries(ctx context.Context, sourcePath string, creds map[string]interface{}) (fs.DirEntries, error) {
	fsrc, leaf, err := GetSourceFs(ctx, sourcePath, creds)
	if err != nil {
		return nil, err
	}

	// List the directory
	// If sourcePath ends in '/', leaf will be empty or '.', so we list that.
	entries, err := fsrc.List(ctx, leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory '%s': %w", leaf, err)
	}

	return entries, nil
}
