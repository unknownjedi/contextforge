package connector

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteConnector implements DatabaseConnector for SQLite databases using pure-Go modernc.org/sqlite.
type SQLiteConnector struct{}

// NewSQLiteConnector constructs a connector for SQLite.
func NewSQLiteConnector() *SQLiteConnector {
	return &SQLiteConnector{}
}

func (c *SQLiteConnector) Type() DatabaseType {
	return TypeSQLite
}

func (c *SQLiteConnector) ParseURL(rawURL string) (*ConnectionConfig, error) {
	// Handle memory databases
	if rawURL == ":memory:" || strings.HasPrefix(rawURL, "sqlite://:memory:") {
		return &ConnectionConfig{
			Type:     TypeSQLite,
			FilePath: ":memory:",
			Database: "memory",
			RawURL:   rawURL,
		}, nil
	}

	var filePath string
	if strings.HasPrefix(rawURL, "sqlite://") {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("invalid sqlite URL: %w", err)
		}
		filePath = filepath.Join(u.Host, u.Path)
		if u.Host == "" {
			filePath = u.Path
		}
	} else if strings.HasPrefix(rawURL, "file:") {
		filePath = strings.TrimPrefix(rawURL, "file:")
		if idx := strings.Index(filePath, "?"); idx != -1 {
			filePath = filePath[:idx]
		}
	} else {
		filePath = rawURL
	}

	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil, fmt.Errorf("sqlite file path cannot be empty")
	}

	// Security: Prevent path traversal to sensitive system files
	cleaned := filepath.Clean(filePath)
	lowerClean := strings.ToLower(cleaned)
	if strings.HasPrefix(lowerClean, "/etc") || strings.HasPrefix(lowerClean, "/proc") ||
		strings.HasPrefix(lowerClean, "/sys") || strings.HasPrefix(lowerClean, "/dev") ||
		strings.HasPrefix(lowerClean, "/root") || strings.Contains(lowerClean, "/.ssh") ||
		strings.Contains(lowerClean, "/.env") || strings.Contains(lowerClean, "/.git") ||
		strings.HasPrefix(lowerClean, "/var/run/secrets") {
		return nil, fmt.Errorf("access to sensitive system directory is forbidden: %s", cleaned)
	}

	dbName := filepath.Base(cleaned)

	return &ConnectionConfig{
		Type:     TypeSQLite,
		FilePath: cleaned,
		Database: dbName,
		RawURL:   rawURL,
	}, nil
}

func (c *SQLiteConnector) openDB(cfg *ConnectionConfig) (*sql.DB, error) {
	var dsn string
	if cfg.FilePath == ":memory:" {
		dsn = ":memory:"
	} else {
		// Open with read-only mode where possible
		dsn = fmt.Sprintf("file:%s?mode=ro", cfg.FilePath)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite connection: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite single-writer safe
	return db, nil
}

func (c *SQLiteConnector) TestConnection(ctx context.Context, cfg *ConnectionConfig) (*ConnectionTestResult, error) {
	start := time.Now()

	// Check if file exists on disk if not memory
	if cfg.FilePath != ":memory:" {
		if _, err := os.Stat(cfg.FilePath); os.IsNotExist(err) {
			return &ConnectionTestResult{
				Success:      false,
				DatabaseType: string(TypeSQLite),
				Latency:      time.Since(start),
				LatencyMs:    time.Since(start).Milliseconds(),
				ErrorMessage: fmt.Sprintf("sqlite file does not exist: %s", cfg.FilePath),
			}, nil
		}
	}

	db, err := c.openDB(cfg)
	if err != nil {
		return &ConnectionTestResult{
			Success:      false,
			DatabaseType: string(TypeSQLite),
			Latency:      time.Since(start),
			LatencyMs:    time.Since(start).Milliseconds(),
			ErrorMessage: c.SanitizeError(err).Error(),
		}, nil
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var version string
	err = db.QueryRowContext(pingCtx, "SELECT sqlite_version();").Scan(&version)
	duration := time.Since(start)

	if err != nil {
		return &ConnectionTestResult{
			Success:      false,
			DatabaseType: string(TypeSQLite),
			Latency:      duration,
			LatencyMs:    duration.Milliseconds(),
			ErrorMessage: c.SanitizeError(err).Error(),
		}, nil
	}

	return &ConnectionTestResult{
		Success:         true,
		DatabaseType:    string(TypeSQLite),
		DatabaseVersion: version,
		Latency:         duration,
		LatencyMs:       duration.Milliseconds(),
	}, nil
}

func (c *SQLiteConnector) DiscoverMetadata(ctx context.Context, cfg *ConnectionConfig) (*DatabaseMetadata, error) {
	return c.ExtractSchema(ctx, cfg, DefaultExtractionConfig())
}

func (c *SQLiteConnector) ExtractSchema(ctx context.Context, cfg *ConnectionConfig, opts ExtractionConfig) (*DatabaseMetadata, error) {
	db, err := c.openDB(cfg)
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer db.Close()

	var version string
	_ = db.QueryRowContext(ctx, "SELECT sqlite_version();").Scan(&version)

	tableCond := "type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'"
	if len(opts.Tables) > 0 {
		var quoted []string
		for _, t := range opts.Tables {
			cleanT := strings.ReplaceAll(t, "\x00", "")
			quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(cleanT, "'", "''")))
		}
		tableCond = fmt.Sprintf("%s AND name IN (%s)", tableCond, strings.Join(quoted, ","))
	}

	// 1. Tables and views
	queryTables := fmt.Sprintf(`
		SELECT name, type
		FROM sqlite_master
		WHERE %s
		ORDER BY name;
	`, tableCond)

	rows, err := db.QueryContext(ctx, queryTables)
	if err != nil {
		return nil, c.SanitizeError(fmt.Errorf("querying sqlite tables: %w", err))
	}
	defer rows.Close()

	var tables []TableMetadata

	for rows.Next() {
		var name, objType string
		if err := rows.Scan(&name, &objType); err != nil {
			return nil, c.SanitizeError(err)
		}
		tables = append(tables, TableMetadata{
			Schema:  "main",
			Name:    name,
			Type:    objType,
			Columns: make([]ColumnMetadata, 0),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, c.SanitizeError(err)
	}

	// 2. Discover columns, primary keys, foreign keys per table
	for i := range tables {
		tName := tables[i].Name

		// Columns from PRAGMA table_info
		pragmaInfo := fmt.Sprintf(`PRAGMA table_info(%s);`, quoteSQLiteIdent(tName))
		infoRows, err := db.QueryContext(ctx, pragmaInfo)
		if err == nil {
			for infoRows.Next() {
				var cid, pk int
				var colName, colType string
				var notNull bool
				var dfltValue sql.NullString
				if err := infoRows.Scan(&cid, &colName, &colType, &notNull, &dfltValue, &pk); err == nil {
					isPK := pk > 0
					if isPK {
						tables[i].PrimaryKey = append(tables[i].PrimaryKey, colName)
					}
					tables[i].Columns = append(tables[i].Columns, ColumnMetadata{
						Name:         colName,
						DataType:     colType,
						Nullable:     !notNull,
						DefaultValue: dfltValue.String,
						Position:     cid,
						IsPrimaryKey: isPK,
						IsSensitive:  IsSensitiveColumn(colName),
					})
				}
			}
			if err := infoRows.Err(); err != nil {
				infoRows.Close()
				return nil, c.SanitizeError(err)
			}
			infoRows.Close()
		}

		// Foreign keys from PRAGMA foreign_key_list
		pragmaFK := fmt.Sprintf(`PRAGMA foreign_key_list(%s);`, quoteSQLiteIdent(tName))
		fkRows, err := db.QueryContext(ctx, pragmaFK)
		if err == nil {
			for fkRows.Next() {
				var id, seq int
				var refTable, fromCol, toCol, onUpdate, onDelete, match string
				if err := fkRows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match); err == nil {
					tables[i].ForeignKeys = append(tables[i].ForeignKeys, ForeignKeyMetadata{
						Columns:           []string{fromCol},
						ReferencedSchema:  "main",
						ReferencedTable:   refTable,
						ReferencedColumns: []string{toCol},
					})
				}
			}
			if err := fkRows.Err(); err != nil {
				fkRows.Close()
				return nil, c.SanitizeError(err)
			}
			fkRows.Close()
		}

		// Indexes from PRAGMA index_list
		pragmaIdx := fmt.Sprintf(`PRAGMA index_list(%s);`, quoteSQLiteIdent(tName))
		idxRows, err := db.QueryContext(ctx, pragmaIdx)
		if err == nil {
			for idxRows.Next() {
				var seq, unique int
				var idxName, origin, partial string
				if err := idxRows.Scan(&seq, &idxName, &unique, &origin, &partial); err == nil {
					tables[i].Indexes = append(tables[i].Indexes, IndexMetadata{
						Name:   idxName,
						Unique: unique == 1,
					})
				}
			}
			if err := idxRows.Err(); err != nil {
				idxRows.Close()
				return nil, c.SanitizeError(err)
			}
			idxRows.Close()
		}
	}

	return &DatabaseMetadata{
		DatabaseType: string(TypeSQLite),
		DatabaseName: cfg.Database,
		Version:      version,
		Schemas: []SchemaMetadata{
			{
				Name:   "main",
				Tables: tables,
			},
		},
	}, nil
}

func (c *SQLiteConnector) ExtractRows(ctx context.Context, cfg *ConnectionConfig, schemaName, tableName string, columns []string, limit, offset int) ([]TableRecord, error) {
	if len(columns) == 0 {
		return nil, nil
	}

	db, err := c.openDB(cfg)
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer db.Close()

	safeTable := quoteSQLiteIdent(tableName)
	var safeCols []string
	for _, col := range columns {
		safeCols = append(safeCols, quoteSQLiteIdent(col))
	}

	if limit <= 0 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf("SELECT %s FROM %s LIMIT ? OFFSET ?;",
		strings.Join(safeCols, ", "),
		safeTable,
	)

	rows, err := db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, c.SanitizeError(err)
	}

	var records []TableRecord
	rowIndex := offset

	for rows.Next() {
		vals := make([]any, len(colNames))
		valPtrs := make([]any, len(colNames))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		if err := rows.Scan(valPtrs...); err != nil {
			return nil, c.SanitizeError(err)
		}

		recValues := make(map[string]any, len(colNames))
		for i, name := range colNames {
			val := vals[i]
			if b, ok := val.([]byte); ok {
				recValues[name] = string(b)
			} else {
				recValues[name] = val
			}
		}

		records = append(records, TableRecord{
			Schema:   "main",
			Table:    tableName,
			RowIndex: rowIndex,
			Values:   recValues,
		})
		rowIndex++
	}

	return records, rows.Err()
}

func (c *SQLiteConnector) SanitizeError(err error) error {
	return SanitizeConnectionError(err, "")
}

func quoteSQLiteIdent(name string) string {
	clean := strings.ReplaceAll(name, "\x00", "")
	return `"` + strings.ReplaceAll(clean, `"`, `""`) + `"`
}
