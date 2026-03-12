package db

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Database struct {
	conn *gorm.DB
}

// Open creates or opens a SQLite database at the specified path using GORM.
// verbose enables SQL query logging.
func Open(dbPath string, verbose bool) (*Database, error) {
	logLevel := logger.Silent
	if verbose {
		logLevel = logger.Info
	}

	conn, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign key enforcement in SQLite.
	if err := conn.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// WAL mode allows concurrent reads during writes — significant speedup
	// for the parallel indexer that reads hash checks while others write.
	if err := conn.Exec("PRAGMA journal_mode = WAL").Error; err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	db := &Database{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *Database) migrate() error {
	return db.conn.AutoMigrate(&File{}, &Entity{}, &Dependency{}, &EntityRelation{})
}

func (db *Database) Close() error {
	sqlDB, err := db.conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// --------------------------------------------------------------------------
// File operations
// --------------------------------------------------------------------------

// InsertFile upserts a file record.  If the path already exists the hash and
// language are updated and the existing ID is returned.
func (db *Database) InsertFile(path, language, hash string) (int64, error) {
	var file File
	result := db.conn.Where("path = ?", path).First(&file)
	if result.Error == gorm.ErrRecordNotFound {
		file = File{Path: path, Language: language, Hash: hash}
		if err := db.conn.Create(&file).Error; err != nil {
			return 0, fmt.Errorf("failed to insert file: %w", err)
		}
		return file.ID, nil
	}
	if result.Error != nil {
		return 0, fmt.Errorf("failed to query file: %w", result.Error)
	}
	// Existing file — update metadata in case language or hash changed.
	if err := db.conn.Model(&file).Updates(map[string]interface{}{
		"language": language,
		"hash":     hash,
	}).Error; err != nil {
		return 0, fmt.Errorf("failed to update file: %w", err)
	}
	return file.ID, nil
}

func (db *Database) GetFileByPath(path string) (*File, error) {
	var file File
	result := db.conn.Where("path = ?", path).First(&file)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get file: %w", result.Error)
	}
	return &file, nil
}

func (db *Database) GetFileByID(id int64) (*File, error) {
	var file File
	result := db.conn.First(&file, id)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get file: %w", result.Error)
	}
	return &file, nil
}

func (db *Database) GetFiles() ([]*File, error) {
	var files []*File
	if err := db.conn.Find(&files).Error; err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}
	return files, nil
}

// --------------------------------------------------------------------------
// Entity operations
// --------------------------------------------------------------------------

// InsertEntity inserts a new entity, or returns the existing ID if an entity
// with the same file+name+type+start_line already exists.
func (db *Database) InsertEntity(fileID int64, name, entityType, kind, signature string, startLine, endLine int, docs, parent, visibility, scope, language string) (int64, error) {
	entity := &Entity{
		FileID:        fileID,
		Name:          name,
		Type:          entityType,
		Kind:          kind,
		Signature:     signature,
		StartLine:     startLine,
		EndLine:       endLine,
		Documentation: docs,
		Parent:        parent,
		Visibility:    visibility,
		Scope:         scope,
		Language:      language,
	}
	result := db.conn.
		Where("file_id = ? AND name = ? AND type = ? AND start_line = ?", fileID, name, entityType, startLine).
		FirstOrCreate(entity)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to insert entity: %w", result.Error)
	}
	return entity.ID, nil
}

func (db *Database) GetEntitiesByFile(fileID int64) ([]*Entity, error) {
	var entities []*Entity
	if err := db.conn.Where("file_id = ?", fileID).Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to query entities: %w", err)
	}
	return entities, nil
}

func (db *Database) GetEntityByName(name string) ([]*Entity, error) {
	var entities []*Entity
	if err := db.conn.Where("name = ?", name).Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to query entities: %w", err)
	}
	return entities, nil
}

func (db *Database) GetEntityByID(id int64) (*Entity, error) {
	var entity Entity
	result := db.conn.First(&entity, id)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get entity: %w", result.Error)
	}
	return &entity, nil
}

func (db *Database) GetAllEntities() ([]*Entity, error) {
	var entities []*Entity
	if err := db.conn.Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to get all entities: %w", err)
	}
	return entities, nil
}

// --------------------------------------------------------------------------
// Delete helpers (used when re-indexing a changed file)
// --------------------------------------------------------------------------

// DeleteEntitiesByFile removes all entities for the given file.
// The CASCADE constraint on EntityRelation automatically removes relation rows.
func (db *Database) DeleteEntitiesByFile(fileID int64) error {
	return db.conn.Where("file_id = ?", fileID).Delete(&Entity{}).Error
}

// DeleteDependenciesByFile removes all dependency records for the given file.
func (db *Database) DeleteDependenciesByFile(fileID int64) error {
	return db.conn.Where("source_file_id = ?", fileID).Delete(&Dependency{}).Error
}

// --------------------------------------------------------------------------
// Dependency operations
// --------------------------------------------------------------------------

// InsertDependency upserts a dependency record.
func (db *Database) InsertDependency(sourceFileID int64, targetPath, depType string, lineNumber int) (int64, error) {
	dep := &Dependency{
		SourceFileID: sourceFileID,
		TargetPath:   targetPath,
		ImportType:   depType,
		LineNumber:   lineNumber,
	}
	result := db.conn.
		Where("source_file_id = ? AND target_path = ? AND import_type = ? AND line_number = ?",
			sourceFileID, targetPath, depType, lineNumber).
		FirstOrCreate(dep)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to insert dependency: %w", result.Error)
	}
	return dep.ID, nil
}

func (db *Database) GetDependencies(fileID int64) ([]*Dependency, error) {
	var deps []*Dependency
	if err := db.conn.Where("source_file_id = ?", fileID).Find(&deps).Error; err != nil {
		return nil, fmt.Errorf("failed to query dependencies: %w", err)
	}
	return deps, nil
}

func (db *Database) GetAllDependencies() ([]*Dependency, error) {
	var deps []*Dependency
	if err := db.conn.Find(&deps).Error; err != nil {
		return nil, fmt.Errorf("failed to get all dependencies: %w", err)
	}
	return deps, nil
}

// --------------------------------------------------------------------------
// Entity relation operations
// --------------------------------------------------------------------------

// InsertEntityRelation upserts a relation row.
func (db *Database) InsertEntityRelation(sourceID, targetID int64, relationType string, lineNumber int, context string) (int64, error) {
	rel := &EntityRelation{
		SourceEntityID: sourceID,
		TargetEntityID: targetID,
		RelationType:   relationType,
		LineNumber:     lineNumber,
		Context:        context,
	}
	result := db.conn.
		Where("source_entity_id = ? AND target_entity_id = ? AND relation_type = ?",
			sourceID, targetID, relationType).
		FirstOrCreate(rel)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to insert relation: %w", result.Error)
	}
	return rel.ID, nil
}

func (db *Database) GetEntityRelations(entityID int64, relationType string) ([]*EntityRelation, error) {
	var relations []*EntityRelation
	query := db.conn.Where("source_entity_id = ?", entityID)
	if relationType != "" {
		query = query.Where("relation_type = ?", relationType)
	}
	if err := query.Find(&relations).Error; err != nil {
		return nil, fmt.Errorf("failed to query relations: %w", err)
	}
	return relations, nil
}

func (db *Database) GetAllRelations() ([]*EntityRelation, error) {
	var relations []*EntityRelation
	if err := db.conn.Find(&relations).Error; err != nil {
		return nil, fmt.Errorf("failed to get all relations: %w", err)
	}
	return relations, nil
}

// --------------------------------------------------------------------------
// Graph queries
// --------------------------------------------------------------------------

func (db *Database) GetCallGraph(entityID int64, depth int) (map[string]interface{}, error) {
	entity, err := db.GetEntityByID(entityID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("entity not found")
	}

	result := map[string]interface{}{
		"entity": map[string]interface{}{
			"id":   entity.ID,
			"name": entity.Name,
			"type": entity.Type,
		},
	}

	relations, err := db.GetEntityRelations(entityID, "calls")
	if err != nil {
		return nil, err
	}

	var related []map[string]interface{}
	for _, rel := range relations {
		called, err := db.GetEntityByID(rel.TargetEntityID)
		if err != nil || called == nil {
			continue
		}
		related = append(related, map[string]interface{}{
			"id":   called.ID,
			"name": called.Name,
			"type": called.Type,
		})
	}
	result["calls"] = related
	return result, nil
}

func (db *Database) GetDependencyGraph(filePath string) (map[string]interface{}, error) {
	file, err := db.GetFileByPath(filePath)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	deps, err := db.GetDependencies(file.ID)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, dep := range deps {
		paths = append(paths, dep.TargetPath)
	}
	return map[string]interface{}{
		"file":         file.Path,
		"dependencies": paths,
	}, nil
}

// --------------------------------------------------------------------------
// Statistics
// --------------------------------------------------------------------------

func (db *Database) GetFileCount() (int64, error) {
	var count int64
	return count, db.conn.Model(&File{}).Count(&count).Error
}

func (db *Database) GetEntityCount() (int64, error) {
	var count int64
	return count, db.conn.Model(&Entity{}).Count(&count).Error
}

func (db *Database) GetDependencyCount() (int64, error) {
	var count int64
	return count, db.conn.Model(&Dependency{}).Count(&count).Error
}

func (db *Database) GetRelationCount() (int64, error) {
	var count int64
	return count, db.conn.Model(&EntityRelation{}).Count(&count).Error
}
