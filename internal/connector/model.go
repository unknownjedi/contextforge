package connector

import (
	"context"
	"time"
)

// DatabaseType represents a supported external database engine.
type DatabaseType string

const (
	TypePostgres    DatabaseType = "postgres"
	TypeMySQL       DatabaseType = "mysql"
	TypeMariaDB     DatabaseType = "mariadb"
	TypeSQLite      DatabaseType = "sqlite"
	TypeSQLServer   DatabaseType = "sqlserver"
	TypeCockroachDB DatabaseType = "cockroachdb"
)

// ConnectionConfig holds parsed and validated parameters for external database connections.
type ConnectionConfig struct {
	Type     DatabaseType      `json:"type"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	Database string            `json:"database"`
	Username string            `json:"username"`
	Password string            `json:"-"` // Omitted from JSON serialization
	SSLMode  string            `json:"ssl_mode,omitempty"`
	FilePath string            `json:"file_path,omitempty"` // For SQLite
	Options  map[string]string `json:"options,omitempty"`
	RawURL   string            `json:"-"` // Omitted from JSON serialization
}

// ConnectionTestResult contains the outcome of a connection check.
type ConnectionTestResult struct {
	Success         bool          `json:"success"`
	DatabaseType    string        `json:"database_type"`
	DatabaseVersion string        `json:"database_version,omitempty"`
	Latency         time.Duration `json:"latency"`
	LatencyMs       int64         `json:"latency_ms"`
	ErrorMessage    string        `json:"error_message,omitempty"`
}

// ExtractionConfig dictates what metadata and data to ingest.
type ExtractionConfig struct {
	Mode             string              `json:"mode"` // "schema" | "schema_and_data"
	Schemas          []string            `json:"schemas,omitempty"`
	Tables           []string            `json:"tables,omitempty"`
	ExcludedColumns  map[string][]string `json:"excluded_columns,omitempty"` // table -> []column
	BatchSize        int                 `json:"batch_size,omitempty"`
	MaxRowsPerTable  int                 `json:"max_rows_per_table,omitempty"`
}

// DefaultExtractionConfig returns standard secure extraction options.
func DefaultExtractionConfig() ExtractionConfig {
	return ExtractionConfig{
		Mode:            "schema",
		BatchSize:       1000,
		MaxRowsPerTable: 50000,
		ExcludedColumns: make(map[string][]string),
	}
}

// DatabaseMetadata represents normalized database catalog information.
type DatabaseMetadata struct {
	DatabaseType string           `json:"database_type"`
	DatabaseName string           `json:"database_name"`
	Version      string           `json:"version,omitempty"`
	Schemas      []SchemaMetadata `json:"schemas"`
}

// SchemaMetadata represents a logical database schema / namespace.
type SchemaMetadata struct {
	Name   string          `json:"name"`
	Tables []TableMetadata `json:"tables"`
}

// TableMetadata represents a table or view in a schema.
type TableMetadata struct {
	Schema         string               `json:"schema"`
	Name           string               `json:"name"`
	Type           string               `json:"type"` // "table" or "view"
	Comment        string               `json:"comment,omitempty"`
	Columns        []ColumnMetadata     `json:"columns"`
	PrimaryKey     []string             `json:"primary_key,omitempty"`
	ForeignKeys    []ForeignKeyMetadata `json:"foreign_keys,omitempty"`
	Indexes        []IndexMetadata      `json:"indexes,omitempty"`
	ApproxRowCount int64                `json:"approx_row_count,omitempty"`
}

// ColumnMetadata represents a column in a table.
type ColumnMetadata struct {
	Name         string `json:"name"`
	DataType     string `json:"data_type"`
	Nullable     bool   `json:"nullable"`
	DefaultValue string `json:"default_value,omitempty"`
	Comment      string `json:"comment,omitempty"`
	Position     int    `json:"position"`
	IsPrimaryKey bool   `json:"is_primary_key"`
	IsSensitive  bool   `json:"is_sensitive"`
}

// ForeignKeyMetadata represents a relational constraint between tables.
type ForeignKeyMetadata struct {
	Name              string   `json:"name,omitempty"`
	Columns           []string `json:"columns"`
	ReferencedSchema  string   `json:"referenced_schema"`
	ReferencedTable   string   `json:"referenced_table"`
	ReferencedColumns []string `json:"referenced_columns"`
}

// IndexMetadata represents an index on a table.
type IndexMetadata struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Type    string   `json:"type,omitempty"`
}

// TableRecord represents a single streamed record for row-level ingestion.
type TableRecord struct {
	Schema   string         `json:"schema"`
	Table    string         `json:"table"`
	RowIndex int            `json:"row_index"`
	Values   map[string]any `json:"values"`
}

// DatabaseConnector defines the common abstraction for external database engines.
type DatabaseConnector interface {
	Type() DatabaseType
	ParseURL(rawURL string) (*ConnectionConfig, error)
	TestConnection(ctx context.Context, cfg *ConnectionConfig) (*ConnectionTestResult, error)
	DiscoverMetadata(ctx context.Context, cfg *ConnectionConfig) (*DatabaseMetadata, error)
	ExtractSchema(ctx context.Context, cfg *ConnectionConfig, opts ExtractionConfig) (*DatabaseMetadata, error)
	ExtractRows(ctx context.Context, cfg *ConnectionConfig, schemaName, tableName string, columns []string, limit, offset int) ([]TableRecord, error)
	SanitizeError(err error) error
}
