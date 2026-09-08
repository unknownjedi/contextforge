package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// DocumentChunk holds the schema definition for the DocumentChunk entity.
type DocumentChunk struct {
	ent.Schema
}

// Fields of the DocumentChunk.
func (DocumentChunk) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.UUID("document_id", uuid.UUID{}),
		field.Int("chunk_index"),
		field.Int("start_line"),
		field.Int("end_line"),
		field.Text("content"),
		field.String("content_hash").
			NotEmpty(),
		field.Int("token_count").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the DocumentChunk.
func (DocumentChunk) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("chunks").
			Field("project_id").
			Unique().
			Required(),
		edge.From("document", Document.Type).
			Ref("chunks").
			Field("document_id").
			Unique().
			Required(),
	}
}

// Indexes of the DocumentChunk.
func (DocumentChunk) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
		index.Fields("document_id"),
		index.Fields("document_id", "chunk_index").Unique(),
		index.Fields("project_id", "content_hash"),
	}
}
