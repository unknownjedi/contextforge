package connector

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// SQLServerConnector implements DatabaseConnector for Microsoft SQL Server.
type SQLServerConnector struct{}

// NewSQLServerConnector constructs a connector for SQL Server.
func NewSQLServerConnector() *SQLServerConnector {
	return &SQLServerConnector{}
}

func (c *SQLServerConnector) Type() DatabaseType {
	return TypeSQLServer
}

func (c *SQLServerConnector) ParseURL(rawURL string) (*ConnectionConfig, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid sqlserver URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "sqlserver" && scheme != "mssql" {
		return nil, fmt.Errorf("%w: %s (expected sqlserver or mssql)", ErrUnsupportedScheme, scheme)
	}

	host := u.Hostname()
	port := ParsePort(u.Port(), 1433)
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		dbName = u.Query().Get("database")
	}

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
		Type:     TypeSQLServer,
		Host:     host,
		Port:     port,
		Database: dbName,
		Username: username,
		Password: password,
		Options:  options,
		RawURL:   rawURL,
	}, nil
}

func (c *SQLServerConnector) openDB(cfg *ConnectionConfig) (*sql.DB, error) {
	query := url.Values{}
	query.Add("database", cfg.Database)
	query.Add("connection timeout", "10")
	for k, v := range cfg.Options {
		query.Set(k, v)
	}

	u := &url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(cfg.Username, cfg.Password),
		Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		RawQuery: query.Encode(),
	}

	db, err := sql.Open("sqlserver", u.String())
	if err != nil {
		return nil, fmt.Errorf("opening sqlserver connection: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

func (c *SQLServerConnector) TestConnection(ctx context.Context, cfg *ConnectionConfig) (*ConnectionTestResult, error) {
	start := time.Now()
	db, err := c.openDB(cfg)
	if err != nil {
		return &ConnectionTestResult{
			Success:      false,
			DatabaseType: string(TypeSQLServer),
			Latency:      time.Since(start),
			LatencyMs:    time.Since(start).Milliseconds(),
			ErrorMessage: c.SanitizeError(err).Error(),
		}, nil
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var version string
	err = db.QueryRowContext(pingCtx, "SELECT @@VERSION;").Scan(&version)
	duration := time.Since(start)

	if err != nil {
		return &ConnectionTestResult{
			Success:      false,
			DatabaseType: string(TypeSQLServer),
			Latency:      duration,
			LatencyMs:    duration.Milliseconds(),
			ErrorMessage: c.SanitizeError(err).Error(),
		}, nil
	}

	return &ConnectionTestResult{
		Success:         true,
		DatabaseType:    string(TypeSQLServer),
		DatabaseVersion: version,
		Latency:         duration,
		LatencyMs:       duration.Milliseconds(),
	}, nil
}

func (c *SQLServerConnector) DiscoverMetadata(ctx context.Context, cfg *ConnectionConfig) (*DatabaseMetadata, error) {
	return c.ExtractSchema(ctx, cfg, DefaultExtractionConfig())
}

func (c *SQLServerConnector) ExtractSchema(ctx context.Context, cfg *ConnectionConfig, opts ExtractionConfig) (*DatabaseMetadata, error) {
	db, err := c.openDB(cfg)
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer db.Close()

	var version string
	_ = db.QueryRowContext(ctx, "SELECT @@VERSION;").Scan(&version)

	buildFilter := func(schemaCol, tableCol string) (string, string) {
		sCond := fmt.Sprintf("%s NOT IN ('guest', 'INFORMATION_SCHEMA', 'sys', 'db_owner', 'db_securityadmin', 'db_accessadmin')", schemaCol)
		if len(opts.Schemas) > 0 {
			var quoted []string
			for _, s := range opts.Schemas {
				cleanS := strings.ReplaceAll(s, "\x00", "")
				quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(cleanS, "'", "''")))
			}
			sCond = fmt.Sprintf("%s IN (%s)", schemaCol, strings.Join(quoted, ","))
		}

		tCond := "1=1"
		if len(opts.Tables) > 0 {
			var quoted []string
			for _, t := range opts.Tables {
				cleanT := strings.ReplaceAll(t, "\x00", "")
				quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(cleanT, "'", "''")))
			}
			tCond = fmt.Sprintf("%s IN (%s)", tableCol, strings.Join(quoted, ","))
		}
		return sCond, tCond
	}

	schemaCond, tableCond := buildFilter("TABLE_SCHEMA", "TABLE_NAME")

	// 1. Tables
	queryTables := fmt.Sprintf(`
		SELECT TABLE_SCHEMA, TABLE_NAME, TABLE_TYPE
		FROM INFORMATION_SCHEMA.TABLES
		WHERE %s AND %s
		ORDER BY TABLE_SCHEMA, TABLE_NAME;
	`, schemaCond, tableCond)

	rows, err := db.QueryContext(ctx, queryTables)
	if err != nil {
		return nil, c.SanitizeError(fmt.Errorf("querying sqlserver tables: %w", err))
	}
	defer rows.Close()

	type tableKey struct {
		schema string
		table  string
	}
	tableMap := make(map[tableKey]*TableMetadata)
	var orderedKeys []tableKey

	for rows.Next() {
		var s, t, typ string
		if err := rows.Scan(&s, &t, &typ); err != nil {
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
			Columns: make([]ColumnMetadata, 0),
		}
		orderedKeys = append(orderedKeys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, c.SanitizeError(err)
	}

	// 2. Columns
	queryColumns := fmt.Sprintf(`
		SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, DATA_TYPE, IS_NULLABLE, 
		       COALESCE(COLUMN_DEFAULT, ''), ORDINAL_POSITION
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE %s AND %s
		ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION;
	`, schemaCond, tableCond)

	colRows, err := db.QueryContext(ctx, queryColumns)
	if err == nil {
		defer colRows.Close()
		for colRows.Next() {
			var s, t, colName, dataType, isNull, colDef string
			var pos int
			if err := colRows.Scan(&s, &t, &colName, &dataType, &isNull, &colDef, &pos); err == nil {
				k := tableKey{schema: s, table: t}
				if tm, ok := tableMap[k]; ok {
					tm.Columns = append(tm.Columns, ColumnMetadata{
						Name:         colName,
						DataType:     dataType,
						Nullable:     strings.EqualFold(isNull, "YES"),
						DefaultValue: colDef,
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

	// 3. Primary Keys
	tcSchemaCond, tcTableCond := buildFilter("tc.TABLE_SCHEMA", "tc.TABLE_NAME")
	queryPKs := fmt.Sprintf(`
		SELECT tc.TABLE_SCHEMA, tc.TABLE_NAME, kcu.COLUMN_NAME
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
		  ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
		  AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
		WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY' AND %s AND %s;
	`, tcSchemaCond, tcTableCond)

	pkRows, err := db.QueryContext(ctx, queryPKs)
	if err == nil {
		defer pkRows.Close()
		for pkRows.Next() {
			var s, t, col string
			if err := pkRows.Scan(&s, &t, &col); err == nil {
				k := tableKey{schema: s, table: t}
				if tm, ok := tableMap[k]; ok {
					tm.PrimaryKey = append(tm.PrimaryKey, col)
					for i := range tm.Columns {
						if tm.Columns[i].Name == col {
							tm.Columns[i].IsPrimaryKey = true
						}
					}
				}
			}
		}
		if err := pkRows.Err(); err != nil {
			return nil, c.SanitizeError(err)
		}
	}

	// Group into schemas
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
		DatabaseType: string(TypeSQLServer),
		DatabaseName: cfg.Database,
		Version:      version,
		Schemas:      schemas,
	}, nil
}

func (c *SQLServerConnector) ExtractRows(ctx context.Context, cfg *ConnectionConfig, schemaName, tableName string, columns []string, limit, offset int) ([]TableRecord, error) {
	if len(columns) == 0 {
		return nil, nil
	}

	db, err := c.openDB(cfg)
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer db.Close()

	var safeCols []string
	for _, col := range columns {
		safeCols = append(safeCols, quoteMSSQLIdent(col))
	}

	if limit <= 0 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	var target string
	if schemaName != "" {
		target = fmt.Sprintf("%s.%s", quoteMSSQLIdent(schemaName), quoteMSSQLIdent(tableName))
	} else {
		target = quoteMSSQLIdent(tableName)
	}

	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY (SELECT NULL) OFFSET %d ROWS FETCH NEXT %d ROWS ONLY;",
		strings.Join(safeCols, ", "),
		target,
		offset,
		limit,
	)

	rows, err := db.QueryContext(ctx, query)
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

func (c *SQLServerConnector) SanitizeError(err error) error {
	return SanitizeConnectionError(err, "")
}

func quoteMSSQLIdent(name string) string {
	clean := strings.ReplaceAll(name, "\x00", "")
	return "[" + strings.ReplaceAll(clean, "]", "]]") + "]"
}
