package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileAggregation(t *testing.T) {
	// Create a temporary directory
	tmpdir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpdir, "test.go")
	content := `package main

func TestFunc() {
    println("test")
}
`
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Walk and find files
	entries, err := os.ReadDir(tmpdir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 file, got %d", len(entries))
	}

	// Verify file extension detection
	ext := filepath.Ext(testFile)
	if ext != ".go" {
		t.Errorf("Expected .go extension, got %s", ext)
	}
}

func TestDirectoryWalk(t *testing.T) {
	tmpdir := t.TempDir()

	// Create subdirectory
	subdir := filepath.Join(tmpdir, "subdir")
	err := os.Mkdir(subdir, 0755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create files in both directories
	mainFile := filepath.Join(tmpdir, "main.go")
	subFile := filepath.Join(subdir, "helper.go")

	os.WriteFile(mainFile, []byte("package main"), 0644)
	os.WriteFile(subFile, []byte("package main"), 0644)

	// Walk the directory using WalkDir
	fileCount := 0
	filepath.WalkDir(tmpdir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".go" {
			fileCount++
		}
		return nil
	})

	if fileCount != 2 {
		t.Errorf("Expected 2 Go files, found %d", fileCount)
	}
}

func TestHiddenDirSkip(t *testing.T) {
	tmpdir := t.TempDir()

	// Create hidden directory
	hiddenDir := filepath.Join(tmpdir, ".git")
	err := os.Mkdir(hiddenDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create hidden directory: %v", err)
	}

	// Create file in hidden directory
	hiddenFile := filepath.Join(hiddenDir, "config")
	os.WriteFile(hiddenFile, []byte("hidden"), 0644)

	// Create normal file
	normalFile := filepath.Join(tmpdir, "normal.go")
	os.WriteFile(normalFile, []byte("package main"), 0644)

	// Walk the directory, skipping hidden dirs using WalkDir
	fileCount := 0
	filepath.WalkDir(tmpdir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && filepath.Base(path)[0:1] == "." {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			fileCount++
		}
		return nil
	})

	if fileCount != 1 {
		t.Errorf("Expected 1 file (hidden dir skipped), found %d", fileCount)
	}
}
