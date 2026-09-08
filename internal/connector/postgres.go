package connector

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresConnector implements DatabaseConnector for PostgreSQL and CockroachDB.
type PostgresConnector struct {
	dbType DatabaseType
}

// NewPostgresConnector constructs a connector for PostgreSQL or CockroachDB.
func NewPostgresConnector(dbType DatabaseType) *PostgresConnector {
	if dbType == "" {
		dbType = TypePostgres
	}
	return &PostgresConnector{dbType: dbType}
}

func (c *PostgresConnector) Type() DatabaseType {
	return c.dbType
}

func (c *PostgresConnector) ParseURL(rawURL string) (*ConnectionConfig, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid connection URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "postgres" && scheme != "postgresql" && scheme != "cockroachdb" {
		return nil, fmt.Errorf("%w: %s (expected postgres, postgresql, or cockroachdb)", ErrUnsupportedScheme, scheme)
	}

	host := u.Hostname()
	port := ParsePort(u.Port(), 5432)
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		dbName = "postgres"
	}

	var username, password string
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}

	q := u.Query()
	sslMode := q.Get("sslmode")
	if sslMode == "" {
		sslMode = "prefer"
	}

	options := make(map[string]string)
	for k, v := range q {
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
		SSLMode:  sslMode,
		Options:  options,
		RawURL:   rawURL,
	}, nil
}

func (c *PostgresConnector) openDB(cfg *ConnectionConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s&connect_timeout=10",
		url.QueryEscape(cfg.Username),
		url.QueryEscape(cfg.Password),
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.SSLMode,
	)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

func (c *PostgresConnector) TestConnection(ctx context.Context, cfg *ConnectionConfig) (*ConnectionTestResult, error) {
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
	err = db.QueryRowContext(pingCtx, "SELECT version();").Scan(&version)
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

func (c *PostgresConnector) DiscoverMetadata(ctx context.Context, cfg *ConnectionConfig) (*DatabaseMetadata, error) {
	return c.ExtractSchema(ctx, cfg, DefaultExtractionConfig())
}

func (c *PostgresConnector) ExtractSchema(ctx context.Context, cfg *ConnectionConfig, opts ExtractionConfig) (*DatabaseMetadata, error) {
	db, err := c.openDB(cfg)
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, c.SanitizeError(err)
	}
	defer func() { _ = tx.Rollback() }()

	var version string
	_ = tx.QueryRowContext(ctx, "SELECT version();").Scan(&version)

	// Helper to build safe schema and table filter clauses per table alias
	buildFilter := func(schemaCol, tableCol string) (string, string) {
		sCond := fmt.Sprintf("%s NOT IN ('pg_catalog', 'information_schema', 'crdb_internal')", schemaCol)
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

	schemaCond, tableCond := buildFilter("table_schema", "table_name")

	// 1. Discover Tables and Views
	queryTables := fmt.Sprintf(`
		SELECT table_schema, table_name, table_type
		FROM information_schema.tables
		WHERE %s AND %s
		ORDER BY table_schema, table_name;
	`, schemaCond, tableCond)

	rows, err := tx.QueryContext(ctx, queryTables)
	if err != nil {
		return nil, c.SanitizeError(fmt.Errorf("querying tables: %w", err))
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

	// 2. Discover Columns
	queryColumns := fmt.Sprintf(`
		SELECT table_schema, table_name, column_name, data_type, is_nullable, 
		       COALESCE(column_default, ''), ordinal_position
		FROM information_schema.columns
		WHERE %s AND %s
		ORDER BY table_schema, table_name, ordinal_position;
	`, schemaCond, tableCond)

	colRows, err := tx.QueryContext(ctx, queryColumns)
	if err == nil {
		defer colRows.Close()
		for colRows.Next() {
			var s, t, colName, dataType, isNullable, colDef string
			var pos int
			if err := colRows.Scan(&s, &t, &colName, &dataType, &isNullable, &colDef, &pos); err == nil {
				k := tableKey{schema: s, table: t}
				if tm, ok := tableMap[k]; ok {
					tm.Columns = append(tm.Columns, ColumnMetadata{
						Name:         colName,
						DataType:     dataType,
						Nullable:     strings.EqualFold(isNullable, "YES"),
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

	// 3. Discover Primary Keys
	tcSchemaCond, tcTableCond := buildFilter("tc.table_schema", "tc.table_name")
	queryPKs := fmt.Sprintf(`
		SELECT tc.table_schema, tc.table_name, kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		  AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY' AND %s AND %s
		ORDER BY tc.table_schema, tc.table_name, kcu.ordinal_position;
	`, tcSchemaCond, tcTableCond)

	pkRows, err := tx.QueryContext(ctx, queryPKs)
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

	// 4. Discover Foreign Keys
	queryFKs := fmt.Sprintf(`
		SELECT
			tc.table_schema,
			tc.table_name,
			tc.constraint_name,
			kcu.column_name,
			ccu.table_schema AS referenced_schema,
			ccu.table_name AS referenced_table,
			ccu.column_name AS referenced_column
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
		  ON tc.constraint_name = kcu.constraint_name
		  AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
		  ON ccu.constraint_name = tc.constraint_name
		  AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY' AND %s AND %s;
	`, tcSchemaCond, tcTableCond)

	fkRows, err := tx.QueryContext(ctx, queryFKs)
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var s, t, cName, col, refS, refT, refCol string
			if err := fkRows.Scan(&s, &t, &cName, &col, &refS, &refT, &refCol); err == nil {
				k := tableKey{schema: s, table: t}
				if tm, ok := tableMap[k]; ok {
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
		if err := fkRows.Err(); err != nil {
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

func (c *PostgresConnector) ExtractRows(ctx context.Context, cfg *ConnectionConfig, schemaName, tableName string, columns []string, limit, offset int) ([]TableRecord, error) {
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
		safeCols = append(safeCols, quotePgIdent(col))
	}

	if limit <= 0 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	var target string
	if schemaName != "" {
		target = fmt.Sprintf("%s.%s", quotePgIdent(schemaName), quotePgIdent(tableName))
	} else {
		target = quotePgIdent(tableName)
	}

	query := fmt.Sprintf("SELECT %s FROM %s LIMIT $1 OFFSET $2;",
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

func (c *PostgresConnector) SanitizeError(err error) error {
	return SanitizeConnectionError(err, "")
}

func quotePgIdent(name string) string {
	clean := strings.ReplaceAll(name, "\x00", "")
	return `"` + strings.ReplaceAll(clean, `"`, `""`) + `"`
}
