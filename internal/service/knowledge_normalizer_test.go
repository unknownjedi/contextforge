package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/contextforge/internal/connector"
)

func TestKnowledgeNormalizer_GenerateSchemaDocuments(t *testing.T) {
	norm := NewKnowledgeNormalizer()

	meta := &connector.DatabaseMetadata{
		DatabaseType: "postgres",
		DatabaseName: "ecommerce_db",
		Version:      "PostgreSQL 16.2",
		Schemas: []connector.SchemaMetadata{
			{
				Name: "public",
				Tables: []connector.TableMetadata{
					{
						Schema:  "public",
						Name:    "users",
						Type:    "table",
						Comment: "Application user accounts",
						Columns: []connector.ColumnMetadata{
							{Name: "id", DataType: "uuid", Nullable: false, IsPrimaryKey: true, Position: 1},
							{Name: "email", DataType: "varchar", Nullable: false, Position: 2},
							{Name: "password_hash", DataType: "varchar", Nullable: false, IsSensitive: true, Position: 3},
						},
						PrimaryKey: []string{"id"},
						Indexes: []connector.IndexMetadata{
							{Name: "idx_users_email", Columns: []string{"email"}, Unique: true},
						},
					},
					{
						Schema: "public",
						Name:   "orders",
						Type:   "table",
						Columns: []connector.ColumnMetadata{
							{Name: "id", DataType: "bigint", Nullable: false, IsPrimaryKey: true, Position: 1},
							{Name: "user_id", DataType: "uuid", Nullable: false, Position: 2},
							{Name: "total", DataType: "numeric", Nullable: false, DefaultValue: "0.00", Position: 3},
						},
						PrimaryKey: []string{"id"},
						ForeignKeys: []connector.ForeignKeyMetadata{
							{
								Name:              "fk_orders_user",
								Columns:           []string{"user_id"},
								ReferencedSchema:  "public",
								ReferencedTable:   "users",
								ReferencedColumns: []string{"id"},
							},
						},
					},
				},
			},
		},
	}

	docs := norm.GenerateSchemaDocuments(meta)
	require.Len(t, docs, 2)

	// Verify users table doc
	usersDoc := docs[0]
	assert.Equal(t, "schema/public/users.sql", usersDoc.Path)
	assert.Equal(t, "sql", usersDoc.Language)
	assert.NotEmpty(t, usersDoc.ContentHash)
	assert.Contains(t, usersDoc.Content, "CREATE TABLE public.users")
	assert.Contains(t, usersDoc.Content, "id uuid NOT NULL PRIMARY KEY")
	assert.Contains(t, usersDoc.Content, "idx_users_email")

	// Verify orders table doc
	ordersDoc := docs[1]
	assert.Equal(t, "schema/public/orders.sql", ordersDoc.Path)
	assert.Contains(t, ordersDoc.Content, "CREATE TABLE public.orders")
	assert.Contains(t, ordersDoc.Content, "FOREIGN KEY (user_id) REFERENCES public.users(id)")
	assert.Contains(t, ordersDoc.Content, "public.orders.user_id -> public.users.id")

	// Verify deterministic hashing
	docsAgain := norm.GenerateSchemaDocuments(meta)
	assert.Equal(t, usersDoc.ContentHash, docsAgain[0].ContentHash)
	assert.Equal(t, ordersDoc.ContentHash, docsAgain[1].ContentHash)
}

func TestKnowledgeNormalizer_GenerateRowBatchDocument(t *testing.T) {
	norm := NewKnowledgeNormalizer()

	records := []connector.TableRecord{
		{
			Schema:   "public",
			Table:    "products",
			RowIndex: 0,
			Values: map[string]any{
				"id":    101,
				"name":  "Mechanical Keyboard",
				"price": 129.99,
			},
		},
		{
			Schema:   "public",
			Table:    "products",
			RowIndex: 1,
			Values: map[string]any{
				"id":    102,
				"name":  "Wireless Mouse",
				"price": 49.99,
			},
		},
	}

	doc := norm.GenerateRowBatchDocument("postgres", "shop", "public", "products", 1, records)
	require.NotNil(t, doc)
	assert.Equal(t, "data/public/products_batch_1.jsonl", doc.Path)
	assert.Equal(t, "json", doc.Language)
	assert.NotEmpty(t, doc.ContentHash)
	assert.Contains(t, doc.Content, "Mechanical Keyboard")
	assert.Contains(t, doc.Content, "Wireless Mouse")

	lines := strings.Split(strings.TrimSpace(doc.Content), "\n")
	assert.GreaterOrEqual(t, len(lines), 3) // header comments + 2 JSON lines
}
