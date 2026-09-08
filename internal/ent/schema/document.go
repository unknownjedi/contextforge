package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Document holds the schema definition for the Document entity.
type Document struct {
	ent.Schema
}

// Fields of the Document.
func (Document) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.UUID("source_id", uuid.UUID{}),
		field.String("file_path").
			NotEmpty(),
		field.String("language").
			Default("text"),
		field.String("content_hash").
			NotEmpty(),
		field.Int("total_chunks").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Document.
func (Document) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("documents").
			Field("project_id").
			Unique().
			Required(),
		edge.From("source", Source.Type).
			Ref("documents").
			Field("source_id").
			Unique().
			Required(),
		edge.To("chunks", DocumentChunk.Type),
	}
}

// Indexes of the Document.
func (Document) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
		index.Fields("source_id"),
		index.Fields("project_id", "source_id", "file_path").Unique(),
		index.Fields("project_id", "content_hash"),
	}
}
