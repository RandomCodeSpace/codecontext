package db

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Database struct {
	conn *gorm.DB
}

// Open creates or opens a SQLite database at the specified path using GORM
func Open(dbPath string) (*Database, error) {
	dsn := dbPath
	conn, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &Database{conn: conn}

	// Auto migrate creates tables automatically
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

// File operations

func (db *Database) InsertFile(path, language, hash string) (int64, error) {
	file := &File{
		Path:     path,
		Language: language,
		Hash:     hash,
	}

	result := db.conn.Clauses().Save(file)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to insert file: %w", result.Error)
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

func (db *Database) GetFiles() ([]*File, error) {
	var files []*File
	result := db.conn.Find(&files)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get files: %w", result.Error)
	}
	return files, nil
}

// Entity operations

func (db *Database) InsertEntity(fileID int64, name, entityType, kind, signature string, startLine, endLine int, docs string) (int64, error) {
	entity := &Entity{
		FileID:        fileID,
		Name:          name,
		Type:          entityType,
		Kind:          kind,
		Signature:     signature,
		StartLine:     startLine,
		EndLine:       endLine,
		Documentation: docs,
	}

	// Use FirstOrCreate to handle duplicates
	result := db.conn.Where("file_id = ? AND name = ? AND type = ?", fileID, name, entityType).
		FirstOrCreate(entity)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to insert entity: %w", result.Error)
	}

	return entity.ID, nil
}

func (db *Database) GetEntitiesByFile(fileID int64) ([]*Entity, error) {
	var entities []*Entity
	result := db.conn.Where("file_id = ?", fileID).Find(&entities)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to query entities: %w", result.Error)
	}
	return entities, nil
}

func (db *Database) GetEntityByName(name string) ([]*Entity, error) {
	var entities []*Entity
	result := db.conn.Where("name = ?", name).Find(&entities)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to query entities: %w", result.Error)
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

// Dependency operations

func (db *Database) InsertDependency(sourceFileID int64, targetPath, depType string, lineNumber int) (int64, error) {
	dep := &Dependency{
		SourceFileID: sourceFileID,
		TargetPath:   targetPath,
		ImportType:   depType,
		LineNumber:   lineNumber,
	}

	result := db.conn.Create(dep)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to insert dependency: %w", result.Error)
	}

	return dep.ID, nil
}

func (db *Database) GetDependencies(fileID int64) ([]*Dependency, error) {
	var deps []*Dependency
	result := db.conn.Where("source_file_id = ?", fileID).Find(&deps)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to query dependencies: %w", result.Error)
	}
	return deps, nil
}

// Entity relation operations

func (db *Database) InsertEntityRelation(sourceID, targetID int64, relationType string, lineNumber int, context string) (int64, error) {
	rel := &EntityRelation{
		SourceEntityID: sourceID,
		TargetEntityID: targetID,
		RelationType:   relationType,
		LineNumber:     lineNumber,
		Context:        context,
	}

	result := db.conn.Create(rel)
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

	result := query.Find(&relations)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to query relations: %w", result.Error)
	}

	return relations, nil
}

// Graph query operations

func (db *Database) GetCallGraph(entityID int64, depth int) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Get the entity
	entity, err := db.GetEntityByID(entityID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("entity not found")
	}

	result["entity"] = map[string]interface{}{
		"id":   entity.ID,
		"name": entity.Name,
		"type": entity.Type,
	}

	// Get all related entities (calls, implementations, etc.)
	relations, err := db.GetEntityRelations(entityID, "calls")
	if err != nil {
		return nil, err
	}

	var related []map[string]interface{}
	for _, rel := range relations {
		called, err := db.GetEntityByID(rel.TargetEntityID)
		if err != nil {
			continue
		}
		if called == nil {
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

	result := make(map[string]interface{})
	result["file"] = file.Path

	deps, err := db.GetDependencies(file.ID)
	if err != nil {
		return nil, err
	}

	var dependencyPaths []string
	for _, dep := range deps {
		dependencyPaths = append(dependencyPaths, dep.TargetPath)
	}

	result["dependencies"] = dependencyPaths
	return result, nil
}

// Statistics

func (db *Database) GetFileCount() (int64, error) {
	var count int64
	result := db.conn.Model(&File{}).Count(&count)
	return count, result.Error
}

func (db *Database) GetEntityCount() (int64, error) {
	var count int64
	result := db.conn.Model(&Entity{}).Count(&count)
	return count, result.Error
}

func (db *Database) GetDependencyCount() (int64, error) {
	var count int64
	result := db.conn.Model(&Dependency{}).Count(&count)
	return count, result.Error
}

func (db *Database) GetRelationCount() (int64, error) {
	var count int64
	result := db.conn.Model(&EntityRelation{}).Count(&count)
	return count, result.Error
}
