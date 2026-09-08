package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Project holds the schema definition for the Project entity.
type Project struct {
	ent.Schema
}

// Fields of the Project.
func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("name").
			NotEmpty().
			MaxLen(100),
		field.String("description").
			Optional().
			Default("").
			MaxLen(500),
		field.UUID("owner_user_id", uuid.UUID{}),
		field.String("embedding_provider").
			Default("ollama"),
		field.String("embedding_model").
			Default("nomic-embed-text"),
		field.Int("embedding_dimension").
			Default(768),
		field.String("llm_provider").
			Default("cli_opencode"),
		field.String("llm_model").
			Optional().
			Default(""),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Project.
func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).
			Ref("projects").
			Field("owner_user_id").
			Unique().
			Required(),
		edge.To("sources", Source.Type),
		edge.To("documents", Document.Type),
		edge.To("chunks", DocumentChunk.Type),
		edge.To("jobs", IngestionJob.Type),
		edge.To("audit_logs", AuditLog.Type),
	}
}

// Indexes of the Project.
func (Project) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("owner_user_id"),
		index.Fields("name", "owner_user_id").Unique(),
	}
}
