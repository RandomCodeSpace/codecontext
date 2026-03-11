package indexer

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/RandomCodeSpace/codecontext/pkg/db"
	"github.com/RandomCodeSpace/codecontext/pkg/parser"
)

// Indexer builds and queries the code graph.
type Indexer struct {
	db      *db.Database
	verbose bool
	// mu serialises DB writes so multiple goroutines don't race on SQLite.
	mu sync.Mutex
}

func New(database *db.Database) *Indexer {
	return &Indexer{db: database}
}

// SetVerbose enables detailed progress logging with emojis.
func (idx *Indexer) SetVerbose(v bool) {
	idx.verbose = v
}

func (idx *Indexer) log(format string, args ...interface{}) {
	if idx.verbose {
		fmt.Printf(format+"\n", args...)
	}
}

// IndexFile parses a single file and upserts all its entities, dependencies,
// and entity relations into the database.
//
// If the file has not changed since the last index run (hash is identical) it
// is skipped.  If the file has changed, existing entities and dependencies are
// deleted before re-inserting so the graph stays consistent.
//
// This method is safe to call from multiple goroutines concurrently.
func (idx *Indexer) IndexFile(filePath string) error {
	idx.log("  📄 Indexing %s", filePath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	hash := fmt.Sprintf("%x", md5.Sum(content))

	// --- DB read (needs lock for SQLite thread-safety) ---
	idx.mu.Lock()
	existingFile, err := idx.db.GetFileByPath(filePath)
	idx.mu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to check existing file: %w", err)
	}

	if existingFile != nil {
		if existingFile.Hash == hash {
			idx.log("    ⏭️  Unchanged — skipping")
			return nil
		}
		idx.log("    🔄 File changed — clearing old data")
		idx.mu.Lock()
		delEntErr := idx.db.DeleteEntitiesByFile(existingFile.ID)
		delDepErr := idx.db.DeleteDependenciesByFile(existingFile.ID)
		idx.mu.Unlock()
		if delEntErr != nil {
			return fmt.Errorf("failed to delete old entities: %w", delEntErr)
		}
		if delDepErr != nil {
			return fmt.Errorf("failed to delete old dependencies: %w", delDepErr)
		}
	}

	// --- Parse (pure CPU, no lock needed) ---
	parseResult, err := parser.Parse(filePath, string(content))
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	idx.log("    🔍 Parsed: %d entities, %d imports (lang=%s)",
		len(parseResult.Entities), len(parseResult.Dependencies), parseResult.Language)

	// --- DB writes (serialised) ---
	idx.mu.Lock()
	defer idx.mu.Unlock()

	fileID, err := idx.db.InsertFile(filePath, string(parseResult.Language), hash)
	if err != nil {
		return fmt.Errorf("failed to insert file: %w", err)
	}

	// qualKey returns a collision-free map key: "ClassName.methodName" for
	// child entities, plain "Name" for top-level ones.  This prevents methods
	// with the same name in different classes (e.g. toString, equals,
	// constructors) from overwriting each other in the lookup map.
	qualKey := func(parent, name string) string {
		if parent != "" {
			return parent + "." + name
		}
		return name
	}

	entityByName := make(map[string]int64)
	for _, entity := range parseResult.Entities {
		entID, err := idx.db.InsertEntity(
			fileID,
			entity.Name,
			entity.Type,
			entity.Kind,
			entity.Signature,
			entity.StartLine,
			entity.EndLine,
			entity.Docs,
			entity.Parent,
			entity.Visibility,
			entity.Scope,
			string(parseResult.Language),
		)
		if err != nil {
			idx.log("    ⚠️  Could not insert entity %q: %v", entity.Name, err)
			continue
		}
		key := qualKey(entity.Parent, entity.Name)
		if _, exists := entityByName[key]; !exists {
			entityByName[key] = entID
		}
		idx.log("      ✅ Entity: [%s] %s (lines %d-%d)", entity.Type, entity.Name, entity.StartLine, entity.EndLine)
	}

	// Build "defines" relations: parent entity → child entity.
	relationCount := 0
	for _, entity := range parseResult.Entities {
		if entity.Parent == "" {
			continue
		}
		parentID, parentOK := entityByName[qualKey("", entity.Parent)]
		childID, childOK := entityByName[qualKey(entity.Parent, entity.Name)]
		if !parentOK || !childOK {
			continue
		}
		if _, err := idx.db.InsertEntityRelation(parentID, childID, "defines", entity.StartLine, ""); err != nil {
			idx.log("    ⚠️  Could not insert relation %q→%q: %v", entity.Parent, entity.Name, err)
		} else {
			relationCount++
		}
	}
	if relationCount > 0 {
		idx.log("    🔗 Created %d defines relations", relationCount)
	}

	for _, dep := range parseResult.Dependencies {
		if _, err := idx.db.InsertDependency(fileID, dep.Path, dep.Type, dep.LineNumber); err != nil {
			idx.log("    ⚠️  Could not insert dependency %q: %v", dep.Path, err)
		}
	}

	return nil
}

// IndexDirectory recursively indexes all source files in a directory using a
// goroutine worker pool (one worker per CPU core) for parallel parsing.
func (idx *Indexer) IndexDirectory(dirPath string) error {
	// Collect all source file paths first.
	var paths []string
	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != dirPath {
				idx.log("  🚫 Skipping hidden directory: %s", path)
				return filepath.SkipDir
			}
			switch d.Name() {
			case "node_modules", "vendor", "__pycache__", "target", "build", "dist":
				idx.log("  🚫 Skipping build/vendor directory: %s", path)
				return filepath.SkipDir
			}
			return nil
		}
		if isSourceFile(strings.ToLower(filepath.Ext(path))) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	total := len(paths)
	fmt.Printf("  🗂️  Found %d source files — indexing with %d workers\n", total, workers())

	// Worker pool: feed file paths through a channel.
	pathCh := make(chan string, total)
	for _, p := range paths {
		pathCh <- p
	}
	close(pathCh)

	var (
		processed atomic.Int64
		errCount  atomic.Int64
		wg        sync.WaitGroup
	)

	for i := 0; i < workers(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range pathCh {
				if err := idx.IndexFile(p); err != nil {
					fmt.Fprintf(os.Stderr, "  ❌ Error indexing %s: %v\n", p, err)
					errCount.Add(1)
				}
				n := processed.Add(1)
				// Print a compact progress line every 10 files (or on first/last).
				if !idx.verbose && (n%10 == 0 || n == int64(total)) {
					fmt.Printf("  ⏳ Progress: %d/%d files indexed\r", n, total)
				}
			}
		}()
	}

	wg.Wait()
	fmt.Printf("\n  📊 Indexed %d/%d files, %d errors\n", processed.Load()-errCount.Load(), total, errCount.Load())
	return nil
}

// workers returns the number of parallel indexing goroutines to use.
func workers() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	// Cap at 8 to avoid hammering SQLite with too many concurrent writers.
	if n > 8 {
		return 8
	}
	return n
}

func isSourceFile(ext string) bool {
	return map[string]bool{
		".go": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".py": true, ".java": true, ".c": true, ".cpp": true, ".h": true, ".rs": true,
	}[ext]
}

// --------------------------------------------------------------------------
// Query methods
// --------------------------------------------------------------------------

func (idx *Indexer) QueryEntity(name string) ([]*db.Entity, error) {
	return idx.db.GetEntityByName(name)
}

func (idx *Indexer) QueryCallGraph(entityID int64) (map[string]interface{}, error) {
	return idx.db.GetCallGraph(entityID, 1)
}

func (idx *Indexer) QueryDependencyGraph(filePath string) (map[string]interface{}, error) {
	return idx.db.GetDependencyGraph(filePath)
}

func (idx *Indexer) GetStats() (map[string]interface{}, error) {
	fileCount, _ := idx.db.GetFileCount()
	entityCount, _ := idx.db.GetEntityCount()
	depCount, _ := idx.db.GetDependencyCount()
	relCount, _ := idx.db.GetRelationCount()
	return map[string]interface{}{
		"files":        fileCount,
		"entities":     entityCount,
		"dependencies": depCount,
		"relations":    relCount,
	}, nil
}

func (idx *Indexer) GetAllFiles() ([]*db.File, error)              { return idx.db.GetFiles() }
func (idx *Indexer) GetAllEntities() ([]*db.Entity, error)         { return idx.db.GetAllEntities() }
func (idx *Indexer) GetAllRelations() ([]*db.EntityRelation, error) { return idx.db.GetAllRelations() }
func (idx *Indexer) GetAllDependencies() ([]*db.Dependency, error)  { return idx.db.GetAllDependencies() }
