package tests

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// CleanTestArtifacts recursively removes files and directories that start with "test_"
// or end with ".test" within the specified root directory.
func CleanTestArtifacts(root string, dryRun bool) error {
	log.Printf("Cleaning test artifacts in %s (dryRun=%v)...", root, dryRun)

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root dir itself
		if path == root {
			return nil
		}

		name := info.Name()

		if info.IsDir() {
			// Skip protected directories
			if name == "sample_data" || name == ".git" || name == ".idea" || name == ".vscode" {
				return filepath.SkipDir
			}
		}

		shouldDelete := false

		// Criteria for deletion
		if strings.HasPrefix(name, "test_") {
			shouldDelete = true
		}
		// Also clean up temp dirs that might be named somewhat differently but mostly we use test_ prefix
		// or .test binaries
		if strings.HasSuffix(name, ".test") {
			shouldDelete = true
		}

		if shouldDelete {
			if dryRun {
				log.Printf("[DRY RUN] Would delete: %s", path)
			} else {
				log.Printf("Deleting: %s", path)
				if info.IsDir() {
					if err := os.RemoveAll(path); err != nil {
						log.Printf("Failed to remove dir %s: %v", path, err)
					} else {
						return filepath.SkipDir // Don't walk into deleted dir
					}
				} else {
					if err := os.Remove(path); err != nil {
						log.Printf("Failed to remove file %s: %v", path, err)
					}
				}
			}
		}

		return nil
	})
}
