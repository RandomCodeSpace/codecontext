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

	"github.com/odvcencio/gotreesitter/grammars"
	gitignore "github.com/sabhiram/go-gitignore"

	"gorm.io/gorm"

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

// indexOp describes what IndexFile did with a file.
type indexOp int

const (
	opAdded   indexOp = iota // new file indexed for the first time
	opUpdated                // existing file re-indexed (content changed)
	opSkipped                // file unchanged, nothing to do
)

// IndexFile parses a single file and upserts all its entities, dependencies,
// and entity relations into the database.
//
// If the file has not changed since the last index run (hash is identical) it
// is skipped.  If the file has changed, existing entities and dependencies are
// deleted before re-inserting so the graph stays consistent.
//
// This method is safe to call from multiple goroutines concurrently.
// It returns the operation performed so callers can track add/update/skip counts.
func (idx *Indexer) IndexFile(filePath string) (indexOp, error) {
	idx.log("  📄 Indexing %s", filePath)

	idx.log("    📖 Reading & Hashing...")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return opSkipped, fmt.Errorf("failed to read file: %w", err)
	}

	linesOfCode := strings.Count(string(content), "\n")
	if len(content) > 0 && content[len(content)-1] != '\n' {
		linesOfCode++
	}
	// Estimate tokens as ~4 chars per token (standard heuristic)
	tokens := len(content) / 4

	hash := fmt.Sprintf("%x", md5.Sum(content))

	// --- DB read (needs lock for SQLite thread-safety) ---
	idx.mu.Lock()
	existingFile, err := idx.db.GetFileByPath(filePath)
	idx.mu.Unlock()
	if err != nil {
		return opSkipped, fmt.Errorf("failed to check existing file: %w", err)
	}

	op := opAdded
	if existingFile != nil {
		if existingFile.Hash == hash {
			idx.log("    ⏭️  Unchanged — skipping")
			return opSkipped, nil
		}
		op = opUpdated
		idx.log("    🔄 File changed — clearing old data")
		idx.mu.Lock()
		delEntErr := idx.db.DeleteEntitiesByFile(existingFile.ID)
		delDepErr := idx.db.DeleteDependenciesByFile(existingFile.ID)
		idx.mu.Unlock()
		if delEntErr != nil {
			return opSkipped, fmt.Errorf("failed to delete old entities: %w", delEntErr)
		}
		if delDepErr != nil {
			return opSkipped, fmt.Errorf("failed to delete old dependencies: %w", delDepErr)
		}
	}

	// --- Parse (pure CPU, no lock needed) ---
	idx.log("    ⚙️  Extracting entities & dependencies...")
	parseResult, err := parser.Parse(filePath, string(content))
	if err != nil {
		return opSkipped, fmt.Errorf("failed to parse file: %w", err)
	}

	idx.log("    🔍 Parsed: %d entities, %d imports (lang=%s)",
		len(parseResult.Entities), len(parseResult.Dependencies), parseResult.Language)

	// --- DB writes (serialised, wrapped in a single transaction) ---
	idx.mu.Lock()
	defer idx.mu.Unlock()

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

	txErr := idx.db.GetDB().Transaction(func(tx *gorm.DB) error {
		txDB := idx.db.WithTx(tx)

		fileID, err := txDB.InsertFile(filePath, string(parseResult.Language), hash, linesOfCode, tokens)
		if err != nil {
			return fmt.Errorf("failed to insert file: %w", err)
		}

		entityByName := make(map[string]int64)
		for _, entity := range parseResult.Entities {
			entID, err := txDB.InsertEntity(
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
			if _, err := txDB.InsertEntityRelation(parentID, childID, "defines", entity.StartLine, ""); err != nil {
				idx.log("    ⚠️  Could not insert relation %q→%q: %v", entity.Parent, entity.Name, err)
			} else {
				relationCount++
			}
		}
		if relationCount > 0 {
			idx.log("    🔗 Created %d defines relations", relationCount)
		}

		for _, dep := range parseResult.Dependencies {
			if _, err := txDB.InsertDependency(fileID, dep.Path, dep.Type, dep.LineNumber); err != nil {
				idx.log("    ⚠️  Could not insert dependency %q: %v", dep.Path, err)
			}
		}

		return nil
	})

	if txErr != nil {
		return opSkipped, txErr
	}
	return op, nil
}

// ignoreEntry pairs a base directory with the compiled patterns from the
// .gitignore / .ignore file found in that directory.
type ignoreEntry struct {
	base    string
	matcher *gitignore.GitIgnore
}

// loadIgnoreFiles loads .gitignore and .ignore from dir (if present) and
// returns a compiled matcher.  Returns nil when neither file exists.
func loadIgnoreFiles(dir string) *gitignore.GitIgnore {
	var lines []string
	for _, name := range []string{".gitignore", ".ignore"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			lines = append(lines, strings.Split(string(data), "\n")...)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return gitignore.CompileIgnoreLines(lines...)
}

// IndexDirectory recursively indexes all source files in a directory using a
// goroutine worker pool (one worker per CPU core) for parallel parsing.
//
// It respects .gitignore and .ignore files found at any level of the tree.
func (idx *Indexer) IndexDirectory(dirPath string) error {
	// ignoreEntries accumulates matchers as directories are visited.
	// They are populated on first entry into a directory so that by the time
	// files inside that directory are visited the rules are already in place.
	var (
		ignoreMu      sync.Mutex
		ignoreEntries []ignoreEntry
	)

	addIgnores := func(dir string) {
		m := loadIgnoreFiles(dir)
		if m != nil {
			ignoreMu.Lock()
			ignoreEntries = append(ignoreEntries, ignoreEntry{base: dir, matcher: m})
			ignoreMu.Unlock()
			idx.log("  📋 Loaded ignore rules from %s", dir)
		}
	}

	// isIgnored returns true when any ancestor-directory matcher matches path.
	isIgnored := func(path string) bool {
		ignoreMu.Lock()
		entries := ignoreEntries
		ignoreMu.Unlock()
		for _, e := range entries {
			rel, err := filepath.Rel(e.base, path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue // path is not under this matcher's base
			}
			if e.matcher.MatchesPath(rel) {
				return true
			}
		}
		return false
	}

	// Collect all source file paths, respecting ignore rules along the way.
	var paths []string
	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Load this directory's ignore files before checking its children.
			addIgnores(path)

			// Never descend into the root itself; only skip sub-directories.
			if path != dirPath {
				if strings.HasPrefix(d.Name(), ".") {
					idx.log("  🚫 Skipping hidden directory: %s", path)
					return filepath.SkipDir
				}
				switch d.Name() {
				case "node_modules", "vendor", "__pycache__", "target", "build", "dist":
					idx.log("  🚫 Skipping build/vendor directory: %s", path)
					return filepath.SkipDir
				}
				if isIgnored(path) {
					idx.log("  🚫 Ignored (gitignore): %s", path)
					return filepath.SkipDir
				}
			}
			return nil
		}
		if isIgnored(path) {
			idx.log("  🚫 Ignored (gitignore): %s", path)
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
		processed  atomic.Int64
		errCount   atomic.Int64
		addedCount atomic.Int64
		updCount   atomic.Int64
		skipCount  atomic.Int64
		wg         sync.WaitGroup
	)

	for i := 0; i < workers(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range pathCh {
				op, err := idx.IndexFile(p)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  ❌ Error indexing %s: %v\n", p, err)
					errCount.Add(1)
				} else {
					switch op {
					case opAdded:
						addedCount.Add(1)
					case opUpdated:
						updCount.Add(1)
					case opSkipped:
						skipCount.Add(1)
					}
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
	added := addedCount.Load()
	updated := updCount.Load()
	skipped := skipCount.Load()
	errs := errCount.Load()
	fmt.Println()
	fmt.Printf("  📊 Indexing complete: %d files processed\n", total)
	fmt.Printf("     ✅ Added:   %d\n", added)
	fmt.Printf("     🔄 Updated: %d\n", updated)
	fmt.Printf("     ⏭️  Skipped: %d (unchanged)\n", skipped)
	if errs > 0 {
		fmt.Printf("     ❌ Errors:  %d\n", errs)
	}
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
	if ext == "" {
		return false
	}
	return grammars.DetectLanguage("file"+ext) != nil
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
	locCount, _ := idx.db.GetLinesOfCodeCount()
	tokensCount, _ := idx.db.GetTokensCount()
	return map[string]interface{}{
		"files":         fileCount,
		"entities":      entityCount,
		"dependencies":  depCount,
		"relations":     relCount,
		"lines_of_code": locCount,
		"tokens":        tokensCount,
	}, nil
}

func (idx *Indexer) GetAllFiles() ([]*db.File, error)               { return idx.db.GetFiles() }
func (idx *Indexer) GetAllEntities() ([]*db.Entity, error)          { return idx.db.GetAllEntities() }
func (idx *Indexer) GetAllRelations() ([]*db.EntityRelation, error) { return idx.db.GetAllRelations() }
func (idx *Indexer) GetAllDependencies() ([]*db.Dependency, error) {
	return idx.db.GetAllDependencies()
}
func (idx *Indexer) GetFileByID(id int64) (*db.File, error)      { return idx.db.GetFileByID(id) }
func (idx *Indexer) GetFileByPath(path string) (*db.File, error) { return idx.db.GetFileByPath(path) }
func (idx *Indexer) GetEntitiesByFile(fileID int64) ([]*db.Entity, error) {
	return idx.db.GetEntitiesByFile(fileID)
}
func (idx *Indexer) GetEntityRelations(entityID int64, relType string) ([]*db.EntityRelation, error) {
	return idx.db.GetEntityRelations(entityID, relType)
}
func (idx *Indexer) GetEntityByID(id int64) (*db.Entity, error) { return idx.db.GetEntityByID(id) }
