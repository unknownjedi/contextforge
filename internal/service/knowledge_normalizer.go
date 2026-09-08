package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/your-org/contextforge/internal/connector"
	"github.com/your-org/contextforge/internal/ingest"
)

// KnowledgeNormalizer converts external database metadata and records into standard knowledge documents.
type KnowledgeNormalizer struct{}

// NewKnowledgeNormalizer constructs a KnowledgeNormalizer.
func NewKnowledgeNormalizer() *KnowledgeNormalizer {
	return &KnowledgeNormalizer{}
}

// GenerateSchemaDocuments transforms DatabaseMetadata into indexed ScannedFile documents per table.
func (n *KnowledgeNormalizer) GenerateSchemaDocuments(meta *connector.DatabaseMetadata) []*ingest.ScannedFile {
	var docs []*ingest.ScannedFile

	for _, schema := range meta.Schemas {
		for _, table := range schema.Tables {
			content := n.formatTableDocument(meta.DatabaseType, meta.DatabaseName, &table)
			path := fmt.Sprintf("schema/%s/%s.sql", schema.Name, table.Name)
			hash := sha256Hex(content)

			docs = append(docs, &ingest.ScannedFile{
				Path:        path,
				Language:    "sql",
				Content:     content,
				ContentHash: hash,
				SizeBytes:   int64(len(content)),
			})
		}
	}

	return docs
}

// GenerateRowBatchDocument transforms a batch of table records into a knowledge document.
func (n *KnowledgeNormalizer) GenerateRowBatchDocument(
	databaseType, databaseName, schemaName, tableName string,
	batchIndex int,
	records []connector.TableRecord,
) *ingest.ScannedFile {
	if len(records) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("-- Database Records: %s.%s (Batch %d)\n", schemaName, tableName, batchIndex))
	sb.WriteString(fmt.Sprintf("-- Database: %s | Engine: %s | Total Records: %d\n\n", databaseName, databaseType, len(records)))

	for _, rec := range records {
		data, err := json.Marshal(rec.Values)
		if err == nil {
			sb.WriteString(string(data))
			sb.WriteString("\n")
		}
	}

	content := sb.String()
	path := fmt.Sprintf("data/%s/%s_batch_%d.jsonl", schemaName, tableName, batchIndex)
	hash := sha256Hex(content)

	return &ingest.ScannedFile{
		Path:        path,
		Language:    "json",
		Content:     content,
		ContentHash: hash,
		SizeBytes:   int64(len(content)),
	}
}

func (n *KnowledgeNormalizer) formatTableDocument(dbType, dbName string, t *connector.TableMetadata) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("-- ========================================================\n"))
	sb.WriteString(fmt.Sprintf("-- Table: %s.%s\n", t.Schema, t.Name))
	sb.WriteString(fmt.Sprintf("-- Database: %s | Engine: %s | Type: %s\n", dbName, dbType, t.Type))
	if t.Comment != "" {
		sb.WriteString(fmt.Sprintf("-- Description: %s\n", t.Comment))
	}
	sb.WriteString(fmt.Sprintf("-- ========================================================\n\n"))

	// DDL Definition
	sb.WriteString(fmt.Sprintf("CREATE %s %s.%s (\n", strings.ToUpper(t.Type), t.Schema, t.Name))
	for i, col := range t.Columns {
		sb.WriteString(fmt.Sprintf("    %s %s", col.Name, col.DataType))
		if !col.Nullable {
			sb.WriteString(" NOT NULL")
		}
		if col.DefaultValue != "" {
			sb.WriteString(fmt.Sprintf(" DEFAULT %s", col.DefaultValue))
		}
		if col.IsPrimaryKey && len(t.PrimaryKey) == 1 {
			sb.WriteString(" PRIMARY KEY")
		}
		if col.Comment != "" {
			sb.WriteString(fmt.Sprintf(" -- %s", col.Comment))
		}
		if i < len(t.Columns)-1 || len(t.PrimaryKey) > 1 || len(t.ForeignKeys) > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	// Composite primary key
	if len(t.PrimaryKey) > 1 {
		sb.WriteString(fmt.Sprintf("    PRIMARY KEY (%s)", strings.Join(t.PrimaryKey, ", ")))
		if len(t.ForeignKeys) > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	// Foreign Keys
	for i, fk := range t.ForeignKeys {
		sb.WriteString(fmt.Sprintf("    FOREIGN KEY (%s) REFERENCES %s.%s(%s)",
			strings.Join(fk.Columns, ", "),
			fk.ReferencedSchema,
			fk.ReferencedTable,
			strings.Join(fk.ReferencedColumns, ", "),
		))
		if i < len(t.ForeignKeys)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(");\n\n")

	// Indexes
	if len(t.Indexes) > 0 {
		sb.WriteString("-- Indexes:\n")
		for _, idx := range t.Indexes {
			uniqStr := ""
			if idx.Unique {
				uniqStr = "UNIQUE "
			}
			colStr := strings.Join(idx.Columns, ", ")
			if colStr == "" {
				colStr = "..."
			}
			sb.WriteString(fmt.Sprintf("CREATE %sINDEX %s ON %s.%s (%s);\n",
				uniqStr, idx.Name, t.Schema, t.Name, colStr))
		}
		sb.WriteString("\n")
	}

	// Relationships Summary (easy retrieval for RAG)
	if len(t.ForeignKeys) > 0 {
		sb.WriteString("-- Relationships:\n")
		for _, fk := range t.ForeignKeys {
			sb.WriteString(fmt.Sprintf("--   %s.%s.%s -> %s.%s.%s\n",
				t.Schema, t.Name, strings.Join(fk.Columns, ","),
				fk.ReferencedSchema, fk.ReferencedTable, strings.Join(fk.ReferencedColumns, ","),
			))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}
