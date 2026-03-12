package db

import "gorm.io/gorm"

// File represents a source file in the codebase
type File struct {
	ID           int64      `gorm:"primaryKey"`
	Path         string     `gorm:"uniqueIndex;not null"`
	Language     string     `gorm:"not null"`
	Hash         string
	LinesOfCode  int
	Tokens       int
	Entities     []Entity     `gorm:"foreignKey:FileID;constraint:OnDelete:CASCADE"`
	Dependencies []Dependency `gorm:"foreignKey:SourceFileID;constraint:OnDelete:CASCADE"`
}

// Entity represents a code entity (function, class, type, etc.)
type Entity struct {
	ID              int64  `gorm:"primaryKey"`
	FileID          int64  `gorm:"index;not null"`
	Name            string `gorm:"index;not null"`
	Type            string `gorm:"index;not null"` // function, class, interface, type, method, etc.
	Kind            string // specific kind: async_function, arrow_function, method, etc.
	Signature       string
	StartLine       int
	EndLine         int
	ColumnStart     int
	ColumnEnd       int
	Documentation   string

	// Multi-language support fields
	Parent          string // Parent entity name for nested entities (e.g., "ClassName" for methods)
	Visibility      string `gorm:"index"` // public, private, protected, internal
	Scope           string // file, class, module, global, local
	Language        string `gorm:"index"` // go, python, javascript, typescript, java
	Attributes      string // JSON: {decorators: [], typeParams: [], modifiers: [], etc.}

	// Relationships
	File                   *File `gorm:"foreignKey:FileID"`
	SourceRelations        []EntityRelation `gorm:"foreignKey:SourceEntityID;constraint:OnDelete:CASCADE"`
	TargetRelations        []EntityRelation `gorm:"foreignKey:TargetEntityID;constraint:OnDelete:CASCADE"`
}

// Dependency represents a file dependency (import/require)
type Dependency struct {
	ID           int64  `gorm:"primaryKey"`
	SourceFileID int64  `gorm:"index;not null"`
	TargetPath   string `gorm:"not null"` // Import path as written in code
	ImportType   string // import, require, from, include
	LineNumber   int
	Resolved     string // Resolved module/file path if determined
	IsLocal      bool   // True if local module/relative import
	SourceFile   *File `gorm:"foreignKey:SourceFileID"`
}

// EntityRelation represents relationships between entities
type EntityRelation struct {
	ID           int64  `gorm:"primaryKey"`
	SourceEntityID int64 `gorm:"index;not null"`
	TargetEntityID int64 `gorm:"index;not null"`
	RelationType string `gorm:"index;not null"`
	LineNumber   int
	Context      string
	SourceEntity *Entity `gorm:"foreignKey:SourceEntityID"`
	TargetEntity *Entity `gorm:"foreignKey:TargetEntityID"`
}

// TableName specifies custom table names for GORM
func (File) TableName() string {
	return "files"
}

func (Entity) TableName() string {
	return "entities"
}

func (Dependency) TableName() string {
	return "dependencies"
}

func (EntityRelation) TableName() string {
	return "entity_relations"
}

// GetDB returns the underlying GORM database instance.
func (db *Database) GetDB() *gorm.DB {
	return db.conn
}

// WithTx returns a shallow copy of Database that uses the given transaction
// instead of the underlying connection.
func (db *Database) WithTx(tx *gorm.DB) *Database {
	return &Database{conn: tx}
}
