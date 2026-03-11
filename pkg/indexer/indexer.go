package indexer

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RandomCodeSpace/codecontext/pkg/db"
	"github.com/RandomCodeSpace/codecontext/pkg/parser"
)

type Indexer struct {
	db *db.Database
}

func New(database *db.Database) *Indexer {
	return &Indexer{db: database}
}

// IndexFile parses a file and stores its entities and dependencies in the database
func (idx *Indexer) IndexFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)

	// Parse the file
	parseResult, err := parser.Parse(filePath, contentStr)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	// Calculate hash
	hash := fmt.Sprintf("%x", md5.Sum(content))

	// Insert file into database
	fileID, err := idx.db.InsertFile(filePath, parseResult.Language, hash)
	if err != nil {
		return fmt.Errorf("failed to insert file: %w", err)
	}

	// Insert entities
	entityMap := make(map[string]int64)
	for _, entity := range parseResult.Entities {
		entID, err := idx.db.InsertEntity(
			fileID, entity.Name, entity.Type, entity.Kind,
			entity.Signature, entity.StartLine, entity.EndLine, entity.Docs,
		)
		if err != nil {
			return fmt.Errorf("failed to insert entity: %w", err)
		}
		entityMap[entity.Name] = entID
	}

	// Insert dependencies
	for _, dep := range parseResult.Dependencies {
		_, err := idx.db.InsertDependency(fileID, dep.Path, dep.Type, dep.LineNumber)
		if err != nil {
			return fmt.Errorf("failed to insert dependency: %w", err)
		}
	}

	return nil
}

// IndexDirectory recursively indexes all source files in a directory
func (idx *Indexer) IndexDirectory(dirPath string) error {
	return filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != dirPath {
			return filepath.SkipDir
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Skip non-source files
		ext := strings.ToLower(filepath.Ext(path))
		if !isSourceFile(ext) {
			return nil
		}

		// Index the file
		if err := idx.IndexFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "error indexing %s: %v\n", path, err)
			// Continue with other files
		}

		return nil
	})
}

func isSourceFile(ext string) bool {
	sourceExts := map[string]bool{
		".go":   true,
		".js":   true,
		".ts":   true,
		".jsx":  true,
		".tsx":  true,
		".py":   true,
		".java": true,
		".c":    true,
		".cpp":  true,
		".h":    true,
		".rs":   true,
	}
	return sourceExts[ext]
}

// QueryEntity searches for entities by name
func (idx *Indexer) QueryEntity(name string) ([]*db.Entity, error) {
	return idx.db.GetEntityByName(name)
}

// QueryCallGraph returns the call graph for an entity
func (idx *Indexer) QueryCallGraph(entityID int64) (map[string]interface{}, error) {
	return idx.db.GetCallGraph(entityID, 1)
}

// QueryDependencyGraph returns the dependency graph for a file
func (idx *Indexer) QueryDependencyGraph(filePath string) (map[string]interface{}, error) {
	return idx.db.GetDependencyGraph(filePath)
}

// GetStats returns statistics about the indexed code
func (idx *Indexer) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	fileCount, err := idx.db.GetFileCount()
	if err != nil {
		return nil, err
	}
	stats["files"] = fileCount

	entityCount, err := idx.db.GetEntityCount()
	if err != nil {
		return nil, err
	}
	stats["entities"] = entityCount

	depCount, err := idx.db.GetDependencyCount()
	if err != nil {
		return nil, err
	}
	stats["dependencies"] = depCount

	relCount, err := idx.db.GetRelationCount()
	if err != nil {
		return nil, err
	}
	stats["relations"] = relCount

	return stats, nil
}
