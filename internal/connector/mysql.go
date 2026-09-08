package connector

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLConnector implements DatabaseConnector for MySQL and MariaDB.
type MySQLConnector struct {
	dbType DatabaseType
}

// NewMySQLConnector constructs a connector for MySQL or MariaDB.
func NewMySQLConnector(dbType DatabaseType) *MySQLConnector {
	if dbType == "" {
		dbType = TypeMySQL
	}
	return &MySQLConnector{dbType: dbType}
}

func (c *MySQLConnector) Type() DatabaseType {
	return c.dbType
}

func (c *MySQLConnector) ParseURL(rawURL string) (*ConnectionConfig, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid connection URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "mysql" && scheme != "mariadb" {
		return nil, fmt.Errorf("%w: %s (expected mysql or mariadb)", ErrUnsupportedScheme, scheme)
	}

	host := u.Hostname()
	port := ParsePort(u.Port(), 3306)
	dbName := strings.TrimPrefix(u.Path, "/")

	var username, password string
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}

	options := make(map[string]string)
	for k, v := range u.Query() {
		if len(v) > 0 {
			options[k] = v[0]
		}
	}

	return &ConnectionConfig{
		Type:     c.dbType,
		Host:     host,
		Port:     port,
		Database: dbName,
		Username: username,
		Password: password,
		Options:  options,
		RawURL:   rawURL,
	}, nil
}

func (c *MySQLConnector) openDB(cfg *ConnectionConfig) (*sql.DB, error) {
	// Format DSN: [username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]
	auth := cfg.Username
	if cfg.Password != "" {
		auth = fmt.Sprintf("%s:%s", cfg.Username, cfg.Password)
	}

	params := url.Values{}
	params.Set("timeout", "5s")
	params.Set("readTimeout", "30s")
	params.Set("parseTime", "true")
	for k, v := range cfg.Options {
		params.Set(k, v)
	}

	dsn := fmt.Sprintf("%s@tcp(%s:%d)/%s?%s",
		auth,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		params.Encode(),
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening mysql connection: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

func (c *MySQLConnector) TestConnection(ctx context.Context, cfg *ConnectionConfig) (*ConnectionTestResult, error) {
	start := time.Now()
	db, err := c.openDB(cfg)
	if err != nil {
		return &ConnectionTestResult{
			Success:      false,
			DatabaseType: string(c.dbType),
			Latency:      time.Since(start),
			LatencyMs:    time.Since(start).Milliseconds(),
			ErrorMessage: c.SanitizeError(err).Error(),
		}, nil
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var version string
	err = db.QueryRowContext(pingCtx, "SELECT VERSION();").Scan(&version)
	duration := time.Since(start)

	if err != nil {
		return &ConnectionTestResult{
			Success:      false,
			DatabaseType: string(c.dbType),
			Latency:      duration,
			LatencyMs:    duration.Milliseconds(),
			ErrorMessage: c.SanitizeError(err).Error(),
		}, nil
	}

	return &ConnectionTestResult{
		Success:         true,
		DatabaseType:    string(c.dbType),
		DatabaseVersion: version,
		Latency:         duration,
		LatencyMs:       duration.Milliseconds(),
	}, nil
}

func (c *MySQLConnector) DiscoverMetadata(ctx context.Context, cfg *ConnectionConfig) (*DatabaseMetadata, error) {
	return c.ExtractSchema(ctx, cfg, DefaultExtractionConfig())
}

func (c *MySQLConnector) ExtractSchema(ctx context.Context, cfg *ConnectionConfig, opts ExtractionConfig) (*DatabaseMetadata, error) {
	db, err := c.openDB(cfg)
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer db.Close()

	var version string
	_ = db.QueryRowContext(ctx, "SELECT VERSION();").Scan(&version)

	schemaCond := "table_schema NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')"
	if cfg.Database != "" {
		cleanDB := strings.ReplaceAll(cfg.Database, "\x00", "")
		schemaCond = fmt.Sprintf("table_schema = '%s'", strings.ReplaceAll(cleanDB, "'", "''"))
	}
	if len(opts.Schemas) > 0 {
		var quoted []string
		for _, s := range opts.Schemas {
			cleanS := strings.ReplaceAll(s, "\x00", "")
			quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(cleanS, "'", "''")))
		}
		schemaCond = fmt.Sprintf("table_schema IN (%s)", strings.Join(quoted, ","))
	}

	tableCond := "1=1"
	if len(opts.Tables) > 0 {
		var quoted []string
		for _, t := range opts.Tables {
			cleanT := strings.ReplaceAll(t, "\x00", "")
			quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(cleanT, "'", "''")))
		}
		tableCond = fmt.Sprintf("table_name IN (%s)", strings.Join(quoted, ","))
	}

	// 1. Tables
	queryTables := fmt.Sprintf(`
		SELECT table_schema, table_name, table_type, COALESCE(table_comment, '')
		FROM information_schema.tables
		WHERE %s AND %s
		ORDER BY table_schema, table_name;
	`, schemaCond, tableCond)

	rows, err := db.QueryContext(ctx, queryTables)
	if err != nil {
		return nil, c.SanitizeError(fmt.Errorf("querying mysql tables: %w", err))
	}
	defer rows.Close()

	type tableKey struct {
		schema string
		table  string
	}
	tableMap := make(map[tableKey]*TableMetadata)
	var orderedKeys []tableKey

	for rows.Next() {
		var s, t, typ, comment string
		if err := rows.Scan(&s, &t, &typ, &comment); err != nil {
			return nil, c.SanitizeError(err)
		}
		cleanType := "table"
		if strings.Contains(strings.ToLower(typ), "view") {
			cleanType = "view"
		}
		k := tableKey{schema: s, table: t}
		tableMap[k] = &TableMetadata{
			Schema:  s,
			Name:    t,
			Type:    cleanType,
			Comment: comment,
			Columns: make([]ColumnMetadata, 0),
		}
		orderedKeys = append(orderedKeys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, c.SanitizeError(err)
	}

	// 2. Columns
	queryColumns := fmt.Sprintf(`
		SELECT table_schema, table_name, column_name, column_type, is_nullable, 
		       COALESCE(column_default, ''), COALESCE(column_comment, ''), ordinal_position
		FROM information_schema.columns
		WHERE %s AND %s
		ORDER BY table_schema, table_name, ordinal_position;
	`, schemaCond, tableCond)

	colRows, err := db.QueryContext(ctx, queryColumns)
	if err == nil {
		defer colRows.Close()
		for colRows.Next() {
			var s, t, colName, colType, isNull, colDef, colComment string
			var pos int
			if err := colRows.Scan(&s, &t, &colName, &colType, &isNull, &colDef, &colComment, &pos); err == nil {
				k := tableKey{schema: s, table: t}
				if tm, ok := tableMap[k]; ok {
					tm.Columns = append(tm.Columns, ColumnMetadata{
						Name:         colName,
						DataType:     colType,
						Nullable:     strings.EqualFold(isNull, "YES"),
						DefaultValue: colDef,
						Comment:      colComment,
						Position:     pos,
						IsSensitive:  IsSensitiveColumn(colName),
					})
				}
			}
		}
		if err := colRows.Err(); err != nil {
			return nil, c.SanitizeError(err)
		}
	}

	// 3. Keys and Constraints
	queryKeys := fmt.Sprintf(`
		SELECT table_schema, table_name, constraint_name, column_name, 
		       COALESCE(referenced_table_schema, ''), COALESCE(referenced_table_name, ''), COALESCE(referenced_column_name, '')
		FROM information_schema.key_column_usage
		WHERE %s AND %s
		ORDER BY table_schema, table_name, ordinal_position;
	`, schemaCond, tableCond)

	keyRows, err := db.QueryContext(ctx, queryKeys)
	if err == nil {
		defer keyRows.Close()
		for keyRows.Next() {
			var s, t, cName, col, refS, refT, refCol string
			if err := keyRows.Scan(&s, &t, &cName, &col, &refS, &refT, &refCol); err == nil {
				k := tableKey{schema: s, table: t}
				if tm, ok := tableMap[k]; ok {
					if cName == "PRIMARY" {
						tm.PrimaryKey = append(tm.PrimaryKey, col)
						for i := range tm.Columns {
							if tm.Columns[i].Name == col {
								tm.Columns[i].IsPrimaryKey = true
							}
						}
					} else if refT != "" {
						tm.ForeignKeys = append(tm.ForeignKeys, ForeignKeyMetadata{
							Name:              cName,
							Columns:           []string{col},
							ReferencedSchema:  refS,
							ReferencedTable:   refT,
							ReferencedColumns: []string{refCol},
						})
					}
				}
			}
		}
		if err := keyRows.Err(); err != nil {
			return nil, c.SanitizeError(err)
		}
	}

	// Group into SchemaMetadata hierarchy
	schemaGroups := make(map[string][]TableMetadata)
	for _, k := range orderedKeys {
		tm := tableMap[k]
		schemaGroups[tm.Schema] = append(schemaGroups[tm.Schema], *tm)
	}

	var schemas []SchemaMetadata
	for sName, tables := range schemaGroups {
		schemas = append(schemas, SchemaMetadata{
			Name:   sName,
			Tables: tables,
		})
	}

	return &DatabaseMetadata{
		DatabaseType: string(c.dbType),
		DatabaseName: cfg.Database,
		Version:      version,
		Schemas:      schemas,
	}, nil
}

func (c *MySQLConnector) ExtractRows(ctx context.Context, cfg *ConnectionConfig, schemaName, tableName string, columns []string, limit, offset int) ([]TableRecord, error) {
	if len(columns) == 0 {
		return nil, nil
	}

	db, err := c.openDB(cfg)
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer db.Close()

	safeSchema := quoteMySQLIdent(schemaName)
	safeTable := quoteMySQLIdent(tableName)

	var safeCols []string
	for _, col := range columns {
		safeCols = append(safeCols, quoteMySQLIdent(col))
	}

	if limit <= 0 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	var target string
	if schemaName != "" {
		target = fmt.Sprintf("%s.%s", safeSchema, safeTable)
	} else {
		target = safeTable
	}

	query := fmt.Sprintf("SELECT %s FROM %s LIMIT ? OFFSET ?;",
		strings.Join(safeCols, ", "),
		target,
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
			Schema:   schemaName,
			Table:    tableName,
			RowIndex: rowIndex,
			Values:   recValues,
		})
		rowIndex++
	}

	return records, rows.Err()
}

func (c *MySQLConnector) SanitizeError(err error) error {
	return SanitizeConnectionError(err, "")
}

func quoteMySQLIdent(name string) string {
	clean := strings.ReplaceAll(name, "\x00", "")
	return "`" + strings.ReplaceAll(clean, "`", "``") + "`"
}
