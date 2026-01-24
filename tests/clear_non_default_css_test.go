package tests

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyDefaultCSS(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Scanning repository at: %s\n", root)

	foundNonDefault := false

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories like .git
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}

		// Skip vendor, bin, and sample_data directories if they exist
		if info.IsDir() && (info.Name() == "vendor" || info.Name() == "bin" || info.Name() == "node_modules" || info.Name() == "sample_data") {
			return filepath.SkipDir
		}

		// 1. Check for CSS files
		if !info.IsDir() && strings.HasSuffix(path, ".css") {
			relPath, _ := filepath.Rel(root, path)
			if relPath != "cssjs/default.css" {
				fmt.Printf("FOUND NON-DEFAULT CSS FILE: %s\n", relPath)
				foundNonDefault = true
			} else {
				fmt.Printf("Found allowed CSS file: %s\n", relPath)
			}
		}

		// 2. Check for <style> tags in HTML, Go, and Template files
		if !info.IsDir() && (strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".tmpl")) {
			// Skip the test file itself
			if strings.HasSuffix(path, "clear_non_default_css_test.go") {
				return nil
			}
			match, err := fileContainsStyleTag(path)
			if err != nil {
				return err
			}
			if match {
				relPath, _ := filepath.Rel(root, path)
				fmt.Printf("FOUND STYLE TAG IN: %s\n", relPath)
				foundNonDefault = true
			}
		}

		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if foundNonDefault {
		t.Errorf("Test failed: Found non-default CSS or style tags in the repo.")
	} else {
		fmt.Println("Test passed: Only default.css found.")
	}
}

func fileContainsStyleTag(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), "<style") {
			return true, nil
		}
	}

	return false, scanner.Err()
}
